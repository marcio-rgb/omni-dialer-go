package http

import (
	"encoding/json"
	"net/http"

	"dialer-go/internal/core"
	"dialer-go/internal/domain"
)

type RefillHandler struct {
	processor *core.MailingProcessor
}

func NewRefillHandler(processor *core.MailingProcessor) *RefillHandler {
	return &RefillHandler{processor: processor}
}

// Refill processa a ingestão assíncrona de um novo arquivo de mailing armazenado no MinIO S3.
//
// @pattern Adapter (HTTP Handler / Ingestion)
// @governedBy docs/rules/CAMPAIGN_SATURATION.md#1-ingestão-de-mailing-e-refill-via-minio-s3-post-apiv1campaignsrefill
//
// @preExecution
// - Validação de autorização IP em: `httpAdapter.IPWhitelistMiddleware`
// - Validação de campos obrigatórios (`tenant_id`, `campaign_id`, `file_uri`)
// - Verificação de existência e leitura streaming no MinIO S3
//
// @postExecution
// - Inserção em batch de leads no PostgreSQL com Zero Normalização
// - Atualização de status da campanha e reset de saturação para `NOVA`
// - Retorno de total de leads importados com sucesso
func (h *RefillHandler) Refill(w http.ResponseWriter, r *http.Request) {
	var req domain.RefillRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		domain.NewErrBadRequest("INVALID_JSON", "Payload JSON inválido").WriteJSON(w)
		return
	}

	if req.TenantID == "" || req.CampaignID == "" || req.FileURI == "" {
		domain.NewErrBadRequest("MISSING_FIELDS", "tenant_id, campaign_id e file_uri são obrigatórios").WriteJSON(w)
		return
	}

	resp, err := h.processor.ProcessZipRefill(r.Context(), req.TenantID, req.CampaignID, req.FileURI)
	if err != nil {
		if prob, ok := err.(*domain.ProblemDetails); ok {
			prob.WriteJSON(w)
			return
		}
		domain.NewErrInternal(err.Error()).WriteJSON(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    resp,
	})
}
