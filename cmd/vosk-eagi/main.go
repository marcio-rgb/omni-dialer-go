package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"dialer-go/internal/domain"
)

func main() {
	readAGIEnvironment()

	audioFile := os.NewFile(3, "eagi_audio")
	if audioFile == nil {
		agiSetVar("VOSK_AMD_STATUS", "HUMAN")
		agiSetVar("VOSK_AMD_CAUSE", "NO_EAGI_FD3")
		agiSetVar("VOSK_AMD_TEXT", "")
		agiVerbose("VOSK-EAGI: FD 3 indisponivel. Fallback seguro para HUMAN.", 1)
		return
	}
	defer audioFile.Close()
	_ = syscall.SetNonblock(3, false)

	audioName := ""
	workWord := ""
	if len(os.Args) > 1 {
		audioName = strings.TrimSpace(os.Args[1])
	}
	if len(os.Args) > 2 {
		workWord = strings.TrimSpace(os.Args[2])
	}

	routerURL := os.Getenv("VOSK_ROUTER_URL")
	if routerURL == "" {
		routerURL = "ws://classificator-router:2800"
	}

	// 1. Inicia Goroutine de Reprodução Ativa de Áudio Estruturado via Asterisk AGI
	var finished atomic.Bool
	go playStructuredAudio(audioName, workWord, &finished)

	// 2. Conecta ao Classificator-Router de Ultra-Baixa Latência (< 150ms timeout)
	wsClient, err := connectWebSocket(routerURL, 150*time.Millisecond)
	if err != nil {
		// Fallback secundário de contingência para o Vosk legado caso router esteja offline
		legacyURL := os.Getenv("VOSK_SERVER_URL")
		if legacyURL == "" {
			legacyURL = "ws://dialer-vosk:2700"
		}
		wsClient, err = connectWebSocket(legacyURL, 150*time.Millisecond)
	}

	if err != nil {
		agiVerbose(fmt.Sprintf("VOSK-EAGI: Conexao com Classificator falhou (%s): %v. Fallback seguro para HUMAN.", routerURL, err), 1)
		agiSetVar("VOSK_AMD_STATUS", "HUMAN")
		agiSetVar("VOSK_AMD_CAUSE", "ROUTER_FALLBACK_SAFE")
		agiSetVar("VOSK_AMD_TEXT", "")
		finished.Store(true)
		return
	}
	defer wsClient.Close()

	// Handshake com o motor de classificação
	_ = wsClient.WriteText(`{"action":"start","sample_rate":8000,"max_duration_sec":3.5}`)

	status := "HUMAN"
	cause := "FALLBACK_ASSUMED_HUMAN"
	fullText := ""

	// 3. Goroutine leitora de eventos assíncronos do Classificator
	verdictChan := make(chan domain.WireEventMessage, 1)
	go func() {
		for {
			rawMsg, readErr := wsClient.ReadMessage(0)
			if readErr != nil {
				return
			}
			if len(rawMsg) == 0 {
				continue
			}

			var event domain.WireEventMessage
			if err := json.Unmarshal(rawMsg, &event); err == nil {
				if event.Text != "" {
					fullText = event.Text
					agiSetVar("VOSK_TRANSCRIPTION", fullText)
				}
				if event.Type == domain.WireEventVerdict {
					select {
					case verdictChan <- event:
					default:
					}
					return
				}
			}
		}
	}()

	// 4. Loop de envio de frames de áudio PCM 16-bit 8000Hz (FD 3)
	chunkBuf := make([]byte, 1600)
	startTime := time.Now()

	for {
		select {
		case verdict := <-verdictChan:
			status = string(verdict.Status)
			cause = verdict.Cause
			if verdict.Text != "" {
				fullText = verdict.Text
			}
			finished.Store(true)
			goto finish
		default:
		}

		if time.Since(startTime) > 4*time.Second {
			break
		}

		n, readErr := audioFile.Read(chunkBuf)
		if n > 0 {
			if writeErr := wsClient.WriteBinary(chunkBuf[:n]); writeErr != nil {
				break
			}
		}
		if readErr != nil {
			if errors.Is(readErr, syscall.EAGAIN) || errors.Is(readErr, syscall.EWOULDBLOCK) {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			if !errors.Is(readErr, io.EOF) {
				agiVerbose(fmt.Sprintf("VOSK-EAGI: Leitura FD 3: %v", readErr), 1)
			}
			break
		}
	}

	// Aguarda veredito com grace period rápido se ainda não recebido
	select {
	case verdict := <-verdictChan:
		status = string(verdict.Status)
		cause = verdict.Cause
		if verdict.Text != "" {
			fullText = verdict.Text
		}
	case <-time.After(300 * time.Millisecond):
	}

finish:
	finished.Store(true)
	agiVerbose(fmt.Sprintf("VOSK-EAGI Concluido: STATUS=%s CAUSA=%s TEXTO='%s'", status, cause, fullText), 1)
	agiSetVar("VOSK_AMD_STATUS", status)
	agiSetVar("VOSK_AMD_CAUSE", cause)
	agiSetVar("VOSK_AMD_TEXT", fullText)
	agiSetVar("VOSK_TRANSCRIPTION", fullText)
}

func playStructuredAudio(audioName, workWord string, finished *atomic.Bool) {
	audioBaseDir := "/var/lib/asterisk/sounds"
	if envBase := os.Getenv("AUDIO_CACHE_DIR"); envBase != "" {
		audioBaseDir = envBase
	}

	targetAudio := ""
	for _, candidate := range []string{
		filepath.Join(audioBaseDir, "custom", "alo_tudo_bem"),
		filepath.Join(audioBaseDir, "custom", "ola_tudo_bem"),
		filepath.Join(audioBaseDir, "words", "alo_tudo_bem"),
		filepath.Join(audioBaseDir, "alo_tudo_bem"),
		filepath.Join(audioBaseDir, "base", "alo_tudo_bem"),
	} {
		if fileExists(candidate + ".wav") {
			targetAudio = candidate
			break
		}
	}

	if targetAudio == "" {
		finished.Store(true)
		return
	}

	agiSend(fmt.Sprintf("EXEC Background %s", targetAudio))
	finished.Store(true)
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir() && info.Size() > 0
}

func readAGIEnvironment() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			break
		}
	}
	go func() {
		for scanner.Scan() {
			// drena
		}
	}()
}

var agiMu sync.Mutex

func agiSend(cmd string) {
	agiMu.Lock()
	defer agiMu.Unlock()
	fmt.Fprintf(os.Stdout, "%s\n", cmd)
}

func agiSetVar(name, value string) {
	agiSend(fmt.Sprintf("SET VARIABLE %s %q", name, value))
}

func agiVerbose(msg string, level int) {
	agiSend(fmt.Sprintf("VERBOSE %q %d", msg, level))
}
