package domain

import (
	"encoding/json"
	"net/http"
)

// ProblemDetails implementa a especificação RFC 7807 / RFC 9457 para respostas de erro de API.
type ProblemDetails struct {
	Type          string                 `json:"type"`
	Title         string                 `json:"title"`
	Status        int                    `json:"status"`
	Detail        string                 `json:"detail"`
	Instance      string                 `json:"instance,omitempty"`
	Code          string                 `json:"code"`
	InvalidParams []InvalidParam         `json:"invalid_params,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

type InvalidParam struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

func (p *ProblemDetails) Error() string {
	return p.Detail
}

// WriteJSON renderiza a resposta de erro diretamente no http.ResponseWriter.
func (p *ProblemDetails) WriteJSON(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(p)
}

// Erros de Domínio Padronizados:
func NewErrForbidden(detail string) *ProblemDetails {
	return &ProblemDetails{
		Type:   "https://dialer-go.internal/errors/forbidden",
		Title:  "Forbidden",
		Status: http.StatusForbidden,
		Detail: detail,
		Code:   "IP_NOT_WHITELISTED",
	}
}

func NewErrBadRequest(code, detail string, params ...InvalidParam) *ProblemDetails {
	return &ProblemDetails{
		Type:          "https://dialer-go.internal/errors/bad-request",
		Title:         "Bad Request",
		Status:        http.StatusBadRequest,
		Detail:        detail,
		Code:          code,
		InvalidParams: params,
	}
}

func NewErrNotFound(code, detail string) *ProblemDetails {
	return &ProblemDetails{
		Type:   "https://dialer-go.internal/errors/not-found",
		Title:  "Not Found",
		Status: http.StatusNotFound,
		Detail: detail,
		Code:   code,
	}
}

func NewErrConflict(code, detail string, meta map[string]interface{}) *ProblemDetails {
	return &ProblemDetails{
		Type:     "https://dialer-go.internal/errors/conflict",
		Title:    "Conflict",
		Status:   http.StatusConflict,
		Detail:   detail,
		Code:     code,
		Metadata: meta,
	}
}

func NewErrUnprocessable(code, detail string, params ...InvalidParam) *ProblemDetails {
	return &ProblemDetails{
		Type:          "https://dialer-go.internal/errors/unprocessable-entity",
		Title:         "Unprocessable Entity",
		Status:        http.StatusUnprocessableEntity,
		Detail:        detail,
		Code:          code,
		InvalidParams: params,
	}
}

func NewErrTooManyRequests(code, detail string, meta map[string]interface{}) *ProblemDetails {
	return &ProblemDetails{
		Type:     "https://dialer-go.internal/errors/too-many-requests",
		Title:    "Too Many Requests",
		Status:   http.StatusTooManyRequests,
		Detail:   detail,
		Code:     code,
		Metadata: meta,
	}
}

func NewErrInternal(detail string) *ProblemDetails {
	return &ProblemDetails{
		Type:   "https://dialer-go.internal/errors/internal-server-error",
		Title:  "Internal Server Error",
		Status: http.StatusInternalServerError,
		Detail: detail,
		Code:   "INTERNAL_ERROR",
	}
}
