# Diretrizes Globais de Arquitetura, Engenharia de Código, Governança e Segurança

Este documento estabelece os padrões técnicos, regras arquiteturais e diretrizes de governança obrigatórios para desenvolvimento, refatoração e manutenção nos ecossistemas **Node.js / TypeScript**, **Python** e **Go (Golang)**.

---

## 0. Diretrizes Operacionais Absolutas para a IA

### 0.1. Protocolo Bootstrap Zero (Leitura do ARCHITECT.md)
1. **Leitura Obrigatória:** Antes de propor, modificar ou refatorar qualquer linha de código, a IA **DEVE obrigatoriamente ler o arquivo** `.agents/ARCHITECT.md` na raiz do projeto para assimilar o domínio, tech stack, invariantes e grafo de dependências.
2. **Criação de Bootstrap:** Se o projeto ainda não possuir o `.agents/ARCHITECT.md`, a IA deve redigir a primeira versão deste arquivo (seguindo o template oficial) e submetê-la para validação antes de produzir código de negócio.

### 0.2. Sincronização Atômica de Código e Documentação
1. **Contratos em Código:** Toda alteração em funções, métodos ou rotas públicas exige a atualização imediata das tags de rastreabilidade (`@pattern`, `@governedBy`, `@preExecution`, `@postExecution`).
2. **Atualização de Grafo:** Mudanças estruturais exigem a atualização concorrente do `.agents/ARCHITECT.md` e do índice do repositório (`docs/MAP.md`) no mesmo commit/intervenção.
3. **Proibição de Código Truncado:** É terminantemente proibido omitir implementações com comentários de preenchimento (`// TODO`, `/* lógica aqui */`, `...`).

### 0.3. Imposição Rígida de Limite de Arquivos (500 Linhas)
* **Limite Máximo Absoluto:** **500 linhas** de código-fonte.
* **Zona de Alerta e Refatoração:** Ao atingir **350 linhas**, o plano de desacoplamento modular deve ser ativado imediatamente.

### 0.4. Tipagem Estrita e Validação de Bordas
* **Proibição de Tipos Fracos:** Proibido o uso de `any` ou `unknown` genérico no TypeScript, variáveis sem anotação em Python, e `interface{}` genérico em Go.
* **Bordas Validadas:** Toda entrada externa (HTTP, Webhooks, Mensageria) deve ser validada obrigatoriamente com schemas determinísticos: **Zod / TypeBox** (TypeScript), **Pydantic V2** (Python) ou **Structs tipadas** (Go).
* **Tratamento de Erros:** Erros de API devem seguir estritamente o padrão **Problem Details (RFC 7807 / RFC 9457)**.

### 0.5. Comunicação, Interface Limpa e Ocultação de Tecnologias Internas
* **Máximo de 2 Palavras:** Em títulos, botões, abas ou ações da interface visual (ex: *Novo Passo*, *Rodar Script*, *Gerar IA*, *Ver Logs*).
* **Linguagem do Usuário:** Focada na ação, livre de jargões técnicos complexos.
* **Proibição de Tecnologias Internas na UI:** Nenhum nome de tecnologia de infraestrutura utilizada na construção ou funcionamento interno da plataforma pode ser exibido na interface para o usuário final — como *Postgres*, *LiveKit*, *Antigravity*, *ADK*, *GenAI*, *Whisper*, *Krisp*, *Piper* e ferramentas nativas.

### 0.6. Governança e Manutenção Viva de Skills de Agentes
* Toda alteração funcional, refatoração de endpoints, adição de ferramentas ou modificação de parâmetros de agentes exige a **revisão minuciosa e profunda das skills canônicas** em `.agents/skills/<skill_name>/SKILL.md`.
* É proibido manter manuais defasados, instruções obsoletas ou divergências conceituais entre o código executável e os manuais de skills.

### 0.7. Proibição Rígida de Fallbacks Ocultos e Protocolo de Notificação Detalhada
1. **Uso de Fallback Estritamente Sob Demanda Explícita:**
   - É expressamente proibido implementar, acionar ou recorrer a fallbacks automáticos, silenciosos ou mágicos (ex: tentar rotas/URLs alternativas sem aviso prévio, assumir IPs ou endpoints legados, ignorar parâmetros ausentes ou desviar chamadas por conta própria) a menos que o usuário solicite prévia e explicitamente.
   - O sistema nunca deve mascarar ausência de dados, troncos indisponíveis ou configurações incompletas por meio de atalhos paliativos ou silenciosos.
2. **Obrigação de Notificar Ausência de Informação (Fail-Fast e Ação Clara):**
   - Na ausência de qualquer parâmetro, credencial, rota ou configuração requerida para a operação, a aplicação/IA deve interromper imediatamente a execução (fail-fast) e notificar o usuário com riqueza de detalhes sobre o que está faltando, onde configurar, como preencher e exemplos práticos.

### 0.8. Auto-Destilação e Governança Contínua de Workflows (Workflow-to-Skill)
1. **Destilação Pós-Implementação:** Ao concluir a implementação de uma nova arquitetura, integração de microsserviços, rotina de banco de dados ou fluxo operacional relevante, consolidar o conhecimento na skill do respectivo domínio em `.agents/skills/<dominio>/SKILL.md`.

---

### 0.9. PROIBIÇÃO RÍGIDA DE ACESSOS DE BAIXO NÍVEL E ATALHOS DE INFRAESTRUTURA (ADERÊNCIA CANÔNICA AO DEPLOY_DEV.MD)

> [!CAUTION]
> **REGRA DE SEGURANÇA OPERACIONAL E CONFINAMENTO DE ESCOPO PARA A IA:**
> É TERMINANTEMENTE PROIBIDO para o agente ou desenvolvedor com IA buscar caminhos alternativos, atalhos de infraestrutura ou investigar arquivos do sistema operacional host para realizar deploy, testes ou execução em ambiente de desenvolvimento.

1. **Proibição Absoluta de Bisbilhotagem do Host e Comandos de Baixo Nível:**
   - É **estritamente proibido** inspecionar diretórios do sistema host (ex: `/etc/systemd/`, `/var/run/`, `/proc/`, `/sys/`, `/etc/`).
   - É **estritamente proibido** navegar e inspecionar manifestos, scripts ou configurações internas de outros projetos fora do seu escopo (ex: tentar ler `/home/marcio/ecosystem/control-plane/k8s` ou tentar mapear stacks de outros nós a partir de projetos como `chat`, `dialer-go`, `estudio-ia` ou `whatsapp-go`).
   - É **estritamente proibido** executar comandos diretos de orquestração de infraestrutura sem autorização prévia e fora do workflow canônico (ex: `kubectl get svc`, `kubectl apply`, `systemctl restart`, `docker run` direto no host).

2. **Aderência Exclusiva e Obrigatória aos Meios Canônicos (`deploy_dev.md`):**
   - Toda e qualquer operação de subida em ambiente de desenvolvimento, teste ou homologação efêmera **DEVE seguir única e exclusivamente os meios formais documentados no workflow do próprio projeto**:
     👉 **`.agents/workflows/deploy_dev.md`** (e `.agents/workflows/deploy-local.md`).
   - Os meios canônicos oficiais são:
     * **Em desenvolvimento local:** Serviços de apoio via `docker compose -f docker-compose.dev.yml` e execução nativa via runtime (`npm run dev`, `go run`, etc.).
     * **Em cluster de homologação K3s:** Disparo auditado via API REST do Control Plane (`POST http://localhost:3100/api/v1/deploys` com `environment: "test"`) ou via CLI Helper (`request_service.sh request ...`).

3. **Protocolo Obrigatório de Relato de Dificuldade ao Usuário (Fail-Fast Transparente):**
   - Se o endpoint do Control Plane não responder (`Connection Refused`), se uma porta não estiver aberta, se um pod falhar no healthcheck, ou se qualquer pré-requisito técnico estiver indisponível:
     - **A IA NUNCA DEVE tentar contornar a falha procurando outras maneiras mágicas ou bisbilhotando o host.**
     - **A IA DEVE interromper a execução imediatamente e reportar a dificuldade ao usuário com total clareza:**
       1. **O que foi tentado:** O comando canônico ou endpoint exato especificado no `deploy_dev.md`.
       2. **Qual foi o erro observado:** Código HTTP, mensagem de erro ou ausência de resposta.
       3. **Onde está o bloqueio:** Servidor, porta ou serviço dependente inacessível.
       4. **Ação Solicitada ao Usuário:** Pedir expressamente ao usuário que verifique o serviço ou tome a medida de correção de infraestrutura necessária.

---

## 1. Grafo de Execução e Anotações Canônicas em Código

Todo método, função de caso de uso ou rota pública deve ser documentado com metadados estruturados:

### Tags Obrigatórias:
* `@pattern`: Nome do Design Pattern utilizado e papel do componente (ex: `Strategy (Context)`, `Adapter`, `Repository`).
* `@governedBy`: Caminho relativo para o arquivo `.md` que dita a regra de negócio (`docs/rules/*.md` ou `.agents/ARCHITECT.md`).
* `@preExecution`: Validações prévias, travas de idempotência, dependências imediatas ou autorizações necessárias.
* `@postExecution`: Efeitos colaterais, persistência, disparo de eventos assíncronos ou logs transacionais.

---

## 2. Catálogo Oficial de Design Patterns e Matriz de Decisão

A IA nunca deve gerar código procedural monolítico ou cadeias excessivas de `if/else`. Deve selecionar obrigatoriamente um dos padrões abaixo:

| Cenário / Necessidade Identificada | Padrão Obrigatório | Localização Padrão |
| :--- | :--- | :--- |
| **Múltiplos fluxos de negócio alternáveis** | **Strategy** | `services/strategies/` ou `core/` |
| **Integração com SDKs de terceiros ou APIs parceiras** | **Adapter** | `adapters/` ou `providers/` |
| **Acesso ao banco de dados e controle transacional** | **Repository & Unit of Work** | `repositories/` |
| **Eventos assíncronos ou mensageria** | **Observer / Pub-Sub** | `events/` ou Composables |
| **Tratamento transversal (Auth, Auditoria, Rate Limit)** | **Middleware / Chain of Resp.** | `middlewares/` |
| **Chamadas externas sujeitas a instabilidade** | **Circuit Breaker** | `adapters/clients/` |

---

## 3. Protocolo Pre-Flight Git para Equipes Multi-Dev com IA

Antes de iniciar qualquer tarefa de codificação ou deploy:
1. **Checagem de Status:** Executar `git status` para validar se o workspace está limpo.
2. **Sincronização Remota:** Executar `git fetch` e `git pull --rebase` na branch de trabalho para garantir que alterações de outros desenvolvedores foram incorporadas.
3. **Isolamento de Branches:** Nunca codificar diretamente na branch principal (`main`). Criar branches específicas (`feature/`, `fix/`).

---

## 4. Diretrizes de Segurança, Isolamento e Zero Hardcoding

1. **Zero Hardcoding Absoluto:** Nenhuma chave de API, senha, token ou credencial pode ser inserida no código-fonte.
2. **Contrato Estrito (.env.example):** Nenhuma variável pode ser consumida no código sem constar no `.env.example`.
3. **Proibição de Caminhos Relativos de Segredos:** É proibido carregar arquivos de credenciais via caminhos relativos de pastas superiores (ex: `../../.env`).
4. **Resolução Dinâmica via Vault:** Em produção e teste no cluster K3s, as credenciais são injetadas exclusivamente via Secret Vault do Control Plane.
