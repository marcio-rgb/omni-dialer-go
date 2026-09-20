#!/usr/bin/env python3
import sys
import os
import json
import random
import sqlite3
import datetime

# 1. Le cabeçalhos AGI do Asterisk (terminam com linha vazia)
agi_env = {}
while True:
    line = sys.stdin.readline().strip()
    if not line:
        break
    if ":" in line:
        k, v = line.split(":", 1)
        agi_env[k.strip()] = v.strip()

phone = sys.argv[1] if len(sys.argv) > 1 else agi_env.get("agi_extension", "UNKNOWN")
uniqueid = sys.argv[2] if len(sys.argv) > 2 else agi_env.get("agi_uniqueid", "UNKNOWN")

# 2. Carrega catalogo de 30 audios
catalog_path = "/var/lib/asterisk/sounds/benchmark/catalog.json"
try:
    with open(catalog_path, "r", encoding="utf-8") as f:
        catalog = json.load(f)
except Exception:
    catalog = [{"id": 1, "name": "sim_01_human_alo", "type": "HUMAN"}]

# 3. Sorteia aleatoriamente um dos 30 audios
chosen = random.choice(catalog)
audio_id = chosen["id"]
audio_name = chosen["name"]
expected_type = chosen["type"]
now_iso = datetime.datetime.now(datetime.timezone.utc).isoformat()

# 4. Registra no SQLite persistido (/var/log/asterisk/simulator_played.db)
db_path = "/var/log/asterisk/simulator_played.db"
try:
    conn = sqlite3.connect(db_path, timeout=5.0)
    c = conn.cursor()
    c.execute("""
        CREATE TABLE IF NOT EXISTS simulator_played_log (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            phone TEXT NOT NULL,
            uniqueid TEXT,
            audio_id INTEGER,
            audio_name TEXT,
            expected_type TEXT,
            played_at TEXT
        )
    """)
    c.execute("""
        INSERT INTO simulator_played_log (phone, uniqueid, audio_id, audio_name, expected_type, played_at)
        VALUES (?, ?, ?, ?, ?, ?)
    """, (phone, uniqueid, audio_id, audio_name, expected_type, now_iso))
    conn.commit()
    conn.close()
except Exception as e:
    sys.stderr.write(f"Erro SQLite: {e}\n")

# 5. Registra em arquivo append JSON Lines redundante
log_path = "/var/log/asterisk/simulator_played.jsonl"
try:
    with open(log_path, "a", encoding="utf-8") as lf:
        lf.write(json.dumps({
            "phone": phone,
            "uniqueid": uniqueid,
            "audio_id": audio_id,
            "audio_name": audio_name,
            "expected_type": expected_type,
            "played_at": now_iso
        }) + "\n")
except Exception as e:
    sys.stderr.write(f"Erro log: {e}\n")

# 6. Devolve comandos AGI para o Asterisk
sys.stdout.write(f'SET VARIABLE AUDIO_TO_PLAY "/var/lib/asterisk/sounds/benchmark/{audio_name}"\n')
sys.stdout.flush()
_ = sys.stdin.readline()

sys.stdout.write(f'SET VARIABLE AUDIO_TYPE "{expected_type}"\n')
sys.stdout.flush()
_ = sys.stdin.readline()

sys.stdout.write(f'SET VARIABLE AUDIO_NAME "{audio_name}"\n')
sys.stdout.flush()
_ = sys.stdin.readline()

sys.stdout.write(f'VERBOSE "[SIMULATOR-BENCHMARK] Phone: {phone} | Sorteado: {audio_name} ({expected_type})" 1\n')
sys.stdout.flush()
_ = sys.stdin.readline()

sys.exit(0)
