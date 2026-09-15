package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

/**
 * Cliente de transcrição em lote com o Vosk Server via WebSocket.
 *
 * @pattern Strategy / Adapter
 * @governedBy .agents/workflows/agente-especialista-sst.md
 */

type VoskResult struct {
	Text    string `json:"text"`
	Partial string `json:"partial"`
}

func TranscribeAudio(wsURL string, pcmData []byte, timeout time.Duration) (string, error) {
	conn, err := connectWebSocket(wsURL, 5*time.Second)
	if err != nil {
		return "", fmt.Errorf("falha ao conectar ao Vosk (%s): %w", wsURL, err)
	}
	defer conn.Close()

	// Configura taxa de amostragem
	cfgMsg := `{"config" : { "sample_rate" : 8000 }}`
	if err := conn.WriteText(cfgMsg); err != nil {
		return "", fmt.Errorf("falha ao configurar Vosk: %w", err)
	}

	var mu sync.Mutex
	var textParts []string
	var lastPartial string
	readDone := make(chan struct{})

	// Goroutine de leitura contínua de respostas do Vosk
	go func() {
		defer close(readDone)
		for {
			resp, err := conn.ReadMessage(4 * time.Second)
			if err != nil {
				break
			}
			if len(resp) == 0 {
				continue
			}

			var vr VoskResult
			if err := json.Unmarshal(resp, &vr); err == nil {
				mu.Lock()
				txt := strings.TrimSpace(vr.Text)
				part := strings.TrimSpace(vr.Partial)
				if txt != "" {
					textParts = append(textParts, txt)
				}
				if part != "" {
					lastPartial = part
				}
				mu.Unlock()
			}
		}
	}()

	chunkSize := 1600 // 100ms de áudio PCM 16-bit 8000Hz
	for i := 0; i < len(pcmData); i += chunkSize {
		end := i + chunkSize
		if end > len(pcmData) {
			end = len(pcmData)
		}
		chunk := pcmData[i:end]
		if err := conn.WriteBinary(chunk); err != nil {
			break
		}
	}

	// Envia frame de encerramento {"eof": 1}
	_ = conn.WriteText(`{"eof" : 1}`)

	// Aguarda processamento final do Vosk
	select {
	case <-readDone:
	case <-time.After(3 * time.Second):
	}

	mu.Lock()
	defer mu.Unlock()

	fullText := strings.TrimSpace(strings.Join(textParts, " "))
	if fullText == "" && lastPartial != "" {
		fullText = lastPartial
	}

	return fullText, nil
}

