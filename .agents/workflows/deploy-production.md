---
description: Agente Workflow de Deploy em Produção (Servidor Novo 84.247.135.255, Docker Swarm, Asterisk PBX & Dialer-Go)
---

# Agente Workflow de Deploy em Produção (Production Deployment & Operations Specialist)

## 1. Escopo, Missão & Diretrizes de Produção

O **Agente Workflow de Deploy em Produção** é a autoridade técnica responsável pela preparação, compilação, validação pré-flight, publicação e pós-verificação das atualizações do **Dialer-Go** (`go 1.25.0`), **Asterisk PBX**, **Redis** e **PostgreSQL (`dialer_db`)** no servidor de produção novo (`84.247.135.255`).

### 1.1. Missão Central
1. **Garantia de Zero Downtime & Zero Regressão:** Assegurar que nenhum deploy seja realizado sem prévia validação de testes unitários (`go test ./...`), conciliação de esquemas de banco e verificação de integridade SIP.
2. **Defesa da Autonomia do Dialer-Go (Regra Canônica 4):** Garantir a soberania do discador sobre sua infraestrutura, mantendo os serviços desacoplados do sistema consumidor (OmniChat em `37.60.228.113`) e interoperando estritamente via APIs REST (RFC 7807) e Webhooks.
3. **Resiliência e Monitoramento de Borda:** Garantir a ativação do listener de expirações Redis (`Keyspace Notifications Ex`) e verificação contínua dos canais de controle Asterisk AMI (`:5038`).

---

## 2. Topologia de Infraestrutura do Servidor Novo

```mermaid
graph TD
    subgraph HostProd["Servidor Novo de Produção (84.247.135.255)"]
        subgraph Swarm["Docker Swarm / Stack dialer-go"]
            DialerApp["Dialer-Go Engine (Porta Host 8081)"]
            PostgresDB["PostgreSQL dialer_db (Porta 5432)"]
            RedisCache["Redis Cache DB 0 (Porta 6379)"]
            AsteriskPBX["Asterisk PBX (PJSIP / AMI :5038)"]
            ClassificatorRouter["Classificator Router (:2800)"]
            VoskEAGI["vosk-eagi Thin Client"]
        end
    end

    subgraph ExternalConsumers["Ecossistemas Consumidores"]
        OmniChat["OmniChat Backend (37.60.228.113)"]
        LiveKitServer["LiveKit Media Server / SIP Trunk"]
        TelephonyTrunks["Troncos SIP Externos (Vivo, RVX, SobreIP)"]
    end

    DialerApp -->|pgx/v5 Pool| PostgresDB
    DialerApp -->|go-redis/v9 + PubSub| RedisCache
    DialerApp -->|AMI TCP Socket :5038| AsteriskPBX
    AsteriskPBX -->|EAGI FD 3| VoskEAGI
    VoskEAGI -->|WebSocket :2800| ClassificatorRouter
    
    OmniChat -->|HTTP GET / POST :8081| DialerApp
    DialerApp -->|Webhooks HTTP| OmniChat
    AsteriskPBX <-->|PJSIP / RTP| LiveKitServer
    AsteriskPBX <-->|SIP INVITE / RTP| TelephonyTrunks
```

### 2.1. Matriz de Endereçamento e Portas
| Serviço | Host / Container | Porta Interna | Porta Exposta (Host) | Protocolo |
| :--- | :--- | :---: | :---: | :--- |
| **Dialer-Go API** | `dialer-go_dialer-go` | `8080` | **`8081`** | HTTP / REST |
| **PostgreSQL (`dialer_db`)** | `dialer-go_postgres` | `5432` | `5432` | TCP (PostgreSQL) |
| **Redis Cache** | `dialer-go_redis` | `6379` | `6379` | TCP (Redis) |
| **Asterisk AMI** | `dialer-go_asterisk` | `5038` | `5038` | TCP (AMI Socket) |
| **Asterisk PJSIP** | `dialer-go_asterisk` | `5060` | `5060` | UDP / SIP |
| **Classificator Router** | `dialer-go_classificator-router` | `2800` | `2800` | TCP / WebSocket |

---

## 3. Protocolo Pre-Flight Mandatório (Funil Anti-Sobrescrita)

Antes de executar o deploy no servidor novo, a IA/Desenvolvedor DEVE obrigatoriamente executar o funil em 4 etapas:

### 3.1. Etapa 1: Sincronização Git (`Pre-Flight Git`)
```bash
# 1. Validar se o workspace local está limpo
git status

# 2. Buscar atualizações remotas e rebasar na branch main
git fetch origin
git pull --rebase origin main
```

### 3.2. Etapa 2: Execução dos Testes Unitários Go
```bash
# Executar a suíte completa de testes com variável PATH configurada
export PATH=$PATH:/home/marcio/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.0.linux-amd64/bin:/snap/antigravity-cli/21/usr/lib/go-1.22/bin
go test ./...
```
> [!CAUTION]
> **Bloqueio Absoluto:** Se qualquer teste falhar, o deploy deve ser **interrompido imediatamente**. É proibido publicar código com testes falhando ou ignorar erros de compilação.

### 3.3. Etapa 3: Validação de Esquema SQL (`dialer_db`)
Garantir que as tabelas de suporte (`tenants`, `leads`, `cdrs`, `campaigns`, `trunks`, `sip_data`) estejam com a estrutura reconciliada:
- Colunas `att1`, `att2`, `att3` presentes na tabela `leads`.
- Tabela `tenants` contendo o registro tenant `'default'` e webhook configurado.
- Tipos de colunas ID usando `VARCHAR`/`TEXT` (sem tipo PostgreSQL `UUID` estrito).

### 3.4. Etapa 4: Contrato de Variáveis de Ambiente (`servers.ENV` / `.env`)
Verificar a presença das variáveis no `.env` do servidor novo:
```ini
APP_PORT=8080
DATABASE_URL=postgres://dialeruser:dialerpass123@postgres:5432/dialer_db?sslmode=disable
REDIS_ADDR=redis:6379
REDIS_PASSWORD=
REDIS_DB=0
ASTERISK_AMI_HOST=asterisk
ASTERISK_AMI_PORT=5038
ASTERISK_AMI_USER=dialeradmin
ASTERISK_AMI_PASS=dialerpass123
WEBHOOK_URL=http://37.60.228.113:3000/api/telephony/webhook/call-ended
INITIAL_WHITELIST_IPS=127.0.0.1,37.60.228.113,84.247.135.255,10.0.1.0/24,*
```

---

## 4. Orquestração Sequencial do Deploy em Produção

```mermaid
sequenceDiagram
    autonumber
    participant Dev as IA / Engenheiro DevOps
    participant Git as Git Repo (main)
    participant Host as Servidor Novo (84.247.135.255)
    participant Swarm as Docker Swarm Stack
    participant Dialer as Container dialer-go
    participant Asterisk as Container Asterisk

    Dev->>Git: git push origin main
    Dev->>Host: SSH / Deploy Trigger na máquina 84.247.135.255
    Host->>Swarm: docker service update --image ou docker compose build & up -d
    Swarm->>Dialer: Reinicia container dialer-go graciosamente
    Dialer->>Dialer: Executa Boot Check (PostgreSQL, Redis Keyspace Ex, AMI Socket)
    Dev->>Dialer: GET http://84.247.135.255:8081/health
    Dialer-->>Dev: 200 OK {"status":"healthy"}
    Dev->>Dialer: POST /api/v1/configs/apply?reload_ami=true
    Dialer->>Asterisk: AMI Command: pjsip reload & dialplan reload
    Asterisk-->>Dialer: Configurações aplicadas com sucesso
```

### 4.1. Comandos de Atualização no Servidor Novo

```bash
# Navegar até o diretório do projeto no servidor novo
cd /home/marcio/ominichat/dialer-go

# Pull do código comita da branch main
git pull origin main

# Rebuild e atualização do serviço no Docker Swarm / Compose
docker compose build dialer-go
docker compose up -d dialer-go
```

---

## 5. Validação Pós-Deploy e Telemetria em Tempo Real

Após o disparo da atualização, execute o checklist de verificação de borda:

### 5.1. Validação do Endpoint de HealthCheck
```bash
curl -s http://84.247.135.255:8081/health | jq .
```
**Resposta Esperada (`200 OK`):**
```json
{
  "status": "healthy",
  "components": {
    "postgres": "connected",
    "redis": "connected",
    "ami": "connected"
  }
}
```

### 5.2. Validação da Escuta de Mortes Silenciosas no Redis (Keyspace Events)
Inspecione os logs do container em tempo real:
```bash
docker logs --tail 50 dialer-go_dialer-go
```
**Log Obrigatório:**
```text
[INFO] Keyspace Notifications (Ex) ativadas com sucesso no Redis.
[INFO] Escutando quedas de conexão de agentes (Expirações) no canal Redis: __keyevent@0__:expired
```

### 5.3. Aplicação Segura de Configurações do Asterisk (`sip_data`)
```bash
curl -X POST "http://84.247.135.255:8081/api/v1/configs/apply?reload_ami=true" -H "X-Tenant-Id: default"
```

---

## 6. Protocolo de Rollback & Resolução de Erros (Fail-Fast Rule 0.7)

Em caso de falha de conexão com o banco, queda do socket AMI ou recusa de chamadas no PBX:

1. **Notificação Detalhada na UI / Logs:** Interromper imediatamente o deploy e emitir o erro estruturado contendo:
   - **Local / Tela:** *Administração -> Servidores -> Discador Dialer-Go*.
   - **Informação Necessária:** Nome exato da variável ou porta indisponível (ex: `ASTERISK_AMI_PORT` na porta `5038`).
   - **Como Corrigir:** Instrução prática de resolução e exemplos reais.

2. **Comando de Rollback Imediato:**
   ```bash
   # Reverter para o commit/imagem anterior estável
   git reset --hard HEAD~1
   docker compose up -d --build dialer-go
   ```

---

## 7. Governança e Manutenção Sincronizada

Toda alteração efetuada durante o deploy exige a atualização em cadeia dos arquivos mestre:
1. **API Reference:** Atualizar [`.agents/workflows/API_REFERENCE.md`](file:///home/marcio/ominichat/dialer-go/.agents/workflows/API_REFERENCE.md) caso rotas ou DTOs tenham sido modificados.
2. **Grafo Mestre:** Atualizar o documento [`.agents/ARCHITECT.md`](file:///home/marcio/ominichat/dialer-go/.agents/ARCHITECT.md) e a matriz de workflows.
3. **Padrão Telefônico:** Manter gravações Dual-Channel Stereo WAV 8kHz com mesclagem `merge-stereo` assíncrona para MP3.
