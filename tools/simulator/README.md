# Servidor de Testes & Simulador Asterisk (`38.242.219.186`)

Este diretório contém os arquivos canônicos de configuração do Asterisk PBX para o ambiente de testes e simulação de chamadas no IP `38.242.219.186`.

---

## 1. Objetivo & Comportamento em Looping

Para validar e auditar o comportamento do discador (**Dialer-Go**) e a experiência dos operadores humanos / agentes LiveKit:
* **Problema Anterior:** Ao reproduzir um áudio de saudação humana ou de teste, se o Asterisk desligasse automaticamente após o término do arquivo WAV, o lado que desligava a chamada era o "cliente" (simulador). Isso impedia validar o ciclo de vida de desligamento voluntário por parte do usuário/operador da plataforma.
* **Comportamento Atualizado (Looping Contínuo):** 
  - Ao receber a chamada, o Asterisk do simulador atende (`Answer()`), seleciona o áudio do catálogo (`play_benchmark.py`) e entra em **looping contínuo** (`Playback` -> `Wait(1)` -> `Goto(play_loop)`).
  - A chamada **permanece ativa indefinidamente** até que o **usuário/operador/plataforma** desligue ativamente a perna da chamada.

---

## 2. Estrutura de Arquivos

| Arquivo | Descrição |
| :--- | :--- |
| `extensions.conf` | Dialplan Asterisk com contexto `[from-dialer]` e loop contínuo de áudio sem hangup automático. |
| `pjsip.conf` | Configuração de transporte UDP (:5070 e :5060) e endpoint `dialer-inbound` para receber chamadas do Dialer-Go. |
| `asterisk.conf` | Definições de diretórios e parâmetros globais do Asterisk. |
| `modules.conf` | Carregamento de módulos essenciais de canal PJSIP, AGI, formatos de mídia e codecs (alaw/ulaw). |
| `rtp.conf` | Faixa de portas RTP (10000 a 20000). |
| `logger.conf` | Configuração de logs de console e arquivo. |
| `play_benchmark.py` | Script AGI em Python que sorteia o áudio, registra gabarito forense (`simulator_played.db` / `simulator_played.jsonl`) e passa a variável ao dialplan. |
| `catalog.example.json` | Modelo de catálogo de áudios de teste (Humano vs. Máquina). |
| `docker-compose.simulator.yml` | Manifesto Docker Compose para execução do container no host. |
| `CREDENTIALS.md` | Credenciais SSH e comandos de acesso ao servidor de teste (`38.242.219.186`). |

---

## 3. Como Aplicar no Servidor de Teste (`38.242.219.186`)

1. **Sincronizar arquivos para o servidor:**
   ```bash
   scp mode/simulator/* root@38.242.219.186:/opt/simulator/config/
   ```

2. **Recarregar o dialplan no Asterisk:**
   ```bash
   # Recarregar dialplan
   asterisk -rx "dialplan reload"
   # Recarregar PJSIP se alterado
   asterisk -rx "pjsip reload"
   ```

3. **Auditoria Forense Pós-Teste:**
   Utilize o script de auditoria cruzada na raiz do projeto:
   ```bash
   python3 scripts/benchmark_cross_audit.py
   ```
