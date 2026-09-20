// ==============================================================================
// DIALER-GO MANAGER - CONTROLLER JAVASCRIPT (VANILLA JS ES6+)
// ==============================================================================

(function () {
  'use strict';

  const state = {
    tenantId: 'default',
    currentTab: 'tab-trunks',
    currentConfigFile: 'pjsip.conf',
    trunks: [],
    health: null,
    editingTrunkId: null
  };

  // --- DOM Elements ---
  const el = {
    modeBadge: document.getElementById('mode-badge'),
    chipAmi: document.getElementById('chip-ami'),
    chipDb: document.getElementById('chip-db'),
    chipCache: document.getElementById('chip-cache'),
    gaugeNumbers: document.getElementById('gauge-numbers'),
    gaugeBar: document.getElementById('gauge-bar'),
    tenantInput: document.getElementById('tenant-input'),
    btnRefresh: document.getElementById('btn-refresh'),
    
    // Stats
    statTotalTrunks: document.getElementById('stat-total-trunks'),
    statActiveTrunks: document.getElementById('stat-active-trunks'),
    statUsedChannels: document.getElementById('stat-used-channels'),
    statTotalChannels: document.getElementById('stat-total-channels'),
    
    // Trunks
    trunkSearch: document.getElementById('trunk-search'),
    trunksTbody: document.getElementById('trunks-tbody'),
    btnNewTrunk: document.getElementById('btn-new-trunk'),
    btnReloadTrunks: document.getElementById('btn-reload-trunks'),
    
    // Configs
    configFileTabs: document.getElementById('config-file-tabs'),
    configEditor: document.getElementById('config-editor'),
    btnSaveConfig: document.getElementById('btn-save-config'),
    btnApplyConfig: document.getElementById('btn-apply-config'),
    applyOutputCard: document.getElementById('apply-output-card'),
    applyOutputText: document.getElementById('apply-output-text'),
    btnCloseConsole: document.getElementById('btn-close-console'),
    
    // Telemetry
    teleMode: document.getElementById('tele-mode'),
    teleAmi: document.getElementById('tele-ami'),
    teleDb: document.getElementById('tele-db'),
    teleCache: document.getElementById('tele-cache'),
    teleActiveChans: document.getElementById('tele-active-chans'),
    teleHumanQuota: document.getElementById('tele-human-quota'),
    teleAvailChans: document.getElementById('tele-avail-chans'),
    
    // Modal
    trunkModal: document.getElementById('trunk-modal'),
    modalTitle: document.getElementById('modal-title'),
    trunkForm: document.getElementById('trunk-form'),
    btnCloseModal: document.getElementById('btn-close-modal'),
    btnCancelModal: document.getElementById('btn-cancel-modal'),
    
    // Modal Inputs
    inTrunkId: document.getElementById('trunk-id'),
    inTrunkName: document.getElementById('trunk-name'),
    inTrunkType: document.getElementById('trunk-type'),
    inTrunkMaxChans: document.getElementById('trunk-max-channels'),
    inTrunkHost: document.getElementById('trunk-host'),
    inTrunkPort: document.getElementById('trunk-port'),
    inTrunkUser: document.getElementById('trunk-username'),
    inTrunkSecret: document.getElementById('trunk-secret'),
    inTrunkPrefix: document.getElementById('trunk-tech-prefix'),
    inTrunkStatus: document.getElementById('trunk-status'),
    
    toastContainer: document.getElementById('toast-container')
  };

  // --- API Helper ---
  async function apiCall(endpoint, options = {}) {
    const defaultHeaders = {
      'Content-Type': 'application/json',
      'X-Tenant-Id': state.tenantId
    };
    options.headers = { ...defaultHeaders, ...(options.headers || {}) };

    try {
      const res = await fetch(endpoint, options);
      const data = await res.json().catch(() => ({}));
      return { ok: res.ok, status: res.status, data };
    } catch (err) {
      return { ok: false, status: 0, data: { detail: err.message } };
    }
  }

  // --- Toast Notifications ---
  function showToast(message, type = 'info') {
    const toast = document.createElement('div');
    toast.className = `toast toast-${type}`;
    toast.textContent = message;
    el.toastContainer.appendChild(toast);
    setTimeout(() => {
      toast.remove();
    }, 4000);
  }

  // --- Health & Telemetry ---
  async function fetchHealth() {
    const res = await apiCall('/health');
    if (!res.ok) {
      updateHealthUI(false, false, false, 0, 0, 0);
      return;
    }

    const d = res.data;
    state.health = d;
    
    const amiOK = d.components?.asterisk_ami || false;
    const dbOK = d.components?.database || false;
    const cacheOK = d.components?.cache || false;
    
    const active = d.telephony_capacity?.active_global_channels || 0;
    const max = d.telephony_capacity?.max_global_channels || 0;
    const human = d.telephony_capacity?.human_reserved_quota || 0;
    const avail = d.telephony_capacity?.available_channels || 0;

    updateHealthUI(amiOK, dbOK, cacheOK, active, max, human, avail);
  }

  function updateHealthUI(amiOK, dbOK, cacheOK, active, max, human, avail) {
    setChipStatus(el.chipAmi, amiOK);
    setChipStatus(el.chipDb, dbOK);
    setChipStatus(el.chipCache, cacheOK);

    el.gaugeNumbers.textContent = `${active} / ${max}`;
    const pct = max > 0 ? Math.min(100, Math.round((active / max) * 100)) : 0;
    el.gaugeBar.style.width = `${pct}%`;

    el.statUsedChannels.textContent = active;
    el.statTotalChannels.textContent = max;

    el.teleAmi.textContent = amiOK ? 'Conectado (Porta 5038)' : 'Desconectado';
    el.teleAmi.className = `info-val ${amiOK ? 'text-emerald' : 'text-rose'}`;

    el.teleDb.textContent = dbOK ? 'Conectado (PostgreSQL)' : 'Desconectado';
    el.teleDb.className = `info-val ${dbOK ? 'text-emerald' : 'text-rose'}`;

    el.teleCache.textContent = cacheOK ? 'Ativo (Keyspace Ex)' : 'Inativo / Dispensado';
    el.teleCache.className = `info-val ${cacheOK ? 'text-emerald' : 'text-faint'}`;

    el.teleActiveChans.textContent = active;
    el.teleHumanQuota.textContent = human;
    el.teleAvailChans.textContent = avail;
  }

  function setChipStatus(chip, isOnline) {
    const dot = chip.querySelector('.dot-indicator');
    if (isOnline) {
      dot.className = 'dot-indicator dot-online';
    } else {
      dot.className = 'dot-indicator dot-offline';
    }
  }

  // --- Trunks Management ---
  async function fetchTrunks() {
    const res = await apiCall('/api/v1/trunks');
    if (!res.ok) {
      el.trunksTbody.innerHTML = `<tr><td colspan="7" class="table-empty text-rose">Erro ao carregar troncos: ${res.data?.detail || 'Falha na requisição'}</td></tr>`;
      return;
    }

    const trunks = res.data?.data?.trunks || [];
    state.trunks = trunks;

    el.statTotalTrunks.textContent = trunks.length;
    const activeCount = trunks.filter(t => (t.Trunk?.Status || t.status) === 'ACTIVE').length;
    el.statActiveTrunks.textContent = activeCount;

    renderTrunks();
  }

  function renderTrunks() {
    const filter = (el.trunkSearch.value || '').toLowerCase().trim();
    const filtered = state.trunks.filter(item => {
      const t = item.Trunk || item;
      return (
        t.ID?.toLowerCase().includes(filter) ||
        t.Name?.toLowerCase().includes(filter) ||
        t.Host?.toLowerCase().includes(filter) ||
        t.Username?.toLowerCase().includes(filter)
      );
    });

    if (filtered.length === 0) {
      el.trunksTbody.innerHTML = '<tr><td colspan="7" class="table-empty">Nenhum tronco encontrado.</td></tr>';
      return;
    }

    el.trunksTbody.innerHTML = filtered.map(item => {
      const t = item.Trunk || item;
      const h = item.Health || {};
      const active = h.ActiveChannels || 0;
      const max = t.MaxChannels || h.MaxChannels || 30;
      const pct = max > 0 ? Math.min(100, Math.round((active / max) * 100)) : 0;
      const isSaturated = active >= max;
      const isOnline = (t.Status || 'ACTIVE') === 'ACTIVE';

      return `
        <tr>
          <td>
            <div style="font-weight: 600;">${escapeHtml(t.Name || t.ID)}</div>
            <div style="font-size: 0.76rem; color: var(--text-muted); font-family: var(--font-mono);">${escapeHtml(t.ID)}</div>
          </td>
          <td>
            <div style="font-family: var(--font-mono); font-weight: 500;">${escapeHtml(t.Username || '-')}</div>
            <div style="font-size: 0.74rem; color: var(--text-faint);">${escapeHtml(t.TechPrefix ? 'Prefix: ' + t.TechPrefix : 'Sem prefixo')}</div>
          </td>
          <td>
            <span class="badge ${t.TrunkType === 'INBOUND' ? 'badge-inactive' : 'badge-active'}">${escapeHtml(t.TrunkType || 'OUTBOUND')}</span>
          </td>
          <td style="font-family: var(--font-mono); font-size: 0.82rem;">
            ${escapeHtml(t.Host || 'metapabx.vivo.net.br')}:${t.Port || 5060}
          </td>
          <td>
            <div class="channel-progress">
              <div class="channel-track">
                <div class="channel-fill ${isSaturated ? 'saturated' : ''}" style="width: ${pct}%;"></div>
              </div>
              <span style="font-size: 0.78rem; font-family: var(--font-mono);">${active}/${max}</span>
            </div>
          </td>
          <td>
            <span class="badge ${isOnline ? 'badge-active' : 'badge-inactive'}">${isOnline ? 'ATIVO' : 'INATIVO'}</span>
          </td>
          <td class="text-right">
            <button class="btn btn-secondary btn-edit-trunk" data-id="${escapeHtml(t.ID)}" style="padding: 0.3rem 0.6rem; font-size: 0.78rem;">Editar</button>
            <button class="btn btn-danger btn-del-trunk" data-id="${escapeHtml(t.ID)}" style="padding: 0.3rem 0.6rem; font-size: 0.78rem; margin-left: 0.3rem;">Excluir</button>
          </td>
        </tr>
      `;
    }).join('');

    // Attach actions
    el.trunksTbody.querySelectorAll('.btn-edit-trunk').forEach(btn => {
      btn.addEventListener('click', () => openEditModal(btn.dataset.id));
    });

    el.trunksTbody.querySelectorAll('.btn-del-trunk').forEach(btn => {
      btn.addEventListener('click', () => deleteTrunk(btn.dataset.id));
    });
  }

  // --- Trunk Modals & CRUD ---
  function openNewModal() {
    state.editingTrunkId = null;
    el.modalTitle.textContent = 'Novo Tronco SIP';
    el.inTrunkId.disabled = false;
    el.inTrunkId.value = '';
    el.inTrunkName.value = '';
    el.inTrunkType.value = 'OUTBOUND';
    el.inTrunkMaxChans.value = '30';
    el.inTrunkHost.value = 'metapabx.vivo.net.br';
    el.inTrunkPort.value = '5060';
    el.inTrunkUser.value = '';
    el.inTrunkSecret.value = '';
    el.inTrunkPrefix.value = '';
    el.inTrunkStatus.value = 'ACTIVE';
    el.trunkModal.classList.remove('hidden');
  }

  function openEditModal(id) {
    const item = state.trunks.find(t => (t.Trunk?.ID || t.ID) === id);
    if (!item) return;
    const t = item.Trunk || item;

    state.editingTrunkId = t.ID;
    el.modalTitle.textContent = `Editar Tronco: ${t.ID}`;
    el.inTrunkId.disabled = true;
    el.inTrunkId.value = t.ID;
    el.inTrunkName.value = t.Name || '';
    el.inTrunkType.value = t.TrunkType || 'OUTBOUND';
    el.inTrunkMaxChans.value = t.MaxChannels || 30;
    el.inTrunkHost.value = t.Host || '';
    el.inTrunkPort.value = t.Port || 5060;
    el.inTrunkUser.value = t.Username || '';
    el.inTrunkSecret.value = t.Secret || '';
    el.inTrunkPrefix.value = t.TechPrefix || '';
    el.inTrunkStatus.value = t.Status || 'ACTIVE';
    el.trunkModal.classList.remove('hidden');
  }

  function closeModal() {
    el.trunkModal.classList.add('hidden');
  }

  async function handleTrunkSubmit(e) {
    e.preventDefault();
    const payload = {
      id: el.inTrunkId.value.trim(),
      tenant_id: state.tenantId || 'default',
      name: el.inTrunkName.value.trim(),
      trunk_type: el.inTrunkType.value,
      max_channels: parseInt(el.inTrunkMaxChans.value, 10) || 30,
      host: el.inTrunkHost.value.trim(),
      port: parseInt(el.inTrunkPort.value, 10) || 5060,
      username: el.inTrunkUser.value.trim(),
      secret: el.inTrunkSecret.value.trim(),
      tech_prefix: el.inTrunkPrefix.value.trim(),
      status: el.inTrunkStatus.value
    };

    let res;
    if (state.editingTrunkId) {
      res = await apiCall(`/api/v1/trunks/${encodeURIComponent(state.editingTrunkId)}`, {
        method: 'PUT',
        body: JSON.stringify(payload)
      });
    } else {
      res = await apiCall('/api/v1/trunks', {
        method: 'POST',
        body: JSON.stringify(payload)
      });
    }

    if (res.ok) {
      showToast('Tronco salvo com sucesso!', 'success');
      closeModal();
      fetchTrunks();
    } else {
      showToast(`Erro ao salvar tronco: ${res.data?.detail || 'Falha'}`, 'error');
    }
  }

  async function deleteTrunk(id) {
    if (!confirm(`Deseja realmente excluir o tronco "${id}"?`)) return;
    const res = await apiCall(`/api/v1/trunks/${encodeURIComponent(id)}`, { method: 'DELETE' });
    if (res.ok) {
      showToast('Tronco removido com sucesso!', 'success');
      fetchTrunks();
    } else {
      showToast(`Erro ao remover: ${res.data?.detail || 'Falha'}`, 'error');
    }
  }

  async function reloadTrunksAMI() {
    showToast('Enviando reload ao Asterisk...', 'info');
    const res = await apiCall('/api/v1/trunks/reload', { method: 'POST' });
    if (res.ok) {
      showToast('Troncos recarregados no Asterisk com sucesso!', 'success');
      fetchTrunks();
    } else {
      showToast(`Erro no reload: ${res.data?.detail || 'Falha'}`, 'error');
    }
  }

  // --- Config Editor (sip_data) ---
  async function loadConfigFile(filename) {
    state.currentConfigFile = filename;
    el.configEditor.value = 'Carregando...';

    // Update active tab button
    el.configFileTabs.querySelectorAll('.file-tab').forEach(tab => {
      tab.classList.toggle('active', tab.dataset.file === filename);
    });

    const res = await apiCall(`/api/v1/configs/${encodeURIComponent(filename)}`);
    if (res.ok && res.data?.data) {
      el.configEditor.value = res.data.data.data || '';
    } else {
      el.configEditor.value = `; Arquivo ${filename} vazio ou novo\n`;
    }
  }

  async function saveConfigFile() {
    const filename = state.currentConfigFile;
    const content = el.configEditor.value;

    showToast(`Salvando ${filename}...`, 'info');
    const res = await apiCall(`/api/v1/configs/${encodeURIComponent(filename)}`, {
      method: 'POST',
      body: JSON.stringify({
        file: filename,
        data: content
      })
    });

    if (res.ok) {
      showToast(`Arquivo ${filename} salvo no banco com sucesso!`, 'success');
    } else {
      showToast(`Erro ao salvar: ${res.data?.detail || 'Falha'}`, 'error');
    }
  }

  async function applyConfigToAsterisk() {
    if (!confirm('Deseja gravar os arquivos no disco e recarregar o Asterisk via AMI?')) return;

    showToast('Aplicando alterações no Asterisk...', 'info');
    const res = await apiCall('/api/v1/configs/apply?reload_ami=true', {
      method: 'POST'
    });

    el.applyOutputCard.classList.remove('hidden');
    if (res.ok && res.data?.data) {
      const d = res.data.data;
      const text = [
        `Arquivos Gravados em Disco: ${(d.applied_files || []).join(', ')}`,
        '--- Resposta do Reload AMI ---',
        ...(d.reload_results || [])
      ].join('\n');
      el.applyOutputText.textContent = text;
      showToast('Configurações aplicadas com sucesso no Asterisk!', 'success');
      fetchHealth();
    } else {
      el.applyOutputText.textContent = `Erro ao aplicar: ${res.data?.detail || JSON.stringify(res.data)}`;
      showToast('Falha ao aplicar configurações no Asterisk.', 'error');
    }
  }

  // --- Utilities ---
  function escapeHtml(str) {
    if (!str) return '';
    return String(str)
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;');
  }

  // --- Tab Switching ---
  function setupTabs() {
    document.querySelectorAll('.tab-btn').forEach(btn => {
      btn.addEventListener('click', () => {
        const tabId = btn.dataset.tab;
        state.currentTab = tabId;

        document.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
        document.querySelectorAll('.tab-content').forEach(c => c.classList.remove('active'));

        btn.classList.add('active');
        const target = document.getElementById(tabId);
        if (target) target.classList.add('active');

        if (tabId === 'tab-configs') {
          loadConfigFile(state.currentConfigFile);
        } else if (tabId === 'tab-trunks') {
          fetchTrunks();
        }
      });
    });

    el.configFileTabs.querySelectorAll('.file-tab').forEach(tab => {
      tab.addEventListener('click', () => {
        loadConfigFile(tab.dataset.file);
      });
    });
  }

  // --- Event Listeners Initialization ---
  function initEvents() {
    el.tenantInput.addEventListener('change', () => {
      state.tenantId = el.tenantInput.value.trim() || 'default';
      showToast(`Tenant alterado para: ${state.tenantId}`, 'info');
      fetchTrunks();
    });

    el.btnRefresh.addEventListener('click', () => {
      fetchHealth();
      fetchTrunks();
      showToast('Dados atualizados!', 'info');
    });

    el.trunkSearch.addEventListener('input', renderTrunks);
    el.btnNewTrunk.addEventListener('click', openNewModal);
    el.btnReloadTrunks.addEventListener('click', reloadTrunksAMI);

    el.btnCloseModal.addEventListener('click', closeModal);
    el.btnCancelModal.addEventListener('click', closeModal);
    el.trunkForm.addEventListener('submit', handleTrunkSubmit);

    el.btnSaveConfig.addEventListener('click', saveConfigFile);
    el.btnApplyConfig.addEventListener('click', applyConfigToAsterisk);
    el.btnCloseConsole.addEventListener('click', () => el.applyOutputCard.classList.add('hidden'));
  }

  // --- Boot Application ---
  function init() {
    setupTabs();
    initEvents();

    fetchHealth();
    fetchTrunks();

    // Auto-refresh health every 5 seconds
    setInterval(fetchHealth, 5000);
  }

  document.addEventListener('DOMContentLoaded', init);
})();
