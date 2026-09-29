package livekit

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"dialer-go/internal/ports"
)

// Client implementa ports.LiveKitPort consumindo a API Twirp do LiveKit SFU.
//
// @pattern Adapter
// @governedBy .agents/ARCHITECT.md#livekit-integration
type Client struct {
	baseURL    string
	apiKey     string
	apiSecret  string
	httpClient *http.Client
}

// NewClient instancia um novo adapter LiveKit com timeout determinístico.
func NewClient(baseURL, apiKey, apiSecret string) *Client {
	trimmedURL := strings.TrimRight(baseURL, "/")
	if strings.HasPrefix(trimmedURL, "ws://") {
		trimmedURL = "http://" + strings.TrimPrefix(trimmedURL, "ws://")
	} else if strings.HasPrefix(trimmedURL, "wss://") {
		trimmedURL = "https://" + strings.TrimPrefix(trimmedURL, "wss://")
	}

	return &Client{
		baseURL:   trimmedURL,
		apiKey:    apiKey,
		apiSecret: apiSecret,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		},
	}
}

var _ ports.LiveKitPort = (*Client)(nil)

func (c *Client) generateToken() (string, error) {
	headerBytes, _ := json.Marshal(map[string]string{
		"alg": "HS256",
		"typ": "JWT",
	})

	now := time.Now().Unix()
	payloadBytes, err := json.Marshal(map[string]interface{}{
		"exp": now + 3600,
		"iss": c.apiKey,
		"nbf": now - 10,
		"sub": c.apiKey,
		"video": map[string]bool{
			"roomAdmin":  true,
			"roomCreate": true,
			"roomList":   true,
		},
		"sip": map[string]bool{
			"admin": true,
			"call":  true,
		},
	})
	if err != nil {
		return "", fmt.Errorf("falha ao codificar payload jwt livekit: %w", err)
	}

	encHeader := base64.RawURLEncoding.EncodeToString(headerBytes)
	encPayload := base64.RawURLEncoding.EncodeToString(payloadBytes)
	unsignedToken := encHeader + "." + encPayload

	h := hmac.New(sha256.New, []byte(c.apiSecret))
	h.Write([]byte(unsignedToken))
	sig := base64.RawURLEncoding.EncodeToString(h.Sum(nil))

	return unsignedToken + "." + sig, nil
}

func (c *Client) postTwirp(ctx context.Context, endpoint string, reqPayload interface{}, respDest interface{}) error {
	token, err := c.generateToken()
	if err != nil {
		return err
	}

	var reqBody []byte
	if reqPayload != nil {
		reqBody, err = json.Marshal(reqPayload)
		if err != nil {
			return fmt.Errorf("falha ao serializar payload para %s: %w", endpoint, err)
		}
	} else {
		reqBody = []byte("{}")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("falha ao criar requisicao para %s: %w", endpoint, err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("falha de conexao com livekit em %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("livekit twirp %s retornou status %d: %s", endpoint, resp.StatusCode, string(bodyBytes))
	}

	if respDest != nil {
		if err := json.Unmarshal(bodyBytes, respDest); err != nil {
			return fmt.Errorf("falha ao decodificar resposta de %s: %w", endpoint, err)
		}
	}

	return nil
}

type listTrunksResponse struct {
	Items []struct {
		SIPTrunkID string `json:"sip_trunk_id"`
		Name       string `json:"name"`
	} `json:"items"`
}

type listRulesResponse struct {
	Items []struct {
		SIPDispatchRuleID string `json:"sip_dispatch_rule_id"`
		Name              string `json:"name"`
	} `json:"items"`
}

// EnsureSIPTrunkAndRule garante idempotentemente a existência do tronco e regra SIP no LiveKit.
//
// @pattern Adapter (Method)
// @governedBy .agents/ARCHITECT.md#livekit-integration
//
// @preExecution
// - Conexão HTTP válida com o LiveKit SFU
//
// @postExecution
// - Cria Inbound Trunk se ausente
// - Cria Dispatch Rule do tipo Callee se ausente
func (c *Client) EnsureSIPTrunkAndRule(ctx context.Context) error {
	var trunksResp listTrunksResponse
	if err := c.postTwirp(ctx, "/twirp/livekit.SIP/ListSIPInboundTrunk", nil, &trunksResp); err != nil {
		return fmt.Errorf("erro ao listar troncos sip: %w", err)
	}

	var trunkID string
	if len(trunksResp.Items) == 0 {
		createTrunkReq := map[string]interface{}{
			"trunk": map[string]interface{}{
				"name":              "Asterisk-PBX",
				"allowed_addresses": []string{"0.0.0.0/0"},
			},
		}
		var createdTrunk struct {
			SIPTrunkID string `json:"sip_trunk_id"`
		}
		if err := c.postTwirp(ctx, "/twirp/livekit.SIP/CreateSIPInboundTrunk", createTrunkReq, &createdTrunk); err != nil {
			return fmt.Errorf("erro ao criar tronco inbound sip no livekit: %w", err)
		}
		trunkID = createdTrunk.SIPTrunkID
	} else {
		trunkID = trunksResp.Items[0].SIPTrunkID
	}

	var rulesResp listRulesResponse
	if err := c.postTwirp(ctx, "/twirp/livekit.SIP/ListSIPDispatchRule", nil, &rulesResp); err != nil {
		return fmt.Errorf("erro ao listar regras de despacho sip: %w", err)
	}

	if len(rulesResp.Items) == 0 {
		createRuleReq := map[string]interface{}{
			"rule": map[string]interface{}{
				"dispatch_rule_callee": map[string]interface{}{
					"room_prefix": "",
					"randomize":   false,
				},
			},
			"name":      "Asterisk Room Dispatch",
			"trunk_ids": []string{trunkID},
		}
		if err := c.postTwirp(ctx, "/twirp/livekit.SIP/CreateSIPDispatchRule", createRuleReq, nil); err != nil {
			return fmt.Errorf("erro ao criar regra de despacho sip no livekit: %w", err)
		}
	}

	return nil
}

// CheckSIPHealth audita a integridade da ponte LiveKit SIP retornando se há troncos e regras ativos.
//
// @pattern Adapter (Method)
// @governedBy .agents/ARCHITECT.md#livekit-integration
//
// @preExecution
// - Requisitado periodicamente pelo HealthHandler
//
// @postExecution
// - Retorna true se houver ao menos 1 tronco e 1 regra, ou false com erro detalhado
func (c *Client) CheckSIPHealth(ctx context.Context) (bool, error) {
	var trunksResp listTrunksResponse
	if err := c.postTwirp(ctx, "/twirp/livekit.SIP/ListSIPInboundTrunk", nil, &trunksResp); err != nil {
		return false, fmt.Errorf("tronco sip inacessivel: %w", err)
	}

	var rulesResp listRulesResponse
	if err := c.postTwirp(ctx, "/twirp/livekit.SIP/ListSIPDispatchRule", nil, &rulesResp); err != nil {
		return false, fmt.Errorf("regras sip inacessiveis: %w", err)
	}

	if len(trunksResp.Items) == 0 || len(rulesResp.Items) == 0 {
		return false, fmt.Errorf("ponte sip incompleta (troncos: %d, regras: %d)", len(trunksResp.Items), len(rulesResp.Items))
	}

	return true, nil
}

// StartReconciler inicia um loop contínuo de reconciliação em background para auto-healing da ponte SIP.
//
// @pattern Observer / Daemon Worker
// @governedBy .agents/ARCHITECT.md#livekit-integration
//
// @preExecution
// - Contexto pai para cancelamento no shutdown gracioso
// - Intervalo de verificação periódico (default 30s)
//
// @postExecution
// - Audita e recria Inbound Trunk e Dispatch Rule no LiveKit caso desapareçam
func (c *Client) StartReconciler(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}

	go func() {
		log.Printf("[INFO] LiveKit SIP: Reconciler daemon ativo (intervalo: %v).", interval)

		// Executa primeira validação imediatamente no boot
		bootCtx, bootCancel := context.WithTimeout(ctx, 10*time.Second)
		if err := c.EnsureSIPTrunkAndRule(bootCtx); err != nil {
			log.Printf("[WARN] LiveKit SIP: reconciliação inicial pendente/falhou: %v", err)
		} else {
			log.Println("[INFO] LiveKit SIP: Inbound Trunk e Dispatch Rule validados com sucesso no boot.")
		}
		bootCancel()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		var wasDegraded bool
		for {
			select {
			case <-ctx.Done():
				log.Println("[INFO] LiveKit SIP: Reconciler daemon finalizado.")
				return
			case <-ticker.C:
				runCtx, runCancel := context.WithTimeout(ctx, 10*time.Second)
				err := c.EnsureSIPTrunkAndRule(runCtx)
				runCancel()

				if err != nil {
					if !wasDegraded {
						log.Printf("[WARN] LiveKit SIP: ponte inconsistente detectada, tentando auto-reparo: %v", err)
						wasDegraded = true
					}
				} else {
					if wasDegraded {
						log.Println("[INFO] LiveKit SIP: ponte restabelecida com sucesso pelo reconciler.")
						wasDegraded = false
					}
				}
			}
		}
	}()
}
