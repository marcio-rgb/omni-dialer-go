# Arquitetura e Guia Operacional: Modo Dispatcher (Dialer-Go Gateway)

## 1. Visão Geral

O modo **`dispatcher`** do **Dialer-Go** (`go 1.25.0`) é um perfil de execução leve projetado para atuar como **Gateway SIP / SBC Transit** no ambiente do escritório local (on-premise), onde reside o IP público autorizado pela operadora **Vivo**.

### 1.1. Principais Características
1. **Sem Dependência de Redis:** O modo dispatcher não inicializa conexões Redis nem consome cache em memória para filas de campanhas.
2. **Sem Webhooks de Retorno a Parceiros:** Toda a inteligência de CRM, auditoria forense, CDRs e webhooks é mantida na VPS (que opera em modo `dialer`). O Dispatcher foca unicamente no chaveamento SIP ultrarrápido.
3. **Distribuição em Round-Robin com Controle Estrito de Canais:**
   - O Asterisk local recebe as chamadas da VPS pelo contexto `[from-vps]`.
   - Executa o balanceamento nos **60 troncos da Vivo**.
   - Monitora os canais ocupados de cada tronco via `GROUP()` / `GROUP_COUNT()`.
   - Se um tronco atingir seu limite de canais simultâneos ou falhar, avança automaticamente para o próximo tronco no anel de Round-Robin.
4. **API REST Padronizada e Autônoma:**
   - Utiliza os mesmos endpoints canônicos de troncos (`/api/v1/trunks`), configurações do Asterisk (`/api/v1/configs` via `sip_data`) e monitoramento (`/health`).

---

## 2. Topologia de Interconexão

```mermaid
flowchart TD
    subgraph VPS["VPS Nuvem (84.247.135.255) - OPERATION_MODE=dialer"]
        DialerVPS["Dialer-Go Core (Completo)<br/>Preditivo, Manual, Vosk, LiveKit"]
        AsteriskVPS["Asterisk PBX (VPS)"]
        TrunkOthers["Troncos Telecom Parceiros (RVX, SobreIP, etc.)"]
        TrunkToOffice["1 Tronco Inter-PBX -> Escritório"]
    end

    subgraph Office["Escritório Local - OPERATION_MODE=dispatcher"]
        DispatcherOffice["Dialer-Go (Modo Dispatcher)<br/>APIs REST: /trunks, /configs, /health"]
        AsteriskOffice["Asterisk PBX Local (IP Autorizado Vivo)"]
        VivoRing["Anel de 60 Troncos Vivo (Round-Robin com GROUP_COUNT)"]
    end

    DialerVPS --> AsteriskVPS
    AsteriskVPS --> TrunkOthers
    AsteriskVPS == "SIP INVITE (Número Discado)" ==> TrunkToOffice
    TrunkToOffice --> AsteriskOffice

    AsteriskOffice <--> DispatcherOffice
    AsteriskOffice --> VivoRing
    VivoRing --> PSTN["Rede Pública Telefônica (PSTN)"]
```

---

## 3. Variáveis de Ambiente no Escritório (`.env`)

Para ativar o modo no servidor do escritório:

```ini
# Ativação do Modo Dispatcher
OPERATION_MODE=dispatcher
PORT=8080

# Banco de Dados Local (dialer_db local)
DATABASE_URL=postgres://dialer_user:dialer_pass@localhost:5432/dialer_db?sslmode=disable

# Asterisk Local (AMI)
ASTERISK_AMI_HOST=127.0.0.1
ASTERISK_AMI_PORT=5038
ASTERISK_AMI_USER=dialer_admin
ASTERISK_AMI_PASS=dialer_secret_ami

# Limite Global de Canais Simultâneos (60 troncos x N canais)
MAX_GLOBAL_CHANNELS=1800
HUMAN_RESERVED_QUOTA=0

# Diretório de Semeadura Automática de Configurações
CONFIG_SEED_DIR=mode/dispatcher
```

---

## 4. Endpoints Disponíveis no Modo Dispatcher

| Método | Endpoint | Responsabilidade |
| :--- | :--- | :--- |
| `GET` | `/health` | Checagem de integridade (PostgreSQL e Asterisk AMI). |
| `GET` | `/api/v1/trunks` | Lista os troncos cadastrados no banco local com ocupação de canais. |
| `POST` | `/api/v1/trunks` | Cadastra novo tronco local. |
| `PUT` | `/api/v1/trunks/{id}` | Atualiza parâmetros de um tronco. |
| `POST` | `/api/v1/trunks/reload` | Força reload de troncos via AMI no Asterisk. |
| `GET` | `/api/v1/configs` | Lista arquivos de configuração (`pjsip.conf`, `extensions.conf`) no banco `sip_data`. |
| `POST` | `/api/v1/configs/apply?reload_ami=true` | Grava arquivos no disco do Asterisk e executa `pjsip reload` e `dialplan reload`. |

---

## 5. Como o Dialplan Opera o Round-Robin Local

No arquivo [`mode/dispatcher/extensions.conf`](file:///home/marcio/ecosystem/dialer-go/mode/dispatcher/extensions.conf):

1. **Incremento Atômico:** Cada chamada incrementa o ponteiro global `GLOBAL(VIVO_DISPATCH_IDX) = (GLOBAL(VIVO_DISPATCH_IDX) % 60) + 1`.
2. **Seleção do Tronco:** Determina o tronco alvo (ex: `vivo_01`, `vivo_02`, ..., `vivo_60`).
3. **Checagem de Canais:** Avalia `GROUP_COUNT(${TRUNK_NAME}@vivo_trunks)`. Se for menor que o limite (`VIVO_CHANNELS_PER_TRUNK`), prossegue com o disparo. Se estiver saturado, pula imediatamente para o próximo tronco.
4. **Failover de Discagem:** Se o `DIALSTATUS` for `CONGESTION` ou `CHANUNAVAIL`, avança para o próximo tronco do anel sem derrubar a chamada.
