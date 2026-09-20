package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"dialer-go/internal/domain"
	"dialer-go/internal/ports"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// CampaignHandler implementa as rotas REST canônicas de gerenciamento de campanhas.
type CampaignHandler struct {
	repo  ports.CampaignRepository
	cache ports.CachePort
}

func NewCampaignHandler(repo ports.CampaignRepository, cache ports.CachePort) *CampaignHandler {
	return &CampaignHandler{
		repo:  repo,
		cache: cache,
	}
}

// List retorna a lista de campanhas de um tenant, com filtro opcional por status.
//
// @pattern Adapter (HTTP Handler / Query)
// @governedBy .agents/ARCHITECT.md
//
// @preExecution
// - Extração de `tenant_id` via header `X-Tenant-Id` ou query parameter `tenant_id`
// - Leitura opcional do query parameter `status`
//
// @postExecution
// - Retorna array serializado em `CampaignListResponse` com HTTP 200
func (h *CampaignHandler) List(w http.ResponseWriter, r *http.Request) {
	tenantID := r.Header.Get("X-Tenant-Id")
	if tenantID == "" {
		tenantID = r.URL.Query().Get("tenant_id")
	}
	if tenantID == "" {
		domain.NewErrBadRequest("MISSING_TENANT_ID", "O parâmetro 'tenant_id' ou header 'X-Tenant-Id' é obrigatório.").WriteJSON(w)
		return
	}

	var statusPtr *domain.CampaignStatus
	if st := r.URL.Query().Get("status"); st != "" {
		cStatus := domain.CampaignStatus(st)
		statusPtr = &cStatus
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	list, err := h.repo.ListByTenant(ctx, tenantID, statusPtr)
	if err != nil {
		domain.NewErrInternal(fmt.Sprintf("Falha ao listar campanhas: %s", err.Error())).WriteJSON(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(domain.CampaignListResponse{
		Success: true,
		Data:    list,
	})
}

// Get recupera uma campanha específica pelo ID e tenant.
//
// @pattern Adapter (HTTP Handler / Query)
// @governedBy .agents/ARCHITECT.md
//
// @preExecution
// - Validação de `id` na rota e `tenant_id` no contexto
//
// @postExecution
// - Retorna `CampaignResponse` com HTTP 200 ou RFC 7807 (404 Not Found)
func (h *CampaignHandler) Get(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "id")
	if campaignID == "" {
		domain.NewErrBadRequest("MISSING_ID", "ID da campanha é obrigatório").WriteJSON(w)
		return
	}

	tenantID := r.Header.Get("X-Tenant-Id")
	if tenantID == "" {
		tenantID = r.URL.Query().Get("tenant_id")
	}
	if tenantID == "" {
		domain.NewErrBadRequest("MISSING_TENANT_ID", "O parâmetro 'tenant_id' ou header 'X-Tenant-Id' é obrigatório.").WriteJSON(w)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	campaign, err := h.repo.GetByID(ctx, tenantID, campaignID)
	if err != nil {
		domain.NewErrInternal(fmt.Sprintf("Falha ao buscar campanha: %s", err.Error())).WriteJSON(w)
		return
	}
	if campaign == nil {
		domain.NewErrNotFound("CAMPAIGN_NOT_FOUND", fmt.Sprintf("Campanha '%s' não encontrada para o tenant '%s'", campaignID, tenantID)).WriteJSON(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(domain.CampaignResponse{
		Success: true,
		Data:    campaign,
	})
}

// Create registra uma nova campanha no discador.
//
// @pattern Adapter (HTTP Handler / Command)
// @governedBy .agents/ARCHITECT.md
//
// @preExecution
// - Parse de JSON `CreateCampaignRequest`
// - Validação de obrigatoriedade de `tenant_id`
// - Geração de UUID determinístico se `id` omitido
//
// @postExecution
// - Persistência no PostgreSQL (`campaigns.Create`)
// - Retorno de `CampaignResponse` com HTTP 201 Created
func (h *CampaignHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req domain.CreateCampaignRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		domain.NewErrBadRequest("INVALID_JSON", "Payload JSON inválido para criação de campanha").WriteJSON(w)
		return
	}

	tenantID := req.TenantID
	if tenantID == "" {
		tenantID = r.Header.Get("X-Tenant-Id")
	}
	if tenantID == "" {
		domain.NewErrBadRequest("MISSING_TENANT_ID", "Campo 'tenant_id' é obrigatório").WriteJSON(w)
		return
	}

	campaignID := req.ID
	if campaignID == "" {
		campaignID = uuid.New().String()
	}

	name := req.Name
	if name == "" {
		name = campaignID
	}

	mode := req.Mode
	if mode == "" {
		mode = domain.CampaignModePredictive
	}

	status := req.Status
	if status == "" {
		status = domain.CampaignStatusActive
	}

	aggressiveness := req.Aggressiveness
	if aggressiveness <= 0 {
		aggressiveness = 1.20
	}

	trunkName := req.TrunkName
	if trunkName == "" {
		trunkName = "auto"
	}

	camp := &domain.Campaign{
		ID:              campaignID,
		TenantID:        tenantID,
		Name:            &name,
		Mode:            mode,
		Status:          status,
		Aggressiveness:  aggressiveness,
		TrunkName:       trunkName,
		CycleCount:      0,
		SaturationLevel: domain.SaturationNova,
		CreatedAt:       time.Now(),
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := h.repo.Create(ctx, camp); err != nil {
		domain.NewErrInternal(fmt.Sprintf("Falha ao criar campanha: %s", err.Error())).WriteJSON(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(domain.CampaignResponse{
		Success: true,
		Data:    camp,
	})
}

// Update atualiza configurações ou parâmetros operacionais de uma campanha existente.
//
// @pattern Adapter (HTTP Handler / Command)
// @governedBy .agents/ARCHITECT.md
//
// @preExecution
// - Obtenção de `id` da rota e `tenant_id`
// - Obtenção da entidade existente via `repo.GetByID`
//
// @postExecution
// - Atualização no PostgreSQL (`campaigns.Update`)
// - Sincronização de cache Redis se houver alteração de status ativo/pausado
// - Retorno de `CampaignResponse` com HTTP 200 OK
func (h *CampaignHandler) Update(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "id")
	if campaignID == "" {
		domain.NewErrBadRequest("MISSING_ID", "ID da campanha é obrigatório").WriteJSON(w)
		return
	}

	var req domain.UpdateCampaignRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		domain.NewErrBadRequest("INVALID_JSON", "Payload JSON inválido para atualização de campanha").WriteJSON(w)
		return
	}

	tenantID := req.TenantID
	if tenantID == "" {
		tenantID = r.Header.Get("X-Tenant-Id")
	}
	if tenantID == "" {
		tenantID = r.URL.Query().Get("tenant_id")
	}
	if tenantID == "" {
		domain.NewErrBadRequest("MISSING_TENANT_ID", "Campo 'tenant_id' ou header 'X-Tenant-Id' é obrigatório").WriteJSON(w)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	existing, err := h.repo.GetByID(ctx, tenantID, campaignID)
	if err != nil {
		domain.NewErrInternal(fmt.Sprintf("Falha ao buscar campanha para atualização: %s", err.Error())).WriteJSON(w)
		return
	}
	if existing == nil {
		domain.NewErrNotFound("CAMPAIGN_NOT_FOUND", fmt.Sprintf("Campanha '%s' não encontrada", campaignID)).WriteJSON(w)
		return
	}

	if req.Name != nil {
		existing.Name = req.Name
	}
	if req.Mode != nil {
		existing.Mode = *req.Mode
	}
	if req.Status != nil {
		existing.Status = *req.Status
		if h.cache != nil {
			isPaused := existing.Status == domain.CampaignStatusPaused
			_ = h.cache.SetCampaignPaused(ctx, existing.ID, isPaused)
		}
	}
	if req.Aggressiveness != nil && *req.Aggressiveness > 0 {
		existing.Aggressiveness = *req.Aggressiveness
	}
	if req.TrunkName != nil && *req.TrunkName != "" {
		existing.TrunkName = *req.TrunkName
	}

	if err := h.repo.Update(ctx, existing); err != nil {
		domain.NewErrInternal(fmt.Sprintf("Falha ao persistir atualização da campanha: %s", err.Error())).WriteJSON(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(domain.CampaignResponse{
		Success: true,
		Data:    existing,
	})
}

// Delete remove uma campanha do sistema.
//
// @pattern Adapter (HTTP Handler / Command)
// @governedBy .agents/ARCHITECT.md
//
// @preExecution
// - Validação de `id` da rota e `tenant_id`
//
// @postExecution
// - Remoção no PostgreSQL (`campaigns.Delete`)
// - Retorna confirmação em JSON com HTTP 200
func (h *CampaignHandler) Delete(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "id")
	if campaignID == "" {
		domain.NewErrBadRequest("MISSING_ID", "ID da campanha é obrigatório").WriteJSON(w)
		return
	}

	tenantID := r.Header.Get("X-Tenant-Id")
	if tenantID == "" {
		tenantID = r.URL.Query().Get("tenant_id")
	}
	if tenantID == "" {
		domain.NewErrBadRequest("MISSING_TENANT_ID", "Campo 'tenant_id' ou header 'X-Tenant-Id' é obrigatório").WriteJSON(w)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	err := h.repo.Delete(ctx, tenantID, campaignID)
	if errors.Is(err, pgx.ErrNoRows) {
		domain.NewErrNotFound("CAMPAIGN_NOT_FOUND", fmt.Sprintf("Campanha '%s' não encontrada para exclusão", campaignID)).WriteJSON(w)
		return
	}
	if err != nil {
		domain.NewErrInternal(fmt.Sprintf("Falha ao excluir campanha: %s", err.Error())).WriteJSON(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"message": "Campanha removida com sucesso",
		"id":      campaignID,
	})
}
