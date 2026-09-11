import { execSync } from 'child_process';
import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

process.env.NODE_TLS_REJECT_UNAUTHORIZED = '0';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

function getGitHubConfig() {
    try {
        const remoteUrl = execSync('git remote get-url origin', { encoding: 'utf8' }).trim();
        const match = remoteUrl.match(/https:\/\/([^@]+)@github\.com\/([^\/]+)\/([^\.]+)/);
        if (match) {
            return {
                token: match[1],
                owner: match[2],
                repo: match[3]
            };
        }
    } catch (err) {
        // Ignored
    }

    const envPaths = [
        path.join(__dirname, '../ecosystem/.env'),
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

    return {
        token: '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',
        owner: 'marcio-rgb',
        repo: 'omni-dialer-go'
    };
}

function getLocalEnvConfig() {
    const config = {
        portainerUrl: 'https://portainer.creditobr.org/api',
        portainerKey: 'ptr_ubSfIbjJaga7zSenoBqm8mTPzWvccL/jIuWo3t9k6bQ='
    };

    const envPaths = [
        path.join(__dirname, '../ecosystem/.env'),
        path.join(__dirname, '../ecosystem/.ENV'),
        path.join(__dirname, '.env')
    ];

    for (const envPath of envPaths) {
        if (fs.existsSync(envPath)) {
            const content = fs.readFileSync(envPath, 'utf8');
            const urlMatch = content.match(/PORTAINER_URL_PROD\s*=\s*["']?([^"'\r\n]+)/);
            const keyMatch = content.match(/PORTAINER_KEY_PROD\s*=\s*["']?([^"'\r\n]+)/);

            if (urlMatch) config.portainerUrl = urlMatch[1].trim().replace(/\/$/, '') + '/api';
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
        const endpointId = 1;
        const swarmId = 'w2e8eye8z0r9688wzbkubemtw';
        const stackName = 'omni-dialer-go';

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
                        'Content-Type': 'application/json'
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
                        'Content-Type': 'application/json'
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
            console.log('🎉 PROCESSO DE DEPLOY COMPLETO CONCLUÍDO!');
        } catch (err) {
            console.error('❌ Falha ao realizar deploy no Portainer:', err.message);
            process.exit(1);
        }
    }
}

main();
