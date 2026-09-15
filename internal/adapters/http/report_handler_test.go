package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"dialer-go/internal/domain"
	"dialer-go/internal/ports"
	"github.com/go-chi/chi/v5"
)

type mockReportRepoForTest struct {
	ports.ReportRepository
	cdrs []*domain.CDR
}

func (m *mockReportRepoForTest) ListCDRs(ctx context.Context, filter domain.CDRFilter) (*domain.CDRListResponse, error) {
	var filtered []*domain.CDR
	for _, c := range m.cdrs {
		if c.TenantID == filter.TenantID {
			filtered = append(filtered, c)
		}
	}
	return &domain.CDRListResponse{
		Total:      int64(len(filtered)),
		Page:       filter.Page,
		Limit:      filter.Limit,
		TotalPages: 1,
		CDRs:       filtered,
	}, nil
}

func (m *mockReportRepoForTest) GetCDRByID(ctx context.Context, tenantID, cdrID string) (*domain.CDR, error) {
	for _, c := range m.cdrs {
		if c.TenantID == tenantID && c.ID == cdrID {
			return c, nil
		}
	}
	return nil, fmt.Errorf("not found")
}

func (m *mockReportRepoForTest) UpdateCDRTranscription(ctx context.Context, cdrID string, transcription string) error {
	for _, c := range m.cdrs {
		if c.ID == cdrID {
			c.Transcription = &transcription
			return nil
		}
	}
	return nil
}

func TestReportHandler_ListCDRs(t *testing.T) {
	recFile := "/var/spool/asterisk/monitor/2026/09/14/063000-PRED-11999999999-1.wav"
	recURL := "https://api-omnichat.creditobr.org/dialer-go/api/v1/recordings/2026/09/14/063000-PRED-11999999999-1.wav"

	transcription := "alo quem fala e da central"
	mockRepo := &mockReportRepoForTest{
		cdrs: []*domain.CDR{
			{
				ID:              "cdr-123",
				TenantID:        "tenant-a",
				Phone:           "11999999999",
				CallType:        domain.CallTypePredictive,
				Disposition:     domain.DispositionAnswered,
				DurationSeconds: 45,
				BillsecSeconds:  40,
				RingSeconds:     5,
				TrunkUsed:       "trunk-1",
				RecordingFile:   &recFile,
				RecordingURL:    &recURL,
				Transcription:   &transcription,
				CreatedAt:       time.Now(),
			},
		},
	}

	handler := NewReportHandler(mockRepo, nil)

	// 1. Falha: Sem tenant_id
	t.Run("MissingTenantID", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/cdrs", nil)
		w := httptest.NewRecorder()
		handler.ListCDRs(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("esperado status 400, obteve %d", w.Code)
		}
	})

	// 2. Sucesso: Com tenant_id e retorno de gravação
	t.Run("SuccessWithRecordings", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/cdrs?tenant_id=tenant-a", nil)
		w := httptest.NewRecorder()
		handler.ListCDRs(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("esperado status 200, obteve %d", w.Code)
		}

		var resp struct {
			Success bool                    `json:"success"`
			Data    *domain.CDRListResponse `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("falha ao deserializar resposta: %v", err)
		}

		if resp.Data == nil || resp.Data.Total != 1 {
			t.Fatalf("esperado 1 CDR, obteve %v", resp.Data)
		}

		cdr := resp.Data.CDRs[0]
		if cdr.RecordingFile == nil || *cdr.RecordingFile != recFile {
			t.Fatalf("recording_file incorreto: %v", cdr.RecordingFile)
		}
		if cdr.RecordingURL == nil || *cdr.RecordingURL != recURL {
			t.Fatalf("recording_url incorreto: %v", cdr.RecordingURL)
		}
		if cdr.Transcription == nil || *cdr.Transcription != transcription {
			t.Fatalf("transcription incorreto: %v", cdr.Transcription)
		}
	})
}

func TestReportHandler_GetCDR(t *testing.T) {
	recURL := "https://api-omnichat.creditobr.org/dialer-go/api/v1/recordings/test.wav"
	transText := "ola tudo bem"
	mockRepo := &mockReportRepoForTest{
		cdrs: []*domain.CDR{
			{
				ID:            "cdr-abc",
				TenantID:      "tenant-b",
				Phone:         "11988888888",
				CallType:      domain.CallTypeManual,
				Disposition:   domain.DispositionAnswered,
				RecordingURL:  &recURL,
				Transcription: &transText,
				CreatedAt:     time.Now(),
			},
		},
	}

	handler := NewReportHandler(mockRepo, nil)

	r := chi.NewRouter()
	r.Get("/api/v1/cdrs/{id}", handler.GetCDR)

	// 1. Falha: Não encontrado
	t.Run("NotFound", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/cdrs/cdr-inexistente?tenant_id=tenant-b", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("esperado status 404, obteve %d", w.Code)
		}
	})

	// 2. Sucesso: Encontrado
	t.Run("Success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/cdrs/cdr-abc?tenant_id=tenant-b", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("esperado status 200, obteve %d", w.Code)
		}

		var resp struct {
			Success bool        `json:"success"`
			Data    *domain.CDR `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("falha ao deserializar: %v", err)
		}

		if resp.Data.ID != "cdr-abc" {
			t.Fatalf("ID incorreto: %s", resp.Data.ID)
		}
		if resp.Data.RecordingURL == nil || *resp.Data.RecordingURL != recURL {
			t.Fatalf("recording_url incorreto: %v", resp.Data.RecordingURL)
		}
		if resp.Data.Transcription == nil || *resp.Data.Transcription != transText {
			t.Fatalf("transcription incorreto: %v", resp.Data.Transcription)
		}
	})
}

func TestAudioWordsHandler_ServeRecording(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("ASTERISK_MONITOR_DIR", tmpDir)
	defer os.Unsetenv("ASTERISK_MONITOR_DIR")

	yearDir := filepath.Join(tmpDir, "2026", "09", "14")
	if err := os.MkdirAll(yearDir, 0755); err != nil {
		t.Fatalf("falha ao criar diretorio de teste: %v", err)
	}
	wavPath := filepath.Join(yearDir, "test-call.wav")
	fakeAudio := []byte("RIFF1234WAVEfmt test audio data stream")
	if err := os.WriteFile(wavPath, fakeAudio, 0644); err != nil {
		t.Fatalf("falha ao escrever arquivo de teste: %v", err)
	}

	handler := NewAudioWordsHandler(nil)
	r := chi.NewRouter()
	r.HandleFunc("/api/v1/recordings/*", handler.ServeRecording)

	// 1. Sucesso: OPTIONS preflight com headers de CORS
	t.Run("CORS_OptionsPreflight", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "/api/v1/recordings/2026/09/14/test-call.wav", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("esperado status 200 para OPTIONS, obteve %d", w.Code)
		}
		if origin := w.Header().Get("Access-Control-Allow-Origin"); origin != "*" {
			t.Fatalf("esperado Access-Control-Allow-Origin '*', obteve %s", origin)
		}
	})

	// 2. Sucesso: GET streaming de áudio com headers corretos
	t.Run("GET_AudioStream", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/recordings/2026/09/14/test-call.wav", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("esperado status 200, obteve %d", w.Code)
		}
		if contentType := w.Header().Get("Content-Type"); contentType != "audio/wav" {
			t.Fatalf("esperado Content-Type 'audio/wav', obteve '%s'", contentType)
		}
		if origin := w.Header().Get("Access-Control-Allow-Origin"); origin != "*" {
			t.Fatalf("esperado Access-Control-Allow-Origin '*', obteve %s", origin)
		}
		if w.Body.String() != string(fakeAudio) {
			t.Fatalf("conteudo do audio difere do esperado")
		}
	})

	// 3. Falha: Arquivo não encontrado (404 Problem Details)
	t.Run("NotFound", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/recordings/2026/09/14/not-found.wav", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("esperado status 404, obteve %d", w.Code)
		}
		if origin := w.Header().Get("Access-Control-Allow-Origin"); origin != "*" {
			t.Fatalf("esperado Access-Control-Allow-Origin '*' no 404, obteve %s", origin)
		}
	})
}

