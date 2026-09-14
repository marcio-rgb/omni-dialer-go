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
			{ "cpf": "11122233344", "phone": "11988887777", "name": "MARCIO NASCIMENTO", "first_name": "; Márcio? ;" },
			{ "cpf": "55566677788", "phone": "11977776666", "name": "João da Silva" },
			{ "cpf": "99900011122", "phone": "11966665555", "name": "MARCIO SOUZA", "first_name": "; Márcio? ;" }
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
	// "; Márcio? ;" sintetizado 1 vez, "João da Silva" 1 vez, 2º "Márcio" reutiliza cache
	if resp.Data.NewAudiosSynthesized != 2 {
		t.Errorf("esperava NewAudiosSynthesized == 2, obteve %d", resp.Data.NewAudiosSynthesized)
	}
	if resp.Data.CachedAudiosCount != 1 {
		t.Errorf("esperava CachedAudiosCount == 1, obteve %d", resp.Data.CachedAudiosCount)
	}

	// Verifica se os textos originais com acentos/pontuação foram enviados para síntese TTS
	if len(tts.synthesized) != 2 {
		t.Fatalf("esperava 2 sinteses, obteve %d (%v)", len(tts.synthesized), tts.synthesized)
	}
	hasMarcio := false
	hasJoao := false
	for _, s := range tts.synthesized {
		if s == "; Márcio? ;" {
			hasMarcio = true
		}
		if s == "João" {
			hasJoao = true
		}
	}
	if !hasMarcio || !hasJoao {
		t.Errorf("textos para síntese não preservaram acentuação/pontuação: %v", tts.synthesized)
	}

	// Verifica se os leads foram persistidos no repositório com Name original e FirstName normalizado
	if len(repo.inserted) != 3 {
		t.Fatalf("esperava 3 leads persistidos no banco, obteve %d", len(repo.inserted))
	}
	if repo.inserted[0].Name != "MARCIO NASCIMENTO" || repo.inserted[0].FirstName != "marcio" {
		t.Errorf("lead 0 incorreto: Name=%s, FirstName=%s", repo.inserted[0].Name, repo.inserted[0].FirstName)
	}
	if repo.inserted[1].Name != "João da Silva" || repo.inserted[1].FirstName != "joao" {
		t.Errorf("lead 1 incorreto: Name=%s, FirstName=%s", repo.inserted[1].Name, repo.inserted[1].FirstName)
	}
	if repo.inserted[2].Name != "MARCIO SOUZA" || repo.inserted[2].FirstName != "marcio" {
		t.Errorf("lead 2 incorreto: Name=%s, FirstName=%s", repo.inserted[2].Name, repo.inserted[2].FirstName)
	}

	if len(cache.pushed["camp101"]) != 3 {
		t.Errorf("esperava 3 leads no redis, obteve %d", len(cache.pushed["camp101"]))
	}
}
