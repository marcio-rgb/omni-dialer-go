package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"dialer-go/internal/core"
	"dialer-go/internal/ports"
)

type mockAMI struct {
	connected bool
}

func (m *mockAMI) Connect(ctx context.Context) error { m.connected = true; return nil }
func (m *mockAMI) Close() error                      { m.connected = false; return nil }
func (m *mockAMI) IsConnected() bool                 { return m.connected }
func (m *mockAMI) Originate(ctx context.Context, actionID, channel, context, exten string, priority int, timeout int, callerID, account string, variables map[string]string) error {
	return nil
}
func (m *mockAMI) Redirect(ctx context.Context, actionID, channel, extraChannel, context, exten string, priority int) error {
	return nil
}
func (m *mockAMI) Hangup(ctx context.Context, actionID, channel string, cause int) error {
	return nil
}
func (m *mockAMI) Command(ctx context.Context, actionID, command string) (string, error) {
	return "Output: module reloaded", nil
}
func (m *mockAMI) SetVar(ctx context.Context, actionID, channel, variable, value string) error {
	return nil
}
func (m *mockAMI) SubscribeEvents() <-chan ports.AMIEvent {
	return make(chan ports.AMIEvent)
}

func TestAMDHandler_GetAndUpdateConfig(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := core.NewAMDConfigManager(tmpDir, tmpDir)
	ami := &mockAMI{connected: true}
	handler := NewAMDHandler(mgr, ami)

	// 1. GET /api/v1/amd/config
	reqGet := httptest.NewRequest(http.MethodGet, "/api/v1/amd/config", nil)
	wGet := httptest.NewRecorder()
	handler.GetConfig(wGet, reqGet)

	if wGet.Code != http.StatusOK {
		t.Fatalf("esperava 200 no GET, obteve %d", wGet.Code)
	}

	var getResp struct {
		Success bool `json:"success"`
		Data    struct {
			InitialSilenceMs int `json:"initial_silence_ms"`
		} `json:"data"`
	}
	if err := json.Unmarshal(wGet.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("erro ao decodificar resposta GET: %v", err)
	}
	if getResp.Data.InitialSilenceMs != 2000 {
		t.Errorf("InitialSilenceMs esperado 2000, obteve %d", getResp.Data.InitialSilenceMs)
	}

	// 2. PUT /api/v1/amd/config com apply: true
	payload := []byte(`{
		"initial_silence_ms": 2200,
		"max_silence_ms": 1900,
		"voicemail_phrases": ["caixa postal", "deixe recado", "recado", "assim que possivel"],
		"apply": true
	}`)

	reqPut := httptest.NewRequest(http.MethodPut, "/api/v1/amd/config", bytes.NewReader(payload))
	wPut := httptest.NewRecorder()
	handler.UpdateConfig(wPut, reqPut)

	if wPut.Code != http.StatusOK {
		t.Fatalf("esperava 200 no PUT, obteve %d: %s", wPut.Code, wPut.Body.String())
	}

	// 3. POST /api/v1/amd/reload
	reqReload := httptest.NewRequest(http.MethodPost, "/api/v1/amd/reload", nil)
	wReload := httptest.NewRecorder()
	handler.Reload(wReload, reqReload)

	if wReload.Code != http.StatusOK {
		t.Fatalf("esperava 200 no Reload, obteve %d: %s", wReload.Code, wReload.Body.String())
	}
}
