---
description: Validação Técnica e Contrato de Trânsito SIP N10/N11 entre Dialer-Go, Asterisk e Omnichat
---

# Validação Técnica: Trânsito SIP N10/N11 & Zero Normalização no Dialer-Go

## 1. Visão Geral da Arquitetura em Camadas

A integração entre o **Dialer-Go** e o **Omnichat** adota uma estratégia de desacoplamento rigorosa quanto ao formato de números telefônicos:

```text
+-------------------------------------------------------------------------------+
|                             CAMADA DE USUÁRIO                                 |
| Omnichat Frontend (Vue 3 / AgentVoiceWidget / BusinessDetailModal)            |
| -> Normalização amigável de exibição: formatPhone(phone) -> (21) 99991-4324   |
| -> Extração de iniciais de avatar com sanitização de caracteres especiais     |
+-------------------------------------------------------------------------------+
                                      |
                                      v
+-------------------------------------------------------------------------------+
|                            CAMADA DE APLICAÇÃO                                |
| Omnichat Backend (Node.js / contactLookupService / phoneUtils)                |
| -> toN10(): Extrai base local de 10 ou 11 dígitos                             |
| -> phoneCandidates: Busca multiformato (N10, N11, sem 9º dígito, 0, DDI 55)   |
+-------------------------------------------------------------------------------+
                                      |
                                      v
+-------------------------------------------------------------------------------+
|                            CAMADA DE TELEFONIA                                |
| Dialer-Go (Go 1.25) & Asterisk PBX 20 (PJSIP)                                 |
| -> Zero Normalização: Número trafega exatamente como recebido                 |
| -> Sanitização pura de caracteres: apenas dígitos 0-9 para URI SIP            |
| -> extensions.conf: Injeta CALLERID(num)=${PHONE} no pre-dial LiveKit         |
+-------------------------------------------------------------------------------+
```

### 1.1. Por Que o Dialer-Go Não Normaliza Números?
1. **Evitar Enrijecimento de Regras:** Cada operadora SIP e plano de terminação (PSTN, VoIP, rotas móveis/fixas) possui exigências distintas de discagem (com ou sem DDI `55`, com ou sem `0` de longa distância, ou com CSP da própria operadora como `015` ou `021`).
2. **Separação de Preocupações:** O Dialer-Go é uma central de telecomunicações de alta performance e baixa latência. Ele não deve inferir regras de DDD local ou regras de negócio de CRM.
3. **Imutabilidade e Rastreabilidade:** O número armazenado no CDR (`cdrs.phone`), na fila (`leads.phone`) e nos traces (`execution_traces.phone`) deve coincidir com o identificador do mailing e com o identificador informado na chamada manual.

---

## 2. Contrato de Trânsito SIP (PJSIP -> LiveKit -> WebRTC)

### 2.1. Variáveis de Canal e Pre-Dial Handler
No [`extensions.conf`](file:///home/marcio/ominichat/dialer-go/extensions.conf):
```ini
[predial-livekit-headers]
exten => s,1,NoOp(--- INJETANDO CABECALHOS SIP PARA LIVEKIT ---)
 same => n,ExecIf($[ "${PHONE}" != "" ]?Set(PJSIP_HEADER(add,X-Lead-Phone)=${PHONE}))
 same => n,ExecIf($[ "${PHONE}" != "" ]?Set(CALLERID(num)=${PHONE}))
 same => n,ExecIf($[ "${LEAD_NAME}" != "" ]?Set(PJSIP_HEADER(add,X-Lead-Name)=${LEAD_NAME}))
 same => n,ExecIf($[ "${LEAD_CPF}" != "" ]?Set(PJSIP_HEADER(add,X-Lead-CPF)=${LEAD_CPF}))
 same => n,ExecIf($[ "${CAMPAIGN_ID}" != "" ]?Set(PJSIP_HEADER(add,X-Campaign-Id)=${CAMPAIGN_ID}))
 same => n,ExecIf($[ "${LEAD_NAME}" != "" ]?Set(CALLERID(name)=${LEAD_NAME}))
 same => n,Return()
```

### 2.2. Geração da Identidade do Participante LiveKit
1. O Asterisk envia o SIP INVITE para o endpoint do LiveKit SIP Trunk contendo:
   - `From: "${LEAD_NAME}" <sip:${PHONE}@asterisk>`
   - `X-Lead-Phone: ${PHONE}`
2. O serviço LiveKit SIP cria o participante na sala do operador (`AGENT_ROOM` ou `sala_agente_<id>`) com a identidade:
   - `participant.identity = "sip_" + CALLERID(num)` (ex: `sip_5521999914324` ou `sip_21999914324`).
3. O Omnichat frontend escuta a entrada do participante:
   - Remove o prefixo `sip_`: `cleanCaller = participant.identity.replace(/^sip_/, '')`.
   - Aplica `toN10(cleanCaller)` para lookup e `formatPhone(cleanCaller)` para exibição na tela.

---

## 3. Matriz de Conformidade de Código Go

### 3.1. Manual Engine ([`manual_engine.go`](file:///home/marcio/ominichat/dialer-go/internal/core/manual_engine.go))
- Sanitiza caracteres:
  ```go
  digitsOnly := strings.Map(func(r rune) rune {
      if r >= '0' && r <= '9' {
          return r
      }
      return -1
  }, destPhone)
  if len(digitsOnly) > 0 {
      destPhone = digitsOnly
  }
  ```
- **Conformidade:** Preserva integralmente números de 10, 11, 12, 13 e 14 dígitos.

### 3.2. Predictive Engine ([`predictive_engine.go`](file:///home/marcio/ominichat/dialer-go/internal/core/predictive_engine.go))
- Sanitiza caracteres para discagem e preserva número original para variáveis de canal:
  ```go
  vars["PHONE"] = destPhone
- `callerID := destPhone`: o CallerID preserva estritamente o número real discado sem qualquer mutação de dígitos.

---

## 4. Roteiro de Validação e Testes Automatizados

### 4.1. Execução de Testes Unitários
Para validar a conformidade dos motores com todos os formatos telefônicos:
```bash
/snap/antigravity-cli/21/usr/lib/go-1.22/bin/go test -v ./internal/core -run "TestManualEngine_ZeroNormalization|TestPredictiveEngine"
```

### 4.2. Casos de Teste Cobertos
- **Fixo Local (10 dígitos):** `2133445566` -> Discado: `2133445566`, LiveKit: `sip_2133445566`, Omnichat: `(21) 3344-5566`.
- **Celular (11 dígitos):** `21999914324` -> Discado: `21999914324`, LiveKit: `sip_21999914324`, Omnichat: `(21) 99991-4324`.
- **Com Zero DDD (12 dígitos):** `021999914324` -> Discado: `021999914324`, LiveKit: `sip_021999914324`, Omnichat: `(21) 99991-4324`.
- **Com DDI 55 (13 dígitos):** `5521999914324` -> Discado: `5521999914324`, LiveKit: `sip_5521999914324`, Omnichat: `(21) 99991-4324`.
- **Com CSP 015 (14 dígitos):** `01521999914324` -> Discado: `01521999914324`, LiveKit: `sip_01521999914324`, Omnichat: `(21) 99991-4324`.
- **Formatado com Símbolos:** `(21) 99991-4324` -> Sanitizado: `21999914324`, LiveKit: `sip_21999914324`, Omnichat: `(21) 99991-4324`.
