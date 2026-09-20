package domain

import "time"

// SIPConfigFile representa um registro de arquivo de configuração do Asterisk na tabela sip_data.
type SIPConfigFile struct {
	File      string    `json:"file"`
	Data      string    `json:"data"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SaveSIPConfigFileRequest DTO para criação/atualização de um arquivo de configuração Asterisk.
type SaveSIPConfigFileRequest struct {
	File  string `json:"file"`
	Data  string `json:"data"`
	Apply bool   `json:"apply,omitempty"`
}

// ApplySIPConfigRequest DTO para requisição de aplicação e reload de arquivos.
type ApplySIPConfigRequest struct {
	Files []string `json:"files,omitempty"` // Se vazio, aplica todos os arquivos cadastrados
}

// SIPConfigFileResponse DTO de resposta para operações individuais em arquivos.
type SIPConfigFileResponse struct {
	File      string    `json:"file"`
	Data      string    `json:"data"`
	UpdatedAt time.Time `json:"updated_at"`
	Applied   bool      `json:"applied,omitempty"`
}

// SIPConfigApplyResponse DTO de resposta detalhada após a aplicação e reload no Asterisk.
type SIPConfigApplyResponse struct {
	AppliedFiles   []string `json:"applied_files"`
	ReloadResults  []string `json:"reload_results"`
	TotalFiles     int      `json:"total_files"`
	Success        bool     `json:"success"`
	ErrorMessage   string   `json:"error_message,omitempty"`
}
