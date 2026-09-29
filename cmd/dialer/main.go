package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"dialer-go/config"
	"dialer-go/internal/adapters/ami"
	httpAdapter "dialer-go/internal/adapters/http"
	"dialer-go/internal/adapters/livekit"
	"dialer-go/internal/adapters/postgres"
	redisAdapter "dialer-go/internal/adapters/redis"
	"dialer-go/internal/adapters/storage"
	"dialer-go/internal/adapters/tts"
	"dialer-go/internal/adapters/webhook"
	"dialer-go/internal/core"
)

func main() {
	log.Println("[BOOT] Inicializando Dialer-Go Engine...")

	// 1. Carrega configurações
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("[FATAL] Falha nas variáveis de ambiente: %v", err)
	}

	log.Printf("[BOOT] Modo de Operação Ativo: %s", strings.ToUpper(cfg.OperationMode))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 2. Conecta ao PostgreSQL (dialer_db) com retentativas
	pgPool, err := postgres.NewConnectionPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("[FATAL] Banco relacional indisponível: %v", err)
	}
	defer pgPool.Close()
	log.Println("[INFO] PostgreSQL dialer_db conectado.")

	// 2.1. Executa auto-migração de schema e auto-seed idempotente no banco relacional
	if err := postgres.AutoMigrateSchema(ctx, pgPool); err != nil {
		log.Printf("[WARN] Falha durante auto-migração do schema: %v", err)
	}

	// 3. Conecta ao Asterisk PBX (AMI TCP Socket)
	amiClient := ami.NewAMIClient(cfg.AsteriskAMIHost, cfg.AsteriskAMIPort, cfg.AsteriskAMIUser, cfg.AsteriskAMIPass)
	if err := amiClient.Connect(ctx); err != nil {
		log.Printf("[WARN] AMI Socket (:5038) não conectado no boot (iniciando reconexão em background): %v", err)
		amiClient.StartBackgroundReconnect()
	} else {
		log.Println("[INFO] Asterisk AMI conectado com sucesso.")
	}
	defer amiClient.Close()

	// 4. Repositórios base
	trunkRepo := postgres.NewTrunkRepo(pgPool)
	sipConfigRepo := postgres.NewSIPConfigRepo(pgPool)
	instanceRepo := postgres.NewInstanceRepo(pgPool)
	instanceService := core.NewInstanceService(instanceRepo)
	instanceHandler := httpAdapter.NewInstanceHandler(instanceService)

	// 5. Adaptador LiveKit SIP (Auto-provisionamento, Reconciliação Contínua e Telemetria)
	livekitClient := livekit.NewClient(cfg.LiveKitURL, cfg.LiveKitAPIKey, cfg.LiveKitAPISecret)
	livekitClient.StartReconciler(ctx, 30*time.Second)

	sipConfigMgr := core.NewSIPConfigManager(sipConfigRepo, cfg.ConfigSeedDir)
	sipConfigMgr.SeedFromDiskIfEmpty(ctx)
	sipConfigHandler := httpAdapter.NewSIPConfigHandler(sipConfigMgr, amiClient)
	whitelist := httpAdapter.NewIPWhitelistMiddleware(cfg.InitialWhitelistIPs)

	var handlersConfig httpAdapter.HandlersConfig

	if cfg.OperationMode == "dispatcher" {
		log.Println("[INFO] Executando em modo DISPATCHER (Gateway SIP Local - Sem Redis e Sem Webhooks).")
		channelMgr := core.NewChannelManager(cfg.MaxGlobalChannels, 0, nil)
		trunkMgr := core.NewTrunkManager(trunkRepo, nil, amiClient, channelMgr, nil, nil)
		trunkMgr.StartDaemon(ctx)
		defer trunkMgr.Stop()

		healthHandler := httpAdapter.NewHealthHandler(pgPool, nil, amiClient, channelMgr)
		healthHandler.SetLiveKitPort(livekitClient)

		handlersConfig = httpAdapter.HandlersConfig{
			WhitelistMiddleware: whitelist,
			Trunk:               httpAdapter.NewTrunkHandler(trunkRepo, nil, trunkMgr, channelMgr, amiClient),
			Health:              healthHandler,
			SIPConfig:           sipConfigHandler,
			Instance:            instanceHandler,
		}
	} else {
		// Modo DIALER Completo (Padrão)
		log.Println("[INFO] Executando em modo DIALER Completo (Preditivo, Manual, Pacing, Vosk STT, Redis e Webhooks).")

		// Conecta ao Redis e ativa escuta de mortes silenciosas (expirações de heartbeat)
		cache, err := redisAdapter.NewRedisAdapter(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
		if err != nil {
			log.Fatalf("[FATAL] Redis indisponível no boot: %v", err)
		}
		cache.StartKeyspaceListener(ctx, cfg.RedisDB)
		log.Println("[INFO] Redis conectado e Keyspace Listener (Ex) ativado.")

		routingRepo := postgres.NewRoutingRepo(pgPool)
		leadRepo := postgres.NewLeadRepo(pgPool)
		campaignRepo := postgres.NewCampaignRepo(pgPool)
		reportRepo := postgres.NewReportRepo(pgPool)
		tenantRepo := postgres.NewTenantRepo(pgPool)
		agentRepo := postgres.NewAgentRepo(pgPool)
		storageAdapter := storage.NewStorageAdapter(cfg.MinIOEndpoint, cfg.MinIOAccessKey, cfg.MinIOSecretKey, cfg.MinIOUseSSL)

		webhookAdapter := webhook.NewWebhookClient(cfg.WebhookURL)
		callNotifier := core.NewCallNotifier(webhookAdapter, cfg.WebhookURL)

		channelMgr := core.NewChannelManager(cfg.MaxGlobalChannels, cfg.HumanReserveQuota, cache)
		trunkMgr := core.NewTrunkManager(trunkRepo, cache, amiClient, channelMgr, reportRepo, routingRepo)
		inboundEngine := core.NewInboundEngine(amiClient, routingRepo, channelMgr, "from-internal", "s")
		predictiveEngine := core.NewPredictiveEngine(amiClient, channelMgr, cache, campaignRepo, trunkRepo, leadRepo)
		predictiveEngine.SetMinChannelsPerAgent(cfg.MinChannelsPerAgent)
		predictiveEngine.SetTenantRepository(tenantRepo)
		predictiveEngine.SetReportRepository(reportRepo)
		predictiveEngine.SetWebhookClient(webhookAdapter)
		trunkMgr.SetEngines(predictiveEngine, inboundEngine)
		trunkMgr.SetNotifier(callNotifier)
		trunkMgr.StartDaemon(ctx)
		defer trunkMgr.Stop()
		manualEngine := core.NewManualEngine(amiClient, channelMgr, trunkRepo, cache)
		manualEngine.SetTenantRepository(tenantRepo)
		manualEngine.SetWebhookClient(webhookAdapter)

		// 7.1. Inicializa Workers em Background para Redis Streams (CDR Persister e Faster-Whisper)
		cdrPersister := core.NewCDRPersisterWorker(cache, reportRepo)
		cdrPersister.StartDaemon(ctx)
		defer cdrPersister.Stop()

		whisperWorker := core.NewWhisperTranscriptionWorker(cache, reportRepo, "")
		whisperWorker.StartDaemon(ctx)
		defer whisperWorker.Stop()

		mailingProcessor := core.NewMailingProcessor(storageAdapter, leadRepo, cache)
		saturationService := core.NewSaturationService(leadRepo, campaignRepo)

		piperAdapter, err := tts.NewPiperAdapter("", "", "")
		var audioHandler *httpAdapter.AudioWordsHandler
		if err != nil {
			log.Printf("[WARN] Piper TTS indisponivel (%v). Rotas /audio operarao apenas com cache.", err)
		} else {
			log.Println("[INFO] Piper TTS (Voz Dii PT-BR) inicializado com sucesso.")
		}
		audioConcatenator := core.NewAudioConcatenator()
		audioWordMgr, err := core.NewAudioWordManager("./storage/audio_cache", piperAdapter, audioConcatenator)
		if err != nil {
			log.Printf("[WARN] Falha ao inicializar AudioWordManager: %v", err)
		} else {
			audioHandler = httpAdapter.NewAudioWordsHandler(audioWordMgr)
			mailingProcessor.SetAudioWordManager(audioWordMgr)
		}

		amdConfigMgr := core.NewAMDConfigManager("./storage", "")
		amdHandler := httpAdapter.NewAMDHandler(amdConfigMgr, amiClient)
		leadBatchHandler := httpAdapter.NewLeadBatchHandler(leadRepo, cache, audioWordMgr)

		// 8. Gestão Dinâmica de Filas e Presença de Operadores (Asterisk app_queue + Redis PubSub)
		agentQueueMgr := core.NewAgentQueueManager(amiClient, cache)
		agentQueueMgr.SetAgentRepository(agentRepo)
		cache.StartPresenceListener(ctx, agentQueueMgr)
		queueHandler := httpAdapter.NewQueueHandler(agentQueueMgr)
		agentHandler := httpAdapter.NewAgentHandler(agentRepo)

		healthHandler := httpAdapter.NewHealthHandler(pgPool, cache, amiClient, channelMgr)
		healthHandler.SetLiveKitPort(livekitClient)

		handlersConfig = httpAdapter.HandlersConfig{
			WhitelistMiddleware: whitelist,
			Predictive:          httpAdapter.NewPredictiveHandler(predictiveEngine),
			Manual:              httpAdapter.NewManualHandler(manualEngine),
			Campaign:            httpAdapter.NewCampaignHandler(campaignRepo, cache),
			Refill:              httpAdapter.NewRefillHandler(mailingProcessor),
			Toggle:              httpAdapter.NewToggleHandler(campaignRepo, cache),
			Saturation:          httpAdapter.NewSaturationHandler(saturationService),
			Report:              httpAdapter.NewReportHandler(reportRepo, cache),
			Trunk:               httpAdapter.NewTrunkHandler(trunkRepo, cache, trunkMgr, channelMgr, amiClient),
			Health:              healthHandler,
			Audio:               audioHandler,
			AMD:                 amdHandler,
			LeadBatch:           leadBatchHandler,
			SIPConfig:           sipConfigHandler,
			Instance:            instanceHandler,
			Queue:               queueHandler,
			Agent:               agentHandler,
		}
	}

	server := httpAdapter.NewServer(cfg.AppPort, handlersConfig)

	// 9. Inicialização com Graceful Shutdown
	go func() {
		log.Printf("[INFO] Servidor HTTP pronto na porta :%d", cfg.AppPort)
		if err := server.Start(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[FATAL] Falha no servidor HTTP: %v", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	log.Println("[SHUTDOWN] Sinal de encerramento recebido. Encerrando graciosamente...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("[ERROR] Falha durante encerramento HTTP: %v", err)
	}

	log.Println("[SHUTDOWN] Dialer-Go finalizado com sucesso.")
}
