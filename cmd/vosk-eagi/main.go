package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type VoskMessage struct {
	Text    string `json:"text"`
	Partial string `json:"partial"`
}

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

	wsURL := os.Getenv("VOSK_SERVER_URL")
	if wsURL == "" {
		wsURL = "ws://127.0.0.1:2700"
	}

	maxDuration := 3.5 // Janela de 3.5s: 1.1s para áudio "Olá, tudo bem!?" + 2.4s para resposta do cliente
	if envDur := os.Getenv("VOSK_MAX_DURATION_SEC"); envDur != "" {
		if d, err := strconv.ParseFloat(envDur, 64); err == nil && d > 0 {
			maxDuration = d
		}
	}

	// Carrega personalização dinâmica do AMD salva pelo Dialer-Go se houver
	LoadDynamicConfig(&maxDuration)

	wsClient, err := connectWebSocket(wsURL, 1500*time.Millisecond)
	if err != nil {
		agiVerbose(fmt.Sprintf("VOSK-EAGI: Conexao com Vosk falhou: %v. Fallback seguro para HUMAN.", err), 1)
		agiSetVar("VOSK_AMD_STATUS", "HUMAN")
		agiSetVar("VOSK_AMD_CAUSE", "VOSK_CONN_FALLBACK")
		agiSetVar("VOSK_AMD_TEXT", "")
		return
	}
	defer wsClient.Close()

	_ = wsClient.WriteText(`{"config" : { "sample_rate" : 8000 }}`)

	// Inicia Goroutine de Reprodução Ativa de Áudio Estruturado via Asterisk AGI
	var finished atomic.Bool
	go playStructuredAudio(audioName, workWord, &finished)

	status := "UNKNOWN"
	cause := "PENDING"
	fullText := ""
	startTime := time.Now()
	var firstSpeechTime time.Time
	var lastSpeechTime time.Time

	chunkBuf := make([]byte, 1600) // 100ms de áudio PCM 16-bit 8kHz

	for time.Since(startTime) < time.Duration(maxDuration*float64(time.Second)) {
		n, readErr := audioFile.Read(chunkBuf)
		if n > 0 {
			if writeErr := wsClient.WriteBinary(chunkBuf[:n]); writeErr != nil {
				agiVerbose(fmt.Sprintf("VOSK-EAGI: Erro ao enviar audio ao Vosk: %v", writeErr), 1)
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

		rawMsg, err := wsClient.ReadMessage(40 * time.Millisecond)
		if err == nil && len(rawMsg) > 0 {
			var vm VoskMessage
			if jsonErr := json.Unmarshal(rawMsg, &vm); jsonErr == nil {
				txt := normalizeText(vm.Text)
				part := normalizeText(vm.Partial)

				current := txt
				if current == "" {
					current = part
				}

				if current != "" {
					if firstSpeechTime.IsZero() {
						firstSpeechTime = time.Now()
					}
					lastSpeechTime = time.Now()
					if !strings.Contains(fullText, current) {
						fullText = strings.TrimSpace(fullText + " " + current)
					}
				}

				// 1. Checagem prioritária e inequívoca de Caixa Postal / Operadora com tolerância fonética
				currWords := strings.Fields(current)
				fullWords := strings.Fields(fullText)
				for _, phrase := range highConfidenceVM {
					normPhrase := normalizeText(phrase)
					targetWords := strings.Fields(normPhrase)
					maxTolerance := 1
					if len(targetWords) > 2 {
						maxTolerance = 2
					}
					if fuzzyContainsPhrase(currWords, targetWords, maxTolerance) || fuzzyContainsPhrase(fullWords, targetWords, maxTolerance) {
						status = "MACHINE"
						cause = "VOICEMAIL_MATCH_" + strings.ToUpper(strings.ReplaceAll(normPhrase, " ", "_"))
						break
					}
				}

				if status == "MACHINE" {
					finished.Store(true)
					break
				}

				// 2. Checagem imediata de Saudação / Confirmação Humana ("Alô", "Quem fala", "Oi", etc.)
				paddedCurrent := " " + normalizeText(current) + " "
				for _, greeting := range quickHumanGreetings {
					normGreeting := normalizeText(greeting)
					paddedGreeting := " " + normGreeting + " "
					if strings.Contains(paddedCurrent, paddedGreeting) {
						status = "HUMAN"
						cause = "HUMAN_GREETING_BRIEF"
						break
					}
				}

				if status == "HUMAN" {
					finished.Store(true)
					break
				}
			}
		}
	}

	finished.Store(true)

	if status != "MACHINE" && status != "HUMAN" {
		_ = wsClient.WriteText(`{"eof" : 1}`)
		if rawMsg, err := wsClient.ReadMessage(200 * time.Millisecond); err == nil && len(rawMsg) > 0 {
			var vm VoskMessage
			if jsonErr := json.Unmarshal(rawMsg, &vm); jsonErr == nil {
				candidate := normalizeText(vm.Text)
				if candidate == "" {
					candidate = normalizeText(vm.Partial)
				}
				if candidate != "" && !strings.Contains(fullText, candidate) {
					fullText = strings.TrimSpace(fullText + " " + candidate)
					if firstSpeechTime.IsZero() {
						firstSpeechTime = time.Now()
					}
					lastSpeechTime = time.Now()
				}
			}
		}

		metrics := CallMetrics{
			FullText: fullText,
		}
		if !firstSpeechTime.IsZero() {
			metrics.SpeechDurationSec = lastSpeechTime.Sub(firstSpeechTime).Seconds()
			metrics.SilenceAfterSec = time.Since(lastSpeechTime).Seconds()
		}

		status, cause = ClassifyCall(metrics)
	}

	agiVerbose(fmt.Sprintf("VOSK-EAGI Concluido: STATUS=%s CAUSA=%s TEXTO='%s'", status, cause, fullText), 1)
	agiSetVar("VOSK_AMD_STATUS", status)
	agiSetVar("VOSK_AMD_CAUSE", cause)
	agiSetVar("VOSK_AMD_TEXT", fullText)
}

func playStructuredAudio(audioName, workWord string, finished *atomic.Bool) {
	audioBaseDir := "/var/lib/asterisk/sounds/words"
	if envBase := os.Getenv("AUDIO_CACHE_DIR"); envBase != "" {
		audioBaseDir = envBase
	} else if _, err := os.Stat("/var/lib/asterisk/sounds/words"); err != nil {
		if _, err2 := os.Stat("./storage/audio_cache"); err2 == nil {
			audioBaseDir = "./storage/audio_cache"
		}
	}

	targetAudio := ""
	for _, candidate := range []string{
		filepath.Join(audioBaseDir, "ola_tudo_bem"),
		filepath.Join(audioBaseDir, "words", "ola_tudo_bem"),
		filepath.Join(audioBaseDir, "base", "ola_tudo_bem"),
		filepath.Join(audioBaseDir, "alo_tudo_bem"),
		filepath.Join(audioBaseDir, "words", "alo_tudo_bem"),
		filepath.Join(audioBaseDir, "base", "alo_tudo_bem"),
		filepath.Join(audioBaseDir, "saudacao"),
		filepath.Join(audioBaseDir, "base", "saudacao"),
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


