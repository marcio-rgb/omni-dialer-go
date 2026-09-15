package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

/**
 * Orquestrador do Plano de Teste de Stress & Benchmark AMD (40 Canais).
 *
 * @pattern Orchestrator / Command Runner
 * @governedBy .agents/ARCHITECT.md
 */

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	switch cmd {
	case "prepare-audio":
		runPrepareAudio(os.Args[2:])
	case "transcribe-batch":
		runTranscribeBatch(os.Args[2:])
	case "evaluate-legacy":
		runEvaluateLegacy(os.Args[2:])
	default:
		fmt.Printf("Comando desconhecido: %s\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Uso: stress-benchmark <comando> [opções]")
	fmt.Println("Comandos disponíveis:")
	fmt.Println("  prepare-audio      Segrega canais (TX/RX) e indexa áudios do cliente")
	fmt.Println("  transcribe-batch   Transcreve áudios com Vosk STT e salva no PostgreSQL")
	fmt.Println("  evaluate-legacy    Gera Matriz de Confusão do sistema legado de 14/09")
}

func runPrepareAudio(args []string) {
	fs := flag.NewFlagSet("prepare-audio", flag.ExitOnError)
	inputDir := fs.String("input-dir", "/opt/ominichat/asterisk/monitor/2026/09/14", "Diretório de áudios de entrada")
	outputDir := fs.String("output-dir", "/opt/ominichat/asterisk/monitor/benchmark_saneado", "Diretório raiz para áudios saneados")
	dbURL := fs.String("db-url", "postgresql://postgres:ojr6DNM25InIg3qoYI4nbUL5Cx04iCiCn1122@localhost:5432/dialer_db?sslmode=disable", "URL PostgreSQL")
	trimMs := fs.Int("trim-ms", 1400, "Corte inicial em ms para humanos mono (descarte do prompt do bot)")
	_ = fs.Parse(args)

	humansDir := filepath.Join(*outputDir, "humans")
	machinesDir := filepath.Join(*outputDir, "machines")
	balancedDir := filepath.Join(*outputDir, "balanced")

	for _, dir := range []string{humansDir, machinesDir, balancedDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Printf("Erro ao criar diretório %s: %v\n", dir, err)
			os.Exit(1)
		}
	}

	db, err := ConnectDB(*dbURL)
	if err != nil {
		fmt.Printf("Erro ao conectar ao PostgreSQL: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	query := `
		SELECT phone, disposition, recording_file
		FROM cdrs
		WHERE created_at::date = '2026-09-14'
		  AND recording_file IS NOT NULL
		  AND disposition IN ('DELIVERED', 'VOICEMAIL')
		ORDER BY id ASC;
	`
	rows, err := db.Query(query)
	if err != nil {
		fmt.Printf("Erro ao consultar CDRs de 14/09: %v\n", err)
		os.Exit(1)
	}
	defer rows.Close()

	type CallInfo struct {
		Phone       string
		Disposition string
		RecFile     string
	}
	var calls []CallInfo
	for rows.Next() {
		var c CallInfo
		if err := rows.Scan(&c.Phone, &c.Disposition, &c.RecFile); err == nil {
			calls = append(calls, c)
		}
	}

	fmt.Printf("Iniciando preparação saneada de %d registros CDR do banco...\n", len(calls))

	var humanCount, machineCount, skippedCount int
	var humanFiles, machineFiles []string

	for _, c := range calls {
		baseFile := filepath.Base(c.RecFile)
		srcPath := filepath.Join(*inputDir, baseFile)

		info, err := os.Stat(srcPath)
		if err != nil || info.Size() <= 44 {
			skippedCount++
			continue
		}

		w, err := ReadWav(srcPath)
		if err != nil {
			skippedCount++
			continue
		}

		if c.Disposition == "DELIVERED" {
			// Humano: Se estéreo, pega canal RX. Se mono, aplica trim de trimMs (1.4s)
			var pcm []byte
			if w.NumChannels == 2 {
				_, rightRx, err := SplitStereoToMono(w)
				if err == nil && len(rightRx) > 0 {
					pcm = rightRx
				}
			}
			if len(pcm) == 0 {
				trimmed, err := TrimWavAudio(w, *trimMs)
				if err != nil || len(trimmed.Data) < 8000 {
					skippedCount++
					continue
				}
				pcm = trimmed.Data
			}

			destPath := filepath.Join(humansDir, baseFile)
			if err := WriteMonoWav(destPath, w.SampleRate, pcm); err == nil {
				humanCount++
				humanFiles = append(humanFiles, destPath)
			}
		} else if c.Disposition == "VOICEMAIL" {
			// Máquina: Se estéreo, pega canal RX. Se mono, aplica trim de trimMs (1.4s) para remover o robô institucional
			var pcm []byte
			if w.NumChannels == 2 {
				_, rightRx, err := SplitStereoToMono(w)
				if err == nil && len(rightRx) > 0 {
					pcm = rightRx
				}
			}
			if len(pcm) == 0 {
				trimmed, err := TrimWavAudio(w, *trimMs)
				if err != nil || len(trimmed.Data) < 8000 {
					skippedCount++
					continue
				}
				pcm = trimmed.Data
			}

			destPath := filepath.Join(machinesDir, baseFile)
			if err := WriteMonoWav(destPath, w.SampleRate, pcm); err == nil {
				machineCount++
				machineFiles = append(machineFiles, destPath)
			}
		}
	}


	// Montagem do pool balanceado para o simulador Asterisk
	minPool := humanCount
	if machineCount < minPool {
		minPool = machineCount
	}
	if minPool > 1000 {
		minPool = 1000
	}

	for i := 0; i < minPool; i++ {
		// 1 Humano
		hSrc := humanFiles[i]
		hPhone := extractPhoneFromFilename(filepath.Base(hSrc))
		if hPhone != "" {
			data, _ := os.ReadFile(hSrc)
			_ = os.WriteFile(filepath.Join(balancedDir, hPhone+".wav"), data, 0644)
		}

		// 1 Máquina
		mSrc := machineFiles[i]
		mPhone := extractPhoneFromFilename(filepath.Base(mSrc))
		if mPhone != "" {
			data, _ := os.ReadFile(mSrc)
			_ = os.WriteFile(filepath.Join(balancedDir, mPhone+".wav"), data, 0644)
		}
	}

	fmt.Println("================================================================================")
	fmt.Println("✅ PREPARAÇÃO SANEADA E SEGREGAÇÃO ACÚSTICA CONCLUÍDA")
	fmt.Println("================================================================================")
	fmt.Printf("Total de CDRs Analisados:                         %d\n", len(calls))
	fmt.Printf("  - Humanos Reais Saneados (Trim %d ms aplicado): %d\n", *trimMs, humanCount)
	fmt.Printf("  - Caixas Postais Reais (Áudio integral mantido): %d\n", machineCount)
	fmt.Printf("  - Áudios descartados (curtos/vazios/ausentes):  %d\n", skippedCount)
	fmt.Printf("  - Pool Balanceado 50/50 Gerado para Simulador:   %d áudios em %s\n", minPool*2, balancedDir)
	fmt.Println("================================================================================")
}


func runTranscribeBatch(args []string) {
	fs := flag.NewFlagSet("transcribe-batch", flag.ExitOnError)
	clientsDir := fs.String("clients-dir", "/opt/ominichat/asterisk/monitor/benchmark_saneado", "Diretório com áudios saneados (humans/machines)")
	voskURL := fs.String("vosk-url", "ws://127.0.0.1:2700", "URL WebSocket do Vosk Server")
	dbURL := fs.String("db-url", "postgresql://postgres:ojr6DNM25InIg3qoYI4nbUL5Cx04iCiCn1122@localhost:5432/dialer_db?sslmode=disable", "URL PostgreSQL")
	concurrency := fs.Int("concurrency", 10, "Número de workers concorrentes")
	limit := fs.Int("limit", 0, "Limite de arquivos para transcrever (0 = todos)")
	_ = fs.Parse(args)

	var files []string
	for _, sub := range []string{"humans", "machines"} {
		matches, _ := filepath.Glob(filepath.Join(*clientsDir, sub, "*.wav"))
		files = append(files, matches...)
	}
	if len(files) == 0 {
		files, _ = filepath.Glob(filepath.Join(*clientsDir, "*.wav"))
	}
	if len(files) == 0 {
		fmt.Printf("Nenhum arquivo WAV encontrado em %s\n", *clientsDir)
		os.Exit(1)
	}

	if *limit > 0 && len(files) > *limit {
		files = files[:*limit]
	}

	db, err := ConnectDB(*dbURL)
	if err != nil {
		fmt.Printf("Erro ao conectar ao PostgreSQL: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	fmt.Printf("Iniciando transcrição de %d arquivos de áudio com %d workers concorrentes...\n", len(files), *concurrency)

	fileChan := make(chan string, len(files))
	for _, f := range files {
		fileChan <- f
	}
	close(fileChan)

	var processed, success, withText int64
	var wg sync.WaitGroup
	startTime := time.Now()

	for w := 0; w < *concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for f := range fileChan {
				wavAudio, err := ReadWav(f)
				if err != nil {
					atomic.AddInt64(&processed, 1)
					continue
				}

				text, err := TranscribeAudio(*voskURL, wavAudio.Data, 10*time.Second)
				atomic.AddInt64(&processed, 1)

				if err == nil {
					atomic.AddInt64(&success, 1)
					if text != "" {
						atomic.AddInt64(&withText, 1)
					}
					baseName := strings.TrimSuffix(filepath.Base(f), ".wav")
					_ = UpdateCDRTranscription(db, baseName, text)
				}
			}
		}()
	}

	// Monitor de progresso
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			p := atomic.LoadInt64(&processed)
			t := atomic.LoadInt64(&withText)
			fmt.Printf("Progresso: %d/%d (%.1f%%) - Transcrições com fala: %d\n", p, len(files), float64(p)/float64(len(files))*100, t)
			if int(p) >= len(files) {
				break
			}
		}
	}()

	wg.Wait()
	duration := time.Since(startTime)

	fmt.Println("================================================================================")
	fmt.Println("✅ TRANSCRIÇÃO EM LOTE CONCLUÍDA")
	fmt.Println("================================================================================")
	fmt.Printf("Total Processado:             %d\n", processed)
	fmt.Printf("Sucessos WebSocket:           %d\n", success)
	fmt.Printf("Áudios com Fala Transcrita:   %d\n", withText)
	fmt.Printf("Tempo Total Decorrido:        %s (%.1f áudios/seg)\n", duration, float64(processed)/duration.Seconds())
	fmt.Println("================================================================================")
}

func runEvaluateLegacy(args []string) {
	fs := flag.NewFlagSet("evaluate-legacy", flag.ExitOnError)
	dbURL := fs.String("db-url", "postgresql://postgres:ojr6DNM25InIg3qoYI4nbUL5Cx04iCiCn1122@localhost:5432/dialer_db?sslmode=disable", "URL PostgreSQL")
	_ = fs.Parse(args)

	_, err := EvaluateLegacySystem(*dbURL)
	if err != nil {
		fmt.Printf("Erro na avaliação: %v\n", err)
		os.Exit(1)
	}
}

func extractPhoneFromFilename(name string) string {
	parts := strings.Split(name, "-")
	if len(parts) >= 3 {
		return parts[2]
	}
	return ""
}
