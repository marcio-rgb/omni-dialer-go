---
description: Procedimento canônico de deploy em produção do Dialer-Go via Git Forgejo, Control Plane e K3s.
---

# Deploy em Produção: Dialer-Go (Forgejo Git & K3s Control Plane)

Este workflow estabelece o procedimento operacional padrão para publicação e atualização em produção do microsserviço **`dialer-go`** no cluster K3s.

---

## 1. Princípios Arquiteturais e Invariantes

1. **Zero Deploy Manual via Portainer/SSH:** É terminantemente proibido utilizar Portainer, Docker Swarm ou comandos soltos via SSH para deploys em produção. Todas as ordens são emitidas para a API do **Control Plane** (`POST /api/v1/deploys`).
2. **Exclusão de Artefatos do Portainer:** Os scripts `deploy_github_portainer.js`, `deploy_github_portainer.sh` e `deploy_local_portainer.js` estão **descontinuados e devem ser excluídos/ignorados**.
3. **Alocação Topológica Determinística:** O `dialer-go` executa exclusivamente no **Node Master** (`dialer-new` - `84.247.135.255`), usufruindo de latência ultra-baixa de loopback com o Asterisk PBX e o banco `dialer_db`.
4. **Injeção Segura via Secret Vault:** Senhas de AMI, Asterisk e banco de dados trafegam exclusivamente em memória via Secret Vault (AES-256-GCM).
5. **Rastreabilidade Git Forgejo:** Qualquer deploy em produção deve ter origem em commit auditado na branch `main` do Git Forgejo.

---

## 2. Etapa 1: Validação Pré-Flight e Push no Git Forgejo

Antes de emitir a ordem de deploy:
```bash
# 1. Validar integridade dos testes de unidade
cd /home/marcio/ecosystem/dialer-go
go test -v ./...

# 2. Sincronizar com o Forgejo
git status
git fetch forgejo
git pull --rebase forgejo main

# 3. Enviar alterações validadas para o Forgejo
git push forgejo main
```

---

## 3. Etapa 2: Consulta de Integridade no Control Plane

Verifique se o serviço está registrado e sem colisões no catálogo do cluster:
```bash
curl -s http://localhost:3100/api/v1/discovery/dialer-go | jq
```
*Critério de Sucesso:* `serviceId: "dialer-go"`, `assigned_node: "node-master"`, status `ACTIVE`.

---

## 4. Etapa 3: Ordem de Deploy via Control Plane API

Dispare a ordem de publicação em produção:
```bash
curl -X POST http://localhost:3100/api/v1/deploys \
  -H "Content-Type: application/json" \
  -d '{
    "serviceId": "dialer-go",
    "environment": "production",
    "imageTag": "latest"
  }' | jq
```

**Retorno Esperado:**
```json
{
  "id": "dep-dialer-...",
  "serviceId": "dialer-go",
  "environment": "production",
  "status": "HEALTHY",
  "nodeId": "node-master",
  "port": 8080
}
```

---

## 5. Etapa 4: Validação Pós-Deploy

Execute o checklist operacional:
1. **Liveness Probe e Telemetria HTTP (RFC 7807):**
   ```bash
   curl -s http://84.247.135.255:8080/api/v1/health | jq
   ```
   *Critério de Sucesso:* Status `200` com `database: UP`, `redis: UP`, `ami: CONNECTED`.

2. **Qualify de Troncos SIP:**
   ```bash
   curl -s http://84.247.135.255:8080/api/v1/trunks | jq
   ```
   *Critério de Sucesso:* Lista de troncos cadastrados respondendo status de qualify ativo.

---

## 6. Procedimento de Rollback Imediato

Se houver qualquer instabilidade na nova versão:
```bash
curl -X POST http://localhost:3100/api/v1/deploys/dialer-go/rollback \
  -H "Content-Type: application/json" \
  -d '{}' | jq
```
O Control Plane restaura atomicamente a última revisão saudável.
