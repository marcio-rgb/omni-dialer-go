-- Dialer-Go Database Schema (dialer_db)

CREATE TYPE trunk_registration_mode AS ENUM ('REGISTER', 'IP_BASED');
CREATE TYPE trunk_transport AS ENUM ('UDP', 'TCP', 'TLS');
CREATE TYPE trunk_direction AS ENUM ('INBOUND', 'OUTBOUND', 'BIDIRECTIONAL');
CREATE TYPE trunk_dtmf_mode AS ENUM ('RFC4733', 'RFC2833', 'INBAND', 'INFO', 'AUTO');
CREATE TYPE trunk_nat_mode AS ENUM ('FORCE_RPORT', 'YES', 'NO', 'COMEDIA');

-- 1. Tabela de Troncos SIP/PJSIP
CREATE TABLE IF NOT EXISTS trunks (
    id VARCHAR(64) PRIMARY KEY,
    tenant_id VARCHAR(64) NOT NULL,
    name VARCHAR(128) NOT NULL,
    direction trunk_direction NOT NULL DEFAULT 'BIDIRECTIONAL',
    registration_mode trunk_registration_mode NOT NULL DEFAULT 'IP_BASED',
    host VARCHAR(255) NOT NULL,
    port INTEGER NOT NULL DEFAULT 5060,
    outbound_proxy VARCHAR(255),
    tech_prefix VARCHAR(32),
    auth_username VARCHAR(128),
    auth_password VARCHAR(255),
    auth_realm VARCHAR(128),
    from_user VARCHAR(128),
    from_domain VARCHAR(255),
    user_agent VARCHAR(255),
    transport trunk_transport NOT NULL DEFAULT 'UDP',
    nat_mode trunk_nat_mode NOT NULL DEFAULT 'FORCE_RPORT',
    direct_media BOOLEAN NOT NULL DEFAULT FALSE,
    codecs JSONB NOT NULL DEFAULT '["alaw", "ulaw"]'::jsonb,
    dtmf_mode trunk_dtmf_mode NOT NULL DEFAULT 'RFC4733',
    rtp_timeout INTEGER NOT NULL DEFAULT 30,
    qualify_frequency INTEGER NOT NULL DEFAULT 30,
    qualify_timeout NUMERIC(4,2) NOT NULL DEFAULT 3.00,
    max_channels INTEGER NOT NULL DEFAULT 30,
    is_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_trunks_tenant ON trunks(tenant_id);
CREATE INDEX IF NOT EXISTS idx_trunks_enabled ON trunks(is_enabled);

-- 2. Tabela de Mapeamento Receptivo O(1)
CREATE TABLE IF NOT EXISTS phone_trunk_mappings (
    phone VARCHAR(32) PRIMARY KEY,
    last_trunk VARCHAR(64) NOT NULL,
    last_project VARCHAR(128),
    last_sip_route VARCHAR(128),
    tenant_id VARCHAR(64) NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_phone_trunk_mappings_tenant ON phone_trunk_mappings(tenant_id);

-- 3. Tabela de Campanhas
CREATE TABLE IF NOT EXISTS campaigns (
    id VARCHAR(64) PRIMARY KEY,
    tenant_id VARCHAR(64) NOT NULL,
    name VARCHAR(128),
    mode VARCHAR(32) NOT NULL DEFAULT 'PREDICTIVE',
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    aggressiveness NUMERIC(3,2) NOT NULL DEFAULT 1.20,
    trunk_name VARCHAR(64) NOT NULL,
    cycle_count INTEGER NOT NULL DEFAULT 0,
    saturation_level VARCHAR(32) NOT NULL DEFAULT 'NOVA',
    last_cycle_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_campaigns_tenant_status ON campaigns(tenant_id, status);

-- 4. Tabela de Leads
CREATE TABLE IF NOT EXISTS leads (
    id BIGSERIAL PRIMARY KEY,
    campaign_id VARCHAR(64) NOT NULL,
    tenant_id VARCHAR(64) NOT NULL,
    cpf VARCHAR(14) NOT NULL,
    phone VARCHAR(32) NOT NULL,
    name VARCHAR(255),
    first_name VARCHAR(64),
    work_word VARCHAR(64),
    status VARCHAR(32) NOT NULL DEFAULT 'NEW',
    attempts_count INTEGER NOT NULL DEFAULT 0,
    last_dialed_at TIMESTAMP WITH TIME ZONE,
    dialed_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

ALTER TABLE leads ADD COLUMN IF NOT EXISTS name VARCHAR(255);
ALTER TABLE leads ADD COLUMN IF NOT EXISTS first_name VARCHAR(64);
ALTER TABLE leads ADD COLUMN IF NOT EXISTS work_word VARCHAR(64);

CREATE INDEX IF NOT EXISTS idx_leads_campaign_filo ON leads(campaign_id, status, last_dialed_at, id DESC);
CREATE INDEX IF NOT EXISTS idx_leads_tenant ON leads(tenant_id);
CREATE INDEX IF NOT EXISTS idx_leads_first_name ON leads(first_name);

-- 5. Tabela de CDRs (Call Detail Records)
CREATE TABLE IF NOT EXISTS cdrs (
    id VARCHAR(64) PRIMARY KEY,
    tenant_id VARCHAR(64) NOT NULL,
    campaign_id VARCHAR(64),
    phone VARCHAR(32) NOT NULL,
    agent_id VARCHAR(64),
    call_type VARCHAR(32) NOT NULL,
    disposition VARCHAR(32) NOT NULL,
    sip_status INTEGER,
    hangup_cause INTEGER,
    duration_seconds INTEGER NOT NULL DEFAULT 0,
    billsec_seconds INTEGER NOT NULL DEFAULT 0,
    ring_seconds INTEGER NOT NULL DEFAULT 0,
    trunk_used VARCHAR(64) NOT NULL,
    recording_file VARCHAR(512),
    recording_url VARCHAR(512),
    transcription TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    initiated_at TIMESTAMP WITH TIME ZONE,
    answered_at TIMESTAMP WITH TIME ZONE,
    ended_at TIMESTAMP WITH TIME ZONE
);

-- Índice Composto Cobridor para o Relatório com Buffer de 15 Minutos
CREATE INDEX IF NOT EXISTS idx_cdrs_tenant_dates_disposition 
ON cdrs (tenant_id, created_at, disposition, sip_status, hangup_cause);

-- Índice Textual GIN para busca analítica em transcrições de voz
CREATE INDEX IF NOT EXISTS idx_cdrs_transcription_search 
ON cdrs USING gin (to_tsvector('portuguese', COALESCE(transcription, '')));

-- ============================================================================
-- 6. STORED PROCEDURES DE INTELIGÊNCIA PREDITIVA & TRANSAÇÕES ACID
-- ============================================================================

-- 6.1. Reserva Atômica de Lote FILO com Lock Anti-Concorrência (Pré-Execução)
CREATE OR REPLACE FUNCTION fn_audit_claim_predictive_batch(
    p_campaign_id VARCHAR(64),
    p_tenant_id VARCHAR(64),
    p_limit INTEGER,
    p_cooldown_hours INTEGER DEFAULT 2
) RETURNS TABLE (
    lead_id BIGINT,
    campaign_id VARCHAR(64),
    tenant_id VARCHAR(64),
    cpf VARCHAR(14),
    phone VARCHAR(32),
    name VARCHAR(255),
    first_name VARCHAR(64),
    attempts_count INTEGER
) AS $$
BEGIN
    RETURN QUERY
    WITH selected_leads AS (
        SELECT l.id
        FROM leads l
        WHERE l.campaign_id = p_campaign_id
          AND l.tenant_id = p_tenant_id
          AND l.status IN ('NEW', 'QUEUED')
          AND (l.last_dialed_at IS NULL OR l.last_dialed_at <= CURRENT_TIMESTAMP - (p_cooldown_hours || ' hours')::INTERVAL)
        ORDER BY l.id DESC
        LIMIT p_limit
        FOR UPDATE SKIP LOCKED
    ),
    updated_leads AS (
        UPDATE leads l
        SET status = 'DIALING',
            attempts_count = l.attempts_count + 1,
            dialed_at = CURRENT_TIMESTAMP,
            last_dialed_at = CURRENT_TIMESTAMP
        FROM selected_leads s
        WHERE l.id = s.id
        RETURNING l.id, l.campaign_id, l.tenant_id, l.cpf, l.phone, l.name, l.first_name, l.attempts_count
    )
    SELECT u.id, u.campaign_id, u.tenant_id, u.cpf, u.phone, u.name, u.first_name, u.attempts_count
    FROM updated_leads u;
END;
$$ LANGUAGE plpgsql;

-- 6.2. Persistência Atômica do Desfecho Preditivo, CDR, Lead e Receptivo O(1) (Pós-Execução)
CREATE OR REPLACE FUNCTION fn_audit_persist_predictive_result(
    p_cdr_id VARCHAR(64),
    p_tenant_id VARCHAR(64),
    p_campaign_id VARCHAR(64),
    p_phone VARCHAR(32),
    p_lead_id BIGINT,
    p_agent_id VARCHAR(64),
    p_disposition VARCHAR(32),
    p_sip_status INTEGER,
    p_hangup_cause INTEGER,
    p_duration_seconds INTEGER,
    p_billsec_seconds INTEGER,
    p_ring_seconds INTEGER,
    p_trunk_used VARCHAR(64),
    p_sip_route VARCHAR(128) DEFAULT NULL,
    p_max_attempts INTEGER DEFAULT 5,
    p_recording_file VARCHAR(512) DEFAULT NULL,
    p_recording_url VARCHAR(512) DEFAULT NULL,
    p_transcription TEXT DEFAULT NULL
) RETURNS JSONB AS $$
DECLARE
    v_target_lead_id BIGINT := p_lead_id;
    v_current_attempts INTEGER := 1;
    v_next_status VARCHAR(32);
    v_result JSONB;
BEGIN
    -- 1. Identifica o lead caso p_lead_id não tenha sido informado
    IF v_target_lead_id IS NULL THEN
        SELECT id, attempts_count INTO v_target_lead_id, v_current_attempts
        FROM leads
        WHERE campaign_id = p_campaign_id AND phone = p_phone
        ORDER BY id DESC
        LIMIT 1;
    ELSE
        SELECT attempts_count INTO v_current_attempts
        FROM leads
        WHERE id = v_target_lead_id;
    END IF;

    -- 2. Determina o próximo status do lead
    -- Se sucesso (DELIVERED, ANSWERED), número inválido ou estourou tentativas -> COMPLETED
    -- Se falha temporária (VOICEMAIL, AMD_MACHINE, NO_ANSWER, BUSY, FAILED) e < max_attempts -> QUEUED
    IF p_disposition IN ('DELIVERED', 'ANSWERED', 'INVALID_NUMBER') OR v_current_attempts >= p_max_attempts THEN
        v_next_status := 'COMPLETED';
    ELSE
        v_next_status := 'QUEUED';
    END IF;

    -- 3. Persiste o CDR detalhado
    INSERT INTO cdrs (
        id, tenant_id, campaign_id, phone, agent_id, call_type, disposition,
        sip_status, hangup_cause, duration_seconds, billsec_seconds, ring_seconds,
        trunk_used, recording_file, recording_url, transcription, created_at, initiated_at, answered_at, ended_at
    ) VALUES (
        p_cdr_id, p_tenant_id, p_campaign_id, p_phone, p_agent_id, 'PREDICTIVE', p_disposition,
        p_sip_status, p_hangup_cause, p_duration_seconds, p_billsec_seconds, p_ring_seconds,
        p_trunk_used, p_recording_file, p_recording_url, p_transcription, CURRENT_TIMESTAMP, 
        CURRENT_TIMESTAMP - (p_duration_seconds || ' seconds')::INTERVAL,
        CASE WHEN p_billsec_seconds > 0 THEN CURRENT_TIMESTAMP - (p_billsec_seconds || ' seconds')::INTERVAL ELSE NULL END,
        CURRENT_TIMESTAMP
    );

    -- 4. Atualiza a tupla do Lead
    IF v_target_lead_id IS NOT NULL THEN
        UPDATE leads
        SET status = v_next_status,
            last_dialed_at = CURRENT_TIMESTAMP
        WHERE id = v_target_lead_id;
    END IF;

    -- 5. Grava / Atualiza Mapeamento Receptivo Inteligente O(1)
    INSERT INTO phone_trunk_mappings (
        phone, last_trunk, last_project, last_sip_route, tenant_id, updated_at
    ) VALUES (
        p_phone, p_trunk_used, p_campaign_id, p_sip_route, p_tenant_id, CURRENT_TIMESTAMP
    )
    ON CONFLICT (phone) DO UPDATE SET
        last_trunk = EXCLUDED.last_trunk,
        last_project = EXCLUDED.last_project,
        last_sip_route = EXCLUDED.last_sip_route,
        tenant_id = EXCLUDED.tenant_id,
        updated_at = EXCLUDED.updated_at;

    -- 6. Retorna confirmação estruturada em JSON
    v_result := jsonb_build_object(
        'cdr_id', p_cdr_id,
        'lead_id', v_target_lead_id,
        'lead_status', v_next_status,
        'attempts_count', v_current_attempts,
        'phone', p_phone,
        'disposition', p_disposition,
        'receptive_mapping_updated', true
    );

    RETURN v_result;
END;
$$ LANGUAGE plpgsql;

-- 6.3. Reciclagem de Campanha com Incremento de Ciclo (Saturação)
CREATE OR REPLACE FUNCTION fn_audit_recycle_campaign_leads(
    p_campaign_id VARCHAR(64),
    p_tenant_id VARCHAR(64)
) RETURNS JSONB AS $$
DECLARE
    v_recycled_count INTEGER := 0;
    v_new_cycle INTEGER := 0;
    v_result JSONB;
BEGIN
    -- Reabre leads que não atingiram desfecho final 'COMPLETED'
    UPDATE leads
    SET status = 'QUEUED'
    WHERE campaign_id = p_campaign_id 
      AND tenant_id = p_tenant_id 
      AND status != 'COMPLETED';
    GET DIAGNOSTICS v_recycled_count = ROW_COUNT;

    -- Incrementa contador de ciclos e marca nível de saturação
    UPDATE campaigns
    SET cycle_count = cycle_count + 1,
        last_cycle_at = CURRENT_TIMESTAMP,
        saturation_level = 'RECICLADA'
    WHERE id = p_campaign_id 
      AND tenant_id = p_tenant_id
    RETURNING cycle_count INTO v_new_cycle;

    v_result := jsonb_build_object(
        'campaign_id', p_campaign_id,
        'recycled_leads', v_recycled_count,
        'new_cycle_count', v_new_cycle,
        'saturation_level', 'RECICLADA'
    );

    RETURN v_result;
END;
$$ LANGUAGE plpgsql;

-- ============================================================================
-- 7. TABELA DE RASTREAMENTO DE PROCESSOS COM CORRELATION_ID (TRACING)
-- ============================================================================

CREATE TABLE IF NOT EXISTS execution_traces (
    id BIGSERIAL PRIMARY KEY,
    correlation_id VARCHAR(64) NOT NULL,
    tenant_id VARCHAR(64) NOT NULL DEFAULT 'default',
    campaign_id VARCHAR(64),
    lead_id BIGINT,
    phone VARCHAR(32),
    agent_id VARCHAR(64),
    step VARCHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'SUCCESS',
    message TEXT NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_traces_correlation ON execution_traces(correlation_id);
CREATE INDEX IF NOT EXISTS idx_traces_campaign ON execution_traces(campaign_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_traces_lead ON execution_traces(lead_id);

-- 7.1. Stored Procedure para Registrar Passos do Fluxo Preditivo
CREATE OR REPLACE FUNCTION fn_audit_log_trace(
    p_correlation_id VARCHAR(64),
    p_step VARCHAR(64),
    p_status VARCHAR(32),
    p_message TEXT,
    p_tenant_id VARCHAR(64) DEFAULT 'default',
    p_campaign_id VARCHAR(64) DEFAULT NULL,
    p_lead_id BIGINT DEFAULT NULL,
    p_phone VARCHAR(32) DEFAULT NULL,
    p_agent_id VARCHAR(64) DEFAULT NULL,
    p_metadata JSONB DEFAULT '{}'::jsonb
) RETURNS BIGINT AS $$
DECLARE
    v_trace_id BIGINT;
BEGIN
    INSERT INTO execution_traces (
        correlation_id, step, status, message, tenant_id, campaign_id, lead_id, phone, agent_id, metadata, created_at
    ) VALUES (
        p_correlation_id, p_step, p_status, p_message, p_tenant_id, p_campaign_id, p_lead_id, p_phone, p_agent_id, p_metadata, CURRENT_TIMESTAMP
    ) RETURNING id INTO v_trace_id;

    RETURN v_trace_id;
END;
$$ LANGUAGE plpgsql;

-- 8. Tabela de Arquivos de Configuração Asterisk (sip_data)
CREATE TABLE IF NOT EXISTS sip_data (
    file VARCHAR(60) PRIMARY KEY,
    data TEXT NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);



