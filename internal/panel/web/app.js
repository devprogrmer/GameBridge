(() => {
  'use strict';

  const $ = (selector, root = document) => root.querySelector(selector);
  const $$ = (selector, root = document) => [...root.querySelectorAll(selector)];

  const state = {
    me: null,
    page: 'dashboard',
    filter: '',
    confirmResolve: null,
    live: {enabled: true, intervalMs: 15000, timer: null},
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


  function pct(used, limit) {
    const u = Number(used || 0), l = Number(limit || 0);
    if (!l) return 0;
    return Math.max(0, Math.min(100, Math.round((u / l) * 100)));
  }

  function progressMeter(value, label = '') {
    const p = Math.max(0, Math.min(100, Math.round(Number(value || 0))));
    return `<div class="usage"><progress class="usage-progress" max="100" value="${p}" aria-label="${esc(label || `${p}%`)}">${p}%</progress>${label ? `<small>${esc(label)}</small>` : ''}</div>`;
  }

  function usageBar(used, limit) {
    const p = pct(used, limit);
    const label = limit ? `${fmtBytes(used)} / ${fmtBytes(limit)}` : `${fmtBytes(used)} / Unlimited`;
    return progressMeter(p, label);
  }

  function donutChart(value, label, detail = '') {
    const p = Math.max(0, Math.min(100, Math.round(Number(value || 0))));
    return `<div class="donut-chart" role="img" aria-label="${esc(label)} ${p}%">
      <svg viewBox="0 0 42 42" aria-hidden="true">
        <circle class="donut-track" cx="21" cy="21" r="15.9155" pathLength="100"></circle>
        <circle class="donut-value" cx="21" cy="21" r="15.9155" pathLength="100" stroke-dasharray="${p} ${100-p}" transform="rotate(-90 21 21)"></circle>
      </svg>
      <div><strong>${p}%</strong><span>${esc(label)}</span>${detail ? `<small>${esc(detail)}</small>` : ''}</div>
    </div>`;
  }

  function trafficChart(rows) {
    const top = [...(rows || [])].sort((a,b) => Number(b.traffic_bytes||0)-Number(a.traffic_bytes||0)).slice(0, 8);
    if (!top.length) return '<div class="empty">Traffic sample وجود ندارد.</div>';
    const max = Math.max(1, ...top.map(x => Number(x.traffic_bytes || 0)));
    const width = 720, height = 230, baseY = 180, chartH = 145, barW = 48, gap = 34, startX = 45;
    const bars = top.map((u, i) => {
      const total = Number(u.traffic_bytes || 0);
      const xray = Math.min(total, Number(u.xray_bytes || 0));
      const wg = Math.min(Math.max(0,total-xray), Number(u.wireguard_bytes || 0));
      const totalH = Math.max(2, Math.round((total/max)*chartH));
      const xrayH = total ? Math.round((xray/total)*totalH) : 0;
      const wgH = Math.max(0, totalH-xrayH);
      const x = startX + i*(barW+gap);
      const label = String(u.username || 'user').slice(0, 10);
      return `<g>
        <rect class="chart-bar xray" x="${x}" y="${baseY-xrayH}" width="${barW}" height="${xrayH}" rx="6"></rect>
        <rect class="chart-bar wireguard" x="${x}" y="${baseY-totalH}" width="${barW}" height="${wgH}" rx="6"></rect>
        <text class="chart-label" x="${x+barW/2}" y="202" text-anchor="middle">${esc(label)}</text>
      </g>`;
    }).join('');
    return `<div class="chart-shell"><div class="chart-legend"><span><i class="xray"></i>Xray</span><span><i class="wireguard"></i>WireGuard</span></div><svg class="traffic-chart" viewBox="0 0 ${width} ${height}" role="img" aria-label="Top traffic users">${bars}</svg></div>`;
  }

  function fleetChart(nodes) {
    const rows = (nodes || []).slice(0, 8);
    if (!rows.length) return '<div class="empty">Node metric وجود ندارد.</div>';
    return `<div class="fleet-chart">${rows.map(n => {
      const ram = n.metrics?.memory_total ? Math.round(100*(1-n.metrics.memory_available/n.metrics.memory_total)) : 0;
      const disk = n.metrics?.disk_total ? Math.round(100*(1-n.metrics.disk_free/n.metrics.disk_total)) : 0;
      return `<div class="fleet-chart-row"><div><strong>${esc(n.name)}</strong><small>${esc(n.status)}</small></div><span>RAM ${ram}%</span><progress max="100" value="${ram}" aria-label="RAM ${ram}%"></progress><span>Disk ${disk}%</span><progress max="100" value="${disk}" aria-label="Disk ${disk}%"></progress></div>`;
    }).join('')}</div>`;
  }

  function iconTile(icon, tone = '') {
    return `<span class="resource-icon ${esc(tone)}">${esc(icon)}</span>`;
  }

  function pageHero(kicker, title, text, stats = [], actions = '') {
    return `<section class="page-hero">
      <div class="hero-copy"><span class="eyebrow">${esc(kicker)}</span><h3>${esc(title)}</h3><p>${esc(text)}</p></div>
      <div class="hero-stats">${stats.map(x => `<div><strong>${esc(x[0])}</strong><span>${esc(x[1])}</span></div>`).join('')}</div>
      <div class="hero-actions">${actions}</div>
    </section>`;
  }

  function filterToolbar(placeholder, meta = '') {
    return `<div class="resource-toolbar">
      <div class="resource-search"><span>⌕</span><input class="page-filter" placeholder="${esc(placeholder)}"></div>
      <div class="resource-meta">${esc(meta)}</div>
    </div>`;
  }

  function applyPageFilter(value) {
    state.filter = String(value || '');
    const q = state.filter.trim().toLowerCase();
    $$('.filterable').forEach(el => {
      const hay = (el.dataset.search || el.textContent || '').toLowerCase();
      el.classList.toggle('hidden-by-filter', !!q && !hay.includes(q));
    });
  }

  function resourceActions(items) {
    return `<div class="resource-actions">${items.join('')}</div>`;
  }

  function kv(label, value, extra = '') {
    return `<div class="kv"><span>${esc(label)}</span><strong class="${esc(extra)}">${esc(value ?? '-')}</strong></div>`;
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
    updateLiveControl();
    startLiveRefresh();
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
    stopLiveRefresh();
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
    if (state.confirmResolve) {
      const resolve = state.confirmResolve;
      state.confirmResolve = null;
      resolve(false);
    }
    $('#modal').classList.add('hidden');
    $('#modal-body').innerHTML = '';
  }

  function settleConfirm(value) {
    const resolve = state.confirmResolve;
    state.confirmResolve = null;
    $('#modal').classList.add('hidden');
    $('#modal-body').innerHTML = '';
    if (resolve) resolve(Boolean(value));
  }

  function confirmAction(message, title = 'تأیید عملیات') {
    return new Promise(resolve => {
      state.confirmResolve = resolve;
      modal(title, `<div class="confirm-panel">
        <div class="confirm-icon">!</div>
        <div><h4>این عملیات نیاز به تأیید دارد</h4><p>${esc(message)}</p></div>
      </div>
      <div class="confirm-actions"><button class="btn ghost" data-action="confirm-cancel">انصراف</button><button class="btn danger" data-action="confirm-accept">تأیید و ادامه</button></div>`, 'SAFE ACTION');
    });
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

  const renderers = {
    dashboard: renderDashboard, users: renderUsers, plans: renderPlans, nodes: renderNodes,
    inbounds: renderInbounds, outbounds: renderOutbounds, groups: renderGroups, routing: renderRouting,
    online: renderOnline, traffic: renderTraffic, tunnels: renderTunnels, forwards: renderForwards,
    logs: renderLogs, audit: renderAudit, admins: renderAdmins, settings: renderSettings
  };

  async function renderPage(page) {
    await (renderers[page] || renderDashboard)();
    if (state.filter) applyPageFilter(state.filter);
  }

  async function navigate(page) {
    if (!pages[page]) page = 'dashboard';
    if (state.page !== page) {
      state.filter = '';
      if ($('#global-search')) $('#global-search').value = '';
    }
    setPageChrome(page);
    history.replaceState(null, '', '#/' + page);
    try {
      await renderPage(page);
    } catch (error) {
      $('#content').innerHTML = emptyState('خطا در بارگذاری', error.message);
      toast(error.message, 'error');
    }
  }

  const livePages = new Set(['dashboard','nodes','outbounds','groups','online','traffic']);

  function updateLiveControl() {
    const button = $('#live-toggle');
    if (!button) return;
    button.classList.toggle('active', state.live.enabled);
    button.setAttribute('aria-pressed', state.live.enabled ? 'true' : 'false');
    button.innerHTML = `<span></span> ${state.live.enabled ? 'LIVE · 15s' : 'PAUSED'}`;
  }

  function startLiveRefresh() {
    if (state.live.timer) return;
    state.live.timer = window.setInterval(async () => {
      if (!state.live.enabled || document.hidden || !livePages.has(state.page)) return;
      if (!$('#modal').classList.contains('hidden')) return;
      try { await renderPage(state.page); } catch {}
    }, state.live.intervalMs);
  }

  function stopLiveRefresh() {
    if (state.live.timer) window.clearInterval(state.live.timer);
    state.live.timer = null;
  }

  function toggleLiveRefresh() {
    state.live.enabled = !state.live.enabled;
    updateLiveControl();
    toast(state.live.enabled ? 'Live refresh فعال شد' : 'Live refresh متوقف شد', 'success');
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
    const enabledOut = state.cache.outbounds.filter(x => x.enabled);
    const healthy = enabledOut.filter(x => x.health_status === 'healthy').length;
    const unhealthy = enabledOut.filter(x => x.health_status === 'unhealthy').length;
    const onlineNodes = state.cache.nodes.filter(x => x.status === 'online').length;
    const enabledInbounds = state.cache.inbounds.filter(x => x.enabled).length;
    const healthRate = enabledOut.length ? Math.round((healthy / enabledOut.length) * 100) : 0;

    $('#content').innerHTML = `
      ${pageHero('GAMEBRIDGE NETWORK OS', 'مرکز فرمان شبکه', 'وضعیت زیرساخت، ورودی‌ها، خروجی‌ها و Failover را از یک نقطه کنترل کنید.', [
        [`${onlineNodes}/${state.cache.nodes.length}`, 'Nodes online'],
        [`${healthy}/${enabledOut.length}`, 'Healthy egress'],
        [`${state.cache.groups.length}`, 'Failover groups']
      ], '<button class="btn primary" data-page="outbounds">Open Egress Center</button>')}

      <div class="grid metrics premium-metrics">
        ${metricCard('Users', dashboard.users || 0, fmtBytes(dashboard.traffic_bytes || 0) + ' consumed', '◎')}
        ${metricCard('Node Fleet', `${onlineNodes}/${state.cache.nodes.length}`, 'Agents reachable', '◉')}
        ${metricCard('Ingress', `${enabledInbounds}/${state.cache.inbounds.length}`, 'Listeners active', '⇢')}
        ${metricCard('Egress Health', `${healthRate}%`, `${unhealthy} unhealthy`, '◌')}
        ${metricCard('Routing', state.cache.routing.length, `${state.cache.groups.length} balancers`, '⌁')}
      </div>

      <div class="grid dashboard-layout">
        <div class="card fleet-panel">
          <div class="card-head">
            <div><span class="eyebrow">FLEET</span><h3>Node Fleet</h3><p>سلامت و مصرف منابع نودها</p></div>
            <button class="btn small ghost" data-page="nodes">Manage Nodes</button>
          </div>
          <div class="node-fleet">
            ${state.cache.nodes.map(n => {
              const ram = n.metrics?.memory_total ? Math.round(100 * (1 - n.metrics.memory_available / n.metrics.memory_total)) : 0;
              const load = Number(n.metrics?.load_1 || 0);
              return `<article class="fleet-node filterable" data-search="${esc(`${n.name} ${n.public_ip} ${n.status}`)}">
                <div class="fleet-node-head">${iconTile('N', n.status)}<div><strong>${esc(n.name)}</strong><small>${esc(n.public_ip || n.agent_url || '-')}</small></div>${status(n.status)}</div>
                <div class="mini-metrics">
                  <div><span>RAM</span><strong>${ram || 0}%</strong></div>
                  <div><span>LOAD</span><strong>${load.toFixed(2)}</strong></div>
                  <div><span>FAIL</span><strong>${Number(n.failure_count || 0)}</strong></div>
                </div>
                ${progressMeter(Math.min(100, ram || 0))}
              </article>`;
            }).join('') || '<div class="empty">نودی اضافه نشده است.</div>'}
          </div>
        </div>

        <div class="card egress-panel">
          <div class="card-head">
            <div><span class="eyebrow">EGRESS</span><h3>Outbound Health</h3><p>Probe واقعی از مسیر خروجی</p></div>
            <span class="health-score">${healthRate}%</span>
          </div>
          <div class="egress-stack">
            ${state.cache.outbounds.slice(0, 7).map(x => `<button class="egress-row filterable" data-action="outbound-detail" data-id="${esc(x.id)}" data-search="${esc(`${x.name} ${x.tag} ${x.protocol}`)}">
              <div>${iconTile((x.protocol || '?').slice(0, 1).toUpperCase(), x.health_status)}<span><strong>${esc(x.name)}</strong><small>${esc(x.protocol)} · ${esc(nodeName(x.node_id))}</small></span></div>
              <div>${latency(x.health_latency_ms)}${status(x.enabled ? (x.health_status || 'unknown') : 'disabled')}</div>
            </button>`).join('') || '<div class="empty">Outbound تعریف نشده است.</div>'}
          </div>
        </div>

        <div class="card span-full analytics-panel">
          <div class="card-head"><div><span class="eyebrow">LIVE ANALYTICS</span><h3>Fleet & Egress Snapshot</h3><p>نمودارهای داده‌محور بدون CDN؛ هر 15 ثانیه در حالت Live به‌روزرسانی می‌شوند.</p></div><span class="live-badge"><i></i> LIVE</span></div>
          <div class="analytics-grid">
            <div class="chart-card">${donutChart(healthRate, 'Healthy egress', `${healthy}/${enabledOut.length} enabled`)}</div>
            <div class="chart-card"><h4>Node utilization</h4>${fleetChart(state.cache.nodes)}</div>
          </div>
        </div>

        <div class="card span-full topology-panel">
          <div class="card-head"><div><span class="eyebrow">TOPOLOGY</span><h3>Traffic Architecture</h3><p>نمای خلاصه‌ی مسیرهای GameBridge</p></div></div>
          <div class="topology-strip">
            <div class="topology-stage"><span>01</span><strong>${state.cache.inbounds.length}</strong><small>Inbounds</small></div>
            <i>→</i>
            <div class="topology-stage"><span>02</span><strong>${state.cache.routing.length}</strong><small>Routing Rules</small></div>
            <i>→</i>
            <div class="topology-stage"><span>03</span><strong>${state.cache.groups.length}</strong><small>Failover Groups</small></div>
            <i>→</i>
            <div class="topology-stage"><span>04</span><strong>${state.cache.outbounds.length}</strong><small>Outbounds</small></div>
          </div>
        </div>
      </div>`;
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
    const active = state.cache.users.filter(u => u.status === 'active').length;
    const totalTraffic = state.cache.users.reduce((sum, u) => sum + Number(u.traffic_used_bytes || 0), 0);

    $('#content').innerHTML = `
      ${pageHero('SUBSCRIPTIONS', 'کاربران و دسترسی‌ها', 'چرخه عمر کاربر، سهمیه، پلن و مصرف را در یک نمای عملیاتی مدیریت کنید.', [
        [state.cache.users.length, 'Total users'], [active, 'Active'], [fmtBytes(totalTraffic), 'Traffic']
      ])}
      ${filterToolbar('جستجو با نام، ایمیل یا وضعیت...', `${active} active`)}
      <div class="resource-grid user-grid">
        ${state.cache.users.map(u => {
          const limit = Number(u.data_limit_bytes || u.entitlements?.data_limit_bytes || 0);
          return `<article class="resource-card user-card filterable" data-search="${esc(`${u.username} ${u.display_name || ''} ${u.email || ''} ${u.status}`)}">
            <div class="resource-card-head">
              <div class="resource-title">${iconTile((u.username || 'U').slice(0,1).toUpperCase(), u.status)}<div><strong>${esc(u.username)}</strong><small>${esc(u.display_name || u.email || 'GameBridge user')}</small></div></div>
              ${status(u.status)}
            </div>
            <div class="resource-kvs">
              ${kv('Plan', planByID.get(u.plan_id) || 'No plan')}
              ${kv('Devices', u.device_limit || u.entitlements?.device_limit || '-')}
              ${kv('Expires', fmtDate(u.expires_at))}
            </div>
            ${usageBar(u.traffic_used_bytes, limit)}
            ${resourceActions([
              `<button class="btn small ghost" data-action="user-detail" data-id="${esc(u.id)}">Details</button>`,
              `<button class="btn small ghost" data-action="edit-user" data-id="${esc(u.id)}">Edit</button>`,
              `<button class="btn small danger" data-action="delete-user" data-id="${esc(u.id)}">Delete</button>`
            ])}
          </article>`;
        }).join('') || emptyState('کاربری وجود ندارد', 'از + جدید اولین کاربر را بسازید.')}
      </div>`;
  }

  async function renderPlans() {
    state.cache.plans = await api('plans');
    const enabled = state.cache.plans.filter(p => p.enabled).length;
    $('#content').innerHTML = `
      ${pageHero('POLICY', 'پلن‌ها و سیاست مصرف', 'پلن‌ها را به‌صورت policy object مدیریت کنید؛ حجم، زمان، device limit و reset.', [
        [state.cache.plans.length, 'Plans'], [enabled, 'Enabled'], [state.cache.plans.reduce((s,p)=>s+Number(p.device_limit||0),0), 'Device slots']
      ])}
      ${filterToolbar('جستجوی پلن...', `${enabled} enabled`)}
      <div class="resource-grid plan-grid">
        ${state.cache.plans.map(p => `<article class="resource-card plan-card filterable" data-search="${esc(`${p.name} ${p.enabled ? 'active' : 'disabled'}`)}">
          <div class="resource-card-head"><div class="resource-title">${iconTile('P', p.enabled ? 'healthy':'disabled')}<div><strong>${esc(p.name)}</strong><small>${p.duration_days || 0} روز</small></div></div>${status(p.enabled ? 'active':'disabled')}</div>
          <div class="plan-volume"><strong>${p.data_limit_bytes ? fmtBytes(p.data_limit_bytes) : '∞'}</strong><span>Data allowance</span></div>
          <div class="resource-kvs">${kv('Devices', p.device_limit || 1)}${kv('Reset', p.reset_interval_days ? `${p.reset_interval_days} days` : 'Never')}${kv('Duration', p.duration_days ? `${p.duration_days} days` : 'No limit')}</div>
          ${resourceActions([
            `<button class="btn small ghost" data-action="plan-detail" data-id="${esc(p.id)}">Details</button>`,
            `<button class="btn small ghost" data-action="edit-plan" data-id="${esc(p.id)}">Edit</button>`,
            `<button class="btn small danger" data-action="delete-plan" data-id="${esc(p.id)}">Delete</button>`
          ])}
        </article>`).join('') || emptyState('پلنی وجود ندارد')}
      </div>`;
  }

  async function renderNodes() {
    state.cache.nodes = await api('nodes');
    const online = state.cache.nodes.filter(n => n.status === 'online').length;
    const maintenance = state.cache.nodes.filter(n => n.maintenance).length;
    $('#content').innerHTML = `
      ${pageHero('INFRASTRUCTURE', 'Node Fleet', 'نودهای ایران/خارج، Agent، ظرفیت، maintenance و health را از اینجا کنترل کنید.', [
        [state.cache.nodes.length, 'Nodes'], [online, 'Online'], [maintenance, 'Maintenance']
      ])}
      ${filterToolbar('جستجو با نام، IP، role یا status...', `${online}/${state.cache.nodes.length} online`)}
      <div class="resource-grid node-grid">
        ${state.cache.nodes.map(n => {
          const ram = n.metrics?.memory_total ? Math.round(100 * (1 - n.metrics.memory_available / n.metrics.memory_total)) : 0;
          const load = Number(n.metrics?.load_1 || 0);
          return `<article class="resource-card node-card filterable" data-search="${esc(`${n.name} ${n.public_ip || ''} ${n.role || ''} ${n.status}`)}">
            <div class="resource-card-head">
              <div class="resource-title">${iconTile('N', n.status)}<div><strong>${esc(n.name)}</strong><small class="mono">${esc(n.public_ip || n.agent_url || '-')}</small></div></div>
              ${status(n.maintenance ? 'maintenance' : n.status)}
            </div>
            <div class="node-gauges">
              <div><span>RAM</span><strong>${ram}%</strong>${progressMeter(ram, `RAM ${ram}%`)}</div>
              <div><span>LOAD</span><strong>${load.toFixed(2)}</strong>${progressMeter(Math.min(100, load*25), `Load ${load.toFixed(2)}`)}</div>
            </div>
            <div class="resource-kvs">${kv('Role', n.role || '-')}${kv('Core', n.metrics?.core_version || '-')}${kv('Failures', n.failure_count || 0)}</div>
            ${resourceActions([
              `<button class="btn small ghost" data-action="node-detail" data-id="${esc(n.id)}">Details</button>`,
              `<button class="btn small ghost" data-action="probe-node" data-id="${esc(n.id)}">Probe</button>`,
              `<button class="btn small ${n.maintenance ? 'ghost':'warn'}" data-action="toggle-maintenance" data-id="${esc(n.id)}" data-enabled="${!!n.maintenance}">${n.maintenance?'Exit maintenance':'Maintenance'}</button>`,
              `<button class="btn small ${n.enabled ? 'warn':'ghost'}" data-action="toggle-node" data-id="${esc(n.id)}" data-enabled="${!!n.enabled}">${n.enabled?'Disable':'Enable'}</button>`
            ])}
          </article>`;
        }).join('') || emptyState('نودی وجود ندارد')}
      </div>`;
  }

  async function renderInbounds() {
    const [inbounds, nodes] = await Promise.all([api('inbounds'), api('nodes')]);
    state.cache.inbounds = inbounds || [];
    state.cache.nodes = nodes || [];
    const enabled = state.cache.inbounds.filter(x => x.enabled).length;
    $('#content').innerHTML = `
      ${pageHero('INGRESS', 'Inbound Gateway', 'ورودی‌های Xray را با transport، TLS/REALITY و binding کاربران به‌صورت اختصاصی مدیریت کنید.', [
        [state.cache.inbounds.length, 'Listeners'], [enabled, 'Enabled'], [new Set(state.cache.inbounds.map(x=>x.node_id)).size, 'Nodes']
      ])}
      ${filterToolbar('جستجو با name، protocol، node، port...', `${enabled} enabled`)}
      <div class="resource-grid inbound-grid">
        ${state.cache.inbounds.map(x => `<article class="resource-card inbound-card filterable" data-search="${esc(`${x.name} ${x.protocol} ${nodeName(x.node_id)} ${x.port} ${x.tls_mode}`)}">
          <div class="resource-card-head">
            <div class="resource-title">${iconTile('⇢', x.enabled ? 'healthy':'disabled')}<div><strong>${esc(x.name)}</strong><small>${protocol(x.protocol)} ${esc(x.transport || 'tcp')}</small></div></div>
            ${status(x.enabled ? (x.status || 'configured'):'disabled')}
          </div>
          <div class="endpoint-box"><span>LISTEN</span><strong class="mono">${esc(x.listen)}:${esc(x.port)}</strong></div>
          <div class="resource-kvs">${kv('Node', nodeName(x.node_id))}${kv('Security', (x.tls_mode || 'none').toUpperCase())}${kv('Transport', x.transport || 'tcp')}</div>
          ${resourceActions([
            `<button class="btn small ghost" data-action="inbound-detail" data-id="${esc(x.id)}">Details</button>`,
            `<button class="btn small ghost" data-action="edit-inbound" data-id="${esc(x.id)}">Edit</button>`,
            `<button class="btn small ghost" data-action="redeploy-inbound" data-id="${esc(x.id)}">Deploy</button>`,
            `<button class="btn small ${x.enabled ? 'warn':'ghost'}" data-action="toggle-inbound" data-id="${esc(x.id)}" data-enabled="${x.enabled}">${x.enabled?'Disable':'Enable'}</button>`
          ])}
        </article>`).join('') || emptyState('Inbound تعریف نشده است')}
      </div>`;
  }

  async function renderOutbounds() {
    const [outbounds, nodes] = await Promise.all([api('outbounds'), api('nodes')]);
    state.cache.outbounds = outbounds || [];
    state.cache.nodes = nodes || [];
    const enabled = state.cache.outbounds.filter(x => x.enabled).length;
    const healthy = state.cache.outbounds.filter(x => x.enabled && x.health_status === 'healthy').length;
    const unhealthy = state.cache.outbounds.filter(x => x.enabled && x.health_status === 'unhealthy').length;
    $('#content').innerHTML = `
      ${pageHero('EGRESS', 'Outbound Control Center', 'خروجی‌های Native و Xray را با health probe، latency و deployment از یک مرکز مدیریت کنید.', [
        [state.cache.outbounds.length, 'Outbounds'], [healthy, 'Healthy'], [unhealthy, 'Unhealthy']
      ], '<button class="btn ghost" data-action="probe-all-outbounds">Probe all</button>')}
      ${filterToolbar('جستجو با name، tag، protocol، node...', `${healthy}/${enabled} healthy`)}
      <div class="resource-grid outbound-grid">
        ${state.cache.outbounds.map(x => `<article class="resource-card outbound-card filterable ${safeStatus(x.health_status)}" data-search="${esc(`${x.name} ${x.tag} ${x.protocol} ${nodeName(x.node_id)} ${x.health_status}`)}">
          <div class="resource-card-head">
            <div class="resource-title">${iconTile((x.protocol || '?').slice(0,1).toUpperCase(), x.health_status)}<div><strong>${esc(x.name)}</strong><small class="mono">${esc(x.tag)}</small></div></div>
            ${status(x.enabled ? (x.health_status || 'unknown') : 'disabled')}
          </div>
          <div class="outbound-protocol-line">${protocol(x.protocol)}<span>${latency(x.health_latency_ms)}</span></div>
          <div class="endpoint-box"><span>TARGET</span><strong class="mono">${esc(outboundTarget(x))}</strong></div>
          <div class="resource-kvs">${kv('Node', nodeName(x.node_id))}${kv('Failures', x.health_failure_count || 0)}${kv('Checked', fmtDate(x.health_last_checked_at))}</div>
          ${resourceActions([
            `<button class="btn small ghost" data-action="outbound-detail" data-id="${esc(x.id)}">Health</button>`,
            `<button class="btn small ghost" data-action="edit-outbound" data-id="${esc(x.id)}">Edit</button>`,
            `<button class="btn small ghost" data-action="probe-outbound" data-id="${esc(x.id)}">Probe</button>`,
            `<button class="btn small ${x.enabled ? 'warn':'ghost'}" data-action="toggle-outbound" data-id="${esc(x.id)}" data-enabled="${x.enabled}">${x.enabled?'Disable':'Enable'}</button>`
          ])}
        </article>`).join('') || emptyState('Outbound تعریف نشده است')}
      </div>`;
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
    const [groups, nodes, outbounds] = await Promise.all([api('outbound-groups'), api('nodes'), api('outbounds')]);
    state.cache.groups = groups || [];
    state.cache.nodes = nodes || [];
    state.cache.outbounds = outbounds || [];
    $('#content').innerHTML = `
      ${pageHero('RESILIENCE', 'Failover & Load Balancing', 'گروه‌های خروجی را با priority، weight، fallback و strategyهای Xray مدیریت کنید.', [
        [state.cache.groups.length, 'Groups'],
        [state.cache.groups.reduce((s,g)=>s+(g.members||[]).length,0), 'Members'],
        [state.cache.groups.filter(g=>g.enabled).length, 'Enabled']
      ])}
      ${filterToolbar('جستجوی group، strategy، node...', `${state.cache.groups.length} groups`)}
      <div class="group-grid">
        ${state.cache.groups.map(g => {
          const members = (g.members || []).map(m => state.cache.outbounds.find(o=>o.id===m.outbound_id)).filter(Boolean);
          return `<article class="failover-card filterable" data-search="${esc(`${g.name} ${g.strategy} ${nodeName(g.node_id)}`)}">
            <div class="failover-head"><div><span class="eyebrow">${esc((g.strategy || '').toUpperCase())}</span><h3>${esc(g.name)}</h3><p>${esc(nodeName(g.node_id))}</p></div>${status(g.enabled?'active':'disabled')}</div>
            <div class="failover-flow">
              <div class="flow-source">ROUTE</div><i>→</i><div class="flow-balancer">GB<span>${esc(g.expected || 1)}</span></div><i>→</i>
              <div class="flow-members">${members.slice(0,4).map(o=>`<span class="${safeStatus(o.health_status)}" title="${esc(o.name)}">${esc((o.protocol||'?').slice(0,1).toUpperCase())}</span>`).join('') || '<span>?</span>'}</div>
              <i>→</i><div class="flow-fallback">${esc(outboundName(g.fallback_outbound_id || 'blocked'))}</div>
            </div>
            <div class="resource-kvs">${kv('Members',(g.members||[]).length)}${kv('Expected',g.expected||1)}${kv('Fallback',outboundName(g.fallback_outbound_id||'blocked'))}</div>
            ${resourceActions([
              `<button class="btn small ghost" data-action="group-detail" data-id="${esc(g.id)}">Topology</button>`,
              `<button class="btn small ghost" data-action="edit-group" data-id="${esc(g.id)}">Edit</button>`,
              `<button class="btn small danger" data-action="delete-group" data-id="${esc(g.id)}">Delete</button>`
            ])}
          </article>`;
        }).join('') || emptyState('Failover Group وجود ندارد')}
      </div>`;
  }

  async function renderRouting() {
    await loadCore();
    const enabled = state.cache.routing.filter(r=>r.enabled).length;
    $('#content').innerHTML = `
      ${pageHero('POLICY ROUTING', 'Routing Rules', 'قوانین را بر اساس inbound، user، domain، IP، port و protocol به egress یا failover group وصل کنید.', [
        [state.cache.routing.length, 'Rules'], [enabled, 'Enabled'], [state.cache.groups.length, 'Group targets']
      ])}
      ${filterToolbar('جستجو با rule، target، node یا match...', `${enabled} enabled`)}
      <div class="routing-stack">
        ${state.cache.routing.map(r => {
          const target = r.outbound_group_id ? `Group · ${groupName(r.outbound_group_id)}` : outboundName(r.outbound_id);
          const chips = [
            ...(r.inbound_ids||[]).map(()=> 'Inbound'),
            ...(r.user_ids||[]).map(()=> 'User'),
            ...(r.domains||[]).slice(0,2),
            ...(r.ips||[]).slice(0,2),
            r.ports ? `Ports ${r.ports}` : '',
            r.network || ''
          ].filter(Boolean);
          return `<article class="route-card filterable" data-search="${esc(`${r.name} ${target} ${nodeName(r.node_id)} ${chips.join(' ')}`)}">
            <div class="route-priority">${esc(r.priority)}</div>
            <div class="route-main"><div class="route-head"><div><strong>${esc(r.name)}</strong><small>${esc(nodeName(r.node_id))}</small></div>${status(r.enabled?'active':'disabled')}</div>
              <div class="match-chips">${chips.map(c=>`<span>${esc(c)}</span>`).join('') || '<span>Any traffic</span>'}</div>
            </div>
            <div class="route-arrow">→</div>
            <div class="route-target"><span>${r.outbound_group_id?'FAILOVER':'OUTBOUND'}</span><strong>${esc(target)}</strong></div>
            ${resourceActions([
              `<button class="btn small ghost" data-action="routing-detail" data-id="${esc(r.id)}">Details</button>`,
              `<button class="btn small ghost" data-action="edit-routing" data-id="${esc(r.id)}">Edit</button>`,
              `<button class="btn small danger" data-action="delete-routing" data-id="${esc(r.id)}">Delete</button>`
            ])}
          </article>`;
        }).join('') || emptyState('Routing Rule وجود ندارد')}
      </div>`;
  }

  async function renderOnline() {
    const rows = await api('online-users');
    const total = rows.reduce((s,u)=>s+Number(u.traffic_bytes||0),0);
    $('#content').innerHTML = `
      ${pageHero('LIVE SESSIONS', 'Online Users', 'کاربران فعال و آخرین فعالیت ثبت‌شده در مسیرهای Xray و WireGuard.', [
        [rows.length,'Online'], [fmtBytes(total),'Traffic'], [rows.filter(x=>x.status==='active').length,'Active']
      ], '<button class="btn ghost" data-action="sync-traffic">Sync Traffic</button>')}
      ${filterToolbar('جستجوی کاربر...', `${rows.length} online`)}
      <div class="live-user-grid">
        ${rows.map(u=>`<article class="live-user filterable" data-search="${esc(`${u.username} ${u.status}`)}">
          <div class="live-user-head">${iconTile((u.username||'U').slice(0,1).toUpperCase(),u.status)}<div><strong>${esc(u.username)}</strong><small>${fmtDate(u.last_online_at)}</small></div>${status(u.status)}</div>
          <div class="traffic-split"><div><span>Xray</span><strong>${fmtBytes(u.xray_bytes)}</strong></div><div><span>WireGuard</span><strong>${fmtBytes(u.wireguard_bytes)}</strong></div><div><span>Total</span><strong>${fmtBytes(u.traffic_bytes)}</strong></div></div>
        </article>`).join('') || emptyState('کاربر آنلاین نیست')}
      </div>`;
  }

  async function renderTraffic() {
    const rows = await api('traffic');
    const total = rows.reduce((s,u)=>s+Number(u.traffic_bytes||0),0);
    $('#content').innerHTML = `
      ${pageHero('ACCOUNTING', 'Traffic Analytics', 'مصرف تجمیعی کاربران، سهم Xray/WireGuard و زمان reset بعدی.', [
        [rows.length,'Accounts'], [fmtBytes(total),'Total traffic'], [rows.filter(x=>Number(x.limit_bytes||0)>0).length,'Quota controlled']
      ], '<button class="btn ghost" data-action="sync-traffic">Sync now</button>')}
      ${filterToolbar('جستجوی user یا status...', `${fmtBytes(total)} total`)}
      <div class="card traffic-analytics-card"><div class="card-head"><div><span class="eyebrow">BREAKDOWN</span><h3>Top Consumers</h3><p>تقسیم مصرف Xray و WireGuard برای کاربران پرمصرف</p></div></div>${trafficChart(rows)}</div>
      <div class="traffic-list">
        ${rows.map(u => `<article class="traffic-card filterable" data-search="${esc(`${u.username} ${u.status}`)}">
          <div class="traffic-user"><div>${iconTile((u.username||'U').slice(0,1).toUpperCase(),u.status)}<span><strong>${esc(u.username)}</strong><small>${status(u.status)}</small></span></div><strong>${fmtBytes(u.traffic_bytes)}</strong></div>
          ${usageBar(u.traffic_bytes,u.limit_bytes)}
          <div class="traffic-breakdown"><span>Xray <b>${fmtBytes(u.xray_bytes)}</b></span><span>WireGuard <b>${fmtBytes(u.wireguard_bytes)}</b></span><span>Reset <b>${fmtDate(u.next_reset_at)}</b></span></div>
        </article>`).join('') || emptyState('داده‌ای وجود ندارد')}
      </div>`;
  }

  async function renderTunnels() {
    const [tunnels, nodes] = await Promise.all([api('tunnels'), api('nodes')]);
    state.cache.tunnels = tunnels || [];
    state.cache.nodes = nodes || [];
    $('#content').innerHTML = `
      ${pageHero('BACKBONE', 'GameBridge Tunnels', 'مسیرهای Server-to-Server را به‌صورت توپولوژی source → destination ببینید.', [
        [state.cache.tunnels.length,'Tunnels'], [state.cache.tunnels.filter(t=>['online','created'].includes(t.status)).length,'Active'], [state.cache.nodes.length,'Nodes']
      ])}
      ${filterToolbar('جستجو با tunnel، node یا transport...', `${state.cache.tunnels.length} tunnels`)}
      <div class="tunnel-grid">
        ${state.cache.tunnels.map(t=>`<article class="tunnel-card filterable" data-search="${esc(`${t.name} ${t.transport} ${nodeName(t.source_node_id)} ${nodeName(t.destination_node_id)} ${t.status}`)}">
          <div class="tunnel-head"><div><strong>${esc(t.name)}</strong><small>${protocol(t.transport)} · ${esc(t.profile||'-')}</small></div>${status(t.status)}</div>
          <div class="tunnel-path"><div><span>SOURCE</span><strong>${esc(nodeName(t.source_node_id))}</strong></div><i>⟶</i><div><span>DESTINATION</span><strong>${esc(nodeName(t.destination_node_id))}</strong></div></div>
          <div class="resource-kvs">${kv('CIDR',t.cidr||'-')}${kv('MTU',t.mtu||'-')}${kv('Ports',(t.ports||[]).length||'-')}</div>
        </article>`).join('') || emptyState('Tunnel وجود ندارد')}
      </div>`;
  }

  async function renderForwards() {
    const [rows, nodes] = await Promise.all([api('forwards'), api('nodes')]);
    state.cache.nodes = nodes || [];
    $('#content').innerHTML = `
      ${pageHero('FORWARDING', 'Port Forward', 'Forwardهای TCP/UDP را با مسیر ورودی و مقصد در یک نمای ساده بررسی کنید.', [
        [rows.length,'Rules'], [rows.filter(f=>f.enabled!==false).length,'Active'], [new Set(rows.map(f=>f.node_id)).size,'Nodes']
      ])}
      ${filterToolbar('جستجو با node، protocol یا destination...', `${rows.length} forwards`)}
      <div class="forward-grid">
        ${rows.map(f=>`<article class="forward-card filterable" data-search="${esc(`${nodeName(f.node_id)} ${f.protocol} ${f.dest_ip} ${f.dest_port}`)}">
          <div>${iconTile('↪', f.enabled===false?'disabled':'healthy')}<span><strong>${esc(nodeName(f.node_id))}</strong><small>${protocol(f.protocol)}</small></span>${status(f.enabled===false?'disabled':'active')}</div>
          <div class="forward-path"><strong class="mono">:${esc(f.listen_port)}</strong><i>→</i><strong class="mono">${esc(f.dest_ip)}:${esc(f.dest_port)}</strong></div>
        </article>`).join('') || emptyState('Port Forward وجود ندارد')}
      </div>`;
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
    $('#content').innerHTML = `
      ${pageHero('GOVERNANCE', 'Audit Timeline', 'تمام تغییرات مدیریتی، actor، object و IP را به‌صورت timeline مشاهده کنید.', [
        [rows.length,'Events'], [new Set(rows.map(x=>x.admin_name).filter(Boolean)).size,'Admins'], [new Set(rows.map(x=>x.action).filter(Boolean)).size,'Actions']
      ])}
      ${filterToolbar('جستجو در action، object، admin یا IP...', `${rows.length} events`)}
      <div class="audit-timeline">
        ${rows.map(x=>`<article class="audit-event filterable" data-search="${esc(`${x.admin_name} ${x.action} ${x.object} ${x.remote_ip}`)}">
          <span class="audit-dot"></span><div class="audit-time">${esc(fmtDate(x.created_at))}</div>
          <div class="audit-body"><div><strong>${esc(x.admin_name||'-')}</strong>${protocol(x.action)}</div><p>${esc(x.object||'-')}</p><small class="mono">${esc(x.remote_ip||'-')}</small></div>
        </article>`).join('') || emptyState('Audit event وجود ندارد')}
      </div>`;
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

  function setInput(selector, value) {
    const el = $(selector);
    if (el) el.value = value ?? '';
  }

  function setChecked(selector, value) {
    const el = $(selector);
    if (el) el.checked = Boolean(value);
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
    const [summary, health] = await Promise.all([api(`outbounds/${id}/summary`),api(`outbounds/${id}/health`)]);
    const out = summary.outbound || {};
    const healthValue = (health.health_status || summary.health_status) === 'healthy' ? 100 : 0;
    modal(out.name || 'Outbound', `
      <div class="outbound-detail-layout">
        <div class="card">${donutChart(healthValue, health.health_status || summary.health_status || 'unknown', health.health_latency_ms ? `${health.health_latency_ms} ms` : 'No latency')}</div>
        <div class="grid three-col">
          ${metricCard('Latency', health.health_latency_ms ? `${health.health_latency_ms} ms` : '-', `Failures: ${health.health_failure_count || 0}`, '↯')}
          ${metricCard('Routing', summary.enabled_rules || 0, `${summary.routing_rules || 0} total rules`, '⌁')}
          ${metricCard('Node', nodeName(out.node_id), summary.node_status || 'unknown', '◉')}
        </div>
      </div>
      <div class="card"><div class="stat-list">
        <div class="stat-line"><span>Protocol</span>${protocol(out.protocol)}</div>
        <div class="stat-line"><span>Tag</span><span class="mono">${esc(out.tag)}</span></div>
        <div class="stat-line"><span>Target</span><span class="mono">${esc(outboundTarget(out))}</span></div>
        <div class="stat-line"><span>Last checked</span><span>${fmtDate(health.health_last_checked_at)}</span></div>
        <div class="stat-line"><span>Last success</span><span>${fmtDate(health.health_last_success_at)}</span></div>
        <div class="stat-line"><span>Last error</span><span>${esc(health.health_last_error || '-')}</span></div>
      </div></div>`, 'OUTBOUND TELEMETRY');
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


  async function showUserDetail(id) {
    const d = await api(`users/${id}`);
    const u = d.user || d, ent = d.entitlements || {};
    const limit=Number(u.data_limit_bytes||ent.data_limit_bytes||0);
    const quota=limit?pct(u.traffic_used_bytes,limit):0;
    modal(u.username || 'User', `
      <div class="detail-banner"><div>${iconTile((u.username||'U').slice(0,1).toUpperCase(),u.status)}<span><strong>${esc(u.username)}</strong><small>${esc(u.display_name||u.email||'')}</small></span></div>${status(u.status)}</div>
      <div class="user-detail-layout"><div class="card">${donutChart(quota, limit?'Quota used':'Unlimited', limit?`${fmtBytes(u.traffic_used_bytes)} / ${fmtBytes(limit)}`:fmtBytes(u.traffic_used_bytes))}</div>
      <div class="grid three-col detail-metrics">${metricCard('Traffic',fmtBytes(u.traffic_used_bytes),'Consumed','↕')}${metricCard('Devices',u.device_limit||ent.device_limit||'-','Allowed','◎')}${metricCard('Expires',fmtDate(u.expires_at),'Lifecycle','◷')}</div></div>
      <div class="card"><div class="resource-kvs">${kv('Plan',u.plan_id||'-')}${kv('Email',u.email||'-')}${kv('Xray',fmtBytes(u.xray_traffic_bytes||0))}${kv('WireGuard',fmtBytes(u.wireguard_traffic_bytes||0))}${kv('Reset',fmtDate(u.next_traffic_reset_at))}${kv('Last online',fmtDate(u.last_online_at))}</div></div>`, 'USER TELEMETRY');
  }

  async function openUserEdit(id) {
    const d = await api(`users/${id}`);
    const u = d.user || d;
    state.cache.plans = await api('plans');
    modal('ویرایش کاربر', `<div class="form-grid">
      <label class="field"><span>Username</span><input id="edit-user-name" value="${esc(u.username||'')}"></label>
      <label class="field"><span>Display name</span><input id="edit-user-display" value="${esc(u.display_name||'')}"></label>
      <label class="field"><span>Email</span><input id="edit-user-email" value="${esc(u.email||'')}"></label>
      <label class="field"><span>Plan</span><select id="edit-user-plan">${planOptions(u.plan_id||'')}</select></label>
      <label class="field"><span>Data limit bytes</span><input id="edit-user-data" type="number" value="${Number(u.data_limit_bytes||0)}"></label>
      <label class="field"><span>Device limit</span><input id="edit-user-devices" type="number" value="${Number(u.device_limit||1)}"></label>
      <label class="field"><span>Reset days</span><input id="edit-user-reset" type="number" value="${Number(u.reset_interval_days||0)}"></label>
      <div class="span-2"><button class="btn primary wide" data-action="save-user" data-id="${esc(id)}">Save User</button></div>
    </div>`, 'USER POLICY');
  }

  async function saveUser(id) {
    await api(`users/${id}`, {method:'PUT', body:{
      username: val('#edit-user-name').trim(), display_name: val('#edit-user-display').trim(), email: val('#edit-user-email').trim(),
      plan_id: val('#edit-user-plan'), data_limit_bytes: num('#edit-user-data'), device_limit: num('#edit-user-devices',1),
      reset_interval_days: num('#edit-user-reset')
    }});
    closeModal(); toast('User بروزرسانی شد'); navigate('users');
  }

  async function showPlanDetail(id) {
    const d = await api(`plans/${id}`);
    const p = d.plan || d;
    modal(p.name || 'Plan', `<div class="grid three-col detail-metrics">${metricCard('Users',d.assigned_users||0,'Assigned','◎')}${metricCard('Data',p.data_limit_bytes?fmtBytes(p.data_limit_bytes):'∞','Allowance','↕')}${metricCard('Devices',p.device_limit||1,'Per user','▣')}</div>
      <div class="card"><div class="resource-kvs">${kv('Duration',p.duration_days?`${p.duration_days} days`:'Unlimited')}${kv('Reset',p.reset_interval_days?`${p.reset_interval_days} days`:'Never')}${kv('Status',p.enabled?'Enabled':'Disabled')}</div></div>`, 'PLAN');
  }

  async function openPlanEdit(id) {
    const d = await api(`plans/${id}`); const p = d.plan || d;
    modal('ویرایش پلن', `<div class="form-grid">
      <label class="field"><span>Name</span><input id="edit-plan-name" value="${esc(p.name||'')}"></label>
      <label class="field"><span>Data bytes</span><input id="edit-plan-data" type="number" value="${Number(p.data_limit_bytes||0)}"></label>
      <label class="field"><span>Duration days</span><input id="edit-plan-days" type="number" value="${Number(p.duration_days||0)}"></label>
      <label class="field"><span>Device limit</span><input id="edit-plan-devices" type="number" value="${Number(p.device_limit||1)}"></label>
      <label class="field"><span>Reset days</span><input id="edit-plan-reset" type="number" value="${Number(p.reset_interval_days||0)}"></label>
      <label class="check-row"><input id="edit-plan-enabled" type="checkbox" ${p.enabled?'checked':''}> Enabled</label>
      <div class="span-2"><button class="btn primary wide" data-action="save-plan" data-id="${esc(id)}">Save Plan</button></div>
    </div>`, 'PLAN POLICY');
  }

  async function savePlan(id) {
    await api(`plans/${id}`, {method:'PUT', body:{
      name:val('#edit-plan-name').trim(), data_limit_bytes:num('#edit-plan-data'), duration_days:num('#edit-plan-days'),
      device_limit:num('#edit-plan-devices',1), reset_interval_days:num('#edit-plan-reset'), enabled:$('#edit-plan-enabled').checked
    }});
    closeModal(); toast('Plan بروزرسانی شد'); navigate('plans');
  }

  async function showNodeDetail(id) {
    const [d,m] = await Promise.all([api(`nodes/${id}`),api(`nodes/${id}/metrics`)]);
    const n=d.node||d, metrics=m.metrics||{};
    const ram=metrics.memory_total?Math.round(100*(1-metrics.memory_available/metrics.memory_total)):0;
    const disk=metrics.disk_total?Math.round(100*(1-metrics.disk_free/metrics.disk_total)):0;
    const uptime=Number(metrics.uptime_seconds||0);
    modal(n.name||'Node', `<div class="detail-banner"><div>${iconTile('N',n.status)}<span><strong>${esc(n.name)}</strong><small class="mono">${esc(n.public_ip||n.agent_url||'-')}</small></span></div>${status(n.maintenance?'maintenance':n.status)}</div>
      <div class="grid three-col detail-metrics">${metricCard('Load',Number(metrics.load_1||0).toFixed(2),'1 minute','↯')}${metricCard('RAM',`${ram}%`,fmtBytes(metrics.memory_total||0),'▣')}${metricCard('Disk',`${disk}%`,fmtBytes(metrics.disk_total||0),'◫')}</div>
      <div class="card node-detail-grid">
        <div>${donutChart(ram,'RAM used',fmtBytes((metrics.memory_total||0)-(metrics.memory_available||0)))}</div>
        <div>${donutChart(disk,'Disk used',fmtBytes((metrics.disk_total||0)-(metrics.disk_free||0)))}</div>
        <div class="stat-list">
          <div class="stat-line"><span>Network RX</span><strong>${fmtBytes(metrics.network_rx||0)}</strong></div>
          <div class="stat-line"><span>Network TX</span><strong>${fmtBytes(metrics.network_tx||0)}</strong></div>
          <div class="stat-line"><span>Uptime</span><strong>${Math.floor(uptime/3600)} h</strong></div>
          <div class="stat-line"><span>Core</span><strong>${esc(metrics.core_version||'-')}</strong></div>
        </div>
      </div>
      <div class="card"><div class="resource-kvs">${kv('Role',n.role||'-')}${kv('Interface',n.internet_interface||'-')}${kv('Agent URL',n.agent_url||'-')}${kv('Last seen',fmtDate(m.last_seen))}${kv('Failures',m.failure_count||0)}${kv('Last error',m.last_error||'-')}</div></div>`, 'NODE TELEMETRY');
  }

  async function openInboundEdit(id) {
    const d = await api(`inbounds/${id}`); const x=d.inbound||d;
    modal('ویرایش Inbound', `<div class="form-grid">
      <label class="field"><span>Name</span><input id="ei-name" value="${esc(x.name||'')}"></label>
      <label class="field"><span>Node ID</span><input id="ei-node" value="${esc(x.node_id||'')}"></label>
      <label class="field"><span>Protocol</span><input id="ei-protocol" value="${esc(x.protocol||'')}"></label>
      <label class="field"><span>Listen</span><input id="ei-listen" value="${esc(x.listen||'0.0.0.0')}"></label>
      <label class="field"><span>Port</span><input id="ei-port" type="number" value="${Number(x.port||443)}"></label>
      <label class="field"><span>Transport</span><input id="ei-transport" value="${esc(x.transport||'tcp')}"></label>
      <label class="field"><span>Security</span><input id="ei-tls" value="${esc(x.tls_mode||'none')}"></label>
      <label class="field"><span>Server name</span><input id="ei-server" value="${esc(x.server_name||'')}"></label>
      <label class="field"><span>Path</span><input id="ei-path" value="${esc(x.path||'/')}"></label>
      <label class="field"><span>Host</span><input id="ei-host" value="${esc(x.host||'')}"></label>
      <label class="field"><span>gRPC service</span><input id="ei-service" value="${esc(x.service_name||'gamebridge')}"></label>
      <label class="field"><span>TLS certificate</span><input id="ei-cert" value="${esc(x.cert_file||'')}"></label>
      <label class="field"><span>TLS key</span><input id="ei-key" value="${esc(x.key_file||'')}"></label>
      <label class="field"><span>REALITY destination</span><input id="ei-rdest" value="${esc(x.reality_dest||'')}"></label>
      <label class="field"><span>REALITY server names</span><input id="ei-rnames" value="${esc((x.reality_server_names||[]).join(','))}"></label>
      <label class="field"><span>REALITY short IDs</span><input id="ei-rids" value="${esc((x.reality_short_ids||[]).join(','))}"></label>
      <label class="field"><span>Fingerprint</span><input id="ei-fp" value="${esc(x.reality_fingerprint||'chrome')}"></label>
      <label class="field"><span>Shadowsocks method</span><input id="ei-ss" value="${esc(x.shadowsocks_method||'aes-128-gcm')}"></label>
      <label class="field span-2"><span>Remark</span><input id="ei-remark" value="${esc(x.remark||'')}"></label>
      <label class="check-row"><input id="ei-enabled" type="checkbox" ${x.enabled?'checked':''}> Enabled</label>
      <div class="span-2"><button class="btn primary wide" data-action="save-inbound" data-id="${esc(id)}">Save + Deploy</button></div>
    </div>`, 'INGRESS');
  }

  async function saveInbound(id) {
    await api(`inbounds/${id}`, {method:'PUT', body:{
      name:val('#ei-name').trim(), node_id:val('#ei-node').trim(), protocol:val('#ei-protocol').trim(),
      listen:val('#ei-listen').trim(), port:num('#ei-port'), transport:val('#ei-transport').trim(), tls_mode:val('#ei-tls').trim(),
      enabled:$('#ei-enabled').checked, server_name:val('#ei-server').trim(), path:val('#ei-path').trim(), host:val('#ei-host').trim(),
      service_name:val('#ei-service').trim(), cert_file:val('#ei-cert').trim(), key_file:val('#ei-key').trim(),
      reality_dest:val('#ei-rdest').trim(), reality_server_names:csv(val('#ei-rnames')), reality_short_ids:csv(val('#ei-rids')),
      reality_fingerprint:val('#ei-fp').trim() || 'chrome', shadowsocks_method:val('#ei-ss').trim() || 'aes-128-gcm', remark:val('#ei-remark').trim()
    }});
    closeModal(); toast('Inbound بروزرسانی و deploy شد'); navigate('inbounds');
  }


  async function openOutboundEdit(id) {
    const [x, nodes] = await Promise.all([api(`outbounds/${id}`), api('nodes')]);
    state.cache.nodes = nodes || [];
    modal('ویرایش Outbound', `<div class="form-grid">
      <label class="field"><span>Name</span><input id="out-name" value="${esc(x.name||'')}"></label>
      <label class="field"><span>Node</span><select id="out-node">${nodeOptions(x.node_id)}</select></label>
      <label class="field"><span>Tag</span><input id="out-tag" value="${esc(x.tag||'')}"></label>
      <label class="field"><span>Protocol</span><select id="out-protocol" data-role="outbound-protocol">
        ${['vless','vmess','trojan','shadowsocks','socks','http','wireguard','tor','openvpn','custom','freedom','blackhole'].map(p=>`<option value="${p}" ${p===x.protocol?'selected':''}>${p}</option>`).join('')}
      </select></label>
      <div id="outbound-fields" class="form-section"></div>
      <label class="field span-2"><span>Remark</span><input id="out-remark" value="${esc(x.remark||'')}"></label>
      <label class="check-row"><input id="out-enabled" type="checkbox" ${x.enabled?'checked':''}> Enabled</label>
      <div class="span-2"><button class="btn primary wide" data-action="save-outbound" data-id="${esc(id)}">Save + Deploy</button></div>
    </div>`, 'EGRESS EDIT');
    renderOutboundFields(x.protocol);
    setInput('#out-address', x.address); setInput('#out-port', x.port); setInput('#out-user', x.username);
    setInput('#out-transport', x.transport); setInput('#out-tls', x.tls_mode); setInput('#out-path', x.path);
    setInput('#out-host', x.host); setInput('#out-service', x.service_name); setInput('#out-server-name', x.server_name);
    setInput('#out-fingerprint', x.fingerprint); setInput('#out-flow', x.flow); setInput('#out-ss-method', x.shadowsocks_method);
    setInput('#out-reality-key', x.reality_public_key); setInput('#out-reality-short', x.reality_short_id);
    setChecked('#out-insecure', x.allow_insecure); setInput('#out-wg-interface', x.wireguard_interface);
    setInput('#out-wg-address', x.wireguard_address); setInput('#out-wg-peer', x.wireguard_peer_public_key);
    setInput('#out-wg-allowed', (x.wireguard_allowed_ips||[]).join(',')); setInput('#out-wg-keepalive', x.wireguard_keepalive);
    setInput('#out-wg-mtu', x.wireguard_mtu); setInput('#out-tor-port', x.tor_socks_port);
    setInput('#out-ovpn-interface', x.openvpn_interface); setInput('#out-ovpn-table', x.openvpn_routing_table);
    setInput('#out-ovpn-mark', x.openvpn_mark);
    const secret = $('#out-secret'); if (secret) secret.placeholder = 'خالی = credential فعلی حفظ می‌شود';
  }

  async function saveOutbound(id) {
    const p = val('#out-protocol');
    const body = {
      name: val('#out-name').trim(), node_id: val('#out-node'), tag: val('#out-tag').trim(), protocol:p,
      address: val('#out-address').trim(), port:num('#out-port'), username:val('#out-user').trim(),
      password:val('#out-password'), secret:val('#out-secret'), transport:val('#out-transport'), tls_mode:val('#out-tls'),
      path:val('#out-path').trim(), host:val('#out-host').trim(), service_name:val('#out-service').trim(),
      server_name:val('#out-server-name').trim(), allow_insecure:Boolean($('#out-insecure')?.checked),
      fingerprint:val('#out-fingerprint').trim(), flow:val('#out-flow'), shadowsocks_method:val('#out-ss-method'),
      reality_public_key:val('#out-reality-key').trim(), reality_short_id:val('#out-reality-short').trim(),
      wireguard_interface:val('#out-wg-interface').trim(), wireguard_address:val('#out-wg-address').trim(),
      wireguard_peer_public_key:val('#out-wg-peer').trim(), wireguard_allowed_ips:csv(val('#out-wg-allowed')),
      wireguard_keepalive:num('#out-wg-keepalive'), wireguard_mtu:num('#out-wg-mtu'),
      tor_socks_port:num('#out-tor-port'), openvpn_interface:val('#out-ovpn-interface').trim(),
      openvpn_routing_table:num('#out-ovpn-table'), openvpn_mark:num('#out-ovpn-mark'),
      enabled:Boolean($('#out-enabled')?.checked), remark:val('#out-remark').trim()
    };
    await api(`outbounds/${id}`, {method:'PUT', body});
    closeModal(); toast('Outbound بروزرسانی و deploy شد'); navigate('outbounds');
  }

  async function openGroupEdit(id) {
    const [g,nodes,outbounds] = await Promise.all([api(`outbound-groups/${id}`),api('nodes'),api('outbounds')]);
    state.cache.nodes=nodes||[]; state.cache.outbounds=outbounds||[];
    modal('ویرایش Failover Group', `<div class="form-grid">
      <label class="field"><span>Name</span><input id="group-name" value="${esc(g.name||'')}"></label>
      <label class="field"><span>Node</span><select id="group-node" data-role="group-node">${nodeOptions(g.node_id)}</select></label>
      <label class="field"><span>Strategy</span><select id="group-strategy">${['least_ping','least_load','round_robin','random'].map(s=>`<option value="${s}" ${s===g.strategy?'selected':''}>${s}</option>`).join('')}</select></label>
      <label class="field"><span>Expected</span><input id="group-expected" type="number" value="${Number(g.expected||1)}"></label>
      <label class="field span-2"><span>Fallback</span><select id="group-fallback"></select></label>
      <div class="span-2"><div class="form-section-title">Members</div><div id="group-members" class="member-list"></div></div>
      <label class="check-row"><input id="group-enabled" type="checkbox" ${g.enabled?'checked':''}> Enabled</label>
      <div class="span-2"><button class="btn primary wide" data-action="save-group" data-id="${esc(id)}">Save + Deploy</button></div>
    </div>`, 'FAILOVER EDIT');
    refreshGroupCandidates();
    for (const m of (g.members||[])) {
      setChecked(`[data-group-member="${CSS.escape(m.outbound_id)}"]`,true);
      setInput(`[data-member-priority="${CSS.escape(m.outbound_id)}"]`,m.priority);
      setInput(`[data-member-weight="${CSS.escape(m.outbound_id)}"]`,m.weight);
    }
    setInput('#group-fallback',g.fallback_outbound_id||'blocked');
  }

  async function saveGroup(id) {
    const members=$$('[data-group-member]:checked').map(box=>{
      const oid=box.dataset.groupMember;
      return {outbound_id:oid,priority:Number($(`[data-member-priority="${CSS.escape(oid)}"]`)?.value||0),weight:Number($(`[data-member-weight="${CSS.escape(oid)}"]`)?.value||1)};
    });
    await api(`outbound-groups/${id}`,{method:'PUT',body:{
      name:val('#group-name').trim(),node_id:val('#group-node'),strategy:val('#group-strategy'),members,
      fallback_outbound_id:val('#group-fallback')||'blocked',expected:num('#group-expected',1),enabled:Boolean($('#group-enabled')?.checked)
    }});
    closeModal(); toast('Failover group بروزرسانی شد'); navigate('groups');
  }

  async function openRoutingEdit(id) {
    const [r,nodes,inbounds,outbounds,groups,users]=await Promise.all([
      api(`routing/${id}`),api('nodes'),api('inbounds'),api('outbounds'),api('outbound-groups'),api('users')
    ]);
    state.cache.nodes=nodes||[]; state.cache.inbounds=inbounds||[]; state.cache.outbounds=outbounds||[]; state.cache.groups=groups||[]; state.cache.users=normalizeUsers(users);
    const targetType=r.outbound_group_id?'group':'outbound';
    modal('ویرایش Routing Rule', `<div class="form-grid">
      <label class="field"><span>Name</span><input id="route-name" value="${esc(r.name||'')}"></label>
      <label class="field"><span>Priority</span><input id="route-priority" type="number" value="${Number(r.priority||0)}"></label>
      <label class="field"><span>Node</span><select id="route-node" data-role="route-node">${nodeOptions(r.node_id)}</select></label>
      <label class="field"><span>Target type</span><select id="route-target-type" data-role="route-target-type"><option value="outbound" ${targetType==='outbound'?'selected':''}>Outbound</option><option value="group" ${targetType==='group'?'selected':''}>Failover Group</option></select></label>
      <label class="field span-2"><span>Target</span><select id="route-target"></select></label>
      <label class="field"><span>Inbound</span><select id="route-inbound"></select></label>
      <label class="field"><span>User</span><select id="route-user"><option value="">Any user</option>${state.cache.users.map(u=>`<option value="${esc(u.id)}" ${(r.user_ids||[])[0]===u.id?'selected':''}>${esc(u.username)}</option>`).join('')}</select></label>
      <label class="field span-2"><span>Domains</span><input id="route-domains" value="${esc((r.domains||[]).join(','))}"></label>
      <label class="field span-2"><span>IPs</span><input id="route-ips" value="${esc((r.ips||[]).join(','))}"></label>
      <label class="field"><span>Ports</span><input id="route-ports" value="${esc(r.ports||'')}"></label>
      <label class="field"><span>Network</span><select id="route-network"><option value="">Any</option>${['tcp','udp','tcp,udp'].map(n=>`<option value="${n}" ${r.network===n?'selected':''}>${n}</option>`).join('')}</select></label>
      <label class="field span-2"><span>Protocols</span><input id="route-protocols" value="${esc((r.protocols||[]).join(','))}"></label>
      <label class="check-row"><input id="route-enabled" type="checkbox" ${r.enabled?'checked':''}> Enabled</label>
      <div class="span-2"><button class="btn primary wide" data-action="save-routing" data-id="${esc(id)}">Save + Deploy</button></div>
    </div>`, 'ROUTING EDIT');
    refreshRoutingCandidates();
    setInput('#route-target', r.outbound_group_id||r.outbound_id);
    setInput('#route-inbound',(r.inbound_ids||[])[0]||'');
  }

  async function saveRouting(id) {
    const type=val('#route-target-type'),target=val('#route-target'),inbound=val('#route-inbound'),user=val('#route-user');
    await api(`routing/${id}`,{method:'PUT',body:{
      name:val('#route-name').trim(),node_id:val('#route-node'),priority:num('#route-priority'),enabled:Boolean($('#route-enabled')?.checked),
      inbound_ids:inbound?[inbound]:[],user_ids:user?[user]:[],domains:csv(val('#route-domains')),ips:csv(val('#route-ips')),
      ports:val('#route-ports').trim(),network:val('#route-network'),protocols:csv(val('#route-protocols')),
      outbound_id:type==='outbound'?target:'',outbound_group_id:type==='group'?target:''
    }});
    closeModal(); toast('Routing rule بروزرسانی و deploy شد'); navigate('routing');
  }

  async function showRoutingDetail(id) {
    const r=await api(`routing/${id}`);
    modal(r.name||'Routing Rule', `<div class="detail-banner"><div>${iconTile('⌁','healthy')}<span><strong>${esc(r.name)}</strong><small>${esc(nodeName(r.node_id))}</small></span></div>${status(r.enabled?'active':'disabled')}</div>
      <div class="card"><div class="resource-kvs">${kv('Priority',r.priority)}${kv('Target',r.outbound_group_id?groupName(r.outbound_group_id):outboundName(r.outbound_id))}${kv('Network',r.network||'Any')}${kv('Ports',r.ports||'Any')}</div>
      <div class="match-chips">${[...(r.domains||[]),...(r.ips||[]),...(r.protocols||[])].map(x=>`<span>${esc(x)}</span>`).join('')||'<span>Any traffic</span>'}</div></div>`, 'ROUTING');
  }

  async function probeAllOutbounds() {
    const ids = state.cache.outbounds.filter(x=>x.enabled).map(x=>x.id);
    let ok=0, fail=0;
    for (const id of ids) {
      try { await api(`outbounds/${id}/probe`,{method:'POST'}); ok++; } catch { fail++; }
    }
    toast(`Probe complete: ${ok} ok, ${fail} failed`, fail?'error':'success');
    navigate('outbounds');
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
    return confirmAction(message, 'تأیید حذف');
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
        case 'confirm-cancel': return settleConfirm(false);
        case 'confirm-accept': return settleConfirm(true);
        case 'toggle-live': return toggleLiveRefresh();
        case 'refresh-page': return navigate(state.page);
        case 'new-current': return openCreate(state.page);

        case 'create-user': return createUser();
        case 'create-plan': return createPlan();
        case 'create-node': return createNode();
        case 'create-inbound': return createInbound();
        case 'create-outbound': return createOutbound();
        case 'create-group': return createGroup();
        case 'create-routing': return createRouting();
        case 'save-user': return saveUser(id);
        case 'save-plan': return savePlan(id);
        case 'save-inbound': return saveInbound(id);
        case 'save-outbound': return saveOutbound(id);
        case 'save-group': return saveGroup(id);
        case 'save-routing': return saveRouting(id);

        case 'user-detail': return showUserDetail(id);
        case 'edit-user': return openUserEdit(id);
        case 'delete-user':
          if (await confirmDelete('User حذف شود؟ Peerها و bindingهای مرتبط نیز پاک می‌شوند.')) {
            await api(`users/${id}`, {method:'DELETE'}); toast('User حذف شد'); return navigate('users');
          }
          return;
        case 'plan-detail': return showPlanDetail(id);
        case 'edit-plan': return openPlanEdit(id);
        case 'delete-plan':
          if (await confirmDelete('Plan حذف شود؟ فقط plan بدون user قابل حذف است.')) {
            await api(`plans/${id}`, {method:'DELETE'}); toast('Plan حذف شد'); return navigate('plans');
          }
          return;
        case 'node-detail': return showNodeDetail(id);
        case 'toggle-maintenance':
          await api(`nodes/${id}/maintenance`, {method:'POST', body:{enabled:button.dataset.enabled!=='true'}});
          toast('Maintenance state تغییر کرد'); return navigate('nodes');
        case 'toggle-node':
          await api(`nodes/${id}/${button.dataset.enabled==='true'?'disable':'enable'}`, {method:'POST'});
          toast('Node state تغییر کرد'); return navigate('nodes');
        case 'probe-all-outbounds': return probeAllOutbounds();
        case 'routing-detail': return showRoutingDetail(id);

        case 'probe-node':
          await api(`nodes/${id}/probe`, {method: 'POST'}); toast('Node probe شد'); return navigate('nodes');

        case 'inbound-detail': return showInboundDetail(id);
        case 'edit-inbound': return openInboundEdit(id);
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
        case 'edit-outbound': return openOutboundEdit(id);
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
        case 'edit-group': return openGroupEdit(id);
        case 'delete-group':
          if (await confirmDelete('Failover group حذف شود؟')) {
            await api(`outbound-groups/${id}`, {method: 'DELETE'}); toast('Group حذف شد'); return navigate('groups');
          }
          return;

        case 'edit-routing': return openRoutingEdit(id);
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


  document.addEventListener('input', event => {
    if (event.target.matches('.page-filter') || event.target.matches('#global-search')) {
      applyPageFilter(event.target.value);
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
    if (event.key === '/' && !['INPUT','TEXTAREA','SELECT'].includes(document.activeElement?.tagName)) {
      event.preventDefault();
      $('#global-search')?.focus();
    }
  });

  window.addEventListener('hashchange', () => {
    const next = location.hash.replace(/^#\/?/, '');
    if (next && next !== state.page) navigate(next);
  });

  window.addEventListener('beforeunload', stopLiveRefresh);
  document.addEventListener('visibilitychange', updateLiveControl);

  bootstrap();
})();