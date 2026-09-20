# Segurança de Rede, Isolamento de Dados e IP Whitelist

Este documento especifica os requisitos de segurança, autenticação de rede, tratamento padronizado de erros e isolamento de banco de dados do **Dialer-Go**.

---

## 1. Autenticação por IP Whitelist em Memória (`sync.Map`)

Por se tratar de um serviço interno de infraestrutura de alta frequência e baixa latência consumido pelo OmniChat e Estúdio IA:
1. **Sem JWT / Tokens de Sessão:** O discador descarta o overhead de validação de tokens criptográficos ou consultas a banco a cada chamada HTTP.
2. **Tabela de Hashing em Memória:**
   - O `IPWhitelistMiddleware` armazena faixas autorizadas (IPs simples e blocos CIDR) em um `sync.Map` Go.
   - A validação ocorre em tempo **sub-microssegundo** antes da execução de qualquer handler de rota.
3. **Extração de IP Real:**
   - Suporta cabeçalhos de reverse proxy confiáveis: `X-Forwarded-For`, `X-Real-IP` ou `RemoteAddr`.
4. **Rejeição com RFC 7807 (403 Forbidden):**
   - Requisições originadas de IPs não autorizados recebem resposta padronizada sem expor detalhes internos da infraestrutura:

```json
{
  "type": "https://dialer-go.omnichat.internal/errors/forbidden",
  "title": "Acesso Proibido",
  "status": 403,
  "detail": "O endereço IP de origem não está autorizado a interagir com a API do discador.",
  "code": "IP_NOT_WHITELISTED"
}
```

---

## 2. Isolamento de Base de Dados Relacional (`dialer_db`)

1. **Base Standalone Exclusiva:**
   - O discador conecta estritamente ao PostgreSQL `dialer_db`.
   - É terminantemente vedada a conexão ou realização de joins entre a base do discador e bancos de dados externos de CRM, chat ou pagamentos.
2. **Pool Transacional Resiliente (`pgx/v5`):**
   - Configuração de conexões máximas e mínimas dimensionadas para a capacidade de concorrência.
   - Sanitização estrita de inputs via consultas parametrizadas (`$1, $2, ...`), impedindo injeção de SQL.

---

## 3. Protocolo de Credenciais e Zero Hardcoding

1. **Proibição Absoluta de Segredos em Código:** Nenhuma credencial (AMI, PostgreSQL, Redis, MinIO) é inserida no repositório.
2. **Contrato Oficial:** O arquivo [`.env.example`](file:///home/marcio/ominichat/dialer-go/.env.example) é a única fonte da verdade de variáveis aceitas pelo discador.
3. **Injeção de Ambiente:** Em produção, a injeção ocorre exclusivamente via Docker Compose / Swarm secrets ou variáveis gerenciadas de orquestração.
