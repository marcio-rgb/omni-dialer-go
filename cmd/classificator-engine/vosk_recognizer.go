package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"dialer-go/internal/domain"
	"dialer-go/internal/ports"
)

type voskJSONMessage struct {
	Text    string `json:"text"`
	Partial string `json:"partial"`
}

// VoskRecognizer implementa ports.SpeechRecognizerPort consumindo o servidor Kaldi-Vosk via WebSocket.
//
// @pattern Adapter Pattern
// @governedBy .agents/ARCHITECT.md
type VoskRecognizer struct {
	wsConn    *wsConn
	url       string
	mu        sync.Mutex
	lastPart  string
	lastFinal string
	isClosed  bool
}

// NewVoskRecognizer conecta ao servidor Kaldi-Vosk e envia o handshake de configuração (8kHz).
func NewVoskRecognizer(wsURL string, timeout time.Duration) (ports.SpeechRecognizerPort, error) {
	if wsURL == "" {
		wsURL = "ws://127.0.0.1:2700"
	}

	conn, err := connectWebSocket(wsURL, timeout)
	if err != nil {
		return nil, fmt.Errorf("falha ao conectar no servidor Vosk (%s): %w", wsURL, err)
	}

	// Handshake de áudio telefônico nativo 8000 Hz
	if err := conn.WriteText(`{"config" : { "sample_rate" : 8000 }}`); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("falha ao enviar config de sample_rate ao Vosk: %w", err)
	}

	r := &VoskRecognizer{
		wsConn: conn,
		url:    wsURL,
	}

	// Goroutine em background para ler mensagens recebidas do Vosk
	go r.readLoop()

	return r, nil
}

func (r *VoskRecognizer) readLoop() {
	for {
		raw, err := r.wsConn.ReadMessage(0)
		if err != nil {
			r.mu.Lock()
			r.isClosed = true
			r.mu.Unlock()
			return
		}

		if len(raw) == 0 {
			continue
		}

		var vm voskJSONMessage
		if err := json.Unmarshal(raw, &vm); err == nil {
			r.mu.Lock()
			if vm.Text != "" {
				if r.lastFinal != "" {
					r.lastFinal = strings.TrimSpace(r.lastFinal + " " + vm.Text)
				} else {
					r.lastFinal = vm.Text
				}
				r.lastPart = ""
			} else if vm.Partial != "" {
				r.lastPart = vm.Partial
			}
			r.mu.Unlock()
		}
	}
}

// AcceptAudio despacha o chunk binário PCM diretamente para o WebSocket do Vosk.
func (r *VoskRecognizer) AcceptAudio(frame domain.AudioFrame) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.isClosed || r.wsConn == nil {
		return fmt.Errorf("conexao com vosk esta fechada")
	}

	return r.wsConn.WriteBinary(frame.PCMData)
}

// GetPartial retorna a hipótese parcial em tempo real.
func (r *VoskRecognizer) GetPartial() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastPart
}

// GetFinal retorna o texto acumulado final.
func (r *VoskRecognizer) GetFinal() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastFinal
}

// Reset limpa buffers textuais para a chamada atual.
func (r *VoskRecognizer) Reset() {
	r.mu.Lock()
	r.lastPart = ""
	r.lastFinal = ""
	r.mu.Unlock()
}

// Close finaliza o socket com o Vosk.
func (r *VoskRecognizer) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.isClosed = true
	if r.wsConn != nil {
		_ = r.wsConn.WriteText(`{"eof" : 1}`)
		return r.wsConn.Close()
	}
	return nil
}
