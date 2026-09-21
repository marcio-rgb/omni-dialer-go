# Protocolo Pre-Flight Git para Equipes Multi-Dev com IA (Dialer-Go)

Este documento estabelece o funil anti-sobrescrita, o gerenciamento de remotes e as práticas obrigatórias de sincronização de código no **Dialer-Go** integrado ao repositório unificado no **Git Forgejo**.

---

## 1. Configuração do Remote Canônico (Git Forgejo)

O ecossistema OmniChat reside no monorepo unificado no **Forgejo Git**. O remote principal deve estar configurado para o servidor de controle de versão do cluster.

### 1.1. Verificar Remotes Configurados:
```bash
git remote -v
```

### 1.2. Adicionar ou Ajustar o Remote Forgejo:
- **Acesso HTTP (Porta 30080):**
  ```bash
  git remote add forgejo http://<USUARIO>:<SENHA_OU_TOKEN>@207.180.251.25:30080/marcio/omnichat-ecosystem.git
  ```
- **Acesso SSH (Porta 30222):**
  ```bash
  git remote add forgejo ssh://git@84.247.135.255:30222/marcio/omnichat-ecosystem.git
  ```
- **Acesso Interno do Cluster K3s:**
  ```bash
  git remote add forgejo http://forgejo.omnichat.svc.cluster.local:3000/marcio/omnichat-ecosystem.git
  ```

---

## 2. Funil Anti-Sobrescrita Obrigatório

Antes de iniciar qualquer edição de código, compilação ou alteração de dialplan:

1. **Checagem de Status Local:**
   ```bash
   git status
   ```
   Validar se o workspace está limpo e sem alterações conflitantes.

2. **Sincronização com o Remoto Forgejo:**
   ```bash
   git fetch forgejo
   git pull --rebase forgejo main
   ```
   Garantir que os commits mais recentes de outros desenvolvedores e agentes foram incorporados.

3. **Isolamento de Branches para Novas Funcionalidades:**
   - É proibido codificar diretamente na branch `main`.
   - Padrão de branches:
     - `feature/<nome-da-funcionalidade>`
     - `fix/<correcao-especifica>`
     - `refactor/<modulo-refatorado>`

---

## 3. Padrão de Commits Semânticos

Os commits devem seguir a convenção *Conventional Commits*:
- `feat(amd): aprimorar sensibilidade do classificador vosk`
- `fix(pacing): ajustar calculo de overdialing para operacao hibrida`
- `refactor(ami): otimizar pool de conexoes do asterisk manager`
- `docs(forgejo): documentar deploy via control plane no k3s`

---

## 4. Segurança de Credenciais e Zero Hardcoding

- **Zero Hardcoding e Zero Commit de Senhas:** Nenhuma credencial de AMI, Asterisk, PostgreSQL ou Redis deve ser comitada.
- **Injeção via Secret Vault:** Todas as credenciais de produção são injetadas em tempo de execução via **Secret Vault (AES-256-GCM)** do Control Plane.
- **Proibição de Artefatos Portainer:** Nunca criar ou manter tokens ou scripts de deploy baseados no Portainer.
