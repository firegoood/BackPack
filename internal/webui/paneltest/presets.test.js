import { test } from 'node:test';
import assert from 'node:assert/strict';
import { presetsForTransport } from '../panel/js/ui/presets.js';

const profiles = [
  { value: 'balance' }, { value: 'turbo' }, { value: 'aggressive' },
  { value: 'throughput', kcpOnly: true },
];

test('HTTPS and other non-KCP editors never offer the KCP-only profile', () => {
  for (const transport of ['naive', 'xhttp', 'reality', 'tcp', 'wss', 'quic', 'pck', 'xdi']) {
    assert.deepEqual(presetsForTransport(profiles, transport).map(p => p.value),
      ['balance', 'turbo', 'aggressive'], transport);
  }
});

test('returning to KCP restores all profiles without changing the API list', () => {
  presetsForTransport(profiles, 'xhttp');
  assert.deepEqual(presetsForTransport(profiles, 'kcp'), profiles);
  assert.equal(profiles.length, 4);
  assert.deepEqual(presetsForTransport(undefined, 'naive'), []);
});
