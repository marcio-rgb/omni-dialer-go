// ==============================================================================
// DIALER-GO PLATFORM - INSTANCES MODULE (WARM DARK MODE)
// Governed by: .agents/skills/platform-design-system/SKILL.md
// ==============================================================================

window.InstancesModule = (function () {
  'use strict';

  const $ = (id) => document.getElementById(id);
  const $$ = (sel) => document.querySelectorAll(sel);

  let state = {
    instances: [],
    instanceFilter: 'all',
    pings: {},
    editingInstanceId: null
  };

  const el = {
    instancesGrid: $('instances-grid'),
    instanceSearch: $('instance-search'),
    statTotalInst: $('stat-total-instances'),
    statDialerInst: $('stat-dialer-instances'),
    statDispInst: $('stat-dispatcher-instances'),
    statInstChans: $('stat-instances-channels'),
    btnPingAll: $('btn-ping-all'),
    btnNewInst: $('btn-new-instance'),
    drawerOverlay: $('instance-drawer-overlay'),
    drawerPanel: $('instance-drawer-panel'),
    drawerTitle: $('drawer-title'),
    btnCloseDrawer: $('btn-close-drawer'),
    btnCancelDrawer: $('btn-cancel-drawer'),
    instForm: $('instance-form'),
    instId: $('inst-id'),
    instName: $('inst-name'),
    instMode: $('inst-mode'),
    btnModeDialer: $('btn-mode-dialer'),
    btnModeDisp: $('btn-mode-dispatcher'),
    instHostUrl: $('inst-host-url'),
    instApiKey: $('inst-api-key'),
    instMaxChannels: $('inst-max-channels'),
    instDesc: $('inst-desc'),
    instIsActive: $('inst-is-active')
  };

  async function fetchInstances() {
    const res = await window.App.apiCall('/api/v1/instances');
    if (!res.ok) {
      el.instancesGrid.innerHTML = `<div style="grid-column: 1/-1; padding: 2rem; text-align: center; color: var(--rose-400);">Erro ao carregar instâncias: ${res.data?.detail || ''}</div>`;
      return;
    }
    state.instances = res.data?.instances || [];
    updateStats();
    render();
  }

  function updateStats() {
    const total = state.instances.length;
    const dialers = state.instances.filter(i => i.mode === 'dialer').length;
    const dispatchers = state.instances.filter(i => i.mode === 'dispatcher').length;
    const channels = state.instances.reduce((acc, i) => acc + (i.max_channels || 0), 0);

    el.statTotalInst.textContent = total;
    el.statDialerInst.textContent = dialers;
    el.statDispInst.textContent = dispatchers;
    el.statInstChans.textContent = channels;
  }

  function render() {
    const filter = (el.instanceSearch.value || '').toLowerCase().trim();
    const modeFilter = state.instanceFilter;

    const list = state.instances.filter(i => {
      const matchMode = modeFilter === 'all' || i.mode === modeFilter;
      const matchText = (i.name || '').toLowerCase().includes(filter) ||
                        (i.id || '').toLowerCase().includes(filter) ||
                        (i.host_url || '').toLowerCase().includes(filter);
      return matchMode && matchText;
    });

    if (list.length === 0) {
      el.instancesGrid.innerHTML = `<div style="grid-column: 1/-1; padding: 3rem; text-align: center; color: var(--text-stone-500);">Nenhuma instância encontrada.</div>`;
      return;
    }

    el.instancesGrid.innerHTML = list.map(inst => {
      const id = inst.id || inst.ID;
      const ping = state.pings[id] || null;
      const isOnline = ping ? ping.status === 'online' : (inst.is_active !== false);
      const latencyText = ping ? `${ping.latency_ms} ms` : '-';
      const activeChans = ping ? ping.active_channels : 0;
      const maxChans = ping && ping.max_channels ? ping.max_channels : (inst.max_channels || 30);
      const isDialer = inst.mode === 'dialer';
      const modeLabel = isDialer ? 'Discador' : 'Distribuidor';
      const modeClass = isDialer ? 'dialer' : 'dispatcher';

      return `
        <div class="instance-card" data-id="${id}">
          <div class="card-header-row">
            <div class="card-title-group">
              <span class="card-title" title="${inst.name}">${inst.name}</span>
              <span class="mode-pill ${modeClass}">${modeLabel}</span>
            </div>
            <div style="display: flex; align-items: center; gap: 0.5rem;">
              <span class="status-badge ${isOnline ? 'online' : 'offline'}">
                <span class="status-dot ${isOnline ? 'pulse' : ''}" style="background-color: ${isOnline ? 'var(--emerald-400)' : 'var(--rose-400)'};"></span>
                <span>${isOnline ? 'Online' : 'Offline'}</span>
              </span>
              <div class="dropdown-container">
                <button class="btn-ghost" data-action="toggle-menu" title="Ações">
                  <i class="pi pi-ellipsis-v"></i>
                </button>
                <div class="dropdown-menu">
                  <button class="dropdown-item" data-action="ping-inst" data-id="${id}">
                    <i class="pi pi-sync"></i>
                    <span>Verificar</span>
                  </button>
                  <button class="dropdown-item" data-action="edit-inst" data-id="${id}">
                    <i class="pi pi-pencil"></i>
                    <span>Editar</span>
                  </button>
                  <button class="dropdown-item danger" data-action="delete-inst" data-id="${id}">
                    <i class="pi pi-trash"></i>
                    <span>Excluir</span>
                  </button>
                </div>
              </div>
            </div>
          </div>

          <div class="card-meta-row">
            <span class="card-host-text" title="Host">${inst.host_url || '-'}</span>
            <div class="metrics-group-right">
              <span class="metric-pill" title="Canais em Uso">
                <i class="pi pi-phone"></i>
                <span>${activeChans} / ${maxChans}</span>
              </span>
              <span class="metric-pill" title="Latência HTTP">
                <i class="pi pi-bolt"></i>
                <span>${latencyText}</span>
              </span>
            </div>
          </div>
          ${inst.description ? `<p style="font-size: 0.6875rem; color: var(--text-stone-400); margin-top: -0.25rem;">${inst.description}</p>` : ''}
        </div>
      `;
    }).join('');

    attachListeners();
  }

  function attachListeners() {
    $$('.instance-card').forEach(card => {
      const id = card.getAttribute('data-id');
      const menuBtn = card.querySelector('[data-action="toggle-menu"]');
      const menu = card.querySelector('.dropdown-menu');

      if (menuBtn && menu) {
        menuBtn.onclick = (e) => {
          e.stopPropagation();
          $$('.dropdown-menu').forEach(m => { if (m !== menu) m.classList.remove('show'); });
          menu.classList.toggle('show');
        };
      }

      const pingBtn = card.querySelector('[data-action="ping-inst"]');
      if (pingBtn) pingBtn.onclick = () => ping(id);

      const editBtn = card.querySelector('[data-action="edit-inst"]');
      if (editBtn) editBtn.onclick = () => openDrawer(id);

      const delBtn = card.querySelector('[data-action="delete-inst"]');
      if (delBtn) delBtn.onclick = () => remove(id);
    });
  }

  async function ping(id) {
    window.App.showToast('Verificando...', 'info');
    const res = await window.App.apiCall(`/api/v1/instances/${id}/ping`, { method: 'POST' });
    if (res.ok) {
      state.pings[id] = res.data;
      window.App.showToast(`Latência: ${res.data.latency_ms}ms`, 'success');
    } else {
      state.pings[id] = { status: 'offline', latency_ms: 0, active_channels: 0 };
      window.App.showToast('Nó inacessível', 'error');
    }
    render();
  }

  async function pingAll() {
    window.App.showToast('Verificando nós...', 'info');
    const ids = state.instances.map(i => i.id || i.ID);
    await Promise.all(ids.map(id => window.App.apiCall(`/api/v1/instances/${id}/ping`, { method: 'POST' }).then(res => {
      if (res.ok) state.pings[id] = res.data;
      else state.pings[id] = { status: 'offline', latency_ms: 0 };
    })));
    window.App.showToast('Verificação concluída', 'success');
    render();
  }

  function openDrawer(id = null) {
    state.editingInstanceId = id;
    if (id) {
      const inst = state.instances.find(i => (i.id || i.ID) === id);
      if (!inst) return;
      el.drawerTitle.textContent = 'Editar Instância';
      el.instId.value = inst.id || inst.ID;
      el.instId.disabled = true;
      el.instName.value = inst.name || '';
      setMode(inst.mode || 'dialer');
      el.instHostUrl.value = inst.host_url || '';
      el.instApiKey.value = inst.api_key || '';
      el.instMaxChannels.value = inst.max_channels || 30;
      el.instDesc.value = inst.description || '';
      el.instIsActive.checked = inst.is_active !== false;
    } else {
      el.drawerTitle.textContent = 'Nova Instância';
      el.instForm.reset();
      el.instId.disabled = false;
      setMode('dialer');
      el.instMaxChannels.value = 30;
      el.instIsActive.checked = true;
    }
    el.drawerOverlay.classList.add('open');
    el.drawerPanel.classList.add('open');
  }

  function closeDrawer() {
    el.drawerOverlay.classList.remove('open');
    el.drawerPanel.classList.remove('open');
    state.editingInstanceId = null;
  }

  function setMode(mode) {
    el.instMode.value = mode;
    if (mode === 'dialer') {
      el.btnModeDialer.classList.add('active');
      el.btnModeDisp.classList.remove('active');
    } else {
      el.btnModeDisp.classList.add('active');
      el.btnModeDialer.classList.remove('active');
    }
  }

  async function handleFormSubmit(e) {
    e.preventDefault();
    const payload = {
      name: el.instName.value.trim(),
      mode: el.instMode.value,
      host_url: el.instHostUrl.value.trim(),
      api_key: el.instApiKey.value.trim(),
      max_channels: parseInt(el.instMaxChannels.value, 10) || 30,
      description: el.instDesc.value.trim(),
      is_active: el.instIsActive.checked
    };

    if (state.editingInstanceId) {
      const res = await window.App.apiCall(`/api/v1/instances/${state.editingInstanceId}`, {
        method: 'PUT',
        body: JSON.stringify(payload)
      });
      if (!res.ok) {
        window.App.showToast(`Erro ao atualizar: ${res.data?.detail || ''}`, 'error');
        return;
      }
      window.App.showToast('Instância atualizada', 'success');
    } else {
      payload.id = el.instId.value.trim();
      const res = await window.App.apiCall('/api/v1/instances', {
        method: 'POST',
        body: JSON.stringify(payload)
      });
      if (!res.ok) {
        window.App.showToast(`Erro ao cadastrar: ${res.data?.detail || ''}`, 'error');
        return;
      }
      window.App.showToast('Instância cadastrada', 'success');
    }

    closeDrawer();
    fetchInstances();
  }

  async function remove(id) {
    const res = await window.App.apiCall(`/api/v1/instances/${id}`, { method: 'DELETE' });
    if (!res.ok) {
      window.App.showToast(`Erro ao excluir: ${res.data?.detail || ''}`, 'error');
      return;
    }
    window.App.showToast('Instância removida', 'success');
    fetchInstances();
  }

  function init() {
    el.instanceSearch.oninput = render;
    el.btnPingAll.onclick = pingAll;
    el.btnNewInst.onclick = () => openDrawer();
    el.btnCloseDrawer.onclick = closeDrawer;
    el.btnCancelDrawer.onclick = closeDrawer;
    el.drawerOverlay.onclick = closeDrawer;
    el.btnModeDialer.onclick = () => setMode('dialer');
    el.btnModeDisp.onclick = () => setMode('dispatcher');
    el.instForm.onsubmit = handleFormSubmit;

    $$('.filter-chip').forEach(chip => {
      chip.onclick = () => {
        $$('.filter-chip').forEach(c => c.classList.remove('active'));
        chip.classList.add('active');
        state.instanceFilter = chip.getAttribute('data-mode-filter');
        render();
      };
    });
  }

  return {
    init,
    fetch: fetchInstances
  };
})();
