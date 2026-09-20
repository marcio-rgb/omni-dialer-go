package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"dialer-go/internal/domain"
	"dialer-go/internal/ports"
)

// AMDConfigManager gerencia as configurações de triagem acústica e palavras-chave de caixa postal em memória.
//
// @pattern Strategy / Configuration Manager
// @governedBy docs/rules/TELEPHONY_POLICIES.md
//
// @preExecution
// - Inicialização do cache em memória e leitura do arquivo JSON persistente no boot
//
// @postExecution
// - Gravação do arquivo `amd.conf` e `vosk_amd.json`
// - Notificação ao Asterisk PBX via AMI `Command: module reload app_amd.so`
type AMDConfigManager struct {
	mu           sync.RWMutex
	config       domain.AMDConfig
	storagePath  string
	asteriskPath string
}

// NewAMDConfigManager inicializa o gerenciador e carrega a configuração em memória.
func NewAMDConfigManager(storagePath, asteriskPath string) *AMDConfigManager {
	if storagePath == "" {
		storagePath = "./storage"
	}
	if asteriskPath == "" {
		if envPath := os.Getenv("ASTERISK_CONF_DIR"); envPath != "" {
			asteriskPath = envPath
		} else if _, err := os.Stat("/opt/ominichat/asterisk/conf"); err == nil {
			asteriskPath = "/opt/ominichat/asterisk/conf"
		} else {
			asteriskPath = "/etc/asterisk"
		}
	}

	mgr := &AMDConfigManager{
		storagePath:  storagePath,
		asteriskPath: asteriskPath,
		config:       domain.DefaultAMDConfig(),
	}

	_ = os.MkdirAll(storagePath, 0755)
	mgr.loadFromDisk()
	return mgr
}

// GetConfig retorna uma cópia atômica da configuração ativa em memória.
func (m *AMDConfigManager) GetConfig() domain.AMDConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config
}

// UpdateConfig atualiza os parâmetros em memória, persiste nos arquivos e aplica hot-reload se solicitado.
func (m *AMDConfigManager) UpdateConfig(ctx context.Context, req domain.UpdateAMDConfigRequest, ami ports.AMIPort) (domain.AMDConfig, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if req.InitialSilenceMs != nil {
		m.config.InitialSilenceMs = *req.InitialSilenceMs
	}
	if req.GreetingMs != nil {
		m.config.GreetingMs = *req.GreetingMs
	}
	if req.AfterGreetingSilenceMs != nil {
		m.config.AfterGreetingSilenceMs = *req.AfterGreetingSilenceMs
	}
	if req.TotalAnalysisTimeMs != nil {
		m.config.TotalAnalysisTimeMs = *req.TotalAnalysisTimeMs
	}
	if req.MinWordLengthMs != nil {
		m.config.MinWordLengthMs = *req.MinWordLengthMs
	}
	if req.BetweenWordsSilenceMs != nil {
		m.config.BetweenWordsSilenceMs = *req.BetweenWordsSilenceMs
	}
	if req.MaximumNumberOfWords != nil {
		m.config.MaximumNumberOfWords = *req.MaximumNumberOfWords
	}
	if req.SilenceThreshold != nil {
		m.config.SilenceThreshold = *req.SilenceThreshold
	}
	if req.MaxSilenceMs != nil {
		m.config.MaxSilenceMs = *req.MaxSilenceMs
	}
	if len(req.VoicemailPhrases) > 0 {
		m.config.VoicemailPhrases = req.VoicemailPhrases
	}
	if len(req.HumanGreetings) > 0 {
		m.config.HumanGreetings = req.HumanGreetings
	}
	if req.VoskMaxDurationSec != nil && *req.VoskMaxDurationSec > 0 {
		m.config.VoskMaxDurationSec = *req.VoskMaxDurationSec
	}
	m.config.UpdatedAt = time.Now()

	// 1. Salva estado persistente em JSON
	if err := m.saveToDisk(); err != nil {
		log.Printf("[AMD-CONFIG] Erro ao salvar storage/amd_config.json: %v", err)
	}

	// 2. Exporta amd.conf e vosk_amd.json
	m.exportAsteriskConfigs()

	// 3. Hot-reload no Asterisk via AMI se solicitado
	applied := false
	if req.Apply && ami != nil && ami.IsConnected() {
		actionID := fmt.Sprintf("reload-amd-%d", time.Now().UnixNano())
		out, err := ami.Command(ctx, actionID, "module reload app_amd.so")
		if err != nil {
			log.Printf("[AMD-CONFIG] Falha ao recarregar app_amd.so no Asterisk: %v", err)
		} else {
			log.Printf("[AMD-CONFIG] app_amd.so recarregado com sucesso no Asterisk: %s", out)
			applied = true
		}
	}

	return m.config, applied, nil
}

// ReloadAsterisk força a recarga do módulo app_amd.so no Asterisk PBX.
func (m *AMDConfigManager) ReloadAsterisk(ctx context.Context, ami ports.AMIPort) (string, error) {
	if ami == nil || !ami.IsConnected() {
		return "", fmt.Errorf("ami: central Asterisk nao conectada")
	}
	m.mu.RLock()
	m.exportAsteriskConfigs()
	m.mu.RUnlock()

	actionID := fmt.Sprintf("reload-amd-%d", time.Now().UnixNano())
	return ami.Command(ctx, actionID, "module reload app_amd.so")
}

func (m *AMDConfigManager) loadFromDisk() {
	configFile := filepath.Join(m.storagePath, "amd_config.json")
	data, err := os.ReadFile(configFile)
	if err != nil {
		// Gera defaults iniciais no disco
		_ = m.saveToDisk()
		m.exportAsteriskConfigs()
		return
	}

	var loaded domain.AMDConfig
	if err := json.Unmarshal(data, &loaded); err == nil {
		m.config = loaded
	}
	m.exportAsteriskConfigs()
}

func (m *AMDConfigManager) saveToDisk() error {
	configFile := filepath.Join(m.storagePath, "amd_config.json")
	data, err := json.MarshalIndent(m.config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configFile, data, 0644)
}

func (m *AMDConfigManager) exportAsteriskConfigs() {
	// 1. Gera conteudo INI para amd.conf
	iniContent := fmt.Sprintf(`[general]
initial_silence = %d
greeting = %d
after_greeting_silence = %d
total_analysis_time = %d
min_word_length = %d
between_words_silence = %d
maximum_number_of_words = %d
silence_threshold = %d
`,
		m.config.InitialSilenceMs,
		m.config.GreetingMs,
		m.config.AfterGreetingSilenceMs,
		m.config.TotalAnalysisTimeMs,
		m.config.MinWordLengthMs,
		m.config.BetweenWordsSilenceMs,
		m.config.MaximumNumberOfWords,
		m.config.SilenceThreshold,
	)

	// Grava localmente se existir na raiz do projeto
	if _, err := os.Stat("./amd.conf"); err == nil {
		_ = os.WriteFile("./amd.conf", []byte(iniContent), 0644)
	}
	if m.storagePath != "" {
		_ = os.WriteFile(filepath.Join(m.storagePath, "amd.conf"), []byte(iniContent), 0644)
	}

	// Se o caminho do Asterisk estiver acessivel (volume montado), atualiza
	if _, err := os.Stat(m.asteriskPath); err == nil {
		_ = os.WriteFile(filepath.Join(m.asteriskPath, "amd.conf"), []byte(iniContent), 0644)
	}

	// 2. Gera vosk_amd.json para consumo do vosk-eagi
	voskPayload, _ := json.MarshalIndent(map[string]interface{}{
		"max_silence_ms":        m.config.MaxSilenceMs,
		"vosk_max_duration_sec": m.config.VoskMaxDurationSec,
		"voicemail_phrases":     m.config.VoicemailPhrases,
		"human_greetings":       m.config.HumanGreetings,
	}, "", "  ")

	_ = os.WriteFile(filepath.Join(m.storagePath, "vosk_amd.json"), voskPayload, 0644)
	if _, err := os.Stat(m.asteriskPath); err == nil {
		_ = os.WriteFile(filepath.Join(m.asteriskPath, "vosk_amd.json"), voskPayload, 0644)
	}
}
