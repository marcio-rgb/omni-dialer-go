# Arquitetura Mestre do Projeto: Dialer-Go

Documento de entrada canônico do protocolo **Bootstrap Zero** (conforme Seção 0.1 de `RULE[user_global]`).

> [!NOTE]
> Este arquivo sintetiza a arquitetura canônica do projeto. Para o detalhamento completo dos fluxos, algoritmos de pacing, DDLs, queries e análise forense, consulte:
> - 👉 [Documento Mestre Canônico na Raiz: `ARCHITECT.md`](file:///home/marcio/ominichat/dialer-go/ARCHITECT.md)
> - 👉 [Manual de Arquitetura Operacional Exaustivo (2.000+ linhas): `.agents/ARCHITECTURE.md`](file:///home/marcio/ominichat/dialer-go/.agents/ARCHITECTURE.md)
> - 👉 [Índice do Repositório & Grafo de Componentes: `docs/MAP.md`](file:///home/marcio/ominichat/dialer-go/docs/MAP.md)

---

## 1. Responsabilidade Unívoca

O **Dialer-Go** é o orquestrador e motor central de telefonia em **Go puro (Go 1.25)** da plataforma, responsável por:
- Discagem automatizada **Preditiva** (pacing com auto-balanceamento de overdialing), **Manual** (preempção humana) e **Receptiva** (lookup O(1) de retorno dinâmico).
- Comunicação de baixa latência com a central Asterisk via socket TCP nativo AMI (:5038).
- Triagem ultrarrápida de atendimento humano vs. secretária eletrônica (AMD Híbrido Asterisk + Vosk STT via EAGI) com entrega em menos de 1,5s.
- Arbitragem de canais simultâneos globais e por tronco com cota de reserva humana.
- Gestão e hot-reload dinâmico de troncos SIP/PJSIP sem interrupção de chamadas ativas.

---

## 2. Invariantes Arquiteturais (Regras Inquebráveis)

1. **Linguagem Única (100% Go):** Binário estático Go 1.25 sem runtime interpretado (Node/Python) no discador.
2. **Limite Rígido de 500 Linhas:** Nenhum arquivo `.go` ultrapassa 500 linhas de código.
3. **Tipagem Estrita Go 1.25:** Proibido uso de `any` ou `interface{}` genérico; bordas validadas via structs tipadas e RFC 7807 (`ProblemDetails`).
4. **IP Whitelist em Memória (`sync.Map`):** Autorização REST sub-microssegundo sem dependência de banco ou tokens pesados.
5. **Base PostgreSQL Isolada (`dialer_db`):** Banco relacional standalone exclusivo da telefonia.
6. **Lookup Receptivo O(1):** Retorno roteado via tabela indexada `phone_trunk_mappings`.
7. **Zero Normalização Telefônica:** Discador preserva estritamente os dígitos do número (`10` a `14` dígitos).
8. **Quota Humana Garantida:** 10 canais globais rigidamente reservados para operadores manuais.
9. **Abandono Regulatório < 2s:** Chamada sem destino em 2s é desconectada e auditada.
10. **Rastreabilidade CorrelationID:** Toda jornada gravada em `execution_traces`.

---

## 3. Padrões de Design Patterns Adotados

- **Strategy Pattern:** Seleção dinâmica de motores de discagem (`PredictiveEngine`, `ManualEngine`, `InboundEngine`).
- **Adapter Pattern:** Isolamento de rede e persistência (`internal/adapters/{ami, postgres, redis, storage, http}`).
- **Repository Pattern:** Abstração de banco relacional em `internal/ports/repository_port.go`.
- **Observer / Pub-Sub:** Leitor assíncrono de eventos AMI multicanais.
- **Middleware:** `IPWhitelistMiddleware` e encadeamento no roteador Chi.
- **Circuit Breaker & Retry:** Retentativas de conexão no boot com backoff exponencial.
- **Transactional Outbox / Stored Procedures ACID:** Atomicidade na gestão de leads e saturação.

---

## 4. Tech Stack e Runtimes

- **Linguagem:** Go 1.25.0
- **HTTP Framework:** Chi v5
- **Telefonia:** Asterisk PBX 20 (AMI TCP + PJSIP)
- **STT/AMD:** Vosk Kaldi Server (WebSocket :2700) via EAGI (`cmd/vosk-eagi`)
- **Bancos:** PostgreSQL 16 (`pgx/v5`), Redis 7 (`go-redis/v9`), MinIO S3

---

## 5. Grafo de Dependências

- **Upstream:** OmniChat (Operadores), Estúdio IA (Agentes LiveKit), Cron de Campanhas.
- **Downstream:** Asterisk PBX (:5038), Operadoras PJSIP, LiveKit SIP, MinIO (:9000).
- **Eventos:** `PredictiveHuman`, `InboundCall`, `AsyncAGI`, `Hangup`.

---

## 6. Master Route

1. **Entrada HTTP:** Chi Router + `IPWhitelistMiddleware`
2. **Validação de DTO:** Structs em `internal/domain/`
3. **Trava de Estado/Pausa:** Consulta atômica ao Redis
4. **Arbitragem de Capacidade:** `ChannelManager` (`atomic.Int32`)
5. **Reserva ACID:** Stored Procedure PostgreSQL `fn_audit_claim_predictive_batch`
6. **Sinalização Telefônica:** Asterisk AMI `Originate`
7. **Triagem de Mídia:** AMD Asterisk + Vosk EAGI (< 1,5s)
8. **Entrega de Áudio:** `Redirect` para Operador / Sala LiveKit
9. **Auditoria e CDR:** Gravação transacional pós-desconexão (`fn_audit_persist_predictive_result`)
