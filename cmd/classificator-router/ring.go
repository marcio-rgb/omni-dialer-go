package main

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// PortTarget representa uma porta de execução no anel de portas do Classificator.
type PortTarget struct {
	ID          int          `json:"id"`
	Address     string       `json:"address"`
	ActiveConns atomic.Int64 `json:"active_conns"`
	TotalServed atomic.Int64 `json:"total_served"`
	Failures    atomic.Int64 `json:"failures"`
}

// AutonomousRing gerencia um anel circular de 3 portas com seleção O(1) e hunting automático na falha.
//
// @pattern Circuit Breaker / Autonomous Ring Hunting
// @governedBy .agents/ARCHITECT.md
type AutonomousRing struct {
	targets       [3]*PortTarget
	activeIndex   atomic.Int32
	huntMu        sync.Mutex
	dialTimeout   time.Duration
	failoverCount atomic.Int64
}

// NewAutonomousRing inicializa o anel de 3 portas.
//
// @preExecution Exige exatamente 3 endereços para o anel.
// @postExecution Configura o índice 0 como a porta ativa inicial.
func NewAutonomousRing(addresses [3]string, dialTimeout time.Duration) (*AutonomousRing, error) {
	if dialTimeout <= 0 {
		dialTimeout = 50 * time.Millisecond
	}

	ring := &AutonomousRing{
		dialTimeout: dialTimeout,
	}

	for i := 0; i < 3; i++ {
		if addresses[i] == "" {
			return nil, fmt.Errorf("endereco da porta %d nao pode ser vazio", i+1)
		}
		ring.targets[i] = &PortTarget{
			ID:      i + 1,
			Address: addresses[i],
		}
	}

	ring.activeIndex.Store(0)
	return ring, nil
}

// GetActiveAddress retorna o endereço atualmente ativo via Fast-Path O(1) lock-free.
//
// @pattern Fast-Path Selector
// @preExecution Leitura atômica de activeIndex.
// @postExecution Zero overhead de mutex ou I/O de rede no caminho feliz.
func (r *AutonomousRing) GetActiveAddress() (int, string) {
	idx := int(r.activeIndex.Load())
	target := r.targets[idx]
	return idx, target.Address
}

// RotateAndHunt realiza o hunting automático no anel circular para encontrar a próxima porta ativa.
// Este método é acionado ESTRITAMENTE quando a tentativa de conexão na porta atual falha.
//
// @pattern Autonomous Hunting / Self-Healing Failover
// @preExecution A porta atual falhou na conexão.
// @postExecution Promove atomicamente a próxima porta funcional no anel circular.
func (r *AutonomousRing) RotateAndHunt(failedIdx int) (int, string, error) {
	r.huntMu.Lock()
	defer r.huntMu.Unlock()

	// Checagem de corrida: se outra goroutine já realizou o hunting e mudou o activeIndex,
	// apenas retorna o novo activeIndex sem repetir a varredura.
	currentIdx := int(r.activeIndex.Load())
	if currentIdx != failedIdx {
		return currentIdx, r.targets[currentIdx].Address, nil
	}

	r.targets[failedIdx].Failures.Add(1)

	// Percorre as outras duas portas no anel circular
	for offset := 1; offset < 3; offset++ {
		candidateIdx := (failedIdx + offset) % 3
		candidate := r.targets[candidateIdx]

		if r.probeAddress(candidate.Address) {
			// Encontrou uma porta saudável! Promove atomicamente como a nova ativa
			r.activeIndex.Store(int32(candidateIdx))
			r.failoverCount.Add(1)
			return candidateIdx, candidate.Address, nil
		}
		candidate.Failures.Add(1)
	}

	return failedIdx, r.targets[failedIdx].Address, errors.New("todas as 3 portas do anel classificator estao inacessiveis")
}

// probeAddress testa a conectividade TCP rápida com timeout configurado.
func (r *AutonomousRing) probeAddress(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, r.dialTimeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// AcquireConn incrementa a contagem de conexões ativas na porta especificada.
func (r *AutonomousRing) AcquireConn(targetIdx int) {
	if targetIdx >= 0 && targetIdx < 3 {
		r.targets[targetIdx].ActiveConns.Add(1)
		r.targets[targetIdx].TotalServed.Add(1)
	}
}

// ReleaseConn decrementa a contagem de conexões ativas na porta especificada.
func (r *AutonomousRing) ReleaseConn(targetIdx int) {
	if targetIdx >= 0 && targetIdx < 3 {
		r.targets[targetIdx].ActiveConns.Add(-1)
	}
}

// RingStatusDTO representa o snapshot consolidado de telemetria do anel.
type RingStatusDTO struct {
	ActivePortIndex int           `json:"active_port_index"`
	ActiveAddress   string        `json:"active_address"`
	FailoverCount   int64         `json:"failover_count"`
	Ports           []PortDTO     `json:"ports"`
}

// PortDTO representa o estado de uma porta individual.
type PortDTO struct {
	ID          int    `json:"id"`
	Address     string `json:"address"`
	ActiveConns int64  `json:"active_conns"`
	TotalServed int64  `json:"total_served"`
	Failures    int64  `json:"failures"`
	IsActive    bool   `json:"is_active"`
}

// GetStatus exporta a telemetria do anel para observabilidade.
func (r *AutonomousRing) GetStatus() RingStatusDTO {
	activeIdx := int(r.activeIndex.Load())
	ports := make([]PortDTO, 3)

	for i := 0; i < 3; i++ {
		t := r.targets[i]
		ports[i] = PortDTO{
			ID:          t.ID,
			Address:     t.Address,
			ActiveConns: t.ActiveConns.Load(),
			TotalServed: t.TotalServed.Load(),
			Failures:    t.Failures.Load(),
			IsActive:    i == activeIdx,
		}
	}

	return RingStatusDTO{
		ActivePortIndex: activeIdx,
		ActiveAddress:   r.targets[activeIdx].Address,
		FailoverCount:   r.failoverCount.Load(),
		Ports:           ports,
	}
}

// GetTotalActiveConns retorna a soma de conexões ativas em todas as portas do anel.
func (r *AutonomousRing) GetTotalActiveConns() int64 {
	var total int64
	for i := 0; i < 3; i++ {
		total += r.targets[i].ActiveConns.Load()
	}
	return total
}

