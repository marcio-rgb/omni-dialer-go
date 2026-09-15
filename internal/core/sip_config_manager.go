package core

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"dialer-go/internal/domain"
	"dialer-go/internal/ports"
)

// SIPConfigManager orquestra o gerenciamento de configurações do Asterisk via banco de dados sip_data,
// persistência física no sistema de arquivos e comandos de hot-reload via AMI TCP.
//
// @pattern Configuration Manager / Orchestrator
// @governedBy /docs/rules/TELEPHONY_POLICIES.md
//
// @preExecution
// - Validação da conectividade com banco relacional `sip_data` e permissão de escrita no diretório Asterisk
//
// @postExecution
// - Persistência atômica no PostgreSQL
// - Escrita do arquivo físico em `$ASTERISK_CONF_DIR`
// - Notificação ao Asterisk PBX via AMI `Command: pjsip reload / dialplan reload`
type SIPConfigManager struct {
	repo            ports.SIPConfigRepository
	asteriskConfDir string
}

func NewSIPConfigManager(repo ports.SIPConfigRepository, asteriskConfDir string) *SIPConfigManager {
	if asteriskConfDir == "" {
		if envPath := os.Getenv("ASTERISK_CONF_DIR"); envPath != "" {
			asteriskConfDir = envPath
		} else if _, err := os.Stat("/etc/asterisk"); err == nil {
			asteriskConfDir = "/etc/asterisk"
		} else if _, err := os.Stat("/opt/ominichat/asterisk/conf"); err == nil {
			asteriskConfDir = "/opt/ominichat/asterisk/conf"
		} else {
			asteriskConfDir = "."
		}
	}

	return &SIPConfigManager{
		repo:            repo,
		asteriskConfDir: asteriskConfDir,
	}
}

// GetFile busca as informações de um arquivo específico em sip_data.
func (m *SIPConfigManager) GetFile(ctx context.Context, filename string) (*domain.SIPConfigFile, error) {
	return m.repo.GetFile(ctx, filename)
}

// ListFiles retorna todos os arquivos cadastrados na tabela sip_data.
func (m *SIPConfigManager) ListFiles(ctx context.Context) ([]domain.SIPConfigFile, error) {
	return m.repo.ListFiles(ctx)
}

// SaveFile salva ou atualiza o conteúdo do arquivo na tabela sip_data.
func (m *SIPConfigManager) SaveFile(ctx context.Context, file *domain.SIPConfigFile) error {
	if file == nil || strings.TrimSpace(file.File) == "" {
		return fmt.Errorf("nome de arquivo obrigatorio")
	}
	return m.repo.SaveFile(ctx, file)
}

// DeleteFile remove um arquivo da tabela sip_data.
func (m *SIPConfigManager) DeleteFile(ctx context.Context, filename string) error {
	return m.repo.DeleteFile(ctx, filename)
}

// ApplyConfigs grava fisicamente os arquivos de sip_data no diretório do Asterisk e dispara o reload AMI.
func (m *SIPConfigManager) ApplyConfigs(ctx context.Context, filenames []string, ami ports.AMIPort) (*domain.SIPConfigApplyResponse, error) {
	var filesToApply []domain.SIPConfigFile

	if len(filenames) == 0 {
		// Aplica todos os arquivos do banco
		all, err := m.repo.ListFiles(ctx)
		if err != nil {
			return nil, fmt.Errorf("falha ao listar arquivos de sip_data para aplicacao: %w", err)
		}
		filesToApply = all
	} else {
		for _, name := range filenames {
			f, err := m.repo.GetFile(ctx, name)
			if err != nil {
				return nil, fmt.Errorf("falha ao buscar arquivo %s: %w", name, err)
			}
			if f != nil {
				filesToApply = append(filesToApply, *f)
			}
		}
	}

	if len(filesToApply) == 0 {
		return &domain.SIPConfigApplyResponse{
			AppliedFiles:  []string{},
			ReloadResults: []string{"Nenhum arquivo encontrado para aplicar."},
			TotalFiles:    0,
			Success:       true,
		}, nil
	}

	var appliedFiles []string
	var reloadResults []string
	hasPJSIP := false
	hasDialplan := false
	hasAMD := false
	hasGeneral := false

	// 1. Grava os arquivos fisicamente no disco
	_ = os.MkdirAll(m.asteriskConfDir, 0755)

	for _, cfg := range filesToApply {
		targetPath := filepath.Join(m.asteriskConfDir, cfg.File)
		err := os.WriteFile(targetPath, []byte(cfg.Data), 0644)
		if err != nil {
			msg := fmt.Errorf("falha ao escrever arquivo fisico em %s: %w", targetPath, err)
			log.Printf("[SIP-CONFIG] %v", msg)
			return &domain.SIPConfigApplyResponse{
				AppliedFiles:  appliedFiles,
				ReloadResults: reloadResults,
				TotalFiles:    len(filesToApply),
				Success:       false,
				ErrorMessage:  msg.Error(),
			}, err
		}

		appliedFiles = append(appliedFiles, cfg.File)
		log.Printf("[SIP-CONFIG] Arquivo fisico gravado com sucesso: %s", targetPath)

		lowerName := strings.ToLower(cfg.File)
		if strings.Contains(lowerName, "pjsip") {
			hasPJSIP = true
		} else if strings.Contains(lowerName, "extensions") || strings.Contains(lowerName, "dialplan") {
			hasDialplan = true
		} else if strings.Contains(lowerName, "amd") {
			hasAMD = true
		} else {
			hasGeneral = true
		}
	}

	// 2. Dispara reloads apropriados via AMI TCP Socket
	if ami != nil && ami.IsConnected() {
		if hasPJSIP {
			actionID := fmt.Sprintf("reload-pjsip-%d", time.Now().UnixNano())
			out, err := ami.Command(ctx, actionID, "pjsip reload")
			if err != nil {
				reloadResults = append(reloadResults, fmt.Sprintf("pjsip reload ERRO: %v", err))
			} else {
				reloadResults = append(reloadResults, fmt.Sprintf("pjsip reload SUCESSO: %s", strings.TrimSpace(out)))
			}
		}

		if hasDialplan {
			actionID := fmt.Sprintf("reload-dialplan-%d", time.Now().UnixNano())
			out, err := ami.Command(ctx, actionID, "dialplan reload")
			if err != nil {
				reloadResults = append(reloadResults, fmt.Sprintf("dialplan reload ERRO: %v", err))
			} else {
				reloadResults = append(reloadResults, fmt.Sprintf("dialplan reload SUCESSO: %s", strings.TrimSpace(out)))
			}
		}

		if hasAMD {
			actionID := fmt.Sprintf("reload-amd-%d", time.Now().UnixNano())
			out, err := ami.Command(ctx, actionID, "module reload app_amd.so")
			if err != nil {
				reloadResults = append(reloadResults, fmt.Sprintf("amd reload ERRO: %v", err))
			} else {
				reloadResults = append(reloadResults, fmt.Sprintf("amd reload SUCESSO: %s", strings.TrimSpace(out)))
			}
		}

		if hasGeneral && !hasPJSIP && !hasDialplan && !hasAMD {
			actionID := fmt.Sprintf("reload-core-%d", time.Now().UnixNano())
			out, err := ami.Command(ctx, actionID, "module reload")
			if err != nil {
				reloadResults = append(reloadResults, fmt.Sprintf("module reload ERRO: %v", err))
			} else {
				reloadResults = append(reloadResults, fmt.Sprintf("module reload SUCESSO: %s", strings.TrimSpace(out)))
			}
		}
	} else {
		reloadResults = append(reloadResults, "Aviso: AMI nao conectado. Arquivos gravados em disco, mas reload nao foi enviado ao Asterisk.")
	}

	return &domain.SIPConfigApplyResponse{
		AppliedFiles:  appliedFiles,
		ReloadResults: reloadResults,
		TotalFiles:    len(filesToApply),
		Success:       true,
	}, nil
}

// SeedFromDiskIfEmpty popula a tabela sip_data com os arquivos do disco caso o banco esteja vazio no boot.
func (m *SIPConfigManager) SeedFromDiskIfEmpty(ctx context.Context) {
	existing, err := m.repo.ListFiles(ctx)
	if err == nil && len(existing) > 0 {
		log.Printf("[SIP-CONFIG] Tabela sip_data ja contem %d arquivos cadastrados.", len(existing))
		return
	}

	log.Println("[SIP-CONFIG] Tabela sip_data vazia. Inicializando seeding a partir dos arquivos locais de configuracao...")
	defaultFiles := []string{"pjsip.conf", "extensions.conf", "amd.conf"}

	for _, filename := range defaultFiles {
		// Procura no diretório Asterisk ou local
		pathsToTry := []string{
			filepath.Join(m.asteriskConfDir, filename),
			filename,
			filepath.Join("./", filename),
		}

		var content []byte
		var readErr error
		for _, p := range pathsToTry {
			content, readErr = os.ReadFile(p)
			if readErr == nil && len(content) > 0 {
				log.Printf("[SIP-CONFIG] Seeding de '%s' localizado em: %s (%d bytes)", filename, p, len(content))
				break
			}
		}

		if len(content) > 0 {
			err := m.repo.SaveFile(ctx, &domain.SIPConfigFile{
				File: filename,
				Data: string(content),
			})
			if err != nil {
				log.Printf("[SIP-CONFIG] ERRO ao salvar seeding do arquivo %s em sip_data: %v", filename, err)
			} else {
				log.Printf("[SIP-CONFIG] Seeding do arquivo '%s' salvo em sip_data com sucesso.", filename)
			}
		}
	}
}
