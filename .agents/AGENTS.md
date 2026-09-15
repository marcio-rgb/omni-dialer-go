# Regras Gerais do Workspace (Dialer-Go)

## 1. Governança de Agentes & Workflows Especializados

O desenvolvimento, manutenção e operação do **Dialer-Go** (`go 1.25.0`) é regido por workflows de agentes especializados e complementares:

1. 👉 [`.agents/workflows/agente-audit-processo.md`](file:///home/marcio/ominichat/dialer-go/.agents/workflows/agente-audit-processo.md): **Agente de Teste, Auditoria Ponta a Ponta & Controle de Quebra de Processo.**
   - Focado em auditoria de ciclo de vida (início, repouso/idle, término), conciliação de banco/cache/AMI, trace dinâmico liga/desliga (`execution_traces`), gravação de CDR e matriz de rastreabilidade.
2. 👉 [`.agents/workflows/agente-go-especialista.md`](file:///home/marcio/ominichat/dialer-go/.agents/workflows/agente-go-especialista.md): **Agente Especialista na Linguagem Go (Go 1.25.0 Architecture & Language Specialist).**
   - Focado em padrões idiomáticos Go 1.25, estruturas de dados, concorrência thread-safe com `sync/atomic.Int32` e `sync.RWMutex`, tipagem estrita, arquitetura hexagonal (`domain`, `ports`, `core`, `adapters`) e especificação detalhada de contratos e assinaturas de métodos.
3. 👉 [`.agents/workflows/agente-asterisk-especialista.md`](file:///home/marcio/ominichat/dialer-go/.agents/workflows/agente-asterisk-especialista.md): **Agente Especialista Asterisk (PBX, Dialplan & AMD Specialist).**
   - Focado em infraestrutura telefônica Asterisk, sintaxe de dialplan (`extensions.conf`), triagem ultrarrápida de atendimento humano (< 1,5s), AMD nativo (`app_amd`), reconhecimento de voz com Vosk STT via EAGI em Go, roteamento PJSIP, pre-dial handlers e codecs.
4. 👉 [`.agents/workflows/agente-tester-dialer.md`](file:///home/marcio/ominichat/dialer-go/.agents/workflows/agente-tester-dialer.md): **Agente Tester Dialer & Validação Ponta a Ponta (SIP Transit & N10/N11 Integration).**
   - Focado em garantia de Zero Normalização no Dialer-Go, testes automatizados de discagem multiformato (`10`, `11`, `12`, `13`, `14` dígitos), contrato de pre-dial LiveKit e conformidade técnica ([`validacao-dialer-n10.md`](file:///home/marcio/ominichat/dialer-go/.agents/workflows/validacao-dialer-n10.md)).
5. 👉 [`.agents/workflows/agente-especialista-sst.md`](file:///home/marcio/ominichat/dialer-go/.agents/workflows/agente-especialista-sst.md): **Agente Especialista STT (Speech-to-Text, Vosk, Whisper & Voice AI Specialist).**
   - Focado em motores de transcrição de voz em tempo real (Vosk Kaldi, Whisper, VAD, Piper TTS), streaming de PCM linear 8kHz via EAGI FD 3, emissão de transcrições parciais/completas via AMI VarSet/UserEvent e persistência contínua na tabela `cdrs` para análises.

---

## 2. Regra de Sincronização Obrigatória de Métodos e Contratos
Qualquer alteração — inclusive adição, remoção, alteração de assinatura, parâmetros, retorno, validação ou dialplan — em:
- `internal/core/` (motores preditivo, manual, receptivo, canais e troncos)
- `internal/adapters/` (HTTP, PostgreSQL, AMI, Redis, Storage)
- `internal/ports/` (interfaces e contratos canônicos)
- `internal/domain/` (entidades e DTOs)
- `extensions.conf` / Dialplan Asterisk e AGI/EAGI (`cmd/vosk-eagi/`)
- `database/schema.sql` (tabelas, índices, views e stored procedures)

**Exige obrigatoriamente e na mesma intervenção a atualização dos workflows correspondentes:**
1. Atualizar contratos, assinaturas e tipagem no [Agente Go Especialista](file:///home/marcio/ominichat/dialer-go/.agents/workflows/agente-go-especialista.md).
2. Atualizar contextos, regras de AMD, variáveis de canal ou roteamento no [Agente Especialista Asterisk](file:///home/marcio/ominichat/dialer-go/.agents/workflows/agente-asterisk-especialista.md).
3. Atualizar o ciclo de vida, rastreamento e matriz no [Agente Auditor de Processo](file:///home/marcio/ominichat/dialer-go/.agents/workflows/agente-audit-processo.md).

> [!CAUTION]
> Nenhuma alteração é dada por finalizada se os mapeamentos dos agentes estiverem desatualizados ou divergentes do código-fonte em execução.

---

## 3. Regra Geral e Obrigatória de Governança da API (API_REFERENCE.md)

Toda e qualquer alteração, adição, remoção ou refatoração nos endpoints HTTP do **Dialer-Go** exige **obrigatoriamente a atualização detalhada e imediata** do documento canônico:
👉 [`.agents/workflows/API_REFERENCE.md`](file:///home/marcio/ominichat/dialer-go/.agents/workflows/API_REFERENCE.md)

### Diretrizes de Cumprimento Obrigatório:
1. **Sincronização Atômica:** Qualquer commit ou intervenção que modifique ou adicione:
   - Rotas HTTP (`internal/adapters/http/server.go` ou sub-handlers).
   - Parâmetros de requisição (Query Parameters, Path Parameters, Headers HTTP ou JSON Request Body).
   - DTOs de entrada e saída (`internal/domain/*_dto.go` ou `internal/domain/*.go`).
   - Códigos de status HTTP e payloads de erro padronizados (RFC 7807).
   - Disparos de webhooks de saída (egress) ou endpoints consumidos de integração (ex: `/api/dialer/refill`, `/api/telephony/webhook/call-ended`).
   - Barramentos de eventos assíncronos ou canais de monitoramento WebSocket em tempo real.
   **DEVE conter no mesmo commit/intervenção a documentação completa correspondente no `API_REFERENCE.md`.**
2. **Nível de Detalhe Exigido no `API_REFERENCE.md`:**
   - Descrição da responsabilidade operacional do endpoint.
   - Headers necessários (`Content-Type`, `X-Tenant-Id`, `User-Agent`, etc.).
   - Tabela descritiva de campos com tipos, obrigatoriedade, valores padrão e validações.
   - Exemplos reais de JSON Request Body.
   - Exemplos reais de JSON Response de Sucesso (`200 OK`, `201 Created`, etc.).
   - Catálogo de erros mapeados no padrão Problem Details (RFC 7807) com códigos de status (`400`, `404`, `422`, `429`, `500`), códigos internos (`INVALID_JSON`, `TRUNK_NOT_FOUND`, etc.) e causas prováveis.
3. **Proibição de Descompasso:** É terminantemente proibido publicar código ou finalizar tarefas com endpoints sem documentação ou com discrepâncias entre as structs do Go e o catálogo da API.

---

## 4. Cláusula Pétrea: Desacoplamento Total e Autonomia Absoluta do Dialer-Go

> [!CAUTION]
> **REGRA INVIOLÁVEL DE ARQUITETURA:**
> O **Dialer-Go** é um motor de telefonia **100% autônomo, agnóstico e soberano**. Ele **NÃO** é um módulo dependente do OmniChat, não reside no sistema OmniChat e **NUNCA** deve ser acoplado a estruturas, bancos ou legados daquela plataforma.

### Diretrizes Pétreas de Autonomia:
1. **Proibição Absoluta de Acesso ou Delegação ao Legado (`call_history`, etc.):**
   - É terminantemente proibido ler, gravar, delegar persistência ou modelar regras de negócio esperando que tabelas do OmniChat (como `call_history`, `calls`, `contacts` ou `Lead`) supram dados ou funcionalidades do discador.
   - O Dialer-Go possui e gerencia seu próprio banco relacional isolado (**`dialer_db`**), mantendo soberania sobre `cdrs`, `leads`, `campaigns`, `trunks`, `phone_trunk_mappings` e `execution_traces`.
   - **Toda e qualquer informação de chamada — incluindo metadados de tarifação, desfechos e os caminhos/URLs de áudio gravado (`recording_file` e `recording_url`) — DEVE ser armazenada na tabela `cdrs` do Dialer-Go.**
2. **Sistema Preparado para Atender Qualquer Plataforma Externa:**
   - O Dialer-Go foi projetado para interoperar com qualquer ecossistema cliente (OmniChat, CRMs externos, discadores web, bots ou plataformas de IA).
   - A interface entre o Dialer-Go e qualquer sistema consumidor ocorre **exclusivamente através de contratos de API REST padronizados (RFC 7807)** e **Webhooks tipados de saída**.
   - Qualquer consumidor que necessite de histórico de chamadas ou gravações de áudio deve requisitar via API canônica do Dialer-Go (`GET /api/v1/cdrs`, `GET /api/v1/cdrs/{id}`, `GET /api/v1/recordings/*`, `GET /api/v1/reports/calls-summary`).
3. **Proibição de Suposições ou Acoplamentos Ocultos:**
   - Nenhuma decisão técnica ou omissão no discador pode ser justificada sob o pretexto de que *"o OmniChat já guarda isso"* ou *"o OmniChat resolve isso no webhook"*.
   - O Dialer-Go deve funcionar de ponta a ponta com plenitude funcional e auditabilidade mesmo que o OmniChat não exista ou seja substituído por outro sistema.

---

## 5. Política Mandatória de Gravação de Áudio Telefônico (Dual-Channel Stereo & Compressão Assíncrona MP3)

> [!IMPORTANT]
> **REGRA GERAL DE ENGENHARIA DE ÁUDIO & PERFORMANCE:**  
> A integridade forense das chamadas telefônicas exige a separação física absoluta entre o áudio do operador/robô (TX) e o áudio do cliente (RX), sem jamais onerar o PBX com processamento pesado de compressão durante o tráfego telefônico.

### Diretrizes Mandatórias de Áudio:
1. **Gravação Nativa em WAV PCM 16-bit 8000 Hz:**
   - Durante a chamada telefônica no Asterisk, a gravação DEVE ocorrer estritamente no formato nativo WAV (PCM linear 16-bit mono por canal a 8.000 Hz).
   - **Proibição de Encoding MP3 em Tempo Real no PBX:** É expressamente proibido instruir o Asterisk (`MixMonitor`) a codificar MP3 síncrono durante a chamada (via LAME/ffmpeg em tempo real). O encoding MP3 durante a chamada consome ciclos preciosos de CPU, degrada a latência RTP e introduz jitter sob rajadas de chamadas simultâneas.
2. **Segregação Estéreo Dual-Channel Obrigatória:**
   - Toda e qualquer gravação telefônica (Preditiva, Manual ou Receptiva) deve manter os canais físicos independentes:
     - **Canal 1 (Left / Esquerdo):** TX - Voz do Atendente / Sistema / Prompt Institucional
     - **Canal 2 (Right / Direito):** RX - Voz do Cliente / Lead
   - É terminantemente proibido gravar chamadas em mono simples somando TX e RX na mesma trilha, pois impede auditoria forense, transcrição precisa e análise de barge-in.
3. **Pós-Processamento Imediato de Fusão Estéreo (`merge-stereo`):**
   - O comando `MixMonitor` deve obrigatoriamente acionar o pós-processamento atômico em Go ([`cmd/merge-stereo/main.go`](file:///home/marcio/ominichat/dialer-go/cmd/merge-stereo/main.go)):
     ```ini
     same => n,MixMonitor(${REC_FILENAME}.wav,r(${REC_FILENAME}-rx.wav)t(${REC_FILENAME}-tx.wav),/var/lib/asterisk/agi-bin/merge-stereo "${REC_FILENAME}-tx.wav" "${REC_FILENAME}-rx.wav" "${REC_FILENAME}.wav")
     ```
   - O utilitário `merge-stereo` funde instantaneamente os arquivos mono em um único arquivo WAV estéreo final `${REC_FILENAME}.wav` e purga imediatamente os arquivos temporários `-tx.wav` e `-rx.wav`, mantendo o disco limpo e unificado.
4. **Compressão para MP3 Estritamente Assíncrona (Background Job):**
   - Caso seja necessária a redução de consumo de armazenamento em disco, a conversão de `.wav` para `.mp3` DEVE ocorrer **exclusivamente de forma assíncrona pós-chamada** (via worker em background, cron job ou rotina de offloading para storage).
   - O arquivo MP3 comprimido DEVE obrigatoriamente preservar os **dois canais estéreo segregados** (`-ac 2 -map_channel`), garantindo que ferramentas analíticas, reprodutores e transcritores mantenham a segregação de TX e RX.

