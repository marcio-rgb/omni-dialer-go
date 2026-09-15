package http

import (
	"encoding/json"
	"net/http"
	"strings"

	"dialer-go/internal/core"
	"dialer-go/internal/domain"
	"dialer-go/internal/ports"
	"github.com/go-chi/chi/v5"
)

// SIPConfigHandler gerencia os endpoints da API REST para gerenciamento dos arquivos de configuração do Asterisk via sip_data.
//
// @pattern Adapter (HTTP Handler)
// @governedBy /docs/rules/TELEPHONY_POLICIES.md
type SIPConfigHandler struct {
	mgr *core.SIPConfigManager
	ami ports.AMIPort
}

func NewSIPConfigHandler(mgr *core.SIPConfigManager, ami ports.AMIPort) *SIPConfigHandler {
	return &SIPConfigHandler{
		mgr: mgr,
		ami: ami,
	}
}

// List retorna a lista de todos os arquivos de configuração armazenados na tabela sip_data.
func (h *SIPConfigHandler) List(w http.ResponseWriter, r *http.Request) {
	files, err := h.mgr.ListFiles(r.Context())
	if err != nil {
		domain.NewErrInternal("Falha ao listar arquivos de configuração em sip_data: " + err.Error()).WriteJSON(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    files,
		"total":   len(files),
	})
}

// GetDomains retorna apenas a lista de nomes dos arquivos/domínios de configuração disponíveis.
func (h *SIPConfigHandler) GetDomains(w http.ResponseWriter, r *http.Request) {
	files, err := h.mgr.ListFiles(r.Context())
	if err != nil {
		domain.NewErrInternal("Falha ao listar domínios de configuração em sip_data: " + err.Error()).WriteJSON(w)
		return
	}

	domains := make([]string, 0, len(files))
	for _, f := range files {
		domains = append(domains, f.File)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"domains": domains,
		"total":   len(domains),
	})
}

// Get obtém o conteúdo e metadados de um arquivo específico na tabela sip_data.
func (h *SIPConfigHandler) Get(w http.ResponseWriter, r *http.Request) {
	filename := chi.URLParam(r, "file")
	if filename == "" {
		domain.NewErrBadRequest("MISSING_FILE_PARAM", "O parâmetro 'file' na URL é obrigatório.").WriteJSON(w)
		return
	}

	file, err := h.mgr.GetFile(r.Context(), filename)
	if err != nil {
		domain.NewErrInternal("Erro ao buscar arquivo em sip_data: " + err.Error()).WriteJSON(w)
		return
	}
	if file == nil {
		domain.NewErrNotFound("FILE_NOT_FOUND", "O arquivo '"+filename+"' não foi encontrado na tabela sip_data.").WriteJSON(w)
		return
	}

	if r.URL.Query().Get("raw") == "true" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(file.Data))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    file,
	})
}

// Save cria ou atualiza a estrutura de um arquivo na tabela sip_data e opcionalmente aplica/recarrega.
func (h *SIPConfigHandler) Save(w http.ResponseWriter, r *http.Request) {
	var req domain.SaveSIPConfigFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		domain.NewErrBadRequest("INVALID_JSON", "Payload JSON inválido: "+err.Error()).WriteJSON(w)
		return
	}

	// Permite especificar o nome do arquivo pela URL ou pelo Body
	urlFile := chi.URLParam(r, "file")
	if urlFile != "" {
		req.File = urlFile
	}

	if strings.TrimSpace(req.File) == "" {
		domain.NewErrBadRequest("INVALID_FILENAME", "O nome do arquivo de configuração (file) é obrigatório.").WriteJSON(w)
		return
	}

	cfgFile := &domain.SIPConfigFile{
		File: req.File,
		Data: req.Data,
	}

	if err := h.mgr.SaveFile(r.Context(), cfgFile); err != nil {
		domain.NewErrInternal("Falha ao salvar arquivo em sip_data: " + err.Error()).WriteJSON(w)
		return
	}

	// Verifica se a aplicação imediata foi solicitada
	shouldApply := req.Apply || r.URL.Query().Get("apply") == "true"
	var applyResp *domain.SIPConfigApplyResponse

	if shouldApply {
		var err error
		applyResp, err = h.mgr.ApplyConfigs(r.Context(), []string{req.File}, h.ami)
		if err != nil {
			domain.NewErrInternal("Arquivo salvo no banco, mas falhou ao aplicar no Asterisk: " + err.Error()).WriteJSON(w)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Arquivo de configuração salvo com sucesso na tabela sip_data.",
		"file":    req.File,
		"applied": shouldApply,
		"result":  applyResp,
	})
}

// Apply grava os arquivos fisicamente da tabela sip_data no disco e executa o reload AMI no Asterisk.
func (h *SIPConfigHandler) Apply(w http.ResponseWriter, r *http.Request) {
	var req domain.ApplySIPConfigRequest
	if r.Body != nil && r.ContentLength > 0 {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	resp, err := h.mgr.ApplyConfigs(r.Context(), req.Files, h.ami)
	if err != nil {
		domain.NewErrInternal("Falha ao aplicar configurações no Asterisk: " + err.Error()).WriteJSON(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": resp.Success,
		"data":    resp,
	})
}

// Delete remove um arquivo da tabela sip_data.
func (h *SIPConfigHandler) Delete(w http.ResponseWriter, r *http.Request) {
	filename := chi.URLParam(r, "file")
	if filename == "" {
		domain.NewErrBadRequest("MISSING_FILE_PARAM", "O parâmetro 'file' na URL é obrigatório.").WriteJSON(w)
		return
	}

	if err := h.mgr.DeleteFile(r.Context(), filename); err != nil {
		domain.NewErrInternal("Falha ao remover arquivo de sip_data: " + err.Error()).WriteJSON(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Arquivo '" + filename + "' removido da tabela sip_data.",
	})
}
