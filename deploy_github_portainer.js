import { execSync } from 'child_process';
import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

process.env.NODE_TLS_REJECT_UNAUTHORIZED = '0';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

function getGitHubConfig() {
    let token = process.env.GITHUB_TOKEN || '';

    // Permite passar token como argumento se começar com ghp_ ou github_pat_
    if (process.argv[2] && (process.argv[2].startsWith('ghp_') || process.argv[2].startsWith('github_pat_'))) {
        token = process.argv[2].trim();
    }

    if (!token) {
        const envPaths = [
            path.join(__dirname, '.env'),
            path.join(__dirname, '../ecosystem/.env'),
            path.join(__dirname, '../ecosystem/.ENV')
        ];

        for (const envPath of envPaths) {
            if (fs.existsSync(envPath)) {
                const content = fs.readFileSync(envPath, 'utf8');
                const tokenMatch = content.match(/GITHUB_TOKEN\s*=\s*["']?([^"'\r\n]+)/);
                if (tokenMatch) {
                    token = tokenMatch[1].trim();
                    break;
                }
            }
        }
    }

    return {
        token: token,
        owner: 'marcio-rgb',
        repo: 'omni-dialer-go'
    };
}

function getLocalEnvConfig() {
    const config = {
        portainerUrl: 'https://84.247.135.255:9443/api',
        portainerKey: 'ptr_IpehOIRXktqikYl3J7xGRkQEphHYU1mRNRRftORmXE0='
    };

    const envPath = path.join(__dirname, '.env');
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

    // --- PASSO 2: Commit e push das alterações para o GitHub ---
    console.log('\n🔄 2. Sincronizando e publicando no GitHub (marcio-rgb/omni-dialer-go)...');
    const gitConfig = getGitHubConfig();
    if (!gitConfig.token) {
        console.error('❌ GITHUB_TOKEN não configurado no .env ou via parâmetro.');
        process.exit(1);
    }

    let commitMessage = '';
    if (process.argv[2] && (process.argv[2].startsWith('ghp_') || process.argv[2].startsWith('github_pat_'))) {
        commitMessage = process.argv[3] || '';
    } else {
        commitMessage = process.argv[2] || '';
    }

    if (!commitMessage) {
        commitMessage = `feat(prod): auto-deploy dialer-go update timestamp ${epoch}`;
    }

    const deployRepoDir = path.join('/tmp', 'omni-dialer-go-repo');
    const repoAuthUrl = `https://x-access-token:${gitConfig.token}@github.com/${gitConfig.owner}/${gitConfig.repo}.git`;

    try {
        if (!fs.existsSync(path.join(deployRepoDir, '.git'))) {
            console.log(`   - Clonando repositório GitHub em ${deployRepoDir}...`);
            execSync(`rm -rf "${deployRepoDir}"`);
            execSync(`git clone --depth 1 "${repoAuthUrl}" "${deployRepoDir}"`, { stdio: 'inherit' });
        } else {
            console.log(`   - Atualizando repositório local em ${deployRepoDir}...`);
            execSync(`git -C "${deployRepoDir}" remote set-url origin "${repoAuthUrl}"`);
            execSync(`git -C "${deployRepoDir}" fetch origin main`, { stdio: 'inherit' });
            execSync(`git -C "${deployRepoDir}" reset --hard origin/main`, { stdio: 'inherit' });
        }

        // Sincroniza arquivos de dialer-go para o repositório de publicação
        console.log('   - Sincronizando arquivos do projeto...');
        execSync(`rsync -av --delete \
            --exclude='.git' \
            --exclude='.env*' \
            --exclude='*.env' \
            --exclude='*.ENV' \
            --exclude='temp/' \
            --exclude='dialer-bin' \
            --exclude='node_modules' \
            --exclude='.cache' \
            "${__dirname}/" "${deployRepoDir}/"`, { stdio: 'inherit' });

        execSync(`git -C "${deployRepoDir}" config user.email "marcio@fastmob.com.br"`);
        execSync(`git -C "${deployRepoDir}" config user.name "marcio-rgb"`);

        execSync(`git -C "${deployRepoDir}" add .`, { stdio: 'inherit' });
        const ghStatus = execSync(`git -C "${deployRepoDir}" status --porcelain`, { encoding: 'utf8' }).trim();
        if (ghStatus.length > 0) {
            execSync(`git -C "${deployRepoDir}" commit -m "${commitMessage.replace(/"/g, '\\"')}"`, { stdio: 'inherit' });
        } else {
            console.log('   ℹ️ Nenhuma alteração detectada para commit.');
        }

        console.log('📤 Enviando alterações para o repositório GitHub (main)...');
        execSync(`git -C "${deployRepoDir}" push origin main`, { stdio: 'inherit' });
    } catch (err) {
        console.error('❌ Falha ao realizar commit/push das alterações:', err.message);
        process.exit(1);
    }

    const commitSha = execSync(`git -C "${deployRepoDir}" rev-parse HEAD`, { encoding: 'utf8' }).trim();
    console.log(`📌 Commit SHA publicado no GitHub: ${commitSha}`);

    // --- PASSO 3: Monitorar workflow do GitHub Actions ---
    console.log('\n⏳ 3. Aguardando workflow do GitHub Actions compilar a imagem...');
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
        console.log(`   - Alvo Portainer: ${envConfig.portainerUrl}`);
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
            const existingStack = Array.isArray(stacks) ? stacks.find(s => s.Name === stackName || s.Id === 1) : null;

            if (existingStack) {
                console.log(`   - Stack existente encontrada (ID: ${existingStack.Id}, Nome: ${existingStack.Name}). Atualizando...`);
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
                    const dialerService = services.find(s => s.Spec && (s.Spec.Name === 'dialer-go_dialer-go' || s.Spec.Name === 'omni-dialer-go_dialer-go'));
                    if (dialerService) {
                        const resTasks = await fetch(`${envConfig.portainerUrl}/endpoints/${endpointId}/docker/tasks?filters=${encodeURIComponent(JSON.stringify({ service: [dialerService.ID] }))}`, {
                            headers: { 'X-API-Key': envConfig.portainerKey }
                        });
                        if (resTasks.ok) {
                            const tasks = await resTasks.json();
                            tasks.sort((a, b) => new Date(b.UpdatedAt) - new Date(a.UpdatedAt));
                            const latestTask = tasks[0];
                            console.log(`📊 Status do serviço dialer-go: ${latestTask ? latestTask.Status.State : 'indisponível'} (${latestTask ? (latestTask.Status.Message || latestTask.Status.Err || '') : ''})`);
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
