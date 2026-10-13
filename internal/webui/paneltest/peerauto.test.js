import { test } from 'node:test';
import assert from 'node:assert/strict';

globalThis.document = { documentElement: { dataset: {} } };
const { peerSetupPayload } = await import('../panel/js/ui/peerauto.js');
const controls = values => ({ querySelector: selector => {
  const key = selector.match(/data-peer-field="([^"]+)"/)?.[1];
  const value = values[key];
  return value === undefined ? null : typeof value === 'boolean' ? { type: 'checkbox', checked: value } : { type: 'text', value: String(value) };
} });
test('native setup removes incompatible drawers and carries the explicit direction', () => {
  const group = controls({ acceptUdp: true, peerIp: '192.0.2.2', tunnelId: 'test-native-pair-identity', direction: 'reverse', maxPayload: 512, maxStreams: 128, paths: 4 });
  const result = peerSetupPayload({ querySelector: () => group }, { transport: 'waticmp', role: 'server', name: 'wat', token: 'secret', ports: '443', preset: 'turbo', tunnelPort: '8443', conn: { proxy: 'stale' }, managed: { privateKey: 'stale' } });
  assert.equal(result.waticmp.direction, 'reverse'); assert.equal(result.waticmp.peerIp, '192.0.2.2'); assert.equal(result.tune.acceptUDP, true);
  for (const key of ['preset', 'tunnelPort', 'conn', 'managed']) assert.equal(key in result, false, key);
});
test('Auto setup requires explicit approved endpoint data and conservative controls', () => {
  const row = controls({ id: 'noise', transport: 'stealth', endpoint: '192.0.2.1:24443', localBind: '0.0.0.0:24443' });
  const group = controls({ acceptUdp: false, tunnelId: 'pair-identity-example', iranId: 'iran-identity-example', kharejId: 'kharej-identity-example', observeOnly: true, probeInterval: 600, cooldown: 300, minimumDwell: 600, maxSwitchesPerHour: 2, maxConnections: 64, maxMemoryMb: 256 });
  group.querySelectorAll = () => [row];
  const result = peerSetupPayload({ querySelector: () => group }, { transport: 'auto', role: 'server', name: 'auto', token: 'secret', ports: '443', ipv6: true });
  assert.equal(result.auto.enabled, true); assert.equal(result.auto.observeOnly, true);
  assert.deepEqual(result.auto.candidates, [{ id: 'noise', transport: 'stealth', disabled: false, endpoint: '192.0.2.1:24443', localBind: '0.0.0.0:24443' }]);
  assert.equal(result.auto.cooldown, 300); assert.equal('ipv6' in result, false);
});
test('Kharej enrollment sends only the trusted link and local name', () => {
  const group = controls({ enrollmentLink: 'backpack://3.private' });
  const result = peerSetupPayload({ querySelector: () => group }, { name: 'peer', role: 'client', transport: 'auto', token: 'stale', ports: '443' });
  assert.deepEqual(result, { name: 'peer', role: 'client', transport: 'auto', peerEnrollment: 'backpack://3.private' });
});
