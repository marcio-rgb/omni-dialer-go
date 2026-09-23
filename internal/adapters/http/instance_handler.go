package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"dialer-go/internal/domain"
	"dialer-go/internal/ports"
	"github.com/go-chi/chi/v5"
)

// InstanceHandler expõe os endpoints REST para gerenciamento de instâncias Go (Dialer e Dispatcher).
//
// @pattern Adapter (HTTP Handler)
// @governedBy .agents/ARCHITECT.md
type InstanceHandler struct {
	service ports.InstanceService
}

// NewInstanceHandler cria uma nova instância do handler HTTP de instâncias.
func NewInstanceHandler(service ports.InstanceService) *InstanceHandler {
	return &InstanceHandler{service: service}
}

// List retorna a relação de instâncias registradas com suporte a filtro por modo.
func (h *InstanceHandler) List(w http.ResponseWriter, r *http.Request) {
	tenantID := r.Header.Get("X-Tenant-Id")
	if tenantID == "" {
		tenantID = r.URL.Query().Get("tenant_id")
	}
	if tenantID == "" {
		tenantID = "default"
	}

	mode := r.URL.Query().Get("mode")

	instances, err := h.service.List(r.Context(), tenantID, mode)
	if err != nil {
		domain.NewErrInternal(fmt.Sprintf("Falha ao listar instancias: %v", err)).WriteJSON(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"instances": instances,
		"total":     len(instances),
	})
}

// Get recupera uma única instância pelo identificador.
func (h *InstanceHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		domain.NewErrBadRequest("MISSING_ID", "O parametro 'id' e obrigatorio na URL.").WriteJSON(w)
		return
	}

	tenantID := r.Header.Get("X-Tenant-Id")
	if tenantID == "" {
		tenantID = "default"
	}

	inst, err := h.service.Get(r.Context(), tenantID, id)
	if err != nil {
		domain.NewErrNotFound("INSTANCE_NOT_FOUND", err.Error()).WriteJSON(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(inst)
}

// Create processa a criação de uma nova instância.
func (h *InstanceHandler) Create(w http.ResponseWriter, r *http.Request) {
	tenantID := r.Header.Get("X-Tenant-Id")
	if tenantID == "" {
		tenantID = "default"
	}

	var dto domain.CreateInstanceDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		domain.NewErrBadRequest("INVALID_JSON", "Payload JSON invalido.").WriteJSON(w)
		return
	}

	if dto.TenantID == "" {
		dto.TenantID = tenantID
	}

	inst, err := h.service.Create(r.Context(), tenantID, dto)
	if err != nil {
		domain.NewErrBadRequest("CREATE_INSTANCE_FAILED", err.Error()).WriteJSON(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(inst)
}

// Update altera dados de uma instância existente.
func (h *InstanceHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		domain.NewErrBadRequest("MISSING_ID", "O parametro 'id' e obrigatorio na URL.").WriteJSON(w)
		return
	}

	tenantID := r.Header.Get("X-Tenant-Id")
	if tenantID == "" {
		tenantID = "default"
	}

	var dto domain.UpdateInstanceDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		domain.NewErrBadRequest("INVALID_JSON", "Payload JSON invalido.").WriteJSON(w)
		return
	}

	inst, err := h.service.Update(r.Context(), tenantID, id, dto)
	if err != nil {
		if strings.Contains(err.Error(), "nao encontrada") {
			domain.NewErrNotFound("INSTANCE_NOT_FOUND", err.Error()).WriteJSON(w)
		} else {
			domain.NewErrBadRequest("UPDATE_INSTANCE_FAILED", err.Error()).WriteJSON(w)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(inst)
}

// Delete remove uma instância registrada.
func (h *InstanceHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		domain.NewErrBadRequest("MISSING_ID", "O parametro 'id' e obrigatorio na URL.").WriteJSON(w)
		return
	}

	tenantID := r.Header.Get("X-Tenant-Id")
	if tenantID == "" {
		tenantID = "default"
	}

	if err := h.service.Delete(r.Context(), tenantID, id); err != nil {
		domain.NewErrNotFound("INSTANCE_NOT_FOUND", err.Error()).WriteJSON(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"message": "Instancia removida com sucesso.",
		"id":      id,
	})
}

// Ping dispara um teste imediato de conectividade e capacidade HTTP contra a instância remota.
func (h *InstanceHandler) Ping(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		domain.NewErrBadRequest("MISSING_ID", "O parametro 'id' e obrigatorio na URL.").WriteJSON(w)
		return
	}

	tenantID := r.Header.Get("X-Tenant-Id")
	if tenantID == "" {
		tenantID = "default"
	}

	result, err := h.service.Ping(r.Context(), tenantID, id)
	if err != nil {
		domain.NewErrNotFound("INSTANCE_NOT_FOUND", err.Error()).WriteJSON(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(result)
}
