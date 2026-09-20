package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

var bufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 32*1024)
		return &b
	},
}

var handshakeBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 4096)
		return &b
	},
}

// TransparentProxy gerencia a ponte bidirecional de streaming entre o Asterisk e o Classificator Engine.
//
// @pattern Proxy / Adapter
// @governedBy .agents/ARCHITECT.md
type TransparentProxy struct {
	ring        *AutonomousRing
	dialTimeout time.Duration
}

// NewTransparentProxy cria o proxy reverso L7 transparente.
func NewTransparentProxy(ring *AutonomousRing, dialTimeout time.Duration) *TransparentProxy {
	if dialTimeout <= 0 {
		dialTimeout = 100 * time.Millisecond
	}
	return &TransparentProxy{
		ring:        ring,
		dialTimeout: dialTimeout,
	}
}

// HandleClient gerencia uma conexão de entrada do Asterisk.
//
// @pattern Pipeline / Fast-Path
// @preExecution Leitura do preâmbulo inicial de conexão.
// @postExecution Drenagem graciosa e liberação de contadores de slot.
func (p *TransparentProxy) HandleClient(clientConn net.Conn) {
	defer clientConn.Close()

	// Lê o preâmbulo inicial HTTP (Handshake do WebSocket) usando sync.Pool (Zero-Alocação)
	initBufPtr := handshakeBufPool.Get().(*[]byte)
	defer handshakeBufPool.Put(initBufPtr)
	initBuf := *initBufPtr

	_ = clientConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	n, err := clientConn.Read(initBuf)
	if err != nil || n == 0 {
		return
	}
	_ = clientConn.SetReadDeadline(time.Time{})

	// 1. FAST-PATH: Tenta conectar diretamente na porta ativa atual sem overhead
	activeIdx, activeAddr := p.ring.GetActiveAddress()
	backendConn, usedIdx, err := p.connectWithFailover(activeIdx, activeAddr)
	if err != nil {
		log.Printf("[ROUTER-ERROR] Falha critica: todas as portas do anel indisponiveis: %v", err)
		// Responde para o Asterisk / Thin Client que o serviço está fora (HTTP 503 com RFC 7807)
		p.sendServiceUnavailable(clientConn)
		return
	}
	defer backendConn.Close()

	p.ring.AcquireConn(usedIdx)
	defer p.ring.ReleaseConn(usedIdx)

	// Encaminha o preâmbulo recebido para o backend
	if _, err := backendConn.Write(initBuf[:n]); err != nil {
		log.Printf("[ROUTER-ERROR] Falha ao despachar handshake para %s: %v", activeAddr, err)
		return
	}

	// 2. PIPE BIDIRECIONAL DUPLEX (Zero Copy Buffer Pooling)
	var wg sync.WaitGroup
	wg.Add(2)

	// Cliente (Asterisk) -> Backend (Classificator)
	go func() {
		defer wg.Done()
		defer backendConn.Close()
		p.splice(backendConn, clientConn)
	}()

	// Backend (Classificator) -> Cliente (Asterisk)
	go func() {
		defer wg.Done()
		defer clientConn.Close()
		p.splice(clientConn, backendConn)
	}()

	wg.Wait()
}

// connectWithFailover tenta a porta ativa; em caso de recusa ou timeout, aciona o hunting autônomo.
func (p *TransparentProxy) connectWithFailover(initialIdx int, initialAddr string) (net.Conn, int, error) {
	conn, err := net.DialTimeout("tcp", initialAddr, p.dialTimeout)
	if err == nil {
		// Sucesso no Fast-Path!
		return conn, initialIdx, nil
	}

	// Falha na porta ativa: aciona hunting autônomo nas outras portas do anel
	log.Printf("[ROUTER-HUNT] Porta ativa %s (idx %d) falhou: %v. Acionando hunting autonomo...", initialAddr, initialIdx, err)
	newIdx, newAddr, huntErr := p.ring.RotateAndHunt(initialIdx)
	if huntErr != nil {
		return nil, initialIdx, fmt.Errorf("hunting falhou: %w", huntErr)
	}

	log.Printf("[ROUTER-PROMOTED] Nova porta ativa promovida autonomamente: %s (idx %d)", newAddr, newIdx)
	newConn, err := net.DialTimeout("tcp", newAddr, p.dialTimeout*2)
	if err != nil {
		return nil, newIdx, fmt.Errorf("falha ao conectar na nova porta promovida %s: %w", newAddr, err)
	}

	return newConn, newIdx, nil
}

// splice copia dados entre duas conexões utilizando buffers reaproveitáveis do sync.Pool.
func (p *TransparentProxy) splice(dst io.Writer, src io.Reader) {
	bufPtr := bufPool.Get().(*[]byte)
	defer bufPool.Put(bufPtr)

	_, _ = io.CopyBuffer(dst, src, *bufPtr)
}

// SendHTTPResponse envia uma resposta HTTP 503 caso o roteador não encontre nenhum backend.
func (p *TransparentProxy) sendServiceUnavailable(w io.Writer) {
	resp := "HTTP/1.1 503 Service Unavailable\r\n" +
		"Content-Type: application/problem+json\r\n" +
		"Connection: close\r\n\r\n" +
		`{"type":"https://dialer.creditobr.org/errors/no-classifier-available","title":"No Active Classifier","status":503,"detail":"Todas as 3 portas do anel classificator falharam"}`
	_, _ = io.WriteString(w, resp)
}

// Helper para validação
func isWebSocketUpgrade(data []byte) bool {
	return bytes.Contains(bytes.ToLower(data), []byte("upgrade: websocket"))
}
