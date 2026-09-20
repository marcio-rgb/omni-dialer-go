---
description: Agente Workflow de Deploy Local (Portainer Local, Docker Compose, Asterisk & Dialer-Go)
---

# Agente Workflow de Deploy Local (Local Portainer & Docker Compose Operations Specialist)

## 1. Escopo, Missão & Diretrizes de Desenvolvimento Local

O **Agente Workflow de Deploy Local** é a autoridade técnica responsável pelo ciclo de vida, compilação, validação pré-flight, orquestração e verificação em tempo real do **Dialer-Go** (`go 1.25.0`), **PostgreSQL (`dialer_db`)**, **Redis Cache**, **Vosk STT** e **LiveKit SIP Gateway** no ambiente de desenvolvimento local gerenciado via **Portainer CE Local** e **Docker Compose**.

### 1.1. Missão Central
1. **Garantia de Não-Regressão e Zero Quebra de Ambiente:** Assegurar que nenhuma atualização local seja implantada sem a prévia execução e sucesso dos testes unitários Go (`go test ./...`), conciliação de esquemas do banco `dialer_db` e verificação de integridade dos contratos de interfaces.
2. **Defesa da Autonomia Soberana do Dialer-Go (Cláusula Pétrea):** Garantir que o ambiente local execute o Dialer-Go como um subsistema 100% autônomo, agnóstico e isolado de tabelas ou legados de outros sistemas, comunicando-se estritamente por meio de chamadas de API REST padronizadas (RFC 7807) e Webhooks tipados.
3. **Prevenção Ativa de Conflito de Portas no Host de Desenvolvimento:** Monitorar e gerenciar a coexistência harmônica com outros containers locais do ecossistema (OmniChat, LiveKit Server, PostgreSQL secundário, Evolution API e Portainer CE nas portas 9000/9443).

---

## 2. Topologia de Infraestrutura Local (Docker Standalone & Portainer)

```mermaid
graph TD
    subgraph HostLocal["Host de Desenvolvimento Local (localhost)"]
        subgraph PortainerAdmin["Gestão de Containers"]
            PortainerCE["Portainer CE (:9000 / :9443)<br/>Endpoint 1 (Primary / Local Standalone)"]
        end

        subgraph DialerLocalStack["Docker Compose Stack (dialer-net)"]
            DialerCore["Dialer-Go Engine (:8080)<br/>Binário Estático Go 1.25"]
            DialerDB[("PostgreSQL dialer_db (:5432)<br/>Banco Isolado da Telefonia")]
            DialerCache[("Redis dialer_cache (:6379)<br/>Keyspace Notifications Ex")]
            DialerVosk["Vosk STT Server (:2700)<br/>Kaldi ASR WebSocket"]
            DialerLiveKitSIP["LiveKit SIP Gateway (:5062)<br/>RTP 11100-12000"]
        end

        subgraph LocalEcosystem["Serviços Satélites Locais"]
            OmniChatApp["OmniChat Backend (:3000)"]
            OmniChatWeb["OmniChat Frontend (:5174)"]
            LocalLiveKit["LiveKit Server Core (:7880)"]
            AsteriskHost["Asterisk PBX AMI (:5038) / PJSIP (:5060)"]
        end
    end

    PortainerCE -->|Docker Engine API Socket| DialerCore
    PortainerCE -->|Stack Management| DialerLocalStack

    DialerCore -->|pgx/v5 Pool| DialerDB
    DialerCore -->|go-redis/v9 PubSub / Expirações| DialerCache
    DialerCore -->|AMI TCP Socket :5038| AsteriskHost
    DialerCore -->|WebSocket :2700| DialerVosk

    OmniChatApp -->|HTTP REST :8080| DialerCore
    DialerCore -->|Webhooks HTTP| OmniChatApp
    AsteriskHost <-->|PJSIP / Loopback :5062| DialerLiveKitSIP
    DialerLiveKitSIP <-->|WebRTC SRTP| LocalLiveKit
```

### 2.1. Matriz de Endereçamento e Portas Locais

| Serviço Local | Container Name | Porta Interna | Porta Exposta (Host) | Protocolo | Resolução de Conflitos Locais |
| :--- | :--- | :---: | :---: | :--- | :--- |
| **Portainer CE** | `portainer` | `9000` / `9443` | **`9000` / `9443`** | HTTP / HTTPS | Painel administrativo local Portainer. |
| **Dialer-Go API** | `dialer-go-core` | `8080` | **`8080`** | HTTP / REST | Se `:8080` estiver em uso pelo frontend IA, usar `PORT=8081` ou `8085`. |
| **PostgreSQL (`dialer_db`)** | `dialer-db` | `5432` | **`5432`** | TCP | Se `:5432` colidir com postgres local (`5896`), mapear `5433:5432`. |
| **Redis Cache** | `dialer-cache` | `6379` | **`6379`** | TCP | Se `:6379` colidir com redis satélite (`6381`), mapear `6380:6379`. |
| **Vosk STT Server** | `dialer-vosk` | `2700` | **`2700`** | TCP / WebSocket | Streaming de áudio 8kHz PCM linear via EAGI FD 3. |
| **LiveKit-SIP Gateway** | `dialer-livekit-sip` | `5062` | **`5062`** | UDP / TCP | SIP Loopback com Asterisk PBX. |
| **LiveKit-SIP RTP** | `dialer-livekit-sip` | `11100-12000` | **`11100-12000`** | UDP | Tráfego de mídia RTP de áudio. |
| **Asterisk AMI** | Host / Local | `5038` | **`5038`** | TCP | Socket nativo TCP Asterisk Manager Interface. |

---

## 3. Protocolo Pre-Flight Mandatório (Funil Anti-Sobrescrita Local)

Antes de disparar o deploy no Portainer Local ou via Docker Compose, o desenvolvedor/IA DEVE executar o checklist de integridade:

### 3.1. Etapa 1: Checagem de Branch e Status Git
```bash
# 1. Validar se não há arquivos corrompidos ou conflitos locais
git status

# 2. Assegurar que a branch de trabalho está sincronizada
git branch --show-current
```

### 3.2. Etapa 2: Execução dos Testes Unitários Go
```bash
# Executar a suíte de testes com os paths de ferramentas configurados
export PATH=$PATH:/snap/antigravity-cli/21/usr/lib/go-1.22/bin:/home/marcio/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.0.linux-amd64/bin
go test ./...
```
> [!CAUTION]
> **Interrupção Obrigatória:** Se qualquer teste unitário falhar (`FAIL`), o deploy local é terminantemente proibido. Corrija o código antes de recriar containers.

### 3.3. Etapa 3: Validação do Esquema SQL Local (`dialer_db`)
Certificar-se de que o container PostgreSQL local (`dialer-db`) possui as tabelas atualizadas conforme [`database/schema.sql`](file:///home/marcio/ecosystem/dialer-go/database/schema.sql):
- Tabela `leads` com colunas `att1`, `att2`, `att3` e índices B-Tree de status/prioridade.
- Tabela `phone_trunk_mappings` com chave primária e índice para lookup $O(1)$.
- Tabela `sip_data` para gestão centralizada de dialplan e troncos.
- Stored procedures ACID: `fn_audit_claim_predictive_batch` e `fn_audit_persist_predictive_result`.

### 3.4. Etapa 4: Contrato de Variáveis de Ambiente Local (`docker-compose.local.yml`)
Assegurar que as seguintes variáveis estejam configuradas no arquivo [`docker-compose.local.yml`](file:///home/marcio/ecosystem/dialer-go/docker-compose.local.yml):
```yaml
environment:
  - PORT=8080
  - DATABASE_URL=postgres://dialer_user:dialer_pass@dialer-postgres:5432/dialer_db?sslmode=disable
  - REDIS_ADDR=dialer-redis:6379
  - REDIS_DB=0
  - ASTERISK_AMI_HOST=127.0.0.1
  - ASTERISK_AMI_PORT=5038
  - ASTERISK_AMI_USER=dialer_admin
  - ASTERISK_AMI_PASS=dialer_secret_ami
  - INITIAL_WHITELIST_IPS=127.0.0.1,::1,172.16.0.0/12,192.168.0.0/16
  - MAX_GLOBAL_CHANNELS=60
  - HUMAN_RESERVED_QUOTA=10
  - VOSK_SERVER_URL=ws://dialer-vosk:2700
```

---

## 4. Orquestração do Deploy Local

```mermaid
sequenceDiagram
    autonumber
    participant Dev as Desenvolvedor / IA
    participant Portainer as Portainer Local (:9000 / :9443)
    participant Docker as Docker Engine Local
    participant Dialer as Container dialer-go-core
    participant DB as dialer-postgres
    participant Redis as dialer-redis

    Dev->>Docker: Build da imagem local (Dockerfile)
    Docker-->>Dev: Imagem dialer-go compilada (Go 1.25)
    Dev->>Portainer: Atualiza Stack "dialer-go-local" (ou via Docker Compose CLI)
    Portainer->>Docker: Recreia container dialer-go-core
    Docker->>DB: Estabelece pool pgx/v5 (dialer_db)
    Docker->>Redis: Inicia go-redis + Keyspace Listener (Ex)
    Docker->>Dialer: Boot do Chi Router (:8080)
    Dev->>Dialer: GET http://localhost:8080/health
    Dialer-->>Dev: 200 OK {"status":"healthy"}
```

---

## 5. Métodos de Execução do Deploy Local

### 5.1. Método A: Deploy Direto via Docker Compose CLI (Recomendado para Desenvolvimento)
Para rebuild e inicialização rápida de todos os serviços locais:

```bash
# Navegar até a raiz do dialer-go
cd /home/marcio/ecosystem/dialer-go

# Rebuild do binário Go e subida dos containers em background
docker compose -f docker-compose.local.yml up -d --build

# Para subir apenas o motor principal (quando postgres e redis já estiverem rodando):
docker compose -f docker-compose.local.yml up -d --build dialer-go
```

### 5.2. Método B: Deploy Automatizado no Portainer Local (Modo Texto / Web Editor)
Para criar ou atualizar a stack `dialer-go` no Portainer Local com o arquivo em modo texto puro visível no Web Editor:

```bash
# Executar script automatizado em Node.js
node deploy_local_portainer.js
```

Ou via chamada direta à API REST do Portainer CE Local (`https://localhost:9443` - Endpoint 3):
```bash
# 1. Obter lista de stacks locais (Endpoint 3 = Local Docker Engine)
curl -k -s -H "X-API-Key: ptr_NKC/di0i5hzn/O7anLGuwZ64VKf7Czy+hL44vncTAIY=" \
  "https://localhost:9443/api/stacks?endpointId=3" | jq .

# 2. Criar ou Atualizar a stack (ID 41) no Portainer Local via String (Web Editor)
curl -k -X PUT \
  -H "X-API-Key: ptr_NKC/di0i5hzn/O7anLGuwZ64VKf7Czy+hL44vncTAIY=" \
  -H "Content-Type: application/json" \
  -d '{
    "stackFileContent": "'"$(sed 's/"/\\"/g' docker-compose.local.yml | tr '\n' '\n')"'",
    "env": [],
    "prune": true,
    "pullImage": false
  }' \
  "https://localhost:9443/api/stacks/41?endpointId=3"
```

---

## 6. Validação Pós-Deploy e Telemetria em Tempo Real

Após a subida dos containers locais, execute os seguintes passos de validação:

### 6.1. HealthCheck Geral do Sistema
```bash
curl -s http://localhost:8080/health | jq .
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

### 6.2. Auditoria dos Logs do Redis Keyspace Notifications
Verifique se o discador ativou com sucesso a escuta de mortes silenciosas de operadores:
```bash
docker logs --tail 30 dialer-go-core
```
**Trecho de Log Obrigatório:**
```text
[INFO] Keyspace Notifications (Ex) ativadas com sucesso no Redis.
[INFO] Escutando quedas de conexão de agentes (Expirações) no canal Redis: __keyevent@0__:expired
```

### 6.3. Aplicação Segura de Configurações do Asterisk (`sip_data`)
Para validar a persistência e reload das configurações de dialplan e troncos:
```bash
curl -X POST "http://localhost:8080/api/v1/configs/apply?reload_ami=true" \
  -H "X-Tenant-Id: default" | jq .
```

---

## 7. Protocolo de Resolução de Erros & Troubleshooting (Fail-Fast Rule 0.7)

Em conformidade com a regra **0.7 (Proibição Rígida de Fallbacks Ocultos)**, o sistema nunca deve tentar desvios silenciosos. Ao identificar falhas, interrompa a execução e verifique:

### 7.1. Conflito de Porta HTTP (`8080` ou `8081`)
- **Problema:** Erro `bind: address already in use` na porta `8080`.
- **Causa:** Outro serviço (ex: `cbr-estudio-frontend` ou `evolution-api`) está ocupando a porta.
- **Ação:** No [`docker-compose.local.yml`](file:///home/marcio/ecosystem/dialer-go/docker-compose.local.yml), altere o mapeamento para `"8085:8080"` ou ajuste a variável `PORT=8085`.

### 7.2. Falha de Conexão com PostgreSQL (`dialer_db`)
- **Problema:** Erro `dial tcp dialer-postgres:5432: connect: connection refused`.
- **Causa:** O container `dialer-postgres` ainda está inicializando ou o volume `dialer-pgdata` está bloqueado.
- **Ação:** Inspecione com `docker logs dialer-db` e garanta que o schema em [`database/schema.sql`](file:///home/marcio/ecosystem/dialer-go/database/schema.sql) foi aplicado.

### 7.3. Falha de Conexão com Asterisk AMI (`:5038`)
- **Problema:** Erro `AMI connection timeout / authentication failed`.
- **Causa:** Credenciais em `ASTERISK_AMI_USER` / `ASTERISK_AMI_PASS` não coincidem com o `manager.conf` do Asterisk.
- **Ação:** Valide as credenciais com `telnet 127.0.0.1 5038` e confirme os parâmetros de conexão.

---

## 8. Governança e Manutenção Sincronizada

Toda alteração estrutural no deploy local exige a manutenção atômica dos documentos canônicos:
1. **Contratos de API:** Registrar qualquer novo endpoint ou ajuste de DTO em [`.agents/workflows/API_REFERENCE.md`](file:///home/marcio/ecosystem/dialer-go/.agents/workflows/API_REFERENCE.md).
2. **Governança de Agentes:** Manter este arquivo referenciado em [`.agents/AGENTS.md`](file:///home/marcio/ecosystem/dialer-go/.agents/AGENTS.md).
3. **Padrão de Áudio:** Preservar a política de áudio dual-channel stereo WAV 8kHz com mesclagem rápida via [`cmd/merge-stereo`](file:///home/marcio/ecosystem/dialer-go/cmd/merge-stereo).
