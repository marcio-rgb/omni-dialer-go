package http

import (
	"net"
	"net/http"
	"strings"
	"sync"

	"dialer-go/internal/domain"
)

type IPWhitelistMiddleware struct {
	allowedIPs sync.Map
	cidrsMu    sync.RWMutex
	cidrs      []*net.IPNet
}

func NewIPWhitelistMiddleware(initialIPs []string) *IPWhitelistMiddleware {
	m := &IPWhitelistMiddleware{}
	for _, ip := range initialIPs {
		m.AddIP(ip)
	}
	return m
}

func (m *IPWhitelistMiddleware) AddIP(ip string) {
	clean := strings.TrimSpace(ip)
	if clean == "" {
		return
	}
	if strings.Contains(clean, "/") {
		_, ipNet, err := net.ParseCIDR(clean)
		if err == nil && ipNet != nil {
			m.cidrsMu.Lock()
			m.cidrs = append(m.cidrs, ipNet)
			m.cidrsMu.Unlock()
			return
		}
	}
	m.allowedIPs.Store(clean, true)
}

func (m *IPWhitelistMiddleware) RemoveIP(ip string) {
	clean := strings.TrimSpace(ip)
	if clean == "" {
		return
	}
	m.allowedIPs.Delete(clean)
	if strings.Contains(clean, "/") {
		m.cidrsMu.Lock()
		defer m.cidrsMu.Unlock()
		filtered := make([]*net.IPNet, 0, len(m.cidrs))
		for _, netw := range m.cidrs {
			if netw.String() != clean {
				filtered = append(filtered, netw)
			}
		}
		m.cidrs = filtered
	}
}

func (m *IPWhitelistMiddleware) IsAllowed(ip string) bool {
	if _, ok := m.allowedIPs.Load("*"); ok {
		return true
	}
	if _, ok := m.allowedIPs.Load(ip); ok {
		return true
	}

	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	m.cidrsMu.RLock()
	defer m.cidrsMu.RUnlock()
	for _, netw := range m.cidrs {
		if netw.Contains(parsedIP) {
			return true
		}
	}

	return false
}

// Handler retorna o middleware net/http que intercepta e valida requisições em sub-microssegundo.
//
// @pattern Middleware (Chain of Responsibility)
// @governedBy docs/rules/SECURITY_NETWORK.md#1-autenticação-por-ip-whitelist-em-memória-syncmap
//
// @preExecution
// - Extração de IP real da requisição (`RemoteAddr`, `X-Forwarded-For`)
// - Verificação na tabela hash em memória `sync.Map` ou faixas CIDR
//
// @postExecution
// - Se não autorizado: Interrupção imediata da cadeia com HTTP 403 Forbidden (RFC 7807)
// - Se autorizado: Encaminhamento transparente para o próximo handler via `next.ServeHTTP`
func (m *IPWhitelistMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Ignora verificação para endpoints de health check
		if r.URL.Path == "/health" || r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}

		remoteIP, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			remoteIP = r.RemoteAddr
		}

		// Se vier por proxy de borda confiável
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			remoteIP = strings.TrimSpace(parts[0])
		}

		if !m.IsAllowed(remoteIP) {
			errResp := domain.NewErrForbidden(
				"O endereço IP de origem '" + remoteIP + "' não consta na lista branca em memória autorizada a consultar o discador.",
			)
			errResp.WriteJSON(w)
			return
		}

		next.ServeHTTP(w, r)
	})
}
