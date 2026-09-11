---
trigger: always_on
glob: "**/*.go"
description: Referência canônica dos endpoints, contratos REST e schemas de telemetria do Dialer-Go
---

# Catálogo Canônico da API REST do Dialer-Go

Para a especificação completa de payloads, headers, query parameters e respostas RFC 7807, consulte o documento mestre:
- [API_REFERENCE.md](file:///home/marcio/ominichat/dialer-go/.agents/workflows/API_REFERENCE.md)

## Principais Endpoints Monitorados:
- `POST /api/v1/predictive/demand`: Ingestão de operadores ociosos e cálculo de overdialing.
- `POST /api/v1/manual/dial`: Originação de discagem manual com prioridade preemptiva.
- `POST /api/v1/mailing/refill`: Ingestão e carga de lotes de leads para campanhas.
- `GET /api/v1/trunks`: Telemetria em tempo real dos troncos SIP (Status, Latência, Canais Ativos).
- `GET /api/v1/health`: Verificação de integridade dos componentes (Postgres, Redis, Asterisk AMI).
