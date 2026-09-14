package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"dialer-go/internal/domain"
	"dialer-go/internal/ports"
)

// AudioWordManager orquestra o repositório de palavras em cache e a concatenação em tempo de teste
//
// @pattern Service
// @governedBy /docs/rules/ARCHITECT.md
type AudioWordManager struct {
	baseDir      string
	tts          ports.TTSPort
	concatenator *AudioConcatenator
}

// NewAudioWordManager instancia o gerenciador de cache de áudios
//
// @pattern Service (Constructor)
// @governedBy /docs/rules/ARCHITECT.md
func NewAudioWordManager(baseDir string, tts ports.TTSPort, concatenator *AudioConcatenator) (*AudioWordManager, error) {
	if baseDir == "" {
		baseDir = "./storage/audio_cache"
	}
	for _, sub := range []string{"base", "names", "work_words"} {
		p := filepath.Join(baseDir, sub)
		if err := os.MkdirAll(p, 0755); err != nil {
			return nil, fmt.Errorf("falha ao preparar diretorio %s: %w", p, err)
		}
	}
	return &AudioWordManager{
		baseDir:      baseDir,
		tts:          tts,
		concatenator: concatenator,
	}, nil
}

// SanitizeSlug normaliza chaves de arquivos para formato seguro em disco (minúsculas e underscores)
func (m *AudioWordManager) SanitizeSlug(s string) string {
	return domain.Slugify(s)
}

// UpsertWords verifica e gera em background as palavras ausentes no cache
//
// @pattern Service (Method)
// @governedBy /docs/rules/ARCHITECT.md
//
// @preExecution
// - Validar DTO de entrada através de `req.Validate()`
//
// @postExecution
// - Garante a persistência dos arquivos `.wav` ausentes e retorna métricas detalhadas
func (m *AudioWordManager) UpsertWords(ctx context.Context, req domain.UpsertAudioWordsRequest) (*domain.UpsertAudioWordsResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	start := time.Now()
	res := &domain.UpsertAudioWordsResponse{
		Success: true,
		Files: domain.AudioFilesMapDTO{
			Base:      make([]string, 0),
			Names:     make([]string, 0),
			WorkWords: make([]string, 0),
		},
	}

	// 1. Processa frases-base customizadas
	if req.CustomPhrases != nil {
		baseItems := []struct {
			key  string
			text string
		}{
			{"saudacao", req.CustomPhrases.Saudacao},
			{"falo_com", req.CustomPhrases.FaloCom},
			{"momentinho", req.CustomPhrases.Momentinho},
		}

		for _, item := range baseItems {
			if strings.TrimSpace(item.text) == "" {
				continue
			}
			res.TotalRequested++
			slug := m.SanitizeSlug(item.key)
			targetFile := filepath.Join(m.baseDir, "base", slug+".wav")
			fileName := slug + ".wav"

			if !req.ForceOverwrite && fileExists(targetFile) {
				res.CachedCount++
				res.Files.Base = append(res.Files.Base, fileName)
				continue
			}

			if err := m.tts.Synthesize(ctx, item.text, targetFile); err != nil {
				return nil, fmt.Errorf("falha ao sintetizar frase-base %s: %w", item.key, err)
			}
			res.CreatedCount++
			res.Files.Base = append(res.Files.Base, fileName)
		}
	}

	// 2. Processa lista de Nomes
	for _, n := range req.Names {
		res.TotalRequested++
		slug := m.SanitizeSlug(n.Key)
		targetFile := filepath.Join(m.baseDir, "names", slug+".wav")
		fileName := slug + ".wav"

		if !req.ForceOverwrite && fileExists(targetFile) {
			res.CachedCount++
			res.Files.Names = append(res.Files.Names, fileName)
			continue
		}

		if err := m.tts.Synthesize(ctx, n.Text, targetFile); err != nil {
			return nil, fmt.Errorf("falha ao sintetizar nome '%s': %w", n.Text, err)
		}
		res.CreatedCount++
		res.Files.Names = append(res.Files.Names, fileName)
	}

	// 3. Processa lista de Convênios / Órgãos (WorkWords)
	for _, w := range req.WorkWords {
		res.TotalRequested++
		slug := m.SanitizeSlug(w.Key)
		targetFile := filepath.Join(m.baseDir, "work_words", slug+".wav")
		fileName := slug + ".wav"

		if !req.ForceOverwrite && fileExists(targetFile) {
			res.CachedCount++
			res.Files.WorkWords = append(res.Files.WorkWords, fileName)
			continue
		}

		if err := m.tts.Synthesize(ctx, w.Text, targetFile); err != nil {
			return nil, fmt.Errorf("falha ao sintetizar convenio '%s': %w", w.Text, err)
		}
		res.CreatedCount++
		res.Files.WorkWords = append(res.Files.WorkWords, fileName)
	}

	res.ElapsedMs = time.Since(start).Milliseconds()
	return res, nil
}

// GeneratePreview concatena a saudação completa para teste operacional
//
// @pattern Service (Method)
// @governedBy /docs/rules/ARCHITECT.md
//
// @preExecution
// - Validar existência de cada arquivo requisitado
//
// @postExecution
// - Retorna o buffer binário WAV montado com as pausas configuradas
func (m *AudioWordManager) GeneratePreview(ctx context.Context, req domain.AudioPreviewRequest) ([]byte, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	nameSlug := m.SanitizeSlug(req.Name)
	workSlug := m.SanitizeSlug(req.WorkWord)

	saudacaoPath := filepath.Join(m.baseDir, "base", "saudacao.wav")
	workPath := filepath.Join(m.baseDir, "work_words", workSlug+".wav")
	faloComPath := filepath.Join(m.baseDir, "base", "falo_com.wav")
	namePath := filepath.Join(m.baseDir, "names", nameSlug+".wav")
	momentinhoPath := filepath.Join(m.baseDir, "base", "momentinho.wav")

	filesToConcat := []string{
		saudacaoPath,
		workPath,
		faloComPath,
		namePath,
	}

	if req.IncludeMomentinho {
		filesToConcat = append(filesToConcat, momentinhoPath)
	}

	// Validação de existência prévia
	for _, f := range filesToConcat {
		if !fileExists(f) {
			return nil, fmt.Errorf("arquivo ausente no banco de palavras: %s. Execute o upsert previamente", filepath.Base(f))
		}
	}

	return m.concatenator.ConcatenateWAVFilesWithPauses(filesToConcat, req.GetPausesSlice())
}

// EnsureNameAudio garante a existência do áudio do nome sintetizado com texto original e salvo como slug seguro.
//
// @pattern Service (Method)
// @governedBy /docs/rules/ARCHITECT.md
//
// @preExecution
// - Normaliza o nome para a chave do dialplan Asterisk (a-z e _)
//
// @postExecution
// - Se o arquivo .wav nao existir, sintetiza usando o rawName com acentos para maxima naturalidade
// - Retorna audioKey seguro, flag isNew e erro (se houver)
func (m *AudioWordManager) EnsureNameAudio(ctx context.Context, rawName string) (string, bool, error) {
	trimmed := strings.TrimSpace(rawName)
	if trimmed == "" {
		return "", false, nil
	}

	audioKey := m.SanitizeSlug(trimmed)
	if audioKey == "" {
		return "", false, nil
	}

	targetFile := filepath.Join(m.baseDir, "names", audioKey+".wav")
	if fileExists(targetFile) {
		return audioKey, false, nil
	}

	// Sintetiza USANDO O TEXTO ORIGINAL COM ACENTUAÇÃO E PONTUAÇÃO
	if err := m.tts.Synthesize(ctx, trimmed, targetFile); err != nil {
		return audioKey, false, fmt.Errorf("falha ao sintetizar audio para nome '%s': %w", trimmed, err)
	}

	return audioKey, true, nil
}

// EnsureWorkWordAudio garante a existência do áudio do convênio/órgão sintetizado com texto original e salvo como slug seguro.
func (m *AudioWordManager) EnsureWorkWordAudio(ctx context.Context, rawWord string) (string, bool, error) {
	trimmed := strings.TrimSpace(rawWord)
	if trimmed == "" {
		return "", false, nil
	}

	audioKey := m.SanitizeSlug(trimmed)
	if audioKey == "" {
		return "", false, nil
	}

	targetFile := filepath.Join(m.baseDir, "work_words", audioKey+".wav")
	if fileExists(targetFile) {
		return audioKey, false, nil
	}

	// Sintetiza USANDO O TEXTO ORIGINAL COM ACENTUAÇÃO
	if err := m.tts.Synthesize(ctx, trimmed, targetFile); err != nil {
		return audioKey, false, fmt.Errorf("falha ao sintetizar audio para convenio '%s': %w", trimmed, err)
	}

	return audioKey, true, nil
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir() && info.Size() > 0
}
