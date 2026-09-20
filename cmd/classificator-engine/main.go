package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"dialer-go/internal/domain"
	"dialer-go/internal/ports"
)

func main() {
	port := flag.Int("port", getEnvInt("PORT", 2801), "Porta de escuta do classificator engine (2801, 2802, 2803)")
	voskURL := flag.String("vosk-url", getEnvStr("VOSK_SERVER_URL", "ws://127.0.0.1:2700"), "URL do servidor Vosk ASR")
	maxDurSec := flag.Float64("max-dur", getEnvFloat("VOSK_MAX_DURATION_SEC", 3.5), "Duracao maxima da janela em segundos")
	flag.Parse()

	addr := fmt.Sprintf(":%d", *port)
	classifier := NewSemanticClassifier()

	mux := http.NewServeMux()

	// Endpoint de Health-Check para o Router
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":   "UP",
			"port":     *port,
			"vosk_url": *voskURL,
			"time":     time.Now().Unix(),
		})
	})

	// Ponto de entrada TCP/WebSocket nativo
	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("[FATAL] Falha ao escutar na porta %s: %v", addr, err)
	}

	log.Printf("==================================================================")
	log.Printf(" [CLASSIFICATOR-ENGINE] Ativo na porta %s", addr)
	log.Printf(" [VOSK UPSTREAM] %s | Janela Maxima: %.1fs", *voskURL, *maxDurSec)
	log.Printf("==================================================================")

	var wg sync.WaitGroup // Rastreador de sessões ativas

	// Tratamento direto de conexões TCP para garantir compatibilidade com o transparent proxy
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return // Sai do loop quando listener.Close() for chamado
			}

			wg.Add(1) // Adiciona uma sessão ativa
			go func(c net.Conn) {
				defer wg.Done() // Libera ao terminar
				handleRawConnection(c, *voskURL, classifier, *maxDurSec)
			}(conn)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Printf("[ENGINE-%d] Recebido sinal de parada. Recusando novas conexões...", *port)
	_ = listener.Close() // Router instantaneamente fará o Hunting para outra porta
	_ = server.Shutdown(context.Background())

	log.Printf("[ENGINE-%d] Aguardando chamadas em andamento finalizarem (Graceful Draining)...", *port)

	// Espera as goroutines terminarem com um hard-timeout de 5 segundos
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Printf("[ENGINE-%d] Todas as chamadas finalizadas com sucesso.", *port)
	case <-time.After(5 * time.Second):
		log.Printf("[ENGINE-%d] Timeout de draining (5s). Forçando encerramento.", *port)
	}

	log.Printf("[ENGINE-%d] Encerrado.", *port)
}

func handleRawConnection(conn net.Conn, voskURL string, classifier ports.SemanticClassifierPort, maxDur float64) {
	defer conn.Close()

	// 1. Handshake WebSocket RFC 6455 com o Asterisk / vosk-eagi
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	ws, err := UpgradeServerConnection(conn)
	if err != nil {
		log.Printf("[ENGINE-ERROR] Handshake WebSocket falhou: %v", err)
		return
	}
	_ = conn.SetDeadline(time.Time{})

	// 2. Conecta com o reconhecedor de fala Vosk para esta sessão
	recognizer, err := NewVoskRecognizer(voskURL, 2*time.Second)
	if err != nil {
		log.Printf("[ENGINE-ERROR] Falha ao conectar ao Vosk ASR: %v", err)
		return
	}
	defer recognizer.Close()

	session := NewSessionHandler(recognizer, classifier, maxDur)

	// Implementa sink que escreve JSON via WebSocket
	sink := &DirectEventSink{ws: ws}
	audioPipe := &WSFrameAudioReader{ws: ws}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration((maxDur+2.0)*float64(time.Second)))
	defer cancel()

	verdict, err := session.ProcessSession(ctx, audioPipe, sink)
	if err != nil && err != io.EOF && err != context.Canceled {
		log.Printf("[ENGINE-SESSION] Erro no processamento: %v", err)
	}

	log.Printf("[ENGINE-VERDICT] STATUS=%s CAUSA=%s TEXTO='%s' LATENCIA=%dms",
		verdict.Status, verdict.Cause, verdict.Transcription, verdict.LatencyMs)
}

// WSFrameAudioReader encapsula a extração de chunks PCM lineares diretamente dos frames WebSocket binários.
type WSFrameAudioReader struct {
	ws  *wsConn
	buf bytes.Buffer
}

func (r *WSFrameAudioReader) Read(p []byte) (int, error) {
	if r.buf.Len() > 0 {
		return r.buf.Read(p)
	}

	for {
		opcode, payload, err := r.ws.ReadFrame(0)
		if err != nil {
			return 0, err
		}
		// Se for texto (ex: handshake {"action":"start"...}), ignora e continua para o audio
		if opcode == 0x01 {
			continue
		}
		// Se for binario (PCM 16-bit 8000Hz), armazena no buffer
		if opcode == 0x02 {
			if len(payload) > 0 {
				r.buf.Write(payload)
				return r.buf.Read(p)
			}
		}
	}
}

// DirectEventSink emite eventos para a conexão do Asterisk via WebSocket RFC 6455.
type DirectEventSink struct {
	ws *wsConn
}

func (s *DirectEventSink) EmitEvent(msg domain.WireEventMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return s.ws.WriteServerText(string(data))
}

func getEnvStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return def
}

func getEnvFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}
