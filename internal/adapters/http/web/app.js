// ==============================================================================
// DIALER-GO PLATFORM CONTROLLER - CORE APPLICATION (ES6+ VANILLA JS)
// Governed by: .agents/skills/platform-design-system/SKILL.md
// ==============================================================================

window.App = (function () {
  'use strict';

  const state = {
    tenantId: 'default',
    currentTab: 'tab-configs',
    currentConfigFile: 'pjsip.conf',
    trunks: []
  };

  const $ = (id) => document.getElementById(id);
  const $$ = (sel) => document.querySelectorAll(sel);

  const el = {
    tenantInput: $('tenant-input'),
    btnRefresh: $('btn-refresh'),
    gaugeNumbers: $('gauge-numbers'),
    gaugeBar: $('gauge-bar'),
    chipAmi: $('chip-ami'),
    chipDb: $('chip-db'),
    chipCache: $('chip-cache'),
    modeBadge: $('mode-badge'),

    // Troncos
    trunksTbody: $('trunks-tbody'),
    trunkSearch: $('trunk-search'),
    btnReloadTrunks: $('btn-reload-trunks'),
    btnNewTrunk: $('btn-new-trunk'),
    statTotalTrunks: $('stat-total-trunks'),
    statActiveTrunks: $('stat-active-trunks'),
    statUsedChannels: $('stat-used-channels'),
    statTotalChannels: $('stat-total-channels'),
    trunkModal: $('trunk-modal'),
    btnCloseModal: $('btn-close-modal'),
    btnCancelModal: $('btn-cancel-modal'),
    trunkForm: $('trunk-form'),

    // Configs
    configFileTabs: $('config-file-tabs'),
    configEditor: $('config-editor'),
    btnSaveConfig: $('btn-save-config'),
    btnApplyConfig: $('btn-apply-config'),
    applyOutputCard: $('apply-output-card'),
    applyOutputText: $('apply-output-text'),
    btnCloseConsole: $('btn-close-console'),

    // Telemetria
    teleMode: $('tele-mode'),
    teleAmi: $('tele-ami'),
    teleDb: $('tele-db'),
    teleCache: $('tele-cache'),
    teleActiveChans: $('tele-active-chans'),
    teleHumanQuota: $('tele-human-quota'),
    teleAvailChans: $('tele-avail-chans'),

    toastContainer: $('toast-container')
  };

  async function apiCall(endpoint, options = {}) {
    const headers = {
      'Content-Type': 'application/json',
      'X-Tenant-Id': state.tenantId,
      ...(options.headers || {})
    };
    try {
      const res = await fetch(endpoint, { ...options, headers });
      const data = await res.json().catch(() => ({}));
      return { ok: res.ok, status: res.status, data };
    } catch (err) {
      return { ok: false, status: 0, data: { detail: err.message } };
    }
  }

  function showToast(msg, type = 'info') {
    const toast = document.createElement('div');
    toast.className = `toast toast-${type}`;
    toast.textContent = msg;
    el.toastContainer.appendChild(toast);
    setTimeout(() => toast.remove(), 4000);
  }

  // --- Health & Indicadores Globais ---
  async function fetchHealth() {
    const res = await apiCall('/health');
    if (!res.ok) {
      updateHealthUI(false, false, false, 0, 0, 0, 0);
      return;
    }
    const d = res.data;
    const ami = d.components?.asterisk_ami || false;
    const db = d.components?.database || false;
    const cache = d.components?.cache || false;
    const active = d.telephony_capacity?.active_global_channels || 0;
    const max = d.telephony_capacity?.max_global_channels || 0;
    const human = d.telephony_capacity?.human_reserved_quota || 0;
    const avail = d.telephony_capacity?.available_channels || 0;
    updateHealthUI(ami, db, cache, active, max, human, avail);
  }

  function updateHealthUI(ami, db, cache, active, max, human, avail) {
    setIndicator(el.chipAmi, ami);
    setIndicator(el.chipDb, db);
    setIndicator(el.chipCache, cache);

    el.gaugeNumbers.textContent = `${active} / ${max}`;
    const pct = max > 0 ? Math.min(100, Math.round((active / max) * 100)) : 0;
    el.gaugeBar.style.width = `${pct}%`;

    el.statUsedChannels.textContent = active;
    el.statTotalChannels.textContent = max;

    el.teleAmi.textContent = ami ? 'Conectado' : 'Desconectado';
    el.teleDb.textContent = db ? 'Conectado' : 'Desconectado';
    el.teleCache.textContent = cache ? 'Ativo' : 'Dispensado';
    el.teleActiveChans.textContent = active;
    el.teleHumanQuota.textContent = human;
    el.teleAvailChans.textContent = avail;
  }

  function setIndicator(chip, ok) {
    const dot = chip.querySelector('.status-dot');
    if (dot) {
      dot.className = `status-dot status-badge ${ok ? 'online pulse' : 'offline'}`;
    }
  }

  // --- Troncos ---
  async function fetchTrunks() {
    const res = await apiCall('/api/v1/trunks');
    if (!res.ok) return;
    state.trunks = res.data?.data?.trunks || [];
    el.statTotalTrunks.textContent = state.trunks.length;
    el.statActiveTrunks.textContent = state.trunks.filter(t => t.is_enabled && t.health?.status === 'ONLINE').length;
    el.statUsedChannels.textContent = state.trunks.reduce((acc, t) => acc + (t.health?.active_channels || 0), 0);
    el.statTotalChannels.textContent = state.trunks.reduce((acc, t) => acc + (t.max_channels || 0), 0);
    renderTrunks();
  }

  function renderTrunks() {
    const filter = (el.trunkSearch.value || '').toLowerCase().trim();
    const list = state.trunks.filter(t => {
      return (t.id || '').toLowerCase().includes(filter) ||
             (t.name || '').toLowerCase().includes(filter) ||
             (t.host || '').toLowerCase().includes(filter);
    });
    if (list.length === 0) {
      el.trunksTbody.innerHTML = `<tr><td colspan="7" style="text-align: center; color: var(--text-stone-500); padding: 2rem;">Nenhum tronco encontrado.</td></tr>`;
      return;
    }
    el.trunksTbody.innerHTML = list.map(t => {
      const activeCh = t.health?.active_channels || 0;
      const maxCh = t.max_channels || 30;
      const callerId = t.tech_prefix || t.from_user || t.auth_username || '-';
      const transport = t.transport || 'UDP';
      const isOnline = t.health?.status === 'ONLINE';
      const latency = t.health?.latency_ms ? `${Math.round(t.health.latency_ms)}ms` : '';

      let statusHtml = '';
      if (!t.is_enabled) {
        statusHtml = `<span class="status-badge neutral"><span class="status-dot" style="background-color: var(--text-stone-500);"></span><span>Desativado</span></span>`;
      } else if (isOnline) {
        statusHtml = `<span class="status-badge online"><span class="status-dot pulse" style="background-color: var(--emerald-400);"></span><span>Online ${latency}</span></span>`;
      } else {
        statusHtml = `<span class="status-badge offline"><span class="status-dot" style="background-color: var(--rose-400);"></span><span>${t.health?.status || 'Offline'}</span></span>`;
      }

      return `
        <tr>
          <td>
            <strong style="color: var(--text-stone-100); font-family: var(--font-display);">${t.name || t.id}</strong><br>
            <span style="font-size: 0.6875rem; color: var(--text-stone-500); font-family: var(--font-mono);">${t.id}</span>
          </td>
          <td>${callerId}</td>
          <td><span class="mode-pill dispatcher" style="font-size: 0.5625rem;">${transport}</span></td>
          <td><code style="font-family: var(--font-mono); font-size: 0.75rem; color: var(--text-stone-300);">${t.host}:${t.port || 5060}</code></td>
          <td><span style="font-family: var(--font-mono);">${activeCh} / ${maxCh}</span></td>
          <td>${statusHtml}</td>
          <td style="text-align: right;">
            <button class="btn-ghost" data-del-trunk="${t.id}" title="Excluir tronco">
              <i class="pi pi-trash" style="color: var(--text-stone-400);"></i>
            </button>
          </td>
        </tr>
      `;
    }).join('');

    $$('[data-del-trunk]').forEach(btn => {
      btn.onclick = async () => {
        const trunkId = btn.getAttribute('data-del-trunk');
        const res = await apiCall(`/api/v1/trunks/${trunkId}`, { method: 'DELETE' });
        if (res.ok) {
          showToast('Tronco removido', 'success');
          fetchTrunks();
        } else {
          showToast(`Erro ao remover: ${res.data?.detail || ''}`, 'error');
        }
      };
    });
  }

  // --- Configurações PBX ---
  async function fetchConfigFile(fileName) {
    state.currentConfigFile = fileName;
    el.configEditor.value = 'Carregando...';
    const res = await apiCall(`/api/v1/configs/${fileName}`);
    if (res.ok) {
      el.configEditor.value = res.data?.data || '';
    } else {
      el.configEditor.value = '; Arquivo não encontrado ou vazio';
    }
  }

  async function saveConfigFile() {
    showToast('Salvando...', 'info');
    const content = el.configEditor.value;
    const res = await apiCall(`/api/v1/configs/${state.currentConfigFile}`, {
      method: 'POST',
      body: JSON.stringify({ file: state.currentConfigFile, data: content })
    });
    if (res.ok) {
      showToast('Arquivo salvo', 'success');
    } else {
      showToast('Erro ao salvar', 'error');
    }
  }

  async function applyConfigs() {
    showToast('Aplicando...', 'info');
    const res = await apiCall('/api/v1/configs/apply', { method: 'POST' });
    el.applyOutputCard.style.display = 'block';
    if (res.ok) {
      el.applyOutputText.textContent = res.data?.message || 'Configurações aplicadas com sucesso.';
      showToast('Configurações aplicadas', 'success');
    } else {
      el.applyOutputText.textContent = res.data?.detail || 'Erro ao aplicar configurações.';
      showToast('Erro ao aplicar', 'error');
    }
  }

  function initTabs() {
    $$('.tab-btn').forEach(btn => {
      btn.onclick = () => {
        $$('.tab-btn').forEach(b => b.classList.remove('active'));
        $$('.tab-content').forEach(c => c.classList.remove('active'));
        btn.classList.add('active');
        const target = btn.getAttribute('data-tab');
        $(target).classList.add('active');
        if (target === 'tab-configs') fetchConfigFile(state.currentConfigFile);
      };
    });

    $$('#config-file-tabs .file-tab').forEach(tab => {
      tab.onclick = () => {
        $$('#config-file-tabs .file-tab').forEach(t => t.classList.remove('active'));
        tab.classList.add('active');
        fetchConfigFile(tab.getAttribute('data-file'));
      };
    });
  }

  function init() {
    document.addEventListener('click', () => {
      $$('.dropdown-menu').forEach(m => m.classList.remove('show'));
    });

    initTabs();
    fetchConfigFile(state.currentConfigFile);

    el.trunkSearch.oninput = renderTrunks;
    el.btnReloadTrunks.onclick = () => {
      apiCall('/api/v1/trunks/reload', { method: 'POST' }).then(() => {
        showToast('Troncos recarregados', 'success');
        fetchTrunks();
      });
    };

    el.btnRefresh.onclick = () => {
      fetchHealth();
      fetchTrunks();
      if (window.InstancesModule) window.InstancesModule.fetch();
    };

    el.tenantInput.onchange = (e) => {
      state.tenantId = e.target.value.trim() || 'default';
      fetchHealth();
      fetchTrunks();
      if (window.InstancesModule) window.InstancesModule.fetch();
    };

    el.btnNewTrunk.onclick = () => { el.trunkModal.style.display = 'flex'; };
    el.btnCloseModal.onclick = () => { el.trunkModal.style.display = 'none'; };
    el.btnCancelModal.onclick = () => { el.trunkModal.style.display = 'none'; };

    el.trunkForm.onsubmit = async (e) => {
      e.preventDefault();
      const payload = {
        id: $('trunk-id').value.trim(),
        name: $('trunk-name').value.trim(),
        host: $('trunk-host').value.trim(),
        port: parseInt($('trunk-port').value, 10) || 5060,
        max_channels: parseInt($('trunk-max-channels').value, 10) || 30,
        direction: 'BIDIRECTIONAL',
        registration_mode: 'IP_BASED',
        transport: 'UDP',
        is_enabled: true
      };
      const res = await apiCall('/api/v1/trunks', {
        method: 'POST',
        body: JSON.stringify(payload)
      });
      if (res.ok) {
        showToast('Tronco salvo', 'success');
        el.trunkModal.style.display = 'none';
        el.trunkForm.reset();
        fetchTrunks();
      } else {
        showToast(`Erro ao salvar: ${res.data?.detail || ''}`, 'error');
      }
    };

    el.btnSaveConfig.onclick = saveConfigFile;
    el.btnApplyConfig.onclick = applyConfigs;
    el.btnCloseConsole.onclick = () => { el.applyOutputCard.style.display = 'none'; };

    if (window.InstancesModule) {
      window.InstancesModule.init();
      window.InstancesModule.fetch();
    }

    fetchHealth();
    fetchTrunks();
    setInterval(fetchHealth, 10000);
  }

  window.addEventListener('DOMContentLoaded', init);

  return {
    apiCall,
    showToast,
    getState: () => state
  };
})();
