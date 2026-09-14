package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"dialer-go/internal/domain"
)

// mockTTS simula a síntese gerando um WAV PCM sintético
type mockTTS struct {
	sampleRate int
}

func (m *mockTTS) Synthesize(ctx context.Context, text string, outputPath string) error {
	ac := NewAudioConcatenator()
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

func TestAudioWordManager_UpsertAndPreview(t *testing.T) {
	tmpDir := t.TempDir()
	tts := &mockTTS{sampleRate: 22050}
	concatenator := NewAudioConcatenator()

	mgr, err := NewAudioWordManager(tmpDir, tts, concatenator)
	if err != nil {
		t.Fatalf("falha ao instanciar AudioWordManager: %v", err)
	}

	req := domain.UpsertAudioWordsRequest{
		CustomPhrases: &domain.CustomPhrasesDTO{
			Saudacao:   "Olá, sou do convênio",
			FaloCom:    "falo com",
			Momentinho: "um momentinho, por favor",
		},
		PausaMs: 60,
		Names: []domain.WordItemDTO{
			{Key: "marcio", Text: "Márcio"},
		},
		WorkWords: []domain.WordItemDTO{
			{Key: "governo_sao_paulo", Text: "Governo São Paulo"},
		},
		ForceOverwrite: false,
	}

	// 1. Primeiro Upsert (deve criar todos)
	res1, err := mgr.UpsertWords(context.Background(), req)
	if err != nil {
		t.Fatalf("UpsertWords falhou: %v", err)
	}
	if res1.CreatedCount != 5 {
		t.Errorf("esperava 5 arquivos criados, obteve %d", res1.CreatedCount)
	}

	// 2. Segundo Upsert (deve reaproveitar cache)
	res2, err := mgr.UpsertWords(context.Background(), req)
	if err != nil {
		t.Fatalf("segundo UpsertWords falhou: %v", err)
	}
	if res2.CreatedCount != 0 {
		t.Errorf("esperava 0 arquivos criados no cache, obteve %d", res2.CreatedCount)
	}
	if res2.CachedCount != 5 {
		t.Errorf("esperava 5 arquivos em cache, obteve %d", res2.CachedCount)
	}

	// 3. Teste de Preview
	previewReq := domain.AudioPreviewRequest{
		Name:              "marcio",
		WorkWord:          "governo_sao_paulo",
		PausaMs:           60,
		IncludeMomentinho: true,
	}
	wavBytes, err := mgr.GeneratePreview(context.Background(), previewReq)
	if err != nil {
		t.Fatalf("GeneratePreview falhou: %v", err)
	}
	if len(wavBytes) < 44 {
		t.Fatalf("tamanho de WAV invalido: %d", len(wavBytes))
	}
}

func TestGenerateRealPreviewFile(t *testing.T) {
	// Se os áudios reais existem em storage/audio_cache, concatena e salva o preview
	baseDir := "../../storage/audio_cache"
	if _, err := os.Stat(filepath.Join(baseDir, "base/saudacao.wav")); err != nil {
		t.Skip("Audios reais nao encontrados, pulando gravacao de arquivo de teste")
	}

	ac := NewAudioConcatenator()
	files := []string{
		filepath.Join(baseDir, "base/saudacao.wav"),
		filepath.Join(baseDir, "work_words/governo_sao_paulo.wav"),
		filepath.Join(baseDir, "base/falo_com.wav"),
		filepath.Join(baseDir, "names/marcio.wav"),
		filepath.Join(baseDir, "base/momentinho.wav"),
	}

	out, err := ac.ConcatenateWAVFiles(files, 60)
	if err != nil {
		t.Fatalf("falha ao concatenar audios reais: %v", err)
	}

	previewPath := filepath.Join(baseDir, "preview_marcio_governo_sao_paulo.wav")
	if err := os.WriteFile(previewPath, out, 0644); err != nil {
		t.Fatalf("falha ao salvar preview: %v", err)
	}
}

func TestAudioWordManager_EnsureNameAudio(t *testing.T) {
	tmpDir := t.TempDir()
	var synthesizedTexts []string
	tts := &captureTTS{
		sampleRate: 22050,
		onSynthesize: func(text string) {
			synthesizedTexts = append(synthesizedTexts, text)
		},
	}
	concatenator := NewAudioConcatenator()

	mgr, err := NewAudioWordManager(tmpDir, tts, concatenator)
	if err != nil {
		t.Fatalf("falha ao instanciar AudioWordManager: %v", err)
	}

	ctx := context.Background()

	// 1. Primeira chamada com "Márcio" (com acento)
	slug, isNew, err := mgr.EnsureNameAudio(ctx, "Márcio")
	if err != nil {
		t.Fatalf("EnsureNameAudio falhou: %v", err)
	}
	if slug != "marcio" {
		t.Errorf("slug esperado 'marcio', obteve '%s'", slug)
	}
	if !isNew {
		t.Errorf("esperava isNew == true na primeira execução")
	}
	if len(synthesizedTexts) != 1 || synthesizedTexts[0] != "Márcio" {
		t.Errorf("texto sintetizado incorreto: esperava 'Márcio', obteve %v", synthesizedTexts)
	}

	// 2. Segunda chamada com mesmo nome deve retornar cache (isNew == false)
	slug2, isNew2, err := mgr.EnsureNameAudio(ctx, "Márcio")
	if err != nil {
		t.Fatalf("EnsureNameAudio falhou na segunda chamada: %v", err)
	}
	if slug2 != "marcio" {
		t.Errorf("slug esperado 'marcio', obteve '%s'", slug2)
	}
	if isNew2 {
		t.Errorf("esperava isNew == false no cache")
	}
	if len(synthesizedTexts) != 1 {
		t.Errorf("não deveria sintetizar novamente: total sintetizado=%d", len(synthesizedTexts))
	}
}

type captureTTS struct {
	sampleRate   int
	onSynthesize func(text string)
}

func (c *captureTTS) Synthesize(ctx context.Context, text string, outputPath string) error {
	if c.onSynthesize != nil {
		c.onSynthesize(text)
	}
	ac := NewAudioConcatenator()
	pcm := make([]byte, (uint32(c.sampleRate)*50/1000)*2)
	wav, err := ac.BuildWAV(pcm, uint32(c.sampleRate), 1, 16)
	if err != nil {
		return err
	}
	return os.WriteFile(outputPath, wav, 0644)
}

func (c *captureTTS) GetSampleRate() int {
	return c.sampleRate
}
