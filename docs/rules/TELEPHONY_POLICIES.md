# Políticas Telefônicas: Pacing, Overdialing, Quota e Triagem AMD

Este documento especifica as regras de negócio, algoritmos matemáticos e restrições regulatórias para a discagem automatizada do **Dialer-Go**.

---

## 1. Algoritmo de Pacing e Equações de Overdialing

O motor preditivo (`internal/core/predictive_engine.go`) tem como objetivo manter a ocupação máxima dos operadores humanos ou salas virtuais sem gerar abandono regulatório de chamadas.

### 1.1. Equação de Overdialing Dinâmico com Piso Mínimo
O cálculo do número de canais a disparar simultaneamente em cada rodada de pacing obedece à equação de overdialing garantindo um piso mínimo operacional:

$$\text{ChannelsToDial} = \max\left(\text{AvailableAgents} \times \text{MinChannelsPerAgent}, \text{round}\left(\frac{\text{AvailableAgents} \times \text{Aggressiveness}}{\text{ContactProbability}}\right)\right)$$

Onde:
* $\text{AvailableAgents}$: Quantidade de operadores com status livre/ocioso (`idle`) associados à campanha no Redis.
* $\text{MinChannelsPerAgent}$: Piso de canais simultâneos por operador disponível (default seguro: `2`, configurável dinamicamente via `GET/POST /api/v1/predictive/pacing`, variável de ambiente `MIN_CHANNELS_PER_AGENT` ou por requisição em `min_channels_per_agent`). Permite calibrar a agressividade entre `1` e `50` canais por atendente sem reiniciar a aplicação.
* $\text{Aggressiveness}$: Fator multiplicador configurado na campanha ou enviado dinamicamente no payload `PredictiveDemandRequest` (`aggressiveness`) (padrão: `1.20`, variando de `1.00` a `2.50`).
* $\text{ContactProbability}$: Taxa histórica móvel de sucesso de atendimento humano (padrão conservador de boot: `0.28` ou 28%).
* $\text{ChannelsToDial}$: Quantidade de chamadas simultâneas que o discador tentará originar no Asterisk.

### 1.2. Regra de Piso Configurável por Operador Disponível
Mesmo sob condições adversas ou desaceleração pós-abandono regulatório (`hasInflated`), a demanda de discagem preditiva obedece ao piso configurado `AvailableAgents × MinChannelsPerAgent` (a menos que haja esgotamento da capacidade física de troncos SIP ou da quota global de segurança humana):
* **1 operador disponível:** Mínimo de $1 \times \text{minRatio}$ chamadas originadas (default: 2).
* **2 operadores disponíveis:** Mínimo de $2 \times \text{minRatio}$ chamadas originadas (default: 4).
* **$N$ operadores disponíveis:** Mínimo de $N \times \text{minRatio}$ chamadas originadas.
* **Ajuste Dinâmico a Quente:** A taxa pode ser alterada instantaneamente via `POST /api/v1/predictive/pacing` sem reiniciar o discador.

---

## 2. Quota Garantida de Canais para Operadores Humanos (`HumanReserveQuota`)

Para impedir que campanhas preditivas esgotem os recursos de tronco e impeçam operadores manuais de realizar ligações críticas:
* **Capacidade Global Padrão (`MAX_GLOBAL_CHANNELS`):** 120 canais.
* **Quota Reservada Humana (`HUMAN_RESERVED_QUOTA`):** 10 canais.
* **Limite Efetivo para o Motor Preditivo:**

$$\text{MaxPredictiveSlots} = \text{MAX\_GLOBAL\_CHANNELS} - \text{HUMAN\_RESERVED\_QUOTA} = 110 \text{ canais}$$

Se $\text{ActiveChannels} \ge 110$, novas tentativas de discagem preditiva são suspensas até que canais sejam liberados, enquanto requisições manuais (`POST /api/v1/calls/manual`) continuam sendo atendidas até o teto absoluto de 120 canais.

---

## 3. Protocolo de Abandono Regulatório (< 2 Segundos)

Conforme as diretrizes regulatórias de telecomunicações:
1. **Cronômetro de Entrega:** O tempo entre o cliente atender a ligação (evento `200 OK` / `Answer`) e a conexão com um operador humano não pode exceder **2,0 segundos**.
2. **Ação em Caso de Esgotamento:** Se nenhum operador estiver disponível para receber a chamada humana detectada em até 2 segundos, o discador deve:
   - Registrar imediatamente a chamada com status `disposition = 'ABANDONED'` e `hangup_cause = 16`.
   - Desconectar o canal via comando AMI `Hangup`.
   - Gravar a quebra em `execution_traces` para auditoria.

---

## 4. Preservação Estrita do Número Discado como CallerID

Para garantir a integridade relacional e a resolução imediata de contatos no sistema conectado e CRMs externos:
1. O discador preditivo e manual envia como `callerID` estritamente o número real discado do lead (`destPhone`).
2. É proibido aplicar mutações artificiais, sufixos aleatórios ou alterações de dígitos na identificação do lead.
3. O número real discado é injetado nas variáveis de canal Asterisk e no comando AMI Originate, assegurando que o webhook, o histórico de contatos (`contatofone`) e a tela de atendimento identifiquem com exatidão o lead atendido.

---

## 5. Triagem Ativa de Voz & Avaliação Positiva de Atendimento

1. **Triagem Ativa Full-Duplex (Vosk STT via EAGI):**
   - No atendimento da chamada, o Asterisk executa [`cmd/vosk-eagi`](file:///home/marcio/ominichat/dialer-go/cmd/vosk-eagi/main.go), que reproduz a saudação natural estruturada em 8000 Hz (`alo_tudo_bem.wav`/`alo_tudo_bem.alaw`) em background enquanto escuta e transcreve a resposta do cliente no canal de áudio em tempo real via `FD 3` conectado ao Kaldi-Vosk.
2. **Critérios Determinísticos de Avaliação Positiva:**
   - **Confirmação Humana Positiva:** Apenas saudações e termos positivos explícitos (*"alô"*, *"oi"*, *"pronto"*, *"sim"*, *"quem fala"*, *"opa"*, *"bom dia"*, *"boa tarde"*, *"sou eu"*, *"fala"*, *"diga"*) validados com fronteiras exatas de palavras classificam a chamada como `HUMAN`. O Asterisk emite `UserEvent(PredictiveHuman)` e comuta a chamada para a sala do operador.
   - **Caixa Postal Explícita:** Termos de operadora (*"caixa postal"*, *"deixe seu recado"*, *"após o sinal"*, *"vivo informa"*, etc.) classificam a chamada como `MACHINE` com causa `VOICEMAIL_<FRASE>`.
   - **Silêncio ou Áudio Não Confirmado:** Se a janela de triagem (~3,0s) expirar em silêncio (`norm == ""`) ou com ruído ininteligível sem palavra humana positiva, a chamada é classificada como `MACHINE` (`cause = SILENCE_TIMEOUT` ou `cause = UNCONFIRMED_AUDIO`). O discador **não transfere para operadores**, evitando sobrecarga e picos de abandono.
3. **Notificação e Descarte no PBX:**
   - Ao confirmar `MACHINE`, o Asterisk dispara `UserEvent(PredictiveMachine, ..., Cause: ${VOSK_AMD_CAUSE})` e executa `Hangup()`.
   - O `TrunkManager` seta a disposição da chamada como `VOICEMAIL` em memória, assegurando registro fidedigno nos CDRs e permitindo que a stored procedure `fn_audit_persist_predictive_result` requebre o lead para retentativa (`status = 'QUEUED'`).

