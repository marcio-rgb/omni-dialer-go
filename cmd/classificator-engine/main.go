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
	port := flag.Int("port", getEnvInt("PORT", 2801), "Porta de escuta HTTP/WS do engine (2801, 2802, 2803)")
	audioSocketPort := flag.Int("audiosocket-port", getEnvInt("AUDIOSOCKET_PORT", 9092), "Porta de escuta do AudioSocket Asterisk nativo")
	voskURL := flag.String("vosk-url", getEnvStr("VOSK_SERVER_URL", "ws://127.0.0.1:2700"), "URL do servidor Vosk ASR")
	maxDurSec := flag.Float64("max-dur", getEnvFloat("VOSK_MAX_DURATION_SEC", 3.5), "Duracao maxima da janela em segundos")
	redisAddr := flag.String("redis-addr", getEnvStr("REDIS_ADDR", "127.0.0.1:6379"), "Endereco do servidor Redis")
	redisPass := flag.String("redis-pass", getEnvStr("REDIS_PASSWORD", ""), "Senha do servidor Redis")
	redisDB := flag.Int("redis-db", getEnvInt("REDIS_DB", 0), "Database Redis")
	maxSlots := flag.Int("max-slots", getEnvInt("MAX_CONCURRENT_SESSIONS", 40), "Limite maximo de sessoes concorrentes")
	flag.Parse()

	addr := fmt.Sprintf(":%d", *port)
	classifier := NewSemanticClassifier()
	sem := make(chan struct{}, *maxSlots)

	// Inicia o servidor AudioSocket nativo do Asterisk na porta :9092
	audioSocketAddr := fmt.Sprintf(":%d", *audioSocketPort)
	audioSocketServer := NewAudioSocketServer(
		audioSocketAddr,
		*voskURL,
		classifier,
		*maxDurSec,
		*redisAddr,
		*redisPass,
		*redisDB,
		*maxSlots,
	)
	if err := audioSocketServer.Start(); err != nil {
		log.Printf("[WARN] AudioSocket não iniciado em %s: %v", audioSocketAddr, err)
	}

	mux := http.NewServeMux()

	// Endpoint de Health-Check com telemetria de slots
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":          "UP",
			"port":            *port,
			"audiosocket_port": *audioSocketPort,
			"active_slots":    len(sem),
			"max_slots":       *maxSlots,
			"vosk_url":        *voskURL,
			"time":            time.Now().Unix(),
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
	log.Printf(" [CLASSIFICATOR-ENGINE] HTTP/WS: %s | AudioSocket: %s", addr, audioSocketAddr)
	log.Printf(" [VOSK UPSTREAM] %s | Slots Maximos: %d | Janela: %.1fs", *voskURL, *maxSlots, *maxDurSec)
	log.Printf("==================================================================")

	var wg sync.WaitGroup // Rastreador de sessões ativas

	// Tratamento de conexões TCP com Fast-Reject via semáforo
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return // Sai do loop quando listener.Close() for chamado
			}

			select {
			case sem <- struct{}{}:
				wg.Add(1)
				go func(c net.Conn) {
					defer wg.Done()
					defer func() { <-sem }()
					handleRawConnection(c, *voskURL, classifier, *maxDurSec)
				}(conn)
			default:
				log.Printf("[ENGINE-REJECT] Capacidade máxima (%d slots) atingida. Rejeitando conexão de %s",
					*maxSlots, conn.RemoteAddr())
				_ = conn.Close()
			}
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Printf("[ENGINE-%d] Recebido sinal de parada. Recusando novas conexões...", *port)
	_ = listener.Close()
	_ = audioSocketServer.Close()
	_ = server.Shutdown(context.Background())

	log.Printf("[ENGINE-%d] Aguardando chamadas em andamento finalizarem (Graceful Draining)...", *port)

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
