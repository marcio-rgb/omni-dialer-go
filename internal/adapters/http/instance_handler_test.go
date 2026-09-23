package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"dialer-go/internal/core"
	"dialer-go/internal/domain"
	"dialer-go/internal/ports"
	"github.com/go-chi/chi/v5"
)

type mockInstanceRepo struct {
	instances map[string]*domain.Instance
}

var _ ports.InstanceRepository = (*mockInstanceRepo)(nil)

func newMockInstanceRepo() *mockInstanceRepo {
	return &mockInstanceRepo{
		instances: make(map[string]*domain.Instance),
	}
}

func (m *mockInstanceRepo) Create(ctx context.Context, inst *domain.Instance) error {
	m.instances[inst.ID] = inst
	return nil
}

func (m *mockInstanceRepo) GetByID(ctx context.Context, tenantID, id string) (*domain.Instance, error) {
	inst, ok := m.instances[id]
	if !ok || inst.TenantID != tenantID {
		return nil, nil
	}
	return inst, nil
}

func (m *mockInstanceRepo) ListByTenant(ctx context.Context, tenantID string, mode string) ([]*domain.Instance, error) {
	var list []*domain.Instance
	for _, inst := range m.instances {
		if inst.TenantID == tenantID {
			if mode != "" && string(inst.Mode) != mode {
				continue
			}
			list = append(list, inst)
		}
	}
	return list, nil
}

func (m *mockInstanceRepo) Update(ctx context.Context, inst *domain.Instance) error {
	if _, ok := m.instances[inst.ID]; !ok {
		return fmt.Errorf("instancia '%s' nao encontrada", inst.ID)
	}
	m.instances[inst.ID] = inst
	return nil
}

func (m *mockInstanceRepo) Delete(ctx context.Context, tenantID, id string) error {
	if _, ok := m.instances[id]; !ok {
		return fmt.Errorf("instancia '%s' nao encontrada", id)
	}
	delete(m.instances, id)
	return nil
}

func setupInstanceRouter(repo ports.InstanceRepository) chi.Router {
	service := core.NewInstanceService(repo)
	handler := NewInstanceHandler(service)

	r := chi.NewRouter()
	r.Route("/api/v1/instances", func(sub chi.Router) {
		sub.Get("/", handler.List)
		sub.Post("/", handler.Create)
		sub.Get("/{id}", handler.Get)
		sub.Put("/{id}", handler.Update)
		sub.Delete("/{id}", handler.Delete)
		sub.Post("/{id}/ping", handler.Ping)
	})
	return r
}

func TestInstanceHandler_CRUD(t *testing.T) {
	repo := newMockInstanceRepo()
	router := setupInstanceRouter(repo)

	// 1. Create a Dialer instance
	createPayload := domain.CreateInstanceDTO{
		ID:          "vps-dialer-1",
		TenantID:    "default",
		Name:        "VPS Principal",
		Mode:        domain.ModeDialer,
		HostURL:     "http://localhost:8081",
		MaxChannels: 30,
	}
	body, _ := json.Marshal(createPayload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/instances", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-Id", "default")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("Esperado 201 Created, obteve %d: %s", rec.Code, rec.Body.String())
	}

	// 2. Create a Dispatcher instance
	createDispatcher := domain.CreateInstanceDTO{
		ID:          "escritorio-dispatcher-1",
		TenantID:    "default",
		Name:        "Escritorio Vivo",
		Mode:        domain.ModeDispatcher,
		HostURL:     "http://100.123.144.122:8081",
		MaxChannels: 11,
	}
	bodyDisp, _ := json.Marshal(createDispatcher)

	reqDisp := httptest.NewRequest(http.MethodPost, "/api/v1/instances", bytes.NewReader(bodyDisp))
	reqDisp.Header.Set("Content-Type", "application/json")
	reqDisp.Header.Set("X-Tenant-Id", "default")
	recDisp := httptest.NewRecorder()

	router.ServeHTTP(recDisp, reqDisp)
	if recDisp.Code != http.StatusCreated {
		t.Fatalf("Esperado 201 Created para dispatcher, obteve %d: %s", recDisp.Code, recDisp.Body.String())
	}

	// 3. List all instances
	reqList := httptest.NewRequest(http.MethodGet, "/api/v1/instances", nil)
	reqList.Header.Set("X-Tenant-Id", "default")
	recList := httptest.NewRecorder()

	router.ServeHTTP(recList, reqList)
	if recList.Code != http.StatusOK {
		t.Fatalf("Esperado 200 OK na listagem, obteve %d", recList.Code)
	}

	var listResp struct {
		Instances []*domain.Instance `json:"instances"`
		Total     int                `json:"total"`
	}
	_ = json.Unmarshal(recList.Body.Bytes(), &listResp)
	if listResp.Total != 2 {
		t.Errorf("Esperado 2 instancias, obteve %d", listResp.Total)
	}

	// 4. Filter by mode=dispatcher
	reqFilter := httptest.NewRequest(http.MethodGet, "/api/v1/instances?mode=dispatcher", nil)
	reqFilter.Header.Set("X-Tenant-Id", "default")
	recFilter := httptest.NewRecorder()

	router.ServeHTTP(recFilter, reqFilter)
	var filterResp struct {
		Instances []*domain.Instance `json:"instances"`
		Total     int                `json:"total"`
	}
	_ = json.Unmarshal(recFilter.Body.Bytes(), &filterResp)
	if filterResp.Total != 1 || filterResp.Instances[0].Mode != domain.ModeDispatcher {
		t.Errorf("Filtro por dispatcher falhou, obteve: %+v", filterResp)
	}

	// 5. Update instance
	newName := "VPS Central Atualizada"
	newMax := 60
	updateDTO := domain.UpdateInstanceDTO{
		Name:        &newName,
		MaxChannels: &newMax,
	}
	bodyUpdate, _ := json.Marshal(updateDTO)

	reqUpdate := httptest.NewRequest(http.MethodPut, "/api/v1/instances/vps-dialer-1", bytes.NewReader(bodyUpdate))
	reqUpdate.Header.Set("Content-Type", "application/json")
	reqUpdate.Header.Set("X-Tenant-Id", "default")
	recUpdate := httptest.NewRecorder()

	router.ServeHTTP(recUpdate, reqUpdate)
	if recUpdate.Code != http.StatusOK {
		t.Fatalf("Esperado 200 OK no update, obteve %d: %s", recUpdate.Code, recUpdate.Body.String())
	}

	// Verify update
	reqGet := httptest.NewRequest(http.MethodGet, "/api/v1/instances/vps-dialer-1", nil)
	reqGet.Header.Set("X-Tenant-Id", "default")
	recGet := httptest.NewRecorder()
	router.ServeHTTP(recGet, reqGet)

	var getInst domain.Instance
	_ = json.Unmarshal(recGet.Body.Bytes(), &getInst)
	if getInst.Name != newName || getInst.MaxChannels != newMax {
		t.Errorf("Propriedades nao foram atualizadas: %+v", getInst)
	}

	// 6. Delete instance
	reqDel := httptest.NewRequest(http.MethodDelete, "/api/v1/instances/vps-dialer-1", nil)
	reqDel.Header.Set("X-Tenant-Id", "default")
	recDel := httptest.NewRecorder()

	router.ServeHTTP(recDel, reqDel)
	if recDel.Code != http.StatusOK {
		t.Fatalf("Esperado 200 OK na exclusao, obteve %d", recDel.Code)
	}

	// Verify delete
	reqGetAfterDel := httptest.NewRequest(http.MethodGet, "/api/v1/instances/vps-dialer-1", nil)
	reqGetAfterDel.Header.Set("X-Tenant-Id", "default")
	recGetAfterDel := httptest.NewRecorder()
	router.ServeHTTP(recGetAfterDel, reqGetAfterDel)
	if recGetAfterDel.Code != http.StatusNotFound {
		t.Fatalf("Esperado 404 Not Found apos exclusao, obteve %d", recGetAfterDel.Code)
	}
}

func TestInstanceHandler_Ping(t *testing.T) {
	// Mock a remote health server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"status": "ok",
			"telephony_capacity": {
				"active_global_channels": 4,
				"max_global_channels": 11
			}
		}`))
	}))
	defer ts.Close()

	repo := newMockInstanceRepo()
	_ = repo.Create(context.Background(), &domain.Instance{
		ID:          "mock-node",
		TenantID:    "default",
		Name:        "Mock Node",
		Mode:        domain.ModeDispatcher,
		HostURL:     ts.URL,
		MaxChannels: 11,
		IsActive:    true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	})

	router := setupInstanceRouter(repo)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/instances/mock-node/ping", nil)
	req.Header.Set("X-Tenant-Id", "default")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Esperado 200 OK no ping, obteve %d: %s", rec.Code, rec.Body.String())
	}

	var pingResult domain.InstancePingResult
	_ = json.Unmarshal(rec.Body.Bytes(), &pingResult)
	if pingResult.Status != "online" {
		t.Errorf("Esperado status online, obteve '%s'", pingResult.Status)
	}
	if pingResult.ActiveChannels != 4 || pingResult.MaxChannels != 11 {
		t.Errorf("Canais incorretos no ping: active=%d, max=%d", pingResult.ActiveChannels, pingResult.MaxChannels)
	}
}
