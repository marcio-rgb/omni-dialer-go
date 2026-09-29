# Arquitetura Mestre do Projeto: Dialer-Go

Documento Mestre Canônico de Arquitetura, Invariantes, Grafo de Execução e Diretrizes de Governança do subsistema **Dialer-Go** (`go 1.25.0`), regido pelas diretrizes canônicas do protocolo **Bootstrap Zero** (conforme Seção 0.1 de `RULE[user_global]`).

> [!CAUTION]
> **REGRA INVIOLÁVEL DE ESCOPO E AUTONOMIA ABSOLUTA:**
> **NUNCA ALTERAR CÓDIGO DE OUTROS SISTEMAS! O DIALER-GO É UM SISTEMA 100% AUTÔNOMO, NÃO TEM QUE SE METER COM OUTROS SISTEMAS!**
> 
> O Dialer-Go é um motor soberano e agnóstico de telefonia ("caixa-preta"). Ele **JAMAIS** altera, refatora, edita, compila ou adiciona arquivos em repositórios de outros sistemas (como o repositório do OmniChat, CRMs, chat-frontends ou serviços satélites).
> 
> Toda e qualquer interação com sistemas externos — incluindo o OmniChat — ocorre **exclusivamente através de contratos de API REST padronizados (RFC 7807)** e **Webhooks tipados de saída**. Qualquer plataforma que precise consumir dados, desfechos ou gravações de chamadas deve obrigatoriamente consumir as APIs canônicas do Dialer-Go (`/api/v1/cdrs`, etc.).

> [!NOTE]
> Este arquivo é a **ÚNICA FONTE CANÔNICA DE VERDADE** da arquitetura do Dialer-Go. Para aprofundamento operacional e contratos de rotas:
> - 👉 [Manual de Arquitetura Operacional Exaustivo (2.000+ linhas): `.agents/ARCHITECTURE.md`](file:///home/marcio/ominichat/dialer-go/.agents/ARCHITECTURE.md)
> - 👉 [Arquitetura de Filas Preditivas, LiveKit & Estados: `docs/rules/PREDICTIVE_QUEUES_LIVEKIT.md`](file:///home/marcio/ecosystem/dialer-go/docs/rules/PREDICTIVE_QUEUES_LIVEKIT.md)
> - 👉 [Catálogo Oficial de Endpoints & RFC 7807: `.agents/workflows/API_REFERENCE.md`](file:///home/marcio/ominichat/dialer-go/.agents/workflows/API_REFERENCE.md)
> - 👉 [Índice do Repositório & Grafo de Componentes: `docs/MAP.md`](file:///home/marcio/ominichat/dialer-go/docs/MAP.md)
> - 👉 [Regras Gerais e Governança de Agentes: `.agents/AGENTS.md`](file:///home/marcio/ominichat/dialer-go/.agents/AGENTS.md)

---

## 1. Responsabilidade Unívoca

O **Dialer-Go** é o orquestrador e motor central de alta performance para sinalização, comutação e controle de chamadas telefônicas em **Go puro (Go 1.25)**, responsável por:
1. **Discagem Multimodelo:**
   - **Preditiva:** Algoritmo de pacing em tempo real com auto-balanceamento de overdialing baseado em agentes disponíveis, TMA, TMR e taxa de contato.
   - **Manual:** Atendimento a operadores humanos com preempção prioritária sobre campanhas automatizadas.
   - **Receptiva:** Lookup em sub-milissegundo $O(1)$ para direcionamento inteligente de chamadas entrantes via tabela indexada `phone_trunk_mappings`.
2. **Controle de Concorrência & Troncos:**
   - Gestão thread-safe de capacidade global e granular de canais por tronco SIP/PJSIP via `sync/atomic.Int32`.
   - Hot-reload dinâmico de troncos SIP sem perda de chamadas ativas.
3. **Telefonia de Baixa Latência:**
   - Conexão persistente via socket TCP nativo AMI (:5038) com o Asterisk PBX 20 LTS.
4. **Triagem Ultrarrápida de Atendimento Humano vs. Máquina (Subsistema Classificator):**
   - Triagem ativa full-duplex de voz via script EAGI Thin Client no Asterisk comunicando-se com o `classificator-router` (:2800) em anel circular de 3 portas (:2801, :2802, :2803), entregando a chamada ao operador em menos de 1,5 segundo com suporte a hot-reload sem downtime no meio da operação.
5. **Comutação e Destino de Áudio:**
   - Comutação via protocolo SIP para operadores humanos e salas de agentes virtuais de voz (LiveKit).
6. **Soberania do Histórico, Transcrições e Gravações:**
   - Persistência atômica de CDRs com transcrição de fala em tempo real (`transcription`) via Vosk STT, caminhos físicos (`recording_file`) e URLs de streaming (`recording_url`) na tabela `cdrs` do `dialer_db` com suporte a busca textual integrada.

### O que o Dialer-Go NÃO faz:
- ❌ **NÃO altera, modifica ou edita código de outros sistemas:** Proibido tocar no código do OmniChat, CRMs, bots ou outras aplicações.
- ❌ **NÃO processa regras de CRM, funis de venda ou cadastro de clientes:** Exclusividade dos sistemas clientes externos.
- ❌ **NÃO executa raciocínio de LLM ou síntese de voz (TTS) de agentes conversacionais:** Responsabilidade do Estúdio IA / LiveKit Agents.
- ❌ **NÃO gerencia servidores WebRTC de borda de mídia:** Responsabilidade do LiveKit Server.
- ❌ **NÃO transcreve conversas completas pós-atendimento:** O Vosk STT via EAGI atua na triagem inicial e na captura da fala de abertura, persistindo no CDR para facilitar auditorias preditivas imediatas.
- ❌ **NÃO depende de bancos ou tabelas externas:** Proibição absoluta de utilizar `call_history` ou qualquer estrutura de banco externa ao `dialer_db`.

---

## 2. Invariantes Arquiteturais (Regras Inquebráveis)

1. **Autonomia Soberana e Isolamento Absoluto (Cláusula Pétrea):**
   - O Dialer-Go é um motor autônomo, agnóstico e independente. Nunca altera código de terceiros e nunca delega sua persistência a tabelas alheias. O ecossistema cliente interage exclusivamente via API REST e Webhooks.
2. **Linguagem Única e Binário Estático (100% Go):**
   - Sem runtimes externos (Node.js, Python, Ruby) no discador. Binário estático Go 1.25 com baixo consumo de memória e CPU sob alta concorrência.
3. **Limite Rígido de 500 Linhas por Arquivo:**
   - Nenhum arquivo `.go` ultrapassa 500 linhas. Ao atingir 350 linhas, o plano de segregação modular deve ser acionado imediatamente.
4. **Tipagem Estrita e Validação de Bordas:**
   - Proibição absoluta de `interface{}` / `any`. Structs tipadas em `internal/domain/`. Respostas de erro no padrão RFC 7807 / RFC 9457 (`ProblemDetails`).
5. **Fronteira de Segurança por IP Whitelist em Memória:**
   - Autorização de requisições REST internas via lookup thread-safe em memória (`sync.Map`), operando em tempo sub-microssegundo.
6. **Persistência Standalone Exclusiva (`dialer_db`):**
   - Banco relacional PostgreSQL isolado exclusivamente para a telefonia, evitando contenção de locks com sistemas consumidores.
7. **Lookup Receptivo $O(1)$ em Tabela Auxiliar Indexada:**
   - Chamadas entrantes consultam exclusivamente a tabela indexada B-Tree `phone_trunk_mappings`, sem varreduras pesadas em tabelas de CDR.
8. **Zero Normalização de Dígitos Telefônicos no Discador:**
   - O discador preserva estritamente os dígitos do número (`10`, `11`, `12`, `13`, `14` dígitos). Prefixos de operadora são aplicados via parametrização do tronco (`tech_prefix`).
9. **Quota de Reserva de Canais Humanos (10 Canais):**
   - O motor preditivo jamais consome toda a capacidade. No mínimo 10 canais globais permanecem rigidamente reservados para chamadas manuais (`HumanReserveQuota`).
10. **Abandono Regulatório Estrito (< 2 Segundos):**
    - Chamadas atendidas sem operador ou sala disponível em até 2 segundos são desligadas com motivo regulatório (`ABANDONED`) e registradas em auditoria.
11. **Rastreabilidade Ponta a Ponta com CorrelationID:**
    - Toda chamada recebe um `correlation_id` único propagado até a tabela `execution_traces`, permitindo auditoria ponta a ponta e detecção de quebra de processo.
12. **Soberania Absoluta de CDRs, Transcrições e Gravações de Áudio:**
    - Toda chamada, metadados de tarifação, desfechos, transcrições em tempo real (`transcription`) e os links de áudio gravado (`recording_file` e `recording_url`) residem nativamente na tabela `cdrs` do `dialer_db` e são consultados via `GET /api/v1/cdrs` (com suporte a busca textual por `q` / `search`) e `GET /api/v1/cdrs/{id}`.
13. **Proibição Absoluta de Edição Direta de Arquivos no Servidor (Gestão Soberana via `sip_data`):**
    - É terminantemente proibido editar ou criar arquivos de configuração do Asterisk (`pjsip.conf`, `extensions.conf`, `amd.conf`, etc.) diretamente via acesso SSH/SCP ou manipulação manual de arquivos no servidor.
    - Todo o fluxo de configuração DEVE obrigatoriamente passar pela tabela `sip_data` no PostgreSQL (`file VARCHAR(60)` e `data TEXT`) por meio da API REST (`/api/v1/configs`).
    - Somente no momento em que a ação de "Aplicar" for explicitamente acionada (`?apply=true` ou `POST /api/v1/configs/apply`), o sistema escreve os arquivos físicos no diretório `/etc/asterisk` e executa os reloads no PBX via AMI.
14. **Arquitetura de Filas Nativas Asterisk (`app_queue`), Music On Hold & Multi-Tenancy:**
    - Toda distribuição preditiva opera via filas nativas do Asterisk nomeadas no padrão canônico `trim(tenant_id)-trim(campaign_id)` (ex: `default-12`).
    - A espera na fila executa nativamente a classe MOH `dialer_hold` (`audio_espera.wav` a 8000 Hz) e o anúncio `queue-youarenext` (`vc_e_o_proximo.wav` a 8000 Hz).
    - A entrega de áudio aos operadores ocorre com latência zero via canais `Local/<room>@livekit-agent-queue/n` com opção `m(dialer_hold)` para continuidade do áudio de fundo durante o ringing do LiveKit.

---

## 3. Padrões de Design Patterns Adotados

| Subsistema / Domínio | Padrão Adotado | Justificativa Arquitetural | Localização |
| :--- | :--- | :--- | :--- |
| **Motores de Discagem** | **Strategy Pattern** | Permite alternar dinamicamente entre as estratégias (`PredictiveEngine`, `ManualEngine`, `InboundEngine`) respeitando contratos unificados de ciclo de vida. | `internal/core/` |
| **Comunicação Telefônica (AMI)** | **Adapter Pattern** | Isola o protocolo TCP puro do Asterisk Manager Interface atrás do contrato canônico `ports.AMIPort`. | `internal/adapters/ami/` |
| **Persistência Relacional** | **Repository Pattern** | Segrega as queries SQL do PostgreSQL atrás de interfaces canônicas (`TrunkRepository`, `LeadRepository`, `ReportRepository`, etc.). | `internal/adapters/postgres/` |
| **Eventos de Telefonia** | **Observer / Pub-Sub** | O leitor de socket AMI distribui eventos assíncronos (`AsyncAGI`, `UserEvent`, `Hangup`, `Newchannel`) via canais Go desacoplados. | `internal/adapters/ami/` |
| **Controle de Acesso HTTP** | **Middleware (Chain of Resp.)** | `IPWhitelistMiddleware` intercepta as requisições Chi antes de qualquer processamento, validando IPs em sub-microssegundo. | `internal/adapters/http/` |
| **Resiliência de Rede** | **Circuit Breaker / Retry Backoff** | Conexões com PostgreSQL e AMI implementam retentativas com backoff exponencial para absorver reinicializações de infraestrutura sem panics. | `internal/adapters/{ami, postgres}` |
| **Atomicidade de Mailing** | **Transactional Outbox / Stored Procedures ACID** | Stored procedures PostgreSQL (`fn_audit_claim_predictive_batch`, `fn_audit_persist_predictive_result`) garantem consistência transacional sob concorrência massiva. | `database/schema.sql` |
| **Notificação de Desfecho** | **Adapter & Observer Pattern** | Despacha eventos assíncronos de término e falha de chamada via Webhook HTTP com timeout estrito de 5s para o upstream. | `internal/core/call_notifier.go` |
| **Gestão de Configuração Asterisk** | **Configuration Manager / Repository** | Gerencia arquivos de configuração (`pjsip.conf`, `extensions.conf`) via tabela `sip_data`, sincronizando com disco e enviando reloads via AMI. | `internal/core/sip_config_manager.go` |
| **Provisionamento LiveKit SIP** | **Adapter Pattern** | Orquestra auto-provisionamento idempotente de Inbound Trunk e Dispatch Rule via API Twirp, auditando integridade no Healthcheck. | `internal/adapters/livekit/` |

---

## 4. Tech Stack e Runtimes

- **Arquitetura Base:** Arquitetura Hexagonal (Ports and Adapters) com Clean Architecture segregada em `domain`, `ports`, `core` e `adapters`.
- **Linguagem e Runtime:** Go 1.25 (`go 1.25.0 linux/amd64`).
- **Roteamento HTTP:** Chi Router v5 (`github.com/go-chi/chi/v5`).
- **Central Telefônica:** Asterisk PBX v20 LTS (AMI TCP Socket porta :5038, Dialplan `extensions.conf`, PJSIP).
- **Mecanismo de STT / AMD:** 
  - Vosk Speech-to-Text (`alphacep/kaldi-vosk-server:latest`) via WebSocket RFC 6455 em script EAGI Go (`cmd/vosk-eagi`) na porta :2700.
  - Subsistema Classificador Neural via AudioSocket TCP nativo (:9092) em Go puro com envelopes binários de 16 bytes (RFC 4122) e pipeline assíncrono Faster-Whisper.
- **Persistência Relacional:** PostgreSQL 16 Alpine (`pgx/v5` connection pool com pooling transacional, busca GIN FTS e índices com `varchar_pattern_ops`).
- **Cache Rápido, Filas & Event Streaming:** Redis 7 Alpine (`github.com/redis/go-redis/v9`) com suporte a Redis Streams (`dialer:stream:cdr_events`, `dialer:stream:transcription_jobs`) e Consumer Groups.
- **Armazenamento de Objetos (Mailings):** MinIO S3 API (`github.com/minio/minio-go/v7`).
- **Síntese de Nomes Prévia (TTS Offline):** Piper TTS pt-BR CLI (`internal/adapters/tts/piper_adapter.go`).
- **Controle de Versão (VCS):** Git Forgejo interno (NodePort 30080 HTTP / 30222 SSH).
- **Orquestrador de Infraestrutura:** K3s Kubernetes (Rancher) gerenciado via Control Plane API (`http://207.180.251.25:3100`) e Secret Vault (AES-256-GCM). Artefatos e deploys legados via Portainer/Swarm estão formalmente proibidos e descontinuados.

---

## 4.1. Arquitetura de Orquestração, Topologia & Networking em Kubernetes (K3s)

O ecossistema é orquestrado de forma heterogênea sobre um cluster Kubernetes leve (**K3s**) distribuído em topologia multi-node especializada por classe de carga de trabalho (Workload Partitioning), garantindo isolamento estrito de falhas e ultra-baixa latência para os fluxos telefônicos em tempo real.

### 4.1.1. Topologia de Nós e Alocação Determinística de Cargas:
O cluster K3s é particionado em 4 nós físicos com papéis bem definidos:
1. **Node Master (`node-master` — VPS `84.247.135.255`):**
   - **Papel:** Núcleo de Telefonia e Persistência Crítica em Tempo Real.
   - **Serviços Co-localizados:**
     - `dialer-go` (Engine de discagem em Go 1.25, porta :8080)
     - `asterisk-pbx` (PBX Core Asterisk 20 LTS com chan_pjsip, app_queue e AMI :5038)
     - `livekit-sip` (SIP Gateway para WebRTC, porta :5060/:5062)
     - `postgres` (PostgreSQL 16 com pgvector e banco soberano `dialer_db`, porta :5432)
     - `redis` (Redis 7 de alta velocidade para telemetria, locks e streams, porta :6379)
     - `cloudbeaver` (Gestão visual de banco de dados, porta :8978)
   - **Justificativa Arquitetural da Co-locação:**
     A telefonia SIP/RTP e a sinalização AMI não toleram latências inter-máquinas (que introduzem jitter, delay perceptível no atendimento humano e risco de quebra do SLA regulatório de 2 segundos de abandono). A co-locação física no Node Master garante que o tráfego de controle (AMI :5038), streaming de áudio (AudioSocket/EAGI) e consultas de cache/banco ocorram em **loopback de kernel com latência sub-milissegundo (< 0.5ms)**.

2. **Node Worker 1 (`node-worker-1`):**
   - **Papel:** Gateways de Comunicação Externa, Clientes Web e Storage.
   - **Serviços:** `omnichat-client` (Frontend Vite/React :5173), `omnichat-server` (Backend Node.js :3000), `whatsapp-go` (Gateway Baileys/Go :4000), `livekit-server` (Servidor WebRTC SFU :7880/:7882) e `minio` (S3 Object Storage :9000).

3. **Node Worker 2 (`node-worker-2`):**
   - **Papel:** Inteligência Artificial de Mídia e Processamento Pesado de CPU/GPU.
   - **Serviços:** `estudio-ia` (Hub FastAPI :8080/:8384), `faster-whisper` (STT neural :8000), `piper-tts` (Síntese neural :5000), `playwright-server` (Headless browser grid :3000) e `kubernetes-dashboard` (:8443).

4. **Node Worker 3 (`node-worker-3`):**
   - **Papel:** Ambiente de Desenvolvimento Remoto e Ferramental.
   - **Serviços:** `coder` (Plataforma Coder v2 Web IDE :8080).

### 4.1.2. Estratégia de Rede, Ingress e hostNetwork:
A pilha de rede no K3s adota uma abordagem híbrida de alta performance:
- **`hostNetwork: true` para Telefonia de Borda (`asterisk-pbx` e `livekit-sip`):**
  - **Motivo Técnico:** O tráfego de mídia telefônica em tempo real envolve milhares de portas UDP efêmeras (RTP range 10000–20000) e sinalização SIP (UDP/TCP 5060). Encaminhar esse volume maciço de pacotes UDP por meio de tabelas de conntrack do iptables/Kube-Proxy no Kubernetes causaria exaustão de conexões, perda de pacotes e latência inaceitável. O `hostNetwork: true` faz o processo escutar diretamente na interface de rede física da VPS `84.247.135.255`, eliminando overhead de NAT.
- **`hostNetwork: false` com Service e Ingress para o `dialer-go` (:8080):**
  - O Dialer-Go opera isolado em rede de pod (`ClusterIP`), expondo internamente no cluster o DNS `dialer-go.omnichat.svc.cluster.local:8080`.
  - O acesso público/externo é roteado via Ingress Controller (Traefik) sob o domínio seguro HTTPS `https://dialer.cbrpromotora.com`, com terminação TLS automática e proteção na camada de aplicação via `IPWhitelistMiddleware`.

### 4.1.3. Resolução de DNS Interno no Namespace `omnichat`:
Todo o tráfego interno entre os microsserviços trafega por nomes de serviço canônicos fornecidos pelo CoreDNS do K3s:
| Serviço Destino | DNS Canônico Interno | Porta | Protocolo |
| :--- | :--- | :--- | :--- |
| **PostgreSQL** | `postgres.omnichat.svc.cluster.local` | 5432 | PostgreSQL Wire |
| **Redis** | `redis.omnichat.svc.cluster.local` | 6379 | RESP |
| **Asterisk PBX** | `asterisk-pbx.omnichat.svc.cluster.local` | 80 / 5038 | HTTP / AMI TCP |
| **Dialer-Go** | `dialer-go.omnichat.svc.cluster.local` | 8080 | HTTP REST RFC 7807 |
| **LiveKit Server** | `livekit-server.omnichat.svc.cluster.local` | 7880 | WebRTC / HTTP |
| **Faster-Whisper** | `faster-whisper.omnichat.svc.cluster.local` | 8000 | HTTP REST |
| **MinIO S3** | `minio.omnichat.svc.cluster.local` | 9000 | S3 REST API |

### 4.1.4. Governança e Deploy via Control Plane (`http://207.180.251.25:3100`):
Nenhum deploy é executado diretamente via kubectl ou ferramentas manuais. O fluxo é 100% governado e auditado:
1. **Push no Git Forgejo:** Commits validados entram na branch `main` do repositório interno.
2. **Ordem de Deploy Declarativa:** Chamada HTTP para a API do Control Plane:
   ```http
   POST http://207.180.251.25:3100/api/v1/deploys
   Content-Type: application/json

   {
     "serviceId": "dialer-go",
     "environment": "production",
     "imageTag": "latest"
   }
   ```
3. **Injeção Dinâmica de Segredos:** O Control Plane decodifica os segredos do Vault (AES-256-GCM) e os injeta como variáveis de ambiente em tempo de inicialização do Pod, sem gravação de segredos em arquivos no disco.
4. **Reconciliação e Healthcheck:** O Control Plane aguarda a inicialização do container, monitora a liveness probe (`/health`) e confirma a transição para o estado `HEALTHY`. Caso ocorra falha, o rollback é disparado imediatamente.

## 5. Grafo de Dependências

```mermaid
flowchart TD
    subgraph Upstream["Upstream (Consumidores Externos - Via REST / Webhooks)"]
        OmniChat["OmniChat (Operadores Humanos)"]
        EstudioIA["Estúdio IA (Agentes Virtuais LiveKit)"]
        CronTrigger["Cron / Automações de Campanha"]
        ExternalCRM["Sistemas Externos / CRMs"]
    end

    subgraph DialerCore["Dialer-Go Core (:8080) - Motor Autônomo"]
        RouterHTTP["Chi HTTP Router & IP Whitelist"]
        EnginePred["PredictiveEngine"]
        EngineMan["ManualEngine"]
        EngineInb["InboundEngine"]
        ChanMgr["ChannelManager (sync/atomic.Int32)"]
        TrunkMgr["TrunkManager (PJSIP Pool & Audio Resolver)"]
        ReportHandler["ReportHandler (/api/v1/cdrs)"]
    end

    subgraph Downstream["Downstream (Infraestrutura & Serviços)"]
        AsteriskAMI["Asterisk PBX AMI (TCP :5038)"]
        ClassificatorRouter["Classificator Router (WS :2800)"]
        ClassificatorRing["Classificator Engine Ring (:2801, :2802, :2803)"]
        VoskASR["Vosk STT Server (WS :2700)"]
        PostgresDB[("PostgreSQL dialer_db (:5432)")]
        RedisCache[("Redis dialer_cache (:6379)")]
        MinIOS3[("MinIO S3 mailings (:9000)")]
        TelcoPJSIP["Operadoras Telefônicas (PJSIP SIP Trunks)"]
        LiveKitSIP["LiveKit SIP Dispatcher"]
    end

    OmniChat -->|POST /api/v1/calls/manual| RouterHTTP
    EstudioIA -->|POST /api/v1/predictive/demand| RouterHTTP
    CronTrigger -->|POST /api/v1/campaigns/refill| RouterHTTP
    ExternalCRM -->|GET /api/v1/cdrs| RouterHTTP

    RouterHTTP --> EnginePred & EngineMan & EngineInb & ReportHandler
    EnginePred & EngineMan & EngineInb <--> ChanMgr
    EnginePred & EngineMan <--> TrunkMgr

    ChanMgr <--> RedisCache
    TrunkMgr <--> PostgresDB
    ReportHandler <--> PostgresDB
    EnginePred & EngineMan <--> PostgresDB
    EnginePred & EngineMan -->|Originate / Redirect / Hangup| AsteriskAMI

    AsteriskAMI <-->|EAGI FD 3 Audio| ClassificatorRouter
    ClassificatorRouter <-->|Fast-Path O(1)| ClassificatorRing
    ClassificatorRing <-->|Kaldi WS| VoskASR
    AsteriskAMI <-->|SIP INVITE| TelcoPJSIP
    AsteriskAMI <-->|SIP Transfer| LiveKitSIP
    RouterHTTP <--> MinIOS3
```

### Eventos Produzidos / Consumidos:
- **Consumidos do Asterisk AMI:** `AsyncAGIStart`, `AsyncAGIExec`, `UserEvent` (`PredictiveHuman`, `InboundCall`), `Hangup`, `Newchannel`, `Newstate`.
- **Disparados para o Asterisk AMI:** `Originate`, `Redirect`, `Hangup`, `PJSIPShowEndpoints`, `Command` (`pjsip reload`).
- **Persistência e Auditoria:** Registros em `execution_traces`, persistência atômica com áudio em `cdrs` (`recording_file`, `recording_url`) e disponibilização via endpoint `/api/v1/cdrs`.

---

## 6. Master Route (Fluxos Principais de Execução)

### 6.1. Rota Preditiva (`POST /api/v1/predictive/demand`)
1. **Entrada HTTP:** `httpAdapter.PredictiveHandler.HandleDemand` interceptado por `IPWhitelistMiddleware`.
2. **Validação de Contrato:** Parsing e validação da struct `domain.PredictiveDemandRequest`.
3. **Checagem de Estado:** Verifica se campanha está pausada ou sem operadores disponíveis no Redis (`IsCampaignPaused`, `StoreAvailableAgents`).
4. **Cálculo de Pacing:** `PredictiveEngine.CalculateOverdialing` calcula número de canais a disparar baseado em operadores livres, TMR, TMA, probabilidade de contato e agressividade.
5. **Arbitragem de Slot:** `ChannelManager.CanAcquireSlot` valida capacidade global (`MaxGlobalChannels - HumanReserveQuota`) e capacidade por tronco.
6. **Reserva Atômica FILO:** `leadRepo.PopLead` invoca stored procedure ACID `fn_audit_claim_predictive_batch` para reservar leads elegíveis.
7. **Disparo Telefônico AMI:** `AMIPort.Originate` envia ação para o Asterisk no contexto de triagem AMD (`from-dialer-amd`).
8. **Triagem de Voz AMD Híbrida:** Asterisk executa `app_amd`. Se inconclusivo/fala longa, aciona `vosk-eagi` via streaming WebSocket.
9. **Entrega de Chamada:** Ao confirmar atendimento humano (`PredictiveHuman`), Asterisk AMI executa `Redirect` para a sala do operador ou sala LiveKit (`AGENT_ROOM`).
10. **Persistência Pós-Execução:** Na desconexão (`Hangup`), `TrunkManager` vincula o áudio gravado e `ReportRepo.SaveCDR` / `fn_audit_persist_predictive_result` gravam o CDR com caminho físico e URL de streaming na tabela `cdrs`.

### 6.2. Rota Manual (`POST /api/v1/calls/manual`)
1. **Entrada HTTP:** `httpAdapter.ManualHandler.HandleCall` validado por IP Whitelist.
2. **Validação de Contrato:** Validação de `domain.ManualCallRequest` (número de telefone, operador, rota SIP de destino).
3. **Prioridade Preemptiva:** `ChannelManager.AcquireSlot` consome prioritariamente a quota reservada (`HumanReserveQuota`), garantindo que operadores manuais nunca sofram bloqueio por campanhas automáticas.
4. **Disparo Telefônico:** `AMIPort.Originate` conecta diretamente o operador ao cliente via contexto `manual-outbound` com injeção de pre-dial headers.
5. **Auditoria:** Gravação de áudio com `MixMonitor` e log com `correlation_id` em `execution_traces`.

### 6.3. Rota Receptiva (Inbound Telephony)
1. **Entrada Telefônica:** Chamada externa atinge o Asterisk no contexto `[receptivo]`.
2. **Gatilho de Evento AMI:** Dialplan dispara `UserEvent(InboundCall, Channel, Phone, DID, Uniqueid)`.
3. **Lookup $O(1)$:** `InboundEngine` consulta `phone_trunk_mappings` buscando `last_trunk` e `last_project`.
4. **Decisão de Roteamento:** Se encontrado retorno, comuta para a rota SIP prévia do cliente; caso contrário, direciona para o tronco receptivo padrão.
5. **Comutação Asterisk:** AMI despacha `Redirect` para o contexto `cos-inbound` com entrega sem fila residual.

### 6.4. Rota Canônica de Consulta, Transcrições e Áudios (`GET /api/v1/cdrs` e `GET /api/v1/reports/cdrs`)
1. **Entrada e Parsing Resiliente:** Requisição HTTP interceptada por `IPWhitelistMiddleware`. Extração e validação de parâmetros delegada ao `ParseCDRFilter` (`report_filter_parser.go`):
   - Suporte a 18 parâmetros de Contact Center (`agent_id`, `call_type`, `trunk_used`, `lead_cpf`, `lead_id`, `amd_status`, `has_recording`, `min/max_billsec`, `min/max_duration`, `disposition` singular e em CSV/array múltiplo, `sort_by`, `sort_order`, `include_total`).
   - Normalização flexível de datas (`parseFlexibleDate`) aceitando ISO 8601 UTC (`RFC3339`/`RFC3339Nano`) e `DateOnly` (`YYYY-MM-DD` com expansão determinística para `00:00:00` no início e `23:59:59.999` no fim).
   - Validação de segurança temporal: teto rígido de 31 dias para buscas genéricas e 90 dias quando acompanhada de filtros seletivos (`phone`, `lead_cpf`, `lead_id`, `agent_id`).
   - Respostas de validação estritamente padronizadas no formato Problem Details (RFC 7807 / RFC 9457).
2. **Busca Inteligente (Discriminator Pattern):**
   - O `buildSmartSearchClause` (`report_repo_search.go`) analisa a expressão de busca (`q` / `search`).
   - **Termos Numéricos (Telefone / CPF):** Normaliza removendo formatações (`()-.+ `). Se possuir $\ge 4$ dígitos, desvia para índice B-Tree cobridor com `varchar_pattern_ops` (`idx_cdrs_tenant_phone`), aplicando busca por prefixo para $< 10$ dígitos e igualdade com suporte a DDI 55 para números completos.
   - **Termos Textuais:** Desvia para busca morfológica e semântica sobre o índice GIN `idx_cdrs_transcription_search` utilizando `websearch_to_tsquery('portuguese', $N)`, permitindo frases literais entre aspas (`"atendimento humano"`) e operadores lógicos sem risco de crash de sintaxe.
3. **Paginação Escalável via Overfetch (`limit + 1`) & Total Opcional:**
   - Para evitar o bug clássico de "página fantasma" sem onerar o banco, a query executa com `LIMIT limit + 1 OFFSET offset`. Se retornar mais de `limit` registros, remove o excedente em memória e crava `has_more = true`; caso contrário, `has_more = false`.
   - Se `include_total=false`, o cálculo caro de `COUNT(*)` é completamente ignorado pelo banco, retornando `total: null` e entregando tempo de resposta sub-10ms mesmo em tabelas com milhões de linhas.
4. **Retorno Enriquecido Soberano:**
   - Entrega dados completos de tarifação, durações, desfecho, transcrição da fala (`transcription`), identificadores de lead (`lead_id`, `lead_name`, `lead_cpf`), status de AMD e os links completos para streaming de áudio segregado estéreo (`recording_file`, `recording_url`).
