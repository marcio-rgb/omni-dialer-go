# Regras Gerais do Workspace (Dialer-Go)

## 1. Governança de Agentes & Workflows Especializados

O desenvolvimento, manutenção e operação do **Dialer-Go** (`go 1.25.0`) é regido por workflows de agentes especializados e complementares:

1. 👉 [`.agents/workflows/agente-audit-processo.md`](file:///home/marcio/ominichat/dialer-go/.agents/workflows/agente-audit-processo.md): **Agente de Teste, Auditoria Ponta a Ponta & Controle de Quebra de Processo.**
   - Focado em auditoria de ciclo de vida (início, repouso/idle, término), conciliação de banco/cache/AMI, trace dinâmico liga/desliga (`execution_traces`), gravação de CDR e matriz de rastreabilidade.
2. 👉 [`.agents/workflows/agente-go-especialista.md`](file:///home/marcio/ominichat/dialer-go/.agents/workflows/agente-go-especialista.md): **Agente Especialista na Linguagem Go (Go 1.25.0 Architecture & Language Specialist).**
   - Focado em padrões idiomáticos Go 1.25, estruturas de dados, concorrência thread-safe com `sync/atomic.Int32` e `sync.RWMutex`, tipagem estrita, arquitetura hexagonal (`domain`, `ports`, `core`, `adapters`) e especificação detalhada de contratos e assinaturas de métodos.
3. 👉 [`.agents/workflows/agente-asterisk-especialista.md`](file:///home/marcio/ominichat/dialer-go/.agents/workflows/agente-asterisk-especialista.md): **Agente Especialista Asterisk (PBX, Dialplan & AMD Specialist).**
   - Focado em infraestrutura telefônica Asterisk, sintaxe de dialplan (`extensions.conf`), triagem ultrarrápida de atendimento humano (< 1,5s), AMD nativo (`app_amd`), reconhecimento de voz com Vosk STT via EAGI em Go, roteamento PJSIP, pre-dial handlers e codecs.
4. 👉 [`.agents/workflows/agente-tester-dialer.md`](file:///home/marcio/ominichat/dialer-go/.agents/workflows/agente-tester-dialer.md): **Agente Tester Dialer & Validação Ponta a Ponta (SIP Transit & N10/N11 Integration).**
   - Focado em garantia de Zero Normalização no Dialer-Go, testes automatizados de discagem multiformato (`10`, `11`, `12`, `13`, `14` dígitos), contrato de pre-dial LiveKit e conformidade técnica ([`validacao-dialer-n10.md`](file:///home/marcio/ominichat/dialer-go/.agents/workflows/validacao-dialer-n10.md)).

---

## 2. Regra de Sincronização Obrigatória de Métodos e Contratos
Qualquer alteração — inclusive adição, remoção, alteração de assinatura, parâmetros, retorno, validação ou dialplan — em:
- `internal/core/` (motores preditivo, manual, receptivo, canais e troncos)
- `internal/adapters/` (HTTP, PostgreSQL, AMI, Redis, Storage)
- `internal/ports/` (interfaces e contratos canônicos)
- `internal/domain/` (entidades e DTOs)
- `extensions.conf` / Dialplan Asterisk e AGI/EAGI (`cmd/vosk-eagi/`)
- `database/schema.sql` (tabelas, índices, views e stored procedures)

**Exige obrigatoriamente e na mesma intervenção a atualização dos workflows correspondentes:**
1. Atualizar contratos, assinaturas e tipagem no [Agente Go Especialista](file:///home/marcio/ominichat/dialer-go/.agents/workflows/agente-go-especialista.md).
2. Atualizar contextos, regras de AMD, variáveis de canal ou roteamento no [Agente Especialista Asterisk](file:///home/marcio/ominichat/dialer-go/.agents/workflows/agente-asterisk-especialista.md).
3. Atualizar o ciclo de vida, rastreamento e matriz no [Agente Auditor de Processo](file:///home/marcio/ominichat/dialer-go/.agents/workflows/agente-audit-processo.md).

> [!CAUTION]
> Nenhuma alteração é dada por finalizada se os mapeamentos dos agentes estiverem desatualizados ou divergentes do código-fonte em execução.

---

## 3. Regra Geral e Obrigatória de Governança da API (API_REFERENCE.md)

Toda e qualquer alteração, adição, remoção ou refatoração nos endpoints HTTP do **Dialer-Go** exige **obrigatoriamente a atualização detalhada e imediata** do documento canônico:
👉 [`.agents/workflows/API_REFERENCE.md`](file:///home/marcio/ominichat/dialer-go/.agents/workflows/API_REFERENCE.md)

### Diretrizes de Cumprimento Obrigatório:
1. **Sincronização Atômica:** Qualquer commit ou intervenção que modifique ou adicione:
   - Rotas HTTP (`internal/adapters/http/server.go` ou sub-handlers).
   - Parâmetros de requisição (Query Parameters, Path Parameters, Headers HTTP ou JSON Request Body).
   - DTOs de entrada e saída (`internal/domain/*_dto.go` ou `internal/domain/*.go`).
   - Códigos de status HTTP e payloads de erro padronizados (RFC 7807).
   - Disparos de webhooks de saída (egress) ou endpoints consumidos de integração (ex: `/api/dialer/refill`, `/api/telephony/webhook/call-ended`).
   - Barramentos de eventos assíncronos ou canais de monitoramento WebSocket em tempo real.
   **DEVE conter no mesmo commit/intervenção a documentação completa correspondente no `API_REFERENCE.md`.**
2. **Nível de Detalhe Exigido no `API_REFERENCE.md`:**
   - Descrição da responsabilidade operacional do endpoint.
   - Headers necessários (`Content-Type`, `X-Tenant-Id`, `User-Agent`, etc.).
   - Tabela descritiva de campos com tipos, obrigatoriedade, valores padrão e validações.
   - Exemplos reais de JSON Request Body.
   - Exemplos reais de JSON Response de Sucesso (`200 OK`, `201 Created`, etc.).
   - Catálogo de erros mapeados no padrão Problem Details (RFC 7807) com códigos de status (`400`, `404`, `422`, `429`, `500`), códigos internos (`INVALID_JSON`, `TRUNK_NOT_FOUND`, etc.) e causas prováveis.
3. **Proibição de Descompasso:** É terminantemente proibido publicar código ou finalizar tarefas com endpoints sem documentação ou com discrepâncias entre as structs do Go e o catálogo da API.
