package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"dialer-go/internal/domain"
	"dialer-go/internal/ports"
)

// WhisperTranscriptionWorker consome jobs de transcrição de áudio assincronamente via Redis Streams.
//
// @pattern Consumer Worker / Async Audio Processor
// @governedBy docs/rules/TELEPHONY_POLICIES.md
// @preExecution Valida a existência do arquivo WAV gravado e a conectividade com o motor Faster-Whisper.
// @postExecution Atualiza a coluna transcription na tabela cdrs do Dialer-Go e confirma o ACK no stream.
type WhisperTranscriptionWorker struct {
	cache      ports.CachePort
	repRepo    ports.ReportRepository
	whisperURL string
	httpClient *http.Client
	stopCh     chan struct{}
	wg         sync.WaitGroup
}

// WhisperResponse mapeia a resposta padrão compatível com OpenAI/Faster-Whisper API.
type WhisperResponse struct {
	Text  string `json:"text"`
	Error string `json:"error,omitempty"`
}

// NewWhisperTranscriptionWorker cria uma nova instância do worker de transcrição Faster-Whisper.
func NewWhisperTranscriptionWorker(cache ports.CachePort, repRepo ports.ReportRepository, whisperURL string) *WhisperTranscriptionWorker {
	if strings.TrimSpace(whisperURL) == "" {
		whisperURL = os.Getenv("WHISPER_URL")
	}
	if strings.TrimSpace(whisperURL) == "" {
		whisperURL = "http://localhost:8090/v1/audio/transcriptions"
	}

	return &WhisperTranscriptionWorker{
		cache:      cache,
		repRepo:    repRepo,
		whisperURL: whisperURL,
		httpClient: &http.Client{
			Timeout: 120 * time.Second, // Timeout generoso para arquivos longos
		},
		stopCh: make(chan struct{}),
	}
}

// StartDaemon inicializa a rotina de consumo contínuo da fila de transcrição.
func (w *WhisperTranscriptionWorker) StartDaemon(ctx context.Context) {
	if w.cache == nil || w.repRepo == nil {
		log.Println("[WARN] [WHISPER-WORKER] Cache ou Repositório nulos. Worker de transcrição não iniciado.")
		return
	}

	w.wg.Add(1)
	go w.workerLoop(ctx)
	log.Printf("[INFO] [WHISPER-WORKER] Worker assíncrono Faster-Whisper iniciado (endpoint: %s)\n", w.whisperURL)
}

// Stop sinaliza a parada graciosa do worker.
func (w *WhisperTranscriptionWorker) Stop() {
	close(w.stopCh)
	w.wg.Wait()
}

func (w *WhisperTranscriptionWorker) workerLoop(ctx context.Context) {
	defer w.wg.Done()

	group := "whisper_workers"
	consumer := fmt.Sprintf("whisper_%d", os.Getpid())

	for {
		select {
		case <-w.stopCh:
			return
		case <-ctx.Done():
			return
		default:
			messages, err := w.cache.ReadTranscriptionJobs(ctx, group, consumer, 5, 2*time.Second)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				time.Sleep(1 * time.Second)
				continue
			}

			for _, msg := range messages {
				if msg == nil || msg.Job == nil {
					continue
				}
				w.processJob(ctx, group, msg)
			}
		}
	}
}

func (w *WhisperTranscriptionWorker) processJob(ctx context.Context, group string, msg *domain.TranscriptionJobMessage) {
	job := msg.Job

	// 1. Localização e validação do arquivo de áudio no disco
	resolvedPath, err := w.resolveAudioFile(job.RecordingFile)
	if err != nil {
		// Se o arquivo ainda não existe e a chamada é recente (< 30s), pode ser delay do merge-stereo
		if time.Since(job.EnqueuedAt) < 30*time.Second {
			log.Printf("[DEBUG] [WHISPER-WORKER] Áudio %s ainda não encontrado (aguardando merge): %v\n", job.RecordingFile, err)
			return // Não faz ACK ainda, será retentado
		}

		log.Printf("[WARN] [WHISPER-WORKER] Áudio %s expirou sem ser encontrado após 30s. Descartando job: %v\n", job.RecordingFile, err)
		_ = w.cache.AckTranscriptionJob(ctx, group, msg.ID)
		return
	}

	// 2. Disparo de transcrição para o Faster-Whisper
	transcription, err := w.sendToWhisper(ctx, resolvedPath)
	if err != nil {
		log.Printf("[ERROR] [WHISPER-WORKER] Falha na transcrição do áudio %s (CallID: %s): %v\n", resolvedPath, job.CallID, err)
		// Em caso de falha de conexão com Whisper, não dá ACK imediatamente para retentar depois
		return
	}

	// 3. Atualização do CDR no PostgreSQL
	if strings.TrimSpace(transcription) != "" {
		if err := w.repRepo.UpdateCDRTranscription(ctx, job.CallID, transcription); err != nil {
			log.Printf("[ERROR] [WHISPER-WORKER] Falha ao salvar transcrição no CDR %s: %v\n", job.CallID, err)
			return
		}
		log.Printf("[INFO] [WHISPER-WORKER] Transcrição concluída com sucesso para CallID %s (%d chars)\n", job.CallID, len(transcription))
	} else {
		log.Printf("[DEBUG] [WHISPER-WORKER] Áudio vazio ou sem fala detectada para CallID %s\n", job.CallID)
	}

	// 4. Confirmação de processamento no Redis Stream
	if err := w.cache.AckTranscriptionJob(ctx, group, msg.ID); err != nil {
		log.Printf("[WARN] [WHISPER-WORKER] Falha ao enviar ACK para job %s: %v\n", msg.ID, err)
	}
}

// resolveAudioFile verifica a existência física do arquivo, testando diretórios padrão de monitoramento.
func (w *WhisperTranscriptionWorker) resolveAudioFile(audioPath string) (string, error) {
	candidates := []string{
		audioPath,
		filepath.Join("/var/spool/asterisk/monitor", filepath.Base(audioPath)),
		filepath.Join("/var/spool/asterisk/monitor", audioPath),
	}

	for _, cand := range candidates {
		if cand == "" {
			continue
		}
		if info, err := os.Stat(cand); err == nil && !info.IsDir() && info.Size() > 0 {
			return cand, nil
		}
	}

	return "", fmt.Errorf("arquivo não encontrado ou vazio: %s", audioPath)
}

// sendToWhisper envia o arquivo de áudio WAV via requisição HTTP Multipart para o Faster-Whisper.
func (w *WhisperTranscriptionWorker) sendToWhisper(ctx context.Context, filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("erro ao abrir arquivo: %w", err)
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return "", fmt.Errorf("erro ao criar form file: %w", err)
	}

	if _, err := io.Copy(part, file); err != nil {
		return "", fmt.Errorf("erro ao copiar conteúdo do áudio: %w", err)
	}

	_ = writer.WriteField("model", "base")
	_ = writer.WriteField("language", "pt")
	_ = writer.WriteField("response_format", "json")

	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("erro ao fechar multipart writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.whisperURL, body)
	if err != nil {
		return "", fmt.Errorf("erro ao criar requisição HTTP: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("erro na chamada HTTP para o Whisper: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("erro ao ler resposta do Whisper: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("whisper retornou status %d: %s", resp.StatusCode, string(respBody))
	}

	var wResp WhisperResponse
	if err := json.Unmarshal(respBody, &wResp); err != nil {
		return "", fmt.Errorf("erro ao decodificar JSON do Whisper: %w", err)
	}

	if wResp.Error != "" {
		return "", fmt.Errorf("whisper reportou erro: %s", wResp.Error)
	}

	return strings.TrimSpace(wResp.Text), nil
}
