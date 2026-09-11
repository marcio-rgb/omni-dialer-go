#!/bin/bash
# ========================================================
#   PUBLISHING OMNI-DIALER-GO TO GITHUB & PORTAINER
# ========================================================

set -e

# Executa o orquestrador Node.js repassando os parâmetros
node "$(dirname "$0")/deploy_github_portainer.js" "$@"
