package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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

	maxDuration := 6.0 // Janela para reprodução do áudio estruturado + resposta
	if envDur := os.Getenv("VOSK_MAX_DURATION_SEC"); envDur != "" {
		if d, err := strconv.ParseFloat(envDur, 64); err == nil && d > 0 {
			maxDuration = d
		}
	}

	// Carrega personalização dinâmica do AMD salva pelo Dialer-Go se houver
	loadDynamicConfig(&maxDuration)

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

	chunkBuf := make([]byte, 1600) // 100ms de áudio PCM 16-bit 8kHz

	for time.Since(startTime) < time.Duration(maxDuration*float64(time.Second)) {
		n, readErr := audioFile.Read(chunkBuf)
		if n > 0 {
			if writeErr := wsClient.WriteBinary(chunkBuf[:n]); writeErr != nil {
				break
			}
		}
		if readErr != nil {
			break
		}

		rawMsg, err := wsClient.ReadMessage(50 * time.Millisecond)
		if err == nil && len(rawMsg) > 0 {
			var vm VoskMessage
			if jsonErr := json.Unmarshal(rawMsg, &vm); jsonErr == nil {
				txt := normalizeText(vm.Text)
				part := normalizeText(vm.Partial)

				current := txt
				if current == "" {
					current = part
				}

				if txt != "" && !strings.Contains(fullText, txt) {
					fullText = strings.TrimSpace(fullText + " " + txt)
				}

				// 1. Checagem de Caixa Postal / Operadora
				for _, phrase := range voicemailPhrases {
					if strings.Contains(current, phrase) || strings.Contains(fullText, phrase) {
						status = "MACHINE"
						cause = "VOICEMAIL_" + strings.ToUpper(strings.ReplaceAll(phrase, " ", "_"))
						break
					}
				}

				if status == "MACHINE" {
					finished.Store(true)
					break
				}

				// 2. Checagem de Saudação / Confirmação Humana ("Alô", "Sim, sou eu", etc.)
				for _, greeting := range humanGreetings {
					if strings.Contains(current, greeting) || strings.Contains(fullText, greeting) {
						status = "HUMAN"
						cause = "HUMAN_" + strings.ToUpper(strings.ReplaceAll(greeting, " ", "_"))
						break
					}
				}

				if status == "HUMAN" && strings.HasPrefix(cause, "HUMAN_") {
					finished.Store(true)
					break
				}
			}
		}
	}

	finished.Store(true)

	if status != "MACHINE" {
		_ = wsClient.WriteText(`{"eof" : 1}`)
		if rawMsg, err := wsClient.ReadMessage(200 * time.Millisecond); err == nil && len(rawMsg) > 0 {
			var vm VoskMessage
			if jsonErr := json.Unmarshal(rawMsg, &vm); jsonErr == nil {
				txt := normalizeText(vm.Text)
				if txt != "" && !strings.Contains(fullText, txt) {
					fullText = strings.TrimSpace(fullText + " " + txt)
				}
			}
		}

		// Classificação final: saudações, termos de operadora, fala natural ou silêncio (VOICEMAIL_SILENCE)
		status, cause = ClassifyOutcome(fullText, voicemailPhrases, humanGreetings)
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

	var files []string
	if fileExists(filepath.Join(audioBaseDir, "saudacao.wav")) {
		files = append(files, filepath.Join(audioBaseDir, "saudacao"))
	} else if fileExists(filepath.Join(audioBaseDir, "base", "saudacao.wav")) {
		files = append(files, filepath.Join(audioBaseDir, "base", "saudacao"))
	}

	if workWord != "" {
		normWork := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(workWord), " ", "_"))
		if fileExists(filepath.Join(audioBaseDir, "work_words", normWork+".wav")) {
			files = append(files, filepath.Join(audioBaseDir, "work_words", normWork))
		}
	}

	if fileExists(filepath.Join(audioBaseDir, "falo_com.wav")) {
		files = append(files, filepath.Join(audioBaseDir, "falo_com"))
	} else if fileExists(filepath.Join(audioBaseDir, "base", "falo_com.wav")) {
		files = append(files, filepath.Join(audioBaseDir, "base", "falo_com"))
	}

	if audioName != "" {
		normName := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(audioName), " ", "_"))
		if fileExists(filepath.Join(audioBaseDir, "names", normName+".wav")) {
			files = append(files, filepath.Join(audioBaseDir, "names", normName))
		}
	}

	if len(files) == 0 {
		finished.Store(true)
		return
	}

	concatPath := strings.Join(files, "&")
	agiSend(fmt.Sprintf("EXEC Background %s", concatPath))
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


