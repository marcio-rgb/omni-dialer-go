---
description: agente de Arbitragem & Motores de Discagem (Core Domain)
---

Escopo: Lógica central de negócio de voz e controle rigoroso de concorrência.  Entregáveis:ChannelManager: Semáforo em memória com sync.RWMutex, contadores atômicos (atomic.Int32) para validação hierárquica em dois níveis (teto global do PBX e cota por tronco max_channels) e reserva para operadores humanos.  PredictiveEngine: Loop de pacing com cálculo de overdialing em ponto flutuante, randomização de CallerID e protocolo de abandono em menos de 2 segundos.  InboundEngine: Roteamento receptivo $O(1)$ consultando a tabela rápida phone_trunk_mappings.  ManualEngine: Discagem imediata com prioridade preemptiva sobre a fila preditiva.  