import { test } from 'node:test';
import assert from 'node:assert/strict';
import { managedSettingsChanged, shapeManaged } from '../panel/js/ui/managed.js';

test('clearing HTTPS identity or host settings requires a fresh peer setup link', () => {
  for (const key of ['host', 'path', 'uuid', 'shortId', 'certificate', 'key', 'serverName']) {
    assert.equal(managedSettingsChanged({ [key]: '' }, { [key]: 'old-value' }), true, key);
  }
});

test('blank redacted secrets preserve the identity and need no handoff', () => {
  assert.equal(managedSettingsChanged({ password: '', privateKey: '', host: '' }, {}), false);
  assert.equal(managedSettingsChanged(undefined, undefined), false);
  assert.equal(managedSettingsChanged({ uuid: 'same-id' }, { uuid: 'same-id' }), false);
});

test('new HTTPS credentials require a fresh peer setup link', () => {
  for (const key of ['password', 'privateKey']) {
    assert.equal(managedSettingsChanged({ [key]: 'new-secret' }, {}), true, key);
  }
});

const helperForm = () => {
  const binary = { value: '' };
  const root = { querySelector: () => binary, querySelectorAll: () => [] };
  return { root, binary };
};

test('switching from Xray to Naive retires the incompatible executable', () => {
  const { root, binary } = helperForm();
  shapeManaged(root, 'xhttp', 'server');
  binary.value = '/custom/xray';
  shapeManaged(root, 'naive', 'server');
  assert.equal(binary.value, '', 'Naive would try to run Xray as sing-box');
  binary.value = '/custom/sing-box';
  shapeManaged(root, 'reality', 'server');
  assert.equal(binary.value, '', 'REALITY would try to run sing-box as Xray');
});

test('XHTTP and REALITY preserve their shared custom Xray executable', () => {
  const { root, binary } = helperForm();
  shapeManaged(root, 'xhttp', 'server');
  binary.value = '/custom/xray';
  shapeManaged(root, 'reality', 'server');
  assert.equal(binary.value, '/custom/xray');
});

test('changing Naive role selects the other helper while same-role redraws preserve it', () => {
  const { root, binary } = helperForm();
  shapeManaged(root, 'naive', 'server');
  binary.value = '/custom/sing-box';
  shapeManaged(root, 'naive', 'server');
  assert.equal(binary.value, '/custom/sing-box');
  shapeManaged(root, 'naive', 'client');
  assert.equal(binary.value, '', 'the Naive client requires naive rather than sing-box');
});
