package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	httpAdapter "dialer-go/internal/adapters/http"
	"dialer-go/internal/adapters/tts"
	"dialer-go/internal/core"
	"dialer-go/internal/domain"
)

type mockTTS struct {
	sampleRate int
}

func (m *mockTTS) Synthesize(ctx context.Context, text string, outputPath string) error {
	ac := core.NewAudioConcatenator()
	pcm := make([]byte, (uint32(m.sampleRate)*100/1000)*2) // 100ms
	wav, err := ac.BuildWAV(pcm, uint32(m.sampleRate), 1, 16)
	if err != nil {
		return err
	}
	return os.WriteFile(outputPath, wav, 0644)
}

func (m *mockTTS) GetSampleRate() int {
	return m.sampleRate
}

func TestAudioWordsHandler_MockFlow(t *testing.T) {
	tmpDir := t.TempDir()
	ttsMock := &mockTTS{sampleRate: 22050}
	concatenator := core.NewAudioConcatenator()
	mgr, err := core.NewAudioWordManager(tmpDir, ttsMock, concatenator)
	if err != nil {
		t.Fatalf("falha ao instanciar manager: %v", err)
	}

	handler := httpAdapter.NewAudioWordsHandler(mgr)

	// 1. Testa UpsertWords com formato plano e acentuação
	payload := []byte(`{
		"saudacao": "Oláa! , sou do convênio: ",
		"work_words": "Governo de São Paulo",
		"falo_com": " Estou falando com ",
		"pausa_ms": 30,
		"first_name": "Márcio ?",
		"momentinho": "Só um momentinho, por favor..."
	}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/audio/words", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.UpsertWords(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("esperava HTTP 200, obteve %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Success bool                            `json:"success"`
		Data    domain.UpsertAudioWordsResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("falha ao parsear resposta: %v", err)
	}

	if !resp.Success || resp.Data.CreatedCount != 5 {
		t.Errorf("esperava 5 arquivos criados, obteve %d", resp.Data.CreatedCount)
	}

	// 2. Testa Preview via GET com query params flexíveis
	previewURL := "/api/v1/audio/preview?first_name=Márcio%20?&work_words=Governo%20de%20São%20Paulo&pausa_ms=30ms&momentinho=true"
	previewReq := httptest.NewRequest(http.MethodGet, previewURL, nil)
	previewW := httptest.NewRecorder()

	handler.Preview(previewW, previewReq)

	if previewW.Code != http.StatusOK {
		t.Fatalf("esperava HTTP 200 no preview, obteve %d: %s", previewW.Code, previewW.Body.String())
	}

	contentType := previewW.Header().Get("Content-Type")
	if contentType != "audio/wav" {
		t.Errorf("esperava Content-Type audio/wav, obteve %s", contentType)
	}
	if previewW.Body.Len() < 44 {
		t.Errorf("tamanho do WAV retornado invalido: %d", previewW.Body.Len())
	}
}

func TestAudioWordsHandler_LivePiperSynthesis(t *testing.T) {
	binPath := "../../../bin/piper/piper"
	modelPath := "../../../models/piper/dii_pt-BR.onnx"
	configPath := "../../../models/piper/dii_pt-BR.onnx.json"
	storagePath := "../../../storage/audio_cache"

	if _, err := os.Stat(binPath); err != nil {
		t.Skip("Piper binario nao encontrado, pulando live test")
	}

	piper, err := tts.NewPiperAdapter(binPath, modelPath, configPath)
	if err != nil {
		t.Skipf("Piper nao inicializavel: %v", err)
	}

	concatenator := core.NewAudioConcatenator()
	mgr, err := core.NewAudioWordManager(storagePath, piper, concatenator)
	if err != nil {
		t.Fatalf("falha ao criar manager: %v", err)
	}

	handler := httpAdapter.NewAudioWordsHandler(mgr)

	// Dispara o payload exato solicitado pelo usuário
	userPayload := []byte(`{
		"saudacao": "Oláa...! , sou do convênio: ",
		"work_words": " do Governo de São Paulo...",
		"falo_com": " Estou falando com. ;",
		"first_name": "Márcio? ;",
		"momentinho": "... momentinho, por favor...",
		"pausas": {
			"saudacao_convenio_ms": 0,
			"convenio_falo_com_ms": 0,
			"falo_com_nome_ms": 0,
			"nome_momentinho_ms": 0
		},
		"force_overwrite": true
	}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/audio/words", bytes.NewReader(userPayload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.UpsertWords(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("UpsertWords falhou: %d - %s", w.Code, w.Body.String())
	}

	// Gera o Preview concatenado via POST com pausas customizadas
	previewBody := []byte(`{
		"first_name": "Márcio? ;",
		"work_words": " do Governo de São Paulo...",
		"custom_pausas": {
			"saudacao_convenio_ms": 0,
			"convenio_falo_com_ms": 0,
			"falo_com_nome_ms": 0,
			"nome_momentinho_ms": 0
		},
		"include_momentinho": true
	}`)

	previewReq := httptest.NewRequest(http.MethodPost, "/api/v1/audio/preview", bytes.NewReader(previewBody))
	previewReq.Header.Set("Content-Type", "application/json")
	previewW := httptest.NewRecorder()

	handler.Preview(previewW, previewReq)
	if previewW.Code != http.StatusOK {
		t.Fatalf("Preview falhou: %d - %s", previewW.Code, previewW.Body.String())
	}

	// Salva o preview gerado em storage
	previewPath := filepath.Join(storagePath, "preview_marcio_governo_de_sao_paulo.wav")
	_ = os.WriteFile(previewPath, previewW.Body.Bytes(), 0644)
}
