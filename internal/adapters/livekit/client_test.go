package livekit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClient_GenerateToken(t *testing.T) {
	client := NewClient("http://localhost:7880", "testkey", "testsecret")
	token, err := client.generateToken()
	if err != nil {
		t.Fatalf("erro inesperado gerando token: %v", err)
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token jwt invalido, partes esperadas 3, obtidas %d", len(parts))
	}
}

func TestClient_EnsureSIPTrunkAndRule_CreatesWhenMissing(t *testing.T) {
	inboundCreated := false
	ruleCreated := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/twirp/livekit.SIP/ListSIPInboundTrunk":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"items": []interface{}{}})
		case "/twirp/livekit.SIP/CreateSIPInboundTrunk":
			inboundCreated = true
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"sip_trunk_id": "ST_mock123"})
		case "/twirp/livekit.SIP/ListSIPDispatchRule":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"items": []interface{}{}})
		case "/twirp/livekit.SIP/CreateSIPDispatchRule":
			ruleCreated = true
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"sip_dispatch_rule_id": "SDR_mock123"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, "key", "secret")
	err := client.EnsureSIPTrunkAndRule(context.Background())
	if err != nil {
		t.Fatalf("EnsureSIPTrunkAndRule falhou: %v", err)
	}

	if !inboundCreated {
		t.Errorf("esperava que CreateSIPInboundTrunk fosse chamado")
	}
	if !ruleCreated {
		t.Errorf("esperava que CreateSIPDispatchRule fosse chamado")
	}
}

func TestClient_EnsureSIPTrunkAndRule_IdempotentWhenAlreadyPresent(t *testing.T) {
	inboundCreated := false
	ruleCreated := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/twirp/livekit.SIP/ListSIPInboundTrunk":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"items": []map[string]string{{"sip_trunk_id": "ST_existing"}},
			})
		case "/twirp/livekit.SIP/CreateSIPInboundTrunk":
			inboundCreated = true
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"sip_trunk_id": "ST_new"})
		case "/twirp/livekit.SIP/ListSIPDispatchRule":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"items": []map[string]string{{"sip_dispatch_rule_id": "SDR_existing"}},
			})
		case "/twirp/livekit.SIP/CreateSIPDispatchRule":
			ruleCreated = true
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"sip_dispatch_rule_id": "SDR_new"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, "key", "secret")
	err := client.EnsureSIPTrunkAndRule(context.Background())
	if err != nil {
		t.Fatalf("EnsureSIPTrunkAndRule falhou: %v", err)
	}

	if inboundCreated {
		t.Errorf("CreateSIPInboundTrunk nao deveria ter sido chamado quando tronco ja existe")
	}
	if ruleCreated {
		t.Errorf("CreateSIPDispatchRule nao deveria ter sido chamado quando regra ja existe")
	}
}

func TestClient_CheckSIPHealth(t *testing.T) {
	tests := []struct {
		name       string
		trunksResp map[string]interface{}
		rulesResp  map[string]interface{}
		wantOK     bool
	}{
		{
			name:       "Both present",
			trunksResp: map[string]interface{}{"items": []map[string]string{{"sip_trunk_id": "ST_1"}}},
			rulesResp:  map[string]interface{}{"items": []map[string]string{{"sip_dispatch_rule_id": "SDR_1"}}},
			wantOK:     true,
		},
		{
			name:       "Trunks empty",
			trunksResp: map[string]interface{}{"items": []interface{}{}},
			rulesResp:  map[string]interface{}{"items": []map[string]string{{"sip_dispatch_rule_id": "SDR_1"}}},
			wantOK:     false,
		},
		{
			name:       "Rules empty",
			trunksResp: map[string]interface{}{"items": []map[string]string{{"sip_trunk_id": "ST_1"}}},
			rulesResp:  map[string]interface{}{"items": []interface{}{}},
			wantOK:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/twirp/livekit.SIP/ListSIPInboundTrunk" {
					_ = json.NewEncoder(w).Encode(tt.trunksResp)
				} else if r.URL.Path == "/twirp/livekit.SIP/ListSIPDispatchRule" {
					_ = json.NewEncoder(w).Encode(tt.rulesResp)
				}
			}))
			defer server.Close()

			client := NewClient(server.URL, "key", "secret")
			ok, err := client.CheckSIPHealth(context.Background())
			if ok != tt.wantOK {
				t.Errorf("CheckSIPHealth ok=%v, esperava %v (err=%v)", ok, tt.wantOK, err)
			}
		})
	}
}

func TestClient_StartReconciler(t *testing.T) {
	called := make(chan bool, 5)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/twirp/livekit.SIP/ListSIPInboundTrunk" {
			select {
			case called <- true:
			default:
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"items": []map[string]string{{"sip_trunk_id": "ST_reconcile"}}})
		} else if r.URL.Path == "/twirp/livekit.SIP/ListSIPDispatchRule" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"items": []map[string]string{{"sip_dispatch_rule_id": "SDR_reconcile"}}})
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := NewClient(server.URL, "key", "secret")
	client.StartReconciler(ctx, 20*time.Millisecond)

	select {
	case <-called:
		// Sucesso, o reconciler rodou e chamou a API
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timeout: StartReconciler nao disparou no intervalo esperado")
	}
}

