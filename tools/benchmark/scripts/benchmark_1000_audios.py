import os
import glob
import json
import time
import wave
import asyncio
import gc
import subprocess
import numpy as np
import scipy.signal
import requests
import websockets

AUDIO_DIR = os.path.join(os.path.dirname(__file__), "../temp/benchmark_1000/sample_1000")
OUTPUT_JSON = os.path.join(os.path.dirname(__file__), "../temp/benchmark_1000/results_1000.json")
OUTPUT_CSV = os.path.join(os.path.dirname(__file__), "../temp/benchmark_1000/results_1000.csv")

VOSK_WS_URL = "ws://localhost:2700"
WHISPER_URL = "http://localhost:8090/v1/audio/transcriptions"
CONCURRENCY = 6
BATCH_SIZE = 30

def analyze_acoustic_dsp(wav_path):
    """
    Realiza análise acústica do áudio WAV:
    - Duração total
    - Silêncio inicial (ms)
    - Duração da saudação inicial / fala contínua (ms)
    - Silêncio pós-saudação (ms)
    - Detecção de Beep / Tom de correio de voz (FFT)
    """
    with wave.open(wav_path, "rb") as wf:
        n_channels = wf.getnchannels()
        sampwidth = wf.getsampwidth()
        framerate = wf.getframerate()
        n_frames = wf.getnframes()
        raw_data = wf.readframes(n_frames)

    total_duration_sec = n_frames / float(framerate) if framerate > 0 else 0.0
    
    if sampwidth == 2:
        audio = np.frombuffer(raw_data, dtype=np.int16).astype(np.float32)
    elif sampwidth == 1:
        audio = (np.frombuffer(raw_data, dtype=np.uint8).astype(np.float32) - 128) * 256
    else:
        audio = np.frombuffer(raw_data, dtype=np.int16).astype(np.float32)

    if n_channels == 2:
        audio = audio[1::2]  # RX: Voz do Cliente / Operadora

    if len(audio) == 0:
        return {
            "duration_sec": 0.0, "initial_silence_ms": 0, "greeting_ms": 0,
            "after_greeting_silence_ms": 0, "beep_detected": False, "beep_freq_hz": 0
        }

    frame_len = int(framerate * 0.02)  # 20ms
    hop_len = int(framerate * 0.01)    # 10ms
    
    num_frames = (len(audio) - frame_len) // hop_len
    if num_frames <= 0:
        num_frames = 1
        
    frames_rms = []
    for i in range(num_frames):
        chunk = audio[i * hop_len : i * hop_len + frame_len]
        rms = np.sqrt(np.mean(chunk**2)) if len(chunk) > 0 else 0
        frames_rms.append(rms)

    frames_rms = np.array(frames_rms)
    max_rms = np.max(frames_rms) if len(frames_rms) > 0 else 1.0
    silence_threshold = max(250.0, max_rms * 0.12)

    is_speech = frames_rms > silence_threshold
    first_speech_idx = np.where(is_speech)[0]
    
    if len(first_speech_idx) == 0:
        initial_silence_ms = int(total_duration_sec * 1000)
        greeting_ms = 0
        after_greeting_silence_ms = 0
    else:
        initial_silence_ms = int(first_speech_idx[0] * 10)
        
        speech_segments = []
        current_start = first_speech_idx[0]
        current_end = first_speech_idx[0]
        
        for idx in first_speech_idx[1:]:
            if idx - current_end <= 20:  # <= 200ms de pausa
                current_end = idx
            else:
                speech_segments.append((current_start, current_end))
                current_start = idx
                current_end = idx
        speech_segments.append((current_start, current_end))
        
        first_burst = speech_segments[0]
        greeting_ms = int((first_burst[1] - first_burst[0]) * 10)
        
        if len(speech_segments) > 1:
            after_greeting_silence_ms = int((speech_segments[1][0] - first_burst[1]) * 10)
        else:
            remaining_frames = len(frames_rms) - first_burst[1]
            after_greeting_silence_ms = int(max(0, remaining_frames * 10))

    beep_detected = False
    beep_freq_hz = 0
    
    if len(audio) > framerate * 0.5:
        f, t, Sxx = scipy.signal.spectrogram(audio, framerate, nperseg=int(framerate*0.05))
        band_idx = np.where((f >= 400) & (f <= 1200))[0]
        if len(band_idx) > 0:
            band_power = Sxx[band_idx, :]
            total_power = np.sum(Sxx, axis=0) + 1e-6
            tonality = np.max(band_power, axis=0) / total_power
            consecutive_tonal = 0
            for ton in tonality:
                if ton > 0.65:
                    consecutive_tonal += 1
                    if consecutive_tonal >= 4:  # ~150ms
                        beep_detected = True
                        peak_f_idx = band_idx[np.argmax(np.mean(band_power, axis=1))]
                        beep_freq_hz = int(f[peak_f_idx])
                        break
                else:
                    consecutive_tonal = 0

    return {
        "duration_sec": round(total_duration_sec, 2),
        "initial_silence_ms": initial_silence_ms,
        "greeting_ms": greeting_ms,
        "after_greeting_silence_ms": after_greeting_silence_ms,
        "beep_detected": beep_detected,
        "beep_freq_hz": beep_freq_hz
    }

async def transcribe_vosk(wav_path):
    """Transcreve com Vosk Kaldi WebSocket."""
    start_t = time.time()
    text = ""
    try:
        with wave.open(wav_path, "rb") as wf:
            sampwidth = wf.getsampwidth()
            framerate = wf.getframerate()
            raw_data = wf.readframes(wf.getnframes())

        if wf.getnchannels() == 2:
            arr = np.frombuffer(raw_data, dtype=np.int16)[1::2]
            raw_data = arr.tobytes()

        async with websockets.connect(VOSK_WS_URL, open_timeout=4.0) as ws:
            await ws.send(json.dumps({"config": {"sample_rate": framerate}}))
            
            chunk_size = 4000
            for i in range(0, len(raw_data), chunk_size):
                await ws.send(raw_data[i : i + chunk_size])
                await asyncio.sleep(0.0002)

            await ws.send('{"eof" : 1}')
            
            while True:
                msg = await ws.recv()
                res = json.loads(msg)
                if "text" in res:
                    text = res["text"]
                    break
    except Exception as e:
        text = f"[VOSK_ERROR: {str(e)}]"

    elapsed_ms = int((time.time() - start_t) * 1000)
    return text.strip(), elapsed_ms

def transcribe_whisper_sync(wav_path):
    """Transcreve síncrono com Faster-Whisper."""
    start_t = time.time()
    text = ""
    try:
        with open(wav_path, "rb") as f:
            files = {"file": (os.path.basename(wav_path), f, "audio/wav")}
            data = {"language": "pt", "temperature": "0.0"}
            r = requests.post(WHISPER_URL, files=files, data=data, timeout=15.0)
            if r.status_code == 200:
                res = r.json()
                text = res.get("text", "")
            else:
                text = f"[WHISPER_HTTP_{r.status_code}]"
    except Exception as e:
        text = f"[WHISPER_ERROR: {str(e)}]"

    elapsed_ms = int((time.time() - start_t) * 1000)
    return text.strip(), elapsed_ms

def classify_ground_truth(whisper_text, dsp):
    """Classifica se o áudio é Humano ou Máquina com base no texto e acústica."""
    txt = whisper_text.lower()
    
    machine_keywords = [
        "recado", "caixa postal", "após o sinal", "chamada está sendo encaminhada",
        "mensagem", "deixe sua mensagem", "não pode atender", "temporariamente indisponível",
        "número que você ligou", "programado para não receber", "créditos", "recarga",
        "vivo", "claro", "tim", "oi", "atendente", "secretária", "bip", "grave seu recado",
        "desligado", "fora da área", "caixa de mensagens", "encontra-se", "não foi possível completar",
        "sua ligação", "todos os nossos atendentes", "chamada encaminhada", "ocupado"
    ]
    
    human_keywords = [
        "alô", "alo", "oi", "olá", "ola", "fala", "quem fala", "quem é", "boa tarde",
        "bom dia", "boa noite", "pois não", "pois nao", "opa", "sim", "estou ouvindo",
        "pronto", "fale", "pode falar", "com quem falo"
    ]
    
    for kw in machine_keywords:
        if kw in txt:
            return "MACHINE"

    if dsp["beep_detected"]:
        return "MACHINE"

    if dsp["greeting_ms"] > 2200 and len(txt) > 25:
        return "MACHINE"

    for kw in human_keywords:
        if kw in txt:
            return "HUMAN"

    if dsp["greeting_ms"] > 0 and dsp["greeting_ms"] <= 1200:
        return "HUMAN"

    return "UNCERTAIN"

def insert_batch_to_postgres(items):
    """Insere lote de itens no PostgreSQL local dialer_db."""
    if not items:
        return
    sql_lines = []
    for r in items:
        b_id = r["id"]
        filename = r["filename"].replace("'", "''")
        category = r["category"].replace("'", "''")
        dur = r["duration_sec"]
        init_sil = r["initial_silence_ms"]
        greet = r["greeting_ms"]
        after_sil = r["after_greeting_silence_ms"]
        beep = 'TRUE' if r["beep_detected"] else 'FALSE'
        freq = r["beep_freq_hz"]
        vosk_ms = r["vosk_latency_ms"]
        whisper_ms = r["whisper_latency_ms"]
        vosk_txt = r["vosk_text"].replace("'", "''")
        whisper_txt = r["whisper_text"].replace("'", "''")
        
        sql_lines.append(f"""
        INSERT INTO benchmark_results (
            id, filename, category, duration_sec, initial_silence_ms, greeting_ms,
            after_greeting_silence_ms, beep_detected, beep_freq_hz,
            vosk_latency_ms, whisper_latency_ms, vosk_text, whisper_text
        ) VALUES (
            {b_id}, '{filename}', '{category}', {dur}, {init_sil}, {greet},
            {after_sil}, {beep}, {freq},
            {vosk_ms}, {whisper_ms}, '{vosk_txt}', '{whisper_txt}'
        ) ON CONFLICT (id) DO UPDATE SET
            filename = EXCLUDED.filename,
            category = EXCLUDED.category,
            duration_sec = EXCLUDED.duration_sec,
            initial_silence_ms = EXCLUDED.initial_silence_ms,
            greeting_ms = EXCLUDED.greeting_ms,
            after_greeting_silence_ms = EXCLUDED.after_greeting_silence_ms,
            beep_detected = EXCLUDED.beep_detected,
            beep_freq_hz = EXCLUDED.beep_freq_hz,
            vosk_latency_ms = EXCLUDED.vosk_latency_ms,
            whisper_latency_ms = EXCLUDED.whisper_latency_ms,
            vosk_text = EXCLUDED.vosk_text,
            whisper_text = EXCLUDED.whisper_text;
        """)
    
    full_sql = "\n".join(sql_lines)
    cmd = ["docker", "exec", "-i", "postgres-postgres-1", "psql", "-U", "postgres", "-d", "dialer_db"]
    proc = subprocess.Popen(cmd, stdin=subprocess.PIPE, stdout=subprocess.DEVNULL, stderr=subprocess.PIPE)
    proc.communicate(input=full_sql.encode('utf-8'))

async def process_file(wav_path, current_id, loop):
    filename = os.path.basename(wav_path)
    
    # 1. DSP Acústico
    dsp = analyze_acoustic_dsp(wav_path)
    
    # 2. Vosk
    vosk_text, vosk_ms = await transcribe_vosk(wav_path)
    
    # 3. Whisper (no executor)
    whisper_text, whisper_ms = await loop.run_in_executor(None, transcribe_whisper_sync, wav_path)
    
    # 4. Classificação
    category = classify_ground_truth(whisper_text, dsp)
    
    return {
        "id": current_id,
        "filename": filename,
        "category": category,
        "duration_sec": dsp["duration_sec"],
        "initial_silence_ms": dsp["initial_silence_ms"],
        "greeting_ms": dsp["greeting_ms"],
        "after_greeting_silence_ms": dsp["after_greeting_silence_ms"],
        "beep_detected": dsp["beep_detected"],
        "beep_freq_hz": dsp["beep_freq_hz"],
        "vosk_text": vosk_text,
        "vosk_latency_ms": vosk_ms,
        "whisper_text": whisper_text,
        "whisper_latency_ms": whisper_ms
    }

async def process_all():
    print("🚀 === INICIANDO BENCHMARK DE 1000 ÁUDIOS REAIS (CONCORRÊNCIA EM LOTES) ===", flush=True)
    wav_files = sorted(glob.glob(os.path.join(AUDIO_DIR, "*.wav")))
    
    if not wav_files:
        print(f"❌ Nenhum arquivo WAV encontrado em: {AUDIO_DIR}", flush=True)
        return

    print(f"📁 Total de arquivos para processar: {len(wav_files)}", flush=True)
    
    # Busca o maior id existente no banco para continuar a numeração (a partir de 101)
    res = subprocess.check_output("docker exec -i postgres-postgres-1 psql -U postgres -d dialer_db -t -c 'SELECT COALESCE(MAX(id), 0) FROM benchmark_results;'", shell=True)
    max_id = int(res.decode().strip())
    start_id = max(101, max_id + 1)
    print(f"🔢 Numeração dos novos IDs iniciando em: {start_id}", flush=True)
    
    # Carrega existentes se houver
    results = []
    processed_filenames = set()
    if os.path.isfile(OUTPUT_JSON):
        try:
            with open(OUTPUT_JSON, "r", encoding="utf-8") as f:
                results = json.load(f)
                processed_filenames = {r["filename"] for r in results}
                print(f"🔄 Retomando execução anterior: {len(results)} já processados.", flush=True)
        except Exception:
            results = []

    remaining_files = [f for f in wav_files if os.path.basename(f) not in processed_filenames]
    print(f"⏳ Arquivos restantes para processar: {len(remaining_files)}", flush=True)

    sem = asyncio.Semaphore(CONCURRENCY)
    loop = asyncio.get_event_loop()
    
    start_benchmark_t = time.time()
    
    # Processa em blocos de BATCH_SIZE
    for chunk_start in range(0, len(remaining_files), BATCH_SIZE):
        chunk = remaining_files[chunk_start : chunk_start + BATCH_SIZE]
        
        async def bounded_worker(file_path, file_id):
            async with sem:
                return await process_file(file_path, file_id, loop)
        
        chunk_tasks = []
        for i, f in enumerate(chunk):
            fid = start_id + len(results) + i
            chunk_tasks.append(bounded_worker(f, fid))
            
        chunk_results = await asyncio.gather(*chunk_tasks)
        results.extend(chunk_results)
        
        # Persiste lote no PostgreSQL
        insert_batch_to_postgres(chunk_results)
        
        # Atualiza JSON
        with open(OUTPUT_JSON, "w", encoding="utf-8") as f:
            json.dump(results, f, indent=2, ensure_ascii=False)
            
        # Atualiza CSV
        with open(OUTPUT_CSV, "w", encoding="utf-8") as f:
            f.write("id,filename,category,duration_sec,initial_silence_ms,greeting_ms,after_greeting_silence_ms,beep_detected,beep_freq_hz,vosk_latency_ms,whisper_latency_ms,vosk_text,whisper_text\n")
            for r in results:
                v_t = r["vosk_text"].replace('"', '""').replace('\n', ' ')
                w_t = r["whisper_text"].replace('"', '""').replace('\n', ' ')
                f.write(f'{r["id"]},"{r["filename"]}",{r["category"]},{r["duration_sec"]},{r["initial_silence_ms"]},{r["greeting_ms"]},{r["after_greeting_silence_ms"]},{r["beep_detected"]},{r["beep_freq_hz"]},{r["vosk_latency_ms"]},{r["whisper_latency_ms"]},"{v_t}","{w_t}"\n')

        elapsed = time.time() - start_benchmark_t
        rate = len(results) / elapsed if elapsed > 0 else 1
        total_all = len(wav_files)
        rem_sec = (total_all - len(results)) / (len(results) / elapsed) if len(results) > 0 else 0
        
        last_item = chunk_results[-1]
        print(f"  [{len(results):4d}/{total_all}] Concluído lote de {len(chunk_results)} | Último: {last_item['filename'][:25]} -> {last_item['category']:<8} | ETA: {rem_sec/60:.1f}min", flush=True)
        
        gc.collect()

    print(f"\n✅ Concluído! {len(results)} registros processados e inseridos no PostgreSQL dialer_db.", flush=True)

if __name__ == "__main__":
    asyncio.run(process_all())
