# Dialer-Go — Motor de Telefonia Preditiva, Receptiva e Manual

> **Componente Canônico do Ecossistema OmniChat (`omnichat-ecosystem`)**  
> **Linguagem:** 100% Go (`go 1.25.0`) | **PBX:** Asterisk 20 LTS (AMI TCP + PJSIP) | **Mídia/IA:** LiveKit WebRTC & Vosk STT  
> **Status:** Canônico / Produção

---

## 1. Visão Geral

O **Dialer-Go** (`dialer-go/`) é o subsistema central de telefonia de alto rendimento e ultrabaixa latência do ecossistema OmniChat:
- **Triagem AMD Ultrarrápida (< 1,5s):** Combinação do módulo Asterisk `app_amd` com transcrição de áudio em tempo real via script EAGI Vosk STT (`cmd/vosk-eagi`).
- **Pacing Preditivo Auto-balanceado:** Overdialing dinâmico que mantém ocupação ideal de operadores humanos e agentes virtuais prevenindo abandono regulatório (< 2s).
- **Lookup Receptivo O(1):** Roteamento em sub-milissegundo de chamadas entrantes via tabela indexada `phone_trunk_mappings`.
- **Salvaguarda de Operadores Humanos:** Quota rígida de canais globais reservados para chamadas manuais (`HumanReserveQuota`).
- **Segurança Sub-microssegundo:** Autorização REST baseada em tabela hash em memória (`sync.Map`) via IP Whitelist, sem handshakes pesados.
- **Isolamento de Persistência:** Base relacional standalone (`dialer_db`), blindando o discador contra concorrência de I/O com o CRM.

---

## 2. Conexão ao Repositório Git Forgejo

O repositório canônico de controle de versão reside no **Git Forgejo** do cluster K3s.

### 2.1. Endereços do Repositório:
- **HTTP (NodePort 30080):** `http://207.180.251.25:30080/marcio/omnichat-ecosystem.git`
- **SSH (NodePort 30222):** `ssh://git@84.247.135.255:30222/marcio/omnichat-ecosystem.git`
- **DNS Cluster K3s:** `http://forgejo.omnichat.svc.cluster.local:3000/marcio/omnichat-ecosystem.git`

### 2.2. Protocolo Pre-Flight Git:
Na raiz do monorepo (`/home/marcio/ecosystem`):
```bash
# 1. Verificar estado da árvore local
git status

# 2. Sincronizar com o Forgejo antes de alterar qualquer código
git fetch forgejo
git pull --rebase forgejo main

# 3. Criar branch de funcionalidade
git checkout -b feature/minha-melhoria-telefonia
```

---

## 3. Como Operar em Desenvolvimento (Dev)

### 3.1. Modo 1: Local Híbrido com Docker Compose
Ideal para execução e testes de regras do discador e banco de dados:
```bash
cd /home/marcio/ecosystem/dialer-go
docker compose -f docker-compose.local.yml up -d
```
Para rodar os testes de unidade:
```bash
go test -v ./...
```
Para compilar os binários:
```bash
go build -o bin/dialer ./cmd/dialer
go build -o bin/vosk-eagi ./cmd/vosk-eagi
```

### 3.2. Modo 2: Sandbox Efêmero no Cluster K3s (Worker 3)
Permite validar alterações em ambiente de cluster real no nó isolado **Worker 3** (`38.242.219.186`):
```bash
curl -X POST http://localhost:3100/api/v1/deploys \
  -H "Content-Type: application/json" \
  -d '{
    "serviceId": "dialer-go",
    "environment": "test",
    "imageTag": "feature-minha-branch"
  }'
```

---

## 4. Como Fazer Deploy em Produção (Prod)

O deploy de produção é orquestrado de forma centralizada pelo **Control Plane** no cluster **K3s**.

### 4.1. Passo a Passo do Deploy:
1. **Commit e Push no Git Forgejo:**
   ```bash
   git checkout main
   git pull --rebase forgejo main
   git merge feature/minha-melhoria-telefonia
   git push forgejo main
   ```
2. **Disparo de Deploy via API do Control Plane:**
   ```bash
   curl -X POST http://localhost:3100/api/v1/deploys \
     -H "Content-Type: application/json" \
     -d '{
       "serviceId": "dialer-go",
       "environment": "production",
       "imageTag": "latest"
     }'
   ```
3. **Alocação Topológica:** O `dialer-go` roda no **Node Master** (`dialer-new` - `84.247.135.255`), conectado diretamente ao Asterisk PBX local e ao PostgreSQL `dialer_db`.
4. **Injeção de Segredos:** O Control Plane injeta em tempo de execução via **Secret Vault (AES-256-GCM)** as credenciais AMI, senhas do banco e chaves de acesso.
5. **Rollback Automático ou Sob Demanda:**
   ```bash
   curl -X POST http://localhost:3100/api/v1/deploys/dialer-go/rollback \
     -H "Content-Type: application/json" \
     -d '{}'
   ```

---

## 5. Exclusão Obrigatória de Artefatos Legados do Portainer

> [!CAUTION]
> **Atenção Desenvolvedores e Agentes de IA:**  
> É **terminantemente PROIBIDO** utilizar o Portainer ou Docker Swarm para o deploy do Dialer-Go. Todos os artefatos legados associados ao Portainer (`deploy_github_portainer.js`, `deploy_github_portainer.sh`, `deploy_local_portainer.js`, bem como variáveis `PORTAINER_URL` e `PORTAINER_KEY`) estão formalmente **DESCONTINUADOS e DEVEM SER EXCLUÍDOS/IGNORADOS**. O único mecanismo de deploy aprovado e suportado é o **Control Plane API** sobre o cluster **K3s**.

---

## 6. Estrutura de Diretórios

```text
dialer-go/
├── cmd/
│   ├── dialer/              # Ponto de entrada do serviço HTTP e orquestrador
│   └── vosk-eagi/           # Binário EAGI executado pelo Asterisk para triagem Vosk STT
├── config/                  # Carregamento e validação de variáveis de ambiente
├── database/                # Schema DDL (tabelas e stored procedures ACID)
├── docs/                    # Grafo do projeto (MAP.md) e regras de governança
├── internal/
│   ├── domain/              # Entidades puras, DTOs, Enums e erros RFC 7807
│   ├── ports/               # Interfaces canônicas (AMI, Cache, Repositories, Storage)
│   ├── core/                # Regras de negócio, motores e gerenciador de canais
│   └── adapters/            # Implementações concretas (HTTP Chi, PostgreSQL, Redis, AMI)
└── .agents/                 # Governança, ARCHITECT.md, regras e workflows
```

---

## 7. Referências e Governança Canônica

- **Arquitetura Mestre do Ecossistema:** [`ecosystem/ARCHITECT.md`](file:///home/marcio/ecosystem/ecosystem/ARCHITECT.md)
- **Workflow de Deploy em Produção:** [`.agents/workflows/deploy-production.md`](file:///home/marcio/ecosystem/dialer-go/.agents/workflows/deploy-production.md)
- **Workflow de Desenvolvimento e Testes:** [`.agents/workflows/deploy-local.md`](file:///home/marcio/ecosystem/dialer-go/.agents/workflows/deploy-local.md)
- **Regras de Git Pre-Flight:** [`.agents/rules/GIT_PREFLIGHT.md`](file:///home/marcio/ecosystem/dialer-go/.agents/rules/GIT_PREFLIGHT.md)
