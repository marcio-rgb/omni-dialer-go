# Regra de Auditoria Obrigatória de Métodos e Processos

Esta regra é de cumprimento **estrito, obrigatório e inegociável** por qualquer IA ou desenvolvedor atuando neste workspace.

---

## 1. Gatilho de Aplicação
Qualquer modificação no código-fonte do projeto — incluindo, mas não se limitando a:
- Alteração de uma única linha, parâmetro, tipo, validação, retorno ou vírgula em métodos de:
  - `internal/core/` (`predictive_engine.go`, `manual_engine.go`, `channel_manager.go`, `trunk_manager.go`, `inbound_engine.go`, etc.)
  - `internal/adapters/` (`http/`, `postgres/`, `ami/`, `redis/`, `storage/`)
  - `internal/ports/` (`ami_port.go`, `cache_port.go`, `repository_port.go`, `storage_port.go`)
  - `internal/domain/` (`call.go`, `trunk.go`, `campaign.go`, `lead.go`, `report.go`, `errors.go`)
- Adição ou remoção de rotas HTTP no servidor Chi (`internal/adapters/http/`).
- Alterações no dialplan telefônico do Asterisk (`extensions.conf`), configurações SIP (`pjsip.conf`, `amd.conf`) ou AGI/EAGI (`cmd/vosk-eagi`).
- Alterações em stored procedures ou tabelas de banco (`database/schema.sql`).

---

## 2. Ação Mandatória (Sincronização Imediata dos Workflows)
Sempre que o gatilho acima ocorrer, é **obrigatório atualizar imediatamente na mesma intervenção** os documentos canônicos:
1. 👉 [`.agents/workflows/agente-go-especialista.md`](file:///home/marcio/ominichat/dialer-go/.agents/workflows/agente-go-especialista.md):
   - Assinaturas exatas de métodos, interfaces de portas, structs de domínio, DTOs e tipagem estrita Go 1.25.
2. 👉 [`.agents/workflows/agente-asterik-especialista.md`](file:///home/marcio/ominichat/dialer-go/.agents/workflows/agente-asterik-especialista.md):
   - Contextos do dialplan, parâmetros de AMD, scripts EAGI, modelos Vosk STT, roteamento PJSIP e headers SIP.
3. 👉 [`.agents/workflows/agente-audit-processo.md`](file:///home/marcio/ominichat/dialer-go/.agents/workflows/agente-audit-processo.md):
   - Ciclo de vida, gatilhos de início/repouso/término, trace dinâmico, gravação de CDR e matriz de rastreabilidade.

---

## 3. Cláusula de Bloqueio
> [!CAUTION]
> Nenhuma tarefa, refatoração ou correção de bug é considerada **concluída** sem que todos os workflows estejam 100% atualizados e refletindo a verdade do código em execução.
