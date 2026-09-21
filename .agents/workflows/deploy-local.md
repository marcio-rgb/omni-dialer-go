---
description: Procedimento operacional de desenvolvimento e testes do Dialer-Go (Local e Sandbox Efêmero no K3s).
---

# Desenvolvimento e Testes: Dialer-Go (Local & Sandboxes K3s)

Este workflow estabelece os procedimentos para desenvolvimento diário, testes de unidade e execução de ambientes efêmeros do **Dialer-Go**.

---

## 1. Topologia de Ambientes

| Ambiente | Execução | Alocação | Finalidade |
| :--- | :--- | :--- | :--- |
| **Local Híbrido** | Host Nativo (`go run`) + Compose de Apoio | Máquina de Desenvolvimento | Ciclo rápido de TDD, desenvolvimento de motores de pacing e testes de dialplan. |
| **Sandbox K3s Efêmero** | Namespace `test-dialer-go` no K3s | Worker 3 (`38.242.219.186`) | Testes de integração E2E com o OmniChat Server e Asterisk sem impacto em produção. |

---

## 2. Modo 1: Desenvolvimento Local Híbrido

### 2.1. Subir Serviços de Apoio:
```bash
cd /home/marcio/ecosystem/dialer-go
docker compose -f docker-compose.local.yml up -d
```
- **PostgreSQL (`dialer_db`):** Porta local `5432`
- **Redis:** Porta local `6379`
- **Vosk EAGI Server:** Porta local `2700`

### 2.2. Executar Testes Unitários:
```bash
go test -v -race ./...
```

### 2.3. Executar o Discador Localmente:
```bash
go run ./cmd/dialer/main.go
```
*O servidor inicializa com rotas ativas na porta `8080`.*

---

## 3. Modo 2: Deploy Efêmero em Sandbox K3s (Worker 3)

Para testar uma branch de telefonia em ambiente de cluster isolado:
```bash
# 1. Enviar branch ao Git Forgejo
git checkout -b feature/minha-melhoria
git commit -m "feat(dialer): novo algoritmo de pacing"
git push forgejo feature/minha-melhoria

# 2. Requisitar deploy de teste ao Control Plane
curl -X POST http://localhost:3100/api/v1/deploys \
  -H "Content-Type: application/json" \
  -d '{
    "serviceId": "dialer-go",
    "environment": "test",
    "imageTag": "feature-minha-melhoria"
  }' | jq
```

---

## 4. Diretriz de Exclusão do Portainer Local

> [!WARNING]
> O método legado de deploy via script `deploy_local_portainer.js` e Portainer Web Editor foi **completamente descontinuado**. Não utilize mais scripts de Portainer. Utilize exclusivamente o Docker Compose local para dependências ou o Control Plane API para testes em cluster.
