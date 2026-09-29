package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"dialer-go/internal/domain"
)

// handleOriginateResponse vincula o canal Asterisk ao CallID interno no sucesso do Originate.
func (tm *TrunkManager) handleOriginateResponse(ctx context.Context, attrs map[string]string) {
	actionID := attrs["ActionID"]
	response := attrs["Response"]
	astChannel := attrs["Channel"]
	uniqueID := attrs["Uniqueid"]

	callID := actionID
	if strings.HasPrefix(callID, "act-") {
		callID = strings.TrimPrefix(callID, "act-")
	} else if strings.HasPrefix(callID, "orig-") {
		callID = strings.TrimPrefix(callID, "orig-")
	}

	if response != "Success" {
		if callID != "" {
			tm.channels.ReleaseSlot(ctx, callID)
			if tm.cache != nil {
				evt := &domain.CDREvent{
					CallID:      callID,
					Disposition: domain.DispositionFailed,
					Type:        domain.CDREventOriginateFailed,
					Timestamp:   time.Now().Unix(),
				}
				_ = tm.cache.PublishCDREvent(ctx, evt)
			}
		}
	} else {
		if callID != "" {
			tm.channels.LinkAsteriskChannel(callID, astChannel, uniqueID)
		}
	}
}

// handleVarSet captura variáveis de canal propagadas pelo Asterisk Dialplan.
func (tm *TrunkManager) handleVarSet(ctx context.Context, attrs map[string]string) {
	variable := attrs["Variable"]
	value := attrs["Value"]
	astChannel := attrs["Channel"]
	uniqueID := attrs["Uniqueid"]

	if variable == "CALL_ID" {
		if value != "" {
			tm.channels.LinkAsteriskChannel(value, astChannel, uniqueID)
		}
	} else if variable == "REC_FILE" || variable == "REC_FILENAME" || variable == "MIXMONITOR_FILENAME" {
		if value != "" {
			tm.channels.SetRecordingFile(astChannel, uniqueID, value)
		}
	} else if variable == "VOSK_TRANSCRIPTION" || variable == "VOSK_AMD_TEXT" {
		tm.handleTranscriptionUpdate(ctx, astChannel, uniqueID, value)
	}
}

// handleHangup consolida a tarifação, gravação e disposição final do canal antes de desalocar o slot.
func (tm *TrunkManager) handleHangup(ctx context.Context, attrs map[string]string) {
	astChannel := attrs["Channel"]
	uniqueID := attrs["Uniqueid"]
	activeChan := tm.channels.ReleaseByAsterisk(ctx, astChannel, uniqueID)
	if activeChan == nil {
		return
	}

	tm.mu.RLock()
	repRepo := tm.reportRepo
	routRepo := tm.routing
	tm.mu.RUnlock()

	causeStr := attrs["Cause"]
	causeInt, _ := strconv.Atoi(causeStr)

	disposition := domain.DispositionFailed
	if activeChan.Disposition != nil {
		disposition = *activeChan.Disposition
	} else {
		switch causeInt {
		case 16: // Normal Clearing
			if activeChan.IsAnswered {
				if activeChan.AgentID != nil {
					disposition = domain.DispositionDelivered
				} else {
					disposition = domain.DispositionAnswered
				}
			} else {
				disposition = domain.DispositionAnswered
			}
		case 17:
			disposition = domain.DispositionBusy
		case 19:
			disposition = domain.DispositionNoAnswer
		case 34:
			disposition = domain.DispositionCongestion
		case 1, 28:
			disposition = domain.DispositionInvalidNumber
		default:
			if causeInt > 0 {
				disposition = domain.DispositionFailed
			}
		}
	}

	now := time.Now()
	duration := int(now.Sub(activeChan.StartedAt).Seconds())
	if duration < 0 {
		duration = 0
	}

	var answeredAt *time.Time
	if activeChan.AnsweredAt != nil {
		answeredAt = activeChan.AnsweredAt
	} else if activeChan.IsAnswered || disposition == domain.DispositionAnswered || disposition == domain.DispositionDelivered {
		ans := activeChan.StartedAt
		answeredAt = &ans
	}

	billsec := 0
	ringSeconds := duration
	if answeredAt != nil {
		billsec = int(now.Sub(*answeredAt).Seconds())
		if billsec < 0 {
			billsec = 0
		}
		ringSeconds = int(answeredAt.Sub(activeChan.StartedAt).Seconds())
		if ringSeconds < 0 {
			ringSeconds = 0
		}
		duration = ringSeconds + billsec
	}

	recURL := ""
	recFile := activeChan.RecordingFile
	if recFile == "" && uniqueID != "" {
		nowStr := activeChan.StartedAt.Format("2006/01/02")
		pattern := fmt.Sprintf("/var/spool/asterisk/monitor/%s/*-%s.wav", nowStr, uniqueID)
		if matches, _ := filepath.Glob(pattern); len(matches) > 0 {
			recFile = matches[0]
		}
	}

	if recFile != "" {
		rel := strings.TrimPrefix(recFile, "/var/spool/asterisk/monitor/")
		rel = strings.TrimPrefix(rel, "/")
		baseURL := os.Getenv("RECORDINGS_BASE_URL")
		if baseURL == "" {
			baseURL = "http://localhost:8080/api/v1/recordings"
		}
		baseURL = strings.TrimSuffix(baseURL, "/")
		recURL = fmt.Sprintf("%s/%s", baseURL, rel)
	}

	var recFilePtr *string
	if recFile != "" {
		recFilePtr = &recFile
	}
	var recURLPtr *string
	if recURL != "" {
		recURLPtr = &recURL
	}

	var transPtr *string
	if activeChan.Transcription != "" {
		transPtr = &activeChan.Transcription
	}

	cdrID := activeChan.ChannelID
	if cdrID == "" {
		cdrID = fmt.Sprintf("cdr-%d", now.UnixNano())
	}

	if repRepo != nil {
		cdr := &domain.CDR{
			ID:              cdrID,
			TenantID:        activeChan.TenantID,
			CampaignID:      activeChan.CampaignID,
			Phone:           activeChan.Phone,
			LeadID:          activeChan.LeadID,
			LeadName:        &activeChan.Name,
			LeadCPF:         &activeChan.CPF,
			AgentID:         activeChan.AgentID,
			CallType:        activeChan.CallType,
			Disposition:     disposition,
			SIPStatus:       nil,
			HangupCause:     &causeInt,
			DurationSeconds: duration,
			BillsecSeconds:  billsec,
			RingSeconds:     ringSeconds,
			TrunkUsed:       activeChan.TrunkID,
			RecordingFile:   recFilePtr,
			RecordingURL:    recURLPtr,
			Transcription:   transPtr,
			AMDStatus:       activeChan.AMDStatus,
			AMDCause:        activeChan.AMDCause,
			CreatedAt:       activeChan.StartedAt,
			InitiatedAt:     &activeChan.StartedAt,
			AnsweredAt:      answeredAt,
			EndedAt:         &now,
		}
		_ = repRepo.SaveCDR(ctx, cdr)
	}

	if tm.cache != nil {
		hangupEvt := &domain.CDREvent{
			CallID:          cdrID,
			TenantID:        activeChan.TenantID,
			CampaignID:      activeChan.CampaignID,
			TrunkUsed:       activeChan.TrunkID,
			Phone:           activeChan.Phone,
			LeadID:          activeChan.LeadID,
			LeadName:        &activeChan.Name,
			LeadCPF:         &activeChan.CPF,
			AgentID:         activeChan.AgentID,
			CallType:        activeChan.CallType,
			Disposition:     disposition,
			Type:            domain.CDREventHangup,
			Timestamp:       now.Unix(),
			DurationSeconds: duration,
			BillsecSeconds:  billsec,
			RingSeconds:     ringSeconds,
			HangupCause:     &causeInt,
			RecordingFile:   recFilePtr,
			RecordingURL:    recURLPtr,
			Transcription:   transPtr,
			AMDStatus:       activeChan.AMDStatus,
			AMDCause:        activeChan.AMDCause,
			StartedAt:       activeChan.StartedAt,
			InitiatedAt:     &activeChan.StartedAt,
			AnsweredAt:      answeredAt,
			EndedAt:         &now,
		}
		_ = tm.cache.PublishCDREvent(ctx, hangupEvt)

		// Se a chamada teve áudio gravado e duração relevante (>= 5s), enfileira transcrição assíncrona
		if recFilePtr != nil && *recFilePtr != "" && billsec >= 5 {
			job := &domain.TranscriptionJob{
				CallID:        cdrID,
				TenantID:      activeChan.TenantID,
				RecordingFile: *recFilePtr,
				Duration:      billsec,
				Language:      "pt",
			}
			_ = tm.cache.EnqueueTranscriptionJob(ctx, job)
		}
	}

	if routRepo != nil && (activeChan.CallType == domain.CallTypePredictive || activeChan.CallType == domain.CallTypeManual) {
		_ = routRepo.SaveRouting(ctx, &domain.PhoneTrunkMapping{
			Phone:        activeChan.Phone,
			LastTrunk:    activeChan.TrunkID,
			LastProject:  activeChan.TenantID,
			LastSIPRoute: "",
			TenantID:     activeChan.TenantID,
			UpdatedAt:    time.Now(),
		})
	}

	tm.mu.RLock()
	notifier := tm.notifier
	tm.mu.RUnlock()

	if notifier != nil {
		if activeChan.CallType == domain.CallTypeManual || activeChan.IsAnswered || recURL != "" {
			notifier.DispatchCallEnded(activeChan, disposition, causeInt, duration, billsec, ringSeconds, recURL)
		}
	}
}

// handleNewstate rastreia transições de canal para UP (atendido).
func (tm *TrunkManager) handleNewstate(ctx context.Context, attrs map[string]string) {
	if attrs["ChannelState"] == "6" || attrs["ChannelStateDesc"] == "Up" {
		tm.channels.MarkAnswered(attrs["Channel"], attrs["Uniqueid"], time.Now())
		callID := tm.channels.GetCallIDByAsterisk(attrs["Channel"], attrs["Uniqueid"])
		if tm.cache != nil && callID != "" {
			evt := &domain.CDREvent{
				CallID:      callID,
				Disposition: domain.DispositionAnswered,
				Type:        domain.CDREventAnswered,
				Timestamp:   time.Now().Unix(),
			}
			_ = tm.cache.PublishCDREvent(ctx, evt)
		}
	}
}

// handleUserEvent despacha eventos customizados da central telefônica (PredictiveHuman, PredictiveMachine, InboundCall).
func (tm *TrunkManager) handleUserEvent(ctx context.Context, attrs map[string]string) {
	userEvent := attrs["UserEvent"]
	channel := attrs["Channel"]
	uniqueID := attrs["Uniqueid"]
	phone := attrs["Phone"]
	campaignID := attrs["CampaignId"]
	leadID := attrs["LeadId"]

	// Captura e sincroniza transcrição anexada ao UserEvent se disponível
	transcript := attrs["Transcript"]
	if transcript == "" {
		transcript = attrs["Transcription"]
	}
	if transcript != "" {
		tm.handleTranscriptionUpdate(ctx, channel, uniqueID, transcript)
	}

	tm.mu.RLock()
	pred := tm.predictive
	inb := tm.inbound
	tm.mu.RUnlock()

	switch userEvent {
	case "CallAnswered", "ManualAnswered":
		tm.channels.MarkAnswered(channel, uniqueID, time.Now())
	case "PredictiveHuman":
		tm.channels.MarkAnswered(channel, uniqueID, time.Now())
		callID := tm.channels.GetCallIDByAsterisk(channel, uniqueID)
		if callID != "" {
			tm.channels.SetAMDStatus(callID, "HUMAN", "HUMAN_GREETING")
			if tm.cache != nil {
				amdStatus := "HUMAN"
				amdCause := "HUMAN_GREETING"
				evt := &domain.CDREvent{
					CallID:      callID,
					AMDStatus:   &amdStatus,
					AMDCause:    &amdCause,
					Disposition: domain.DispositionAnswered,
					Type:        domain.CDREventAMDClassified,
					Timestamp:   time.Now().Unix(),
				}
				_ = tm.cache.PublishCDREvent(ctx, evt)
			}
		}
		if pred != nil {
			_ = pred.HandlePredictiveHuman(ctx, channel, uniqueID, phone, campaignID, leadID)
		}
	case "PredictiveAi":
		tm.channels.MarkAnswered(channel, uniqueID, time.Now())
		if pred != nil {
			_ = pred.HandlePredictiveAi(ctx, channel, uniqueID, phone, campaignID, leadID)
		}
	case "PredictiveMachine":
		callID := tm.channels.GetCallIDByAsterisk(channel, uniqueID)
		cause := attrs["Cause"]
		if cause == "" {
			cause = "VOICEMAIL"
		}
		if callID != "" {
			tm.channels.SetCallDisposition(callID, domain.DispositionVoicemail)
			tm.channels.SetAMDStatus(callID, "MACHINE", cause)
			if tm.cache != nil {
				amdStatus := "MACHINE"
				evt := &domain.CDREvent{
					CallID:      callID,
					AMDStatus:   &amdStatus,
					AMDCause:    &cause,
					Disposition: domain.DispositionVoicemail,
					Type:        domain.CDREventAMDClassified,
					Timestamp:   time.Now().Unix(),
				}
				_ = tm.cache.PublishCDREvent(ctx, evt)
			}
		}
	case "PredictiveRoomBusy":
		callID := tm.channels.GetCallIDByAsterisk(channel, uniqueID)
		if callID != "" {
			tm.channels.SetCallDisposition(callID, domain.DispositionAbandoned)
		}
	case "InboundCall":
		tm.channels.MarkAnswered(channel, uniqueID, time.Now())
		if inb != nil {
			callerPhone := attrs["CallerIDNum"]
			didNumber := attrs["Exten"]
			_ = inb.ProcessInboundCall(ctx, channel, callerPhone, didNumber)
		}
	}
}

// handleAgentConnect processa o evento AMI emitido quando um operador é conectado à chamada via Asterisk Queue.
func (tm *TrunkManager) handleAgentConnect(ctx context.Context, attrs map[string]string) {
	channel := attrs["Channel"]
	uniqueID := attrs["Uniqueid"]
	queue := attrs["Queue"]
	member := attrs["MemberName"]
	if member == "" {
		member = attrs["Interface"]
	}
	if member == "" {
		member = attrs["Member"]
	}

	callID := tm.channels.GetCallIDByAsterisk(channel, uniqueID)
	activeChan := tm.channels.GetActiveChannelByAsterisk(channel, uniqueID)

	agentID := cleanAgentID(member)

	if callID != "" && agentID != "" {
		tm.channels.AssignAgent(callID, agentID)
		if tm.cache != nil {
			evt := &domain.CDREvent{
				CallID:      callID,
				AgentID:     &agentID,
				Disposition: domain.DispositionDelivered,
				Type:        domain.CDREventAgentConnected,
				Timestamp:   time.Now().Unix(),
			}
			_ = tm.cache.PublishCDREvent(ctx, evt)
		}
	}

	campaignID := ""
	tenantID := ""
	phone := ""
	var cpf, name string
	var custom map[string]string

	if activeChan != nil {
		if activeChan.CampaignID != nil {
			campaignID = *activeChan.CampaignID
		}
		tenantID = activeChan.TenantID
		phone = activeChan.Phone
		cpf = activeChan.CPF
		name = activeChan.Name
		custom = activeChan.Custom
	}

	if campaignID == "" && strings.HasPrefix(queue, "q_") {
		campaignID = strings.TrimPrefix(queue, "q_")
	}

	// 1. Zera o contador de erros consecutivos da campanha no Redis
	if tm.cache != nil && campaignID != "" {
		_ = tm.cache.ResetConsecutiveErrors(ctx, campaignID)
	}

	// 2. Enfileira o evento de atendimento no Redis (dialer:answered_leads)
	if tm.cache != nil {
		now := time.Now()
		event := &domain.AnsweredLeadEvent{
			Event:      "call.answered",
			CallID:     callID,
			Channel:    channel,
			TenantID:   tenantID,
			CampaignID: campaignID,
			Phone:      phone,
			CPF:        cpf,
			Name:       name,
			AgentID:    agentID,
			RoomName:   member,
			Custom:     custom,
			AnsweredAt: now,
			Timestamp:  now.Unix(),
		}
		_ = tm.cache.PushAnsweredLead(ctx, event)
	}
}

