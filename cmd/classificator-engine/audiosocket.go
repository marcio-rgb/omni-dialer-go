package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"time"

	"dialer-go/internal/domain"
	"dialer-go/internal/ports"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// AudioSocketMessageType define os tipos de mensagens do protocolo Asterisk AudioSocket.
const (
	AudioSocketTypeHangup  byte = 0x00
	AudioSocketTypeUUID    byte = 0x01
	AudioSocketTypeSilence byte = 0x02
	AudioSocketTypeAudio   byte = 0x10
	AudioSocketTypeError   byte = 0xff
)

// AudioSocketServer gerencia o listener TCP nativo para Asterisk AudioSocket (porta padrão :9092).
//
// @pattern Adapter / Server
// @governedBy .agents/ARCHITECT.md
type AudioSocketServer struct {
	addr        string
	voskURL     string
	classifier  ports.SemanticClassifierPort
	maxDurSec   float64
	redisClient *redis.Client
	sem         chan struct{}
	listener    net.Listener
}

// NewAudioSocketServer cria uma nova instância do servidor AudioSocket.
func NewAudioSocketServer(
	addr string,
	voskURL string,
	classifier ports.SemanticClassifierPort,
	maxDurSec float64,
	redisAddr string,
	redisPassword string,
	redisDB int,
	maxSlots int,
) *AudioSocketServer {
	if maxSlots <= 0 {
		maxSlots = 40
	}
	var rClient *redis.Client
	if redisAddr != "" {
		rClient = redis.NewClient(&redis.Options{
			Addr:     redisAddr,
			Password: redisPassword,
			DB:       redisDB,
		})
	}

	return &AudioSocketServer{
		addr:        addr,
		voskURL:     voskURL,
		classifier:  classifier,
		maxDurSec:   maxDurSec,
		redisClient: rClient,
		sem:         make(chan struct{}, maxSlots),
	}
}

// Start inicia o listener TCP do AudioSocket em segundo plano.
func (s *AudioSocketServer) Start() error {
	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("falha ao iniciar AudioSocket listener em %s: %w", s.addr, err)
	}
	s.listener = l

	log.Printf("[AUDIOSOCKET] Servidor ativo em %s (Slots: %d)", s.addr, cap(s.sem))

	go s.acceptLoop()
	return nil
}

// Close finaliza o listener TCP e conexões de apoio.
func (s *AudioSocketServer) Close() error {
	if s.listener != nil {
		_ = s.listener.Close()
	}
	if s.redisClient != nil {
		_ = s.redisClient.Close()
	}
	return nil
}

func (s *AudioSocketServer) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}

		// Fast-Reject via semáforo de concorrência (40 slots)
		select {
		case s.sem <- struct{}{}:
			go func(c net.Conn) {
				defer func() { <-s.sem }()
				s.handleConn(c)
			}(conn)
		default:
			log.Printf("[AUDIOSOCKET-FAST-REJECT] Limite de concorrência atingido (%d slots). Rejeitando conexão de %s",
				cap(s.sem), conn.RemoteAddr())
			_ = sendAudioSocketHangup(conn)
			_ = conn.Close()
		}
	}
}

func (s *AudioSocketServer) handleConn(conn net.Conn) {
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))

	// 1. Lê a mensagem inicial do AudioSocket: Espera Tipo UUID (0x01) com 16 bytes de payload
	header := make([]byte, 3)
	if _, err := io.ReadFull(conn, header); err != nil {
		log.Printf("[AUDIOSOCKET-ERROR] Falha ao ler cabeçalho inicial: %v", err)
		return
	}

	msgType := header[0]
	payloadLen := binary.BigEndian.Uint16(header[1:3])

	var callUUID string
	if msgType == AudioSocketTypeUUID && payloadLen == 16 {
		uuidBytes := make([]byte, 16)
		if _, err := io.ReadFull(conn, uuidBytes); err != nil {
			log.Printf("[AUDIOSOCKET-ERROR] Falha ao ler payload UUID: %v", err)
			return
		}
		parsedUUID, err := uuid.FromBytes(uuidBytes)
		if err == nil {
			callUUID = parsedUUID.String()
		}
	} else {
		// Se não iniciou com UUID, consome payload
		if payloadLen > 0 {
			_, _ = io.CopyN(io.Discard, conn, int64(payloadLen))
		}
	}

	_ = conn.SetDeadline(time.Time{})

	// 2. Busca imediata de metadados no Redis (call:meta:<UUID>)
	var meta *domain.CallMetadata
	if callUUID != "" && s.redisClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		key := fmt.Sprintf("call:meta:%s", callUUID)
		data, err := s.redisClient.Get(ctx, key).Bytes()
		if err == nil {
			var m domain.CallMetadata
			if jsonErr := json.Unmarshal(data, &m); jsonErr == nil {
				meta = &m
			}
		}
	}

	leadInfo := "UNKNOWN"
	if meta != nil {
		leadInfo = fmt.Sprintf("Lead=%s Phone=%s Camp=%s", meta.LeadID, meta.Phone, meta.CampaignID)
	}
	log.Printf("[AUDIOSOCKET-SESSION] Iniciada UUID=%s (%s)", callUUID, leadInfo)

	// 3. Conecta ao Vosk local em Loopback
	recognizer, err := NewVoskRecognizer(s.voskURL, 2*time.Second)
	if err != nil {
		log.Printf("[AUDIOSOCKET-ERROR] Falha ao conectar ao Vosk ASR (%s): %v", s.voskURL, err)
		return
	}
	defer recognizer.Close()

	session := NewSessionHandler(recognizer, s.classifier, s.maxDurSec)
	audioReader := &AudioSocketStreamReader{conn: conn}

	sink := &AudioSocketEventSink{
		callUUID: callUUID,
		meta:     meta,
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration((s.maxDurSec+2.0)*float64(time.Second)))
	defer cancel()

	verdict, err := session.ProcessSession(ctx, audioReader, sink)
	if err != nil && err != io.EOF && err != context.Canceled {
		log.Printf("[AUDIOSOCKET-SESSION] Erro no processamento UUID=%s: %v", callUUID, err)
	}

	log.Printf("[AUDIOSOCKET-VERDICT] UUID=%s STATUS=%s CAUSA=%s LATENCIA=%dms CONF=%.2f TEXT='%s'",
		callUUID, verdict.Status, verdict.Cause, verdict.LatencyMs, verdict.Confidence, verdict.Transcription)
}

// AudioSocketStreamReader lê o fluxo contínuo de PCM linear 8000Hz descartando cabeçalhos de controle.
type AudioSocketStreamReader struct {
	conn net.Conn
	buf  []byte
}

func (r *AudioSocketStreamReader) Read(p []byte) (int, error) {
	if len(r.buf) > 0 {
		n := copy(p, r.buf)
		r.buf = r.buf[n:]
		return n, nil
	}

	header := make([]byte, 3)
	for {
		if _, err := io.ReadFull(r.conn, header); err != nil {
			return 0, err
		}

		msgType := header[0]
		payloadLen := int(binary.BigEndian.Uint16(header[1:3]))

		if msgType == AudioSocketTypeHangup {
			return 0, io.EOF
		}

		if payloadLen == 0 {
			continue
		}

		payload := make([]byte, payloadLen)
		if _, err := io.ReadFull(r.conn, payload); err != nil {
			return 0, err
		}

		if msgType == AudioSocketTypeAudio {
			n := copy(p, payload)
			if n < payloadLen {
				r.buf = payload[n:]
			}
			return n, nil
		}
	}
}

// AudioSocketEventSink emite eventos de classificação.
type AudioSocketEventSink struct {
	callUUID string
	meta     *domain.CallMetadata
}

func (s *AudioSocketEventSink) EmitEvent(msg domain.WireEventMessage) error {
	if msg.Type == domain.WireEventVerdict {
		log.Printf("[AUDIOSOCKET-EVENT] UUID=%s Veredito=%s Causa=%s Texto='%s'",
			s.callUUID, msg.Status, msg.Cause, msg.Text)
	}
	return nil
}

func sendAudioSocketHangup(conn net.Conn) error {
	msg := []byte{AudioSocketTypeHangup, 0x00, 0x00}
	_, err := conn.Write(msg)
	return err
}
