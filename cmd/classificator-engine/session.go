package main

import (
	"context"
	"io"
	"strings"
	"time"

	"dialer-go/internal/domain"
	"dialer-go/internal/ports"
)

// SessionHandler orquestra o ciclo de vida de classificação de uma chamada telefônica.
//
// @pattern Facade / Pipeline
// @governedBy .agents/ARCHITECT.md
type SessionHandler struct {
	recognizer ports.SpeechRecognizerPort
	classifier ports.SemanticClassifierPort
	maxDurSec  float64
}

// NewSessionHandler inicializa a sessão com seus respectivos componentes desacoplados.
func NewSessionHandler(rec ports.SpeechRecognizerPort, cls ports.SemanticClassifierPort, maxDurSec float64) ports.ClassificationEnginePort {
	if maxDurSec <= 0 {
		maxDurSec = 3.5
	}
	return &SessionHandler{
		recognizer: rec,
		classifier: cls,
		maxDurSec:  maxDurSec,
	}
}

// ProcessSession processa o stream de áudio recebido até que um veredito seja confirmado ou a janela expire.
//
// @pattern Fast-Exit Streaming Pipeline
// @preExecution audioReader provê fluxo contínuo PCM linear 8000Hz.
// @postExecution Emissão de eventos no sink e retorno do ClassificationVerdict.
func (s *SessionHandler) ProcessSession(ctx context.Context, audioReader io.Reader, sink ports.EventSink) (domain.ClassificationVerdict, error) {
	s.recognizer.Reset()
	startTime := time.Now()

	var firstSpeechTime time.Time
	var lastSpeechTime time.Time
	lastEmittedText := ""

	chunkBuf := make([]byte, 1600) // 100ms de áudio PCM 16-bit 8000Hz mono

	for {
		select {
		case <-ctx.Done():
			verdict := s.classifier.Finalize(s.recognizer.GetFinal(), 0, 0)
			verdict.LatencyMs = time.Since(startTime).Milliseconds()
			return verdict, ctx.Err()
		default:
		}

		elapsed := time.Since(startTime).Seconds()

		// Grace period dinâmico: se o cliente começou a falar nos últimos 800ms, permite até 1.0s extra
		if elapsed >= s.maxDurSec {
			isRecentSpeech := !firstSpeechTime.IsZero() && time.Since(lastSpeechTime) < 800*time.Millisecond
			if !isRecentSpeech || elapsed >= (s.maxDurSec+1.0) {
				break
			}
		}

		n, err := audioReader.Read(chunkBuf)
		if n > 0 {
			frame := domain.AudioFrame{
				SampleRate: 8000,
				Channels:   1,
				PCMData:    chunkBuf[:n],
			}
			_ = s.recognizer.AcceptAudio(frame)
		}
		if err != nil {
			break
		}

		// Checa hipótese textual em tempo real
		partial := s.recognizer.GetPartial()
		finalText := s.recognizer.GetFinal()

		currentText := finalText
		if currentText == "" {
			currentText = partial
		}
		currentText = strings.TrimSpace(currentText)

		if currentText != "" && currentText != lastEmittedText {
			if firstSpeechTime.IsZero() {
				firstSpeechTime = time.Now()
			}
			lastSpeechTime = time.Now()
			lastEmittedText = currentText

			if sink != nil {
				if finalText != "" {
					_ = sink.EmitEvent(domain.WireEventMessage{
						Type: domain.WireEventTranscriptionFinal,
						Text: currentText,
					})
				} else {
					_ = sink.EmitEvent(domain.WireEventMessage{
						Type: domain.WireEventTranscriptionPart,
						Text: currentText,
					})
				}
			}

			speechSec := 0.0
			silenceSec := 0.0
			if !firstSpeechTime.IsZero() {
				speechSec = lastSpeechTime.Sub(firstSpeechTime).Seconds()
				silenceSec = time.Since(lastSpeechTime).Seconds()
			}

			// Avalia se atingiu condição de Fast-Exit (Ex.: Saudação humana confirmada ou Caixa postal óbvia)
			if verdict, isFinal := s.classifier.Evaluate(partial, finalText, speechSec, silenceSec); isFinal {
				verdict.LatencyMs = time.Since(startTime).Milliseconds()
				if sink != nil {
					_ = sink.EmitEvent(domain.WireEventMessage{
						Type:          domain.WireEventVerdict,
						Status:        verdict.Status,
						Cause:         verdict.Cause,
						Text:          verdict.Transcription,
						LatencyMs:     verdict.LatencyMs,
						Confidence:    verdict.Confidence,
						VerdictDetail: &verdict,
					})
				}
				return verdict, nil
			}
		}

		// Pequena pausa para sincronismo de chunk de 100ms se leitura rápida
		time.Sleep(10 * time.Millisecond)
	}

	// Janela esgotada sem Fast-Exit: invoca Finalize para tomada de decisão final
	speechSec := 0.0
	silenceSec := 0.0
	if !firstSpeechTime.IsZero() {
		speechSec = lastSpeechTime.Sub(firstSpeechTime).Seconds()
		silenceSec = time.Since(lastSpeechTime).Seconds()
	}

	finalVerdict := s.classifier.Finalize(s.recognizer.GetFinal(), speechSec, silenceSec)
	finalVerdict.LatencyMs = time.Since(startTime).Milliseconds()

	if sink != nil {
		_ = sink.EmitEvent(domain.WireEventMessage{
			Type:          domain.WireEventVerdict,
			Status:        finalVerdict.Status,
			Cause:         finalVerdict.Cause,
			Text:          finalVerdict.Transcription,
			LatencyMs:     finalVerdict.LatencyMs,
			Confidence:    finalVerdict.Confidence,
			VerdictDetail: &finalVerdict,
		})
	}

	return finalVerdict, nil
}
