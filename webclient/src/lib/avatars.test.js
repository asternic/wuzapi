import { test } from 'node:test';
import assert from 'node:assert/strict';
import { get } from 'svelte/store';
import { createAvatarCache } from './avatars.js';

async function until(predicate) {
  const deadline = Date.now() + 2000;
  while (!predicate()) {
    if (Date.now() > deadline) throw new Error('Avatar queue did not settle');
    await new Promise((resolve) => setTimeout(resolve, 1));
  }
}
function deferredClient() {
  const calls = [];
  return {
    calls,
    request(path, body) {
      return new Promise((resolve, reject) =>
        calls.push({ path, body, resolve, reject }),
      );
    },
  };
}

test('visible avatars share one request, prioritize the active chat, and bound concurrency', async () => {
  const cache = createAvatarCache({ interval: 0 });
  const client = deferredClient();
  cache.setClient(client);
  try {
    cache.request('1@lid');
    cache.request('2@g.us');
    cache.request('1@lid');
    cache.request('3@s.whatsapp.net', true);
    await until(() => client.calls.length === 2);
    assert.equal(client.calls[0].body.Phone, '3@s.whatsapp.net');
    assert.equal(client.calls[1].body.Phone, '1@lid');
    for (const call of client.calls) {
      assert.equal(call.path, '/user/avatar');
      assert.equal(call.body.Preview, true);
    }
    client.calls[0].resolve({ URL: 'https://example.test/three.jpg' });
    await until(() => client.calls.length === 3);
    assert.equal(client.calls[2].body.Phone, '2@g.us');
    client.calls[1].resolve({ URL: 'https://example.test/one.jpg' });
    client.calls[2].resolve({ URL: 'https://example.test/two.jpg' });
    await until(() => get(cache).entries.size === 3);
    cache.request('1@lid');
    await new Promise((resolve) => setTimeout(resolve, 5));
    assert.equal(client.calls.length, 3);
  } finally {
    cache.close();
  }
});

test('photo privacy failures and broken URLs are cached instead of retried each poll', async () => {
  let now = 0;
  const calls = [];
  const cache = createAvatarCache({
    now: () => now,
    interval: 0,
    negativeTTL: 100,
    ttl: 1000,
  });
  cache.setClient({
    async request(path, body) {
      calls.push(body.Phone);
      if (body.Phone === '1@lid') throw new Error('profile picture is private');
      return { URL: 'https://example.test/two.jpg' };
    },
  });
  try {
    cache.request('1@lid');
    await until(() => get(cache).entries.has('1@lid'));
    for (let i = 0; i < 10; i++) cache.request('1@lid');
    assert.equal(calls.length, 1);
    cache.request('2@lid');
    await until(() => get(cache).entries.get('2@lid')?.url);
    cache.unavailable('2@lid', 'https://example.test/two.jpg');
    cache.request('2@lid');
    assert.equal(get(cache).entries.get('2@lid').url, '');
    assert.equal(calls.length, 2);
    now = 101;
    cache.request('1@lid');
    await until(() => calls.length === 3);
  } finally {
    cache.close();
  }
});

test('positive avatar cache expires without being coupled to message polling', async () => {
  let now = 0;
  let calls = 0;
  const cache = createAvatarCache({ now: () => now, interval: 0, ttl: 100 });
  cache.setClient({
    async request() {
      calls++;
      return { URL: 'https://example.test/avatar.jpg' };
    },
  });
  try {
    cache.request('1@g.us');
    await until(() => get(cache).entries.has('1@g.us'));
    now = 99;
    cache.request('1@g.us');
    assert.equal(calls, 1);
    // Let the prior job release its in-flight slot before requesting renewal.
    await new Promise((resolve) => setTimeout(resolve, 1));
    now = 101;
    cache.request('1@g.us');
    await until(() => calls === 2);
  } finally {
    cache.close();
  }
});

test('logout and account switching discard cached photos and late responses', async () => {
  const first = deferredClient();
  const second = deferredClient();
  const cache = createAvatarCache({ interval: 0 });
  cache.setClient(first);
  cache.request('1@lid');
  await until(() => first.calls.length === 1);
  cache.setClient(second);
  cache.request('1@lid');
  await until(() => second.calls.length === 1);
  first.calls[0].resolve({ URL: 'https://example.test/old-account.jpg' });
  second.calls[0].resolve({ URL: 'https://example.test/current-account.jpg' });
  await until(() => get(cache).entries.get('1@lid')?.url);
  assert.equal(
    get(cache).entries.get('1@lid').url,
    'https://example.test/current-account.jpg',
  );
  cache.close();
  assert.equal(get(cache).entries.size, 0);
  assert.equal(get(cache).enabled, false);
  cache.request('2@lid');
  assert.equal(second.calls.length, 1);
});

test('unsafe URLs, status broadcasts, and unbounded cache growth are prevented', async () => {
  let calls = 0;
  const cache = createAvatarCache({ interval: 0, maximum: 2 });
  cache.setClient({
    async request() {
      calls++;
      return { URL: 'javascript:alert(1)' };
    },
  });
  try {
    cache.request('status@broadcast');
    cache.request('1@newsletter');
    cache.request('alice');
    assert.equal(calls, 0);
    for (const jid of ['1@lid', '2@lid', '3@lid']) {
      cache.request(jid);
      await until(() => get(cache).entries.has(jid));
    }
    assert.equal(get(cache).entries.size, 2);
    assert.ok(
      [...get(cache).entries.values()].every((entry) => entry.url === ''),
    );
  } finally {
    cache.close();
  }
});
