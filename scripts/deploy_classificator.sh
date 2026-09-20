#!/usr/bin/env bash
# ==============================================================================
# Script de Deploy e Hot-Reload Zero-Downtime do Classificator
# ==============================================================================
set -euo pipefail

STACK_NAME="classificator"
COMPOSE_FILE="docker-compose.classificator.yml"
ROUTER_STATUS_URL="http://127.0.0.1:2809/status"

echo "=== [CLASSIFICATOR] Iniciando orquestracao da stack ==="

# Verifica se o Docker Swarm esta ativo
if docker info 2>/dev/null | grep -q "Swarm: active"; then
    echo ">> Ambiente Docker Swarm detectado. Realizando deploy da stack ${STACK_NAME}..."
    docker stack deploy -c "${COMPOSE_FILE}" "${STACK_NAME}"
else
    echo ">> Ambiente Docker Compose detectado. Subindo servicos..."
    docker compose -f "${COMPOSE_FILE}" up -d
fi

echo ">> Aguardando inicializacao dos servicos..."
sleep 3

# Valida o status do Router
if command -v curl >/dev/null 2>&1; then
    echo ">> Consultando status do anel de portas no Router..."
    if curl -s -f "${ROUTER_STATUS_URL}" 2>/dev/null | grep -q "active_address"; then
        echo ">> Router ativo e operacional:"
        curl -s "${ROUTER_STATUS_URL}" || true
        echo ""
    else
        echo ">> [AVISO] Router ainda nao respondeu no status HTTP. Verifique os logs com: docker service logs classificator_classificator-router"
    fi
fi

echo "=== [CLASSIFICATOR] Deploy finalizado com sucesso! ==="
