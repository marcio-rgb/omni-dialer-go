package http

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Server struct {
	router     *chi.Mux
	httpServer *http.Server
	port       int
}

type HandlersConfig struct {
	WhitelistMiddleware *IPWhitelistMiddleware
	Predictive          *PredictiveHandler
	Manual              *ManualHandler
	Campaign            *CampaignHandler
	Refill              *RefillHandler
	Toggle              *ToggleHandler
	Saturation          *SaturationHandler
	Report              *ReportHandler
	Trunk               *TrunkHandler
	Health              *HealthHandler
	Audio               *AudioWordsHandler
	AMD                 *AMDHandler
	LeadBatch           *LeadBatchHandler
}

func NewServer(port int, handlers HandlersConfig) *Server {
	r := chi.NewRouter()

	// Middlewares globais essenciais
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	// Health check (sem bloqueio de IP)
	r.Get("/health", handlers.Health.HealthCheck)

	// Rotas protegidas por IP Whitelist
	r.Group(func(protected chi.Router) {
		if handlers.WhitelistMiddleware != nil {
			protected.Use(handlers.WhitelistMiddleware.Handler)
		}

		protected.Route("/api/v1", func(api chi.Router) {
			// 1. Preditivo & Demanda
			api.Post("/predictive/demand", handlers.Predictive.Demand)

			// 2. Chamadas Manuais
			api.Post("/calls/manual", handlers.Manual.DialManual)

			// 3. Campanhas (CRUD), Refill, Ingestão de Leads, Toggle & Saturação
			api.Route("/campaigns", func(camp chi.Router) {
				if handlers.Campaign != nil {
					camp.Get("/", handlers.Campaign.List)
					camp.Post("/", handlers.Campaign.Create)
					camp.Get("/{id}", handlers.Campaign.Get)
					camp.Put("/{id}", handlers.Campaign.Update)
					camp.Delete("/{id}", handlers.Campaign.Delete)
				}
				camp.Post("/refill", handlers.Refill.Refill)
				if handlers.LeadBatch != nil {
					camp.Post("/{campaign_id}/leads", handlers.LeadBatch.IngestBatch)
					camp.Post("/leads", handlers.LeadBatch.IngestBatch)
				}
				camp.Post("/toggle", handlers.Toggle.Toggle)
				camp.Get("/saturation", handlers.Saturation.GetConsolidated)
				camp.Get("/{campaign_id}/saturation", handlers.Saturation.GetIndividual)
			})

			// 4. Relatório Analítico de Chamadas & CDRs Canônicos
			api.Get("/reports/calls-summary", handlers.Report.GetCallsSummary)
			api.Get("/reports/cdrs", handlers.Report.ListCDRs)
			api.Get("/cdrs", handlers.Report.ListCDRs)
			api.Get("/cdrs/{id}", handlers.Report.GetCDR)

			// 5. Troncos SIP/PJSIP & Telemetria
			api.Route("/trunks", func(trunks chi.Router) {
				trunks.Get("/enumerations", handlers.Trunk.Enumerations)
				trunks.Get("/", handlers.Trunk.List)
				trunks.Post("/", handlers.Trunk.Create)
				trunks.Put("/{trunk_id}", handlers.Trunk.Update)
				trunks.Delete("/{trunk_id}", handlers.Trunk.Delete)
				trunks.Post("/reload", handlers.Trunk.Reload)
			})

			// 6. Pré-renderização e Concatenação de Áudios (Piper TTS)
			if handlers.Audio != nil {
				api.Route("/audio", func(audio chi.Router) {
					audio.Post("/words", handlers.Audio.UpsertWords)
					audio.Get("/preview", handlers.Audio.Preview)
					audio.Post("/preview", handlers.Audio.Preview)
				})
				// 6.1. Streaming de Gravações Reais do Asterisk (MixMonitor)
				api.Get("/recordings/*", handlers.Audio.ServeRecording)
				api.Head("/recordings/*", handlers.Audio.ServeRecording)
			}

			// 7. Configuração Dinâmica de AMD & Hot-Reload Asterisk
			if handlers.AMD != nil {
				api.Route("/amd", func(amd chi.Router) {
					amd.Get("/config", handlers.AMD.GetConfig)
					amd.Put("/config", handlers.AMD.UpdateConfig)
					amd.Post("/config", handlers.AMD.UpdateConfig)
					amd.Post("/reload", handlers.AMD.Reload)
				})
			}
		})
	})

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return &Server{
		router:     r,
		httpServer: srv,
		port:       port,
	}
}

func (s *Server) Start() error {
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
