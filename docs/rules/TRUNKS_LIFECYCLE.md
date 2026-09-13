# Ciclo de Vida, Gestão e Monitoramento de Troncos SIP/PJSIP

Este documento estabelece as regras de negócio, ciclo de vida, hot reload a quente e salvaguardas operacionais para os troncos de telefonia SIP/PJSIP gerenciados pelo **Dialer-Go**.

---

## 1. Persistência Relacional de Troncos (Tabela `trunks`)

Os troncos de telefonia são mantidos na tabela `trunks` do PostgreSQL standalone (`dialer_db`):
- Modos de Registro suportados: `REGISTER` (autenticação digest usuário/senha) ou `IP_BASED` (autenticação por IP de origem/destino).
- Transportes suportados: `UDP`, `TCP`, `TLS`.
- Direções: `INBOUND`, `OUTBOUND`, `BIDIRECTIONAL`.
- Codecs suportados: `alaw`, `ulaw`, `g729`, `opus`.

---

## 2. Prefixo Técnico de Rota (`tech_prefix`)

1. **Definição:** Cadeia de caracteres numérica ou alfanumérica exigida por certas operadoras VoIP no início do número discado para fins de tarifação ou roteamento interno (ex: `0031`, `5511`).
2. **Injeção Transparente:** O discador aplica o `tech_prefix` no momento da montagem da perna de chamada no Asterisk sem alterar o número original armazenado no lead:

$$\text{DialString} = \text{"PJSIP/"} + \text{tech\_prefix} + \text{PhoneNumber} + \text{"@"} + \text{TrunkHost}$$

---

## 3. Hot Reload a Quente no Asterisk PBX

Ao cadastrar (`POST /api/v1/trunks`) ou atualizar (`PUT /api/v1/trunks/{trunk_id}`):
1. O registro é persistido de forma atômica no PostgreSQL.
2. O `TrunkManager` recompõe a configuração PJSIP em memória e dispara o comando AMI `Command(Action: Command, Command: pjsip reload)`.
3. **Invariante de Mídia:** O reload do PJSIP no Asterisk é dinâmico e nunca desconecta ou afeta chamadas que estejam em curso naquele ou em outros troncos.

---

## 4. Trava de Proteção Contra Exclusão de Tronco Ativo (HTTP 409)

1. Ao receber `DELETE /api/v1/trunks/{trunk_id}`, o sistema executa verificação prévia no `ChannelManager`:
2. Se `ActiveChannelsForTrunk(trunk_id) > 0`:
   - A requisição é rejeitada imediatamente com status **`409 Conflict`** (RFC 7807).
   - O payload informa a quantidade de chamadas ativas impedindo o encerramento forçado.
3. Se e somente se não houver chamadas ativas (`channels == 0`), a exclusão prossegue no banco e a rota é desprovisionada do Asterisk.

---

## 5. Limites de Capacidade por Tronco (`max_channels`)

* Cada tronco possui um parâmetro configurável de canais máximos simultâneos (`max_channels`).
* O `ChannelManager` mantém contadores atômicos (`sync/atomic.Int32`) por tronco:
  - `CanAcquireSlot(trunkID)` retorna `false` caso os canais ativos do tronco atinjam `max_channels`.
  - O motor de discagem pula para o próximo tronco elegível do pool caso o tronco primário esteja saturado.
