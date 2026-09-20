# Auditoria Forense, Rastreabilidade Ponta a Ponta e Breakpoints Críticos

Este documento estabelece os padrões técnicos de auditoria, conciliação transacional e diagnóstico forense para prevenção e detecção de quebras de fluxo no **Dialer-Go**.

---

## 1. Padrão CorrelationID & Rastreamento de Jornada

1. Toda chamada que ingressa no discador recebe um identificador único universal:
   - Formato Preditivo: `pred-corr-<uuid>`
   - Formato Manual: `man-corr-<uuid>`
   - Formato Receptivo: `inb-corr-<uuid>`
2. O `correlation_id` é injetado como variável de canal Asterisk (`__CORRELATION_ID`) e propagado por todas as pernas de chamada SIP e eventos AMI.
3. Cada transição de estado relevante é gravada na tabela `execution_traces` via procedure `fn_audit_log_trace`.

---

## 2. Controle Dinâmico de Tracing (Modo Diagnóstico vs. Alta Performance)

* **Produção Estável (`ENABLE_EXECUTION_TRACE=false`):**
  - Executa bypass de inserções na tabela `execution_traces`.
  - Evita overhead de I/O em banco relacional e consumo desnecessário de storage.
* **Modo Diagnóstico / Homologação (`ENABLE_EXECUTION_TRACE=true`):**
  - Grava cada etapa do ciclo em tempo real.
  - Permite reconstrução histórica completa da jornada de cada lead.

---

## 3. O Ciclo de Vida Canônico dos 11 Passos Telefônicos

```text
[Passo 1: Recebimento HTTP / Validação] 
   └──> [Passo 2: Verificação de Pausa e Agentes Disponíveis]
          └──> [Passo 3: Filtragem de Troncos Elegíveis no Pool PJSIP]
                 └──> [Passo 4: Arbitragem de Canais no ChannelManager]
                        └──> [Passo 5: Reserva Atômica de Leads FILO via Procedure]
                               └──> [Passo 6: Disparo de Ação Originate via Socket AMI]
                                      └──> [Passo 7: Execução de Pre-Dial & Injeção SIP]
                                             └──> [Passo 8: Triagem AMD Híbrida (Asterisk + Vosk)]
                                                    └──> [Passo 9: Desvio/Redirect para Sala LiveKit / Agente]
                                                           └──> [Passo 10: Conversação e Gravação MixMonitor]
                                                                  └──> [Passo 11: Evento Hangup & Persistência ACID do CDR]
```

---

## 4. As 4 Stored Procedures Canônicas ACID

1. **`fn_audit_claim_predictive_batch(p_campaign_id, p_tenant_id, p_batch_size)`:**
   - Reserva leads com trava de concorrência `FOR UPDATE SKIP LOCKED`.
   - Altera status de `NEW` para `DIALING` e atualiza `last_dialed_at` atomicamente.
2. **`fn_audit_persist_predictive_result(p_lead_id, p_campaign_id, p_tenant_id, p_disposition, p_hangup_cause, p_duration, ...)`:**
   - Insere registro consolidado na tabela `cdrs`.
   - Atualiza o lead com status final (`ANSWERED`, `NO_ANSWER`, `BUSY`, etc.).
   - Se atendimento for confirmado, atualiza imediatamente `phone_trunk_mappings` para roteamento receptivo O(1).
3. **`fn_audit_recycle_campaign_leads(p_campaign_id, p_tenant_id, p_cooldown_minutes)`:**
   - Incrementa o `cycle_count` da campanha.
   - Recicla contatos não convertidos e atualiza o nível de saturação.
4. **`fn_audit_log_trace(p_correlation_id, p_campaign_id, p_lead_id, p_step, p_action, p_payload, p_status, p_error)`:**
   - Registra eventos de rastreamento com timestamp microsegundo.

---

## 5. Matriz Forense dos 6 Breakpoints Críticos de Processo

| Breakpoint | Descrição da Falha | Mecanismo de Prevenção Implementado |
| :--- | :--- | :--- |
| **BP-01** | Seleção de tronco não registrado ou indisponível na operadora. | `TrunkManager` filtra previamente troncos elegíveis (`is_enabled` e health check AMI positivo). |
| **BP-02** | Fila Redis desconectada da base relacional PostgreSQL. | Ingestão com inserção direta em PostgreSQL e sincronização em duas vias via stored procedures. |
| **BP-03** | Evento `Hangup` sem hook de persistência transacional de resultados. | Handler AMI intercepta `Hangup` obrigatoriamente e invoca `fn_audit_persist_predictive_result`. |
| **BP-04** | Ausência de propagação do `correlation_id` no dialplan Asterisk. | Injeção com herança dupla de sub-canal (`__CORRELATION_ID`) no `extensions.conf`. |
| **BP-05** | Risco de Panic / Crash por ponteiro nulo de banco de dados no boot. | Retry com backoff exponencial no boot e fail-fast controlado via `log.Fatalf` impedindo serviço zumbi. |
| **BP-06** | Represamento de leads por ausência de reciclagem automática. | Gatilho automático ao esgotar fila chamando `fn_audit_recycle_campaign_leads`. |
