package http

import (
	"encoding/json"
	"net/http"

	"dialer-go/internal/core"
	"dialer-go/internal/domain"
	"dialer-go/internal/ports"
)

// AMDHandler expõe as operações de consulta, ajuste dinâmico e reload de parâmetros AMD no Asterisk PBX.
//
// @pattern Adapter (HTTP Handler)
// @governedBy docs/rules/TELEPHONY_POLICIES.md
type AMDHandler struct {
	mgr *core.AMDConfigManager
	ami ports.AMIPort
}

// NewAMDHandler instancia o controlador HTTP para configurações de AMD.
func NewAMDHandler(mgr *core.AMDConfigManager, ami ports.AMIPort) *AMDHandler {
	return &AMDHandler{
		mgr: mgr,
		ami: ami,
	}
}

// GetConfig retorna os parâmetros ativos em memória do AMD (nativo Asterisk + Vosk STT).
//
// @pattern Adapter (HTTP Handler)
// @governedBy docs/rules/TELEPHONY_POLICIES.md
//
// @preExecution
// - Autorização IP via IPWhitelistMiddleware
//
// @postExecution
// - Retorna JSON RFC 7807 compatível com AMDConfig
func (h *AMDHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	cfg := h.mgr.GetConfig()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    cfg,
	})
}

// UpdateConfig atualiza parâmetros em memória, persiste arquivos e opcionalmente aplica hot-reload no Asterisk.
//
// @pattern Adapter (HTTP Handler)
// @governedBy docs/rules/TELEPHONY_POLICIES.md
//
// @preExecution
// - Validação do JSON de entrada
//
// @postExecution
// - Atualização do AMDConfig em memória
// - Gravação de amd.conf e vosk_amd.json
// - AMI reload se `apply` == true
func (h *AMDHandler) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	var req domain.UpdateAMDConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		domain.NewErrBadRequest("INVALID_JSON", "Payload JSON inválido").WriteJSON(w)
		return
	}

	cfg, applied, err := h.mgr.UpdateConfig(r.Context(), req, h.ami)
	if err != nil {
		domain.NewErrInternal(err.Error()).WriteJSON(w)
		return
	}

	msg := "Configurações atualizadas em memória e disco com sucesso"
	if applied {
		msg += " e módulo app_amd.so recarregado no Asterisk"
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data": domain.AMDConfigResponse{
			Config:          cfg,
			AppliedAsterisk: applied,
			Message:         msg,
		},
	})
}

// Reload força a recarga do módulo app_amd.so no Asterisk PBX.
func (h *AMDHandler) Reload(w http.ResponseWriter, r *http.Request) {
	out, err := h.mgr.ReloadAsterisk(r.Context(), h.ami)
	if err != nil {
		domain.NewErrInternal(err.Error()).WriteJSON(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"message": "app_amd.so recarregado no Asterisk com sucesso",
			"output":  out,
		},
	})
}
