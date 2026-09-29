package postgres

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AutoMigrateSchema executa DDLs e seeds idempotentes para assegurar a consistência
// estrutural de dialer_db ao inicializar ou recuperar o serviço.
//
// @pattern Migrator / Unit of Work
// @governedBy .agents/ARCHITECT.md#2-invariantes-arquiteturais
//
// @preExecution
// - Pool de conexões pgxpool ativo e autenticado com dialer_db
//
// @postExecution
// - Cria tabelas essenciais ausentes (tenants, campaigns, leads, cdrs, trunks, sip_data, phone_trunk_mappings)
// - Garante existência de colunas requeridas com ADD COLUMN IF NOT EXISTS
// - Insere registros de seed (Tenant Padrão e Troncos PJSIP essenciais) de forma idempotente
func AutoMigrateSchema(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return fmt.Errorf("migrator: pool de conexao nulo")
	}

	log.Println("[INFO] [MIGRATOR] Iniciando checagem e auto-migração de schema no PostgreSQL...")

	ddlStatements := []string{
		// 1. Tabela de Tenants
		`CREATE TABLE IF NOT EXISTS tenants (
			id VARCHAR(64) PRIMARY KEY,
			name VARCHAR(128) NOT NULL,
			webhook VARCHAR(255),
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		);`,

		// 2. Tabela de Troncos PJSIP
		`CREATE TABLE IF NOT EXISTS trunks (
			id VARCHAR(64) PRIMARY KEY,
			tenant_id VARCHAR(64) NOT NULL DEFAULT 'default',
			name VARCHAR(128) NOT NULL,
			direction VARCHAR(32) NOT NULL DEFAULT 'BIDIRECTIONAL',
			registration_mode VARCHAR(32) NOT NULL DEFAULT 'IP_BASED',
			host VARCHAR(255) NOT NULL,
			port INTEGER NOT NULL DEFAULT 5060,
			username VARCHAR(128),
			secret VARCHAR(255),
			tech_prefix VARCHAR(32),
			user_agent VARCHAR(128),
			transport VARCHAR(16) NOT NULL DEFAULT 'UDP',
			nat_mode VARCHAR(32) NOT NULL DEFAULT 'FORCE_RPORT',
			direct_media BOOLEAN NOT NULL DEFAULT false,
			codecs JSONB NOT NULL DEFAULT '["alaw", "ulaw"]'::jsonb,
			dtmf_mode VARCHAR(16) NOT NULL DEFAULT 'RFC4733',
			rtp_timeout INTEGER NOT NULL DEFAULT 0,
			qualify_frequency INTEGER NOT NULL DEFAULT 30,
			qualify_timeout INTEGER NOT NULL DEFAULT 3,
			max_channels INTEGER NOT NULL DEFAULT 30,
			is_enabled BOOLEAN NOT NULL DEFAULT true,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		);`,

		// 3. Tabela de Campanhas
		`CREATE TABLE IF NOT EXISTS campaigns (
			id VARCHAR(64) PRIMARY KEY,
			tenant_id VARCHAR(64) NOT NULL DEFAULT 'default',
			name VARCHAR(128) NOT NULL,
			mode VARCHAR(32) NOT NULL DEFAULT 'PREDICTIVE',
			status VARCHAR(32) NOT NULL DEFAULT 'active',
			aggressiveness NUMERIC(5,2) NOT NULL DEFAULT 1.0,
			trunk_name VARCHAR(64) NOT NULL DEFAULT 'rvx',
			cycle_count INTEGER NOT NULL DEFAULT 0,
			saturation_level VARCHAR(32) NOT NULL DEFAULT 'NOVA',
			last_cycle_at TIMESTAMP WITH TIME ZONE,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		);`,

		// 4. Tabela de Leads
		`CREATE TABLE IF NOT EXISTS leads (
			id BIGSERIAL PRIMARY KEY,
			campaign_id VARCHAR(64) NOT NULL,
			tenant_id VARCHAR(64) NOT NULL DEFAULT 'default',
			cpf VARCHAR(14) NOT NULL,
			phone VARCHAR(32) NOT NULL,
			name VARCHAR(255),
			first_name VARCHAR(64),
			work_word VARCHAR(64),
			work_words VARCHAR(64),
			att1 VARCHAR(255),
			att2 VARCHAR(255),
			att3 VARCHAR(255),
			custom JSONB DEFAULT '{}'::jsonb,
			status VARCHAR(32) NOT NULL DEFAULT 'NEW',
			attempts_count INTEGER NOT NULL DEFAULT 0,
			last_dialed_at TIMESTAMP WITH TIME ZONE,
			dialed_at TIMESTAMP WITH TIME ZONE,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		);`,

		// 5. Tabela de CDRs
		`CREATE TABLE IF NOT EXISTS cdrs (
			id VARCHAR(64) PRIMARY KEY,
			tenant_id VARCHAR(64) NOT NULL DEFAULT 'default',
			campaign_id VARCHAR(64),
			phone VARCHAR(32) NOT NULL,
			agent_id VARCHAR(64),
			call_type VARCHAR(32) NOT NULL,
			disposition VARCHAR(32) NOT NULL,
			sip_status INTEGER,
			hangup_cause INTEGER,
			duration_seconds INTEGER DEFAULT 0,
			billsec_seconds INTEGER DEFAULT 0,
			ring_seconds INTEGER DEFAULT 0,
			trunk_used VARCHAR(64) NOT NULL,
			recording_file VARCHAR(512),
			recording_url VARCHAR(512),
			transcription TEXT,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
			initiated_at TIMESTAMP WITH TIME ZONE,
			answered_at TIMESTAMP WITH TIME ZONE,
			ended_at TIMESTAMP WITH TIME ZONE
		);`,

		// 6. Tabela de Configurações Asterisk (sip_data)
		`CREATE TABLE IF NOT EXISTS sip_data (
			file VARCHAR(60) PRIMARY KEY,
			data TEXT NOT NULL,
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		);`,

		// 7. Tabela de Roteamento Receptivo (phone_trunk_mappings)
		`CREATE TABLE IF NOT EXISTS phone_trunk_mappings (
			phone VARCHAR(32) PRIMARY KEY,
			last_trunk VARCHAR(64) NOT NULL,
			last_project VARCHAR(64),
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		);`,

		// 8. Tabela de Execution Traces
		`CREATE TABLE IF NOT EXISTS execution_traces (
			id BIGSERIAL PRIMARY KEY,
			correlation_id VARCHAR(64) NOT NULL,
			event_name VARCHAR(64) NOT NULL,
			source_component VARCHAR(64) NOT NULL,
			payload JSONB DEFAULT '{}'::jsonb,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		);`,

		// 9. Tabela de Instâncias (Modo Dialer e Dispatcher)
		`CREATE TABLE IF NOT EXISTS instances (
			id VARCHAR(64) PRIMARY KEY,
			tenant_id VARCHAR(64) NOT NULL DEFAULT 'default',
			name VARCHAR(128) NOT NULL,
			mode VARCHAR(32) NOT NULL DEFAULT 'dialer',
			host_url VARCHAR(255) NOT NULL,
			api_key VARCHAR(255),
			max_channels INTEGER NOT NULL DEFAULT 30,
			is_active BOOLEAN NOT NULL DEFAULT true,
			description VARCHAR(255),
			metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		);`,

		// 10. Tabela de Cadastro Soberano de Agentes (public.tenants_agents)
		`CREATE TABLE IF NOT EXISTS tenants_agents (
			id BIGSERIAL PRIMARY KEY,
			tenant_id VARCHAR(64) NOT NULL DEFAULT 'default',
			agent_id VARCHAR(64) NOT NULL,
			agent_name VARCHAR(128) NOT NULL,
			is_active BOOLEAN NOT NULL DEFAULT true,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT uq_tenants_agents_tenant_agent UNIQUE (tenant_id, agent_id)
		);`,

		// 11. Tabela de Histórico Temporal de Presença/Estados do Agente (public.tenant_agent_history)
		`CREATE TABLE IF NOT EXISTS tenant_agent_history (
			id BIGSERIAL PRIMARY KEY,
			tenant_id VARCHAR(64) NOT NULL DEFAULT 'default',
			agent_id VARCHAR(64) NOT NULL,
			campaign_id VARCHAR(64),
			status VARCHAR(32) NOT NULL,
			action VARCHAR(32) NOT NULL,
			reason VARCHAR(128),
			livekit_room VARCHAR(128),
			call_id VARCHAR(64),
			started_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
			ended_at TIMESTAMP WITH TIME ZONE,
			duration_seconds INTEGER DEFAULT 0,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		);`,

		// 12. Garantia de colunas retrocompatíveis
		`ALTER TABLE leads ADD COLUMN IF NOT EXISTS work_word VARCHAR(64);`,
		`ALTER TABLE leads ADD COLUMN IF NOT EXISTS work_words VARCHAR(64);`,
		`ALTER TABLE leads ADD COLUMN IF NOT EXISTS custom JSONB DEFAULT '{}'::jsonb;`,
		`ALTER TABLE cdrs ADD COLUMN IF NOT EXISTS transcription TEXT;`,
		`ALTER TABLE cdrs ADD COLUMN IF NOT EXISTS recording_file VARCHAR(512);`,
		`ALTER TABLE cdrs ADD COLUMN IF NOT EXISTS recording_url VARCHAR(512);`,
		`ALTER TABLE cdrs ADD COLUMN IF NOT EXISTS lead_id BIGINT;`,
		`ALTER TABLE cdrs ADD COLUMN IF NOT EXISTS lead_name VARCHAR(255);`,
		`ALTER TABLE cdrs ADD COLUMN IF NOT EXISTS lead_cpf VARCHAR(32);`,
		`ALTER TABLE cdrs ADD COLUMN IF NOT EXISTS amd_status VARCHAR(32);`,
		`ALTER TABLE cdrs ADD COLUMN IF NOT EXISTS amd_cause VARCHAR(64);`,
		`ALTER TABLE trunks ADD COLUMN IF NOT EXISTS amd_enabled BOOLEAN NOT NULL DEFAULT FALSE;`,

		// 11. Índices determinísticos de alta performance
		`CREATE INDEX IF NOT EXISTS idx_leads_camp_status_id ON leads(campaign_id, status, id DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_leads_cooldown ON leads(campaign_id, last_dialed_at);`,
		`CREATE INDEX IF NOT EXISTS idx_cdrs_tenant_created ON cdrs(tenant_id, created_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_cdrs_camp ON cdrs(campaign_id);`,
		`CREATE INDEX IF NOT EXISTS idx_cdrs_phone ON cdrs(phone);`,
		`CREATE INDEX IF NOT EXISTS idx_cdrs_lead_cpf ON cdrs(lead_cpf);`,
		`CREATE INDEX IF NOT EXISTS idx_cdrs_lead_id ON cdrs(lead_id);`,
		`CREATE INDEX IF NOT EXISTS idx_trunks_tenant ON trunks(tenant_id);`,
		`CREATE INDEX IF NOT EXISTS idx_instances_tenant ON instances(tenant_id);`,
		`CREATE INDEX IF NOT EXISTS idx_instances_mode ON instances(mode);`,
		`CREATE INDEX IF NOT EXISTS idx_tenants_agents_lookup ON tenants_agents(tenant_id, agent_id);`,
		`CREATE INDEX IF NOT EXISTS idx_agent_hist_tenant_agent_started ON tenant_agent_history(tenant_id, agent_id, started_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_agent_hist_tenant_status_started ON tenant_agent_history(tenant_id, status, started_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_agent_hist_agent_id ON tenant_agent_history(agent_id);`,
	}

	for _, stmt := range ddlStatements {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("migrator: falha ao executar DDL [%s]: %w", stmt[:min(40, len(stmt))], err)
		}
	}

	// Seeds idempotentes (Tenant default, Troncos e Instâncias essenciais)
	seeds := []string{
		`INSERT INTO tenants (id, name, webhook)
		 VALUES ('default', 'Default Tenant', 'https://api-omnichat.creditobr.org/api/v1/telephony/webhook/inject-lead')
		 ON CONFLICT (id) DO UPDATE SET webhook = EXCLUDED.webhook WHERE tenants.webhook IS NULL OR tenants.webhook = '' OR tenants.webhook LIKE '%37.60.228.113%';`,

		`INSERT INTO trunks (id, tenant_id, name, direction, registration_mode, host, port, tech_prefix, user_agent, transport, nat_mode, codecs, dtmf_mode, max_channels, is_enabled)
		 VALUES 
		   ('ventitore', 'default', 'Ventitore', 'BIDIRECTIONAL', 'IP_BASED', '15.228.9.161', 5060, '430855', 'Asterisk', 'UDP', 'NO', '["ulaw","alaw"]'::jsonb, 'RFC4733', 40, true),
		   ('rvx', 'default', 'rvx', 'BIDIRECTIONAL', 'IP_BASED', '54.207.13.68', 5060, '943755', 'cbr-dialer', 'UDP', 'FORCE_RPORT', '["alaw","ulaw","g729","opus"]'::jsonb, 'RFC4733', 10, true),
		   ('livekit-sip', 'default', 'LiveKit SIP Gateway', 'BIDIRECTIONAL', 'IP_BASED', '84.247.135.255', 5062, '', '', 'UDP', 'FORCE_RPORT', '["alaw","ulaw","opus"]'::jsonb, 'RFC4733', 60, true),
		   ('dialer-teste', 'default', 'dialer-teste', 'OUTBOUND', 'IP_BASED', '38.242.219.186', 5070, '', '', 'UDP', 'FORCE_RPORT', '["alaw","ulaw"]'::jsonb, 'RFC4733', 50, true),
		   ('trunk-stress-legacy', 'default', 'trunk-stress-legacy', 'OUTBOUND', 'IP_BASED', '38.242.219.186', 5070, '', '', 'UDP', 'FORCE_RPORT', '["alaw","ulaw"]'::jsonb, 'RFC4733', 50, true)
		 ON CONFLICT (id) DO NOTHING;`,

		`INSERT INTO instances (id, tenant_id, name, mode, host_url, max_channels, is_active, description)
		 VALUES
		   ('dialer-cloud', 'default', 'Nuvem Principal', 'dialer', 'http://localhost:8081', 30, true, 'Nó central de inteligência e discagem'),
		   ('dispatcher-escritorio', 'default', 'Escritório Vivo', 'dispatcher', 'http://100.123.144.122:8081', 11, true, 'Gateway de 11 ramais Vivo MetaPBX')
		 ON CONFLICT (id) DO NOTHING;`,
	}

	for _, seed := range seeds {
		if _, err := pool.Exec(ctx, seed); err != nil {
			return fmt.Errorf("migrator: falha ao executar seed: %w", err)
		}
	}

	log.Println("[INFO] [MIGRATOR] Auto-migração concluída com sucesso. Esquema e seeds íntegros.")
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
