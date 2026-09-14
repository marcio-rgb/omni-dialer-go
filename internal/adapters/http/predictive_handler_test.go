package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"dialer-go/internal/core"
)

func TestPredictiveHandler_PacingEndpoints(t *testing.T) {
	pe := core.NewPredictiveEngine(nil, nil, nil, nil, nil, nil)
	handler := NewPredictiveHandler(pe)

	// 1. GET /pacing inicial (default = 2)
	reqGet := httptest.NewRequest(http.MethodGet, "/api/v1/predictive/pacing", nil)
	recGet := httptest.NewRecorder()
	handler.GetPacing(recGet, reqGet)

	if recGet.Code != http.StatusOK {
		t.Fatalf("GET /pacing status incorreto: %d", recGet.Code)
	}

	var resGet map[string]interface{}
	if err := json.Unmarshal(recGet.Body.Bytes(), &resGet); err != nil {
		t.Fatalf("falha ao decodificar JSON GET: %v", err)
	}
	dataGet := resGet["data"].(map[string]interface{})
	if int(dataGet["min_channels_per_agent"].(float64)) != 2 {
		t.Fatalf("esperava min_channels_per_agent = 2, obteve %v", dataGet["min_channels_per_agent"])
	}

	// 2. POST /pacing com valor válido (ex: 5)
	bodyValid := bytes.NewBufferString(`{"min_channels_per_agent": 5}`)
	reqPost := httptest.NewRequest(http.MethodPost, "/api/v1/predictive/pacing", bodyValid)
	recPost := httptest.NewRecorder()
	handler.SetPacing(recPost, reqPost)

	if recPost.Code != http.StatusOK {
		t.Fatalf("POST /pacing status incorreto: %d, corpo: %s", recPost.Code, recPost.Body.String())
	}

	// 3. GET /pacing para confirmar que mudou para 5
	recGet2 := httptest.NewRecorder()
	handler.GetPacing(recGet2, reqGet)
	var resGet2 map[string]interface{}
	_ = json.Unmarshal(recGet2.Body.Bytes(), &resGet2)
	dataGet2 := resGet2["data"].(map[string]interface{})
	if int(dataGet2["min_channels_per_agent"].(float64)) != 5 {
		t.Fatalf("esperava min_channels_per_agent = 5 após atualização, obteve %v", dataGet2["min_channels_per_agent"])
	}

	// 4. POST /pacing com valor inválido (< 1 ou > 50)
	bodyInvalid := bytes.NewBufferString(`{"min_channels_per_agent": 0}`)
	reqInvalid := httptest.NewRequest(http.MethodPost, "/api/v1/predictive/pacing", bodyInvalid)
	recInvalid := httptest.NewRecorder()
	handler.SetPacing(recInvalid, reqInvalid)

	if recInvalid.Code != http.StatusBadRequest {
		t.Fatalf("esperava 400 Bad Request para ratio 0, obteve %d", recInvalid.Code)
	}
}
