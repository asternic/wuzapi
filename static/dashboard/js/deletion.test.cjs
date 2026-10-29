const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { test } = require('node:test');
const vm = require('node:vm');
const app = readFileSync(`${__dirname}/app.js`, 'utf8');
const source = 'let deletionInProgress = false;\n' + app.slice(app.indexOf('function deleteInstance(id)'), app.indexOf('function openDashboard'));

function setup(fetchImpl) {
  const calls = [], modals = new Map(), classes = new Map();
  const context = vm.createContext({
    baseUrl: 'http://localhost', Headers, encodeURIComponent,
    getLocalStorageItem: () => 'test-admin-token',
    fetch: async (url, options) => { calls.push(['fetch', url, options]); return fetchImpl(url, options); },
    updateAdmin: () => calls.push(['refresh']),
    showError: message => calls.push(['error', message]),
    $: selector => {
      const api = {
        modal(arg, key, value) {
          if (typeof arg === 'object') modals.set(selector, { ...modals.get(selector), ...arg });
          else if (arg === 'setting') modals.set(selector, { ...modals.get(selector), [key]: value });
          else {
            calls.push([selector, arg]);
            if (arg === 'hide') modals.get(selector)?.onHidden?.();
          }
          return api;
        },
        addClass(name) { if (!classes.has(selector)) classes.set(selector, new Set()); classes.get(selector).add(name); return api; },
        removeClass(name) { classes.get(selector)?.delete(name); return api; },
        toast(options) { calls.push(['toast', options.message]); return api; },
      };
      return api;
    },
  });
  vm.runInContext(source, context);
  return { context, calls, modals, classes };
}
const reply = (status, data) => ({ status, ok: status >= 200 && status < 300, json: async () => data });
const flush = () => new Promise(resolve => setImmediate(resolve));

test('ambiguity offers explicit recovery; cancellation sends no recovery request', async () => {
  const { context, calls, modals } = setup(() => reply(409, { success: false, error_code: 'ambiguous_device_ownership' }));
  context.deleteInstance('original');
  assert.equal(modals.get('#deleteInstanceModal').onApprove(), false);
  await flush();
  assert.ok(calls.some(c => c[0] === '#preserveDevicesModal' && c[1] === 'show'));
  assert.equal(modals.get('#preserveDevicesModal').onDeny(), true);
  assert.equal(calls.filter(c => c[0] === 'fetch').length, 1);
});

test('recovery captures the original instance ID and uses the admin endpoint', async () => {
  const { context, calls, modals } = setup(url => url.endsWith('/full')
    ? reply(409, { error_code: 'ambiguous_device_ownership' })
    : reply(200, { success: true, data: { devices_preserved: true } }));
  context.deleteInstance('original');
  modals.get('#deleteInstanceModal').onApprove();
  await flush();
  context.deleteInstance('another');
  modals.get('#preserveDevicesModal').onApprove();
  await flush();
  const requests = calls.filter(c => c[0] === 'fetch');
  assert.equal(requests[1][1], 'http://localhost/admin/users/original/preserve-devices');
  assert.equal(requests[1][2].method, 'DELETE');
  assert.equal(requests[1][2].headers.get('authorization'), 'test-admin-token');
  assert.ok(calls.some(c => c[0] === 'refresh'));
  assert.ok(calls.some(c => c[0] === 'toast' && c[1].includes('credentials preserved')));
});

test('other failures show the server error without offering recovery', async () => {
  for (const [status, body] of [[409, { error: 'session is still stopping' }], [500, { error: 'database error', error_code: 'ambiguous_device_ownership' }]]) {
    const { context, calls, modals, classes } = setup(() => reply(status, body));
    await context.performDelete('id');
    assert.equal(modals.has('#preserveDevicesModal'), false);
    assert.ok(calls.some(c => c[0] === 'error' && c[1] === body.error));
    assert.equal(classes.get('#deleteInstanceModal .actions .button').has('disabled'), false);
  }
});

test('in-flight requests block duplicate submits and cancel; failures allow retry', async () => {
  let reject;
  const { context, calls, modals, classes } = setup(() => new Promise((_, no) => { reject = no; }));
  context.showPreserveDevicesRecovery('id');
  modals.get('#preserveDevicesModal').onApprove();
  assert.equal(modals.get('#preserveDevicesModal').onDeny(), false);
  modals.get('#preserveDevicesModal').onApprove();
  await context.performDelete('other');
  assert.equal(calls.filter(c => c[0] === 'fetch').length, 1);
  reject(new Error('offline'));
  await flush();
  assert.equal(modals.get('#preserveDevicesModal').onDeny(), true);
  assert.equal(classes.get('#preserveDevicesModal .positive.button').has('loading'), false);
  assert.ok(calls.some(c => c[0] === 'error' && c[1].includes('try again')));
  const retry = context.performDelete('id', true);
  assert.equal(calls.filter(c => c[0] === 'fetch').length, 2);
  reject(new Error('offline'));
  await retry;
});

test('ordinary deletion keeps the full endpoint and standard success message', async () => {
  const { context, calls } = setup(() => reply(200, { success: true }));
  await context.performDelete('id/with spaces');
  assert.equal(calls.find(c => c[0] === 'fetch')[1], 'http://localhost/admin/users/id%2Fwith%20spaces/full');
  assert.ok(calls.some(c => c[0] === 'toast' && c[1] === 'Instance deleted successfully'));
});
