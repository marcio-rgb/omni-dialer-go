package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIPWhitelistMiddleware(t *testing.T) {
	mw := NewIPWhitelistMiddleware([]string{"127.0.0.1", "10.0.0.1"})

	handler := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))

	// 1. IP Autorizado
	reqAllowed := httptest.NewRequest(http.MethodGet, "/api/v1/trunks", nil)
	reqAllowed.RemoteAddr = "127.0.0.1:45321"
	recAllowed := httptest.NewRecorder()
	handler.ServeHTTP(recAllowed, reqAllowed)

	if recAllowed.Code != http.StatusOK {
		t.Errorf("esperava status 200 para IP autorizado, obtido %d", recAllowed.Code)
	}

	// 2. IP Não Autorizado -> 403 Forbidden
	reqForbidden := httptest.NewRequest(http.MethodGet, "/api/v1/trunks", nil)
	reqForbidden.RemoteAddr = "192.168.1.99:54321"
	recForbidden := httptest.NewRecorder()
	handler.ServeHTTP(recForbidden, reqForbidden)

	if recForbidden.Code != http.StatusForbidden {
		t.Errorf("esperava status 403 para IP não autorizado, obtido %d", recForbidden.Code)
	}

	// 3. Health check deve passar sem bloqueio
	reqHealth := httptest.NewRequest(http.MethodGet, "/health", nil)
	reqHealth.RemoteAddr = "192.168.1.99:54321"
	recHealth := httptest.NewRecorder()
	handler.ServeHTTP(recHealth, reqHealth)

	if recHealth.Code != http.StatusOK {
		t.Errorf("esperava status 200 no /health mesmo para IP não listado, obtido %d", recHealth.Code)
	}

	// 4. CIDR Autorizado
	mwCIDR := NewIPWhitelistMiddleware([]string{"10.0.0.0/8", "172.16.0.0/12"})
	handlerCIDR := mwCIDR.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	reqCIDR := httptest.NewRequest(http.MethodGet, "/api/v1/trunks", nil)
	reqCIDR.RemoteAddr = "10.0.4.15:12345"
	recCIDR := httptest.NewRecorder()
	handlerCIDR.ServeHTTP(recCIDR, reqCIDR)
	if recCIDR.Code != http.StatusOK {
		t.Errorf("esperava status 200 para IP em faixa CIDR 10.0.0.0/8, obtido %d", recCIDR.Code)
	}

	// 5. Wildcard * Autorizado
	mwWildcard := NewIPWhitelistMiddleware([]string{"*"})
	handlerWildcard := mwWildcard.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	reqWild := httptest.NewRequest(http.MethodGet, "/api/v1/trunks", nil)
	reqWild.RemoteAddr = "200.150.100.50:5555"
	recWild := httptest.NewRecorder()
	handlerWildcard.ServeHTTP(recWild, reqWild)
	if recWild.Code != http.StatusOK {
		t.Errorf("esperava status 200 para wildcard *, obtido %d", recWild.Code)
	}
}
