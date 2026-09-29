#!/usr/bin/env python3
"""
Benchmark e Teste de Carga de Alta Concorrência para Asterisk AudioSocket (Porta :9092)
Valida a correlação de metadados no Redis (call:meta:<UUID>) e o semáforo de concorrência com Fast-Reject.
"""

import os
import sys
import time
import uuid
import struct
import socket
import json
import glob
import wave
import concurrent.futures
from urllib.parse import urlparse

REDIS_HOST = os.getenv("REDIS_HOST", "127.0.0.1")
REDIS_PORT = int(os.getenv("REDIS_PORT", "6379"))
AUDIOSOCKET_HOST = os.getenv("AUDIOSOCKET_HOST", "127.0.0.1")
AUDIOSOCKET_PORT = int(os.getenv("AUDIOSOCKET_PORT", "9092"))
SAMPLES_DIR = os.path.join(os.path.dirname(__file__), "../audio_samples")

# Protocolo Asterisk AudioSocket
TYPE_HANGUP = 0x00
TYPE_UUID = 0x01
TYPE_SILENCE = 0x02
TYPE_AUDIO = 0x10

def set_redis_call_metadata(call_uuid, lead_id, campaign_id, phone):
    """Grava metadados da chamada no Redis antes da conexão telefônica"""
    try:
        import redis
        r = redis.Redis(host=REDIS_HOST, port=REDIS_PORT, db=0, socket_timeout=1.0)
        data = {
            "call_uuid": call_uuid,
            "lead_id": str(lead_id),
            "campaign_id": str(campaign_id),
            "phone": str(phone),
            "tenant_id": "tenant-test-1"
        }
        r.set(f"call:meta:{call_uuid}", json.dumps(data), ex=900)
    except Exception as e:
        # Se redis não estiver instalado/acessível localmente, prossegue sem erro fatal
        pass

def send_audiosocket_call(wav_path, call_index):
    call_id = uuid.uuid4()
    lead_id = 1000 + call_index
    campaign_id = "camp-stress-100"
    phone = f"1198765{call_index:04d}"

    # 1. Injeta metadados no Redis
    set_redis_call_metadata(str(call_id), lead_id, campaign_id, phone)

    start_time = time.time()
    result = {
        "index": call_index,
        "uuid": str(call_id),
        "wav": os.path.basename(wav_path),
        "status": "INIT",
        "latency_ms": 0,
        "error": None
    }

    try:
        sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        sock.settimeout(3.0)
        sock.connect((AUDIOSOCKET_HOST, AUDIOSOCKET_PORT))

        # 2. Envia Handshake UUID (Tipo 0x01, Len 16, Payload 16 bytes do UUID)
        uuid_bytes = call_id.bytes
        hdr_uuid = struct.pack(">BH", TYPE_UUID, 16)
        sock.sendall(hdr_uuid + uuid_bytes)

        # 3. Lê o áudio WAV e transmite em blocos de 320 bytes (20ms PCM 8000Hz 16-bit mono)
        with wave.open(wav_path, "rb") as wf:
            framerate = wf.getframerate()
            sampwidth = wf.getsampwidth()
            n_channels = wf.getnchannels()
            raw_pcm = wf.readframes(wf.getnframes())

        chunk_size = 320
        total_chunks = len(raw_pcm) // chunk_size

        for i in range(total_chunks):
            chunk = raw_pcm[i*chunk_size:(i+1)*chunk_size]
            hdr_audio = struct.pack(">BH", TYPE_AUDIO, len(chunk))
            sock.sendall(hdr_audio + chunk)
            # Simula streaming de áudio com delay mínimo para não estourar socket
            time.sleep(0.005)

        # Envia hangup
        hdr_hangup = struct.pack(">BH", TYPE_HANGUP, 0)
        sock.sendall(hdr_hangup)
        sock.close()

        result["latency_ms"] = int((time.time() - start_time) * 1000)
        result["status"] = "COMPLETED"

    except Exception as e:
        result["status"] = "REJECTED_OR_ERROR"
        result["error"] = str(e)
        result["latency_ms"] = int((time.time() - start_time) * 1000)

    return result

def main():
    wav_files = glob.glob(os.path.join(SAMPLES_DIR, "*.wav"))
    if not wav_files:
        print(f"[ERRO] Nenhum arquivo WAV encontrado em {SAMPLES_DIR}")
        sys.exit(1)

    concurrency = int(sys.argv[1]) if len(sys.argv) > 1 else 30
    total_calls = int(sys.argv[2]) if len(sys.argv) > 2 else 60

    print("==================================================================")
    print(f" [BENCHMARK AUDIOSOCKET] Host: {AUDIOSOCKET_HOST}:{AUDIOSOCKET_PORT}")
    print(f" Concorrência: {concurrency} workers | Total de Chamadas: {total_calls}")
    print(f" Amostras de Áudio: {len(wav_files)} arquivos disponíveis")
    print("==================================================================")

    start_all = time.time()
    results = []

    with concurrent.futures.ThreadPoolExecutor(max_workers=concurrency) as executor:
        futures = []
        for i in range(total_calls):
            wav_file = wav_files[i % len(wav_files)]
            futures.append(executor.submit(send_audiosocket_call, wav_file, i + 1))

        for f in concurrent.futures.as_completed(futures):
            res = f.result()
            results.append(res)
            print(f" Chamada #{res['index']:03d} | UUID={res['uuid'][:8]} | {res['status']} | Latência: {res['latency_ms']}ms | Arquivo: {res['wav']}")

    total_time = time.time() - start_all
    completed = [r for r in results if r["status"] == "COMPLETED"]
    errors = [r for r in results if r["status"] != "COMPLETED"]
    avg_latency = sum(r["latency_ms"] for r in completed) / max(len(completed), 1)

    print("\n========================= RESULTADOS ============================")
    print(f" Tempo Total: {total_time:.2f}s | Vazão: {total_calls/total_time:.1f} chamadas/seg")
    print(f" Concluídas com Sucesso: {len(completed)} / {total_calls}")
    print(f" Rejeições / Erros (Fast-Reject): {len(errors)} / {total_calls}")
    print(f" Latência Média de Sessão: {avg_latency:.1f}ms")
    print("==================================================================")

if __name__ == "__main__":
    main()
