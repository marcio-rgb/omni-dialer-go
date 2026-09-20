import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

process.env.NODE_TLS_REJECT_UNAUTHORIZED = '0';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

function getLocalPortainerConfig() {
    const config = {
        url: 'https://localhost:9443/api',
        key: 'ptr_NKC/di0i5hzn/O7anLGuwZ64VKf7Czy+hL44vncTAIY=',
        endpointId: 3,
        stackName: 'dialer-go'
    };

    const envPaths = [
        path.join(__dirname, '.agents/servers.ENV'),
        path.join(__dirname, '.env')
    ];

    for (const envPath of envPaths) {
        if (fs.existsSync(envPath)) {
            const content = fs.readFileSync(envPath, 'utf8');
            const urlMatch = content.match(/LOCAL_PORTAINER_URL\s*=\s*["']?([^"'\r\n]+)/);
            const keyMatch = content.match(/LOCAL_PORTAINER_KEY\s*=\s*["']?([^"'\r\n]+)/);
            const epMatch = content.match(/LOCAL_PORTAINER_ENDPOINT_ID\s*=\s*["']?([^"'\r\n]+)/);

            if (urlMatch) {
                let u = urlMatch[1].trim().replace(/\/$/, '');
                if (!u.endsWith('/api')) u += '/api';
                config.url = u;
            }
            if (keyMatch) config.key = keyMatch[1].trim();
            if (epMatch) config.endpointId = parseInt(epMatch[1].trim(), 10) || 3;
        }
    }
    return config;
}

function generateStackYaml() {
    return `
# =============================================================================
# STACK: DIALER-GO ENGINE & TELEFONIA (MODO LOCAL)
# Descritor texto puro para o Web Editor do Portainer Local (Endpoint 3)
# Utiliza o PostgreSQL existente (postgres-postgres-1 / Stack ID 12)
# =============================================================================

services:
  dialer-go:
    image: dialer-go:latest
    container_name: dialer-go-core
    restart: unless-stopped
    ports:
      - "8085:8080"
    environment:
      - PORT=8080
      - OPERATION_MODE=dialer
      - DATABASE_URL=postgres://postgres:ojr6DNM25InIg3qoYI4nbUL5Cx04iCiC@postgres-postgres-1:5432/dialer_db?sslmode=disable
      - REDIS_ADDR=dialer-redis:6379
      - REDIS_DB=0
      - ASTERISK_AMI_HOST=host.docker.internal
      - ASTERISK_AMI_PORT=5038
      - ASTERISK_AMI_USER=dialer_admin
      - ASTERISK_AMI_PASS=dialer_secret_ami
      - INITIAL_WHITELIST_IPS=127.0.0.1,::1,172.16.0.0/12,192.168.0.0/16
      - MAX_GLOBAL_CHANNELS=60
      - HUMAN_RESERVED_QUOTA=10
      - VOSK_SERVER_URL=ws://dialer-vosk:2700
    extra_hosts:
      - "host.docker.internal:host-gateway"
    depends_on:
      - dialer-redis
      - dialer-vosk
    networks:
      - omnichat_network

  dialer-redis:
    image: redis:7-alpine
    container_name: dialer-cache
    restart: unless-stopped
    ports:
      - "6379:6379"
    volumes:
      - dialer-redisdata:/data
    networks:
      - omnichat_network

  dialer-vosk:
    image: alphacep/kaldi-vosk-server:latest
    container_name: dialer-vosk
    restart: unless-stopped
    command: python3 /opt/vosk-server/websocket/asr_server.py /opt/vosk-model/model
    ports:
      - "2700:2700"
    volumes:
      - /home/marcio/ecosystem/dialer-go/vosk-model:/opt/vosk-model/model
    environment:
      - VOSK_SAMPLE_RATE=8000
    networks:
      - omnichat_network

volumes:
  dialer-redisdata:

networks:
  omnichat_network:
    external: true
`.trim();
}

async function main() {
    console.log('🚀 === DEPLOY DA STACK LOCAL (PORTAINER MODO TEXTO) ===');
    const cfg = getLocalPortainerConfig();
    console.log(`📡 Conectando ao Portainer: ${cfg.url}`);
    console.log(`🎯 Endpoint: ${cfg.endpointId} | Stack: ${cfg.stackName}`);

    const stackContent = generateStackYaml();

    // 1. Consulta stacks existentes no endpoint
    const listRes = await fetch(`${cfg.url}/stacks?endpointId=${cfg.endpointId}`, {
        headers: { 'X-API-Key': cfg.key }
    });

    if (!listRes.ok) {
        console.error(`❌ Falha ao listar stacks: HTTP ${listRes.status} ${listRes.statusText}`);
        const errText = await listRes.text();
        console.error(errText);
        process.exit(1);
    }

    const stacks = await listRes.json();
    const existingStack = stacks.find(s => s.Name === cfg.stackName);

    if (existingStack) {
        console.log(`🔄 Atualizando Stack existente ID ${existingStack.Id} (${cfg.stackName}) via Web Editor / String...`);
        const updateRes = await fetch(`${cfg.url}/stacks/${existingStack.Id}?endpointId=${cfg.endpointId}`, {
            method: 'PUT',
            headers: {
                'X-API-Key': cfg.key,
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                stackFileContent: stackContent,
                env: [],
                prune: true,
                pullImage: false
            })
        });

        if (!updateRes.ok) {
            console.error(`❌ Falha ao atualizar stack: HTTP ${updateRes.status}`);
            console.error(await updateRes.text());
            process.exit(1);
        }
        console.log('✅ Stack atualizada com sucesso no Portainer!');
    } else {
        console.log(`✨ Criando nova Stack (${cfg.stackName}) em modo Texto Puro...`);
        const createRes = await fetch(`${cfg.url}/stacks/create/standalone/string?endpointId=${cfg.endpointId}`, {
            method: 'POST',
            headers: {
                'X-API-Key': cfg.key,
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                name: cfg.stackName,
                stackFileContent: stackContent,
                env: []
            })
        });

        if (!createRes.ok) {
            console.error(`❌ Falha ao criar stack: HTTP ${createRes.status}`);
            console.error(await createRes.text());
            process.exit(1);
        }
        console.log('✅ Stack criada com sucesso no Portainer Local!');
    }

    console.log('\n⏳ Aguardando 5 segundos para estabilização dos containers...');
    await new Promise(r => setTimeout(r, 5000));

    console.log('\n🔍 Validando HealthCheck HTTP do Dialer-Go...');
    try {
        const healthRes = await fetch('http://localhost:8085/health');
        const healthData = await healthRes.json();
        console.log('📊 Status do HealthCheck:', JSON.stringify(healthData, null, 2));
    } catch (e) {
        console.warn('⚠️ HealthCheck HTTP inicial ainda indisponível:', e.message);
    }

    console.log('\n🎉 Deploy concluído! Acesse o painel web em: http://localhost:8085');
}

main().catch(err => {
    console.error('❌ Erro inesperado:', err);
    process.exit(1);
});
