package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"dialer-go/internal/core"
	"dialer-go/internal/domain"
	"dialer-go/internal/ports"
)

type mockLeadRepo struct {
	ports.LeadRepository
	inserted []*domain.Lead
}

func (m *mockLeadRepo) BatchInsert(ctx context.Context, leads []*domain.Lead) (int64, error) {
	m.inserted = append(m.inserted, leads...)
	return int64(len(leads)), nil
}

type mockCacheForLeadBatch struct {
	ports.CachePort
	pushed map[string][]string
}

func (m *mockCacheForLeadBatch) PushLeads(ctx context.Context, campaignID string, leads []string) error {
	if m.pushed == nil {
		m.pushed = make(map[string][]string)
	}
	m.pushed[campaignID] = append(m.pushed[campaignID], leads...)
	return nil
}

type mockTTSForBatch struct {
	synthesized []string
}

func (m *mockTTSForBatch) Synthesize(ctx context.Context, text string, outputPath string) error {
	m.synthesized = append(m.synthesized, text)
	ac := core.NewAudioConcatenator()
	pcm := make([]byte, 1000)
	wav, err := ac.BuildWAV(pcm, 22050, 1, 16)
	if err != nil {
		return err
	}
	return os.WriteFile(outputPath, wav, 0644)
}

func (m *mockTTSForBatch) GetSampleRate() int {
	return 22050
}

func TestLeadBatchHandler_IngestBatch(t *testing.T) {
	tmpDir := t.TempDir()
	tts := &mockTTSForBatch{}
	concatenator := core.NewAudioConcatenator()
	audioWordMgr, err := core.NewAudioWordManager(tmpDir, tts, concatenator)
	if err != nil {
		t.Fatalf("falha ao criar AudioWordManager: %v", err)
	}

	repo := &mockLeadRepo{}
	cache := &mockCacheForLeadBatch{}
	handler := NewLeadBatchHandler(repo, cache, audioWordMgr)

	payload := []byte(`{
		"campaign_id": "camp101",
		"tenant_id": "tenant1",
		"leads": [
			{ "cpf": "11122233344", "phone": "11988887777", "name": "Márcio" },
			{ "cpf": "55566677788", "phone": "11977776666", "name": "João da Silva" },
			{ "cpf": "99900011122", "phone": "11966665555", "name": "Márcio" }
		]
	}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns/camp101/leads", bytes.NewReader(payload))
	w := httptest.NewRecorder()

	handler.IngestBatch(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("esperava 200 OK, obteve %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Success bool                     `json:"success"`
		Data    domain.BatchLeadResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("erro ao decodificar resposta: %v", err)
	}

	if resp.Data.TotalReceived != 3 {
		t.Errorf("esperava TotalReceived == 3, obteve %d", resp.Data.TotalReceived)
	}
	if resp.Data.LeadsQueued != 3 {
		t.Errorf("esperava LeadsQueued == 3, obteve %d", resp.Data.LeadsQueued)
	}
	// "Márcio" sintetizado 1 vez, "João da Silva" 1 vez, 2º "Márcio" reutiliza cache
	if resp.Data.NewAudiosSynthesized != 2 {
		t.Errorf("esperava NewAudiosSynthesized == 2, obteve %d", resp.Data.NewAudiosSynthesized)
	}
	if resp.Data.CachedAudiosCount != 1 {
		t.Errorf("esperava CachedAudiosCount == 1, obteve %d", resp.Data.CachedAudiosCount)
	}

	// Verifica se os textos originais com acentos foram preservados na chamada TTS
	if len(tts.synthesized) != 2 {
		t.Fatalf("esperava 2 sinteses, obteve %d (%v)", len(tts.synthesized), tts.synthesized)
	}
	if tts.synthesized[0] != "Márcio" || tts.synthesized[1] != "João da Silva" {
		t.Errorf("textos com acento nao preservados: %v", tts.synthesized)
	}

	// Verifica se os leads foram persistidos no repositório
	if len(repo.inserted) != 3 {
		t.Errorf("esperava 3 leads persistidos no banco, obteve %d", len(repo.inserted))
	}
	if len(cache.pushed["camp101"]) != 3 {
		t.Errorf("esperava 3 leads no redis, obteve %d", len(cache.pushed["camp101"]))
	}
}
