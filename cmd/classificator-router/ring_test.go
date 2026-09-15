package main

import (
	"net"
	"testing"
	"time"
)

func TestAutonomousRing_FastPathAndFailover(t *testing.T) {
	// Cria 3 listeners locais simulando as 3 portas do anel
	l1, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("falha ao criar listener 1: %v", err)
	}
	defer l1.Close()

	l2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("falha ao criar listener 2: %v", err)
	}
	defer l2.Close()

	l3, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("falha ao criar listener 3: %v", err)
	}
	defer l3.Close()

	addrs := [3]string{
		l1.Addr().String(),
		l2.Addr().String(),
		l3.Addr().String(),
	}

	ring, err := NewAutonomousRing(addrs, 30*time.Millisecond)
	if err != nil {
		t.Fatalf("falha ao criar ring: %v", err)
	}

	// 1. Valida estado inicial (porta 1 deve ser ativa)
	idx, addr := ring.GetActiveAddress()
	if idx != 0 || addr != addrs[0] {
		t.Fatalf("esperado porta 0 (%s), obtido %d (%s)", addrs[0], idx, addr)
	}

	// 2. Derruba a porta 1 (simulando falha de container ou shutdown no meio da operação)
	_ = l1.Close()

	// 3. Executa RotateAndHunt
	newIdx, newAddr, err := ring.RotateAndHunt(0)
	if err != nil {
		t.Fatalf("hunting falhou inesperadamente: %v", err)
	}

	// A porta promovida deve ser a porta 2 (índice 1)
	if newIdx != 1 || newAddr != addrs[1] {
		t.Fatalf("esperava que a porta 2 (%s) fosse promovida, mas obteve porta %d (%s)", addrs[1], newIdx, newAddr)
	}

	// Valida que o activeIndex foi comutado no Fast-Path
	fastIdx, fastAddr := ring.GetActiveAddress()
	if fastIdx != 1 || fastAddr != addrs[1] {
		t.Fatalf("Fast-Path deveria refletir a nova porta ativa, obteve %d (%s)", fastIdx, fastAddr)
	}

	// 4. Derruba a porta 2
	_ = l2.Close()

	// 5. Executa novo hunting
	thirdIdx, thirdAddr, err := ring.RotateAndHunt(1)
	if err != nil {
		t.Fatalf("hunting para terceira porta falhou: %v", err)
	}

	if thirdIdx != 2 || thirdAddr != addrs[2] {
		t.Fatalf("esperava que a porta 3 (%s) fosse promovida, mas obteve porta %d (%s)", addrs[2], thirdIdx, thirdAddr)
	}

	// 6. Valida métricas de telemetria
	status := ring.GetStatus()
	if status.FailoverCount != 2 {
		t.Fatalf("esperado 2 failovers no status, obtido %d", status.FailoverCount)
	}
}

func TestAutonomousRing_ConnsAccounting(t *testing.T) {
	addrs := [3]string{"127.0.0.1:2801", "127.0.0.1:2802", "127.0.0.1:2803"}
	ring, err := NewAutonomousRing(addrs, 20*time.Millisecond)
	if err != nil {
		t.Fatalf("erro ao criar ring: %v", err)
	}

	ring.AcquireConn(0)
	ring.AcquireConn(0)
	ring.AcquireConn(1)

	status := ring.GetStatus()
	if status.Ports[0].ActiveConns != 2 || status.Ports[0].TotalServed != 2 {
		t.Fatalf("contagem incorreta para porta 0: %+v", status.Ports[0])
	}
	if status.Ports[1].ActiveConns != 1 || status.Ports[1].TotalServed != 1 {
		t.Fatalf("contagem incorreta para porta 1: %+v", status.Ports[1])
	}

	ring.ReleaseConn(0)
	status = ring.GetStatus()
	if status.Ports[0].ActiveConns != 1 {
		t.Fatalf("esperado 1 conn ativa na porta 0, obtido %d", status.Ports[0].ActiveConns)
	}

	// Valida GetTotalActiveConns
	if total := ring.GetTotalActiveConns(); total != 2 {
		t.Fatalf("esperado total de 2 conexoes ativas no anel, obtido %d", total)
	}
}

func TestTransparentProxy_503ResponseWhenAllPortsDown(t *testing.T) {
	// Cria anel com portas inexistentes na máquina local
	addrs := [3]string{"127.0.0.1:59991", "127.0.0.1:59992", "127.0.0.1:59993"}
	ring, err := NewAutonomousRing(addrs, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("erro ao criar ring: %v", err)
	}

	proxy := NewTransparentProxy(ring, 10*time.Millisecond)

	// Cria par de conexões em memória via net.Pipe
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()

	done := make(chan string)
	go func() {
		buf := make([]byte, 1024)
		n, _ := clientConn.Read(buf)
		done <- string(buf[:n])
	}()

	// Simula requisição de cliente que falha em todas as portas
	go proxy.HandleClient(serverConn)

	// Simula cliente enviando preâmbulo HTTP
	_, _ = clientConn.Write([]byte("GET / HTTP/1.1\r\nHost: localhost\r\n\r\n"))

	select {
	case resp := <-done:
		if !containsSubstr(resp, "503 Service Unavailable") {
			t.Fatalf("esperado resposta '503 Service Unavailable', obtido:\n%s", resp)
		}
		if !containsSubstr(resp, "application/problem+json") {
			t.Fatalf("esperado header Problem Details RFC 7807, obtido:\n%s", resp)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout aguardando resposta 503 do proxy")
	}
}

func containsSubstr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && len(sub) > 0 && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

