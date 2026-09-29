package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"dialer-go/internal/domain"
	"github.com/go-chi/chi/v5"
)

type mockAgentRepo struct {
	agents      map[string]*domain.TenantAgent
	histories   []*domain.TenantAgentHistory
	performance []*domain.AgentPerformanceItem
}

func newMockAgentRepo() *mockAgentRepo {
	return &mockAgentRepo{
		agents: make(map[string]*domain.TenantAgent),
	}
}

func (m *mockAgentRepo) UpsertAgent(ctx context.Context, agent *domain.TenantAgent) error {
	m.agents[agent.TenantID+":"+agent.AgentID] = agent
	return nil
}

func (m *mockAgentRepo) GetAgent(ctx context.Context, tenantID, agentID string) (*domain.TenantAgent, error) {
	if a, ok := m.agents[tenantID+":"+agentID]; ok {
		return a, nil
	}
	return nil, nil
}

func (m *mockAgentRepo) ListAgents(ctx context.Context, tenantID string, isActive *bool, limit, offset int) ([]*domain.TenantAgent, int64, error) {
	var list []*domain.TenantAgent
	for _, a := range m.agents {
		if a.TenantID == tenantID {
			if isActive == nil || a.IsActive == *isActive {
				list = append(list, a)
			}
		}
	}
	return list, int64(len(list)), nil
}

func (m *mockAgentRepo) DeleteAgent(ctx context.Context, tenantID, agentID string) error {
	delete(m.agents, tenantID+":"+agentID)
	return nil
}

func (m *mockAgentRepo) RecordHistory(ctx context.Context, h *domain.TenantAgentHistory) error {
	m.histories = append(m.histories, h)
	return nil
}

func (m *mockAgentRepo) GetAgentHistory(ctx context.Context, tenantID, agentID string, startDate, endDate time.Time, limit, offset int) ([]*domain.TenantAgentHistory, error) {
	return m.histories, nil
}

func (m *mockAgentRepo) GetAgentPerformance(ctx context.Context, tenantID string, startDate, endDate time.Time, agentID *string) ([]*domain.AgentPerformanceItem, error) {
	if m.performance != nil {
		return m.performance, nil
	}
	return []*domain.AgentPerformanceItem{
		{
			AgentID:                "101",
			AgentName:              "Marcio Silva",
			TotalCalls:             50,
			AnsweredCalls:          30,
			CallsOver40s:           18,
			TotalTalkTimeSeconds:   2400,
			AverageTalkTimeSeconds: 80,
			TotalAvailableSeconds:  14400,
			TotalPausedSeconds:     1800,
		},
	}, nil
}

func TestUpsertAgent(t *testing.T) {
	repo := newMockAgentRepo()
	handler := NewAgentHandler(repo)

	// 1. Sucesso
	body, _ := json.Marshal(domain.CreateAgentRequest{
		TenantID:  "default",
		AgentID:   "101",
		AgentName: "Marcio Silva",
	})
	req := httptest.NewRequest("POST", "/api/v1/agents", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.UpsertAgent(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Esperava status 200, obteve %d. Body: %s", w.Code, w.Body.String())
	}

	// 2. Falha campos obrigatórios
	invalidBody, _ := json.Marshal(domain.CreateAgentRequest{
		AgentID: "101",
	})
	req2 := httptest.NewRequest("POST", "/api/v1/agents", bytes.NewBuffer(invalidBody))
	w2 := httptest.NewRecorder()

	handler.UpsertAgent(w2, req2)
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("Esperava status 400 para campos ausentes, obteve %d", w2.Code)
	}
}

func TestGetAndListAgents(t *testing.T) {
	repo := newMockAgentRepo()
	_ = repo.UpsertAgent(context.Background(), &domain.TenantAgent{
		TenantID:  "default",
		AgentID:   "202",
		AgentName: "Emerson Tech",
		IsActive:  true,
	})
	handler := NewAgentHandler(repo)

	r := chi.NewRouter()
	r.Get("/api/v1/agents/{id}", handler.GetAgent)
	r.Get("/api/v1/agents", handler.ListAgents)

	// Get exist
	req := httptest.NewRequest("GET", "/api/v1/agents/202", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Esperava 200 para agente existente, obteve %d", w.Code)
	}

	// Get not found
	reqNF := httptest.NewRequest("GET", "/api/v1/agents/999", nil)
	wNF := httptest.NewRecorder()
	r.ServeHTTP(wNF, reqNF)
	if wNF.Code != http.StatusNotFound {
		t.Fatalf("Esperava 404 para agente não encontrado, obteve %d", wNF.Code)
	}

	// List
	reqList := httptest.NewRequest("GET", "/api/v1/agents?tenant_id=default", nil)
	wList := httptest.NewRecorder()
	r.ServeHTTP(wList, reqList)
	if wList.Code != http.StatusOK {
		t.Fatalf("Esperava 200 ao listar, obteve %d", wList.Code)
	}
}

func TestAgentPerformanceReport(t *testing.T) {
	repo := newMockAgentRepo()
	handler := NewAgentHandler(repo)

	req := httptest.NewRequest("GET", "/api/v1/reports/agent-performance?start_date=2026-09-28T00:00:00Z&end_date=2026-09-28T23:59:59Z", nil)
	w := httptest.NewRecorder()

	handler.GetAgentPerformance(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Esperava 200 no relatório de performance, obteve %d", w.Code)
	}

	var res struct {
		Success bool                            `json:"success"`
		Data    domain.AgentPerformanceResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("Falha ao deserializar resposta: %v", err)
	}
	if len(res.Data.Agents) != 1 {
		t.Fatalf("Esperava 1 agente no retorno, obteve %d", len(res.Data.Agents))
	}
	ag := res.Data.Agents[0]
	if ag.AgentName != "Marcio Silva" || ag.CallsOver40s != 18 {
		t.Fatalf("Dados inesperados no relatório: %+v", ag)
	}
}
