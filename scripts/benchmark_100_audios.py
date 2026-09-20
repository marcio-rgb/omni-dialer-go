import os
import glob
import json
import time
import wave
import asyncio
import numpy as np
import scipy.signal
import requests
import websockets

AUDIO_DIR = os.path.join(os.path.dirname(__file__), "../temp/benchmark_100/sample_100")
OUTPUT_JSON = os.path.join(os.path.dirname(__file__), "../temp/benchmark_100/results_100.json")
OUTPUT_CSV = os.path.join(os.path.dirname(__file__), "../temp/benchmark_100/results_100.csv")

VOSK_WS_URL = "ws://localhost:2700"
WHISPER_URL = "http://localhost:8090/v1/audio/transcriptions"

def analyze_acoustic_dsp(wav_path):
    """
    Realiza análise acústica profunda do áudio WAV:
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

    total_duration_sec = n_frames / float(framerate)
    
    # Converte para int16 numpy array
    if sampwidth == 2:
        audio = np.frombuffer(raw_data, dtype=np.int16).astype(np.float32)
    elif sampwidth == 1:
        audio = (np.frombuffer(raw_data, dtype=np.uint8).astype(np.float32) - 128) * 256
    else:
        audio = np.frombuffer(raw_data, dtype=np.int16).astype(np.float32)

    # Se estéreo, analisa o canal RX (canal 1 se intercalado L/R) ou faz a média
    if n_channels == 2:
        audio = audio[1::2]  # RX: Voz do Cliente / Operadora

    if len(audio) == 0:
        return {
            "duration_sec": 0, "initial_silence_ms": 0, "greeting_ms": 0,
            "after_greeting_silence_ms": 0, "beep_detected": False, "beep_freq_hz": 0
        }

    # Frame-based Energy (janelas de 20ms)
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
    silence_threshold = max(250.0, max_rms * 0.12)  # Limiar de ruído/voz

    # 1. Silêncio inicial (ms)
    is_speech = frames_rms > silence_threshold
    first_speech_idx = np.where(is_speech)[0]
    
    if len(first_speech_idx) == 0:
        initial_silence_ms = int(total_duration_sec * 1000)
        greeting_ms = 0
        after_greeting_silence_ms = 0
    else:
        initial_silence_ms = int(first_speech_idx[0] * 10)  # Cada hop = 10ms
        
        # 2. Duração do primeiro bloco contínuo de fala (greeting)
        # Permite pausas curtas de até 200ms dentro da mesma fala
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
        
        # 3. Silêncio após o primeiro bloco de fala
        if len(speech_segments) > 1:
            after_greeting_silence_ms = int((speech_segments[1][0] - first_burst[1]) * 10)
        else:
            remaining_frames = len(frames_rms) - first_burst[1]
            after_greeting_silence_ms = int(max(0, remaining_frames * 10))

    # 4. Detecção de Beep / Tom puro de secretária eletrônica (FFT entre 400Hz e 1200Hz)
    beep_detected = False
    beep_freq_hz = 0
    
    # Analisa espectrograma para detectar picos tonais concentrados
    if len(audio) > framerate * 0.5:
        f, t, Sxx = scipy.signal.spectrogram(audio, framerate, nperseg=int(framerate*0.05))
        # Filtra frequências entre 400 e 1200 Hz
        band_idx = np.where((f >= 400) & (f <= 1200))[0]
        if len(band_idx) > 0:
            band_power = Sxx[band_idx, :]
            total_power = np.sum(Sxx, axis=0) + 1e-6
            tonality = np.max(band_power, axis=0) / total_power
            # Se a tonalidade de uma frequência pura na faixa for > 0.65 por pelo menos 100ms
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
    """Transcreve com Vosk Kaldi WebSocket e mede tempo de resposta."""
    start_t = time.time()
    text = ""
    try:
        with wave.open(wav_path, "rb") as wf:
            sampwidth = wf.getsampwidth()
            framerate = wf.getframerate()
            raw_data = wf.readframes(wf.getnframes())

        # Se estéreo, extrai canal RX
        if wf.getnchannels() == 2:
            arr = np.frombuffer(raw_data, dtype=np.int16)[1::2]
            raw_data = arr.tobytes()

        async with websockets.connect(VOSK_WS_URL, open_timeout=3.0) as ws:
            # Envia config
            await ws.send(json.dumps({"config": {"sample_rate": framerate}}))
            
            # Envia áudio em blocos
            chunk_size = 4000
            for i in range(0, len(raw_data), chunk_size):
                await ws.send(raw_data[i : i + chunk_size])
                await asyncio.sleep(0.001)

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

def transcribe_whisper(wav_path):
    """Transcreve com Faster-Whisper (Ground Truth)."""
    start_t = time.time()
    text = ""
    try:
        with open(wav_path, "rb") as f:
            files = {"file": (os.path.basename(wav_path), f, "audio/wav")}
            data = {"language": "pt", "temperature": "0.0"}
            r = requests.post(WHISPER_URL, files=files, data=data, timeout=10.0)
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
    """Classifica se o áudio é Humano ou Máquina com base no texto real do Whisper e métricas acústicas."""
    txt = whisper_text.lower()
    
    machine_keywords = [
        "recado", "caixa postal", "após o sinal", "chamada está sendo encaminhada",
        "mensagem", "deixe sua mensagem", "não pode atender", "temporariamente indisponível",
        "número que você ligou", "programado para não receber", "créditos", "recarga",
        "vivo", "claro", "tim", "oi", "atendente", "secretária", "bip", "grave seu recado"
    ]
    
    human_keywords = [
        "alô", "alo", "oi", "olá", "ola", "fala", "quem fala", "quem é", "boa tarde",
        "bom dia", "boa noite", "pois não", "pois nao", "opa", "sim", "estou ouvindo"
    ]
    
    for kw in machine_keywords:
        if kw in txt:
            return "MACHINE"

    if dsp["beep_detected"]:
        return "MACHINE"

    # Se a fala contínua for muito longa (> 2200ms) sem pausa, grande probabilidade de ser gravação
    if dsp["greeting_ms"] > 2200 and len(txt) > 25:
        return "MACHINE"

    for kw in human_keywords:
        if kw in txt:
            return "HUMAN"

    if dsp["greeting_ms"] > 0 and dsp["greeting_ms"] <= 1200:
        return "HUMAN"

    return "UNCERTAIN"

async def process_all():
    print("🚀 === INICIANDO BENCHMARK DE 100 ÁUDIOS REAIS (VOSK vs WHISPER vs DSP) ===")
    wav_files = sorted(glob.glob(os.path.join(AUDIO_DIR, "*.wav")))[:100]
    
    if not wav_files:
        print(f"❌ Nenhum arquivo WAV encontrado em: {AUDIO_DIR}")
        return

    print(f"📁 Processando {len(wav_files)} arquivos de áudio...")
    
    results = []
    
    for idx, wav_path in enumerate(wav_files, 1):
        filename = os.path.basename(wav_path)
        
        # 1. DSP Acústico
        dsp = analyze_acoustic_dsp(wav_path)
        
        # 2. Vosk
        vosk_text, vosk_ms = await transcribe_vosk(wav_path)
        
        # 3. Whisper
        whisper_text, whisper_ms = transcribe_whisper(wav_path)
        
        # 4. Classificação de Verdade
        category = classify_ground_truth(whisper_text, dsp)
        
        item = {
            "id": idx,
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
        results.append(item)
        
        if idx % 10 == 0 or idx == len(wav_files):
            print(f"  [{idx:3d}/{len(wav_files)}] {filename[:25]}... -> Cat: {category:<8} | Greet: {dsp['greeting_ms']:4d}ms | Vosk: {vosk_ms}ms | Whisper: {whisper_ms}ms")

    # Salva JSON
    with open(OUTPUT_JSON, "w", encoding="utf-8") as f:
        json.dump(results, f, indent=2, ensure_ascii=False)
    print(f"\n✅ Relatório JSON salvo em: {OUTPUT_JSON}")

    # Salva CSV
    with open(OUTPUT_CSV, "w", encoding="utf-8") as f:
        f.write("id,filename,category,duration_sec,initial_silence_ms,greeting_ms,after_greeting_silence_ms,beep_detected,beep_freq_hz,vosk_latency_ms,whisper_latency_ms,vosk_text,whisper_text\n")
        for r in results:
            v_t = r["vosk_text"].replace('"', '""').replace('\n', ' ')
            w_t = r["whisper_text"].replace('"', '""').replace('\n', ' ')
            f.write(f'{r["id"]},"{r["filename"]}",{r["category"]},{r["duration_sec"]},{r["initial_silence_ms"]},{r["greeting_ms"]},{r["after_greeting_silence_ms"]},{r["beep_detected"]},{r["beep_freq_hz"]},{r["vosk_latency_ms"]},{r["whisper_latency_ms"]},"{v_t}","{w_t}"\n')
    print(f"✅ Relatório CSV salvo em: {OUTPUT_CSV}")

    # --- ANÁLISE ESTATÍSTICA ---
    machines = [r for r in results if r["category"] == "MACHINE"]
    humans = [r for r in results if r["category"] == "HUMAN"]
    uncertain = [r for r in results if r["category"] == "UNCERTAIN"]

    print("\n" + "="*70)
    print(f"📊 RESUMO ESTATÍSTICO DO BENCHMARK ({len(results)} ÁUDIOS)")
    print("="*70)
    print(f"🔹 Total Máquinas / Caixas Postais: {len(machines)} ({len(machines)/len(results)*100:.1f}%)")
    print(f"🔹 Total Atendimentos Humanos:       {len(humans)} ({len(humans)/len(results)*100:.1f}%)")
    print(f"🔹 Total Indeterminados / Silêncio:   {len(uncertain)} ({len(uncertain)/len(results)*100:.1f}%)")
    print("-" * 70)

    if machines:
        m_greet = [m["greeting_ms"] for m in machines if m["greeting_ms"] > 0]
        m_sil = [m["initial_silence_ms"] for m in machines]
        m_beeps = sum(1 for m in machines if m["beep_detected"])
        print(f"📌 PADRÕES DE CAIXA POSTAL / MÁQUINA:")
        print(f"   - Saudação Inicial Contínua Média: {np.mean(m_greet):.1f}ms (Mediana: {np.median(m_greet):.1f}ms | Mín: {np.min(m_greet) if m_greet else 0}ms | Máx: {np.max(m_greet) if m_greet else 0}ms)")
        print(f"   - Silêncio Inicial Médio:         {np.mean(m_sil):.1f}ms")
        print(f"   - Bips de Correio de Voz Detectados: {m_beeps}/{len(machines)}")

    if humans:
        h_greet = [h["greeting_ms"] for h in humans if h["greeting_ms"] > 0]
        h_sil = [h["initial_silence_ms"] for h in humans]
        h_after = [h["after_greeting_silence_ms"] for h in humans]
        print(f"\n📌 PADRÕES DE ATENDIMENTO HUMANO:")
        print(f"   - Saudação 'Alô' Média:            {np.mean(h_greet):.1f}ms (Mediana: {np.median(h_greet):.1f}ms | Mín: {np.min(h_greet) if h_greet else 0}ms | Máx: {np.max(h_greet) if h_greet else 0}ms)")
        print(f"   - Silêncio Inicial Médio:         {np.mean(h_sil):.1f}ms")
        print(f"   - Pausa Pós-Alô (Espera Resposta): {np.mean(h_after):.1f}ms")

    # Comparativo Vosk vs Whisper
    vosk_times = [r["vosk_latency_ms"] for r in results]
    whisper_times = [r["whisper_latency_ms"] for r in results]
    print(f"\n⚡ COMPARATIVO DE LATÊNCIA:")
    print(f"   - Vosk Local (WebSocket):       Média: {np.mean(vosk_times):.1f}ms | Mediana: {np.median(vosk_times):.1f}ms")
    print(f"   - Faster-Whisper (HTTP Server): Média: {np.mean(whisper_times):.1f}ms | Mediana: {np.median(whisper_times):.1f}ms")
    print("="*70)

if __name__ == "__main__":
    asyncio.run(process_all())
