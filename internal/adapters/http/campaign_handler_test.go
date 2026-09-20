package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"dialer-go/internal/domain"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type mockCampaignRepoForHandler struct {
	campaigns map[string]*domain.Campaign
}

func (m *mockCampaignRepoForHandler) GetByID(ctx context.Context, tenantID, campaignID string) (*domain.Campaign, error) {
	c, ok := m.campaigns[campaignID]
	if !ok || c.TenantID != tenantID {
		return nil, nil
	}
	return c, nil
}

func (m *mockCampaignRepoForHandler) ListActive(ctx context.Context, tenantID string) ([]*domain.Campaign, error) {
	var list []*domain.Campaign
	for _, c := range m.campaigns {
		if c.TenantID == tenantID && c.Status == domain.CampaignStatusActive {
			list = append(list, c)
		}
	}
	return list, nil
}

func (m *mockCampaignRepoForHandler) ListByTenant(ctx context.Context, tenantID string, status *domain.CampaignStatus) ([]*domain.Campaign, error) {
	var list []*domain.Campaign
	for _, c := range m.campaigns {
		if c.TenantID == tenantID {
			if status != nil && c.Status != *status {
				continue
			}
			list = append(list, c)
		}
	}
	return list, nil
}

func (m *mockCampaignRepoForHandler) Create(ctx context.Context, c *domain.Campaign) error {
	m.campaigns[c.ID] = c
	return nil
}

func (m *mockCampaignRepoForHandler) Update(ctx context.Context, c *domain.Campaign) error {
	if _, ok := m.campaigns[c.ID]; !ok {
		return pgx.ErrNoRows
	}
	m.campaigns[c.ID] = c
	return nil
}

func (m *mockCampaignRepoForHandler) Delete(ctx context.Context, tenantID, campaignID string) error {
	c, ok := m.campaigns[campaignID]
	if !ok || c.TenantID != tenantID {
		return pgx.ErrNoRows
	}
	delete(m.campaigns, campaignID)
	return nil
}

func (m *mockCampaignRepoForHandler) SetStatus(ctx context.Context, tenantID, campaignID string, status domain.CampaignStatus) (*domain.Campaign, error) {
	c, ok := m.campaigns[campaignID]
	if !ok {
		c = &domain.Campaign{
			ID:       campaignID,
			TenantID: tenantID,
			Status:   status,
		}
		m.campaigns[campaignID] = c
		return c, nil
	}
	c.Status = status
	return c, nil
}

func (m *mockCampaignRepoForHandler) IncrementCycle(ctx context.Context, campaignID string) error {
	if c, ok := m.campaigns[campaignID]; ok {
		c.CycleCount++
	}
	return nil
}

func TestCampaignHandler_CRUD(t *testing.T) {
	repo := &mockCampaignRepoForHandler{
		campaigns: make(map[string]*domain.Campaign),
	}
	handler := NewCampaignHandler(repo, nil)

	r := chi.NewRouter()
	r.Get("/campaigns", handler.List)
	r.Post("/campaigns", handler.Create)
	r.Get("/campaigns/{id}", handler.Get)
	r.Put("/campaigns/{id}", handler.Update)
	r.Delete("/campaigns/{id}", handler.Delete)

	// 1. Create
	createBody, _ := json.Marshal(domain.CreateCampaignRequest{
		ID:             "camp-teste-1",
		TenantID:       "tenant-1",
		Name:           "Campanha Vendas",
		Mode:           domain.CampaignModePredictive,
		Status:         domain.CampaignStatusActive,
		Aggressiveness: 1.5,
		TrunkName:      "rvx",
	})
	reqCreate := httptest.NewRequest(http.MethodPost, "/campaigns", bytes.NewReader(createBody))
	reqCreate.Header.Set("Content-Type", "application/json")
	wCreate := httptest.NewRecorder()
	r.ServeHTTP(wCreate, reqCreate)

	if wCreate.Code != http.StatusCreated {
		t.Fatalf("esperava status 201 Created, obteve %d: %s", wCreate.Code, wCreate.Body.String())
	}

	var respCreate domain.CampaignResponse
	if err := json.NewDecoder(wCreate.Body).Decode(&respCreate); err != nil {
		t.Fatalf("falha ao decodificar resposta de criação: %v", err)
	}
	if respCreate.Data.ID != "camp-teste-1" || *respCreate.Data.Name != "Campanha Vendas" {
		t.Fatalf("dados da campanha criada incorretos: %+v", respCreate.Data)
	}

	// 2. Get By ID
	reqGet := httptest.NewRequest(http.MethodGet, "/campaigns/camp-teste-1", nil)
	reqGet.Header.Set("X-Tenant-Id", "tenant-1")
	wGet := httptest.NewRecorder()
	r.ServeHTTP(wGet, reqGet)

	if wGet.Code != http.StatusOK {
		t.Fatalf("esperava status 200 OK no Get, obteve %d: %s", wGet.Code, wGet.Body.String())
	}

	// 3. List By Tenant
	reqList := httptest.NewRequest(http.MethodGet, "/campaigns?tenant_id=tenant-1", nil)
	wList := httptest.NewRecorder()
	r.ServeHTTP(wList, reqList)

	if wList.Code != http.StatusOK {
		t.Fatalf("esperava status 200 OK na List, obteve %d: %s", wList.Code, wList.Body.String())
	}
	var respList domain.CampaignListResponse
	if err := json.NewDecoder(wList.Body).Decode(&respList); err != nil {
		t.Fatalf("falha ao decodificar lista: %v", err)
	}
	if len(respList.Data) != 1 {
		t.Fatalf("esperava 1 campanha na lista, obteve %d", len(respList.Data))
	}

	// 4. Update
	newName := "Campanha Vendas Atualizada"
	newAgg := 1.1
	updateBody, _ := json.Marshal(domain.UpdateCampaignRequest{
		TenantID:       "tenant-1",
		Name:           &newName,
		Aggressiveness: &newAgg,
	})
	reqUpdate := httptest.NewRequest(http.MethodPut, "/campaigns/camp-teste-1", bytes.NewReader(updateBody))
	reqUpdate.Header.Set("Content-Type", "application/json")
	wUpdate := httptest.NewRecorder()
	r.ServeHTTP(wUpdate, reqUpdate)

	if wUpdate.Code != http.StatusOK {
		t.Fatalf("esperava status 200 OK no Update, obteve %d: %s", wUpdate.Code, wUpdate.Body.String())
	}
	if *repo.campaigns["camp-teste-1"].Name != newName || repo.campaigns["camp-teste-1"].Aggressiveness != 1.1 {
		t.Fatalf("atualização não persistiu corretamente: %+v", repo.campaigns["camp-teste-1"])
	}

	// 5. Delete
	reqDelete := httptest.NewRequest(http.MethodDelete, "/campaigns/camp-teste-1", nil)
	reqDelete.Header.Set("X-Tenant-Id", "tenant-1")
	wDelete := httptest.NewRecorder()
	r.ServeHTTP(wDelete, reqDelete)

	if wDelete.Code != http.StatusOK {
		t.Fatalf("esperava status 200 OK no Delete, obteve %d: %s", wDelete.Code, wDelete.Body.String())
	}
	if _, exists := repo.campaigns["camp-teste-1"]; exists {
		t.Fatalf("campanha deveria ter sido excluída do repositório")
	}
}
