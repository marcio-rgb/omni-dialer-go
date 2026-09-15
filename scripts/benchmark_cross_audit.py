#!/usr/bin/env python3
"""
benchmark_cross_audit.py
------------------------
Rotina padrão de auditoria forense e cruzamento de dados entre:
1. Gabarito do Simulador de Testes (38.242.219.186: /opt/simulator/logs/simulator_played.jsonl)
2. Tabela Canônica de Produção do Servidor Novo (84.247.135.255: dialer_db.cdrs)

Zero modificação no discador ou tabelas do servidor novo.
"""

import subprocess
import json
import sys
from datetime import datetime, timezone

SERVER_NEW_IP = "84.247.135.255"
SERVER_SIM_IP = "38.242.219.186"

def fetch_simulator_ground_truth():
    """Busca os registros de áudios tocados no servidor simulador de testes."""
    cmd = f"ssh -o StrictHostKeyChecking=no root@{SERVER_SIM_IP} 'docker exec asterisk_simulator cat /var/log/asterisk/simulator_played.jsonl 2>/dev/null || cat /opt/simulator/logs/simulator_played.jsonl'"
    try:
        out = subprocess.check_output(cmd, shell=True).decode('utf-8')
    except Exception as e:
        print(f"❌ Erro ao ler gabarito do simulador ({SERVER_SIM_IP}): {e}")
        return []

    records = []
    for line in out.strip().split('\n'):
        if line.strip():
            try:
                records.append(json.loads(line.strip()))
            except Exception:
                pass
    return records

def fetch_dialer_cdrs(limit_minutes=60):
    """Consulta a tabela canônica cdrs no banco de produção dialer_db do servidor novo."""
    sql = f"""
    SELECT json_build_object(
        'id', id,
        'phone', phone,
        'disposition', disposition,
        'duration_seconds', duration_seconds,
        'billsec_seconds', billsec_seconds,
        'transcription', COALESCE(transcription, ''),
        'created_at', created_at,
        'answered_at', answered_at
    )::text
    FROM cdrs
    WHERE created_at >= (now() - interval '{limit_minutes} minutes')
    ORDER BY created_at DESC;
    """
    cmd = f"ssh -o StrictHostKeyChecking=no root@{SERVER_NEW_IP} \"docker exec -i \\$(docker ps -q -f name=postgres_postgres | head -n 1) psql -U postgres -d dialer_db -t -c \\\"{sql}\\\"\""
    try:
        out = subprocess.check_output(cmd, shell=True).decode('utf-8')
    except Exception as e:
        print(f"❌ Erro ao consultar cdrs no servidor novo ({SERVER_NEW_IP}): {e}")
        return []

    cdrs = []
    for line in out.strip().split('\n'):
        line = line.strip()
        if line and line.startswith('{'):
            try:
                cdrs.append(json.loads(line))
            except Exception:
                pass
    return cdrs

def main():
    print("=" * 85)
    print("🔍 AUDITORIA FORENSE DE CLASSIFICAÇÃO: GABARITO SIMULADOR vs. DIALER-GO CDRS")
    print("=" * 85)

    sim_records = fetch_simulator_ground_truth()
    dialer_cdrs = fetch_dialer_cdrs(120)

    if not sim_records:
        print("⚠️ Nenhum registro encontrado no log do simulador.")
        sys.exit(0)

    # Indexa CDRs pelo telefone (último CDR por telefone)
    cdr_by_phone = {}
    for c in dialer_cdrs:
        p = str(c.get('phone', '')).strip()
        if p and p not in cdr_by_phone:
            cdr_by_phone[p] = c

    # Indexa registros do simulador pelo telefone
    sim_by_phone = {}
    for s in sim_records:
        p = str(s.get('phone', '')).strip()
        if p:
            sim_by_phone[p] = s

    total_evaluated = 0
    true_humans = 0
    true_machines = 0
    false_positives = 0  # Humano derrubado (descarte indevido)
    false_negatives = 0  # Caixa postal entregue
    unmatched = 0

    print(f"\n>> Cruzando chamadas (Simulador: {len(sim_records)} | CDRs: {len(dialer_cdrs)})...\n")
    print(f"{'TELEFONE':12s} | {'ÁUDIO TOCADO':35s} | {'GABARITO':7s} | {'DISPOSIÇÃO':10s} | {'RESULTADO':14s}")
    print("-" * 85)

    for phone, sim_info in sim_by_phone.items():
        audio_name = sim_info.get('audio_name', 'N/A')
        expected = sim_info.get('expected_type', 'UNKNOWN').upper()
        
        cdr = cdr_by_phone.get(phone)
        if not cdr:
            print(f"{phone:12s} | {audio_name[:35]:35s} | {expected:7s} | {'SEM CDR':10s} | ⚠️ NÃO DISPARADO")
            unmatched += 1
            continue

        total_evaluated += 1
        disp = str(cdr.get('disposition', '')).upper()
        duration = cdr.get('duration_seconds', 0)

        # Regra de classificação de desfecho:
        # Se esperado HUMAN: Sucesso se ANSWERED ou DELIVERED. Falha (FP) se ABANDONED.
        # Se esperado MACHINE: Sucesso se ABANDONED (desligou rápido). Falha (FN) se DELIVERED.
        is_delivered = disp in ['DELIVERED', 'ANSWERED']
        is_dropped = disp in ['ABANDONED', 'FAILED']

        if expected == 'HUMAN':
            if is_delivered:
                true_humans += 1
                outcome = "✅ HUMANO OK"
            else:
                false_positives += 1
                outcome = "🚨 FALSO POSITIVO"
        elif expected == 'MACHINE':
            if is_dropped:
                true_machines += 1
                outcome = "✅ CAIXA OK"
            else:
                false_negatives += 1
                outcome = "⚠️ FALSO NEGATIVO"
        else:
            outcome = "❓ INDEFINIDO"

        print(f"{phone:12s} | {audio_name[:35]:35s} | {expected:7s} | {disp:10s} | {outcome:14s}")

    # Consolidação Estatística
    print("\n" + "=" * 85)
    print("📊 MATRIZ DE CONFUSÃO & MÉTRICAS FORENSES")
    print("=" * 85)
    print(f"• Total de Chamadas Avaliadas : {total_evaluated}")
    print(f"• Verdadeiros Humanos (Atendidos) : {true_humans}")
    print(f"• Verdadeiras Máquinas (Descartadas) : {true_machines}")
    print(f"• Falsos Positivos (Humanos Derrubados): {false_positives} (Regra de Ouro: deve ser 0%)")
    print(f"• Falsos Negativos (Máquinas Entregues) : {false_negatives}")

    if total_evaluated > 0:
        accuracy = ((true_humans + true_machines) / total_evaluated) * 100.0
        fp_rate = (false_positives / total_evaluated) * 100.0
        fn_rate = (false_negatives / total_evaluated) * 100.0
        print(f"\n📈 Acurácia Global              : {accuracy:.2f}%")
        print(f"🚨 Taxa de Falso Positivo (FP) : {fp_rate:.2f}%")
        print(f"⚠️ Taxa de Falso Negativo (FN) : {fn_rate:.2f}%")
    print("=" * 85)

if __name__ == '__main__':
    main()
