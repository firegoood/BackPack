import { esc } from '../lib/dom.js';
import * as api from '../api.js';

const isPeerTransport = t => t === 'waticmp' || t === 'auto';
const forms = new WeakMap();
const field = (key, title, value = '', type = 'text', hint = '') =>
  `<label class="f3"><span>${esc(title)}</span><input data-peer-field="${esc(key)}" type="${type}" ${type === 'checkbox' ? (value ? 'checked' : '') : `value="${esc(String(value))}"`}><small class="hint">${esc(hint)}</small></label>`;
const direction = value => `<label class="f3"><span>Echo Request direction</span><select data-peer-field="direction"><option value="normal" ${value === 'normal' ? 'selected' : ''}>Normal: Iran → Kharej</option><option value="reverse" ${value === 'reverse' ? 'selected' : ''}>Reverse: Kharej → Iran</option></select></label>`;
const newID = () => Array.from(crypto.getRandomValues(new Uint8Array(16)), b => b.toString(16).padStart(2, '0')).join('');
const get = (root, key) => {
  const n = root?.querySelector(`[data-peer-field="${key}"]`);
  return n?.type === 'checkbox' ? n.checked : n?.value.trim() || '';
};
const num = (root, key) => Number(get(root, key));
const watFields = (w = {}) => field('peerIp', 'Other server IPv4', w.peerIp || '', 'text', 'Explicit address; ICMP has no tunnel port.')
  + field('tunnelId', 'Shared tunnel ID', w.tunnelId || newID()) + direction(w.direction || 'normal')
  + field('maxPayload', 'Maximum native payload', w.maxPayload || 512, 'number', '256–1400 bytes; authenticated PMTU discovery may lower this.')
  + field('maxStreams', 'Maximum concurrent streams', w.maxStreams || 128, 'number')
  + field('paths', 'ICMP lanes', w.paths || 4, 'number');

function candidateMarkup(p = {}, index = 0) {
  return `<fieldset class="peer-candidate"><legend>Approved carrier ${index + 1}</legend>
    ${field('id', 'Candidate ID', p.id || `c${newID().slice(0, 8)}`)}
    <label class="f3"><span>Carrier</span><select data-peer-field="transport">${[['waticmp', 'Wat ICMP'], ['stealth', 'TCP + Stealth (Noise)'], ['quic', 'QUIC'], ['wss', 'WSS'], ['wssmux', 'WSS Mux']].map(([v, label]) => `<option value="${v}" ${p.transport === v ? 'selected' : ''}>${label}</option>`).join('')}</select></label>
    <div data-peer-wat>${field('iranIp', 'Iran IPv4', p.iranIp || '')}${field('kharejIp', 'Kharej IPv4', p.kharejIp || '')}${direction(p.direction || 'normal')}${field('maxPayload', 'Maximum payload', p.maxPayload || 512, 'number')}</div>
    <div data-peer-endpoint>${field('endpoint', 'Approved Iran listener host:port', p.endpoint || '', 'text', 'An explicitly allowed data port. No extra management port is opened.')}${field('localBind', 'Iran local bind (optional)', p.localBind || '', 'text', 'For example 0.0.0.0 with the same enrolled port.')}</div>
    <button type="button" data-peer-remove>Remove carrier</button></fieldset>`;
}
function wireCandidates(group) {
  group.querySelectorAll('.peer-candidate').forEach(row => {
    const select = row.querySelector('[data-peer-field="transport"]');
    const shape = () => { row.querySelector('[data-peer-wat]').hidden = select.value !== 'waticmp'; row.querySelector('[data-peer-endpoint]').hidden = select.value === 'waticmp'; };
    select.addEventListener('change', shape); shape();
    row.querySelector('[data-peer-remove]').onclick = () => row.remove();
  });
}
function autoFields(a = {}) {
  return field('tunnelId', 'Pair tunnel ID', a.tunnelId || newID())
    + field('iranId', 'Iran identity', a.iranId || newID()) + field('kharejId', 'Kharej identity', a.kharejId || newID())
    + field('observeOnly', 'Observe only', a.observeOnly ?? true, 'checkbox', 'Bootstrap the first approved carrier; automatic cross-carrier selection stays off.')
    + field('allowDegradationSwitch', 'Switch on sustained severe degradation', a.allowDegradationSwitch || false, 'checkbox')
    + field('probeInterval', 'Full probe interval (seconds)', a.probeInterval || 600, 'number')
    + field('cooldown', 'Switch cooldown (seconds)', a.cooldown || 300, 'number')
    + field('minimumDwell', 'Minimum dwell (seconds)', a.minimumDwell || 600, 'number')
    + field('maxSwitchesPerHour', 'Maximum switches per hour', a.maxSwitchesPerHour || 2, 'number')
    + field('maxConnections', 'Maximum user connections', a.maxConnections || 64, 'number')
    + field('maxMemoryMb', 'Probe memory pressure threshold (MB)', a.maxMemoryMb || 256, 'number')
    + `<div data-peer-candidates>${(a.candidates?.length ? a.candidates : [{}]).map(candidateMarkup).join('')}</div><button type="button" data-peer-add>Add approved carrier (up to four)</button>`;
}
export function resetPeerSetup(root) {
  root.querySelectorAll('[data-peer-hidden]').forEach(n => { n.hidden = false; delete n.dataset.peerHidden; });
  root.querySelectorAll('.peer-settings').forEach(n => { n.hidden = true; });
}
export function shapePeerSetup(root, transport, role, direct = false) {
  const enabled = !direct && isPeerTransport(transport);
  resetPeerSetup(root);
  const here = root.querySelector('.here-card');
  if (here && enabled) {
    here.querySelector('.here-flag').textContent = role === 'client' ? '🌍' : '🇮🇷';
    here.querySelector('b').textContent = role === 'client' ? 'Kharej — this server' : 'Iran — this server';
    here.querySelector('small').textContent = role === 'client'
      ? 'Import the trusted Iran enrollment link here. Both servers need this feature version.'
      : 'Build Iran here, then transfer its private setup link to Kharej.';
  }
  if (!enabled) return;
  root.querySelectorAll('input[name], select[name]').forEach(n => {
    if (['name', ...(role === 'server' ? ['token', 'ports'] : [])].includes(n.name)) return;
    const row = n.closest('.f3, .tg3, .dr2b, .grp3');
    if (row && !row.hidden) { row.hidden = true; row.dataset.peerHidden = '1'; }
  });
  root.querySelectorAll('.dr2b, .rp').forEach(n => { if (!n.hidden) { n.hidden = true; n.dataset.peerHidden = '1'; } });
  // These groups contain only legacy presets/drawers. Hide the whole group so
  // the wizard can skip an empty step, while reset restores other transports.
  root.querySelectorAll('.rpgrid, .dr2b').forEach(n => {
    const group = n.closest('.grp3');
    if (group && !group.hidden) { group.hidden = true; group.dataset.peerHidden = '1'; }
  });
  let cache = forms.get(root); if (!cache) { cache = {}; forms.set(root, cache); }
  const cacheKey = `${transport}-${role}`;
  let group = cache[cacheKey];
  if (!group) {
    group = document.createElement('div'); group.className = 'grp3 peer-settings';
    group.dataset.peerTransport = transport;
    group.dataset.peerRole = role;
    group.innerHTML = `<h3>${transport === 'auto' ? 'Peer Auto (experimental)' : 'Wat ICMP (WatTunnel Engine)'}</h3><p>Linux is required. Wat needs CAP_NET_RAW. TCP/UDP forwarding stays on Iran. Keep the setup link private; it contains the enrollment secret.</p>`
      + `<button type="button" data-peer-side>${role === 'client' ? 'Build Iran instead' : 'Import an enrolled Kharej endpoint'}</button>`
      + (role === 'client' ? field('enrollmentLink', 'Trusted Iran enrollment link', '', 'password', 'Version 3 only. Match this carrier and obtain the secret-bearing link over a trusted channel; both peers need the new version.')
        : field('acceptUdp', 'Forward UDP as well as TCP', false, 'checkbox')
        + (transport === 'auto' ? autoFields() : field('iranIp', 'This Iran server IPv4', '', 'text', 'Included in the peer setup link.') + watFields()));
    const anchor = root.querySelector('[name="ports"]')?.closest('.grp3') || root.querySelector('[name="name"]')?.closest('.grp3');
    anchor?.after(group); cache[cacheKey] = group;
    group.querySelector('[data-peer-side]').onclick = () => {
      root.querySelector(`[data-fn="setSide"][data-args*="${role === 'client' ? 'server' : 'client'}"]`)?.click();
    };
    if (transport === 'auto' && role === 'server') {
      wireCandidates(group);
      group.querySelector('[data-peer-add]').onclick = () => {
        const box = group.querySelector('[data-peer-candidates]');
        if (box.children.length >= 4) return;
        box.insertAdjacentHTML('beforeend', candidateMarkup({}, box.children.length)); wireCandidates(group);
      };
    }
  }
  group.hidden = false;
}

function nativePayload(group) {
  return { enabled: true, peerIp: get(group, 'peerIp'), tunnelId: get(group, 'tunnelId'), direction: get(group, 'direction'),
    maxPayload: num(group, 'maxPayload'), maxStreams: num(group, 'maxStreams'), paths: num(group, 'paths') };
}
// Retain only supported setup fields. Hidden legacy drawers never enter the API.
export function peerSetupPayload(root, payload) {
  if (!isPeerTransport(payload.transport)) return payload;
  const group = root.querySelector(`.peer-settings[data-peer-transport="${payload.transport}"][data-peer-role="${payload.role}"]`);
  if (payload.role === 'client') return { name: payload.name, role: payload.role, transport: payload.transport, peerEnrollment: get(group, 'enrollmentLink') };
  const result = { name: payload.name, role: payload.role, transport: payload.transport, token: payload.token, ports: payload.ports, tune: { acceptUDP: get(group, 'acceptUdp') } };
  if (payload.transport === 'waticmp') result.waticmp = nativePayload(group);
  else {
    const candidates = [...group.querySelectorAll('.peer-candidate')].map(row => {
      const c = { id: get(row, 'id'), transport: get(row, 'transport'), disabled: false };
      if (c.transport === 'waticmp') Object.assign(c, { iranIp: get(row, 'iranIp'), kharejIp: get(row, 'kharejIp'), direction: get(row, 'direction'), maxPayload: num(row, 'maxPayload') });
      else Object.assign(c, { endpoint: get(row, 'endpoint'), localBind: get(row, 'localBind') });
      return c;
    });
    result.auto = { enabled: true, tunnelId: get(group, 'tunnelId'), iranId: get(group, 'iranId'), kharejId: get(group, 'kharejId'),
      observeOnly: get(group, 'observeOnly'), allowDegradationSwitch: get(group, 'allowDegradationSwitch'), acceptUdp: get(group, 'acceptUdp'), candidates,
      probeInterval: num(group, 'probeInterval'), cooldown: num(group, 'cooldown'), minimumDwell: num(group, 'minimumDwell'), maxSwitchesPerHour: num(group, 'maxSwitchesPerHour'),
      maxConnections: num(group, 'maxConnections'), maxMemoryMb: num(group, 'maxMemoryMb') };
  }
  return result;
}
export const peerSetupHost = root => get(root.querySelector('.peer-settings[data-peer-transport="waticmp"]'), 'iranIp');

export function peerEditForm(root, settings, name) {
  const panes = root.querySelector('.panes');
  root.querySelectorAll('.tabs > button:not(.hist)').forEach(n => { n.hidden = true; });
  const auto = settings.auto?.tunnelId;
  panes.innerHTML = `<div class="pane grp3 peer-edit"><h3>${auto ? 'Enrolled peer Auto' : 'Native Wat ICMP'}</h3>`
    + (auto ? `<p>Pair identities, backend maps and permitted endpoints are fixed by enrollment. Create a new pair to rotate its secret or change that manifest. Live actions below do not rewrite the config.</p>
        ${field('observeOnly', 'Observe only after restart', settings.auto.observeOnly, 'checkbox')}${field('maxMemoryMb', 'Probe memory pressure threshold (MB)', settings.auto.maxMemoryMb, 'number')}
        <div data-peer-status role="status"></div><button type="button" data-peer-refresh>Refresh status</button><div data-peer-actions></div>`
      : `<p>Direction and shared tunnel ID must match on both servers. An edit requires applying the new setup link on the peer.</p>${watFields(settings.waticmp)}`) + '</div>';
  const group = panes.querySelector('.peer-edit');
  if (auto && settings.auto.enabled) {
    const output = group.querySelector('[data-peer-status]');
    const actions = group.querySelector('[data-peer-actions]');
    const paint = s => {
      output.innerHTML = `<p><b>${esc(s.state)}</b> · Active: ${esc(s.active || 'none')} · Last good: ${esc(s.lastGood || 'none')} · Generation: ${esc(String(s.generation))}</p>
        <p>${esc(s.reason || '')}</p><p>Observe: ${s.observeOnly ? 'on' : 'off'} · Lock: ${esc(s.manualLock || 'none')}</p>
        <p>Last commit: ${esc(s.lastSwitch || 'none')} · Next eligible switch: ${esc(s.nextEligibleSwitch || 'policy permits')}</p>
        <p>Flows: ${Number(s.flows?.live || 0)} live, ${Number(s.flows?.drained || 0)} drained, ${Number(s.flows?.interrupted || 0)} interrupted, ${Number(s.flows?.rejected || 0)} refused; UDP queue drops: ${Number(s.flows?.udpQueueDrops || 0)}.</p>
        <table><thead><tr><th>Carrier / direction</th><th>Readiness</th><th>Probe Mbps / RTT ms</th></tr></thead><tbody>${(s.candidates || []).map(c => `<tr><td>${esc(c.id)} · ${esc(c.transport)} ${esc(c.direction || '')}</td><td>${esc(c.ready ? 'ready' : c.reason || 'unavailable')}</td><td>${c.lastProbe && !c.lastProbe.startsWith('0001-') ? `${Number(c.usefulMbps || 0).toFixed(3)} / ${Number(c.rttMs || 0).toFixed(1)}` : 'not measured here'}</td></tr>`).join('')}</tbody></table>
        <small>Probe application traffic is capped at 128 KB/s. Reported throughput is a bounded lower-bound measurement.</small>
        <ul>${(s.history || []).slice(-8).map(e => `<li>${esc(e.at)} ${esc(e.candidate || '')} ${esc(e.result)}: ${esc(e.reason)}</li>`).join('')}</ul>`;
      actions.innerHTML = '';
      if (settings.role !== 'server') { actions.textContent = 'Switching actions are available on the enrolled Iran authority.'; return; }
      for (const [op, label] of [['observe', 'Observe only'], ['resume', 'Resume Auto'], ['unlock', 'Release runtime lock']]) addButton(op, '', label);
      for (const c of s.candidates || []) {
        for (const [op, label] of [['test-now', 'Test'], ['switch', 'Select'], ['lock', 'Lock'], ['disable-carrier', 'Disable until restart']]) addButton(op, c.id, `${label} ${c.id}`);
      }
    };
    const addButton = (op, candidate, label) => {
      const b = document.createElement('button'); b.type = 'button'; b.textContent = label; actions.append(b);
      b.onclick = async () => {
        actions.querySelectorAll('button').forEach(x => { x.disabled = true; }); output.textContent = 'Waiting for the peer-coordinated action…';
        try { paint(await api.autoAction(name, op, candidate)); }
        catch (e) { output.textContent = e.message; actions.querySelectorAll('button').forEach(x => { x.disabled = false; }); }
      };
    };
    const refresh = async () => { try { paint(await api.autoStatus(name)); } catch (e) { output.textContent = e.message; } };
    group.querySelector('[data-peer-refresh]').onclick = refresh; refresh();
  }
  return () => auto ? { auto: { ...settings.auto, observeOnly: get(group, 'observeOnly'), maxMemoryMb: num(group, 'maxMemoryMb') } }
    : { waticmp: { ...settings.waticmp, ...nativePayload(group) } };
}
