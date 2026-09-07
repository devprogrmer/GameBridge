// GameBridge Panel-first Phase 2: Xray Inbounds UI.
(() => {
  const oldNavigate = navigate;
  const oldOpenCreate = openCreate;

  const navButton = document.querySelector('#nav button[data-page="inbounds"]');
  if (navButton) navButton.onclick = () => navigate('inbounds');

  navigate = async function(p) {
    if (p !== 'inbounds') return oldNavigate(p);
    page = p;
    document.querySelectorAll('#nav button').forEach(b => b.classList.toggle('active', b.dataset.page === p));
    $('#page-title').textContent = 'Inbounds';
    $('#page-subtitle').textContent = 'VLESS / VMess / Trojan / Shadowsocks روی Xray';
    $('#content').innerHTML = '<div class="card">در حال بارگذاری...</div>';
    $('#page-action').style.display = 'inline-block';
    $('#page-action').onclick = () => openCreate('inbounds');
    try { await gbRenderInbounds(); } catch (e) {
      $('#content').innerHTML = `<div class="card empty">${esc(e.message)}</div>`;
      toast(e.message, true);
    }
  };

  openCreate = function(p) {
    if (p === 'inbounds') return gbOpenInboundCreate();
    return oldOpenCreate(p);
  };

  async function gbRenderInbounds() {
    const list = await api('inbounds');
    cache.nodes = await api('nodes');
    $('#content').innerHTML = `<div class="card table-wrap"><table><thead><tr>
      <th>Name</th><th>Protocol</th><th>Node</th><th>Listen</th><th>Transport</th><th>Security</th><th>Status</th><th></th>
      </tr></thead><tbody>${list.map(x => `<tr>
      <td><b>${esc(x.name)}</b><br><small class="muted">${esc(x.remark || '')}</small></td>
      <td>${esc(x.protocol)}</td><td>${nodeName(x.node_id)}</td><td>${esc(x.listen)}:${x.port}</td>
      <td>${esc(x.transport)}</td><td>${esc(x.tls_mode)}</td><td>${badge(x.status)}</td>
      <td class="actions">
        <button onclick="gbInboundUsers('${x.id}','${esc(x.name)}')">Users</button>
        <button onclick="gbDeployInbound('${x.id}')">Deploy</button>
        <button onclick="gbXrayStatus('${x.id}')">Xray</button>
        <button class="danger" onclick="gbDeleteInbound('${x.id}')">حذف</button>
      </td></tr>`).join('')}</tbody></table></div>`;
  }

  window.gbRenderInbounds = gbRenderInbounds;

  async function gbOpenInboundCreate() {
    cache.nodes = await api('nodes');
    modal('Inbound جدید', `<div class="form-grid">
      <div class="field"><label>Name</label><input id="xi-name" value="vless-main"></div>
      <div class="field"><label>Protocol</label><select id="xi-proto"><option>vless</option><option>vmess</option><option>trojan</option><option>shadowsocks</option></select></div>
      <div class="field"><label>Node</label><select id="xi-node">${cache.nodes.map(n=>`<option value="${n.id}">${esc(n.name)}</option>`).join('')}</select></div>
      <div class="field"><label>Listen</label><input id="xi-listen" value="0.0.0.0"></div>
      <div class="field"><label>Port</label><input id="xi-port" type="number" value="443"></div>
      <div class="field"><label>Transport</label><select id="xi-transport"><option>tcp</option><option>ws</option><option>grpc</option></select></div>
      <div class="field"><label>Security</label><select id="xi-tls"><option>none</option><option>tls</option><option>reality</option></select></div>
      <div class="field"><label>Server Name / Domain</label><input id="xi-sni"></div>
      <div class="field"><label>WS Path</label><input id="xi-path" value="/gamebridge"></div>
      <div class="field"><label>WS Host</label><input id="xi-host"></div>
      <div class="field"><label>gRPC Service</label><input id="xi-grpc" value="gamebridge"></div>
      <div class="field"><label>Shadowsocks Method</label><select id="xi-ss"><option>aes-128-gcm</option><option>aes-256-gcm</option><option>chacha20-poly1305</option></select></div>
      <div class="field"><label>TLS cert file (Node)</label><input id="xi-cert" placeholder="/etc/letsencrypt/live/.../fullchain.pem"></div>
      <div class="field"><label>TLS key file (Node)</label><input id="xi-key" placeholder="/etc/letsencrypt/live/.../privkey.pem"></div>
      <div class="field"><label>REALITY Dest</label><input id="xi-rdest" placeholder="www.cloudflare.com:443"></div>
      <div class="field"><label>REALITY Server Names</label><input id="xi-rnames" placeholder="www.cloudflare.com,cloudflare.com"></div>
      <div class="field"><label>REALITY Short IDs</label><input id="xi-rids" placeholder="خالی = خودکار"></div>
      <div class="field"><label>Fingerprint</label><input id="xi-fp" value="chrome"></div>
      <div class="field span2"><label>Remark</label><input id="xi-remark"></div>
      <div class="span2"><button class="primary" onclick="gbCreateInbound()">Create + Deploy</button></div>
    </div>`);
  }
  window.gbOpenInboundCreate = gbOpenInboundCreate;

  window.gbCreateInbound = async function() {
    try {
      const split = id => $(id).value.split(',').map(x => x.trim()).filter(Boolean);
      const d = await api('inbounds', {method:'POST', body:{
        name:$('#xi-name').value, protocol:$('#xi-proto').value, node_id:$('#xi-node').value,
        listen:$('#xi-listen').value, port:+$('#xi-port').value, transport:$('#xi-transport').value,
        tls_mode:$('#xi-tls').value, enabled:true, server_name:$('#xi-sni').value,
        path:$('#xi-path').value, host:$('#xi-host').value, service_name:$('#xi-grpc').value,
        cert_file:$('#xi-cert').value, key_file:$('#xi-key').value,
        reality_dest:$('#xi-rdest').value, reality_server_names:split('#xi-rnames'),
        reality_short_ids:split('#xi-rids'), reality_fingerprint:$('#xi-fp').value,
        shadowsocks_method:$('#xi-ss').value, remark:$('#xi-remark').value
      }});
      closeModal();
      if (d.deploy_error) toast('Inbound ذخیره شد؛ Deploy: '+d.deploy_error, true);
      else toast('Inbound ساخته و Deploy شد');
      gbRenderInbounds();
    } catch(e) { toast(e.message, true); }
  };

  window.gbDeployInbound = async function(id) {
    try { await api(`inbounds/${id}/deploy`, {method:'POST'}); toast('Xray Deploy شد'); gbRenderInbounds(); }
    catch(e) { toast(e.message, true); }
  };

  window.gbDeleteInbound = async function(id) {
    if (!confirm('Inbound و اتصال کاربرانش حذف شود؟')) return;
    try { await api(`inbounds/${id}`, {method:'DELETE'}); toast('Inbound حذف شد'); gbRenderInbounds(); }
    catch(e) { toast(e.message, true); }
  };

  window.gbXrayStatus = async function(id) {
    try {
      const d = await api(`inbounds/${id}/xray-status`);
      modal('Xray Runtime', `<p>Installed: <b>${d.installed?'Yes':'No'}</b></p>
        <p>Active: <b>${d.active?'Yes':'No'}</b></p><p>${esc(d.version||'')}</p>
        ${d.installed?'':`<button class="primary" onclick="gbInstallXray('${id}')">Install Xray on Node</button>`}`);
    } catch(e) { toast(e.message, true); }
  };

  window.gbInstallXray = async function(id) {
    try { const d=await api(`inbounds/${id}/xray-install`,{method:'POST'}); toast('Xray نصب شد'); gbXrayStatus(id); }
    catch(e) { toast(e.message,true); }
  };

  window.gbInboundUsers = async function(id, name) {
    try {
      const detail = await api(`inbounds/${id}`);
      cache.users = await api('users');
      const attached = new Set((detail.clients||[]).map(x=>x.user_id));
      modal('Users - '+name, `<div class="form-grid">
        <div class="field"><label>User</label><select id="xi-user">${cache.users.filter(u=>!attached.has(u.id)).map(u=>`<option value="${u.id}">${esc(u.username)}</option>`).join('')}</select></div>
        <div class="field"><label>&nbsp;</label><button class="primary" onclick="gbAttachInboundUser('${id}','${esc(name)}')">Attach User</button></div>
      </div>
      <div class="table-wrap"><table><thead><tr><th>User</th><th>Status</th><th></th></tr></thead><tbody>
      ${(detail.clients||[]).map(c=>`<tr><td>${esc(c.username)}</td><td>${badge(c.enabled?'active':'disabled')}</td><td class="actions">
        <button onclick="gbShareInbound('${id}','${c.user_id}')">Share</button>
        <button class="danger" onclick="gbDetachInboundUser('${id}','${c.user_id}','${esc(name)}')">Detach</button>
      </td></tr>`).join('')}</tbody></table></div>`);
    } catch(e) { toast(e.message,true); }
  };

  window.gbAttachInboundUser = async function(id, name) {
    const el=$('#xi-user'); if(!el || !el.value) return toast('کاربری برای اتصال وجود ندارد', true);
    try {
      const d=await api(`inbounds/${id}/users`,{method:'POST',body:{user_id:el.value}});
      if(d.deploy_error) toast(d.deploy_error,true); else toast('User متصل شد');
      gbInboundUsers(id,name);
    } catch(e){toast(e.message,true)}
  };

  window.gbDetachInboundUser = async function(id,userID,name) {
    try { await api(`inbounds/${id}/users/${userID}`,{method:'DELETE'}); toast('User جدا شد'); gbInboundUsers(id,name); }
    catch(e){toast(e.message,true)}
  };

  window.gbShareInbound = async function(id,userID) {
    try {
      const d=await api(`inbounds/${id}/users/${userID}/share`);
      modal('Share Link', `<pre>${esc(d.link)}</pre><button onclick='navigator.clipboard.writeText(${JSON.stringify(d.link)})'>Copy</button>`);
    } catch(e){toast(e.message,true)}
  };
})();
