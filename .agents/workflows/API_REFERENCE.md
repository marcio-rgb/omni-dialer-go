# Dialer-Go: Catálogo Canônico da API REST (Entrada e Saída de Dados)

Este documento estabelece a especificação exaustiva de todas as rotas, contratos de entrada (payloads, headers, query parameters), contratos de saída (respostas de sucesso e erros padronizados RFC 7807), restrições de validação e comportamentos operacionais do **Dialer-Go**.

---

## 1. Diretrizes Globais de Comunicação e Segurança

### 1.1. Base URL e Versionamento
- **Prefixo de Rota:** `/api/v1`
- **Porta Padrão:** `8080` (configurável via variável `PORT`)
- **Protocolo:** HTTP/1.1 (suporte a proxies reversos NGINX, Traefik ou Cloudflare)

### 1.2. Segurança e Controle de Acesso
- **Autorização por IP Whitelist em Memória:** O discador não utiliza tokens JWT ou sessões para evitar overhead na telefonia. Cada requisição é validada em sub-microssegundo consultando uma tabela hash thread-safe (`sync.Map`).
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
  "type": "https://api.ominichat.com/errors/invalid-date-range",
  "title": "Unprocessable Entity",
  "status": 422,
  "detail": "A data inicial 'start_date' não pode ser posterior à data final 'end_date'.",
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
| `POST` | `/api/v1/predictive/demand` | Processa a demanda de agentes e calcula disparos preditivos | Whitelist + Body (`tenant_id`) |
| `POST` | `/api/v1/calls/manual` | Origina chamada manual com prioridade preemptiva | Whitelist + Body (`tenant_id`) |
| `POST` | `/api/v1/campaigns/refill` | Ingestão em streaming de lote de mailing via ZIP do MinIO | Whitelist + Body (`tenant_id`) |
| `POST` | `/api/v1/campaigns/toggle` | Ativa ou pausa uma campanha (timeout síncrono de 15s) | Whitelist + Body (`tenant_id`) |
| `GET` | `/api/v1/campaigns/{campaign_id}/saturation` | Consulta métricas de saturação e reciclagem FILO de campanha | Whitelist + `X-Tenant-Id` |
| `GET` | `/api/v1/campaigns/saturation` | Retorna visão consolidada de saturação de todas as campanhas | Whitelist + `X-Tenant-Id` |
| `GET` | `/api/v1/reports/calls-summary` | Estatísticas agregadas de chamadas com buffer de 15 minutos | Whitelist + `X-Tenant-Id` ou Query |
| `GET` | `/api/v1/trunks` | Lista troncos com telemetria RTT e canais ativos em tempo real | Whitelist + `X-Tenant-Id` |
| `POST` | `/api/v1/trunks` | Cadastra novo tronco telefônico com recarga dinâmica no PBX | Whitelist + Body (`tenant_id`) |
| `PUT` | `/api/v1/trunks/{trunk_id}` | Atualiza parâmetros operacionais do tronco a quente | Whitelist + `X-Tenant-Id` |
| `DELETE` | `/api/v1/trunks/{trunk_id}` | Exclui tronco telefônico (com proteção de canais ativos) | Whitelist + `X-Tenant-Id` |
| `GET` | `/api/v1/trunks/enumerations` | Lista opções canônicas de codecs, transportes e modos de registro | Aberto na Whitelist |
| `GET` | `/health` | Diagnóstico de integridade (AMI, Postgres, Redis, Canais) | Sem bloqueio de IP |

---

## 3. Especificação Detalhada por Endpoint

---

### 3.1. Processar Demanda Preditiva (`POST /api/v1/predictive/demand`)
Disparado ciclicamente pelo OmniChat Core ou Estúdio IA para informar a quantidade de operadores disponíveis e solicitar que o motor preditivo dimensione o ritmo de originação telefônica.

#### A. Headers:
```http
Content-Type: application/json
```

#### B. Request Body:
| Campo | Tipo | Obrigatório | Descrição / Regra |
| :--- | :--- | :--- | :--- |
| `tenant_id` | `string` | **Sim** | Identificador multi-tenant da organização. |
| `campaign_id` | `string` | **Sim** | Identificador UUID da campanha cujos leads serão consumidos. |
| `available_agents` | `array[object]` | **Sim** | Lista de atendentes humanos ou agentes virtuais prontos para receber chamada. |
| `available_agents[].agent_id` | `string` | **Sim** | Identificador do operador. |
| `available_agents[].priority_order` | `integer` | Não | Ordem de prioridade de distribuição (default: 1). |
| `available_agents[].sip_route` | `string` | **Sim** | Rota SIP/LiveKit para onde a perna de áudio será transferida. |
| `available_agents[].idle_time_seconds` | `integer` | Não | Tempo em que o operador está ocioso. |

##### Exemplo de Entrada:
```json
{
  "tenant_id": "org_alpha",
  "campaign_id": "camp_vendas_01",
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
| Campo | Tipo | Descrição |
| :--- | :--- | :--- |
| `success` | `boolean` | `true` |
| `data.campaign_id` | `string` | Identificador da campanha processada. |
| `data.dialing_channels` | `integer` | Quantidade de chamadas disparadas simultaneamente via AMI. |
| `data.status` | `string` | `"active"`, `"paused"` (se travada via toggle) ou `"idle_no_agents"`. |

##### Exemplo de Saída:
```json
{
  "success": true,
  "data": {
    "campaign_id": "camp_vendas_01",
    "dialing_channels": 4,
    "status": "active"
  }
}
```

#### D. Erros Mapeados:
- **`400 Bad Request` (`MISSING_REQUIRED_FIELDS`):** `tenant_id` ou `campaign_id` não informados.
- **`400 Bad Request` (`TRUNK_UNAVAILABLE`):** Tronco vinculado à campanha está desabilitado.
- **`404 Not Found` (`CAMPAIGN_NOT_FOUND`):** Campanha inexistente.

---

### 3.2. Discagem Manual Preemptiva (`POST /api/v1/calls/manual`)
Permite que um operador humano dispare uma ligação pontual com **prioridade preemptiva imediata**, suspendendo temporariamente novos disparos preditivos para garantir o canal.

#### A. Headers:
```http
Content-Type: application/json
```

#### B. Request Body:
| Campo | Tipo | Obrigatório | Descrição / Regra |
| :--- | :--- | :--- | :--- |
| `tenant_id` | `string` | **Sim** | Identificador multi-tenant. |
| `agent_id` | `string` | **Sim** | Identificador do operador que iniciou a chamada. |
| `phone` | `string` | **Sim** | Número telefônico de destino no formato E.164 ou nacional. |
| `sip_route` | `string` | **Sim** | Endpoint SIP/WebRTC do operador para conexão direta. |
| `trunk_id` | `string` | Não | Tronco específico desejado. Se omitido, usa o tronco padrão do tenant. |

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
- **`429 Too Many Requests` (`TRUNK_CHANNELS_EXHAUSTED`):** O tronco atingiu seu limite `max_channels`. Retorna metadados de canais ativos.

---

### 3.3. Ingestão de Mailing via ZIP (`POST /api/v1/campaigns/refill`)
Realiza o download em streaming de um arquivo compactado do storage MinIO/S3, valida o layout canônico de 4 colunas e enfileira os leads no banco relacional e no Redis.

#### A. Headers:
```http
Content-Type: application/json
```

#### B. Request Body:
| Campo | Tipo | Obrigatório | Descrição / Regra |
| :--- | :--- | :--- | :--- |
| `tenant_id` | `string` | **Sim** | Identificador do tenant. |
| `campaign_id` | `string` | **Sim** | Identificador da campanha a ser reabastecida. |
| `file_uri` | `string` | **Sim** | URI do pacote compactado no MinIO (`s3://mailings/camp101/base.zip`). |

##### Regras de Validação do Arquivo:
1. O ZIP deve conter um arquivo `.csv` delimitado por vírgula.
2. Layout obrigatório de 4 colunas: `cpf,telefone,campanha_id,tenant_id`.
3. **Regra de Corrupção Total:** Se as **30 primeiras linhas** consecutivas forem inválidas ou divergirem de `campaign_id`/`tenant_id`, a requisição é cancelada imediatamente.

##### Exemplo de Entrada:
```json
{
  "tenant_id": "org_alpha",
  "campaign_id": "camp_vendas_01",
  "file_uri": "s3://mailings/camp_vendas_01/lote_setembro.zip"
}
```

#### C. Resposta de Sucesso (`200 OK`):
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

#### D. Erros Mapeados:
- **`400 Bad Request` (`CORRUPTED_FILE`):** As 30 primeiras linhas são inválidas ou violam o layout canônico.
- **`400 Bad Request` (`INVALID_ZIP_ARCHIVE`):** Arquivo compactado corrompido.
- **`400 Bad Request` (`NO_CSV_IN_ZIP`):** Nenhum arquivo CSV encontrado dentro do ZIP.

---

### 3.4. Ativar / Desativar Campanha (`POST /api/v1/campaigns/toggle`)
Processo síncrono com **timeout estrito de 15 segundos** para pausar ou retomar uma campanha imediatamente.

#### A. Headers:
```http
Content-Type: application/json
```

#### B. Request Body:
| Campo | Tipo | Obrigatório | Descrição / Regra |
| :--- | :--- | :--- | :--- |
| `campaign_id` | `string` | **Sim** | Identificador da campanha. |
| `tenant_id` | `string` | **Sim** | Identificador do tenant. |
| `enable` | `boolean` | **Sim** | `true` para ativar/retomar; `false` para pausar. |

##### Exemplo de Entrada:
```json
{
  "campaign_id": "camp_vendas_01",
  "tenant_id": "org_alpha",
  "enable": false
}
```

#### C. Resposta de Sucesso (`200 OK`):
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

#### D. Resposta de Erro (`404 Not Found`):
```json
{
  "success": false,
  "error": {
    "code": "CAMPAIGN_NOT_FOUND",
    "message": "A campanha solicitada não existe ou foi removida."
  }
}
```

---

### 3.5. Consultar Saturação de Campanha Individual (`GET /api/v1/campaigns/{campaign_id}/saturation`)
Retorna os indicadores de queima da base, ciclos FILO e urgência de refill da campanha. **Não retorna `campaign_name`** (somente `campaign_id`).

#### A. Headers:
```http
X-Tenant-Id: org_alpha
```

#### B. Path Parameters:
- `campaign_id` (`string`, obrigatório): UUID da campanha.

#### C. Resposta de Sucesso (`200 OK`):
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

### 3.6. Consultar Saturação Consolidada do Tenant (`GET /api/v1/campaigns/saturation`)
Retorna a síntese de todas as campanhas ativas do tenant com contadores de criticidade.

#### A. Headers:
```http
X-Tenant-Id: org_alpha
```

#### B. Resposta de Sucesso (`200 OK`):
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

### 3.7. Relatório Analítico de Chamadas (`GET /api/v1/reports/calls-summary`)
Endpoint de alta performance protegido por uma camada de **Buffer de Cache no Redis com Validade Estrita de 15 Minutos (900 Segundos)**. Os dados só sofrem recálculo no banco a cada 15 minutos por combinação de filtros.

#### A. Parâmetros da Requisição:
- **Tenant Obrigatório:** Deve ser informado via Query Parameter `?tenant_id=...` **ou** via Header HTTP `X-Tenant-Id: ...`.
- **Query Parameters:**

| Parâmetro | Local | Tipo | Obrigatório | Padrão | Descrição |
| :--- | :--- | :--- | :--- | :--- | :--- |
| `tenant_id` | Query / Header | `string` | **Sim** | — | Identificador do tenant. |
| `start_date` | Query | `string (ISO 8601)` | Não | Início do dia `00:00:00 UTC` | Data e hora inicial da apuração analítica. |
| `end_date` | Query | `string (ISO 8601)` | Não | Instante atual (`now()`) | Data e hora final da apuração analítica. |
| `campaign_id` | Query | `string (UUID)` | Não | Todas as campanhas | Filtra métricas de uma campanha específica. |

#### B. Resposta de Sucesso (`200 OK`):
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
      "invalid_number_breakdown": {
        "unallocated_number_calls": 820,
        "incomplete_number_calls": 260,
        "unassigned_service_calls": 120
      },
      "failure_breakdown": {
        "congestion_calls": 210,
        "timeout_calls": 130,
        "carrier_error_calls": 95,
        "trunk_limit_exhausted_calls": 45
      },
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
      "buffer_duration_seconds": 900,
      "refresh_interval_minutes": 15,
      "last_refresh_reason": "BUFFER_HIT"
    }
  }
}
```

#### C. Erros Mapeados:
- **`400 Bad Request` (`MISSING_TENANT_ID`):** `tenant_id` não informado.
- **`422 Unprocessable Entity` (`INVALID_DATE_RANGE`):** `start_date` posterior a `end_date`.
- **`422 Unprocessable Entity` (`INVALID_DATE_FORMAT`):** Data fora do padrão ISO 8601.

---

### 3.8. Listagem de Troncos com Telemetria (`GET /api/v1/trunks`)
Retorna todos os troncos cadastrados enriquecidos com a latência RTT em milissegundos e contadores de canais em tempo real.

#### A. Headers:
```http
X-Tenant-Id: org_alpha
```

#### B. Resposta de Sucesso (`200 OK`):
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

### 3.9. Cadastrar Tronco SIP/PJSIP (`POST /api/v1/trunks`)
Cadastra novo tronco telefônico com suporte a autenticação por registro ou IP, prefixo técnico (`tech_prefix`) e sincronização dinâmica com o PBX Asterisk.

#### A. Headers:
```http
Content-Type: application/json
```

#### B. Request Body:
| Campo | Tipo | Obrigatório | Padrão | Descrição |
| :--- | :--- | :--- | :--- | :--- |
| `id` | `string` | **Sim** | — | Identificador único do tronco (`trunk_vivo_01`). |
| `tenant_id` | `string` | **Sim** | — | Identificador multi-tenant. |
| `name` | `string` | **Sim** | — | Nome amigável do tronco. |
| `host` | `string` | **Sim** | — | IP ou FQDN da operadora VoIP. |
| `port` | `integer` | Não | `5060` | Porta SIP. |
| `outbound_proxy` | `string` | Não | `null` | Proxy de saída (`sip:proxy.operadora.com:5060;lr`). |
| `tech_prefix` | `string` | Não | `null` | Prefixo técnico prepended ao número (ex: `55001`, `0021`). |
| `registration_mode`| `string` | Não | `IP_BASED` | `REGISTER` ou `IP_BASED`. |
| `auth_username` | `string` | Condicional | `null` | Obrigatório se `registration_mode = REGISTER`. |
| `auth_password` | `string` | Condicional | `null` | Senha SIP de autenticação. |
| `transport` | `string` | Não | `UDP` | `UDP`, `TCP` ou `TLS`. |
| `nat_mode` | `string` | Não | `FORCE_RPORT`| `FORCE_RPORT`, `YES`, `NO` ou `COMEDIA`. |
| `codecs` | `array[string]`| Não | `["alaw", "ulaw"]` | Codecs autorizados (`alaw`, `ulaw`, `g729`, `opus`, `gsm`). |
| `dtmf_mode` | `string` | Não | `RFC4733` | `RFC4733`, `RFC2833`, `INBAND`, `INFO`, `AUTO`. |
| `qualify_frequency`| `integer` | Não | `30` | Intervalo em segundos entre pings OPTIONS. |
| `qualify_timeout` | `float` | Não | `3.0` | Timeout do ping OPTIONS em segundos. |
| `max_channels` | `integer` | Não | `30` | Cota máxima de ligações simultâneas autorizadas. |
| `is_enabled` | `boolean` | Não | `true` | Status operacional do tronco. |

##### Exemplo de Entrada:
```json
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
  "codecs": ["alaw", "ulaw", "g729"],
  "max_channels": 45,
  "is_enabled": true
}
```

#### C. Resposta de Sucesso (`201 Created`):
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

### 3.10. Atualizar Tronco Telefônico (`PUT /api/v1/trunks/{trunk_id}`)
Atualiza parâmetros de sinalização, mídia, credenciais, limites simultâneos ou prefixo técnico com **recarga a quente no Asterisk** sem derrubar chamadas em andamento.

#### A. Headers:
```http
X-Tenant-Id: org_alpha
Content-Type: application/json
```

#### B. Request Body (Campos Parciais):
```json
{
  "name": "Vivo E1 SIP Principal (Alta Capacidade)",
  "tech_prefix": "55001",
  "codecs": ["alaw", "ulaw", "g729"],
  "max_channels": 60,
  "is_enabled": true
}
```

#### C. Resposta de Sucesso (`200 OK`):
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

### 3.11. Excluir Tronco Telefônico (`DELETE /api/v1/trunks/{trunk_id}`)
Remove o tronco da base relacional e do PBX Asterisk.

#### A. Query Parameters:
- `force` (`boolean`, opcional, padrão: `false`): Se `false` e houver chamadas ativas, a exclusão é rejeitada com `409 Conflict`. Se `true`, derruba forçadamente as pernas ativas antes de excluir.

#### B. Resposta de Sucesso (`200 OK`):
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

#### C. Resposta de Erro de Conflito (`409 Conflict`):
```json
{
  "type": "https://api.ominichat.com/errors/conflict",
  "title": "Conflict",
  "status": 409,
  "detail": "O tronco possui 4 chamadas ativas em andamento. Envie ?force=true para encerramento forçado imediato.",
  "code": "ACTIVE_CHANNELS_PRESENT",
  "metadata": {
    "active_channels_count": 4
  }
}
```

---

### 3.12. Opções Canônicas de Configuração (`GET /api/v1/trunks/enumerations`)
Fornece aos frontends a lista canônica e tipada de todos os parâmetros avançados aceitos pelo discador.

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

### 3.13. Diagnóstico de Saúde do Discador (`GET /health`)
Endpoint público de monitoramento e liveness probe (Kubernetes / Docker). Não exige verificação na lista branca de IPs.

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
