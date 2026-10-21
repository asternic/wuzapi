import { writable } from 'svelte/store';
import { field } from './model.js';

export const avatarContextKey = Symbol('wuzapi-avatar-cache');

// One cache per authenticated client, never global or persisted. Signed picture
// URLs expire; negative results also need caching to respect profile privacy.
export function createAvatarCache({
  now = Date.now,
  ttl = 15 * 60000,
  negativeTTL = 5 * 60000,
  maximum = 256,
  concurrency = 2,
  interval = 200,
} = {}) {
  let client = null;
  let generation = 0;
  let active = 0;
  let timer;
  let lastStart = -Infinity;
  const entries = new Map();
  const pending = new Map();
  const queue = [];
  const store = writable({ enabled: false, entries: new Map() });
  const emit = () =>
    store.set({ enabled: !!client, entries: new Map(entries) });
  function save(jid, url) {
    entries.delete(jid);
    entries.set(jid, { url, expires: now() + (url ? ttl : negativeTTL) });
    while (entries.size > maximum) entries.delete(entries.keys().next().value);
    emit();
  }
  function schedule() {
    if (
      timer !== undefined ||
      !client ||
      active >= concurrency ||
      !queue.length
    )
      return;
    timer = setTimeout(
      () => {
        timer = undefined;
        pump();
      },
      Math.max(0, interval - (now() - lastStart)),
    );
  }
  function pump() {
    if (!client || active >= concurrency || !queue.length) return;
    const job = queue.shift();
    const current = client;
    active++;
    lastStart = now();
    Promise.resolve()
      .then(() =>
        current.request('/user/avatar', { Phone: job.jid, Preview: true }),
      )
      .then((result) => {
        if (job.generation !== generation) return;
        let url = '';
        try {
          const parsed = new URL(field(result, 'URL'));
          if (
            ['http:', 'https:'].includes(parsed.protocol) &&
            !parsed.username &&
            !parsed.password
          )
            url = parsed.href;
        } catch {
          /* Missing or unsafe URLs use initials. */
        }
        save(job.jid, url);
      })
      .catch(() => {
        if (job.generation === generation) save(job.jid, '');
      })
      .finally(() => {
        active--;
        if (pending.get(job.jid) === job) pending.delete(job.jid);
        schedule();
      });
    schedule();
  }
  return {
    subscribe: store.subscribe,
    setClient(next) {
      if (next === client) return;
      generation++;
      client = next;
      clearTimeout(timer);
      timer = undefined;
      entries.clear();
      pending.clear();
      queue.length = 0;
      emit();
    },
    request(jid, priority = false) {
      if (
        !client ||
        typeof jid !== 'string' ||
        !/^[^\s@]+@(s\.whatsapp\.net|c\.us|lid|g\.us)$/.test(jid)
      )
        return;
      const cached = entries.get(jid);
      if (cached && cached.expires > now()) return;
      const existing = pending.get(jid);
      if (existing) {
        // Promote a queued list thumbnail when that chat becomes active.
        const index = queue.indexOf(existing);
        if (priority && index > 0) {
          queue.splice(index, 1);
          queue.unshift(existing);
        }
        return;
      }
      if (queue.length >= maximum) return;
      const job = { jid, generation };
      pending.set(jid, job);
      if (priority) queue.unshift(job);
      else queue.push(job);
      schedule();
    },
    unavailable(jid, url) {
      if (entries.get(jid)?.url === url) save(jid, '');
    },
    close() {
      this.setClient(null);
    },
  };
}
