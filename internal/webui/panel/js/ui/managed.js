/* Role-specific HTTPS fields are shared by setup and edit. Hidden fields are
 * marked off so switching transports cannot submit stale credentials. */
export const isManaged = transport => ['naive', 'xhttp', 'reality'].includes(transport);
const helperKinds = new WeakMap();

/* Empty secrets mean keep the current value. Clearing any other field can
 * change the peer identity (including server-generated UUIDs/paths), so it
 * still needs a fresh setup link. */
export function managedSettingsChanged(next, current = {}) {
  return Object.entries(next || {}).some(([key, value]) => {
    if ((key === 'password' || key === 'privateKey') && value === '') return false;
    return String(value) !== String(current?.[key] ?? '');
  });
}

export function shapeManaged(root, transport, role, direct = false) {
  const enabled = !direct && isManaged(transport);
  const server = role === 'server';
  if (enabled) {
    const tool = transport === 'naive' ? (server ? 'sing-box' : 'naive') : 'xray';
    const previous = helperKinds.get(root);
    if (previous && previous !== tool) {
      const binary = root.querySelector('[name="managed.binary"]');
      if (binary) binary.value = ''; // Use the newly selected helper's CLI default.
    }
    helperKinds.set(root, tool);
  }
  const matches = condition => {
    if (!condition) return true;
    if (condition === 'naive') return transport === 'naive';
    if (condition === 'xray') return transport !== 'naive';
    if (condition === 'sni') return transport !== 'naive' || server;
    if (condition === 'xhttp') return transport === 'xhttp';
    if (condition === 'reality') return transport === 'reality';
    if (condition === 'server-tls') return server && transport !== 'reality';
    if (condition === 'client-tls') return !server && transport !== 'reality';
    if (condition === 'server-reality') return server && transport === 'reality';
    if (condition === 'client-reality') return !server && transport === 'reality';
    return false;
  };
  root.querySelectorAll('.managed-https').forEach(group => {
    group.hidden = !enabled;
    if (enabled) group.removeAttribute('data-off'); else group.setAttribute('data-off', '1');
  });
  root.querySelectorAll('[data-helper]').forEach(row => {
    const on = enabled && matches(row.dataset.helper);
    row.hidden = !on;
    if (on) row.removeAttribute('data-off'); else row.setAttribute('data-off', '1');
  });
}
