# Ingestão de Mailing, Saturação e Looping Infinito FILO

Este documento estabelece as regras de ingestão de arquivos de mailing, medição de saturação de campanhas, reciclagem atômica de leads e controle de métricas operacionais no **Dialer-Go**.

---

## 1. Ingestão de Mailing e Refill via MinIO S3 (`POST /api/v1/campaigns/refill`)

1. **Origem:** O arquivo de mailing (CSV ou TXT) é enviado pelo OmniChat ou Estúdio IA para um bucket MinIO S3 (`mailings`).
2. **Processamento Assíncrono / Streaming:** O `MailingProcessor` lê o arquivo em streaming via `io.Reader` sem carregar o arquivo inteiro na memória RAM.
3. **Normalização de Entrada:**
   - Extrai `cpf`, `phone` e metadados customizados.
   - Telefones sem DDI/DDD são preservados fielmente (Zero Normalização).
   - Registros duplicados na mesma campanha são descartados ou consolidados.
4. **Inserção em Lote (Batch Insert):** Inserções no PostgreSQL utilizam transações em blocos de até 5.000 registros para maximizar vazão de I/O.

---

## 2. Looping Infinito FILO (First-In-Last-Out) & Reciclagem Atômica

Para campanhas com bases fechadas que exigem rotação contínua de contatos sem recarga manual:
1. **Ordem de Discagem FILO:** Leads recém-inseridos ou reciclados têm prioridade imediata (`ORDER BY last_dialed_at ASC NULLS FIRST, id DESC`).
2. **Reserva Atômica (`fn_audit_claim_predictive_batch`):** Stored procedure com trava de linha (`FOR UPDATE SKIP LOCKED`) reserva blocos de leads para o discador impedindo condições de corrida entre workers.
3. **Reciclagem Automática de Ciclo (`fn_audit_recycle_campaign_leads`):**
   - Quando não restarem leads com status `NEW` na campanha, a procedure incrementa o contador `cycle_count` da campanha.
   - Leads não atendidos (`NO_ANSWER`, `BUSY`) recebem reset para status `NEW` após janela de cooldown estipulada.
   - Leads com atendimento confirmado (`HUMAN`, `SALE`) não são reciclados, preservando o mailing contra re-discagens indevidas.

---

## 3. Escala de 5 Níveis de Saturação de Campanha

A saturação mede a degradação da base de contatos em função das tentativas de discagem e penetração:

| Nível de Saturação | Condição Matemática / Operacional | Ação do Sistema |
| :--- | :--- | :--- |
| **`NOVA`** | Base recém-carregada (`cycle_count == 0` e penetração de discagem < 25%). | Discagem plena com agressividade padrão. |
| **`OPERACIONAL`** | Penetração entre 25% e 75% da base, com taxa de contato estável. | Operação regular de pacing. |
| **`ALERTA`** | Penetração > 75% ou início do 2º ciclo (`cycle_count == 1`). | Redução gradual de agressividade para mitigar bloqueios de BINA. |
| **`CRITICA`** | Base no 3º ou 4º ciclo (`cycle_count >= 2`), taxa de contato em queda acentuada. | Alerta visual ao supervisor solicitando novo refill de base limpa. |
| **`ESGOTADA`** | Base exaurida (`cycle_count >= 5` ou 100% de leads trabalhados sem margem). | Sugestão de pausa automática ou encerramento da campanha. |

---

## 4. Política de Cache de 15 Minutos para Métricas Operacionais

Para evitar que dashboards analíticos e consultas frequentes de supervisores gerem sobrecarga de queries analíticas na tabela `cdrs`:
1. O endpoint `GET /api/v1/campaigns/{id}/reports/operational` utiliza cache em Redis com TTL de **15 minutos** (`900s`).
2. As queries analíticas utilizam passadas únicas (`FILTER (WHERE ...)`) com índices cobridores compostos no PostgreSQL (`idx_cdrs_operational_metrics`).
3. Requisições subsequentes dentro da janela de 15 minutos são respondidas em sub-milissegundo direto da memória.
