package main

import (
	"encoding/json"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	listenAddr := flag.String("listen", getEnv("LISTEN_ADDR", ":2800"), "Endereco de escuta do router WebSocket")
	adminAddr := flag.String("admin", getEnv("ADMIN_ADDR", ":2809"), "Endereco da API HTTP administrativa/status")
	port1 := flag.String("p1", getEnv("PORT_1", "127.0.0.1:2801"), "Endereco da porta 1 do anel")
	port2 := flag.String("p2", getEnv("PORT_2", "127.0.0.1:2802"), "Endereco da porta 2 do anel")
	port3 := flag.String("p3", getEnv("PORT_3", "127.0.0.1:2803"), "Endereco da porta 3 do anel")
	dialTimeoutMs := flag.Int("dial-timeout-ms", 80, "Timeout de conexao TCP com as portas do anel em ms")
	flag.Parse()

	addresses := [3]string{
		strings.TrimSpace(*port1),
		strings.TrimSpace(*port2),
		strings.TrimSpace(*port3),
	}

	ring, err := NewAutonomousRing(addresses, time.Duration(*dialTimeoutMs)*time.Millisecond)
	if err != nil {
		log.Fatalf("[FATAL] Falha ao inicializar AutonomousRing: %v", err)
	}

	proxy := NewTransparentProxy(ring, time.Duration(*dialTimeoutMs)*time.Millisecond)

	// 1. Inicia o listener TCP principal (:2800)
	listener, err := net.Listen("tcp", *listenAddr)
	if err != nil {
		log.Fatalf("[FATAL] Falha ao escutar na porta %s: %v", *listenAddr, err)
	}
	defer listener.Close()

	log.Printf("==================================================================")
	log.Printf(" [CLASSIFICATOR-ROUTER] Iniciado com sucesso na porta %s", *listenAddr)
	log.Printf(" [ANEL AUTONOMO] P1=%s | P2=%s | P3=%s", addresses[0], addresses[1], addresses[2])
	log.Printf(" [STATUS HTTP] Disponivel em %s/status e %s/health", *adminAddr, *adminAddr)
	log.Printf("==================================================================")

	// 2. Inicia servidor de status HTTP em background
	startAdminServer(*adminAddr, ring)

	// 3. Loop principal de aceitação de conexões
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				if strings.Contains(err.Error(), "use of closed network connection") {
					return
				}
				log.Printf("[ROUTER-WARN] Erro no Accept: %v", err)
				continue
			}

			// Despacha cada chamada para o proxy concorrente
			go proxy.HandleClient(conn)
		}
	}()

	// 4. Aguarda sinal de encerramento do sistema operacional
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("[ROUTER] Sinal de encerramento recebido. Recusando novas conexões...")
	_ = listener.Close()

	log.Println("[ROUTER] Aguardando chamadas ativas terminarem (Graceful Draining)...")
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if ring.GetTotalActiveConns() == 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	log.Printf("[ROUTER] Encerrado com sucesso. Conexões ativas restantes: %d", ring.GetTotalActiveConns())
}

func startAdminServer(addr string, ring *AutonomousRing) {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, activeAddr := ring.GetActiveAddress()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":      "UP",
			"active_port": activeAddr,
			"timestamp":   time.Now().Unix(),
		})
	})

	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		status := ring.GetStatus()
		_ = json.NewEncoder(w).Encode(status)
	})

	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[ADMIN-WARN] Falha no servidor HTTP administrativo: %v", err)
		}
	}()
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
