#!/usr/bin/env python3
import urllib.request
import json
import time
import subprocess
import os
import sys

print('=' * 80)
print('🚀 TESTE DE STRESS CONTÍNUO COM HOT-SWAP DINÂMICO ENTRE NÓS DO CLASSIFICATOR')
print('=' * 80)

# 0. Garante que os 3 nós estão ativos e reinicia o router para zerar contadores
print('>> [PRE-FLIGHT] Verificando e escalando os 3 nós para 1 réplica...')
subprocess.run('docker service scale classificator_classificator-node1=1 classificator_classificator-node2=1 classificator_classificator-node3=1', shell=True, check=True)
time.sleep(2)

print('>> [PRE-FLIGHT] Reiniciando classificator-router para zerar contadores de telemetria...')
subprocess.run('docker service update --force classificator_classificator-router', shell=True, check=True)
time.sleep(4)

# Verifica se o router respondeu
try:
    with urllib.request.urlopen('http://127.0.0.1:2809/status') as r:
        init_status = json.loads(r.read().decode('utf-8'))
        print(f'>> [ROUTER INICIAL] Porta Ativa: {init_status["active_address"]} | Failovers: {init_status["failover_count"]}')
        print(f'>> Portas no Anel: {[p["address"] for p in init_status["ports"]]}')
except Exception as e:
    print(f'ERRO conectando ao Router: {e}')
    sys.exit(1)

# Função para injetar demanda preditiva
def send_predictive_demand(num_agents=35):
    agents = [{'agent_id': f'agent-{i}', 'skills': []} for i in range(1, num_agents + 1)]
    payload = {
        'tenant_id': 'default',
        'campaign_id': 'campaign-stress-40',
        'aggressiveness': 1.2,
        'min_channels_per_agent': 1,
        'available_agents': agents
    }
    req_data = json.dumps(payload).encode('utf-8')
    req = urllib.request.Request(
        'http://127.0.0.1:8081/api/v1/predictive/demand',
        data=req_data,
        headers={'Content-Type': 'application/json', 'X-Tenant-Id': 'default'}
    )
    try:
        with urllib.request.urlopen(req, timeout=3) as resp:
            data = json.loads(resp.read().decode('utf-8'))
            return data.get('data', {}).get('dialing_channels', 0)
    except Exception as err:
        return f'Err: {err}'

# Registra histórico de telemetria
telemetry_timeline = []

start_time = time.time()
print('\n>> INICIANDO DISPAROS CONTÍNUOS & MONITORAMENTO (Duração prevista: 45s)...')

swap1_done = False
swap2_done = False

for sec in range(1, 46):
    # Injeta demanda preditiva a cada 3 segundos até o segundo 28
    demand_info = ''
    if sec <= 28 and (sec % 3 == 1):
        channels_dialing = send_predictive_demand(35)
        demand_info = f' [Injeção Demanda: {channels_dialing} canais]'
    
    # Consulta Asterisk canais ativos
    try:
        ast_out = subprocess.check_output(
            "docker exec $(docker ps -q -f name=asterisk) asterisk -rx 'core show channels count'",
            shell=True, stderr=subprocess.DEVNULL
        ).decode('utf-8')
        active_channels = 0
        for line in ast_out.strip().split('\n'):
            if 'active channel' in line.lower():
                active_channels = int(line.strip().split()[0])
                break
    except Exception:
        active_channels = -1

    # Consulta Router Status
    router_status = {}
    try:
        with urllib.request.urlopen('http://127.0.0.1:2809/status', timeout=2) as r:
            router_status = json.loads(r.read().decode('utf-8'))
    except Exception as e:
        router_status = {'error': str(e)}

    active_addr = router_status.get('active_address', 'N/A')
    failovers = router_status.get('failover_count', 0)
    ports = router_status.get('ports', [])
    served_summary = '/'.join([f"P{p['id']}:{p['total_served']}(act:{p['active_conns']})" for p in ports]) if ports else 'N/A'

    print(f'[{sec:02d}s] Ast: {active_channels:02d} canais | Router: {active_addr} (Failovers: {failovers}) | {served_summary}{demand_info}')

    telemetry_timeline.append({
        'sec': sec,
        'active_channels': active_channels,
        'active_addr': active_addr,
        'failovers': failovers,
        'ports': ports
    })

    # COMUTAÇÃO 1: Derrubar Node 1 no segundo 10
    if sec == 10 and not swap1_done:
        print('\n' + '='*80)
        print('⚡⚡⚡ [HOT-SWAP 1] DERRUBANDO CLASSICATOR-NODE1 (Porta 2801) AO VIVO! ⚡⚡⚡')
        print('='*80)
        subprocess.Popen('docker service scale classificator_classificator-node1=0', shell=True)
        swap1_done = True

    # COMUTAÇÃO 2: Derrubar Node 2 no segundo 22
    if sec == 22 and not swap2_done:
        print('\n' + '='*80)
        print('⚡⚡⚡ [HOT-SWAP 2] DERRUBANDO CLASSICATOR-NODE2 (Porta 2802) AO VIVO! ⚡⚡⚡')
        print('='*80)
        subprocess.Popen('docker service scale classificator_classificator-node2=0', shell=True)
        swap2_done = True

    time.sleep(1)

print('\n' + '='*80)
print('✅ FASE DE EXECUÇÃO E COMUTAÇÕES CONCLUÍDA. RESTAURANDO NÓS E ANALISANDO DADOS...')
print('='*80)

# Restaura réplicas
subprocess.run('docker service scale classificator_classificator-node1=1 classificator_classificator-node2=1 classificator_classificator-node3=1', shell=True)

# Status consolidado do router
try:
    with urllib.request.urlopen('http://127.0.0.1:2809/status') as r:
        final_router = json.loads(r.read().decode('utf-8'))
        print('\n📊 TELEMETRIA FINAL DO ROUTER:')
        print(json.dumps(final_router, indent=2))
except Exception as e:
    print(f'Erro obtendo status final do router: {e}')

print('\n=== AUDITORIA DE CDRs NO BANCO DE DADOS (dialer_db.cdrs) ===')
pg_cmd = "SELECT disposition, count(*), count(transcription) as with_transcription, round(avg(duration_seconds), 1) as avg_duration_sec, round(avg(billsec_seconds), 1) as avg_billsec_sec FROM cdrs WHERE created_at >= (now() - interval '3 minutes') GROUP BY disposition;"

try:
    res = subprocess.check_output(
        f"docker exec $(docker ps -q -f name=postgres_postgres | head -n 1) psql -U postgres -d dialer_db -c \"{pg_cmd}\"",
        shell=True
    ).decode('utf-8')
    print(res)
except Exception as e:
    print(f'Erro na consulta do banco: {e}')

print('\n=== AMOSTRAGEM DE TRANSCRIÇÕES CAPTURADAS PELO VOSK (ÚLTIMOS CDRs) ===')
sample_cmd = "SELECT id, phone, disposition, duration_seconds, LEFT(transcription, 50) as transcription_sample FROM cdrs WHERE created_at >= (now() - interval '3 minutes') AND transcription IS NOT NULL AND transcription != '' LIMIT 5;"

try:
    res_sample = subprocess.check_output(
        f"docker exec $(docker ps -q -f name=postgres_postgres | head -n 1) psql -U postgres -d dialer_db -c \"{sample_cmd}\"",
        shell=True
    ).decode('utf-8')
    print(res_sample)
except Exception as e:
    print(f'Erro na amostragem: {e}')

print('\n=== TESTE DE STRESS E COMUTAÇÃO DINÂMICA CONCLUÍDO COM SUCESSO! ===')
