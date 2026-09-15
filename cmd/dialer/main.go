package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"dialer-go/config"
	"dialer-go/internal/adapters/ami"
	httpAdapter "dialer-go/internal/adapters/http"
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 2. Conecta ao PostgreSQL (dialer_db) com retentativas
	pgPool, err := postgres.NewConnectionPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("[FATAL] Banco relacional indisponível: %v", err)
	}
	defer pgPool.Close()
	log.Println("[INFO] PostgreSQL dialer_db conectado.")

	// 3. Conecta ao Redis e ativa escuta de mortes silenciosas (expirações de heartbeat)
	cache, err := redisAdapter.NewRedisAdapter(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if err != nil {
		log.Fatalf("[FATAL] Redis indisponível no boot: %v", err)
	}
	cache.StartKeyspaceListener(ctx, cfg.RedisDB)
	log.Println("[INFO] Redis conectado e Keyspace Listener (Ex) ativado.")


	// 4. Conecta ao Asterisk PBX (AMI TCP Socket)
	amiClient := ami.NewAMIClient(cfg.AsteriskAMIHost, cfg.AsteriskAMIPort, cfg.AsteriskAMIUser, cfg.AsteriskAMIPass)
	if err := amiClient.Connect(ctx); err != nil {
		log.Printf("[WARN] AMI Socket (:5038) não conectado no boot (iniciando reconexão em background): %v", err)
		amiClient.StartBackgroundReconnect()
	} else {
		log.Println("[INFO] Asterisk AMI conectado com sucesso.")
	}
	defer amiClient.Close()

	// 5. Repositórios
	trunkRepo := postgres.NewTrunkRepo(pgPool)
	routingRepo := postgres.NewRoutingRepo(pgPool)
	leadRepo := postgres.NewLeadRepo(pgPool)
	campaignRepo := postgres.NewCampaignRepo(pgPool)
	reportRepo := postgres.NewReportRepo(pgPool)
	sipConfigRepo := postgres.NewSIPConfigRepo(pgPool)
	tenantRepo := postgres.NewTenantRepo(pgPool)
	storageAdapter := storage.NewStorageAdapter(cfg.MinIOEndpoint, cfg.MinIOAccessKey, cfg.MinIOSecretKey, cfg.MinIOUseSSL)

	// 6. Core Engines & Arbitragem
	webhookAdapter := webhook.NewWebhookClient(cfg.WebhookURL)
	callNotifier := core.NewCallNotifier(webhookAdapter, cfg.WebhookURL)

	channelMgr := core.NewChannelManager(cfg.MaxGlobalChannels, cfg.HumanReserveQuota, cache)
	trunkMgr := core.NewTrunkManager(trunkRepo, cache, amiClient, channelMgr, reportRepo, routingRepo)
	inboundEngine := core.NewInboundEngine(amiClient, routingRepo, channelMgr, "from-internal", "s")
	predictiveEngine := core.NewPredictiveEngine(amiClient, channelMgr, cache, campaignRepo, trunkRepo, leadRepo)
	predictiveEngine.SetMinChannelsPerAgent(cfg.MinChannelsPerAgent)
	predictiveEngine.SetTenantRepository(tenantRepo)
	predictiveEngine.SetWebhookClient(webhookAdapter)
	trunkMgr.SetEngines(predictiveEngine, inboundEngine)
	trunkMgr.SetNotifier(callNotifier)
	trunkMgr.StartDaemon(ctx)
	defer trunkMgr.Stop()
	manualEngine := core.NewManualEngine(amiClient, channelMgr, trunkRepo, cache)
	manualEngine.SetTenantRepository(tenantRepo)
	manualEngine.SetWebhookClient(webhookAdapter)
	mailingProcessor := core.NewMailingProcessor(storageAdapter, leadRepo, cache)
	saturationService := core.NewSaturationService(leadRepo, campaignRepo)

	// 7. Motor TTS & Cache de Áudios
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

	// 8. Gestão Dinâmica de AMD, Configurações Asterisk (sip_data) & Reconhecimento de Voz
	amdConfigMgr := core.NewAMDConfigManager("./storage", "")
	amdHandler := httpAdapter.NewAMDHandler(amdConfigMgr, amiClient)
	leadBatchHandler := httpAdapter.NewLeadBatchHandler(leadRepo, cache, audioWordMgr)

	sipConfigMgr := core.NewSIPConfigManager(sipConfigRepo, "")
	sipConfigMgr.SeedFromDiskIfEmpty(ctx)
	sipConfigHandler := httpAdapter.NewSIPConfigHandler(sipConfigMgr, amiClient)

	// 9. Middlewares & Handlers HTTP
	whitelist := httpAdapter.NewIPWhitelistMiddleware(cfg.InitialWhitelistIPs)

	handlersConfig := httpAdapter.HandlersConfig{
		WhitelistMiddleware: whitelist,
		Predictive:          httpAdapter.NewPredictiveHandler(predictiveEngine),
		Manual:              httpAdapter.NewManualHandler(manualEngine),
		Campaign:            httpAdapter.NewCampaignHandler(campaignRepo, cache),
		Refill:              httpAdapter.NewRefillHandler(mailingProcessor),
		Toggle:              httpAdapter.NewToggleHandler(campaignRepo, cache),
		Saturation:          httpAdapter.NewSaturationHandler(saturationService),
		Report:              httpAdapter.NewReportHandler(reportRepo, cache),
		Trunk:               httpAdapter.NewTrunkHandler(trunkRepo, cache, trunkMgr, channelMgr, amiClient),
		Health:              httpAdapter.NewHealthHandler(pgPool, cache, amiClient, channelMgr),
		Audio:               audioHandler,
		AMD:                 amdHandler,
		LeadBatch:           leadBatchHandler,
		SIPConfig:           sipConfigHandler,
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
