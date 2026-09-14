package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dialer-go/internal/domain"
	"dialer-go/internal/ports"
)

type mockAMIForReload struct {
	lastCommand string
	connected   bool
}

func (m *mockAMIForReload) Connect(ctx context.Context) error { m.connected = true; return nil }
func (m *mockAMIForReload) Close() error                      { m.connected = false; return nil }
func (m *mockAMIForReload) IsConnected() bool                 { return m.connected }
func (m *mockAMIForReload) Originate(ctx context.Context, actionID, channel, context, exten string, priority int, timeout int, callerID, account string, variables map[string]string) error {
	return nil
}
func (m *mockAMIForReload) Redirect(ctx context.Context, actionID, channel, extraChannel, context, exten string, priority int) error {
	return nil
}
func (m *mockAMIForReload) Hangup(ctx context.Context, actionID, channel string, cause int) error {
	return nil
}
func (m *mockAMIForReload) Command(ctx context.Context, actionID, command string) (string, error) {
	m.lastCommand = command
	return "Module 'app_amd.so' reloaded successfully.", nil
}
func (m *mockAMIForReload) SubscribeEvents() <-chan ports.AMIEvent {
	return make(chan ports.AMIEvent)
}

func TestAMDConfigManager_LifecycleAndReload(t *testing.T) {
	storageDir := t.TempDir()
	asteriskDir := t.TempDir()

	mgr := NewAMDConfigManager(storageDir, asteriskDir)

	// 1. Verifica defaults
	cfg := mgr.GetConfig()
	if cfg.InitialSilenceMs != 2000 {
		t.Errorf("InitialSilenceMs esperado 2000, obteve %d", cfg.InitialSilenceMs)
	}
	if len(cfg.VoicemailPhrases) == 0 {
		t.Errorf("VoicemailPhrases não deveria estar vazio")
	}

	// 2. Atualiza parâmetros com hot-reload ativo
	mockAMI := &mockAMIForReload{connected: true}
	newInitSilence := 2500
	newMaxSilence := 1800
	newPhrases := []string{"caixa postal", "deixe recado", "recado", "assim que possivel"}

	upReq := domain.UpdateAMDConfigRequest{
		InitialSilenceMs: &newInitSilence,
		MaxSilenceMs:     &newMaxSilence,
		VoicemailPhrases: newPhrases,
		Apply:            true,
	}

	updated, applied, err := mgr.UpdateConfig(context.Background(), upReq, mockAMI)
	if err != nil {
		t.Fatalf("UpdateConfig falhou: %v", err)
	}
	if !applied {
		t.Errorf("esperava applied == true com mockAMI conectado")
	}
	if updated.InitialSilenceMs != 2500 {
		t.Errorf("esperava InitialSilenceMs == 2500, obteve %d", updated.InitialSilenceMs)
	}
	if mockAMI.lastCommand != "module reload app_amd.so" {
		t.Errorf("comando AMI inesperado: %s", mockAMI.lastCommand)
	}

	// 3. Verifica se os arquivos foram gravados no disco
	amdConfPath := filepath.Join(storageDir, "amd.conf")
	amdBytes, err := os.ReadFile(amdConfPath)
	if err != nil {
		t.Fatalf("amd.conf não encontrado em storage: %v", err)
	}
	if !strings.Contains(string(amdBytes), "initial_silence = 2500") {
		t.Errorf("amd.conf não contém initial_silence = 2500:\n%s", string(amdBytes))
	}

	voskJsonPath := filepath.Join(storageDir, "vosk_amd.json")
	voskBytes, err := os.ReadFile(voskJsonPath)
	if err != nil {
		t.Fatalf("vosk_amd.json não encontrado em storage: %v", err)
	}
	if !strings.Contains(string(voskBytes), "assim que possivel") {
		t.Errorf("vosk_amd.json não contém 'assim que possivel':\n%s", string(voskBytes))
	}

	// 4. Recria gerenciador para testar persistência após reboot
	mgrRestarted := NewAMDConfigManager(storageDir, asteriskDir)
	restartedCfg := mgrRestarted.GetConfig()
	if restartedCfg.InitialSilenceMs != 2500 {
		t.Errorf("após reboot: esperava InitialSilenceMs == 2500, obteve %d", restartedCfg.InitialSilenceMs)
	}
}
