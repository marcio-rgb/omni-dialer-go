# Políticas Telefônicas: Pacing, Overdialing, Quota e Triagem AMD

Este documento especifica as regras de negócio, algoritmos matemáticos e restrições regulatórias para a discagem automatizada do **Dialer-Go**.

---

## 1. Algoritmo de Pacing e Equações de Overdialing

O motor preditivo (`internal/core/predictive_engine.go`) tem como objetivo manter a ocupação máxima dos operadores humanos ou salas virtuais sem gerar abandono regulatório de chamadas.

### 1.1. Equação de Overdialing Dinâmico
O cálculo do número de canais a disparar simultaneamente em cada rodada de pacing segue a equação:

$$\text{ChannelsToDial} = \max\left(1, \text{round}\left(\frac{\text{AvailableAgents} \times \text{Aggressiveness}}{\text{ContactProbability}}\right)\right)$$

Onde:
* $\text{AvailableAgents}$: Quantidade de operadores com status livre/ocioso (`idle`) associados à campanha no Redis.
* $\text{Aggressiveness}$: Fator multiplicador configurado na campanha ou enviado dinamicamente no payload `PredictiveDemandRequest` (`aggressiveness`) (padrão: `1.20`, variando de `1.00` a `2.50`).
* $\text{ContactProbability}$: Taxa histórica móvel de sucesso de atendimento humano (padrão conservador de boot: `0.30` ou 30%).
* $\text{ChannelsToDial}$: Quantidade de chamadas simultâneas que o discador tentará originar no Asterisk.

---

## 2. Quota Garantida de Canais para Operadores Humanos (`HumanReserveQuota`)

Para impedir que campanhas preditivas esgotem os recursos de tronco e impeçam operadores manuais de realizar ligações críticas:
* **Capacidade Global Padrão (`MAX_GLOBAL_CHANNELS`):** 60 canais.
* **Quota Reservada Humana (`HUMAN_RESERVED_QUOTA`):** 10 canais.
* **Limite Efetivo para o Motor Preditivo:**

$$\text{MaxPredictiveSlots} = \text{MAX\_GLOBAL\_CHANNELS} - \text{HUMAN\_RESERVED\_QUOTA} = 50 \text{ canais}$$

Se $\text{ActiveChannels} \ge 50$, novas tentativas de discagem preditiva são suspensas até que canais sejam liberados, enquanto requisições manuais (`POST /api/v1/calls/manual`) continuam sendo atendidas até o teto absoluto de 60 canais.

---

## 3. Protocolo de Abandono Regulatório (< 2 Segundos)

Conforme as diretrizes regulatórias de telecomunicações:
1. **Cronômetro de Entrega:** O tempo entre o cliente atender a ligação (evento `200 OK` / `Answer`) e a conexão com um operador humano não pode exceder **2,0 segundos**.
2. **Ação em Caso de Esgotamento:** Se nenhum operador estiver disponível para receber a chamada humana detectada em até 2 segundos, o discador deve:
   - Registrar imediatamente a chamada com status `disposition = 'ABANDONED'` e `hangup_cause = 16`.
   - Desconectar o canal via comando AMI `Hangup`.
   - Gravar a quebra em `execution_traces` para auditoria.

---

## 4. Randomização Dinâmica de CallerID (BINA)

Para evitar bloqueios de operadoras (SPAM / Robo-call flag):
1. Cada campanha possui uma lista de números DID autorizados (`caller_ids`).
2. O discador seleciona aleatoriamente um número da lista a cada chamada via gerador criptográfico pseudo-randômico.
3. O DID selecionado é injetado nas variáveis de canal Asterisk `CALLERID(num)` e `P-Asserted-Identity` antes do envio do `INVITE` SIP pela operadora.

---

## 5. Triagem de Voz Híbrida: AMD Nativo + Vosk STT via EAGI

1. **Estágio 1 - Asterisk `app_amd`:**
   - Analisa os primeiros 1.500ms de áudio pós-atendimento.
   - Fala curta inicial (< 1.200ms) seguida de silêncio (> 500ms) classifica como `AMDSTATUS=HUMAN` e despacha `Redirect` imediato para o operador.
2. **Estágio 2 - Vosk STT via EAGI (`cmd/vosk-eagi`):**
   - Se o `app_amd` suspeitar de mensagem gravada longa (`AMDSTATUS=MACHINE`), o áudio do descritor de arquivo 3 (`FD 3`) é transmitido via WebSocket PCM 8kHz para o servidor Vosk.
   - Se detectar saudações humanas ("alô", "oi", "pronto"), converte o veredito para `HUMAN` e entrega a chamada.
   - Se transcrever termos de correio de voz ("caixa postal", "deixe seu recado", "após o sinal"), finaliza com `Hangup` silencioso descartando o canal.
