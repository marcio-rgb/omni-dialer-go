# Credenciais de Acesso SSH - Servidor de Testes / Simulador

> [!NOTE] Ambiente isolado de testes e simulação de tráfego de chamadas.

* **Host / IP:** `38.242.219.186`

* **Usuário:** `root`

- **Senha SSH:** `XH9deYZ8I0KqPDjnF05pfH8x11`

- **Porta SSH:** `22`

***

## Comandos Rápidos de Acesso e Gestão

### Acesso SSH Direto

```bash
sshpass -p 'XH9deYZ8I0KqPDjnF05pfH8x11' ssh -o StrictHostKeyChecking=no root@38.242.219.186
```

### Sincronização de Configurações

```bash
sshpass -p 'XH9deYZ8I0KqPDjnF05pfH8x11' scp -o StrictHostKeyChecking=no mode/simulator/* root@38.242.219.186:/opt/simulator/config/
```

### Recarregar Asterisk Remotamente

```bash
sshpass -p 'XH9deYZ8I0KqPDjnF05pfH8x11' ssh -o StrictHostKeyChecking=no root@38.242.219.186 "asterisk -rx 'dialplan reload' && asterisk -rx 'pjsip reload'"
```
