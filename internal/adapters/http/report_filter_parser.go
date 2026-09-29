package http

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"dialer-go/internal/domain"
)

// ParseCDRFilter extrai, sanitiza e valida os parâmetros de consulta para listagem de CDRs.
//
// @pattern Adapter (Request Parameter Parser & Validator)
// @governedBy docs/rules/TELEPHONY_POLICIES.md#cdr-queries
//
// @preExecution
// - Recebe *http.Request autenticado via IP Whitelist
//
// @postExecution
// - Retorna struct domain.CDRFilter preenchida com defaults seguros e sanitizada
// - Retorna *domain.ProblemDetails caso ocorra erro de validação (RFC 7807)
func ParseCDRFilter(r *http.Request) (*domain.CDRFilter, *domain.ProblemDetails) {
	// 1. Tenant ID obrigatório
	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		tenantID = r.Header.Get("X-Tenant-Id")
	}
	if tenantID == "" {
		return nil, domain.NewErrBadRequest(
			"MISSING_TENANT_ID",
			"O parâmetro obrigatório 'tenant_id' não foi informado via query parameter (?tenant_id=...) nem via header HTTP (X-Tenant-Id).",
		)
	}

	filter := &domain.CDRFilter{
		TenantID:     tenantID,
		Page:         1,
		Limit:        20,
		SortBy:       "created_at",
		SortOrder:    "desc",
		IncludeTotal: true,
	}

	// 2. Paginação
	if pStr := r.URL.Query().Get("page"); pStr != "" {
		if p, err := strconv.Atoi(pStr); err == nil && p >= 1 {
			filter.Page = p
		}
	}
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l >= 1 {
			if l > 100 {
				l = 100
			}
			filter.Limit = l
		}
	}

	// 3. Filtros básicos
	if campID := r.URL.Query().Get("campaign_id"); campID != "" {
		filter.CampaignID = &campID
	}
	if phone := r.URL.Query().Get("phone"); phone != "" {
		filter.Phone = &phone
	}

	// 4. Filtros de Contact Center
	if agentID := r.URL.Query().Get("agent_id"); agentID != "" {
		filter.AgentID = &agentID
	}
	if callType := r.URL.Query().Get("call_type"); callType != "" {
		ct := domain.CallType(callType)
		filter.CallType = &ct
	}
	if trunk := r.URL.Query().Get("trunk_used"); trunk != "" {
		filter.TrunkUsed = &trunk
	}
	if cpf := r.URL.Query().Get("lead_cpf"); cpf != "" {
		filter.LeadCPF = &cpf
	}
	if lidStr := r.URL.Query().Get("lead_id"); lidStr != "" {
		if lid, err := strconv.ParseInt(lidStr, 10, 64); err == nil && lid > 0 {
			filter.LeadID = &lid
		}
	}
	if amd := r.URL.Query().Get("amd_status"); amd != "" {
		filter.AMDStatus = &amd
	}
	if hasRec := r.URL.Query().Get("has_recording"); hasRec != "" {
		if b, err := strconv.ParseBool(hasRec); err == nil {
			filter.HasRecording = &b
		}
	}

	// 5. Disposições (suporta ?disposition=A,B e ?disposition=A&disposition=B)
	dispValues := r.URL.Query()["disposition"]
	if len(dispValues) == 0 {
		dispValues = r.URL.Query()["dispositions"]
	}
	var extractedDisps []domain.CallDisposition
	for _, rawVal := range dispValues {
		parts := strings.Split(rawVal, ",")
		for _, part := range parts {
			cleaned := strings.TrimSpace(part)
			if cleaned != "" {
				extractedDisps = append(extractedDisps, domain.CallDisposition(cleaned))
			}
		}
	}
	if len(extractedDisps) == 1 {
		d := extractedDisps[0]
		filter.Disposition = &d
		filter.Dispositions = extractedDisps
	} else if len(extractedDisps) > 1 {
		filter.Dispositions = extractedDisps
	}

	// 6. Faixas de duração e faturamento
	if v := r.URL.Query().Get("min_billsec"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			filter.MinBillsec = &n
		}
	}
	if v := r.URL.Query().Get("max_billsec"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			filter.MaxBillsec = &n
		}
	}
	if v := r.URL.Query().Get("min_duration"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			filter.MinDuration = &n
		}
	}
	if v := r.URL.Query().Get("max_duration"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			filter.MaxDuration = &n
		}
	}

	if filter.MinBillsec != nil && filter.MaxBillsec != nil && *filter.MinBillsec > *filter.MaxBillsec {
		return nil, domain.NewErrUnprocessable("INVALID_BILLSEC_RANGE",
			"O valor 'min_billsec' não pode ser superior a 'max_billsec'.",
			domain.InvalidParam{Name: "min_billsec", Reason: "superior a max_billsec"})
	}
	if filter.MinDuration != nil && filter.MaxDuration != nil && *filter.MinDuration > *filter.MaxDuration {
		return nil, domain.NewErrUnprocessable("INVALID_DURATION_RANGE",
			"O valor 'min_duration' não pode ser superior a 'max_duration'.",
			domain.InvalidParam{Name: "min_duration", Reason: "superior a max_duration"})
	}

	// 7. Busca textual / inteligente
	if q := r.URL.Query().Get("q"); q != "" {
		filter.Search = &q
	} else if search := r.URL.Query().Get("search"); search != "" {
		filter.Search = &search
	}

	// 8. Ordenação segura (todos os 25 campos canônicos da tabela cdrs)
	if sortBy := r.URL.Query().Get("sort_by"); sortBy != "" {
		switch sortBy {
		case "id", "tenant_id", "campaign_id", "agent_id", "trunk_used",
			"phone", "lead_name", "lead_cpf", "lead_id",
			"call_type", "disposition", "sip_status", "hangup_cause", "amd_status", "amd_cause",
			"duration_seconds", "billsec_seconds", "ring_seconds",
			"created_at", "initiated_at", "answered_at", "ended_at",
			"recording_file", "recording_url", "transcription":
			filter.SortBy = sortBy
		}
	}
	if sortOrder := r.URL.Query().Get("sort_order"); sortOrder != "" {
		if strings.EqualFold(sortOrder, "asc") {
			filter.SortOrder = "asc"
		}
	}

	// 9. include_total (default: true)
	if itStr := r.URL.Query().Get("include_total"); itStr != "" {
		if b, err := strconv.ParseBool(itStr); err == nil {
			filter.IncludeTotal = b
		}
	}

	// 10. Janela temporal flexível (RFC3339, DateOnly, etc.) com default de 7 dias
	if startStr := r.URL.Query().Get("start_date"); startStr != "" {
		st, err := parseFlexibleDate(startStr, false)
		if err != nil {
			return nil, domain.NewErrUnprocessable("INVALID_DATE_FORMAT",
				"O formato de 'start_date' deve ser ISO 8601 (ex: 2026-09-09T00:00:00Z ou 2026-09-09)",
				domain.InvalidParam{Name: "start_date", Reason: "formato inválido"})
		}
		filter.StartDate = st
	} else {
		defaultStart := time.Now().UTC().AddDate(0, 0, -7)
		filter.StartDate = &defaultStart
	}

	if endStr := r.URL.Query().Get("end_date"); endStr != "" {
		et, err := parseFlexibleDate(endStr, true)
		if err != nil {
			return nil, domain.NewErrUnprocessable("INVALID_DATE_FORMAT",
				"O formato de 'end_date' deve ser ISO 8601 (ex: 2026-09-09T23:59:59Z ou 2026-09-09)",
				domain.InvalidParam{Name: "end_date", Reason: "formato inválido"})
		}
		filter.EndDate = et
	} else {
		defaultEnd := time.Now().UTC()
		filter.EndDate = &defaultEnd
	}

	if filter.StartDate != nil && filter.EndDate != nil && filter.StartDate.After(*filter.EndDate) {
		return nil, domain.NewErrUnprocessable("INVALID_DATE_RANGE",
			"A data inicial 'start_date' não pode ser posterior à data final 'end_date'.",
			domain.InvalidParam{Name: "start_date", Reason: "posterior a end_date"})
	}

	// Teto de janela temporal: 31d sem filtro seletivo, 90d com filtro seletivo
	if filter.StartDate != nil && filter.EndDate != nil {
		days := filter.EndDate.Sub(*filter.StartDate).Hours() / 24
		hasSelectiveFilter := filter.Phone != nil || filter.LeadCPF != nil || filter.LeadID != nil || filter.AgentID != nil
		maxDays := 31.0
		if hasSelectiveFilter {
			maxDays = 90.0
		}
		if days > maxDays {
			return nil, domain.NewErrUnprocessable("DATE_RANGE_TOO_BROAD",
				fmt.Sprintf("A janela temporal de %.0f dias excede o limite de %.0f dias. Refine a busca com filtros seletivos (phone, lead_cpf, agent_id) ou reduza o período.", days, maxDays),
				domain.InvalidParam{Name: "start_date", Reason: "janela temporal excessiva"})
		}
	}

	return filter, nil
}

// parseFlexibleDate converte strings de datas com suporte a múltiplos formatos aceitos em contact center.
func parseFlexibleDate(raw string, isEndOfDay bool) (*time.Time, error) {
	trimmed := strings.TrimSpace(raw)
	formats := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		time.DateOnly,
	}

	for _, fmtStr := range formats {
		if t, err := time.Parse(fmtStr, trimmed); err == nil {
			tUTC := t.UTC()
			if fmtStr == time.DateOnly && isEndOfDay {
				tUTC = time.Date(tUTC.Year(), tUTC.Month(), tUTC.Day(), 23, 59, 59, 999999999, time.UTC)
			}
			return &tUTC, nil
		}
	}

	return nil, fmt.Errorf("formato de data desconhecido: %s", raw)
}
