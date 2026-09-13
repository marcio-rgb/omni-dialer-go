# Dialer-Go

> **Subsolo e Motor Central de Telefonia Preditiva, Receptiva e Manual da Plataforma OmniChat**  
> **Linguagem:** 100% Go (`go 1.25.0`) | **Central Telefônica:** Asterisk 20 LTS (AMI TCP + PJSIP) | **Mídia/IA:** LiveKit WebRTC & Vosk STT

---

## 1. Visão Geral

O **Dialer-Go** é o subsistema de telefonia de alta performance e baixa latência projetado para executar discagem automatizada em escala, garantindo:
- **Triagem AMD Ultrarrápida (< 1,5s):** Combinação do módulo Asterisk `app_amd` com transcrição de áudio em tempo real via script EAGI Vosk STT (`cmd/vosk-eagi`).
- **Pacing Preditivo Auto-balanceado:** Overdialing matemático dinâmico que mantém ocupação de agentes humanos e virtuais prevenindo abandono regulatório (< 2s).
- **Lookup Receptivo O(1):** Roteamento em sub-milissegundo de chamadas entrantes via tabela indexada `phone_trunk_mappings`.
- **Salvaguarda de Operadores Humanos:** Quota rígida de 10 canais globais reservados para chamadas manuais (`HumanReserveQuota`).
- **Segurança Sub-microssegundo:** Autorização REST baseada em tabela hash em memória (`sync.Map`) via IP Whitelist, sem tokens ou handshakes pesados.
- **Isolamento Total:** Base relacional standalone (`dialer_db`), blindando o discador contra concorrência de banco com o CRM.

---

## 2. Arquitetura do Sistema

O projeto adota os preceitos da **Arquitetura Hexagonal (Ports and Adapters)** e **Clean Architecture**:

```text
dialer-go/
├── cmd/
│   ├── dialer/              # Ponto de entrada do serviço HTTP e orquestrador
│   └── vosk-eagi/           # Binário EAGI executado pelo Asterisk para triagem Vosk STT
├── config/                  # Carregamento e validação de variáveis de ambiente
├── database/                # Schema DDL (tabelas e stored procedures ACID)
├── docs/
│   ├── MAP.md               # Grafo e índice completo do repositório
│   └── rules/               # Políticas de negócio e especificações funcionais
├── internal/
│   ├── domain/              # Entidades puras, DTOs, Enums e erros RFC 7807
│   ├── ports/               # Interfaces canônicas (AMI, Cache, Repositories, Storage)
│   ├── core/                # Regras de negócio, motores e gerenciador de canais
│   └── adapters/            # Implementações concretas (HTTP Chi, PostgreSQL, Redis, AMI, MinIO)
├── ARCHITECT.md             # Documento Mestre de Arquitetura e Invariantes
├── GEMINI.md                # Regras Absolutas de Engenharia para IAs
└── extensions.conf          # Dialplan Asterisk com contexts de triagem e pre-dials
```

---

## 3. Matriz de Endpoints REST

Todas as rotas exigem IP de origem cadastrado no `IPWhitelistMiddleware`.

| Método | Rota | Descrição | Governança |
| :--- | :--- | :--- | :--- |
| `POST` | `/api/v1/predictive/demand` | Processa rodada de pacing preditivo para agentes livres. | [`TELEPHONY_POLICIES.md`](file:///home/marcio/ominichat/dialer-go/docs/rules/TELEPHONY_POLICIES.md) |
| `POST` | `/api/v1/calls/manual` | Dispara chamada manual sob demanda para operador humano. | [`TELEPHONY_POLICIES.md`](file:///home/marcio/ominichat/dialer-go/docs/rules/TELEPHONY_POLICIES.md) |
| `POST` | `/api/v1/campaigns/refill` | Ingestão e carga de novo arquivo de mailing via MinIO S3. | [`CAMPAIGN_SATURATION.md`](file:///home/marcio/ominichat/dialer-go/docs/rules/CAMPAIGN_SATURATION.md) |
| `POST` | `/api/v1/campaigns/toggle` | Pausa ou retoma o fluxo de discagem de uma campanha. | [`CAMPAIGN_SATURATION.md`](file:///home/marcio/ominichat/dialer-go/docs/rules/CAMPAIGN_SATURATION.md) |
| `GET` | `/api/v1/campaigns/{id}/saturation` | Retorna o nível de saturação e penetração da campanha. | [`CAMPAIGN_SATURATION.md`](file:///home/marcio/ominichat/dialer-go/docs/rules/CAMPAIGN_SATURATION.md) |
| `GET` | `/api/v1/campaigns/saturation` | Retorna saturação consolidada de todas as campanhas ativas. | [`CAMPAIGN_SATURATION.md`](file:///home/marcio/ominichat/dialer-go/docs/rules/CAMPAIGN_SATURATION.md) |
| `GET` | `/api/v1/campaigns/{id}/reports/operational` | Métricas de chamadas com cache Redis de 15 minutos. | [`CAMPAIGN_SATURATION.md`](file:///home/marcio/ominichat/dialer-go/docs/rules/CAMPAIGN_SATURATION.md) |
| `GET` | `/api/v1/trunks` | Lista troncos com telemetria e qualify em tempo real via AMI. | [`TRUNKS_LIFECYCLE.md`](file:///home/marcio/ominichat/dialer-go/docs/rules/TRUNKS_LIFECYCLE.md) |
| `POST` | `/api/v1/trunks` | Cadastra tronco e aplica hot-reload no Asterisk PJSIP. | [`TRUNKS_LIFECYCLE.md`](file:///home/marcio/ominichat/dialer-go/docs/rules/TRUNKS_LIFECYCLE.md) |
| `PUT` | `/api/v1/trunks/{id}` | Atualiza parâmetros do tronco e reaplica hot-reload. | [`TRUNKS_LIFECYCLE.md`](file:///home/marcio/ominichat/dialer-go/docs/rules/TRUNKS_LIFECYCLE.md) |
| `DELETE`| `/api/v1/trunks/{id}` | Exclui tronco com salvaguarda contra chamadas ativas (`409`). | [`TRUNKS_LIFECYCLE.md`](file:///home/marcio/ominichat/dialer-go/docs/rules/TRUNKS_LIFECYCLE.md) |
| `GET` | `/api/v1/health` | Telemetria e conectividade de PostgreSQL, Redis, AMI e Canais. | [`SECURITY_NETWORK.md`](file:///home/marcio/ominichat/dialer-go/docs/rules/SECURITY_NETWORK.md) |

---

## 4. Execução e Desenvolvimento Local

### 4.1. Inicialização do Ambiente com Docker Compose
Para subir a stack completa local (PostgreSQL, Redis, Vosk Server e Dialer-Go):

```bash
docker compose -f docker-compose.local.yml up -d --build
```

### 4.2. Execução dos Testes de Unidade
```bash
/snap/antigravity-cli/21/usr/lib/go-1.22/bin/go test -v ./...
```

### 4.3. Compilação dos Binários
```bash
# Binário do discador
/snap/antigravity-cli/21/usr/lib/go-1.22/bin/go build -o bin/dialer ./cmd/dialer

# Binário do script EAGI Vosk
/snap/antigravity-cli/21/usr/lib/go-1.22/bin/go build -o bin/vosk-eagi ./cmd/vosk-eagi
```

---

## 5. Documentação Adicional & Governança

- 👉 [**Documento Mestre de Arquitetura (`ARCHITECT.md`)**](file:///home/marcio/ominichat/dialer-go/ARCHITECT.md)
- 👉 [**Regras Absolutas de Engenharia para IAs (`GEMINI.md`)**](file:///home/marcio/ominichat/dialer-go/GEMINI.md)
- 👉 [**Índice e Grafo do Repositório (`docs/MAP.md`)**](file:///home/marcio/ominichat/dialer-go/docs/MAP.md)
- 👉 [**Diretório de Políticas de Negócio (`docs/rules/`)**](file:///home/marcio/ominichat/dialer-go/docs/rules/)
- 👉 [**Workflows de Agentes Especializados (`.agents/workflows/`)**](file:///home/marcio/ominichat/dialer-go/.agents/workflows/)
