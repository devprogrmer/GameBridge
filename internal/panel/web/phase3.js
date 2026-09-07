// GameBridge Panel-first Phase 3: Outbounds, Routing, Traffic Accounting.
(() => {
  const previousNavigate = navigate;
  const previousOpenCreate = openCreate;

  const splitCSV = value => String(value || '').split(',').map(x => x.trim()).filter(Boolean);

  navigate = async function(p) {
    if (!['outbounds', 'routing', 'online', 'traffic'].includes(p)) {
      return previousNavigate(p);
    }
    page = p;
    document.querySelectorAll('#nav button').forEach(b => b.classList.toggle('active', b.dataset.page === p));
    const title = {
      outbounds: ['Outbounds', 'Freedom / Blackhole / SOCKS / HTTP'],
      routing: ['Routing', 'قوانین مسیریابی Xray'],
      online: ['Online Users', 'کاربران فعال بر اساس ترافیک اخیر'],
      traffic: ['Traffic', 'مصرف Xray + WireGuard و دوره Reset']
    }[p];
    $('#page-title').textContent = title[0];
    $('#page-subtitle').textContent = title[1];
    $('#content').innerHTML = '<div class="card">در حال بارگذاری...</div>';
    $('#page-action').style.display = ['online', 'traffic'].includes(p) ? 'none' : 'inline-block';
    $('#page-action').onclick = () => openCreate(p);
    try {
      if (p === 'outbounds') await gbOutbounds();
      if (p === 'routing') await gbRouting();
      if (p === 'online') await gbOnline();
      if (p === 'traffic') await gbTraffic();
    } catch (e) {
      $('#content').innerHTML = `<div class="card empty">${esc(e.message)}</div>`;
      toast(e.message, true);
    }
  };

  openCreate = function(p) {
    if (p === 'outbounds') return gbOutboundModal();
    if (p === 'routing') return gbRoutingModal();
    if (p === 'plans') return gbPlanModal();
    return previousOpenCreate(p);
  };

  // Upgrade the existing plan UI with periodic reset.
  plans = async function() {
    cache.plans = await api('plans');
    $('#content').innerHTML = `<div class="card table-wrap"><table><thead><tr>
      <th>نام</th><th>حجم</th><th>مدت</th><th>Device</th><th>Reset</th><th></th>
      </tr></thead><tbody>${cache.plans.map(p => `<tr>
        <td>${esc(p.name)}</td>
        <td>${p.data_limit_bytes ? fmt(p.data_limit_bytes) : 'Unlimited'}</td>
        <td>${p.duration_days || '-'} روز</td>
        <td>${p.device_limit}</td>
        <td>${p.reset_interval_days ? `هر ${p.reset_interval_days} روز` : '-'}</td>
        <td><button class="danger" onclick="del('plans','${p.id}',plans)">حذف</button></td>
      </tr>`).join('')}</tbody></table></div>`;
  };

  function gbPlanModal() {
    modal('پلن جدید', `<div class="form-grid">
      <div class="field"><label>Name</label><input id="p-name"></div>
      <div class="field"><label>Data GB</label><input id="p-data" type="number" value="50"></div>
      <div class="field"><label>Duration days</label><input id="p-days" type="number" value="30"></div>
      <div class="field"><label>Device limit</label><input id="p-dev" type="number" value="1"></div>
      <div class="field"><label>Traffic reset days</label><input id="p-reset" type="number" value="30"></div>
      <div class="span2"><button class="primary" onclick="gbCreatePlan()">ساخت</button></div>
    </div>`);
  }

  window.gbCreatePlan = async function() {
    try {
      await api('plans', {method:'POST', body:{
        name:$('#p-name').value,
        data_limit_bytes:+$('#p-data').value * 1024 ** 3,
        duration_days:+$('#p-days').value,
        device_limit:+$('#p-dev').value,
        reset_interval_days:+$('#p-reset').value
      }});
      closeModal();
      plans();
    } catch (e) { toast(e.message, true); }
  };

  async function gbOutbounds() {
    const [list, nodes] = await Promise.all([api('outbounds'), api('nodes')]);
    cache.nodes = nodes;
    cache.outbounds = list;
    $('#content').innerHTML = `<div class="card table-wrap"><table><thead><tr>
      <th>Name</th><th>Node</th><th>Tag</th><th>Protocol</th><th>Target</th><th>Status</th><th></th>
      </tr></thead><tbody>${list.map(x => `<tr>
        <td><b>${esc(x.name)}</b><br><small class="muted">${esc(x.remark || '')}</small></td>
        <td>${nodeName(x.node_id)}</td><td>${esc(x.tag)}</td><td>${esc(x.protocol)}</td>
        <td>${x.address ? `${esc(x.address)}:${x.port}` : '-'}</td>
        <td>${badge(x.enabled ? 'active' : 'disabled')}</td>
        <td class="actions"><button class="danger" onclick="gbDeleteOutbound('${x.id}')">حذف</button></td>
      </tr>`).join('')}</tbody></table></div>`;
  }
  window.gbOutbounds = gbOutbounds;

  async function gbOutboundModal() {
    cache.nodes = await api('nodes');
    modal('Outbound جدید', `<div class="form-grid">
      <div class="field"><label>Name</label><input id="o-name" value="proxy-out"></div>
      <div class="field"><label>Node</label><select id="o-node">${cache.nodes.map(n=>`<option value="${n.id}">${esc(n.name)}</option>`).join('')}</select></div>
      <div class="field"><label>Tag</label><input id="o-tag" value="proxy-out"></div>
      <div class="field"><label>Protocol</label><select id="o-proto"><option>freedom</option><option>blackhole</option><option>socks</option><option>http</option></select></div>
      <div class="field"><label>Address</label><input id="o-address" placeholder="127.0.0.1"></div>
      <div class="field"><label>Port</label><input id="o-port" type="number" value="1080"></div>
      <div class="field"><label>Username</label><input id="o-user"></div>
      <div class="field"><label>Password</label><input id="o-pass" type="password"></div>
      <div class="field span2"><label>Remark</label><input id="o-remark"></div>
      <div class="span2"><button class="primary" onclick="gbCreateOutbound()">Create + Deploy</button></div>
    </div>`);
  }

  window.gbCreateOutbound = async function() {
    try {
      const d = await api('outbounds', {method:'POST', body:{
        name:$('#o-name').value, node_id:$('#o-node').value, tag:$('#o-tag').value,
        protocol:$('#o-proto').value, address:$('#o-address').value, port:+$('#o-port').value,
        username:$('#o-user').value, password:$('#o-pass').value, enabled:true, remark:$('#o-remark').value
      }});
      closeModal();
      if (d.deploy_error) toast('Outbound ذخیره شد؛ Deploy: '+d.deploy_error, true);
      else toast('Outbound ساخته شد');
      gbOutbounds();
    } catch (e) { toast(e.message, true); }
  };

  window.gbDeleteOutbound = async function(id) {
    if (!confirm('Outbound حذف شود؟')) return;
    try { await api(`outbounds/${id}`, {method:'DELETE'}); toast('Outbound حذف شد'); gbOutbounds(); }
    catch (e) { toast(e.message, true); }
  };

  async function gbRouting() {
    const [rules, nodes, outbounds] = await Promise.all([api('routing'), api('nodes'), api('outbounds')]);
    cache.nodes = nodes;
    cache.outbounds = outbounds;
    const targetName = id => id === 'direct' || id === 'blocked' ? id : (outbounds.find(x=>x.id===id)?.tag || id);
    $('#content').innerHTML = `<div class="card table-wrap"><table><thead><tr>
      <th>Priority</th><th>Name</th><th>Node</th><th>Match</th><th>Outbound</th><th>Status</th><th></th>
      </tr></thead><tbody>${rules.map(r => `<tr>
        <td>${r.priority}</td><td><b>${esc(r.name)}</b></td><td>${nodeName(r.node_id)}</td>
        <td><small>${r.domains?.length ? 'domain ' + r.domains.length : ''} ${r.ips?.length ? 'ip ' + r.ips.length : ''} ${r.user_ids?.length ? 'users ' + r.user_ids.length : ''}</small></td>
        <td>${esc(targetName(r.outbound_id))}</td><td>${badge(r.enabled ? 'active' : 'disabled')}</td>
        <td><button class="danger" onclick="gbDeleteRouting('${r.id}')">حذف</button></td>
      </tr>`).join('')}</tbody></table></div>`;
  }
  window.gbRouting = gbRouting;

  async function gbRoutingModal() {
    const [nodes, inbounds, outbounds, users] = await Promise.all([api('nodes'), api('inbounds'), api('outbounds'), api('users')]);
    cache.nodes = nodes; cache.inbounds = inbounds; cache.outbounds = outbounds; cache.users = users;
    modal('Routing Rule جدید', `<div class="form-grid">
      <div class="field"><label>Name</label><input id="r-name" value="route-1"></div>
      <div class="field"><label>Priority</label><input id="r-priority" type="number" value="100"></div>
      <div class="field"><label>Node</label><select id="r-node">${nodes.map(n=>`<option value="${n.id}">${esc(n.name)}</option>`).join('')}</select></div>
      <div class="field"><label>Outbound</label><select id="r-out"><option value="direct">direct</option><option value="blocked">blocked</option>${outbounds.map(o=>`<option value="${o.id}">${esc(o.tag)} (${esc(o.name)})</option>`).join('')}</select></div>
      <div class="field"><label>Inbound (optional)</label><select id="r-in"><option value="">All</option>${inbounds.map(i=>`<option value="${i.id}">${esc(i.name)}</option>`).join('')}</select></div>
      <div class="field"><label>User (optional)</label><select id="r-user"><option value="">All</option>${users.map(u=>`<option value="${u.id}">${esc(u.username)}</option>`).join('')}</select></div>
      <div class="field span2"><label>Domains comma separated</label><input id="r-domains" placeholder="domain:example.com,geosite:cn"></div>
      <div class="field span2"><label>IPs comma separated</label><input id="r-ips" placeholder="geoip:private,10.0.0.0/8"></div>
      <div class="field"><label>Ports</label><input id="r-ports" placeholder="80,443,1000-2000"></div>
      <div class="field"><label>Network</label><select id="r-network"><option value="">Any</option><option>tcp</option><option>udp</option><option>tcp,udp</option></select></div>
      <div class="field span2"><label>Protocols comma separated</label><input id="r-protocols" placeholder="bittorrent"></div>
      <div class="span2"><button class="primary" onclick="gbCreateRouting()">Create + Deploy</button></div>
    </div>`);
  }

  window.gbCreateRouting = async function() {
    try {
      const inbound = $('#r-in').value, user = $('#r-user').value;
      const d = await api('routing', {method:'POST', body:{
        name:$('#r-name').value, node_id:$('#r-node').value, priority:+$('#r-priority').value,
        enabled:true, inbound_ids:inbound?[inbound]:[], user_ids:user?[user]:[],
        domains:splitCSV($('#r-domains').value), ips:splitCSV($('#r-ips').value),
        ports:$('#r-ports').value, network:$('#r-network').value,
        protocols:splitCSV($('#r-protocols').value), outbound_id:$('#r-out').value
      }});
      closeModal();
      if (d.deploy_error) toast('Rule ذخیره شد؛ Deploy: '+d.deploy_error, true);
      else toast('Routing rule ساخته شد');
      gbRouting();
    } catch (e) { toast(e.message, true); }
  };

  window.gbDeleteRouting = async function(id) {
    if (!confirm('Routing rule حذف شود؟')) return;
    try { await api(`routing/${id}`, {method:'DELETE'}); toast('Rule حذف شد'); gbRouting(); }
    catch (e) { toast(e.message, true); }
  };

  async function gbOnline() {
    const list = await api('online-users');
    $('#content').innerHTML = `<div class="card"><div class="panel-title"><h3>Online Users</h3><button onclick="gbSyncAndRefresh('online')">Refresh stats</button></div>
      <div class="table-wrap"><table><thead><tr><th>User</th><th>Status</th><th>Traffic</th><th>Xray</th><th>WireGuard</th><th>Last activity</th></tr></thead><tbody>
      ${list.map(u=>`<tr><td><b>${esc(u.username)}</b></td><td>${badge(u.status)}</td><td>${fmt(u.traffic_bytes)}</td><td>${fmt(u.xray_bytes)}</td><td>${fmt(u.wireguard_bytes)}</td><td>${u.last_online_at?new Date(u.last_online_at).toLocaleString('fa-IR'):'-'}</td></tr>`).join('')}
      </tbody></table></div></div>`;
  }
  window.gbOnline = gbOnline;

  async function gbTraffic() {
    const list = await api('traffic');
    $('#content').innerHTML = `<div class="card"><div class="panel-title"><h3>Traffic Accounting</h3><button onclick="gbSyncAndRefresh('traffic')">Sync now</button></div>
      <div class="table-wrap"><table><thead><tr><th>User</th><th>Status</th><th>Total</th><th>Xray</th><th>WireGuard</th><th>Limit</th><th>Next reset</th></tr></thead><tbody>
      ${list.map(u=>`<tr><td><b>${esc(u.username)}</b></td><td>${badge(u.status)}</td><td>${fmt(u.traffic_bytes)}</td><td>${fmt(u.xray_bytes)}</td><td>${fmt(u.wireguard_bytes)}</td><td>${u.limit_bytes?fmt(u.limit_bytes):'Unlimited'}</td><td>${u.next_reset_at&&!String(u.next_reset_at).startsWith('0001')?new Date(u.next_reset_at).toLocaleString('fa-IR'):'-'}</td></tr>`).join('')}
      </tbody></table></div></div>`;
  }
  window.gbTraffic = gbTraffic;

  window.gbSyncAndRefresh = async function(target) {
    try {
      await api('sync-traffic', {method:'POST'});
      toast('Traffic sync شد');
      if (target === 'online') gbOnline();
      else gbTraffic();
    } catch (e) { toast(e.message, true); }
  };
})();
