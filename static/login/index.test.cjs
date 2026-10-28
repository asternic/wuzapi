const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { test } = require('node:test');
const vm = require('node:vm');

const html = readFileSync(`${__dirname}/index.html`, 'utf8');
const startup = html.slice(html.indexOf('  // Starting'));
const initialStatus = startup.slice(
  startup.indexOf('    statusRequest().then'),
  startup.indexOf('      } else if(status.success==false)')
) + '\n      }\n    });';
const checkStatus = html.slice(
  html.indexOf('  function checkStatus()'),
  html.indexOf('  async function showQr()')
);

async function runStatus(source, data) {
  const calls = [];
  const elements = new Map();
  const context = vm.createContext({
    console: { log() {} },
    statusRequest: async () => ({ success: true, data }),
    showQr: () => calls.push('qr'),
    showWidgets: () => calls.push('widgets'),
    connect: async () => { calls.push('connect'); return { success: true }; },
    clearInterval: () => calls.push('stopPolling'),
    scanInterval: 1,
    scanned: false,
    document: {
      getElementById(id) {
        if (!elements.has(id)) {
          elements.set(id, { innerHTML: '', classList: { add() {}, remove() {} } });
        }
        return elements.get(id);
      },
    },
    $: selector => ({ modal: action => calls.push(`${selector}:${action}`) }),
  });
  vm.runInContext(source, context);
  // Flush the status promise callback.
  await Promise.resolve();
  return { calls, elements, context };
}

test('connected unpaired session opens QR without displaying connected widgets', async () => {
  const { calls, context } = await runStatus(initialStatus, { connected: true, loggedIn: false });
  assert.deepEqual(calls, ['qr']);
  assert.equal(context.scanned, false);
});

test('authenticated session displays connected widgets', async () => {
  const { calls, elements, context } = await runStatus(initialStatus, { connected: true, loggedIn: true });
  assert.deepEqual(calls, ['widgets']);
  assert.equal(elements.get('connectstatus').innerHTML, 'Connected!');
  assert.equal(context.scanned, true);
});

test('polling recognizes completed pairing and closes login modals', async () => {
  const { calls, elements, context } = await runStatus(checkStatus + '\ncheckStatus();', {
    connected: true, loggedIn: true,
  });
  assert.equal(context.scanned, true);
  assert.equal(elements.get('connectstatus').innerHTML, 'Connected!');
  assert.deepEqual(calls, ['stopPolling', 'widgets', '#modalLoginQR:hide', '#modalLoginWithCode:hide']);
});
