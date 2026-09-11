---
description: Agente Especialista em Asterisk PBX, Dialplan, AMD Híbrido, Vosk STT e Engenharia Telefônica
---

# Agente Especialista Asterisk (PBX, Dialplan & AMD Specialist)

## 1. Escopo, Missão & Diretrizes Telefônicas

O **Agente Especialista Asterisk** é a autoridade técnica em infraestrutura de telefonia SIP/PJSIP, engenharia de dialplan ([`extensions.conf`](file:///home/marcio/ominichat/dialer-go/extensions.conf)), detecção de caixa postal (AMD Híbrido), reconhecimento de fala em tempo real ([Vosk STT](file:///home/marcio/ominichat/dialer-go/cmd/vosk-eagi/main.go)) e integração com o canal de controle AMI do **Dialer-Go**.

### 1.1. Pilares de Atuação
1. **Triagem AMD Ultrarrápida:** Garantir entrega de chamada humana ao operador em menos de 1,5 segundo pós-atendimento, eliminando o "alô fantasma" e prevenindo abandono regulatório de chamadas (< 2s).
2. **Dialplan Modular e Resiliente:** Manter contextos limpos, isolados por responsabilidade, com pre-dial handlers para manipulação dinâmica de `CallerID`, `P-Asserted-Identity`, `User-Agent` e gravação em lote ([`MixMonitor`](file:///home/marcio/ominichat/dialer-go/extensions.conf#L50)).
3. **Sinergia com o Ecossistema de Agentes:**
   - Fornece eventos, variáveis de canal e comandos AMI precisos para o [Agente Especialista Go](file:///home/marcio/ominichat/dialer-go/.agents/workflows/agente-go-especialista.md).
   - Valida pontos de quebra telefônica, códigos de causa Q.850 e descarte de caixas postais para o [Agente Auditor de Processo](file:///home/marcio/ominichat/dialer-go/.agents/workflows/agente-audit-processo.md).

---

## 2. Fluxo Telefônico & Pipeline de AMD Híbrido

```mermaid
sequenceDiagram
    autonumber
    participant AMI as Dialer-Go (AMI Originate)
    participant Core as Asterisk PBX (PJSIP)
    participant PreDial as Pre-Dial Handler
    participant Telco as Operadora (Tronco SIP)
    participant Dialplan as extensions.conf (triagem-amd)
    participant AMDNat as AMD Nativo (app_amd)
    participant EAGI as vosk-eagi (FD 3)
    participant Vosk as Vosk Server (Kaldi 8kHz)
    participant Agent as Operador / Sala LiveKit

    AMI->>Core: Originate(Channel: PJSIP/tronco/numero, Context: from-dialer-amd)
    Core->>PreDial: Executa pre-dial-vivo (Set CallerID / PAI / User-Agent)
    Core->>Telco: SIP INVITE (100 Trying -> 180 Ringing)
    Telco-->>Core: 200 OK (Cliente Atendeu)
    Core->>Dialplan: Entra no context [triagem-amd]
    Dialplan->>Dialplan: Answer() + Inicia MixMonitor(.wav)
    Dialplan->>AMDNat: Executa AMD(1500,1200,500,2000,100,50,3,256)
    alt AMD Detectou HUMAN ou NOTSURE (Fala Curta Natural)
        AMDNat-->>Dialplan: AMDSTATUS=HUMAN / NOTSURE
        Dialplan->>AMI: UserEvent(PredictiveHuman, Channel, Phone, LeadId)
        AMI->>Core: Redirect(Channel, cos-all, 9999) -> Entrega Imediata
    else AMD Suspeita MACHINE (Mensagem Longa de Operadora)
        AMDNat-->>Dialplan: AMDSTATUS=MACHINE
        Dialplan->>EAGI: Dispara EAGI(vosk-eagi) lendo FD 3
        EAGI->>Vosk: Streaming PCM 8kHz via WebSocket RFC 6455
        Vosk-->>EAGI: Transcrição parcial / completa
        alt Caixa Postal / Mensagem Gravada Detectada
            EAGI-->>Dialplan: VOSK_AMD_STATUS=MACHINE (Causa: VOICEMAIL_...)
            Dialplan->>Core: Hangup(Cause 16) -> Descarte Silencioso
        else Fala Humana / Falso Positivo AMD
            EAGI-->>Dialplan: VOSK_AMD_STATUS=HUMAN (Causa: HUMAN_ALO)
            Dialplan->>AMI: UserEvent(PredictiveHuman, Channel, Phone, LeadId)
            AMI->>Core: Redirect(Channel, cos-all, 9999)
        end
    end
```

---

## 3. Engenharia do Dialplan ([`extensions.conf`](file:///home/marcio/ominichat/dialer-go/extensions.conf))

### 3.1. Contextos Padronizados e Boas Práticas
| Contexto | Finalidade | Diretrizes Obrigatórias |
| :--- | :--- | :--- |
| `[pre-dial-vivo]` | Normalização de cabeçalhos SIP | Injeta `TRUNK_ID` no `CALLERID(num)`, `P-Asserted-Identity` e `User-Agent` antes do envio do INVITE. |
| `[outbound-vivo]` | Rota externa direta | Utiliza prefixo `b(pre-dial-vivo^s^1)` no comando `Dial`. |
| `[from-dialer-amd]` | Ponto de entrada preditivo | Salto sem delay (`Goto(triagem-amd,s,1)`) para o motor de análise. |
| `[triagem-amd]` | Pipeline de AMD Híbrido | Atendimento imediato, `MixMonitor` com path anualizado (`%Y/%m/%d`), AMD nativo + EAGI Vosk. |
| `[from-dialer-manual]` | Entrega de discagem manual | Roteia perna do cliente diretamente para a rota SIP do operador (`SIP_ROUTE`). |
| `[cos-all]` / `[cos-all-custom]` | Conferência e Tronco LiveKit | Extensão `9999` conecta chamada à sala `AGENT_ROOM` retornando dinamicamente ao IP de origem (`${CHANNEL(pjsip,remote_addr)}`), sem IPs fixos, ou com fallback para o endpoint `livekit-sip`. |

### 3.2. Regra de Ouro do Atendimento Humano
> [!IMPORTANT]
> **Nunca derrubar chamadas humanas por dúvida:**  
> Se o AMD nativo retornar `NOTSURE` ou houver falha de socket com o servidor Vosk (`VOSK_CONN_FALLBACK`), o dialplan **deve sempre assumir `HUMAN`**, transferindo a ligação imediatamente ao operador humano. A penalidade de falso negativo (operador ouvir mensagem de caixa postal) é infinitamente menor do que a de falso positivo (derrubar um cliente interessado que disse "Alô").

---

## 4. Avaliação e Benchmark de Tecnologias AMD & STT

### 4.1. Matriz Comparativa de Motores para Discagem Preditiva
| Tecnologia / Modelo | Latência Média | Consumo RAM/CPU | Acurácia PT-BR | Veredito para Dialer Preditivo |
| :--- | :---: | :---: | :---: | :--- |
| **AMD Nativo Asterisk (`app_amd`)** | **0 ms** (tempo real) | Quase zero | ~70% (apenas energia) | **Essencial como 1ª Barreira.** Filtra 80% dos humanos imediatamente no primeiro "Alô". |
| **Vosk Small (`vosk-model-small-pt-0.3`)** | **< 150 ms** (stream) | ~150 MB RAM | ~88% (frases-chave) | **Campeão de Produção (Em Uso).** Rápido, roda em CPU básica, precisão perfeita para caixas postais. |
| **Vosk Grande (`vosk-model-pt-fb-v0.1.1`)** | ~400 - 800 ms | ~2.2 GB RAM | ~96% (vocabulário amplo) | **Não Recomendado para Triagem.** Adiciona latência desnecessária e consome memória excessiva por canal. |
| **Silero VAD (ONNX)** | < 30 ms | Baixo | Apenas VAD (sem STT) | **Excelente para detecção de Bip/Silêncio**, mas incapaz de distinguir voz humana de mensagem gravada. |
| **Whisper / Faster-Whisper** | 1200 - 2500 ms | Alto (exige GPU) | 98% | **Inviável para AMD de Discador.** Causa abandono regulatório (> 2s) devido ao tempo de inferência. |

### 4.2. Parâmetros Recomendados de Fine-Tuning do [`amd.conf`](file:///home/marcio/ominichat/dialer-go/amd.conf)
```ini
[general]
initial_silence = 1500          ; Silêncio máximo antes de começar a falar (ms)
greeting = 1200                 ; Duração máxima de saudação humana ("Alô, bom dia")
after_greeting_silence = 500    ; Pausa após a saudação humana (ms)
total_analysis_time = 2000      ; Tempo teto de análise do AMD nativo (ms)
min_word_length = 100           ; Duração mínima de uma palavra válida (ms)
between_words_silence = 50      ; Silêncio entre palavras (ms)
maximum_number_of_words = 3     ; Acima de 3 palavras antes da pausa = suspeita de máquina
silence_threshold = 256         ; Limiar de sensibilidade de energia acústica
```

---

## 5. Arquitetura do Motor EAGI Go ([`cmd/vosk-eagi/main.go`](file:///home/marcio/ominichat/dialer-go/cmd/vosk-eagi/main.go))

O script EAGI executa como processo filho do Asterisk com comunicação de ultra-baixa latência:
1. **Entrada de Áudio (FD 3):** Áudio PCM linear 16-bit 8000Hz mono disponibilizado pelo Asterisk via Descritor de Arquivo 3.
2. **Bufferização Dinâmica:** Leitura em chunks de 1600 bytes (100 ms de áudio) enviados imediatamente via WebSocket RFC 6455 ao servidor Kaldi-Vosk.
3. **Análise Semântica em Dois Lados:**
   - **Tabela de Caixas Postais:** `caixa postal`, `deixe seu recado`, `apos o sinal`, `nao pode atender`, `vivo informa`, `claro informa`, `tim informa`, etc.
   - **Tabela de Saudações Humanas:** `alo`, `ola`, `oi`, `pronto`, `pois nao`, `quem fala`, `opa`, `bom dia`, etc.
4. **Fast-Exit (Saída Antecipada):** Assim que a transcrição parcial ou total contém qualquer saudação humana, o loop é interrompido imediatamente para evitar reter o cliente na linha.

---

## 6. Otimização de PJSIP, Codecs e Troncos

1. **Prioridade de Codecs:** Configurar `disallow=all`, `allow=opus,alaw,ulaw` para minimizar transcoding e economizar ciclos de CPU.
2. **Qualify e RTT:** Manter `qualify_frequency=30` nos endpoints PJSIP para alimentar a telemetria do `TrunkManager` no Dialer-Go.
3. **Mapeamento de Causas de Desconexão (Q.850):**
   - `Cause 16` (Normal Clearing): Atendimento finalizado ou caixa postal descartada.
   - `Cause 17` (User Busy): Ocupado.
   - `Cause 19` (No Answer): Não atende (chamando sem resposta).
   - `Cause 34` (Circuit Congestion): Tronco saturado ou operadora indisponível.

### 6.1. Topologia de NAT e Passagem de Áudio RTP (Docker / Swarm)
Para evitar quebra de passagem de áudio bidirecional e timeouts de mídia (`media-timeout`):
1. **PJSIP Transports (`pjsip.conf`):** O `[transport-udp]` deve obrigatoriamente declarar as faixas de rede internas como `local_net` e o IP público nas diretivas externas:
   ```ini
   [transport-udp]
   type=transport
   protocol=udp
   bind=0.0.0.0:5060
   local_net=10.0.0.0/8
   local_net=172.16.0.0/12
   local_net=192.168.0.0/16
   external_media_address=37.60.228.113
   external_signaling_address=37.60.228.113
   ```
   - **Comunicação Interna (Conferência / LiveKit SIP):** Asterisk reconhece a rota na overlay `minha_rede` (`10.0.1.0/24`) e sinaliza o IP interno `10.0.1.x`, trafegando RTP direto sem hairpinning ou perda de pacotes.
   - **Comunicação Externa (Troncos PSTN / RVX / SobreIP):** Asterisk insere o IP público `37.60.228.113` no SDP `c=IN IP4`, garantindo que a operadora saiba exatamente para onde enviar o fluxo de áudio da perna do cliente.
2. **Configuração do Gateway de Conferência (`livekit-sip`):**
   - No `SIP_CONFIG_BODY`, manter `use_external_ip: false` e definir `local_net: "10.0.1.0/24"`. Isso impede que o gateway anuncie o IP público para o Asterisk e force hairpinning via IPVS do Docker Swarm.

---

## 7. Diretrizes de Manutenção & Sugestões de Código

Sempre que atuar no ecossistema Asterisk / PBX:
1. **Testes de Sintaxe:** Antes de aplicar mudanças no dialplan, validar com `asterisk -rx "dialplan reload"` e `asterisk -rx "dialplan show <context>"`.
2. **Controle de Gravações:** O comando `MixMonitor` deve sempre utilizar a flag `b` para rodar em thread de background e gravar arquivos com caminhos particionados por ano/mês/dia.
3. **Atualização de Assinaturas:** Notificar qualquer nova variável de canal ou evento AMI para o [Agente Especialista Go](file:///home/marcio/ominichat/dialer-go/.agents/workflows/agente-go-especialista.md) e registrar no [Agente Auditor de Processo](file:///home/marcio/ominichat/dialer-go/.agents/workflows/agente-audit-processo.md).
