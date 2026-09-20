# Arquitetura Mestre do Projeto: Dialer-Go

> [!IMPORTANT]
> **DOCUMENTO CANÔNICO UNIFICADO:**
> Conforme o protocolo **Bootstrap Zero** (Seção 0.1 de `RULE[user_global]`) e a governança deste repositório, o documento canônico oficial de arquitetura reside e é mantido exclusivamente em:
> 👉 [`.agents/ARCHITECT.md`](file:///home/marcio/ominichat/dialer-go/.agents/ARCHITECT.md)

---

## Resumo Arquitetural & Cláusula Pétrea de Autonomia

> [!CAUTION]
> **REGRA INVIOLÁVEL DE ESCOPO E AUTONOMIA ABSOLUTA:**
> **NUNCA ALTERAR CÓDIGO DE OUTROS SISTEMAS! O DIALER-GO É UM SISTEMA 100% AUTÔNOMO, NÃO TEM QUE SE METER COM OUTROS SISTEMAS!**
> 
> O Dialer-Go é um motor soberano e agnóstico de telefonia ("caixa-preta"). Ele **JAMAIS** altera, refatora, edita, compila ou adiciona arquivos em repositórios de outros sistemas (como o repositório do OmniChat, CRMs, chat-frontends ou serviços satélites).
> 
> Toda e qualquer interação com sistemas externos ocorre **exclusivamente através de contratos de API REST padronizados (RFC 7807)** e **Webhooks tipados de saída**. Qualquer plataforma consumidora obtém dados, desfechos e gravações consumindo as APIs canônicas do Dialer-Go (`/api/v1/cdrs`, etc.).

---

Para a especificação completa e exaustiva:
- 👉 [Documento de Arquitetura Mestre Canônico: `.agents/ARCHITECT.md`](file:///home/marcio/ominichat/dialer-go/.agents/ARCHITECT.md)
- 👉 [Manual de Arquitetura Operacional Exaustivo (2.000+ linhas): `.agents/ARCHITECTURE.md`](file:///home/marcio/ominichat/dialer-go/.agents/ARCHITECTURE.md)
- 👉 [Catálogo Oficial de Endpoints & RFC 7807: `.agents/workflows/API_REFERENCE.md`](file:///home/marcio/ominichat/dialer-go/.agents/workflows/API_REFERENCE.md)
- 👉 [Índice do Repositório & Grafo de Componentes: `docs/MAP.md`](file:///home/marcio/ominichat/dialer-go/docs/MAP.md)
- 👉 [Regras Gerais e Governança de Agentes: `.agents/AGENTS.md`](file:///home/marcio/ominichat/dialer-go/.agents/AGENTS.md)
