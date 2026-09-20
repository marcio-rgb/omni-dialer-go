package core

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"dialer-go/internal/domain"
)

func TestChannelManager_HierarchyAndConcurrency(t *testing.T) {
	ctx := context.Background()
	// Teto global = 10, Cota Humana = 3 (Sobram 7 para IA)
	cm := NewChannelManager(10, 3, nil)

	// Registra tronco com limite de 5 canais
	trunkID := "trunk_vivo"
	cm.RegisterTrunkLimit(trunkID, 5)

	// 1. Aloca 5 canais virtuais (IA) no tronco
	for i := 1; i <= 5; i++ {
		ch := &domain.ActiveChannel{
			ChannelID: fmt.Sprintf("chan-%d", i),
			TrunkID:   trunkID,
			CallType:  domain.CallTypePredictive,
			StartedAt: time.Now(),
		}
		if err := cm.AcquireSlot(ctx, ch, false); err != nil {
			t.Fatalf("falha ao alocar slot %d: %v", i, err)
		}
	}

	// 2. Sexta chamada no mesmo tronco deve falhar por limite de tronco (TRUNK_CHANNELS_EXHAUSTED)
	chFail := &domain.ActiveChannel{
		ChannelID: "chan-fail-trunk",
		TrunkID:   trunkID,
		CallType:  domain.CallTypePredictive,
		StartedAt: time.Now(),
	}
	can, reason := cm.CanAcquireSlot(trunkID, false)
	if can || reason != "TRUNK_CHANNELS_EXHAUSTED" {
		t.Fatalf("esperava rejeição por TRUNK_CHANNELS_EXHAUSTED, obtido can=%v, reason=%s", can, reason)
	}
	if err := cm.AcquireSlot(ctx, chFail, false); err == nil {
		t.Fatal("esperava erro ao estourar capacidade do tronco")
	}

	// 3. Registra segundo tronco com 10 canais para testar cota humana e teto global
	trunk2 := "trunk_claro"
	cm.RegisterTrunkLimit(trunk2, 10)

	// Aloca mais 2 canais virtuais (Total global = 7). Atinge o teto para IA (10 - 3 = 7).
	for i := 6; i <= 7; i++ {
		ch := &domain.ActiveChannel{
			ChannelID: fmt.Sprintf("chan-%d", i),
			TrunkID:   trunk2,
			CallType:  domain.CallTypePredictive,
			StartedAt: time.Now(),
		}
		if err := cm.AcquireSlot(ctx, ch, false); err != nil {
			t.Fatalf("falha ao alocar slot IA %d: %v", i, err)
		}
	}

	// O oitavo canal virtual deve ser bloqueado para proteger a cota humana (HUMAN_RESERVED_QUOTA_PROTECTION)
	chFailIA := &domain.ActiveChannel{
		ChannelID: "chan-fail-ia",
		TrunkID:   trunk2,
		CallType:  domain.CallTypePredictive,
		StartedAt: time.Now(),
	}
	canIA, reasonIA := cm.CanAcquireSlot(trunk2, false)
	if canIA || reasonIA != "HUMAN_RESERVED_QUOTA_PROTECTION" {
		t.Fatalf("esperava HUMAN_RESERVED_QUOTA_PROTECTION, obtido %v, %s", canIA, reasonIA)
	}
	if err := cm.AcquireSlot(ctx, chFailIA, false); err == nil {
		t.Fatal("esperava erro ao tentar invadir cota humana reservada")
	}

	// Mas uma chamada humana DEVE passar!
	chHumano := &domain.ActiveChannel{
		ChannelID: "chan-humano-1",
		TrunkID:   trunk2,
		CallType:  domain.CallTypeManual,
		StartedAt: time.Now(),
	}
	canHum, _ := cm.CanAcquireSlot(trunk2, true)
	if !canHum {
		t.Fatal("chamada humana deveria ser permitida na cota reservada")
	}
	if err := cm.AcquireSlot(ctx, chHumano, true); err != nil {
		t.Fatalf("falha ao alocar slot humano: %v", err)
	}

	// Libera um canal e verifica se contadores reduzem atomicamente
	cm.ReleaseSlot(ctx, "chan-1")
	if count := cm.GetTrunkActiveCount(trunkID); count != 4 {
		t.Errorf("esperava 4 canais ativos no tronco, obtido %d", count)
	}
}

func TestChannelManager_RaceCondition(t *testing.T) {
	ctx := context.Background()
	cm := NewChannelManager(100, 10, nil)
	trunkID := "trunk_concurrency"
	cm.RegisterTrunkLimit(trunkID, 50)

	var wg sync.WaitGroup
	// 50 goroutines alocando e desalocando concorrentemente
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ch := &domain.ActiveChannel{
				ChannelID: fmt.Sprintf("concurrent-%d", id),
				TrunkID:   trunkID,
				CallType:  domain.CallTypePredictive,
				StartedAt: time.Now(),
			}
			_ = cm.AcquireSlot(ctx, ch, false)
			time.Sleep(2 * time.Millisecond)
			cm.ReleaseSlot(ctx, ch.ChannelID)
		}(i)
	}

	wg.Wait()
	globalActive, _, _, _ := cm.GetGlobalStats()
	if globalActive != 0 {
		t.Errorf("esperava 0 canais ativos após concorrência, obtido %d", globalActive)
	}
}

func TestChannelManager_AgentAssignmentAndReturnChannel(t *testing.T) {
	ctx := context.Background()
	cm := NewChannelManager(20, 5, nil)
	trunkID := "trunk_test"
	cm.RegisterTrunkLimit(trunkID, 10)

	callID := "pred-call-123"
	ch := &domain.ActiveChannel{
		ChannelID: callID,
		TrunkID:   trunkID,
		TenantID:  "tenant_1",
		Phone:     "11999998888",
		CallType:  domain.CallTypePredictive,
		StartedAt: time.Now(),
	}

	// 1. Aloca como chamada de máquina (isHuman = false)
	if err := cm.AcquireSlot(ctx, ch, false); err != nil {
		t.Fatalf("falha ao alocar slot preditivo: %v", err)
	}

	global, human, _, _ := cm.GetGlobalStats()
	if global != 1 || human != 0 {
		t.Fatalf("esperava global=1 e human=0, obtido global=%d, human=%d", global, human)
	}

	// 2. Associa canal Asterisk
	cm.LinkAsteriskChannel(callID, "PJSIP/trunk_test-00000001", "1700000000.1")
	foundID := cm.GetCallIDByAsterisk("PJSIP/trunk_test-00000001", "1700000000.1")
	if foundID != callID {
		t.Fatalf("esperava callID=%s, obtido=%s", callID, foundID)
	}

	// 3. Cliente atendeu, promove para operador humano
	cm.AssignAgent(callID, "agent-007")
	global, human, _, _ = cm.GetGlobalStats()
	if global != 1 || human != 1 {
		t.Fatalf("esperava global=1 e human=1 após AssignAgent, obtido global=%d, human=%d", global, human)
	}

	// 4. Define disposição explícita
	cm.SetCallDisposition(callID, domain.DispositionDelivered)

	// 5. Desaloca via evento de Asterisk e verifica canal retornado
	released := cm.ReleaseByAsterisk(ctx, "PJSIP/trunk_test-00000001", "1700000000.1")
	if released == nil {
		t.Fatal("esperava canal retornado ao desalocar por Asterisk")
	}
	if released.ChannelID != callID {
		t.Fatalf("esperava canal retornado %s, obtido %s", callID, released.ChannelID)
	}
	if released.AgentID == nil || *released.AgentID != "agent-007" {
		t.Fatal("esperava AgentID preservado no canal retornado")
	}
	if released.Disposition == nil || *released.Disposition != domain.DispositionDelivered {
		t.Fatal("esperava Disposition preservada no canal retornado")
	}

	// 6. Confirma que contadores zeraram sem vazamento
	global, human, _, _ = cm.GetGlobalStats()
	if global != 0 || human != 0 {
		t.Fatalf("esperava contadores zerados, obtido global=%d, human=%d", global, human)
	}
}
