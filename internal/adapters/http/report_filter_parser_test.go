package http

import (
	"net/http/httptest"
	"testing"

	"dialer-go/internal/domain"
)

func TestParseCDRFilter_Success(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/cdrs?tenant_id=t1&start_date=2026-09-01&end_date=2026-09-10&disposition=ANSWERED,DELIVERED&sort_by=billsec_seconds&sort_order=asc&min_billsec=10&max_billsec=60", nil)

	filter, probErr := ParseCDRFilter(req)
	if probErr != nil {
		t.Fatalf("esperava sucesso, obteve erro: %v", probErr)
	}

	if filter.TenantID != "t1" {
		t.Errorf("esperado tenant_id 't1', obteve '%s'", filter.TenantID)
	}
	if len(filter.Dispositions) != 2 {
		t.Fatalf("esperado 2 disposições, obteve %d", len(filter.Dispositions))
	}
	if filter.SortBy != "billsec_seconds" || filter.SortOrder != "asc" {
		t.Errorf("ordenação incorreta: %s %s", filter.SortBy, filter.SortOrder)
	}
	if filter.MinBillsec == nil || *filter.MinBillsec != 10 {
		t.Errorf("min_billsec incorreto")
	}
	if filter.MaxBillsec == nil || *filter.MaxBillsec != 60 {
		t.Errorf("max_billsec incorreto")
	}
	if filter.StartDate == nil || filter.StartDate.Year() != 2026 || filter.StartDate.Month() != 9 || filter.StartDate.Day() != 1 {
		t.Errorf("start_date DateOnly incorreto: %v", filter.StartDate)
	}
	if filter.EndDate == nil || filter.EndDate.Hour() != 23 {
		t.Errorf("end_date DateOnly deve ir até 23:59:59: %v", filter.EndDate)
	}
}

func TestParseCDRFilter_MultipleQueryParamDispositions(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/cdrs?tenant_id=t1&disposition=ANSWERED&disposition=BUSY&disposition=FAILED", nil)

	filter, probErr := ParseCDRFilter(req)
	if probErr != nil {
		t.Fatalf("esperava sucesso, obteve erro: %v", probErr)
	}

	if len(filter.Dispositions) != 3 {
		t.Fatalf("esperado 3 disposições, obteve %d", len(filter.Dispositions))
	}
	if filter.Dispositions[0] != domain.CallDisposition("ANSWERED") ||
		filter.Dispositions[1] != domain.CallDisposition("BUSY") ||
		filter.Dispositions[2] != domain.CallDisposition("FAILED") {
		t.Errorf("disposições incorretas: %v", filter.Dispositions)
	}
}

func TestParseCDRFilter_ValidationErrors(t *testing.T) {
	t.Run("MissingTenantID", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/cdrs", nil)
		_, prob := ParseCDRFilter(req)
		if prob == nil || prob.Code != "MISSING_TENANT_ID" {
			t.Errorf("esperava erro MISSING_TENANT_ID, obteve: %v", prob)
		}
	})

	t.Run("InvalidBillsecRange", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/cdrs?tenant_id=t1&min_billsec=100&max_billsec=20", nil)
		_, prob := ParseCDRFilter(req)
		if prob == nil || prob.Code != "INVALID_BILLSEC_RANGE" {
			t.Errorf("esperava erro INVALID_BILLSEC_RANGE, obteve: %v", prob)
		}
	})

	t.Run("DateRangeTooBroadWithoutFilter", func(t *testing.T) {
		// 35 dias sem filtro seletivo excede teto de 31 dias
		req := httptest.NewRequest("GET", "/api/v1/cdrs?tenant_id=t1&start_date=2026-08-01&end_date=2026-09-10", nil)
		_, prob := ParseCDRFilter(req)
		if prob == nil || prob.Code != "DATE_RANGE_TOO_BROAD" {
			t.Errorf("esperava erro DATE_RANGE_TOO_BROAD, obteve: %v", prob)
		}
	})

	t.Run("DateRangeAllowedWithSelectiveFilter", func(t *testing.T) {
		// 60 dias com filtro seletivo (phone) é permitido (teto de 90 dias)
		req := httptest.NewRequest("GET", "/api/v1/cdrs?tenant_id=t1&start_date=2026-07-01&end_date=2026-08-30&phone=11999999999", nil)
		filter, prob := ParseCDRFilter(req)
		if prob != nil {
			t.Errorf("esperava sucesso para janela de 60 dias com phone, obteve: %v", prob)
		}
		if filter == nil || filter.Phone == nil || *filter.Phone != "11999999999" {
			t.Errorf("filtro phone não atribuído corretamente")
		}
	})
}

func TestParseCDRFilter_AllCanonicalSortFields(t *testing.T) {
	fields := []string{
		"id", "tenant_id", "campaign_id", "agent_id", "trunk_used",
		"phone", "lead_name", "lead_cpf", "lead_id",
		"call_type", "disposition", "sip_status", "hangup_cause", "amd_status", "amd_cause",
		"duration_seconds", "billsec_seconds", "ring_seconds",
		"created_at", "initiated_at", "answered_at", "ended_at",
		"recording_file", "recording_url", "transcription",
	}

	for _, field := range fields {
		t.Run(field, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/cdrs?tenant_id=t1&sort_by="+field+"&sort_order=desc", nil)
			filter, prob := ParseCDRFilter(req)
			if prob != nil {
				t.Fatalf("esperava sucesso para sort_by %s, erro: %v", field, prob)
			}
			if filter.SortBy != field {
				t.Errorf("esperado SortBy %s, obteve %s", field, filter.SortBy)
			}
			if filter.SortOrder != "desc" {
				t.Errorf("esperado SortOrder desc, obteve %s", filter.SortOrder)
			}
		})
	}
}
