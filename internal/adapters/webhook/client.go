package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"dialer-go/internal/domain"
	"dialer-go/internal/ports"
)

// WebhookClient implementa ports.WebhookPort enviando eventos de telefonia para sistemas consumidores via HTTP REST.
//
// @pattern Adapter (HTTP Client)
// @governedBy docs/rules/TELEPHONY_POLICIES.md#3-notificacoes-assincronas-e-webhooks
type WebhookClient struct {
	httpClient *http.Client
	defaultURL string
}

var _ ports.WebhookPort = (*WebhookClient)(nil)

// NewWebhookClient cria uma nova instância do cliente de webhook com timeout estrito de 5 segundos.
func NewWebhookClient(defaultURL string) *WebhookClient {
	return &WebhookClient{
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		defaultURL: defaultURL,
	}
}

// NotifyCallEnded dispara o evento de término de chamada via HTTP POST.
//
// @pattern Adapter (Method)
// @governedBy docs/rules/TELEPHONY_POLICIES.md#3-notificacoes-assincronas-e-webhooks
//
// @preExecution
// - Validar URL de destino (usa webhookURL informada ou fallback para defaultURL)
// - Serializar struct CallEndedWebhookPayload em JSON
//
// @postExecution
// - Registrar log estruturado com CallID, Disposition e status code de resposta
// - Descartar body de resposta para reutilização da conexão HTTP (keep-alive)
func (c *WebhookClient) NotifyCallEnded(ctx context.Context, webhookURL string, payload *domain.CallEndedWebhookPayload) error {
	targetURL := webhookURL
	if targetURL == "" {
		targetURL = c.defaultURL
	}
	if targetURL == "" {
		return nil // Nenhuma URL de webhook configurada. Notificação desativada.
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("falha ao serializar payload de webhook: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("falha ao criar requisição de webhook: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-Id", payload.TenantID)
	req.Header.Set("User-Agent", "DialerGo-WebhookNotifier/1.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("[WARN] [WEBHOOK] Falha ao enviar notificação de término da chamada %s para %s: %v", payload.CallID, targetURL, err)
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("[WARN] [WEBHOOK] Webhook para chamada %s retornou HTTP %d", payload.CallID, resp.StatusCode)
		return fmt.Errorf("webhook retornou status inesperado: %d", resp.StatusCode)
	}

	log.Printf("[INFO] [WEBHOOK] Chamada %s (Disposition: %s) notificada com sucesso para %s (HTTP %d)", payload.CallID, payload.Disposition, targetURL, resp.StatusCode)
	return nil
}
