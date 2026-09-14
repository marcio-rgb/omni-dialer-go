# Dialer-Go: Catálogo Canônico da API REST & Webhooks (Entrada e Saída)

Este documento estabelece a especificação técnica exaustiva de todas as rotas HTTP, contratos de entrada (payloads, headers, query parameters), contratos de saída (respostas de sucesso e erros padronizados RFC 7807), webhooks de notificação, pipeline estéreo e canais de telemetria em tempo real do **Dialer-Go** (`go 1.25.0`).

> [!IMPORTANT]
> **REGRA GERAL DO WORKSPACE (AGENTS.md):**
> Toda e qualquer alteração, adição, remoção ou refatoração em endpoints HTTP, DTOs de entrada/saída ou webhooks exige **obrigatoriamente e na mesma intervenção** a atualização detalhada deste documento.

---

## 1. Diretrizes Globais de Comunicação e Segurança

### 1.1. Base URL e Versionamento
- **Prefixo de Rota:** `/api/v1`
- **Porta Padrão:** `8080` (configurável via variável de ambiente `PORT`)
- **Protocolo:** HTTP/1.1 (suporte a proxies reversos NGINX, Traefik ou Docker Swarm)

### 1.2. Segurança e Controle de Acesso
- **Autorização por IP Whitelist em Memória:** O discador não utiliza tokens JWT nem sessões para evitar overhead na telefonia. Cada requisição é autorizada em sub-microssegundo consultando uma tabela hash thread-safe (`sync.Map`).
- **Header de Identificação Multi-Tenant:** A maioria dos endpoints exige o cabeçalho HTTP canônico:
  ```http
  X-Tenant-Id: <identificador_do_tenant>
  ```
- **Formato de Dados:** Todos os payloads trafegam estritamente em **JSON** (`Content-Type: application/json`) com chaves padronizadas em `snake_case`.
- **Formato de Datas:** Todos os campos temporais utilizam o padrão estrito **ISO 8601 UTC** (`YYYY-MM-DDTHH:MM:SSZ`).

### 1.3. Padrão Universal de Erro (RFC 7807 / RFC 9457)
Em caso de falha de validação, inexistência de recurso, saturação ou erro interno, o Dialer-Go responde com `Content-Type: application/problem+json`:
```json
{
  "type": "https://dialer-go.internal/errors/invalid-date-range",
  "title": "Unprocessable Entity",
  "status": 422,
  "detail": "A data inicial "start_date" não pode ser posterior à data final "end_date".",
  "code": "INVALID_DATE_RANGE",
  "invalid_params": [
    {
      "name": "start_date",
      "reason": "data posterior a end_date"
    }
  ]
}
```

---

## 2. Matriz Geral de Endpoints

| Método | Endpoint | Responsabilidade Operacional | Autenticação / Header |
| :--- | :--- | :--- | :--- |
| `GET` | `/health` | Diagnóstico de integridade (AMI, Postgres, Redis, Canais) | Sem bloqueio de IP |
| `POST` | `/api/v1/predictive/demand` | Processa a demanda de agentes e calcula disparos preditivos | Whitelist + Body (`tenant_id`) |
| `POST` | `/api/v1/calls/manual` | Origina chamada manual com prioridade preemptiva imediata | Whitelist + Body (`tenant_id`) |
| `GET` | `/api/v1/campaigns` | Lista campanhas do tenant com filtro opcional por status | Whitelist + `X-Tenant-Id` / Query |
| `GET` | `/api/v1/campaigns/{id}` | Recupera uma campanha específica por ID | Whitelist + `X-Tenant-Id` / Query |
| `POST` | `/api/v1/campaigns` | Cria nova campanha no discador (modo, agressividade, tronco) | Whitelist + Body (`tenant_id`) |
| `PUT` | `/api/v1/campaigns/{id}` | Atualiza configurações/agressividade de uma campanha | Whitelist + Body (`tenant_id`) |
| `DELETE`| `/api/v1/campaigns/{id}` | Exclui uma campanha do sistema | Whitelist + `X-Tenant-Id` / Query |
| `POST` | `/api/v1/campaigns/refill` | Ingestão em streaming de lote de mailing via ZIP do MinIO/S3 | Whitelist + Body (`tenant_id`) |
| `POST` | `/api/v1/campaigns/{campaign_id}/leads` | Ingestão de leads em lote via JSON com síntese fonética | Whitelist + Body (`tenant_id`) |
| `POST` | `/api/v1/campaigns/leads` | Ingestão de leads em lote via JSON (campanha no body) | Whitelist + Body (`tenant_id`) |
| `POST` | `/api/v1/campaigns/toggle` | Ativa ou pausa uma campanha (timeout síncrono de 15s) | Whitelist + Body (`tenant_id`) |
| `GET` | `/api/v1/campaigns/{campaign_id}/saturation` | Consulta métricas de saturação e reciclagem FILO da campanha | Whitelist + `X-Tenant-Id` |
| `GET` | `/api/v1/campaigns/saturation` | Retorna visão consolidada de saturação de todas as campanhas | Whitelist + `X-Tenant-Id` |
| `GET` | `/api/v1/reports/calls-summary` | Estatísticas agregadas de chamadas com buffer Redis de 15 min | Whitelist + `X-Tenant-Id` / Query |
| `GET` | `/api/v1/cdrs` | Listagem paginada de CDRs com metadados e URLs de áudio gravado | Whitelist + `X-Tenant-Id` / Query |
| `GET` | `/api/v1/cdrs/{id}` | Consulta de CDR individual por ID com áudio gravado | Whitelist + `X-Tenant-Id` |
| `GET` | `/api/v1/trunks` | Lista troncos com telemetria RTT e canais ativos em tempo real | Whitelist + `X-Tenant-Id` |
| `POST` | `/api/v1/trunks` | Cadastra novo tronco telefônico com sincronização PBX Asterisk | Whitelist + Body (`tenant_id`) |
| `PUT` | `/api/v1/trunks/{trunk_id}` | Atualiza parâmetros operacionais do tronco a quente | Whitelist + `X-Tenant-Id` |
| `DELETE`| `/api/v1/trunks/{trunk_id}` | Exclui tronco telefônico (com proteção de chamadas ativas) | Whitelist + `X-Tenant-Id` |
| `POST` | `/api/v1/trunks/reload` | Força recarga dinâmica de troncos no PBX Asterisk via AMI | Whitelist + `X-Tenant-Id` |
| `GET` | `/api/v1/trunks/enumerations` | Lista opções canônicas de codecs, transportes e registros | Aberto na Whitelist |
| `POST` | `/api/v1/audio/words` | Pré-renderiza palavras (nomes e convênios) no Piper TTS | Whitelist + JSON Body |
| `GET` | `/api/v1/audio/preview` | Preview e concatenação de áudios em WAV binário (Query) | Whitelist + Query String |
| `POST` | `/api/v1/audio/preview` | Preview e concatenação de áudios em WAV binário (Body) | Whitelist + JSON Body |
| `GET` | `/api/v1/amd/config` | Consulta parâmetros ativos do AMD (Asterisk + Vosk STT) | Whitelist + JSON |
| `PUT` | `/api/v1/amd/config` | Atualiza parâmetros AMD em disco/memória e hot-reload | Whitelist + JSON Body |
| `POST` | `/api/v1/amd/config` | Atualiza parâmetros AMD em disco/memória (alias POST) | Whitelist + JSON Body |
| `POST` | `/api/v1/amd/reload` | Força recarga do módulo `app_amd.so` no Asterisk via AMI | Whitelist + JSON |

---

## 3. Especificação Detalhada por Endpoint

---

### 3.1. Diagnóstico de Saúde do Discador (`GET /health`)
Endpoint público de monitoramento e liveness probe (Kubernetes / Docker Swarm). Não exige verificação na lista branca de IPs.

#### Resposta de Sucesso (`200 OK`):
```json
{
  "status": "healthy",
  "components": {
    "asterisk_ami": true,
    "database": true,
    "cache": true
  },
  "telephony_capacity": {
    "active_global_channels": 18,
    "active_human_channels": 4,
    "max_global_channels": 60,
    "human_reserved_quota": 10,
    "available_channels": 42
  }
}
```

---

### 3.2. Processar Demanda Preditiva (`POST /api/v1/predictive/demand`)
Disparado ciclicamente pelo OmniChat Core ou Estúdio IA para informar operadores disponíveis e solicitar dimensionamento do ritmo de originação telefônica.

#### A. Headers:
```http
Content-Type: application/json
```

#### B. Request Body:
| Campo | Tipo | Obrigatório | Descrição / Regra |
| :--- | :--- | :--- | :--- |
| `tenant_id` | `string` | **Sim** | Identificador multi-tenant da organização. |
| `campaign_id` | `string` | **Sim** | Identificador da campanha cujos leads serão consumidos. |
| `aggressiveness` | `float` | Não | Fator multiplicador de agressividade (default configurado na campanha). |
| `min_channels_per_agent` | `integer` | Não | Piso mínimo de canais simultâneos por operador disponível (padrão: `7`, garantindo ratio mínimo de 7:1 para acelerar a discagem). |
| `available_agents` | `array[object]` | **Sim** | Lista de atendentes humanos ou virtuais prontos para chamada. |
| `available_agents[].agent_id` | `string` | **Sim** | Identificador do operador. |
| `available_agents[].priority_order` | `integer` | Não | Ordem de prioridade de distribuição (default: 1). |
| `available_agents[].sip_route` | `string` | **Sim** | Rota SIP/LiveKit para onde o áudio será transferido. |
| `available_agents[].idle_time_seconds` | `integer` | Não | Tempo em segundos de ociosidade do operador. |

##### Exemplo de Entrada:
```json
{
  "tenant_id": "org_alpha",
  "campaign_id": "camp_vendas_01",
  "aggressiveness": 1.4,
  "min_channels_per_agent": 7,
  "available_agents": [
    {
      "agent_id": "usr_carlos_10",
      "priority_order": 1,
      "sip_route": "PJSIP/carlos_webrtc",
      "idle_time_seconds": 45
    },
    {
      "agent_id": "usr_ana_22",
      "priority_order": 2,
      "sip_route": "PJSIP/ana_webrtc",
      "idle_time_seconds": 12
    }
  ]
}
```

#### C. Resposta de Sucesso (`200 OK`):
```json
{
  "success": true,
  "data": {
    "campaign_id": "camp_vendas_01",
    "dialing_channels": 14,
    "status": "active"
  }
}
```

#### D. Erros Mapeados:
- **`400 Bad Request` (`MISSING_REQUIRED_FIELDS`):** `tenant_id` ou `campaign_id` ausentes.
- **`400 Bad Request` (`TRUNK_UNAVAILABLE`):** Tronco vinculado está desabilitado ou sem canais.
- **`404 Not Found` (`CAMPAIGN_NOT_FOUND`):** Campanha inexistente ou inativa.

---

### 3.3. Discagem Manual Preemptiva (`POST /api/v1/calls/manual`)
Origina chamada telefônica pontual com **prioridade preemptiva imediata**, pausando momentaneamente novas chamadas preditivas para assegurar o canal.

#### A. Headers:
```http
Content-Type: application/json
```

#### B. Request Body:
| Campo | Tipo | Obrigatório | Descrição / Regra |
| :--- | :--- | :--- | :--- |
| `tenant_id` | `string` | **Sim** | Identificador multi-tenant. |
| `agent_id` | `string` | **Sim** | Identificador do operador que iniciou a chamada. |
| `phone` | `string` | **Sim** | Telefone de destino (preservado sem normalização destrutiva). |
| `sip_route` | `string` | **Sim** | Endpoint SIP/WebRTC do operador para conexão direta. |
| `trunk_id` | `string` | Não | Tronco específico desejado. Se omitido, usa o padrão do tenant. |

##### Exemplo de Entrada:
```json
{
  "tenant_id": "org_alpha",
  "agent_id": "usr_carlos_10",
  "phone": "11999998888",
  "sip_route": "PJSIP/carlos_webrtc",
  "trunk_id": "trunk_vivo_e1"
}
```

#### C. Resposta de Sucesso (`201 Created`):
```json
{
  "success": true,
  "data": {
    "call_id": "manual-7b3d1e4c-1234-4567-89ab-cdef01234567",
    "status": "dialing",
    "trunk_used": "trunk_vivo_e1"
  }
}
```

#### D. Erros Mapeados:
- **`400 Bad Request` (`MISSING_FIELDS`):** Campos obrigatórios ausentes.
- **`404 Not Found` (`TRUNK_NOT_FOUND`):** Tronco especificado não existe ou está inativo.
- **`429 Too Many Requests` (`TRUNK_CHANNELS_EXHAUSTED`):** Tronco atingiu limite `max_channels`.

---

### 3.3.1. Listar Campanhas (`GET /api/v1/campaigns`)
Retorna todas as campanhas cadastradas para o tenant, com filtro opcional por status (`active`, `paused`, `exhausted`).

#### A. Headers / Query Parameters:
| Parâmetro | Tipo | Local | Obrigatório | Descrição |
| :--- | :--- | :--- | :--- | :--- |
| `X-Tenant-Id` | `string` | Header | Condicional | Identificador do tenant (ou via query param `tenant_id`). |
| `tenant_id` | `string` | Query | Condicional | Identificador do tenant se não enviado no header. |
| `status` | `string` | Query | Não | Filtro por status (`active`, `paused`, `exhausted`). |

#### B. Resposta de Sucesso (`200 OK`):
```json
{
  "success": true,
  "data": [
    {
      "id": "camp_vendas_01",
      "tenant_id": "org_alpha",
      "name": "Campanha Cartão Consignado",
      "mode": "PREDICTIVE",
      "status": "active",
      "aggressiveness": 1.20,
      "trunk_name": "rvx",
      "cycle_count": 0,
      "saturation_level": "NOVA",
      "created_at": "2026-09-14T08:00:00Z"
    }
  ]
}
```

---

### 3.3.2. Obter Campanha por ID (`GET /api/v1/campaigns/{id}`)
Recupera uma campanha específica pelo seu identificador unívoco.

#### A. Resposta de Sucesso (`200 OK`):
```json
{
  "success": true,
  "data": {
    "id": "camp_vendas_01",
    "tenant_id": "org_alpha",
    "name": "Campanha Cartão Consignado",
    "mode": "PREDICTIVE",
    "status": "active",
    "aggressiveness": 1.20,
    "trunk_name": "rvx",
    "cycle_count": 0,
    "saturation_level": "NOVA",
    "created_at": "2026-09-14T08:00:00Z"
  }
}
```

---

### 3.3.3. Criar Campanha (`POST /api/v1/campaigns`)
Cadastra uma nova campanha no discador com parametrização de modo, tronco e agressividade de discagem.

#### A. Request Body:
| Campo | Tipo | Obrigatório | Descrição / Regra |
| :--- | :--- | :--- | :--- |
| `tenant_id` | `string` | **Sim** | Identificador do tenant (ou via `X-Tenant-Id`). |
| `id` | `string` | Não | ID personalizado (gera UUID v4 se omitido). |
| `name` | `string` | Não | Nome descritivo da campanha. |
| `mode` | `string` | Não | `PREDICTIVE` (padrão), `POWER` ou `MANUAL`. |
| `status` | `string` | Não | `active` (padrão) ou `paused`. |
| `aggressiveness` | `number` | Não | Fator multiplicador de pacing (padrão: 1.20). |
| `trunk_name` | `string` | Não | Nome do tronco ou `auto` para balanceamento automático. |

#### B. Resposta de Sucesso (`201 Created`):
```json
{
  "success": true,
  "data": {
    "id": "camp_vendas_01",
    "tenant_id": "org_alpha",
    "name": "Campanha Cartão Consignado",
    "mode": "PREDICTIVE",
    "status": "active",
    "aggressiveness": 1.20,
    "trunk_name": "auto",
    "cycle_count": 0,
    "saturation_level": "NOVA",
    "created_at": "2026-09-14T08:25:00Z"
  }
}
```

---

### 3.3.4. Atualizar Campanha (`PUT /api/v1/campaigns/{id}`)
Atualiza configurações operacionais da campanha, incluindo nome, modo, agressividade e tronco.

#### A. Request Body:
| Campo | Tipo | Obrigatório | Descrição |
| :--- | :--- | :--- | :--- |
| `tenant_id` | `string` | **Sim** | Identificador do tenant (ou via `X-Tenant-Id`). |
| `name` | `string` | Não | Novo nome da campanha. |
| `mode` | `string` | Não | Novo modo operacional (`PREDICTIVE`, `POWER`, `MANUAL`). |
| `status` | `string` | Não | Novo status (`active`, `paused`). Sincroniza com Redis. |
| `aggressiveness` | `number` | Não | Novo fator de pacing (ex: 0.5, 1.0, 1.5). |
| `trunk_name` | `string` | Não | Novo tronco selecionado. |

#### B. Resposta de Sucesso (`200 OK`):
```json
{
  "success": true,
  "data": {
    "id": "camp_vendas_01",
    "tenant_id": "org_alpha",
    "name": "Campanha Cartão Atualizada",
    "mode": "PREDICTIVE",
    "status": "active",
    "aggressiveness": 1.00,
    "trunk_name": "rvx",
    "cycle_count": 0,
    "saturation_level": "NOVA",
    "created_at": "2026-09-14T08:25:00Z"
  }
}
```

---

### 3.3.5. Excluir Campanha (`DELETE /api/v1/campaigns/{id}`)
Remove uma campanha do banco relacional do discador.

#### A. Resposta de Sucesso (`200 OK`):
```json
{
  "success": true,
  "message": "Campanha removida com sucesso",
  "id": "camp_vendas_01"
}
```

---

### 3.4. Ingestão de Mailing via ZIP (`POST /api/v1/campaigns/refill`)
Download em streaming de arquivo compactado do storage MinIO/S3, validação de layout canônico de 4 colunas (`cpf,telefone,campanha_id,tenant_id`) e enfileiramento no PostgreSQL e Redis.

#### A. Request Body:
| Campo | Tipo | Obrigatório | Descrição / Regra |
| :--- | :--- | :--- | :--- |
| `tenant_id` | `string` | **Sim** | Identificador do tenant. |
| `campaign_id` | `string` | **Sim** | Identificador da campanha a ser abastecida. |
| `file_uri` | `string` | **Sim** | URI do pacote compactado no MinIO (`s3://mailings/camp101/base.zip`). |

#### B. Resposta de Sucesso (`200 OK`):
```json
{
  "success": true,
  "data": {
    "campaign_id": "camp_vendas_01",
    "tenant_id": "org_alpha",
    "total_rows": 50000,
    "valid_rows": 49850,
    "invalid_rows": 150,
    "status": "INGESTED",
    "queue_size": 49850
  }
}
```

---

### 3.5. Ingestão de Leads em Lote via JSON (`POST /api/v1/campaigns/{campaign_id}/leads`)
Ingestão direta de leads via array JSON com **síntese fonética automática de nomes inéditos via Piper TTS** e enfileiramento atômico no PostgreSQL e Redis.

#### A. Path / Body Parameters:
- Path `campaign_id` (opcional se fornecido no body).
- Body `tenant_id` e `campaign_id` obrigatórios.

#### B. Request Body (`domain.BatchLeadRequest`):
| Campo | Tipo | Obrigatório | Descrição |
| :--- | :--- | :--- | :--- |
| `campaign_id` | `string` | **Sim** | Identificador da campanha (se não informado na URL). |
| `tenant_id` | `string` | **Sim** | Identificador multi-tenant. |
| `leads` | `array[object]` | **Sim** | Lista de leads para discagem. |
| `leads[].phone` | `string` | **Sim** | Telefone de contato (mínimo 8 dígitos). |
| `leads[].cpf` | `string` | Não | CPF do cliente (com ou sem pontuação). |
| `leads[].name` | `string` | Não | Nome completo do cliente (com acentos preservados). |
| `leads[].first_name` | `string` | Não | Primeiro nome específico para pronúncia. |
| `leads[].work_word` | `string` | Não | Palavra de convênio / empresa (ex: `inss`, `siape`). |

##### Exemplo de Entrada:
```json
{
  "campaign_id": "camp_vendas_01",
  "tenant_id": "org_alpha",
  "leads": [
    {
      "phone": "11999998888",
      "cpf": "12345678901",
      "name": "José de Alencar",
      "first_name": "José",
      "work_word": "inss"
    },
    {
      "phone": "21988887777",
      "cpf": "98765432100",
      "name": "Maria Madalena",
      "first_name": "Maria",
      "work_word": "governo"
    }
  ]
}
```

#### C. Comportamento e Pós-Execução:
1. Nomes ausentes no cache de áudio são detectados por slug O(1).
2. O **Piper TTS** sintetiza em lote os nomes únicos pendentes utilizando a acentuação original.
3. Os leads são persistidos no PostgreSQL na tabela `leads` com status `NEW`.
4. Os itens são enfileirados no Redis via `PushLeads` contendo `LeadQueueItem` pronto para o motor preditivo.

#### D. Resposta de Sucesso (`200 OK`):
```json
{
  "success": true,
  "data": {
    "campaign_id": "camp_vendas_01",
    "total_received": 2,
    "leads_queued": 2,
    "new_audios_synthesized": 1,
    "cached_audios_count": 1,
    "elapsed_ms": 142
  }
}
```

#### E. Erros Mapeados:
- **`400 Bad Request` (`INVALID_JSON`):** Formato JSON malformado.
- **`400 Bad Request` (`MISSING_FIELDS`):** `campaign_id` ou `tenant_id` ausentes.
- **`400 Bad Request` (`EMPTY_LEADS`):** Array `leads` vazio.
- **`400 Bad Request` (`NO_VALID_LEADS`):** Nenhum lead possuía telefone com comprimento válido.
- **`500 Internal Server Error`:** Falha na persistência no banco relacional.

---

### 3.6. Ativar / Desativar Campanha (`POST /api/v1/campaigns/toggle`)
Processo síncrono com **timeout estrito de 15 segundos** para pausar ou retomar uma campanha imediatamente.

#### A. Request Body:
| Campo | Tipo | Obrigatório | Descrição |
| :--- | :--- | :--- | :--- |
| `campaign_id` | `string` | **Sim** | Identificador da campanha. |
| `tenant_id` | `string` | **Sim** | Identificador do tenant. |
| `enable` | `boolean` | **Sim** | `true` para ativar; `false` para pausar. |

##### Exemplo de Entrada:
```json
{
  "campaign_id": "camp_vendas_01",
  "tenant_id": "org_alpha",
  "enable": false
}
```

#### B. Resposta de Sucesso (`200 OK`):
```json
{
  "success": true,
  "data": {
    "id": "camp_vendas_01",
    "tenant_id": "org_alpha",
    "name": "Campanha de Vendas",
    "status": "paused",
    "created_at": "2026-09-08T18:24:30Z"
  }
}
```

---

### 3.7. Consultar Saturação de Campanha Individual (`GET /api/v1/campaigns/{campaign_id}/saturation`)
Retorna os indicadores de queima da base, ciclos FILO e urgência de refill da campanha.

#### A. Headers:
```http
X-Tenant-Id: org_alpha
```

#### B. Resposta de Sucesso (`200 OK`):
```json
{
  "success": true,
  "data": {
    "campaign_id": "camp_vendas_01",
    "tenant_id": "org_alpha",
    "saturation_level": "RECICLADA",
    "total_leads": 50000,
    "available_leads": 8500,
    "dialed_leads": 41500,
    "cycle_count": 2,
    "burn_rate_percentage": 83.0,
    "refill_urgency": "HIGH",
    "infinite_looping_mode": "FILO_ACTIVE",
    "estimated_exhaustion_hours": 28.3
  }
}
```

---

### 3.8. Consultar Saturação Consolidada do Tenant (`GET /api/v1/campaigns/saturation`)
Retorna a síntese de todas as campanhas ativas do tenant com contadores de criticidade.

#### Resposta de Sucesso (`200 OK`):
```json
{
  "success": true,
  "data": {
    "tenant_id": "org_alpha",
    "active_campaigns": 3,
    "critical_count": 1,
    "refill_urgent_count": 2,
    "campaigns": [
      {
        "campaign_id": "camp_vendas_01",
        "tenant_id": "org_alpha",
        "saturation_level": "CRITICA",
        "total_leads": 20000,
        "available_leads": 500,
        "dialed_leads": 19500,
        "cycle_count": 3,
        "burn_rate_percentage": 97.5,
        "refill_urgency": "CRITICAL",
        "infinite_looping_mode": "FILO_ACTIVE",
        "estimated_exhaustion_hours": 1.6
      }
    ]
  }
}
```

---

### 3.9. Relatório Analítico de Chamadas (`GET /api/v1/reports/calls-summary`)
Endpoint de alta performance protegido por uma camada de **Buffer de Cache no Redis com Validade Estrita de 15 Minutos (900 Segundos)**.

#### Parâmetros:
| Parâmetro | Local | Tipo | Obrigatório | Descrição |
| :--- | :--- | :--- | :--- | :--- |
| `tenant_id` | Query / Header | `string` | **Sim** | Identificador do tenant. |
| `start_date` | Query | `ISO 8601` | Não | Data inicial (padrão: `00:00:00 UTC` de hoje). |
| `end_date` | Query | `ISO 8601` | Não | Data final (padrão: `now()`). |
| `campaign_id` | Query | `UUID` | Não | Filtra métricas de uma campanha específica. |

#### Resposta de Sucesso (`200 OK`):
```json
{
  "success": true,
  "data": {
    "tenant_id": "org_alpha",
    "period": {
      "start_date": "2026-09-09T00:00:00Z",
      "end_date": "2026-09-09T06:30:00Z"
    },
    "metrics": {
      "total_dialed": 15000,
      "delivered_calls": 3750,
      "answered_calls": 3820,
      "answering_machine_calls": 6450,
      "invalid_number_calls": 1200,
      "failed_calls": 480,
      "busy_calls": 1650,
      "unanswered_calls": 1530,
      "abandoned_calls": 70,
      "cancelled_calls": 100,
      "percentages": {
        "delivery_rate_percentage": 25.0,
        "answer_rate_percentage": 25.47,
        "amd_discard_percentage": 43.0,
        "invalid_number_rate_percentage": 8.0,
        "failure_rate_percentage": 3.2,
        "busy_rate_percentage": 11.0,
        "no_answer_rate_percentage": 10.2,
        "abandonment_rate_percentage": 1.83
      }
    },
    "durations": {
      "total_talk_time_seconds": 780000,
      "average_talk_time_seconds": 208,
      "total_ring_time_seconds": 270000,
      "average_ring_time_seconds": 18
    },
    "cache_meta": {
      "cached": true,
      "cached_at": "2026-09-09T06:20:00Z",
      "cache_expires_at": "2026-09-09T06:35:00Z",
      "ttl_remaining_seconds": 540,
      "buffer_duration_seconds": 900
    }
  }
}
```

---

### 3.9.1. Listagem Paginada de CDRs (`GET /api/v1/cdrs` / `GET /api/v1/reports/cdrs`)
Endpoint canônico para consulta de chamadas (CDRs) detalhadas, contendo caminhos do Asterisk (`recording_file`) e URLs de streaming direto (`recording_url`) gerados pelo discador. Permite que serviços externos (como o Omnichat e dashboards) operem 100% desacoplados de bases internas.

#### Parâmetros de Consulta (Query Params):
| Parâmetro | Tipo | Obrigatório | Padrão | Descrição |
| :--- | :--- | :--- | :--- | :--- |
| `tenant_id` | `string` | **Sim** | - | Identificador do locatário (ou via header `X-Tenant-Id`). |
| `campaign_id`| `string` | Não | - | Filtra chamadas de uma campanha específica. |
| `phone` | `string` | Não | - | Filtra chamadas por número de telefone. |
| `disposition`| `string` | Não | - | Filtra por desfecho (`ANSWERED`, `DELIVERED`, `VOICEMAIL`, etc.). |
| `start_date` | `ISO 8601`| Não | - | Filtro temporal inicial (`created_at >= start_date`). |
| `end_date` | `ISO 8601`| Não | - | Filtro temporal final (`created_at <= end_date`). |
| `page` | `integer` | Não | `1` | Número da página (mínimo: 1). |
| `limit` | `integer` | Não | `20` | Quantidade de registros por página (máximo: 100). |

#### Resposta de Sucesso (`200 OK`):
```json
{
  "success": true,
  "data": {
    "total": 1,
    "page": 1,
    "limit": 20,
    "total_pages": 1,
    "cdrs": [
      {
        "id": "cdr-chan-1726302600.12",
        "tenant_id": "org_alpha",
        "campaign_id": "camp-99",
        "phone": "11999998888",
        "agent_id": "user-42",
        "call_type": "PREDICTIVE",
        "disposition": "ANSWERED",
        "sip_status": 200,
        "hangup_cause": 16,
        "duration_seconds": 45,
        "billsec_seconds": 40,
        "ring_seconds": 5,
        "trunk_used": "trunk-pjsip-01",
        "recording_file": "/var/spool/asterisk/monitor/2026/09/14/063000-PRED-11999998888-1726302600.12.wav",
        "recording_url": "https://api-omnichat.creditobr.org/dialer-go/api/v1/recordings/2026/09/14/063000-PRED-11999998888-1726302600.12.wav",
        "created_at": "2026-09-14T06:30:00Z",
        "initiated_at": "2026-09-14T06:30:00Z",
        "answered_at": "2026-09-14T06:30:05Z",
        "ended_at": "2026-09-14T06:30:45Z"
      }
    ]
  }
}
```

---

### 3.9.2. Consulta de CDR por Identificador (`GET /api/v1/cdrs/{id}`)
Recupera o registro canônico de uma chamada individual através do seu identificador de canal / CDR ID.

#### Resposta de Sucesso (`200 OK`):
```json
{
  "success": true,
  "data": {
    "id": "cdr-chan-1726302600.12",
    "tenant_id": "org_alpha",
    "phone": "11999998888",
    "call_type": "PREDICTIVE",
    "disposition": "ANSWERED",
    "duration_seconds": 45,
    "billsec_seconds": 40,
    "ring_seconds": 5,
    "trunk_used": "trunk-pjsip-01",
    "recording_file": "/var/spool/asterisk/monitor/2026/09/14/063000-PRED-11999998888-1726302600.12.wav",
    "recording_url": "https://api-omnichat.creditobr.org/dialer-go/api/v1/recordings/2026/09/14/063000-PRED-11999998888-1726302600.12.wav",
    "created_at": "2026-09-14T06:30:00Z"
  }
}
```

---

### 3.9.3. Streaming e Download de Gravação de Áudio (`GET /api/v1/recordings/*`)
Transmite arquivos de áudio gravados pelo Asterisk MixMonitor com suporte a streaming HTTP 206 Range (scrubbing/seek no player de áudio do navegador) e CORS aberto para integração web/CRM.

#### Métodos Suportados:
- `GET /api/v1/recordings/{ano}/{mes}/{dia}/{arquivo}.wav`
- `HEAD /api/v1/recordings/{ano}/{mes}/{dia}/{arquivo}.wav`
- `OPTIONS /api/v1/recordings/*` (Preflight CORS)

#### Headers de CORS e Streaming:
- `Access-Control-Allow-Origin: *`
- `Access-Control-Allow-Methods: GET, HEAD, OPTIONS`
- `Access-Control-Allow-Headers: Range, Content-Type, Authorization`
- `Access-Control-Expose-Headers: Content-Range, Content-Length, Accept-Ranges`
- `Content-Type: audio/wav`
- `Accept-Ranges: bytes`

#### Resposta de Sucesso:
- `200 OK` (Download completo) ou `206 Partial Content` (Streaming/Seek com header `Range`).

---

### 3.10. Listagem de Troncos com Telemetria (`GET /api/v1/trunks`)
Retorna todos os troncos cadastrados enriquecidos com telemetria de latência RTT em milissegundos e ocupação de canais em tempo real.

#### Resposta de Sucesso (`200 OK`):
```json
{
  "success": true,
  "data": {
    "tenant_id": "org_alpha",
    "total_trunks": 1,
    "trunks": [
      {
        "id": "trunk_vivo_e1",
        "tenant_id": "org_alpha",
        "name": "Vivo E1 SIP Principal",
        "direction": "BIDIRECTIONAL",
        "registration_mode": "IP_BASED",
        "host": "10.200.5.10",
        "port": 5060,
        "tech_prefix": "55001",
        "transport": "UDP",
        "codecs": ["alaw", "ulaw"],
        "max_channels": 30,
        "is_enabled": true,
        "health": {
          "status": "ONLINE",
          "latency_ms": 14.2,
          "active_channels": 8,
          "max_channels": 30,
          "available_channels": 22,
          "utilization_percentage": 26.6,
          "is_saturated": false,
          "last_qualify_at": "2026-09-09T06:24:10Z"
        },
        "created_at": "2026-09-01T10:00:00Z"
      }
    ]
  }
}
```

---

### 3.11. Cadastrar Tronco SIP/PJSIP (`POST /api/v1/trunks`)
Cadastra novo tronco telefônico com suporte a autenticação por registro ou IP, prefixo técnico e recarga imediata no PBX Asterisk.

#### Request Body:
| Campo | Tipo | Obrigatório | Padrão | Descrição |
| :--- | :--- | :--- | :--- | :--- |
| `id` | `string` | **Sim** | — | Identificador único do tronco (`trunk_vivo_01`). |
| `tenant_id` | `string` | **Sim** | — | Identificador multi-tenant. |
| `name` | `string` | **Sim** | — | Nome descritivo do tronco. |
| `host` | `string` | **Sim** | — | IP ou FQDN da operadora VoIP. |
| `port` | `integer` | Não | `5060` | Porta de sinalização SIP. |
| `tech_prefix` | `string` | Não | `null` | Prefixo técnico prepended ao número (ex: `55001`). |
| `registration_mode`| `string` | Não | `IP_BASED` | `REGISTER` ou `IP_BASED`. |
| `auth_username` | `string` | Condicional | `null` | Obrigatório se `registration_mode = REGISTER`. |
| `auth_password` | `string` | Condicional | `null` | Senha SIP de autenticação. |
| `transport` | `string` | Não | `UDP` | `UDP`, `TCP` ou `TLS`. |
| `codecs` | `array[string]`| Não | `["alaw", "ulaw"]` | Codecs permitidos (`alaw`, `ulaw`, `g729`, `opus`, `gsm`). |
| `max_channels` | `integer` | Não | `30` | Cota máxima de ligações simultâneas autorizadas. |
| `is_enabled` | `boolean` | Não | `true` | Status de ativação do tronco. |

#### Resposta de Sucesso (`201 Created`):
```json
{
  "success": true,
  "data": {
    "id": "trunk_vivo_e1",
    "name": "Vivo E1 SIP Principal",
    "status": "created",
    "message": "Tronco cadastrado e sincronizado com o PBX com sucesso."
  }
}
```

---

### 3.12. Atualizar Tronco Telefônico (`PUT /api/v1/trunks/{trunk_id}`)
Atualiza parâmetros de sinalização, mídia, credenciais, limites simultâneos ou prefixo técnico com **recarga a quente no Asterisk** sem interrupção de canais em curso.

#### Resposta de Sucesso (`200 OK`):
```json
{
  "success": true,
  "data": {
    "id": "trunk_vivo_e1",
    "tenant_id": "org_alpha",
    "name": "Vivo E1 SIP Principal (Alta Capacidade)",
    "updated_at": "2026-09-09T07:15:00Z",
    "pbx_sync": {
      "status": "APPLIED_HOT",
      "message": "Configuração PJSIP recarregada dinamicamente sem interrupção de canais em curso."
    }
  }
}
```

---

### 3.13. Excluir Tronco Telefônico (`DELETE /api/v1/trunks/{trunk_id}`)
Remove o tronco da base de dados e do PBX Asterisk.

#### Query Parameters:
- `force` (`boolean`, opcional, default: `false`): Se `false` e houver chamadas ativas, a exclusão é rejeitada com `409 Conflict`. Se `true`, finaliza as chamadas em curso e exclui.

#### Resposta de Sucesso (`200 OK`):
```json
{
  "success": true,
  "data": {
    "trunk_id": "trunk_vivo_e1",
    "status": "deleted",
    "message": "Tronco excluído com sucesso e configurações removidas do PBX."
  }
}
```

---

### 3.14. Recarregar Troncos e Regras no PBX Asterisk (`POST /api/v1/trunks/reload`)
Força a sincronização dinâmica de todos os troncos ativos do banco de dados para os arquivos de configuração do Asterisk (`pjsip.conf`) e aciona a recarga via AMI sem downtime.

#### A. Headers:
```http
X-Tenant-Id: org_alpha
```

#### B. Resposta de Sucesso (`200 OK`):
```json
{
  "success": true,
  "message": "Agrupamento de troncos e regras do PBX recarregados com sucesso."
}
```

---

### 3.15. Opções Canônicas de Configuração (`GET /api/v1/trunks/enumerations`)
Fornece aos frontends a lista tipada de opções suportadas para troncos (modos de registro, transportes, direções, codecs, DTMF e NAT).

#### Resposta de Sucesso (`200 OK`):
```json
{
  "success": true,
  "data": {
    "registration_modes": [
      { "value": "REGISTER", "label": "Com Registro (User/Senha)" },
      { "value": "IP_BASED", "label": "Sem Registro (Autenticação por IP)" }
    ],
    "transports": [
      { "value": "UDP", "label": "UDP (Padrão de Baixa Latência)" },
      { "value": "TCP", "label": "TCP (Confiável para Pacotes Longos)" },
      { "value": "TLS", "label": "TLS (Sinalização Encriptada)" }
    ],
    "directions": [
      { "value": "BIDIRECTIONAL", "label": "Bidirecional (Entrada e Saída)" },
      { "value": "OUTBOUND", "label": "Somente Saída (Discador Preditivo/Manual)" },
      { "value": "INBOUND", "label": "Somente Entrada (DIDs Receptivos)" }
    ],
    "supported_codecs": [
      { "value": "alaw", "label": "G.711 A-law (PCMA - Padrão Brasil)" },
      { "value": "ulaw", "label": "G.711 u-law (PCMU - Padrão Internacional)" },
      { "value": "g729", "label": "G.729 (Baixo Consumo de Banda)" },
      { "value": "opus", "label": "Opus (HD Audio WebRTC/LiveKit)" },
      { "value": "gsm", "label": "GSM 06.10" }
    ],
    "dtmf_modes": [
      { "value": "RFC4733", "label": "RFC 4733 / RFC 2833 (Recomendado)" },
      { "value": "INBAND", "label": "In-Band (Tons no Áudio)" },
      { "value": "INFO", "label": "SIP INFO" },
      { "value": "AUTO", "label": "Detecção Automática" }
    ],
    "nat_modes": [
      { "value": "FORCE_RPORT", "label": "Force rport (Simétrico / NAT Severo)" },
      { "value": "YES", "label": "Yes (Habilitado)" },
      { "value": "NO", "label": "No (IP Público Direto)" },
      { "value": "COMEDIA", "label": "Comedia" }
    ]
  }
}
```

---

### 3.16. Pré-renderizar Dicionário no Piper TTS (`POST /api/v1/audio/words`)
Recebe listas de nomes, convênios e frases fixas da URA, processa a síntese fonética em lote com o motor **Piper TTS** e armazena os arquivos WAV normalizados no storage de áudio.

#### A. Request Body (`domain.UpsertAudioWordsRequest`):
| Campo | Tipo | Obrigatório | Descrição |
| :--- | :--- | :--- | :--- |
| `custom_phrases` | `object` | Não | Frases-base (`saudacao`, `falo_com`, `momentinho`). |
| `pausa_ms` | `integer` | Não | Silêncio padrão entre frases em milissegundos. |
| `names` | `array[object]` | Não | Lista de nomes para síntese (`key`, `text`). |
| `work_words` | `array[object]` | Não | Lista de convênios/órgãos (`key`, `text`). |
| `force_overwrite` | `boolean` | Não | Se `true`, regera áudios mesmo que já existam em cache. |

##### Exemplo de Entrada:
```json
{
  "custom_phrases": {
    "saudacao": "Olá, bom dia!",
    "falo_com": "Por favor, falo com",
    "momentinho": "Só um momento enquanto localizo seu cadastro."
  },
  "pausa_ms": 150,
  "names": [
    { "key": "jose", "text": "José" },
    { "key": "maria", "text": "Maria" }
  ],
  "work_words": [
    { "key": "inss", "text": "beneficiário do INSS" },
    { "key": "governo", "text": "servidor público estadual" }
  ],
  "force_overwrite": false
}
```

#### B. Resposta de Sucesso (`200 OK`):
```json
{
  "success": true,
  "data": {
    "names_processed": 2,
    "work_words_processed": 2,
    "cached_count": 1,
    "generated_count": 3,
    "errors": []
  }
}
```

---

### 3.17. Preview Dinâmico de Concatenação de Áudio (`GET` / `POST /api/v1/audio/preview`)
Gera instantaneamente a concatenação de áudios pré-sintetizados em memória e devolve o arquivo WAV binário resultante para audição ou teste na interface web.

#### A. Parâmetros (Query String no GET ou JSON Body no POST):
| Parâmetro | Tipo | Obrigatório | Descrição |
| :--- | :--- | :--- | :--- |
| `name` / `first_name` | `string` | **Sim** | Slug ou identificador do nome do cliente (ex: `jose`). |
| `work_word` | `string` | **Sim** | Slug ou identificador do convênio (ex: `inss`). |
| `pausa_ms` | `integer` | Não | Silêncio inserido entre os blocos (padrão do sistema). |
| `include_momentinho`| `boolean` | Não | Se inclui a frase de espera no áudio final. |

#### B. Resposta de Sucesso (`200 OK` Binário):
- **Headers:**
  ```http
  Content-Type: audio/wav
  Content-Disposition: inline; filename="preview_jose_inss.wav"
  ```
- **Body:** Buffer de áudio linear PCM 16-bit 8000Hz (ou 16000Hz) mono pronto para reprodução imediata.

#### C. Erros Mapeados:
- **`404 Not Found` (`WORD_NOT_FOUND`):** Palavra ou nome ausente no banco/cache de áudios.
- **`400 Bad Request` (`CONCATENATION_ERROR`):** Falha no processamento de áudio ou dados inválidos.

---

### 3.18. Consultar Configurações de AMD (`GET /api/v1/amd/config`)
Retorna os parâmetros de triagem acústica em tempo real ativos no Asterisk PBX (`app_amd`) e no motor de reconhecimento de voz Vosk STT.

#### Resposta de Sucesso (`200 OK`):
```json
{
  "success": true,
  "data": {
    "initial_silence_ms": 2000,
    "greeting_ms": 1500,
    "after_greeting_silence_ms": 600,
    "total_analysis_time_ms": 3500,
    "min_word_length_ms": 100,
    "between_words_silence_ms": 50,
    "maximum_number_of_words": 3,
    "silence_threshold": 256,
    "max_silence_ms": 2000,
    "voicemail_phrases": [
      "caixa postal",
      "deixe recado",
      "deixe seu recado",
      "apos o sinal",
      "nao pode atender"
    ],
    "human_greetings": [
      "alo",
      "ola",
      "oi",
      "sim",
      "pronto",
      "pois nao"
    ],
    "vosk_max_duration_sec": 2.8,
    "updated_at": "2026-09-14T02:00:00Z"
  }
}
```

---

### 3.19. Atualizar Configurações de AMD e Hot-Reload (`PUT` / `POST /api/v1/amd/config`)
Atualiza parâmetros de triagem em memória e nos arquivos de configuração do Asterisk (`amd.conf` e `vosk_amd.json`). Opcionalmente executa recarga a quente no Asterisk via AMI.

#### A. Request Body (`domain.UpdateAMDConfigRequest`):
| Campo | Tipo | Obrigatório | Descrição |
| :--- | :--- | :--- | :--- |
| `initial_silence_ms` | `integer` | Não | Duração máxima em ms de silêncio antes da saudação inicial. |
| `greeting_ms` | `integer` | Não | Duração máxima em ms da saudação de atendimento humano. |
| `after_greeting_silence_ms` | `integer` | Não | Silêncio após a saudação para confirmar atendimento. |
| `total_analysis_time_ms` | `integer` | Não | Tempo limite máximo de análise acústica AMD. |
| `silence_threshold` | `integer` | Não | Sensibilidade do detector de áudio (nível de ruído). |
| `voicemail_phrases` | `array[string]` | Não | Lista de termos reconhecidos como caixa postal no Vosk STT. |
| `human_greetings` | `array[string]` | Não | Lista de saudações reconhecidas como humano no Vosk STT. |
| `vosk_max_duration_sec` | `float` | Não | Duração máxima da escuta com EAGI Vosk (segundos). |
| `apply` | `boolean` | **Sim** | Se `true`, aciona imediatamente o reload do módulo no Asterisk via AMI. |

##### Exemplo de Entrada:
```json
{
  "greeting_ms": 1400,
  "after_greeting_silence_ms": 500,
  "total_analysis_time_ms": 3200,
  "apply": true
}
```

#### B. Resposta de Sucesso (`200 OK`):
```json
{
  "success": true,
  "data": {
    "config": {
      "initial_silence_ms": 2000,
      "greeting_ms": 1400,
      "after_greeting_silence_ms": 500,
      "total_analysis_time_ms": 3200,
      "updated_at": "2026-09-14T02:10:00Z"
    },
    "applied_asterisk": true,
    "message": "Configurações atualizadas em memória e disco com sucesso e módulo app_amd.so recarregado no Asterisk"
  }
}
```

---

### 3.20. Forçar Recarga do Módulo AMD no Asterisk (`POST /api/v1/amd/reload`)
Executa o comando `module reload app_amd.so` no PBX Asterisk via AMI.

#### Resposta de Sucesso (`200 OK`):
```json
{
  "success": true,
  "data": {
    "message": "app_amd.so recarregado no Asterisk com sucesso",
    "output": "Module app_amd.so reloaded successfully."
  }
}
```

---

## 4. Webhooks Opcionais de Notificação (Egress)

Por padrão, o **Dialer-Go** opera de forma 100% autônoma e passiva através de suas APIs REST canônicas (`GET /api/v1/cdrs`, `GET /api/v1/reports/calls-summary`), **não direcionando nenhuma chamada nem webhook para sistemas externos**.

Caso o sistema cliente deseje receber notificações ativas (push) de término de chamada, pode fornecer a URL via requisição (`webhook_url` na chamada manual) ou configurar a variável opcional de ambiente `WEBHOOK_URL`. Se nenhuma URL for definida, nenhuma requisição HTTP externa é disparada.

---

### 4.1. Webhook Notificador de Término de Chamada (Opcional)
Disparado pelo componente `WebhookClient` do **Dialer-Go** imediatamente após a liberação da chamada telefônica, apenas quando houver URL de webhook explicitamente informada ou configurada.

#### A. Cabeçalhos HTTP Enviados:
```http
POST {webhook_url} HTTP/1.1
Content-Type: application/json
X-Tenant-Id: <tenant_id>
User-Agent: DialerGo-WebhookNotifier/1.0
```

#### B. Payload Canônico (`domain.CallEndedWebhookPayload`):
| Campo | Tipo | Descrição |
| :--- | :--- | :--- |
| `event` | `string` | Identificador fixo: `"telephony.call_ended"`. |
| `call_id` | `string` | UUID unívoco da chamada gerado no início da discagem. |
| `call_type` | `string` | `"predictive"`, `"manual"` ou `"inbound"`. |
| `tenant_id` | `string` | Identificador multi-tenant do locatário. |
| `agent_id` | `string / null` | Identificador do operador caso a chamada tenha sido atendida. |
| `phone` | `string` | Telefone discado preservado sem truncamento. |
| `trunk_used` | `string` | Identificador do tronco SIP utilizado. |
| `disposition` | `string` | Desfecho: `"ANSWERED"`, `"NO_ANSWER"`, `"BUSY"`, `"FAILED"`, `"VOICEMAIL"`, `"ABANDONED"`. |
| `hangup_cause` | `integer` | Código de causa Q.850 / ISDN do encerramento (ex: 16 = Normal, 17 = Ocupado). |
| `hangup_reason` | `string` | Descrição textual da causa da finalização. |
| `is_answered` | `boolean` | `true` se houve atendimento humano confirmado pelo AMD. |
| `duration_seconds` | `integer` | Duração total da ligação desde o início da sinalização. |
| `billsec_seconds` | `integer` | Duração tarifada (tempo de conversação). |
| `ring_seconds` | `integer` | Tempo em segundos de toque (chamando) antes do atendimento/queda. |
| `started_at` | `ISO 8601` | Timestamp do início da tentativa. |
| `ended_at` | `ISO 8601` | Timestamp do desligamento da perna. |
| `recording_url` | `string` | URL pública de streaming/download da gravação. |
| `timestamp` | `integer` | Unix timestamp em segundos do momento do disparo. |

##### Exemplo de Payload Enviado:
```json
{
  "event": "telephony.call_ended",
  "call_id": "manual-1726302600.12",
  "call_type": "MANUAL",
  "tenant_id": "tenant-empresa-1",
  "agent_id": "op-101",
  "phone": "11999998888",
  "trunk_used": "trunk-vivo-01",
  "disposition": "ANSWERED",
  "hangup_cause": 16,
  "hangup_reason": "Chamada Atendida e Encerrada",
  "is_answered": true,
  "duration_seconds": 64,
  "billsec_seconds": 58,
  "ring_seconds": 6,
  "started_at": "2026-09-14T02:15:00Z",
  "ended_at": "2026-09-14T02:16:04Z",
  "recording_url": "https://api-omnichat.creditobr.org/dialer-go/api/v1/recordings/2026/09/14/063000-MAN-11999998888-1.wav",
  "timestamp": 1789352164
}
```

---

## 5. Pós-Processamento e Pipeline de Áudio Estéreo (`merge-stereo`)

Para auditoria de conformidade regulatória e treinamento de IA, as chamadas atendidas são gravadas com segregação física de faixas em estéreo:

```text
Asterisk MixMonitor (tx.wav + rx.wav) 
       │
       ▼
/var/lib/asterisk/agi-bin/merge-stereo <BASE_FILENAME>
       │
       ▼
SoX Interleave (Left = TX / Right = RX)
       │
       ▼
Arquivo Final Estéreo: <BASE_FILENAME>.wav
       │
       ▼
Upload MinIO/S3 + URL Assinada para o CRM
```

### 5.1. Mapeamento de Canais de Áudio:
- **Canal 1 (Esquerdo / Left / TX):** Voz emitida pelo operador / atendente ou áudio da URA / TTS.
- **Canal 2 (Direito / Right / RX):** Voz captada do cliente externo atendendo pelo tronco telefônico.

### 5.2. Comando do Dialplan Asterisk (`extensions.conf`):
```asterisk
exten => s,1,Set(REC_FILE=/var/spool/asterisk/monitor/rec_${UNIQUEID})
same => n,MixMonitor(${REC_FILE}.wav,b r(${REC_FILE}-rx.wav)t(${REC_FILE}-tx.wav),/var/lib/asterisk/agi-bin/merge-stereo ${REC_FILE})
```

### 5.3. Execução do Script AGI (`merge-stereo`):
O script concatena as pernas temporárias em estéreo real:
```bash
sox -M -c 1 "${REC_FILE}-tx.wav" -c 1 "${REC_FILE}-rx.wav" -c 2 "${REC_FILE}-stereo.wav"
mv "${REC_FILE}-stereo.wav" "${REC_FILE}.wav"
rm -f "${REC_FILE}-tx.wav" "${REC_FILE}-rx.wav"
```

---

## 6. Barramento de Telemetria e Monitoramento em Tempo Real

O monitoramento da progressão de chamadas opera sob um **Ciclo de Vida On-Demand**: o ticker interno de 2 segundos só roda enquanto houver ouvintes conectados. Quando todos os clientes desconectam, a alimentação é suspensa imediatamente para poupar CPU e I/O de rede.

### 6.1. Canais Dedicados de WebSocket / Socket.io:

#### Canal 1: `telephony:monitoring:call_completed`
Disparado instantaneamente no encerramento de cada chamada:
```json
{
  "event": "telephony:monitoring:call_completed",
  "data": {
    "call_id": "call-9b8c7d6e-5f4a",
    "cpf": "123.456.789-01",
    "name": "Maria Silva",
    "phone": "(11) 99999-8888",
    "status": "atendida",
    "duration": 78,
    "agent_id": "usr_carlos_10",
    "agent_name": "Carlos Eduardo",
    "timestamp": "2026-09-14T02:16:04Z"
  }
}
```

#### Canal 2: `telephony:monitoring:stats`
Enviado periodicamente (a cada 2s) com as estatísticas operacionais atualizadas do tenant:
```json
{
  "event": "telephony:monitoring:stats",
  "data": {
    "tenant_id": "org_alpha",
    "available_operators": [
      { "id": "usr_carlos_10", "name": "Carlos Eduardo", "status": "available", "idle_seconds": 45 },
      { "id": "usr_ana_22", "name": "Ana Paula", "status": "available", "idle_seconds": 12 }
    ],
    "operators_count": 2,
    "active_dialing_channels": 6,
    "today_stats": {
      "total_calls": 420,
      "answered": 105,
      "voicemail": 180,
      "busy": 52,
      "failed": 23,
      "abandoned": 12,
      "delivery_rate_percentage": 25.0
    },
    "timestamp": "2026-09-14T02:16:10Z"
  }
}
```

#### Canal 3: `telephony:monitoring:call_log`
Log de progressão detalhada dos estados de telefonia da chamada:
```json
{
  "event": "telephony:monitoring:call_log",
  "data": {
    "call_id": "call-9b8c7d6e-5f4a",
    "phone": "(11) 99999-8888",
    "campaign_id": "camp_vendas_01",
    "step": "answered",
    "message": "Chamada atendida pelo cliente, iniciando análise acústica AMD",
    "error": null,
    "timestamp": "2026-09-14T02:15:06Z"
  }
}
```
Valores aceitos para o campo `step`:
- `placing`: Disparo da perna telefônica no tronco.
- `ringing`: Linha remota chamando (180/183 Session Progress).
- `answered`: Cliente atendeu (200 OK).
- `amd_analyzing`: Triagem acústica e Vosk STT em execução.
- `bridged`: Chamada conectada e áudio entregue ao operador / WebRTC.
- `hangup`: Término da chamada.
- `error`: Falha técnica, congestionamento ou indisponibilidade de canal.
