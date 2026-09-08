(() => {
  'use strict';

  const $ = (selector, root = document) => root.querySelector(selector);
  const $$ = (selector, root = document) => [...root.querySelectorAll(selector)];

  const state = {
    me: null,
    page: 'dashboard',
    cache: {
      dashboard: null,
      nodes: [],
      users: [],
      plans: [],
      inbounds: [],
      outbounds: [],
      groups: [],
      routing: [],
      tunnels: []
    }
  };

  const pages = {
    dashboard: ['داشبورد', 'وضعیت لحظه‌ای Control Plane', false],
    users: ['کاربران', 'Subscription، quota و دسترسی کاربران', true],
    plans: ['پلن‌ها', 'حجم، مدت، Device Limit و Reset', true],
    nodes: ['نودها', 'Agent، سلامت و ظرفیت نودهای GameBridge', true],
    inbounds: ['Inbounds', 'VLESS / VMess / Trojan / Shadowsocks', true],
    outbounds: ['Outbounds', 'Xray، WireGuard، Tor، OpenVPN و Custom', true],
    groups: ['Failover Groups', 'Health-aware balancing و fallback', true],
    routing: ['Routing', 'قوانین مسیریابی Xray و گروه‌های خروجی', true],
    online: ['Online Users', 'کاربران فعال و آخرین فعالیت', false],
    traffic: ['Traffic', 'Accounting تجمیعی Xray و WireGuard', false],
    tunnels: ['Tunnels', 'مسیرهای Server-to-Server', false],
    forwards: ['Port Forward', 'TCP / UDP forwarding', false],
    logs: ['Logs', 'مشاهده journal نودها', false],
    audit: ['Audit', 'تاریخچه تغییرات مدیریتی', false],
    admins: ['Admins', 'Role و دسترسی مدیران', false],
    settings: ['Settings', 'تنظیمات Control Plane', false]
  };

  function cookie(name) {
    const item = document.cookie.split('; ').find(x => x.startsWith(name + '='));
    return item ? item.slice(name.length + 1) : '';
  }

  async function api(path, options = {}) {
    const opts = {...options, headers: {...(options.headers || {})}};
    if (opts.body && typeof opts.body !== 'string') {
      opts.headers['Content-Type'] = 'application/json';
      opts.body = JSON.stringify(opts.body);
    }
    if (opts.method && !['GET', 'HEAD'].includes(opts.method.toUpperCase())) {
      opts.headers['X-GB-CSRF'] = decodeURIComponent(cookie('gb_csrf'));
    }

    const response = await fetch('/api/' + path, opts);
    const text = await response.text();
    let data = {};
    if (text) {
      try { data = JSON.parse(text); }
      catch { data = {raw: text}; }
    }
    if (response.status === 401) {
      showAuth(false);
      throw new Error('نشست شما منقضی شده است.');
    }
    if (!response.ok) {
      throw new Error(data.error || ('HTTP ' + response.status));
    }
    return data;
  }

  const esc = value => String(value ?? '').replace(/[&<>"']/g, ch => ({
    '&': '&amp;',
    '<': '&lt;',
    '>': '&gt;',
    '"': '&quot;',
    "'": '&#39;'
  })[ch]);

  const fmtBytes = value => {
    let n = Number(value || 0);
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    let i = 0;
    while (Math.abs(n) >= 1024 && i < units.length - 1) {
      n /= 1024;
      i++;
    }
    return `${n.toFixed(i ? 1 : 0)} ${units[i]}`;
  };

  const fmtDate = value => {
    if (!value || String(value).startsWith('0001-')) return '-';
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return '-';
    return date.toLocaleString('fa-IR');
  };

  const safeStatus = value => String(value || 'unknown').toLowerCase().replace(/[^a-z0-9_-]/g, '_');

  function status(value, label) {
    const raw = value || 'unknown';
    return `<span class="status ${safeStatus(raw)}">${esc(label || raw)}</span>`;
  }

  function protocol(value) {
    return `<span class="protocol">${esc(value || '-')}</span>`;
  }

  function latency(ms) {
    const n = Number(ms || 0);
    if (!n) return '<span class="latency">-</span>';
    const cls = n < 180 ? 'good' : n < 450 ? 'mid' : 'bad';
    return `<span class="latency ${cls}">${n} ms</span>`;
  }

  function toast(message, type = 'success') {
    const box = document.createElement('div');
    box.className = `toast ${type}`;
    box.textContent = String(message);
    $('#toast-stack').append(box);
    window.setTimeout(() => box.remove(), 4200);
  }

  function showAuth(setup) {
    $('#app').classList.add('hidden');
    $('#auth-screen').classList.remove('hidden');
    $('#setup-card').classList.toggle('hidden', !setup);
    $('#login-card').classList.toggle('hidden', setup);
  }

  function showApp() {
    $('#auth-screen').classList.add('hidden');
    $('#app').classList.remove('hidden');
    const user = state.me?.username || 'admin';
    const role = state.me?.role || 'viewer';
    $('#current-user').textContent = user;
    $('#current-role').textContent = role;
    $('#role-label').textContent = `${role} · Control Center`;
    $('#user-avatar').textContent = user.slice(0, 1).toUpperCase();
  }

  async function bootstrap() {
    try {
      const setup = await fetch('/api/setup/status').then(r => r.json());
      if (setup.needs_setup) {
        showAuth(true);
        return;
      }
      state.me = await api('me');
      showApp();
      await navigate(location.hash.replace(/^#\/?/, '') || 'dashboard');
    } catch {
      showAuth(false);
    }
  }

  async function setupOwner() {
    const username = $('#setup-user').value.trim();
    const password = $('#setup-pass').value;
    try {
      const response = await fetch('/api/setup', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({username, password})
      });
      const data = await response.json();
      if (!response.ok) throw new Error(data.error || 'Setup failed');
      toast('Owner ساخته شد. وارد شوید.');
      showAuth(false);
      $('#login-user').value = username;
    } catch (error) {
      toast(error.message, 'error');
    }
  }

  async function login() {
    try {
      const response = await fetch('/api/auth/login', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({
          username: $('#login-user').value.trim(),
          password: $('#login-pass').value,
          totp: $('#login-totp').value.trim()
        })
      });
      const data = await response.json();
      if (!response.ok) throw new Error(data.error || 'Login failed');
      state.me = data;
      showApp();
      await navigate('dashboard');
    } catch (error) {
      toast(error.message, 'error');
    }
  }

  async function logout() {
    try { await api('auth/logout', {method: 'POST'}); } catch {}
    location.reload();
  }

  function modal(title, body, eyebrow = 'GAMEBRIDGE') {
    $('#modal-title').textContent = title;
    $('#modal-eyebrow').textContent = eyebrow;
    $('#modal-body').innerHTML = body;
    $('#modal').classList.remove('hidden');
  }

  function closeModal() {
    $('#modal').classList.add('hidden');
    $('#modal-body').innerHTML = '';
  }

  function setPageChrome(page) {
    const meta = pages[page] || pages.dashboard;
    state.page = page;
    $('#page-title').textContent = meta[0];
    $('#page-subtitle').textContent = meta[1];
    $('#breadcrumb-page').textContent = meta[0];
    $('#page-action').classList.toggle('hidden', !meta[2]);
    $$('.nav-item').forEach(item => item.classList.toggle('active', item.dataset.page === page));
    $('#content').innerHTML = '<div class="skeleton-card"></div>';
    if (window.innerWidth <= 900) $('#sidebar').classList.remove('open');
  }

  async function navigate(page) {
    if (!pages[page]) page = 'dashboard';
    setPageChrome(page);
    history.replaceState(null, '', '#/' + page);
    try {
      const renderers = {
        dashboard: renderDashboard,
        users: renderUsers,
        plans: renderPlans,
        nodes: renderNodes,
        inbounds: renderInbounds,
        outbounds: renderOutbounds,
        groups: renderGroups,
        routing: renderRouting,
        online: renderOnline,
        traffic: renderTraffic,
        tunnels: renderTunnels,
        forwards: renderForwards,
        logs: renderLogs,
        audit: renderAudit,
        admins: renderAdmins,
        settings: renderSettings
      };
      await (renderers[page] || renderDashboard)();
    } catch (error) {
      $('#content').innerHTML = emptyState('خطا در بارگذاری', error.message);
      toast(error.message, 'error');
    }
  }

  function emptyState(title, text = '') {
    return `<div class="card empty"><strong>${esc(title)}</strong><span>${esc(text)}</span></div>`;
  }

  function normalizeUsers(rows) {
    return (rows || []).map(item => {
      if (item && item.user) return {...item.user, entitlements: item.entitlements || {}};
      return item || {};
    });
  }

  function nodeName(id) {
    return state.cache.nodes.find(n => n.id === id)?.name || id || '-';
  }

  function outboundName(id) {
    if (id === 'direct' || id === 'blocked') return id;
    const out = state.cache.outbounds.find(x => x.id === id);
    return out ? `${out.tag} · ${out.name}` : id || '-';
  }

  function groupName(id) {
    return state.cache.groups.find(x => x.id === id)?.name || id || '-';
  }

  async function loadCore() {
    const [nodes, inbounds, outbounds, groups, routing] = await Promise.all([
      api('nodes'),
      api('inbounds'),
      api('outbounds'),
      api('outbound-groups'),
      api('routing')
    ]);
    state.cache.nodes = nodes || [];
    state.cache.inbounds = inbounds || [];
    state.cache.outbounds = outbounds || [];
    state.cache.groups = groups || [];
    state.cache.routing = routing || [];
  }

  async function renderDashboard() {
    const [dashboard] = await Promise.all([api('dashboard'), loadCore()]);
    state.cache.dashboard = dashboard;
    const healthy = state.cache.outbounds.filter(x => x.enabled && x.health_status === 'healthy').length;
    const unhealthy = state.cache.outbounds.filter(x => x.enabled && x.health_status === 'unhealthy').length;
    const onlineNodes = state.cache.nodes.filter(x => x.status === 'online').length;
    const enabledInbounds = state.cache.inbounds.filter(x => x.enabled).length;

    $('#content').innerHTML = `
      <div class="grid metrics">
        ${metricCard('Users', dashboard.users || 0, fmtBytes(dashboard.traffic_bytes || 0), '◎')}
        ${metricCard('Nodes', `${onlineNodes}/${state.cache.nodes.length}`, 'Online agents', '◉')}
        ${metricCard('Inbounds', `${enabledInbounds}/${state.cache.inbounds.length}`, 'Enabled listeners', '⇢')}
        ${metricCard('Outbounds', `${healthy}/${state.cache.outbounds.length}`, `${unhealthy} unhealthy`, '⇠')}
        ${metricCard('Failover', state.cache.groups.length, `${state.cache.routing.length} routing rules`, '⌘')}
      </div>

      <div class="grid two-col">
        <div class="card">
          <div class="card-head">
            <div><h3>Outbound Health</h3><p>آخرین وضعیت egressهای فعال</p></div>
            <button class="btn small ghost" data-page="outbounds">Open Outbounds</button>
          </div>
          <div class="health-grid">
            ${state.cache.outbounds.slice(0, 8).map(x => `
              <div class="health-card">
                <div class="row">
                  <strong>${esc(x.name)}</strong>
                  ${status(x.enabled ? (x.health_status || 'unknown') : 'disabled')}
                </div>
                <small>${esc(nodeName(x.node_id))} · ${esc(x.protocol)} · ${latency(x.health_latency_ms)}</small>
              </div>
            `).join('') || '<div class="empty">Outbound تعریف نشده است.</div>'}
          </div>
        </div>

        <div class="card">
          <div class="card-head"><div><h3>Quick Actions</h3><p>دسترسی سریع به Control Plane</p></div></div>
          <div class="quick-actions">
            <button class="btn ghost" data-page="inbounds">+ Inbound</button>
            <button class="btn ghost" data-page="outbounds">+ Outbound</button>
            <button class="btn ghost" data-page="groups">Failover Groups</button>
            <button class="btn ghost" data-page="routing">Routing Rules</button>
          </div>
          <div class="stat-list">
            ${state.cache.nodes.slice(0, 5).map(n => `
              <div class="stat-line">
                <span>${esc(n.name)}</span>
                ${status(n.status)}
              </div>`).join('') || '<div class="empty">نودی وجود ندارد.</div>'}
          </div>
        </div>
      </div>
    `;
  }

  function metricCard(label, value, hint, icon) {
    return `<div class="metric-card">
      <div class="metric-top"><span>${esc(label)}</span><span class="metric-icon">${esc(icon)}</span></div>
      <strong>${esc(value)}</strong>
      <small>${esc(hint)}</small>
    </div>`;
  }

  async function renderUsers() {
    const [rows, plans] = await Promise.all([api('users'), api('plans')]);
    state.cache.users = normalizeUsers(rows);
    state.cache.plans = plans || [];
    const planByID = new Map(state.cache.plans.map(p => [p.id, p.name]));
    $('#content').innerHTML = tableCard(
      `${state.cache.users.length} user`,
      `<table><thead><tr><th>User</th><th>Status</th><th>Plan</th><th>Traffic</th><th>Expire</th><th>Devices</th></tr></thead>
      <tbody>${state.cache.users.map(u => `<tr>
        <td><strong>${esc(u.username)}</strong><span class="sub">${esc(u.display_name || u.email || '')}</span></td>
        <td>${status(u.status)}</td>
        <td>${esc(planByID.get(u.plan_id) || '-')}</td>
        <td>${fmtBytes(u.traffic_used_bytes)}</td>
        <td>${fmtDate(u.expires_at)}</td>
        <td>${esc(u.device_limit || u.entitlements?.device_limit || '-')}</td>
      </tr>`).join('')}</tbody></table>`
    );
  }

  async function renderPlans() {
    state.cache.plans = await api('plans');
    $('#content').innerHTML = tableCard(
      `${state.cache.plans.length} plan`,
      `<table><thead><tr><th>Name</th><th>Data</th><th>Duration</th><th>Devices</th><th>Reset</th><th>Status</th></tr></thead>
      <tbody>${state.cache.plans.map(p => `<tr>
        <td><strong>${esc(p.name)}</strong></td>
        <td>${p.data_limit_bytes ? fmtBytes(p.data_limit_bytes) : 'Unlimited'}</td>
        <td>${p.duration_days || '-'} days</td>
        <td>${p.device_limit || '-'}</td>
        <td>${p.reset_interval_days ? `Every ${p.reset_interval_days} days` : '-'}</td>
        <td>${status(p.enabled ? 'active' : 'disabled')}</td>
      </tr>`).join('')}</tbody></table>`
    );
  }

  async function renderNodes() {
    state.cache.nodes = await api('nodes');
    $('#content').innerHTML = tableCard(
      `${state.cache.nodes.length} node`,
      `<table><thead><tr><th>Node</th><th>Role</th><th>Public IP</th><th>Status</th><th>Load</th><th>RAM</th><th>Core</th><th>Actions</th></tr></thead>
      <tbody>${state.cache.nodes.map(n => {
        const ram = n.metrics?.memory_total ? Math.round(100 * (1 - n.metrics.memory_available / n.metrics.memory_total)) : 0;
        return `<tr>
          <td><strong>${esc(n.name)}</strong><span class="sub">${esc(n.agent_url || '')}</span></td>
          <td>${esc(n.role || '-')}</td>
          <td class="mono">${esc(n.public_ip || '-')}</td>
          <td>${status(n.status)}</td>
          <td>${Number(n.metrics?.load_1 || 0).toFixed(2)}</td>
          <td>${ram ? `${ram}%` : '-'}</td>
          <td>${esc(n.metrics?.core_version || '-')}</td>
          <td class="actions">
            <button class="btn small ghost" data-action="probe-node" data-id="${esc(n.id)}">Probe</button>
          </td>
        </tr>`;
      }).join('')}</tbody></table>`
    );
  }

  async function renderInbounds() {
    const [inbounds, nodes] = await Promise.all([api('inbounds'), api('nodes')]);
    state.cache.inbounds = inbounds || [];
    state.cache.nodes = nodes || [];
    $('#content').innerHTML = tableCard(
      `${state.cache.inbounds.length} inbound`,
      `<table><thead><tr><th>Name</th><th>Protocol</th><th>Node</th><th>Listen</th><th>Transport</th><th>Security</th><th>Status</th><th>Actions</th></tr></thead>
      <tbody>${state.cache.inbounds.map(x => `<tr>
        <td><strong>${esc(x.name)}</strong><span class="sub">${esc(x.remark || '')}</span></td>
        <td>${protocol(x.protocol)}</td>
        <td>${esc(nodeName(x.node_id))}</td>
        <td class="mono">${esc(x.listen)}:${esc(x.port)}</td>
        <td>${esc(x.transport || '-')}</td>
        <td>${esc(x.tls_mode || 'none')}</td>
        <td>${status(x.enabled ? (x.status || 'configured') : 'disabled')}</td>
        <td class="actions">
          <button class="btn small ghost" data-action="inbound-detail" data-id="${esc(x.id)}">Details</button>
          <button class="btn small ghost" data-action="redeploy-inbound" data-id="${esc(x.id)}">Deploy</button>
          <button class="btn small ${x.enabled ? 'warn' : 'ghost'}" data-action="toggle-inbound" data-id="${esc(x.id)}" data-enabled="${x.enabled}">${x.enabled ? 'Disable' : 'Enable'}</button>
          <button class="btn small danger" data-action="delete-inbound" data-id="${esc(x.id)}">Delete</button>
        </td>
      </tr>`).join('')}</tbody></table>`
    );
  }

  async function renderOutbounds() {
    const [outbounds, nodes] = await Promise.all([api('outbounds'), api('nodes')]);
    state.cache.outbounds = outbounds || [];
    state.cache.nodes = nodes || [];
    $('#content').innerHTML = tableCard(
      `${state.cache.outbounds.length} outbound`,
      `<table><thead><tr><th>Name</th><th>Protocol</th><th>Node</th><th>Target</th><th>Health</th><th>Latency</th><th>Actions</th></tr></thead>
      <tbody>${state.cache.outbounds.map(x => `<tr>
        <td><strong>${esc(x.name)}</strong><span class="sub">${esc(x.tag)}${x.remark ? ` · ${esc(x.remark)}` : ''}</span></td>
        <td>${protocol(x.protocol)}</td>
        <td>${esc(nodeName(x.node_id))}</td>
        <td class="mono">${esc(outboundTarget(x))}</td>
        <td>${status(x.enabled ? (x.health_status || 'unknown') : 'disabled')}</td>
        <td>${latency(x.health_latency_ms)}</td>
        <td class="actions">
          <button class="btn small ghost" data-action="outbound-detail" data-id="${esc(x.id)}">Details</button>
          <button class="btn small ghost" data-action="probe-outbound" data-id="${esc(x.id)}">Probe</button>
          <button class="btn small ghost" data-action="redeploy-outbound" data-id="${esc(x.id)}">Deploy</button>
          <button class="btn small ${x.enabled ? 'warn' : 'ghost'}" data-action="toggle-outbound" data-id="${esc(x.id)}" data-enabled="${x.enabled}">${x.enabled ? 'Disable' : 'Enable'}</button>
          <button class="btn small danger" data-action="delete-outbound" data-id="${esc(x.id)}">Delete</button>
        </td>
      </tr>`).join('')}</tbody></table>`
    );
  }

  function outboundTarget(x) {
    switch (x.protocol) {
      case 'tor': return `127.0.0.1:${x.tor_socks_port || 19050}`;
      case 'openvpn': return x.openvpn_interface || 'OpenVPN';
      case 'wireguard': return `${x.address || '-'}:${x.port || '-'}`;
      case 'custom': return x.custom_xray_protocol || 'custom';
      case 'freedom':
      case 'blackhole': return '-';
      default: return x.address ? `${x.address}:${x.port}` : '-';
    }
  }

  async function renderGroups() {
    const [groups, nodes, outbounds] = await Promise.all([
      api('outbound-groups'), api('nodes'), api('outbounds')
    ]);
    state.cache.groups = groups || [];
    state.cache.nodes = nodes || [];
    state.cache.outbounds = outbounds || [];

    $('#content').innerHTML = tableCard(
      `${state.cache.groups.length} group`,
      `<table><thead><tr><th>Name</th><th>Node</th><th>Strategy</th><th>Members</th><th>Fallback</th><th>Status</th><th>Actions</th></tr></thead>
      <tbody>${state.cache.groups.map(g => `<tr>
        <td><strong>${esc(g.name)}</strong></td>
        <td>${esc(nodeName(g.node_id))}</td>
        <td>${protocol(g.strategy)}</td>
        <td>${esc((g.members || []).length)}</td>
        <td>${esc(outboundName(g.fallback_outbound_id || 'blocked'))}</td>
        <td>${status(g.enabled ? 'active' : 'disabled')}</td>
        <td class="actions">
          <button class="btn small ghost" data-action="group-detail" data-id="${esc(g.id)}">Details</button>
          <button class="btn small danger" data-action="delete-group" data-id="${esc(g.id)}">Delete</button>
        </td>
      </tr>`).join('')}</tbody></table>`
    );
  }

  async function renderRouting() {
    await loadCore();
    $('#content').innerHTML = tableCard(
      `${state.cache.routing.length} rule`,
      `<table><thead><tr><th>Priority</th><th>Name</th><th>Node</th><th>Match</th><th>Target</th><th>Status</th><th>Actions</th></tr></thead>
      <tbody>${state.cache.routing.map(r => {
        const target = r.outbound_group_id ? `Group · ${groupName(r.outbound_group_id)}` : outboundName(r.outbound_id);
        const match = [
          r.inbound_ids?.length ? `${r.inbound_ids.length} inbound` : '',
          r.user_ids?.length ? `${r.user_ids.length} user` : '',
          r.domains?.length ? `${r.domains.length} domain` : '',
          r.ips?.length ? `${r.ips.length} IP` : '',
          r.ports ? `ports ${r.ports}` : ''
        ].filter(Boolean).join(' · ') || 'Any';
        return `<tr>
          <td>${esc(r.priority)}</td>
          <td><strong>${esc(r.name)}</strong></td>
          <td>${esc(nodeName(r.node_id))}</td>
          <td>${esc(match)}</td>
          <td>${esc(target)}</td>
          <td>${status(r.enabled ? 'active' : 'disabled')}</td>
          <td class="actions"><button class="btn small danger" data-action="delete-routing" data-id="${esc(r.id)}">Delete</button></td>
        </tr>`;
      }).join('')}</tbody></table>`
    );
  }

  async function renderOnline() {
    const rows = await api('online-users');
    $('#content').innerHTML = tableCard(
      `${rows.length} online`,
      `<table><thead><tr><th>User</th><th>Status</th><th>Total</th><th>Xray</th><th>WireGuard</th><th>Last Activity</th></tr></thead>
      <tbody>${rows.map(u => `<tr>
        <td><strong>${esc(u.username)}</strong></td>
        <td>${status(u.status)}</td>
        <td>${fmtBytes(u.traffic_bytes)}</td>
        <td>${fmtBytes(u.xray_bytes)}</td>
        <td>${fmtBytes(u.wireguard_bytes)}</td>
        <td>${fmtDate(u.last_online_at)}</td>
      </tr>`).join('')}</tbody></table>`,
      '<button class="btn small ghost" data-action="sync-traffic">Sync Traffic</button>'
    );
  }

  async function renderTraffic() {
    const rows = await api('traffic');
    $('#content').innerHTML = tableCard(
      `${rows.length} account`,
      `<table><thead><tr><th>User</th><th>Status</th><th>Total</th><th>Xray</th><th>WireGuard</th><th>Limit</th><th>Next Reset</th></tr></thead>
      <tbody>${rows.map(u => `<tr>
        <td><strong>${esc(u.username)}</strong></td>
        <td>${status(u.status)}</td>
        <td>${fmtBytes(u.traffic_bytes)}</td>
        <td>${fmtBytes(u.xray_bytes)}</td>
        <td>${fmtBytes(u.wireguard_bytes)}</td>
        <td>${u.limit_bytes ? fmtBytes(u.limit_bytes) : 'Unlimited'}</td>
        <td>${fmtDate(u.next_reset_at)}</td>
      </tr>`).join('')}</tbody></table>`,
      '<button class="btn small ghost" data-action="sync-traffic">Sync Traffic</button>'
    );
  }

  async function renderTunnels() {
    const [tunnels, nodes] = await Promise.all([api('tunnels'), api('nodes')]);
    state.cache.tunnels = tunnels || [];
    state.cache.nodes = nodes || [];
    $('#content').innerHTML = tableCard(
      `${state.cache.tunnels.length} tunnel`,
      `<table><thead><tr><th>Name</th><th>Transport</th><th>Source</th><th>Destination</th><th>Profile</th><th>MTU</th><th>Status</th></tr></thead>
      <tbody>${state.cache.tunnels.map(t => `<tr>
        <td><strong>${esc(t.name)}</strong><span class="sub">${esc(t.cidr || '')}</span></td>
        <td>${protocol(t.transport)}</td>
        <td>${esc(nodeName(t.source_node_id))}</td>
        <td>${esc(nodeName(t.destination_node_id))}</td>
        <td>${esc(t.profile || '-')}</td>
        <td>${esc(t.mtu || '-')}</td>
        <td>${status(t.status)}</td>
      </tr>`).join('')}</tbody></table>`
    );
  }

  async function renderForwards() {
    const [rows, nodes] = await Promise.all([api('forwards'), api('nodes')]);
    state.cache.nodes = nodes || [];
    $('#content').innerHTML = tableCard(
      `${rows.length} forward`,
      `<table><thead><tr><th>Node</th><th>Protocol</th><th>Listen</th><th>Destination</th><th>Status</th></tr></thead>
      <tbody>${rows.map(f => `<tr>
        <td>${esc(nodeName(f.node_id))}</td>
        <td>${protocol(f.protocol)}</td>
        <td class="mono">${esc(f.listen_port)}</td>
        <td class="mono">${esc(f.dest_ip)}:${esc(f.dest_port)}</td>
        <td>${status(f.enabled === false ? 'disabled' : 'active')}</td>
      </tr>`).join('')}</tbody></table>`
    );
  }

  async function renderLogs() {
    state.cache.nodes = await api('nodes');
    $('#content').innerHTML = `<div class="card">
      <div class="card-head"><div><h3>Node Journal</h3><p>خواندن لاگ سرویس از Agent</p></div></div>
      <div class="form-grid">
        <label class="field"><span>Node</span><select id="log-node">${nodeOptions()}</select></label>
        <label class="field"><span>Unit</span><input id="log-unit" value="gamebridge-agent.service"></label>
        <label class="field"><span>Lines</span><input id="log-lines" type="number" value="200"></label>
        <div class="field"><span>&nbsp;</span><button class="btn primary" data-action="load-logs">Load Logs</button></div>
      </div>
      <div id="log-output" class="code-box">Select a node and load logs.</div>
    </div>`;
  }

  async function renderAudit() {
    const rows = await api('audit');
    $('#content').innerHTML = tableCard(
      `${rows.length} event`,
      `<table><thead><tr><th>Time</th><th>Admin</th><th>Action</th><th>Object</th><th>IP</th></tr></thead>
      <tbody>${rows.map(x => `<tr>
        <td>${fmtDate(x.created_at)}</td>
        <td>${esc(x.admin_name || '-')}</td>
        <td>${protocol(x.action)}</td>
        <td>${esc(x.object || '-')}</td>
        <td class="mono">${esc(x.remote_ip || '-')}</td>
      </tr>`).join('')}</tbody></table>`
    );
  }

  async function renderAdmins() {
    try {
      const rows = await api('admins');
      $('#content').innerHTML = tableCard(
        `${rows.length} admin`,
        `<table><thead><tr><th>User</th><th>Role</th><th>Status</th><th>Created</th></tr></thead>
        <tbody>${rows.map(x => `<tr>
          <td><strong>${esc(x.username)}</strong></td>
          <td>${protocol(x.role)}</td>
          <td>${status(x.enabled ? 'active' : 'disabled')}</td>
          <td>${fmtDate(x.created_at)}</td>
        </tr>`).join('')}</tbody></table>`
      );
    } catch (error) {
      $('#content').innerHTML = emptyState('دسترسی محدود', error.message);
    }
  }

  async function renderSettings() {
    const settings = await api('settings');
    $('#content').innerHTML = `<div class="grid two-col">
      <div class="card">
        <div class="card-head"><div><h3>Control Plane Settings</h3><p>تنظیمات عمومی پنل</p></div></div>
        <div class="form-grid">
          <label class="field span-2"><span>Site name</span><input id="settings-site-name" value="${esc(settings.site_name || 'GameBridge')}"></label>
          <div class="span-2"><button class="btn primary" data-action="save-settings">Save Settings</button></div>
        </div>
      </div>
      <div class="card">
        <div class="card-head"><div><h3>Runtime</h3><p>Backend v1 frozen</p></div></div>
        <div class="notice">این Control Center مستقیماً با APIهای امن GameBridge کار می‌کند و هیچ inline script یا handler اجرا نمی‌کند.</div>
      </div>
    </div>`;
  }

  function tableCard(meta, table, extra = '') {
    return `<div class="card table-card">
      <div class="table-toolbar">
        <span class="meta">${esc(meta)}</span>
        <div>${extra}</div>
      </div>
      <div class="table-wrap">${table}</div>
    </div>`;
  }

  function nodeOptions(selected = '') {
    return state.cache.nodes.map(n =>
      `<option value="${esc(n.id)}" ${n.id === selected ? 'selected' : ''}>${esc(n.name)} · ${esc(n.status || 'unknown')}</option>`
    ).join('');
  }

  function planOptions(selected = '') {
    return '<option value="">No plan</option>' + state.cache.plans.map(p =>
      `<option value="${esc(p.id)}" ${p.id === selected ? 'selected' : ''}>${esc(p.name)}</option>`
    ).join('');
  }

  async function openCreate(page) {
    try {
      if (page === 'users') return openUserCreate();
      if (page === 'plans') return openPlanCreate();
      if (page === 'nodes') return openNodeCreate();
      if (page === 'inbounds') return openInboundCreate();
      if (page === 'outbounds') return openOutboundCreate();
      if (page === 'groups') return openGroupCreate();
      if (page === 'routing') return openRoutingCreate();
    } catch (error) {
      toast(error.message, 'error');
    }
  }

  async function openUserCreate() {
    state.cache.plans = await api('plans');
    modal('کاربر جدید', `<div class="form-grid">
      <label class="field"><span>Username</span><input id="user-name"></label>
      <label class="field"><span>Display name</span><input id="user-display"></label>
      <label class="field"><span>Email</span><input id="user-email" type="email"></label>
      <label class="field"><span>Plan</span><select id="user-plan">${planOptions()}</select></label>
      <label class="field"><span>Data limit (GB, 0 = plan/unlimited)</span><input id="user-data" type="number" min="0" value="0"></label>
      <label class="field"><span>Device limit</span><input id="user-devices" type="number" min="0" value="1"></label>
      <div class="span-2"><button class="btn primary wide" data-action="create-user">Create User</button></div>
    </div>`, 'USERS');
  }

  function openPlanCreate() {
    modal('پلن جدید', `<div class="form-grid">
      <label class="field"><span>Name</span><input id="plan-name"></label>
      <label class="field"><span>Data (GB, 0 = unlimited)</span><input id="plan-data" type="number" min="0" value="50"></label>
      <label class="field"><span>Duration days</span><input id="plan-days" type="number" min="0" value="30"></label>
      <label class="field"><span>Device limit</span><input id="plan-devices" type="number" min="1" value="1"></label>
      <label class="field"><span>Reset interval days</span><input id="plan-reset" type="number" min="0" value="30"></label>
      <div class="span-2"><button class="btn primary wide" data-action="create-plan">Create Plan</button></div>
    </div>`, 'PLANS');
  }

  function openNodeCreate() {
    modal('نود جدید', `<div class="form-grid">
      <label class="field"><span>Name</span><input id="node-name"></label>
      <label class="field"><span>Role</span><input id="node-role" value="edge"></label>
      <label class="field"><span>Public IP</span><input id="node-ip"></label>
      <label class="field"><span>Internet interface</span><input id="node-iface" value="eth0"></label>
      <label class="field span-2"><span>Agent URL</span><input id="node-url" placeholder="https://node.example.com:8443"></label>
      <label class="field span-2"><span>Agent token</span><input id="node-token" type="password"></label>
      <div class="span-2"><button class="btn primary wide" data-action="create-node">Create Node</button></div>
    </div>`, 'NODES');
  }

  async function openInboundCreate() {
    state.cache.nodes = await api('nodes');
    modal('Inbound جدید', `<div class="form-grid">
      <label class="field"><span>Name</span><input id="in-name" value="vless-main"></label>
      <label class="field"><span>Node</span><select id="in-node">${nodeOptions()}</select></label>
      <label class="field"><span>Protocol</span><select id="in-protocol">
        <option value="vless">VLESS</option><option value="vmess">VMess</option>
        <option value="trojan">Trojan</option><option value="shadowsocks">Shadowsocks</option>
      </select></label>
      <label class="field"><span>Listen</span><input id="in-listen" value="0.0.0.0"></label>
      <label class="field"><span>Port</span><input id="in-port" type="number" value="443"></label>
      <label class="field"><span>Transport</span><select id="in-transport">
        <option value="tcp">TCP</option><option value="ws">WebSocket</option><option value="grpc">gRPC</option>
      </select></label>
      <label class="field"><span>Security</span><select id="in-tls">
        <option value="none">None</option><option value="tls">TLS</option><option value="reality">REALITY</option>
      </select></label>
      <label class="field"><span>Server name</span><input id="in-server-name"></label>

      <div class="form-section">
        <div class="form-section-title">Transport</div>
        <label class="field"><span>WS path</span><input id="in-path" value="/"></label>
        <label class="field"><span>WS host</span><input id="in-host"></label>
        <label class="field"><span>gRPC service</span><input id="in-service" value="gamebridge"></label>
        <label class="field"><span>Shadowsocks method</span><select id="in-ss-method">
          <option>aes-128-gcm</option><option>aes-256-gcm</option><option>chacha20-poly1305</option>
        </select></label>
      </div>

      <div class="form-section">
        <div class="form-section-title">TLS / REALITY</div>
        <label class="field"><span>TLS certificate path</span><input id="in-cert"></label>
        <label class="field"><span>TLS key path</span><input id="in-key"></label>
        <label class="field"><span>REALITY destination</span><input id="in-reality-dest" placeholder="www.cloudflare.com:443"></label>
        <label class="field"><span>REALITY server names</span><input id="in-reality-names" placeholder="www.cloudflare.com,cloudflare.com"></label>
        <label class="field"><span>REALITY short IDs</span><input id="in-reality-short" placeholder="optional"></label>
        <label class="field"><span>Fingerprint</span><input id="in-fingerprint" value="chrome"></label>
      </div>

      <label class="field span-2"><span>Remark</span><input id="in-remark"></label>
      <div class="span-2"><button class="btn primary wide" data-action="create-inbound">Create + Deploy</button></div>
    </div>`, 'XRAY INBOUND');
  }

  async function openOutboundCreate() {
    state.cache.nodes = await api('nodes');
    modal('Outbound جدید', `<div class="form-grid">
      <label class="field"><span>Name</span><input id="out-name" value="proxy-main"></label>
      <label class="field"><span>Node</span><select id="out-node">${nodeOptions()}</select></label>
      <label class="field"><span>Tag</span><input id="out-tag" value="proxy-main"></label>
      <label class="field"><span>Protocol</span><select id="out-protocol" data-role="outbound-protocol">
        <option value="vless">VLESS</option>
        <option value="vmess">VMess</option>
        <option value="trojan">Trojan</option>
        <option value="shadowsocks">Shadowsocks</option>
        <option value="socks">SOCKS</option>
        <option value="http">HTTP Proxy</option>
        <option value="wireguard">WireGuard</option>
        <option value="tor">Tor</option>
        <option value="openvpn">OpenVPN</option>
        <option value="custom">Custom Xray</option>
        <option value="freedom">Freedom</option>
        <option value="blackhole">Blackhole</option>
      </select></label>
      <div id="outbound-fields" class="form-section"></div>
      <label class="field span-2"><span>Remark</span><input id="out-remark"></label>
      <div class="span-2"><button class="btn primary wide" data-action="create-outbound">Create + Deploy</button></div>
    </div>`, 'EGRESS');
    renderOutboundFields('vless');
  }

  function renderOutboundFields(proto) {
    const host = $('#outbound-fields');
    if (!host) return;
    const endpoint = `
      <div class="form-section-title">Endpoint</div>
      <label class="field"><span>Address</span><input id="out-address"></label>
      <label class="field"><span>Port</span><input id="out-port" type="number" value="443"></label>`;

    if (['freedom', 'blackhole'].includes(proto)) {
      host.innerHTML = '<div class="form-section-title">No additional settings required.</div>';
      return;
    }
    if (['socks', 'http'].includes(proto)) {
      host.innerHTML = endpoint + `
        <label class="field"><span>Username</span><input id="out-user"></label>
        <label class="field"><span>Password</span><input id="out-password" type="password"></label>`;
      return;
    }
    if (['vless', 'vmess', 'trojan'].includes(proto)) {
      host.innerHTML = endpoint + `
        <label class="field"><span>Credential / UUID / Password</span><input id="out-secret" type="password"></label>
        <label class="field"><span>Transport</span><select id="out-transport"><option value="raw">Raw</option><option value="ws">WebSocket</option><option value="grpc">gRPC</option></select></label>
        <label class="field"><span>Security</span><select id="out-tls"><option value="none">None</option><option value="tls">TLS</option><option value="reality">REALITY</option></select></label>
        <label class="field"><span>Server name</span><input id="out-server-name"></label>
        <label class="field"><span>Fingerprint</span><input id="out-fingerprint" value="chrome"></label>
        <label class="field"><span>WS path</span><input id="out-path" value="/"></label>
        <label class="field"><span>WS host</span><input id="out-host"></label>
        <label class="field"><span>gRPC service</span><input id="out-service" value="gamebridge"></label>
        ${proto === 'vless' ? '<label class="field"><span>Flow</span><select id="out-flow"><option value="">None</option><option value="xtls-rprx-vision">Vision</option></select></label>' : ''}
        <label class="field"><span>REALITY public key</span><input id="out-reality-key"></label>
        <label class="field"><span>REALITY short ID</span><input id="out-reality-short"></label>
        <label class="check-row"><input id="out-insecure" type="checkbox"> Allow insecure TLS</label>`;
      return;
    }
    if (proto === 'shadowsocks') {
      host.innerHTML = endpoint + `
        <label class="field"><span>Password</span><input id="out-secret" type="password"></label>
        <label class="field"><span>Method</span><select id="out-ss-method"><option>aes-128-gcm</option><option>aes-256-gcm</option><option>chacha20-poly1305</option></select></label>`;
      return;
    }
    if (proto === 'wireguard') {
      host.innerHTML = endpoint + `
        <label class="field"><span>Private key</span><input id="out-secret" type="password"></label>
        <label class="field"><span>Interface</span><input id="out-wg-interface" placeholder="auto"></label>
        <label class="field"><span>Local address CIDR</span><input id="out-wg-address" placeholder="10.0.0.2/32"></label>
        <label class="field"><span>Peer public key</span><input id="out-wg-peer"></label>
        <label class="field span-2"><span>Allowed IPs</span><input id="out-wg-allowed" value="0.0.0.0/0,::/0"></label>
        <label class="field"><span>Keepalive</span><input id="out-wg-keepalive" type="number" value="25"></label>
        <label class="field"><span>MTU</span><input id="out-wg-mtu" type="number" value="1420"></label>`;
      return;
    }
    if (proto === 'tor') {
      host.innerHTML = `
        <div class="form-section-title">Native Tor</div>
        <label class="field"><span>Local SOCKS port</span><input id="out-tor-port" type="number" value="19050"></label>
        <div class="notice">Agent یک instance مستقل Tor برای این outbound مدیریت می‌کند.</div>`;
      return;
    }
    if (proto === 'openvpn') {
      host.innerHTML = `
        <div class="form-section-title">Native OpenVPN</div>
        <label class="field"><span>Username</span><input id="out-user"></label>
        <label class="field"><span>Auth password</span><input id="out-password" type="password"></label>
        <label class="field"><span>Interface</span><input id="out-ovpn-interface" placeholder="auto"></label>
        <label class="field"><span>Routing table</span><input id="out-ovpn-table" type="number" value="0"></label>
        <label class="field"><span>fwmark</span><input id="out-ovpn-mark" type="number" value="0"></label>
        <label class="field span-2"><span>Self-contained .ovpn profile</span><textarea id="out-secret" class="mono" rows="10"></textarea></label>`;
      return;
    }
    if (proto === 'custom') {
      host.innerHTML = `
        <div class="form-section-title">Advanced Xray Outbound JSON</div>
        <label class="field span-2"><span>Config JSON (without tag)</span><textarea id="out-secret" class="mono" rows="12" placeholder='{"protocol":"freedom","settings":{}}'></textarea></label>
        <div class="notice warn span-2">GameBridge مالک tag است. proxySettings و dialerProxy در v1 مسدود هستند.</div>`;
    }
  }

  async function openGroupCreate() {
    const [nodes, outbounds] = await Promise.all([api('nodes'), api('outbounds')]);
    state.cache.nodes = nodes || [];
    state.cache.outbounds = outbounds || [];
    modal('Failover Group جدید', `<div class="form-grid">
      <label class="field"><span>Name</span><input id="group-name" value="primary-egress"></label>
      <label class="field"><span>Node</span><select id="group-node" data-role="group-node">${nodeOptions()}</select></label>
      <label class="field"><span>Strategy</span><select id="group-strategy">
        <option value="least_ping">Least Ping</option><option value="least_load">Least Load</option>
        <option value="round_robin">Round Robin</option><option value="random">Random</option>
      </select></label>
      <label class="field"><span>Expected healthy members</span><input id="group-expected" type="number" min="1" value="1"></label>
      <label class="field span-2"><span>Fallback</span><select id="group-fallback"></select></label>
      <div class="span-2">
        <div class="form-section-title">Members</div>
        <div id="group-members" class="member-list"></div>
      </div>
      <div class="span-2"><button class="btn primary wide" data-action="create-group">Create + Deploy</button></div>
    </div>`, 'FAILOVER');
    refreshGroupCandidates();
  }

  function refreshGroupCandidates() {
    const nodeID = $('#group-node')?.value;
    if (!nodeID) return;
    const candidates = state.cache.outbounds.filter(o => o.node_id === nodeID && o.enabled && o.protocol !== 'blackhole');
    const members = $('#group-members');
    if (members) {
      members.innerHTML = candidates.map(o => `<div class="member-row">
        <input type="checkbox" data-group-member="${esc(o.id)}">
        <div class="member-name"><strong>${esc(o.name)}</strong><small>${esc(o.tag)} · ${esc(o.protocol)}</small></div>
        <input type="number" min="0" max="1000" value="0" data-member-priority="${esc(o.id)}" title="Priority">
        <input type="number" min="1" max="100" value="1" data-member-weight="${esc(o.id)}" title="Weight">
      </div>`).join('') || '<div class="empty">Outbound فعال برای این نود وجود ندارد.</div>';
    }
    const fallback = $('#group-fallback');
    if (fallback) {
      fallback.innerHTML = `<option value="blocked">blocked (fail closed)</option><option value="direct">direct</option>` +
        candidates.map(o => `<option value="${esc(o.id)}">${esc(o.name)} · ${esc(o.tag)}</option>`).join('');
    }
  }

  async function openRoutingCreate() {
    const [nodes, inbounds, outbounds, groups, users] = await Promise.all([
      api('nodes'), api('inbounds'), api('outbounds'), api('outbound-groups'), api('users')
    ]);
    state.cache.nodes = nodes || [];
    state.cache.inbounds = inbounds || [];
    state.cache.outbounds = outbounds || [];
    state.cache.groups = groups || [];
    state.cache.users = normalizeUsers(users);

    modal('Routing Rule جدید', `<div class="form-grid">
      <label class="field"><span>Name</span><input id="route-name" value="route-100"></label>
      <label class="field"><span>Priority</span><input id="route-priority" type="number" value="100"></label>
      <label class="field"><span>Node</span><select id="route-node" data-role="route-node">${nodeOptions()}</select></label>
      <label class="field"><span>Target type</span><select id="route-target-type" data-role="route-target-type"><option value="outbound">Outbound</option><option value="group">Failover Group</option></select></label>
      <label class="field span-2"><span>Target</span><select id="route-target"></select></label>
      <label class="field"><span>Inbound</span><select id="route-inbound"></select></label>
      <label class="field"><span>User</span><select id="route-user"><option value="">Any user</option>${state.cache.users.map(u => `<option value="${esc(u.id)}">${esc(u.username)}</option>`).join('')}</select></label>
      <label class="field span-2"><span>Domains (comma separated)</span><input id="route-domains" placeholder="domain:example.com, geosite:google"></label>
      <label class="field span-2"><span>IPs (comma separated)</span><input id="route-ips" placeholder="geoip:private, 10.0.0.0/8"></label>
      <label class="field"><span>Ports</span><input id="route-ports" placeholder="80,443,1000-2000"></label>
      <label class="field"><span>Network</span><select id="route-network"><option value="">Any</option><option value="tcp">TCP</option><option value="udp">UDP</option><option value="tcp,udp">TCP + UDP</option></select></label>
      <label class="field span-2"><span>Protocols (comma separated)</span><input id="route-protocols" placeholder="bittorrent"></label>
      <div class="span-2"><button class="btn primary wide" data-action="create-routing">Create + Deploy</button></div>
    </div>`, 'ROUTING');
    refreshRoutingCandidates();
  }

  function refreshRoutingCandidates() {
    const nodeID = $('#route-node')?.value;
    const type = $('#route-target-type')?.value || 'outbound';
    const target = $('#route-target');
    const inbound = $('#route-inbound');
    if (!nodeID || !target || !inbound) return;

    if (type === 'group') {
      target.innerHTML = state.cache.groups.filter(g => g.node_id === nodeID && g.enabled)
        .map(g => `<option value="${esc(g.id)}">${esc(g.name)} · ${esc(g.strategy)}</option>`).join('');
    } else {
      target.innerHTML = `<option value="direct">direct</option><option value="blocked">blocked</option>` +
        state.cache.outbounds.filter(o => o.node_id === nodeID && o.enabled)
          .map(o => `<option value="${esc(o.id)}">${esc(o.name)} · ${esc(o.tag)}</option>`).join('');
    }
    inbound.innerHTML = `<option value="">Any inbound</option>` +
      state.cache.inbounds.filter(i => i.node_id === nodeID)
        .map(i => `<option value="${esc(i.id)}">${esc(i.name)} · ${esc(i.protocol)}</option>`).join('');
  }

  function val(id, fallback = '') {
    return $(id)?.value ?? fallback;
  }

  function num(id, fallback = 0) {
    const n = Number(val(id, fallback));
    return Number.isFinite(n) ? n : fallback;
  }

  function csv(value) {
    return String(value || '').split(',').map(x => x.trim()).filter(Boolean);
  }

  async function createUser() {
    await api('users', {method: 'POST', body: {
      username: val('#user-name').trim(),
      display_name: val('#user-display').trim(),
      email: val('#user-email').trim(),
      plan_id: val('#user-plan'),
      data_limit_bytes: num('#user-data') * 1024 ** 3,
      device_limit: num('#user-devices', 1)
    }});
    closeModal(); toast('User ساخته شد'); navigate('users');
  }

  async function createPlan() {
    await api('plans', {method: 'POST', body: {
      name: val('#plan-name').trim(),
      data_limit_bytes: num('#plan-data') * 1024 ** 3,
      duration_days: num('#plan-days'),
      device_limit: num('#plan-devices', 1),
      reset_interval_days: num('#plan-reset')
    }});
    closeModal(); toast('Plan ساخته شد'); navigate('plans');
  }

  async function createNode() {
    await api('nodes', {method: 'POST', body: {
      name: val('#node-name').trim(),
      role: val('#node-role').trim(),
      public_ip: val('#node-ip').trim(),
      agent_url: val('#node-url').trim(),
      agent_token: val('#node-token'),
      internet_interface: val('#node-iface').trim()
    }});
    closeModal(); toast('Node ساخته شد'); navigate('nodes');
  }

  async function createInbound() {
    await api('inbounds', {method: 'POST', body: {
      name: val('#in-name').trim(),
      node_id: val('#in-node'),
      protocol: val('#in-protocol'),
      listen: val('#in-listen').trim(),
      port: num('#in-port'),
      transport: val('#in-transport'),
      tls_mode: val('#in-tls'),
      enabled: true,
      server_name: val('#in-server-name').trim(),
      path: val('#in-path').trim(),
      host: val('#in-host').trim(),
      service_name: val('#in-service').trim(),
      shadowsocks_method: val('#in-ss-method'),
      cert_file: val('#in-cert').trim(),
      key_file: val('#in-key').trim(),
      reality_dest: val('#in-reality-dest').trim(),
      reality_server_names: csv(val('#in-reality-names')),
      reality_short_ids: csv(val('#in-reality-short')),
      reality_fingerprint: val('#in-fingerprint').trim(),
      remark: val('#in-remark').trim()
    }});
    closeModal(); toast('Inbound ساخته و deploy شد'); navigate('inbounds');
  }

  async function createOutbound() {
    const p = val('#out-protocol');
    const body = {
      name: val('#out-name').trim(),
      node_id: val('#out-node'),
      tag: val('#out-tag').trim(),
      protocol: p,
      address: val('#out-address').trim(),
      port: num('#out-port'),
      username: val('#out-user').trim(),
      password: val('#out-password'),
      secret: val('#out-secret'),
      transport: val('#out-transport'),
      tls_mode: val('#out-tls'),
      path: val('#out-path').trim(),
      host: val('#out-host').trim(),
      service_name: val('#out-service').trim(),
      server_name: val('#out-server-name').trim(),
      allow_insecure: Boolean($('#out-insecure')?.checked),
      fingerprint: val('#out-fingerprint').trim(),
      flow: val('#out-flow'),
      shadowsocks_method: val('#out-ss-method'),
      reality_public_key: val('#out-reality-key').trim(),
      reality_short_id: val('#out-reality-short').trim(),
      wireguard_interface: val('#out-wg-interface').trim(),
      wireguard_address: val('#out-wg-address').trim(),
      wireguard_peer_public_key: val('#out-wg-peer').trim(),
      wireguard_allowed_ips: csv(val('#out-wg-allowed')),
      wireguard_keepalive: num('#out-wg-keepalive'),
      wireguard_mtu: num('#out-wg-mtu'),
      tor_socks_port: num('#out-tor-port'),
      openvpn_interface: val('#out-ovpn-interface').trim(),
      openvpn_routing_table: num('#out-ovpn-table'),
      openvpn_mark: num('#out-ovpn-mark'),
      enabled: true,
      remark: val('#out-remark').trim()
    };
    await api('outbounds', {method: 'POST', body});
    closeModal(); toast('Outbound ساخته و deploy شد'); navigate('outbounds');
  }

  async function createGroup() {
    const members = $$('[data-group-member]:checked').map(box => {
      const id = box.dataset.groupMember;
      return {
        outbound_id: id,
        priority: Number($(`[data-member-priority="${CSS.escape(id)}"]`)?.value || 0),
        weight: Number($(`[data-member-weight="${CSS.escape(id)}"]`)?.value || 1)
      };
    });
    await api('outbound-groups', {method: 'POST', body: {
      name: val('#group-name').trim(),
      node_id: val('#group-node'),
      strategy: val('#group-strategy'),
      members,
      fallback_outbound_id: val('#group-fallback') || 'blocked',
      expected: num('#group-expected', 1),
      enabled: true
    }});
    closeModal(); toast('Failover group ساخته شد'); navigate('groups');
  }

  async function createRouting() {
    const type = val('#route-target-type');
    const target = val('#route-target');
    const inbound = val('#route-inbound');
    const user = val('#route-user');
    const body = {
      name: val('#route-name').trim(),
      node_id: val('#route-node'),
      priority: num('#route-priority'),
      enabled: true,
      inbound_ids: inbound ? [inbound] : [],
      user_ids: user ? [user] : [],
      domains: csv(val('#route-domains')),
      ips: csv(val('#route-ips')),
      ports: val('#route-ports').trim(),
      network: val('#route-network'),
      protocols: csv(val('#route-protocols')),
      outbound_id: type === 'outbound' ? target : '',
      outbound_group_id: type === 'group' ? target : ''
    };
    await api('routing', {method: 'POST', body});
    closeModal(); toast('Routing rule ساخته و deploy شد'); navigate('routing');
  }

  async function showInboundDetail(id) {
    const detail = await api(`inbounds/${id}`);
    const summary = await api(`inbounds/${id}/summary`);
    const inbound = detail.inbound || summary.inbound || {};
    const clients = detail.clients || [];
    modal(inbound.name || 'Inbound', `
      <div class="grid three-col">
        ${metricCard('Attached users', summary.attached_users || clients.length, 'Bindings', '◎')}
        ${metricCard('Active users', summary.active_users || 0, 'Lifecycle active', '●')}
        ${metricCard('Node', nodeName(inbound.node_id), summary.node_status || 'unknown', '◉')}
      </div>
      <div class="card">
        <div class="card-head"><div><h3>Clients</h3><p>${clients.length} binding</p></div></div>
        <div class="table-wrap"><table><thead><tr><th>User</th><th>Status</th></tr></thead><tbody>
          ${clients.map(c => `<tr><td>${esc(c.username || c.user_id)}</td><td>${status(c.enabled ? 'active' : 'disabled')}</td></tr>`).join('')}
        </tbody></table></div>
      </div>`, 'INBOUND');
  }

  async function showOutboundDetail(id) {
    const [summary, health] = await Promise.all([
      api(`outbounds/${id}/summary`),
      api(`outbounds/${id}/health`)
    ]);
    const out = summary.outbound || {};
    modal(out.name || 'Outbound', `
      <div class="grid three-col">
        ${metricCard('Health', health.health_status || summary.health_status || 'unknown', health.health_target || 'egress probe', '●')}
        ${metricCard('Latency', health.health_latency_ms ? `${health.health_latency_ms} ms` : '-', `Failures: ${health.health_failure_count || 0}`, '↯')}
        ${metricCard('Routing', summary.enabled_rules || 0, `${summary.routing_rules || 0} total rules`, '⌁')}
      </div>
      <div class="card">
        <div class="stat-list">
          <div class="stat-line"><span>Protocol</span>${protocol(out.protocol)}</div>
          <div class="stat-line"><span>Node</span><span>${esc(nodeName(out.node_id))}</span></div>
          <div class="stat-line"><span>Tag</span><span class="mono">${esc(out.tag)}</span></div>
          <div class="stat-line"><span>Last checked</span><span>${fmtDate(health.health_last_checked_at)}</span></div>
          <div class="stat-line"><span>Last error</span><span>${esc(health.health_last_error || '-')}</span></div>
        </div>
      </div>`, 'OUTBOUND HEALTH');
  }

  async function showGroupDetail(id) {
    const d = await api(`outbound-groups/${id}/summary`);
    const g = d.group || {};
    modal(g.name || 'Failover Group', `
      <div class="grid three-col">
        ${metricCard('Strategy', g.strategy || '-', 'Xray balancer', '⌘')}
        ${metricCard('Members', (d.members || []).length, `Expected ${g.expected || 1}`, '◎')}
        ${metricCard('Fallback', d.fallback_tag || 'blocked', 'When all members fail', '↪')}
      </div>
      <div class="card table-card"><div class="table-wrap"><table><thead><tr><th>Member</th><th>Protocol</th><th>Health</th><th>Latency</th><th>Priority</th><th>Weight</th></tr></thead>
      <tbody>${(d.members || []).map(m => `<tr>
        <td><strong>${esc(m.name || m.outbound_id)}</strong><span class="sub">${esc(m.tag || '')}</span></td>
        <td>${protocol(m.protocol)}</td>
        <td>${status(m.health_status || 'unknown')}</td>
        <td>${latency(m.latency_ms)}</td>
        <td>${esc(m.priority)}</td>
        <td>${esc(m.weight)}</td>
      </tr>`).join('')}</tbody></table></div></div>`, 'FAILOVER');
  }

  async function loadLogs() {
    const node = val('#log-node');
    const unit = val('#log-unit').trim();
    const lines = num('#log-lines', 200);
    const data = await api(`logs?node_id=${encodeURIComponent(node)}&unit=${encodeURIComponent(unit)}&lines=${encodeURIComponent(lines)}`);
    $('#log-output').textContent = data.logs || '';
  }

  async function syncTraffic() {
    await api('sync-traffic', {method: 'POST'});
    toast('Traffic sync شد');
    await navigate(state.page);
  }

  async function saveSettings() {
    await api('settings', {method: 'PUT', body: {site_name: val('#settings-site-name').trim()}});
    toast('Settings ذخیره شد');
    await renderSettings();
  }

  async function confirmDelete(message) {
    return window.confirm(message);
  }

  document.addEventListener('click', async event => {
    const pageButton = event.target.closest('[data-page]');
    if (pageButton) {
      event.preventDefault();
      await navigate(pageButton.dataset.page);
      return;
    }

    const button = event.target.closest('[data-action]');
    if (!button) return;
    const action = button.dataset.action;
    const id = button.dataset.id;

    try {
      switch (action) {
        case 'setup-owner': return setupOwner();
        case 'login': return login();
        case 'logout': return logout();
        case 'toggle-sidebar': return $('#sidebar').classList.toggle('open');
        case 'close-modal': return closeModal();
        case 'refresh-page': return navigate(state.page);
        case 'new-current': return openCreate(state.page);

        case 'create-user': return createUser();
        case 'create-plan': return createPlan();
        case 'create-node': return createNode();
        case 'create-inbound': return createInbound();
        case 'create-outbound': return createOutbound();
        case 'create-group': return createGroup();
        case 'create-routing': return createRouting();

        case 'probe-node':
          await api(`nodes/${id}/probe`, {method: 'POST'}); toast('Node probe شد'); return navigate('nodes');

        case 'inbound-detail': return showInboundDetail(id);
        case 'redeploy-inbound':
          await api(`inbounds/${id}/redeploy`, {method: 'POST'}); toast('Inbound deploy شد'); return navigate('inbounds');
        case 'toggle-inbound':
          await api(`inbounds/${id}/${button.dataset.enabled === 'true' ? 'disable' : 'enable'}`, {method: 'POST'});
          toast('Inbound state تغییر کرد'); return navigate('inbounds');
        case 'delete-inbound':
          if (await confirmDelete('Inbound حذف شود؟ Routing referenceها باید قبلاً پاک شده باشند.')) {
            await api(`inbounds/${id}`, {method: 'DELETE'}); toast('Inbound حذف شد'); return navigate('inbounds');
          }
          return;

        case 'outbound-detail': return showOutboundDetail(id);
        case 'probe-outbound':
          await api(`outbounds/${id}/probe`, {method: 'POST'}); toast('Egress probe انجام شد'); return navigate('outbounds');
        case 'redeploy-outbound':
          await api(`outbounds/${id}/redeploy`, {method: 'POST'}); toast('Outbound deploy شد'); return navigate('outbounds');
        case 'toggle-outbound':
          await api(`outbounds/${id}/${button.dataset.enabled === 'true' ? 'disable' : 'enable'}`, {method: 'POST'});
          toast('Outbound state تغییر کرد'); return navigate('outbounds');
        case 'delete-outbound':
          if (await confirmDelete('Outbound حذف شود؟')) {
            await api(`outbounds/${id}`, {method: 'DELETE'}); toast('Outbound حذف شد'); return navigate('outbounds');
          }
          return;

        case 'group-detail': return showGroupDetail(id);
        case 'delete-group':
          if (await confirmDelete('Failover group حذف شود؟')) {
            await api(`outbound-groups/${id}`, {method: 'DELETE'}); toast('Group حذف شد'); return navigate('groups');
          }
          return;

        case 'delete-routing':
          if (await confirmDelete('Routing rule حذف شود؟')) {
            await api(`routing/${id}`, {method: 'DELETE'}); toast('Rule حذف شد'); return navigate('routing');
          }
          return;

        case 'sync-traffic': return syncTraffic();
        case 'load-logs': return loadLogs();
        case 'save-settings': return saveSettings();
      }
    } catch (error) {
      toast(error.message, 'error');
    }
  });

  document.addEventListener('change', event => {
    if (event.target.matches('[data-role="outbound-protocol"]')) {
      renderOutboundFields(event.target.value);
    }
    if (event.target.matches('[data-role="group-node"]')) {
      refreshGroupCandidates();
    }
    if (event.target.matches('[data-role="route-node"], [data-role="route-target-type"]')) {
      refreshRoutingCandidates();
    }
  });

  document.addEventListener('keydown', event => {
    if (event.key === 'Escape' && !$('#modal').classList.contains('hidden')) closeModal();
    if (event.key === 'Enter' && !$('#login-card').classList.contains('hidden')) login();
  });

  window.addEventListener('hashchange', () => {
    const next = location.hash.replace(/^#\/?/, '');
    if (next && next !== state.page) navigate(next);
  });

  bootstrap();
})();