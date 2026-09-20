import { execSync } from 'child_process';
import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

process.env.NODE_TLS_REJECT_UNAUTHORIZED = '0';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

function getGitHubConfig() {
    const envPaths = [
        path.join(__dirname, '../ecosystem/.env'),
        path.join(__dirname, '../ecosystem/.ENV'),
        path.join(__dirname, '.env')
    ];

    for (const envPath of envPaths) {
        if (fs.existsSync(envPath)) {
            const content = fs.readFileSync(envPath, 'utf8');
            const tokenMatch = content.match(/GITHUB_TOKEN\s*=\s*["']?([^"'\r\n]+)/);
            if (tokenMatch) {
                return {
                    token: tokenMatch[1].trim(),
                    owner: 'marcio-rgb',
                    repo: 'omni-dialer-go'
                };
            }
        }
    }

    try {
        const remoteUrl = execSync('git remote get-url origin', { encoding: 'utf8' }).trim();
        const match = remoteUrl.match(/https:\/\/(?:([^:@]+):)?([^@]+)@github\.com\/([^\/]+)\/([^\.]+)/);
        if (match) {
            const token = match[2].startsWith('ghp_') || match[2].startsWith('github_pat_') ? match[2] : (match[1] || match[2]);
            return {
                token: token,
                owner: match[3],
                repo: match[4]
            };
        }
    } catch (err) {
        // Ignored
    }

    return {
        token: process.env.GITHUB_TOKEN || '',
        owner: 'marcio-rgb',
        repo: 'omni-dialer-go'
    };
}

function getLocalEnvConfig() {
    const config = {
        portainerUrl: 'https://84.247.135.255:9443/api',
        portainerKey: 'ptr_IpehOIRXktqikYl3J7xGRkQEphHYU1mRNRRftORmXE0='
    };

    const envPaths = [
        path.join(__dirname, '../ecosystem/.env'),
        path.join(__dirname, '../ecosystem/.ENV'),
        path.join(__dirname, '.env')
    ];

    for (const envPath of envPaths) {
        if (fs.existsSync(envPath)) {
            const content = fs.readFileSync(envPath, 'utf8');
            const urlMatch = content.match(/PORTAINER_URL(?:_PROD)?\s*=\s*["']?([^"'\r\n]+)/);
            const keyMatch = content.match(/PORTAINER_KEY(?:_PROD)?\s*=\s*["']?([^"'\r\n]+)/);

            if (urlMatch) {
                let u = urlMatch[1].trim().replace(/\/$/, '');
                if (!u.endsWith('/api')) u += '/api';
                config.portainerUrl = u;
            }
            if (keyMatch) config.portainerKey = keyMatch[1].trim();
        }
    }
    return config;
}

async function sleep(ms) {
    return new Promise(resolve => setTimeout(resolve, ms));
}

async function main() {
    console.log('🚀 === INICIANDO AUTOMATIZAÇÃO DE DEPLOY DO DIALER-GO ===');

    const composePath = path.join(__dirname, 'docker-compose.yml');
    if (!fs.existsSync(composePath)) {
        console.error('❌ docker-compose.yml não encontrado no diretório.');
        process.exit(1);
    }

    const epoch = Math.floor(Date.now() / 1000);
    const envConfig = getLocalEnvConfig();
    if (!envConfig.portainerKey) {
        console.error('❌ PORTAINER_KEY_PROD não configurada.');
        process.exit(1);
    }

    // --- PASSO 1: Atualizar UPDATE_TIMESTAMP no docker-compose.yml local ---
    console.log('\n📝 1. Atualizando UPDATE_TIMESTAMP no docker-compose.yml...');
    try {
        let composeContent = fs.readFileSync(composePath, 'utf8');
        if (composeContent.includes('UPDATE_TIMESTAMP=')) {
            composeContent = composeContent.replace(/UPDATE_TIMESTAMP=\d+/g, `UPDATE_TIMESTAMP=${epoch}`);
        } else {
            console.error('❌ Linha UPDATE_TIMESTAMP não encontrada no docker-compose.yml');
            process.exit(1);
        }
        fs.writeFileSync(composePath, composeContent, 'utf8');
        console.log(`✅ UPDATE_TIMESTAMP atualizado para: ${epoch}`);
    } catch (err) {
        console.error('❌ Falha ao atualizar o arquivo docker-compose.yml:', err.message);
        process.exit(1);
    }

    // --- PASSO 2: Commit e push das alterações ---
    console.log('\n🔄 2. Realizando commit e push para o GitHub...');
    let commitMessage = process.argv[2] || '';
    if (!commitMessage) {
        try {
            const diffFiles = execSync('git diff --name-only', { encoding: 'utf8' })
                .trim()
                .split('\n')
                .filter(f => f.trim().length > 0 && !f.includes('docker-compose.yml'));

            if (diffFiles.length > 0) {
                commitMessage = `feat(prod): auto-deploy - updated ${diffFiles.map(f => path.basename(f)).slice(0, 5).join(', ')}`;
            } else {
                commitMessage = 'chore: trigger production dialer-go deploy';
            }
        } catch (err) {
            commitMessage = 'chore: trigger production dialer-go deploy';
        }
    }

    try {
        try {
            execSync('git config user.email || git config --global user.email "marcio@fastmob.com.br"');
            execSync('git config user.name || git config --global user.name "marcio-rgb"');
        } catch (gitErr) {
            // Ignored
        }

        execSync('git add .', { stdio: 'inherit' });
        const status = execSync('git status --porcelain', { encoding: 'utf8' }).trim();
        if (status.length > 0) {
            execSync(`git commit -m "${commitMessage.replace(/"/g, '\\"')}"`, { stdio: 'inherit' });
        }
        console.log('📤 Enviando alterações para o repositório GitHub (main)...');
        execSync('git push origin main', { stdio: 'inherit' });
    } catch (err) {
        console.error('❌ Falha ao realizar commit/push das alterações:', err.message);
        process.exit(1);
    }

    const commitSha = execSync('git rev-parse HEAD', { encoding: 'utf8' }).trim();
    console.log(`📌 Commit SHA atual: ${commitSha}`);

    // --- PASSO 3: Monitorar workflow do GitHub Actions ---
    console.log('\n⏳ 3. Aguardando workflow do GitHub Actions compilar a imagem...');
    const gitConfig = getGitHubConfig();
    let buildSuccess = false;
    const startTime = Date.now();

    if (!gitConfig.token) {
        console.warn('⚠️ GITHUB_TOKEN não configurado. Pulando monitoramento de build do GitHub e assumindo sucesso...');
        buildSuccess = true;
    } else {
        while (true) {
            if (Date.now() - startTime > 15 * 60 * 1000) {
                console.error('❌ Timeout de build no GitHub atingido (15 min).');
                process.exit(1);
            }

            try {
                const res = await fetch(`https://api.github.com/repos/${gitConfig.owner}/${gitConfig.repo}/actions/runs`, {
                    headers: {
                        'User-Agent': 'DeployScript',
                        'Authorization': `Bearer ${gitConfig.token}`
                    }
                });

                if (!res.ok) {
                    throw new Error(`GitHub API HTTP ${res.status} ${res.statusText}`);
                }

                const data = await res.json();
                const runs = data.workflow_runs || [];
                const currentRun = runs.find(run => run.head_sha === commitSha);

                if (currentRun) {
                    console.log(`   - Status: ${currentRun.status} | Conclusão: ${currentRun.conclusion || 'Em andamento...'}`);
                    
                    if (currentRun.status === 'completed') {
                        if (currentRun.conclusion === 'success') {
                            buildSuccess = true;
                            console.log('✅ COMPILAÇÃO CONCLUÍDA COM SUCESSO NO GITHUB ACTIONS!');
                            break;
                        } else {
                            console.error(`❌ Falha no build do GitHub. Status final: ${currentRun.conclusion}`);
                            process.exit(1);
                        }
                    }
                } else {
                    console.log('   - Aguardando início do workflow no GitHub...');
                }
            } catch (err) {
                console.warn('⚠️ Erro ao consultar API do GitHub:', err.message);
            }

            await sleep(15000);
        }
    }

    // --- PASSO 4: Deploy da Stack no Portainer de Produção ---
    if (buildSuccess) {
        console.log('\n🐳 4. Verificando Stack no Portainer de produção...');
        const composeContent = fs.readFileSync(composePath, 'utf8');
        const endpointId = 3;
        const swarmId = '8e1q9spcjxxfgnumvlld04eh3';
        const stackName = 'dialer-go';

        const authObj = {
            username: gitConfig.owner,
            password: gitConfig.token,
            serveraddress: 'ghcr.io'
        };
        const authHeader = Buffer.from(JSON.stringify(authObj)).toString('base64');

        // Pré-carrega a imagem atualizada no nó Docker com autenticação do GHCR
        console.log('   - Pré-carregando imagem Docker atualizada no nó Docker com credenciais GHCR...');
        try {
            const pullRes = await fetch(`${envConfig.portainerUrl}/endpoints/${endpointId}/docker/images/create?fromImage=` + encodeURIComponent('ghcr.io/marcio-rgb/omni-dialer-go:latest'), {
                method: 'POST',
                headers: {
                    'X-API-Key': envConfig.portainerKey,
                    'X-Registry-Auth': authHeader
                }
            });
            if (pullRes.ok) {
                console.log('   ✅ Imagem Docker puxada com sucesso no nó Swarm.');
            } else {
                console.warn('   ⚠️ Aviso ao puxar imagem via Docker API:', await pullRes.text());
            }
        } catch (pullErr) {
            console.warn('   ⚠️ Aviso ao pré-carregar imagem:', pullErr.message);
        }

        try {
            const resList = await fetch(`${envConfig.portainerUrl}/stacks`, {
                headers: { 'X-API-Key': envConfig.portainerKey }
            });
            if (!resList.ok) {
                throw new Error(`Erro ao listar stacks: HTTP ${resList.status}`);
            }
            const stacks = await resList.json();
            const existingStack = stacks.find(s => s.Name === stackName);

            if (existingStack) {
                console.log(`   - Stack existente encontrada (ID: ${existingStack.Id}). Atualizando...`);
                const resUpdate = await fetch(`${envConfig.portainerUrl}/stacks/${existingStack.Id}?endpointId=${endpointId}`, {
                    method: 'PUT',
                    headers: {
                        'X-API-Key': envConfig.portainerKey,
                        'Content-Type': 'application/json',
                        'X-Registry-Auth': authHeader
                    },
                    body: JSON.stringify({
                        StackFileContent: composeContent,
                        Env: [],
                        Prune: true,
                        PullImage: true
                    })
                });

                if (!resUpdate.ok) {
                    const errText = await resUpdate.text();
                    throw new Error(`Portainer Stack Update API HTTP ${resUpdate.status}: ${errText}`);
                }
                console.log('✅ STACK DIALER-GO ATUALIZADA E RECREADA COM SUCESSO NO PORTAINER!');
            } else {
                console.log(`   - Stack não encontrada. Criando nova stack "${stackName}"...`);
                const resCreate = await fetch(`${envConfig.portainerUrl}/stacks/create/swarm/string?endpointId=${endpointId}`, {
                    method: 'POST',
                    headers: {
                        'X-API-Key': envConfig.portainerKey,
                        'Content-Type': 'application/json',
                        'X-Registry-Auth': authHeader
                    },
                    body: JSON.stringify({
                        name: stackName,
                        swarmID: swarmId,
                        stackFileContent: composeContent,
                        env: []
                    })
                });

                if (!resCreate.ok) {
                    const errText = await resCreate.text();
                    throw new Error(`Portainer Stack Create API HTTP ${resCreate.status}: ${errText}`);
                }
                console.log('✅ STACK DIALER-GO CRIADA E DEPLOYADA COM SUCESSO NO PORTAINER!');
            }

            console.log('⏳ Aguardando convergência dos serviços Swarm...');
            await sleep(8000);
            try {
                const resServices = await fetch(`${envConfig.portainerUrl}/endpoints/${endpointId}/docker/services`, {
                    headers: { 'X-API-Key': envConfig.portainerKey }
                });
                if (resServices.ok) {
                    const services = await resServices.json();
                    const dialerService = services.find(s => s.Spec && s.Spec.Name === 'omni-dialer-go_dialer-go');
                    if (dialerService) {
                        const resTasks = await fetch(`${envConfig.portainerUrl}/endpoints/${endpointId}/docker/tasks?filters=${encodeURIComponent(JSON.stringify({ service: [dialerService.ID] }))}`, {
                            headers: { 'X-API-Key': envConfig.portainerKey }
                        });
                        if (resTasks.ok) {
                            const tasks = await resTasks.json();
                            const latestTask = tasks[0];
                            console.log(`📊 Status do serviço dialer-go: ${latestTask ? latestTask.Status.State : 'indisponível'} (${latestTask ? latestTask.Status.Message : ''})`);
                        }
                    }
                }
            } catch (statusErr) {
                // Ignore status check error
            }

            console.log('🎉 PROCESSO DE DEPLOY COMPLETO CONCLUÍDO!');
        } catch (err) {
            console.error('❌ Falha ao realizar deploy no Portainer:', err.message);
            process.exit(1);
        }
    }
}

main();
