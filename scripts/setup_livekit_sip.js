#!/usr/bin/env node

/**
 * Script Declarativo de Governança e Auto-Provisionamento LiveKit SIP
 * 
 * Uso:
 *   node scripts/setup_livekit_sip.js --audit
 *   node scripts/setup_livekit_sip.js --provision
 * 
 * Variáveis de Ambiente Opcionais:
 *   LIVEKIT_URL=https://live.creditobr.org
 *   LIVEKIT_API_KEY=devkey
 *   LIVEKIT_API_SECRET=secret
 */

const crypto = require('crypto');

const LIVEKIT_URL = (process.env.LIVEKIT_URL || 'https://live.creditobr.org').replace(/\/+$/, '');
const API_KEY = process.env.LIVEKIT_API_KEY || 'devkey12312312312';
const API_SECRET = process.env.LIVEKIT_API_SECRET || 'secret12312312312';

function base64UrlEncode(str) {
  return Buffer.from(str)
    .toString('base64')
    .replace(/=/g, '')
    .replace(/\+/g, '-')
    .replace(/\//g, '_');
}

function generateToken() {
  const header = { alg: 'HS256', typ: 'JWT' };
  const now = Math.floor(Date.now() / 1000);
  const payload = {
    exp: now + 3600,
    iss: API_KEY,
    nbf: now - 10,
    sub: API_KEY,
    video: {
      roomAdmin: true,
      roomCreate: true,
      roomList: true
    },
    sip: {
      admin: true,
      call: true
    }
  };

  const encHeader = base64UrlEncode(JSON.stringify(header));
  const encPayload = base64UrlEncode(JSON.stringify(payload));
  const signature = crypto
    .createHmac('sha256', API_SECRET)
    .update(`${encHeader}.${encPayload}`)
    .digest('base64')
    .replace(/=/g, '')
    .replace(/\+/g, '-')
    .replace(/\//g, '_');

  return `${encHeader}.${encPayload}.${signature}`;
}

async function postTwirp(endpoint, body = {}) {
  const token = generateToken();
  const res = await fetch(`${LIVEKIT_URL}${endpoint}`, {
    method: 'POST',
    headers: {
      'Authorization': `Bearer ${token}`,
      'Content-Type': 'application/json'
    },
    body: JSON.stringify(body)
  });

  const text = await res.text();
  if (!res.ok) {
    throw new Error(`Twirp ${endpoint} falhou (${res.status}): ${text}`);
  }
  return text ? JSON.parse(text) : {};
}

async function run() {
  const isAudit = process.argv.includes('--audit');
  console.log(`[LIVEKIT-SIP-SETUP] Conectando a: ${LIVEKIT_URL} (Key: ${API_KEY})`);

  // 1. Audita Troncos Inbound
  const trunks = await postTwirp('/twirp/livekit.SIP/ListSIPInboundTrunk');
  const trunkItems = trunks.items || [];
  console.log(`[STATUS] Troncos Inbound cadastrados: ${trunkItems.length}`);
  trunkItems.forEach(t => console.log(`  - Trunk ID: ${t.sip_trunk_id} | Nome: ${t.name}`));

  // 2. Audita Regras de Despacho
  const rules = await postTwirp('/twirp/livekit.SIP/ListSIPDispatchRule');
  const ruleItems = rules.items || [];
  console.log(`[STATUS] Regras de Despacho cadastradas: ${ruleItems.length}`);
  ruleItems.forEach(r => console.log(`  - Rule ID: ${r.sip_dispatch_rule_id} | Nome: ${r.name}`));

  if (isAudit) {
    if (trunkItems.length === 0 || ruleItems.length === 0) {
      console.error(`[FALHA] Auditoria: ponte SIP incompleta! Faltam troncos ou regras.`);
      process.exit(1);
    }
    console.log(`[SUCESSO] Auditoria: ponte SIP 100% integra.`);
    return;
  }

  // Provisionamento idempotente
  let trunkId = trunkItems.length > 0 ? trunkItems[0].sip_trunk_id : null;
  if (!trunkId) {
    console.log(`[PROVISION] Criando Inbound Trunk 'Asterisk-PBX'...`);
    const created = await postTwirp('/twirp/livekit.SIP/CreateSIPInboundTrunk', {
      trunk: {
        name: 'Asterisk-PBX',
        allowed_addresses: ['0.0.0.0/0']
      }
    });
    trunkId = created.sip_trunk_id;
    console.log(`[PROVISION] Inbound Trunk criado: ${trunkId}`);
  }

  if (ruleItems.length === 0) {
    console.log(`[PROVISION] Criando Dispatch Rule 'Asterisk Room Dispatch'...`);
    const created = await postTwirp('/twirp/livekit.SIP/CreateSIPDispatchRule', {
      rule: {
        dispatch_rule_callee: {
          room_prefix: '',
          randomize: false
        }
      },
      name: 'Asterisk Room Dispatch',
      trunk_ids: [trunkId]
    });
    console.log(`[PROVISION] Dispatch Rule criada: ${created.sip_dispatch_rule_id}`);
  }

  console.log(`[CONCLUIDO] Ponte LiveKit SIP devidamente provisionada e pronta.`);
}

run().catch(err => {
  console.error(`[ERRO] ${err.message}`);
  process.exit(1);
});
