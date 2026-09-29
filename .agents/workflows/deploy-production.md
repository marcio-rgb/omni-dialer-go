---
description: Procedimento canônico de deploy em produção do Dialer-Go via Git Forgejo, Control Plane e K3s.
---

# Deploy em Produção: Dialer-Go (Forgejo Git & K3s Control Plane)

Este workflow estabelece o procedimento operacional padrão para publicação e atualização em produção do microsserviço **`dialer-go`** no cluster K3s.

---

## 1. Princípios Arquiteturais e Invariantes Kubernetes (K3s)

1. **Zero Deploy Manual via Portainer/SSH:** É terminantemente proibido utilizar Portainer, Docker Swarm ou comandos soltos via SSH para deploys em produção. Todas as ordens são emitidas declarativamente para a API do **Control Plane** (`POST /api/v1/deploys`).
2. **Exclusão de Artefatos Legados:** Os scripts `deploy_github_portainer.js`, `deploy_github_portainer.sh` e `deploy_local_portainer.js` estão **descontinuados e devem ser ignorados/excluídos**.
3. **Alocação Topológica Determinística no Cluster K3s:** 
   - O cluster é particionado em 4 nós físicos: `node-master` (VPS `84.247.135.255`), `node-worker-1`, `node-worker-2` e `node-worker-3`.
   - O `dialer-go` executa **exclusivamente no Node Master** (`node-master` - `84.247.135.255`), usufruindo de latência ultra-baixa (< 0.5ms) de loopback com o Asterisk PBX 20, o PostgreSQL (`dialer_db`) e o Redis.
4. **Estratégia Híbrida de Networking:**
   - **`hostNetwork: true`** para `asterisk-pbx` e `livekit-sip`: Elimina overhead de NAT/conntrack do iptables para milhares de pacotes UDP/s de RTP (10000-20000) e sinalização SIP (5060/5062).
   - **`hostNetwork: false`** com Ingress Traefik para o `dialer-go`: O Pod roda em rede interna `ClusterIP` (`dialer-go.omnichat.svc.cluster.local:8080`) e é publicado no Ingress seguro sob o domínio HTTPS `https://dialer.cbrpromotora.com`.
5. **Injeção Segura via Secret Vault:** Senhas de AMI, Asterisk e banco de dados trafegam exclusivamente em memória via Secret Vault com criptografia AES-256-GCM gerenciada pelo Control Plane.
6. **Rastreabilidade Git Forgejo:** Qualquer deploy em produção deve ter origem em commit auditado na branch `main` do Git Forgejo (`http://207.180.251.25:30080`).

---

## 2. Etapa 1: Validação Pré-Flight e Push no Git Forgejo

Antes de emitir a ordem de deploy:
```bash
# 1. Validar integridade dos testes de unidade
cd /home/marcio/ecosystem/dialer-go
docker run --rm -v /home/marcio/ecosystem/dialer-go:/app -w /app golang:1.25.0 go test -v -count=1 ./internal/...

# 2. Sincronizar com o Forgejo
git status
git fetch forgejo
git pull --rebase forgejo main

# 3. Enviar alterações validadas para o Forgejo
git push forgejo main
```

---

## 3. Etapa 2: Consulta de Integridade no Control Plane

Verifique se o serviço está registrado e sem colisões no catálogo do cluster K3s:
```bash
curl -s http://207.180.251.25:3100/api/v1/discovery/dialer-go | jq
```
*Critério de Sucesso:* `serviceId: "dialer-go"`, `assigned_node: "node-master"`, status `ACTIVE`.

---

## 4. Etapa 3: Ordem de Deploy via Control Plane API

Dispare a ordem de publicação em produção:
```bash
curl -X POST http://207.180.251.25:3100/api/v1/deploys \
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
  "id": "dep-1790436523942-tc97b",
  "serviceId": "dialer-go",
  "environment": "production",
  "status": "HEALTHY",
  "nodeId": "node-master",
  "details": "Applied deployment 'dialer-go' to namespace 'omnichat' on cluster https://84.247.135.255:6443"
}
```

---

## 5. Etapa 4: Validação Pós-Deploy

Execute o checklist operacional:
1. **Liveness Probe e Telemetria HTTP Pública:**
   ```bash
   curl -s https://dialer.cbrpromotora.com/health | jq
   ```
   *Critério de Sucesso:* Status `200` com `database: true`, `cache: true`.

2. **Validação de Proteção IP Whitelist & CDRs (RFC 7807):**
   ```bash
   curl -s -i https://dialer.cbrpromotora.com/api/v1/cdrs
   ```
   *Critério de Sucesso:* Resposta RFC 7807 (`application/problem+json`) indicando `IP_NOT_WHITELISTED` para origens não autorizadas ou listagem paginada para nós autorizados.

3. **Qualify de Troncos SIP:**
   ```bash
   curl -s -H "X-Tenant-Id: default" https://dialer.cbrpromotora.com/api/v1/trunks | jq
   ```
   *Critério de Sucesso:* Lista de troncos cadastrados com telemetria RTT ativa.

---

## 6. Procedimento de Rollback Imediato

Se houver qualquer instabilidade na nova versão:
```bash
curl -X POST http://207.180.251.25:3100/api/v1/deploys/dialer-go/rollback \
  -H "Content-Type: application/json" \
  -d '{}' | jq
```
O Control Plane restaura atomicamente a última revisão saudável no namespace `omnichat` do K3s.
