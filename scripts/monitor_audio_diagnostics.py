#!/usr/bin/env python3
"""
@pattern: Diagnostic / Telemetry Observer Tool
@governedBy: .agents/AGENTS.md, docs/rules/GEMINI.md
@preExecution: Access to PostgreSQL (dialer_db), Asterisk recordings directory, and LiveKit SIP logs
@postExecution: Outputs real-time call diagnostics, audio RMS levels (TX vs RX), and agent status
"""

import sys
import os
import argparse
import subprocess
import json
import time
from datetime import datetime

# ANSI Color Codes
CLR_RESET = "\033[0m"
CLR_BOLD = "\033[1m"
CLR_GREEN = "\033[32m"
CLR_YELLOW = "\033[33m"
CLR_RED = "\033[31m"
CLR_CYAN = "\033[36m"
CLR_MAGENTA = "\033[35m"
CLR_DIM = "\033[2m"

def log_debug(msg, enabled=True):
    if enabled:
        now = datetime.now().strftime("%H:%M:%S.%f")[:-3]
        print(f"{CLR_DIM}[DEBUG {now}]{CLR_RESET} {msg}")

REMOTE_COLLECTOR_CODE = r'''
import subprocess, json, wave, struct, math, os, glob, sys
from datetime import datetime

limit = int(sys.argv[1]) if len(sys.argv) > 1 else 15
tenant = sys.argv[2] if len(sys.argv) > 2 else "100"
only_answered = sys.argv[3].lower() == "true" if len(sys.argv) > 3 else True

# 1. Asterisk Live Channels & Stats
active_channels = []
live_stats = []
try:
    cmd_ch = "docker exec $(docker ps -q -f name=asterisk) asterisk -rx 'core show channels verbose' && echo '---PJSIP_STATS---' && docker exec $(docker ps -q -f name=asterisk) asterisk -rx 'pjsip show channelstats'"
    ast_out = subprocess.check_output(cmd_ch, shell=True).decode('utf-8', errors='ignore')
    parts = ast_out.split('---PJSIP_STATS---')
    ch_out = parts[0]
    active_channels = [l.strip() for l in ch_out.split('\n') if ("PJSIP/" in l or "AppDial" in l or "Dial" in l) and not l.strip().startswith("Channel")]
    
    if len(parts) > 1:
        for line in parts[1].strip().split('\n'):
            if "livekit-sip" in line or "rvx" in line or "ventitore" in line:
                live_stats.append(line.strip())
except Exception as e:
    pass

# 2. Database CDRs
cdrs = []
try:
    where_clause = f"tenant_id = '{tenant}'"
    if only_answered:
        where_clause += " AND (billsec_seconds > 0 OR disposition IN ('ANSWERED', 'DELIVERED'))"
    
    query = f"SELECT id, phone, COALESCE(lead_name, ''), COALESCE(disposition, ''), COALESCE(hangup_cause::text, ''), duration_seconds, billsec_seconds, COALESCE(agent_id, ''), COALESCE(trunk_used, ''), created_at::text, COALESCE(recording_file, '') FROM cdrs WHERE {where_clause} ORDER BY created_at DESC LIMIT {limit};"
    cmd_db = f"docker exec $(docker ps -q -f name=postgres_postgres) psql -U postgres -d dialer_db -t -A -F '|' -c \"{query}\""
    db_out = subprocess.check_output(cmd_db, shell=True).decode('utf-8', errors='ignore')
    for line in db_out.strip().split('\n'):
        if not line.strip(): continue
        cols = line.strip().split('|')
        if len(cols) >= 11:
            cdrs.append({
                "id": cols[0], "phone": cols[1], "lead_name": cols[2], "disposition": cols[3],
                "hangup_cause": cols[4], "duration": int(cols[5]) if cols[5].isdigit() else 0,
                "billsec": int(cols[6]) if cols[6].isdigit() else 0, "agent_id": cols[7],
                "trunk": cols[8], "created_at": cols[9], "recording_file": cols[10]
            })
except Exception as e:
    pass

# 3. Audio analysis dos CDRs
today = datetime.now().strftime("%Y/%m/%d")
audio_map = {}
for cdr in cdrs:
    phone = cdr["phone"]
    candidates = []
    if cdr.get("recording_file"):
        rec = cdr["recording_file"].replace("/var/spool/asterisk/monitor", "/opt/ominichat/asterisk/monitor")
        if os.path.exists(rec):
            candidates.append(rec)
    
    if not candidates:
        pattern = f"/opt/ominichat/asterisk/monitor/{today}/*-{phone}-*.wav"
        candidates = glob.glob(pattern)

    for f in candidates:
        fname = os.path.basename(f)
        if fname in audio_map:
            continue
        try:
            w = wave.open(f, "rb")
            nch = w.getnchannels()
            sw = w.getsampwidth()
            fr = w.getframerate()
            nf = w.getnframes()
            data = w.readframes(nf)
            w.close()
            if sw == 2 and nch == 2 and nf > 0:
                samples = struct.unpack('<' + str(nf * 2) + 'h', data)
                tx = samples[0::2]
                rx = samples[1::2]
                tx_rms = math.sqrt(sum(s*s for s in tx) / len(tx))
                rx_rms = math.sqrt(sum(s*s for s in rx) / len(rx))
                dur = nf / fr
                audio_map[fname] = {
                    "duration": round(dur, 1),
                    "tx_rms": round(tx_rms, 1),
                    "rx_rms": round(rx_rms, 1),
                    "tx_max": max(abs(s) for s in tx),
                    "rx_max": max(abs(s) for s in rx)
                }
        except Exception as e:
            pass

print(json.dumps({"active_channels": active_channels, "live_stats": live_stats, "cdrs": cdrs, "audio_map": audio_map}))
'''

def fetch_telemetry(limit=15, tenant="100", only_answered=True, host="84.247.135.255", is_local=False, debug=False):
    log_debug(f"Disparando coletor (Host={host}, Tenant={tenant}, Limite={limit}, ApenasAtendidas={only_answered})...", debug)
    start_time = time.time()

    ans_flag = "true" if only_answered else "false"
    if is_local:
        cmd = [sys.executable, "-", str(limit), str(tenant), ans_flag]
        proc = subprocess.run(cmd, input=REMOTE_COLLECTOR_CODE, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    else:
        cmd = [
            "sshpass", "-p", "j#uhrp7rgti3GqG",
            "ssh", "-o", "StrictHostKeyChecking=no", "-o", "ConnectTimeout=5",
            f"root@{host}", f"python3 - {limit} {tenant} {ans_flag}"
        ]
        proc = subprocess.run(cmd, input=REMOTE_COLLECTOR_CODE, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)

    elapsed = time.time() - start_time
    log_debug(f"Coletor executado em {elapsed:.2f}s com código {proc.returncode}", debug)

    if proc.returncode != 0 or not proc.stdout.strip():
        log_debug(f"Erro na execução do coletor: {proc.stderr}", debug)
        return {
            "timestamp": datetime.now().isoformat(),
            "tenant": tenant,
            "active_channels": [],
            "live_stats": [],
            "results": [],
            "error": proc.stderr
        }

    try:
        raw_data = json.loads(proc.stdout.strip())
    except Exception as e:
        log_debug(f"Falha ao decodificar JSON do coletor: {e}\nSaída recebida: {proc.stdout[:200]}", debug)
        return {
            "timestamp": datetime.now().isoformat(),
            "tenant": tenant,
            "active_channels": [],
            "live_stats": [],
            "results": [],
            "error": str(e)
        }

    cdrs = raw_data.get("cdrs", [])
    audio_map = raw_data.get("audio_map", {})
    active_channels = raw_data.get("active_channels", [])
    live_stats = raw_data.get("live_stats", [])

    log_debug(f"Processando {len(cdrs)} CDRs, {len(audio_map)} áudios analisados e {len(active_channels)} canais ativos...", debug)

    results = []
    for cdr in cdrs:
        phone = cdr["phone"]
        matched_audio = None
        for fname, metrics in audio_map.items():
            if f"-{phone}-" in fname:
                matched_audio = metrics
                break

        tx_rms = matched_audio["tx_rms"] if matched_audio else 0.0
        rx_rms = matched_audio["rx_rms"] if matched_audio else 0.0
        tx_max = matched_audio["tx_max"] if matched_audio else 0
        rx_max = matched_audio["rx_max"] if matched_audio else 0
        dur = matched_audio["duration"] if matched_audio else cdr["duration"]

        if cdr["billsec"] > 0 or cdr["disposition"] in ["ANSWERED", "DELIVERED"]:
            if tx_rms >= 100.0 and rx_rms >= 100.0:
                diagnosis = "BIDIRECTIONAL_OK"
                diag_label = f"{CLR_GREEN}✅ Áudio OK (Bidirecional){CLR_RESET}"
            elif tx_rms < 30.0 and rx_rms >= 100.0:
                diagnosis = "AGENT_MUTED"
                diag_label = f"{CLR_RED}⚠️ ATENDENTE MUDO (RX OK, TX Mudo){CLR_RESET}"
            elif tx_rms >= 100.0 and rx_rms < 30.0:
                diagnosis = "CLIENT_SILENT"
                diag_label = f"{CLR_YELLOW}ℹ️ Cliente Mudo / Silêncio{CLR_RESET}"
            elif tx_rms < 30.0 and rx_rms < 30.0:
                diagnosis = "BOTH_SILENT"
                diag_label = f"{CLR_RED}❌ Silêncio Total (Linha Muda){CLR_RESET}"
            else:
                diagnosis = "AUDIO_LOW"
                diag_label = f"{CLR_YELLOW}⚠️ Volume Baixo{CLR_RESET}"
        elif cdr["disposition"] == "FAILED":
            diagnosis = "CALL_FAILED"
            diag_label = f"{CLR_DIM}Não Atendida / Falha{CLR_RESET}"
        else:
            diagnosis = "NO_ANSWER"
            diag_label = f"{CLR_DIM}{cdr['disposition'] or 'SEM RESPOSTA'}{CLR_RESET}"

        results.append({
            "cdr": cdr,
            "audio": matched_audio,
            "tx_rms": tx_rms,
            "rx_rms": rx_rms,
            "tx_max": tx_max,
            "rx_max": rx_max,
            "duration": dur,
            "diagnosis": diagnosis,
            "diag_label": diag_label
        })

    return {
        "timestamp": datetime.now().isoformat(),
        "tenant": tenant,
        "active_channels": active_channels,
        "live_stats": live_stats,
        "results": results
    }

def print_dashboard(data, debug=False):
    print("\n" + "=" * 115)
    print(f"{CLR_BOLD}{CLR_CYAN}  TELEMETRIA & DIAGNÓSTICO DE ÁUDIO PREDITIVO - DIALER-GO / LIVEKIT SIP{CLR_RESET}")
    print(f"{CLR_DIM}  Tenant: {data['tenant']} | Horário: {datetime.now().strftime('%d/%m/%Y %H:%M:%S')} | Host: 84.247.135.255{CLR_RESET}")
    print("=" * 115)

    print(f"\n{CLR_BOLD}📡 CANAIS & PACOTES RTP AO VIVO NO ASTERISK:{CLR_RESET}")
    if data["live_stats"]:
        for st in data["live_stats"][:4]:
            print(f"  • {st}")
    elif data["active_channels"]:
        for ch in data["active_channels"][:4]:
            print(f"  • {ch}")
    else:
        print(f"  {CLR_DIM}Nenhum canal ativo no momento.{CLR_RESET}")

    print(f"\n{CLR_BOLD}📊 ÚLTIMAS {len(data['results'])} CHAMADAS ATENDIDAS / ENTREGUES (TENANT {data['tenant']}):{CLR_RESET}")
    header = f"{'Hora':<8} | {'Telefone':<12} | {'Atendente (ID)':<16} | {'Dur/Bill':<9} | {'TX (Atendente)':<16} | {'RX (Cliente)':<16} | {'Diagnóstico'}"
    print("-" * 115)
    print(f"{CLR_BOLD}{header}{CLR_RESET}")
    print("-" * 115)

    muted_agents = set()

    for item in data["results"]:
        cdr = item["cdr"]
        try:
            created_dt = datetime.fromisoformat(cdr["created_at"])
            time_str = created_dt.strftime("%H:%M:%S")
        except:
            time_str = cdr["created_at"][:8]

        agent_display = cdr["agent_id"][:14] + ".." if len(cdr["agent_id"]) > 14 else (cdr["agent_id"] or "-")
        dur_display = f"{item['duration']:.0f}s/{cdr['billsec']}s"
        tx_display = f"RMS={item['tx_rms']:<5.1f} (P={item['tx_max']})"
        rx_display = f"RMS={item['rx_rms']:<5.1f} (P={item['rx_max']})"

        if item["diagnosis"] == "AGENT_MUTED" and cdr["agent_id"]:
            muted_agents.add(cdr["agent_id"])

        print(f"{time_str:<8} | {cdr['phone']:<12} | {agent_display:<16} | {dur_display:<9} | {tx_display:<16} | {rx_display:<16} | {item['diag_label']}")

    print("-" * 115)

    if muted_agents:
        print(f"\n{CLR_BOLD}{CLR_RED}🚨 ALERTA DE ATENDENTES COM MICROFONE MUDO / DESCONFIGURADO:{CLR_RESET}")
        for ag in muted_agents:
            print(f"  ⚠️  Atendente ID: {CLR_YELLOW}{ag}{CLR_RESET} -> O cliente falou (RX OK), mas o áudio do operador enviou silêncio (RMS < 30)!")
        print(f"  {CLR_BOLD}Ação Imediata:{CLR_RESET} Orientar o operador a verificar botão de Mute no headset, permissões de microfone no navegador ou dispositivo padrão de áudio no painel.")
    else:
        print(f"\n{CLR_GREEN}✨ Todos os operadores analisados estão com áudio bidirecional ativo e operante.{CLR_RESET}")

def main():
    parser = argparse.ArgumentParser(description="Monitoramento e Diagnóstico de Áudio Preditivo (Dialer-Go / LiveKit SIP)")
    parser.add_argument("-n", "--limit", type=int, default=15, help="Quantidade de chamadas recentes para analisar (padrão: 15)")
    parser.add_argument("-t", "--tenant", type=str, default="100", help="ID do Tenant a filtrar (padrão: 100)")
    parser.add_argument("--all", action="store_true", help="Incluir todas as tentativas (inclusive discagens não atendidas/falhas)")
    parser.add_argument("--host", type=str, default="84.247.135.255", help="Host SSH do servidor (padrão: 84.247.135.255)")
    parser.add_argument("--local", action="store_true", help="Executar localmente caso esteja dentro do próprio host")
    parser.add_argument("-d", "--debug", action="store_true", help="Ativar modo debug detalhado com log passo a passo")
    parser.add_argument("-w", "--watch", type=int, default=0, help="Intervalo em segundos para repetição contínua (0 = uma execução)")

    args = parser.parse_args()
    only_answered = not args.all

    if args.watch > 0:
        print(f"{CLR_CYAN}Iniciando modo monitoramento contínuo a cada {args.watch}s (Pressione Ctrl+C para sair)...{CLR_RESET}")
        while True:
            try:
                data = fetch_telemetry(limit=args.limit, tenant=args.tenant, only_answered=only_answered, host=args.host, is_local=args.local, debug=args.debug)
                print("\033[H\033[J", end="") # Limpar tela
                print_dashboard(data, debug=args.debug)
                time.sleep(args.watch)
            except KeyboardInterrupt:
                print("\nEncerrando monitoramento.")
                sys.exit(0)
    else:
        data = fetch_telemetry(limit=args.limit, tenant=args.tenant, only_answered=only_answered, host=args.host, is_local=args.local, debug=args.debug)
        print_dashboard(data, debug=args.debug)

if __name__ == "__main__":
    main()
