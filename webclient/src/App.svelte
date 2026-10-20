<script>
  import { onMount, onDestroy, tick, setContext } from 'svelte';
  import Icon from './Icon.svelte';
  import Avatar from './Avatar.svelte';
  import { createAvatarCache, avatarContextKey } from './lib/avatars.js';
  import Media from './Media.svelte';
  import Recorder from './Recorder.svelte';
  import { createAPI, readFile } from './lib/api.js';
  import {
    contactName,
    displayMessages,
    field,
    mergeMessages,
    preview,
    accountJID,
    isStatusJID,
  } from './lib/model.js';
  import {
    demoContacts,
    demoGroups,
    demoMessages,
    demoChats,
  } from './lib/demo.js';

  const avatars = createAvatarCache();
  setContext(avatarContextKey, avatars);

  let serverURL = '';
  let token = '';
  let remember = false;
  let api;
  let demo = false;
  let authenticated = false;
  let connecting = false;
  let status = {};
  let contacts = {};
  let groups = [];
  let chats = [];
  let selected = '';
  let rows = [];
  let draft = '';
  let search = '';
  let messageSearch = '';
  let showMessageSearch = false;
  let tab = 'all';
  let mode = 'chats';
  let modal = '';
  let error = '';
  let notice = '';
  let loading = false;
  let sending = false;
  let recording = false;
  let older = false;
  let before = 0;
  let moreChats = false;
  let nextOffset = 0;
  let fetchingOlder = false;
  let historyDisabled = false;
  let retention = 500;
  let syncDays = 90;
  let qr = '';
  let groupName = '';
  let participants = '';
  let newDestination = '';
  let actionBusy = false;
  let infoOpen = false;
  let timeline;
  let attachmentInput;
  let timer;
  let pollCount = 0;
  let epoch = 0;
  let chatEpoch = 0;
  let mediaPreviews = new Map();
  let locallyUnread = {};
  let seen = new Map();
  let drafts = {};
  let failedSend;
  let readOnlyHint = '';

  $: groupMap = Object.fromEntries(groups.map((g) => [field(g, 'JID'), g]));
  $: selectedGroup = groupMap[selected];
  $: nameFor = (jid) =>
    field(groupMap[jid], 'Name') || contactName(jid, contacts);
  $: visibleMessages = displayMessages(rows, status.jid).filter(
    (row) =>
      !messageSearch ||
      row.text.toLowerCase().includes(messageSearch.toLowerCase()),
  );
  $: allChats = chats
    .filter((chat) => !isStatusJID(chat.chat_jid))
    .sort((a, b) => new Date(b.timestamp) - new Date(a.timestamp));
  $: directory =
    mode === 'contacts'
      ? Object.keys(contacts)
          .filter((jid) => !isStatusJID(jid))
          .map((jid) => ({
            chat_jid: jid,
            text_content: jid,
            timestamp: '',
          }))
      : mode === 'groups'
        ? groups.map((g) => ({
            chat_jid: field(g, 'JID'),
            text_content: `${(field(g, 'Participants') || []).length} participantes`,
            timestamp:
              chats.find((c) => c.chat_jid === field(g, 'JID'))?.timestamp ||
              '',
          }))
        : allChats;
  $: filteredChats = directory.filter(
    (c) =>
      c.chat_jid &&
      (tab !== 'groups' || c.chat_jid.endsWith('@g.us')) &&
      (tab !== 'unread' || locallyUnread[c.chat_jid]) &&
      `${nameFor(c.chat_jid)} ${c.chat_jid} ${c.text_content}`
        .toLowerCase()
        .includes(search.toLowerCase()),
  );
  $: online = demo || (status.connected && status.loggedIn);
  $: avatars.setClient(authenticated && !demo && online ? api : null);
  $: unreadCount = Object.entries(locallyUnread).filter(
    ([jid, unread]) => unread && !isStatusJID(jid),
  ).length;

  function time(value) {
    const date = new Date(value);
    return Number.isNaN(+date)
      ? ''
      : date.toLocaleTimeString('es', { hour: '2-digit', minute: '2-digit' });
  }
  function day(value) {
    const date = new Date(value);
    return Number.isNaN(+date)
      ? ''
      : date.toLocaleDateString('es', {
          day: 'numeric',
          month: 'long',
          year: 'numeric',
        });
  }
  function shortTime(value) {
    const date = new Date(value);
    if (Number.isNaN(+date)) return '';
    return date.toDateString() === new Date().toDateString()
      ? time(value)
      : date.toLocaleDateString('es', { day: '2-digit', month: '2-digit' });
  }
  function tell(message) {
    notice = message;
  }
  function report(e) {
    if (e?.name !== 'AbortError')
      error = e?.message || 'Ocurrió un error inesperado.';
  }
  function revokePreviews() {
    for (const value of mediaPreviews.values()) URL.revokeObjectURL(value);
    mediaPreviews.clear();
  }
  function logout() {
    epoch++;
    chatEpoch++;
    clearTimeout(timer);
    api?.close();
    api = null;
    avatars.setClient(null);
    authenticated = false;
    demo = false;
    status = {};
    token = '';
    chats = [];
    contacts = {};
    groups = [];
    selected = '';
    rows = [];
    draft = '';
    drafts = {};
    failedSend = null;
    recording = false;
    sending = false;
    modal = '';
    error = '';
    notice = '';
    locallyUnread = {};
    pollCount = 0;
    nextOffset = 0;
    moreChats = false;
    older = false;
    before = 0;
    search = '';
    messageSearch = '';
    tab = 'all';
    mode = 'chats';
    infoOpen = false;
    showMessageSearch = false;
    loading = false;
    fetchingOlder = false;
    historyDisabled = false;
    actionBusy = false;
    qr = '';
    newDestination = '';
    groupName = '';
    participants = '';
    readOnlyHint = '';
    seen.clear();
    revokePreviews();
    sessionStorage.removeItem('wuzapi-client');
  }
  async function connect(event) {
    event?.preventDefault();
    error = '';
    connecting = true;
    const version = ++epoch;
    let candidate;
    try {
      candidate = createAPI(serverURL, token.trim());
      const data = await candidate.request('/session/status');
      if (version !== epoch) {
        candidate.close();
        return;
      }
      api?.close();
      api = candidate;
      status = data;
      authenticated = true;
      demo = false;
      historyDisabled = Number(data.history || 0) <= 0;
      retention = Number(data.history) || 500;
      syncDays = data.days_to_sync_history || 90;
      if (remember)
        sessionStorage.setItem(
          'wuzapi-client',
          JSON.stringify({ serverURL, token, remember }),
        );
      else sessionStorage.removeItem('wuzapi-client');
      if (data.loggedIn) await refreshDirectory();
      if (version !== epoch) return;
      await refreshChats();
      if (version !== epoch) return;
      if (!data.loggedIn) modal = 'pair';
      schedulePoll();
    } catch (e) {
      if (candidate !== api) candidate?.close();
      if (version === epoch) {
        report(e);
        // A temporary directory/history failure must not close an already
        // authenticated API session. Polling can recover it on the next tick.
        if (candidate === api && authenticated) schedulePoll();
      }
    } finally {
      if (version === epoch) connecting = false;
    }
  }
  function startDemo() {
    api?.close();
    epoch++;
    clearTimeout(timer);
    demo = true;
    authenticated = true;
    status = {
      name: 'Nicolás',
      jid: '5491155550000@s.whatsapp.net',
      connected: true,
      loggedIn: true,
      history: '500',
    };
    contacts = demoContacts;
    groups = demoGroups;
    chats = demoChats;
    historyDisabled = false;
    selectChat('5491155550101@s.whatsapp.net');
  }
  async function refreshDirectory() {
    const current = api;
    const version = epoch;
    const results = await Promise.allSettled([
      current.request('/user/contacts'),
      current.request('/group/list'),
    ]);
    if (current !== api || version !== epoch) return;
    if (results[0].status === 'fulfilled') contacts = results[0].value || {};
    if (results[1].status === 'fulfilled')
      groups = field(results[1].value, 'Groups') || [];
    const failures = results.filter((r) => r.status === 'rejected');
    if (failures.length)
      error = `No se pudo actualizar ${results[0].status === 'rejected' ? 'contactos' : ''}${failures.length === 2 ? ' y ' : ''}${results[1].status === 'rejected' ? 'grupos' : ''}: ${failures[0].reason.message}`;
  }
  async function refreshChats(append = false) {
    if (!api || demo) return;
    const current = api;
    const version = epoch;
    try {
      const result = await current.request(
        `/chat/conversations?limit=100&offset=${append ? nextOffset : 0}`,
      );
      if (current !== api || version !== epoch) return;
      historyDisabled = false;
      for (const chat of result.chats) {
        if (isStatusJID(chat.chat_jid)) continue;
        const previous = seen.get(chat.chat_jid);
        if (
          previous &&
          previous !== chat.message_id &&
          chat.chat_jid !== selected &&
          chat.sender_jid !== 'me' &&
          accountJID(chat.sender_jid) !== accountJID(status.jid)
        )
          locallyUnread = { ...locallyUnread, [chat.chat_jid]: true };
        seen.set(chat.chat_jid, chat.message_id);
      }
      const merged = new Map(chats.map((c) => [c.chat_jid, c]));
      for (const chat of result.chats) merged.set(chat.chat_jid, chat);
      chats = [...merged.values()];
      if (append || nextOffset <= 100) {
        nextOffset = result.next_offset;
        moreChats = result.has_more;
      }
    } catch (e) {
      if (e.status === 501) historyDisabled = true;
      else throw e;
    }
  }
  function schedulePoll() {
    clearTimeout(timer);
    const version = epoch;
    timer = setTimeout(async () => {
      if (version !== epoch || !api) return;
      try {
        if (!document.hidden) {
          if (++pollCount % 4 === 0 || !status.loggedIn) {
            const previousLogin = status.loggedIn;
            const updated = await api.request('/session/status');
            if (version !== epoch) return;
            status = updated;
            if (!previousLogin && status.loggedIn) {
              modal = '';
              qr = '';
              await refreshDirectory();
            }
          }
          if (version !== epoch) return;
          await refreshChats();
          if (selected && !historyDisabled) await loadMessages(false, false);
          if (modal === 'pair' && !status.loggedIn && status.connected)
            await fetchQR();
        }
      } catch (e) {
        if (version === epoch) {
          if (e.status === 401) {
            logout();
            error = 'El token dejó de ser válido. Volvé a ingresar.';
            return;
          }
          report(e);
        }
      }
      if (version === epoch) schedulePoll();
    }, 3500);
  }
  async function selectChat(jid) {
    if (isStatusJID(jid)) {
      tell('Los estados de WhatsApp no se muestran como conversaciones.');
      return;
    }
    if (selected) drafts[selected] = draft;
    selected = jid;
    chatEpoch++;
    rows = [];
    draft = drafts[jid] || '';
    failedSend = null;
    older = false;
    before = 0;
    infoOpen = false;
    messageSearch = '';
    recording = false;
    readOnlyHint = '';
    locallyUnread = { ...locallyUnread, [jid]: false };
    if (demo) {
      rows = demoMessages.filter((row) => row.chat_jid === jid);
      await scrollBottom();
      return;
    }
    await loadMessages(false, true);
  }
  async function scrollBottom() {
    await tick();
    if (timeline) timeline.scrollTop = timeline.scrollHeight;
  }
  async function loadMessages(previous = false, initial = false) {
    if (!api || !selected || historyDisabled) return;
    const current = api;
    const version = chatEpoch;
    const jid = selected;
    if (previous) fetchingOlder = true;
    else if (initial) loading = true;
    const oldHeight = timeline?.scrollHeight || 0;
    const stick =
      initial ||
      !timeline ||
      timeline.scrollHeight - timeline.scrollTop - timeline.clientHeight < 110;
    try {
      const page = await current.request(
        `/chat/messages?chat_jid=${encodeURIComponent(jid)}&limit=50${previous ? '&before=' + before : ''}`,
      );
      if (current !== api || version !== chatEpoch) return;
      rows = mergeMessages(rows, page.messages);
      if (initial || previous || !before) {
        before = page.next_before;
        older = page.has_more;
      }
      if (previous) {
        await tick();
        if (timeline) timeline.scrollTop += timeline.scrollHeight - oldHeight;
      } else if (stick) await scrollBottom();
    } catch (e) {
      if (version === chatEpoch) {
        if (e.status === 501) historyDisabled = true;
        else report(e);
      }
    } finally {
      if (version === chatEpoch) {
        loading = false;
        fetchingOlder = false;
      }
    }
  }
  function addSent(jid, id, text, kind, localURL = '') {
    const message = {
      id: Number.MAX_SAFE_INTEGER,
      message_id: id,
      chat_jid: jid,
      sender_jid: 'me',
      timestamp: new Date().toISOString(),
      message_type: kind,
      text_content: text,
      media_link: '',
      previewSource: localURL,
    };
    if (jid === selected) {
      rows = mergeMessages(rows, [message]);
      scrollBottom();
    }
    chats = [message, ...chats.filter((chat) => chat.chat_jid !== jid)];
  }
  async function sendText(event) {
    event?.preventDefault();
    if (demo) {
      readOnlyHint =
        'La demo es de solo lectura. Conectá tu instancia para enviar mensajes.';
      return;
    }
    const body = draft.trim();
    if (!body || sending || !online) return;
    const jid = selected;
    const current = api;
    const version = epoch;
    const id =
      failedSend?.body === body && failedSend?.jid === jid
        ? failedSend.id
        : crypto.randomUUID().replaceAll('-', '').toUpperCase();
    sending = true;
    error = '';
    failedSend = { id, body, jid };
    try {
      const result = await current.request('/chat/send/text', {
        Phone: jid,
        Body: body,
        Id: id,
      });
      if (version !== epoch) return;
      if (selected === jid && draft.trim() === body) draft = '';
      drafts[jid] = '';
      failedSend = null;
      addSent(jid, result.Id || id, body, 'text');
    } catch (e) {
      if (version === epoch) report(e);
    } finally {
      if (version === epoch) sending = false;
    }
  }
  async function sendAttachment(event) {
    const file = event.target.files?.[0];
    event.target.value = '';
    if (!file) return;
    if (demo) {
      readOnlyHint = 'Conectá tu instancia para adjuntar archivos.';
      return;
    }
    if (file.size > 16 * 1024 * 1024) {
      error = 'El cliente permite archivos de hasta 16 MiB.';
      return;
    }
    const current = api;
    const jid = selected;
    const version = epoch;
    sending = true;
    error = '';
    try {
      const encoded = await readFile(file);
      if (version !== epoch) return;
      const kind =
        file.type.startsWith('image/') && file.type !== 'image/svg+xml'
          ? 'image'
          : file.type.startsWith('video/')
            ? 'video'
            : file.type.startsWith('audio/')
              ? 'audio'
              : 'document';
      const payload = {
        Phone: jid,
        Id: crypto.randomUUID().replaceAll('-', '').toUpperCase(),
      };
      payload[
        {
          image: 'Image',
          video: 'Video',
          audio: 'Audio',
          document: 'Document',
        }[kind]
      ] = encoded;
      if (kind === 'document') payload.FileName = file.name;
      if (kind === 'audio') {
        payload.ptt = false;
        payload.mimetype = file.type;
      }
      const result = await current.request(`/chat/send/${kind}`, payload);
      if (version !== epoch) return;
      const localURL = URL.createObjectURL(file);
      mediaPreviews.set(result.Id || payload.Id, localURL);
      addSent(
        jid,
        result.Id || payload.Id,
        kind === 'document' ? file.name : '',
        kind,
        localURL,
      );
    } catch (e) {
      if (version === epoch) report(e);
    } finally {
      if (version === epoch) sending = false;
    }
  }
  async function sendVoice(blob, seconds) {
    if (demo)
      throw new Error('Conectá tu instancia para enviar una nota de voz.');
    const current = api;
    const jid = selected;
    const version = epoch;
    const form = new FormData();
    form.set('Phone', jid);
    form.set('Id', crypto.randomUUID().replaceAll('-', '').toUpperCase());
    form.set('Audio', blob, 'recording');
    form.set('Seconds', String(seconds));
    const result = await current.request('/chat/send/voice', form);
    if (version !== epoch) return;
    const localURL = URL.createObjectURL(blob);
    mediaPreviews.set(result.Id, localURL);
    addSent(jid, result.Id, '', 'audio', localURL);
    if (selected === jid) recording = false;
  }
  async function markPlayed(message) {
    if (demo || message.own) return;
    try {
      await api.request('/chat/markplayed', {
        Id: [message.message_id],
        ChatPhone: message.chat_jid,
        ...(message.chat_jid.endsWith('@g.us')
          ? { SenderPhone: message.sender_jid }
          : {}),
      });
    } catch (e) {
      report(e);
    }
  }
  async function markRead() {
    if (demo) return;
    const jid = selected;
    const current = api;
    const version = epoch;
    const incoming = visibleMessages.filter((row) => !row.own);
    try {
      for (const row of incoming)
        await current.request('/chat/markread', {
          Id: [row.message_id],
          ChatPhone: jid,
          SenderPhone: row.sender_jid,
        });
      if (version === epoch) tell('Confirmaciones de lectura enviadas.');
    } catch (e) {
      if (version === epoch) report(e);
    }
  }
  async function recoverHistory() {
    if (demo) {
      tell(
        'En una sesión real, esto solicita mensajes anteriores al teléfono.',
      );
      return;
    }
    const oldest = displayMessages(rows, status.jid).find(
      (row) => row.id !== Number.MAX_SAFE_INTEGER,
    );
    if (!oldest) {
      error =
        'Necesitás al menos un mensaje en este chat como punto de partida. La sincronización inicial ocurre al vincular el dispositivo.';
      return;
    }
    actionBusy = true;
    try {
      const normalized = displayMessages([oldest], status.jid)[0];
      const infoTimestamp = field(normalized?.info, 'Timestamp');
      const params = new URLSearchParams({
        chat_jid: selected,
        oldest_msg_id: oldest.message_id,
        oldest_msg_from_me: String(!!normalized?.own),
        oldest_msg_timestamp: String(
          new Date(infoTimestamp || oldest.timestamp).getTime(),
        ),
        count: '50',
      });
      await api.request('/session/history?' + params);
      tell(
        'Solicitud enviada al teléfono. La recuperación es asíncrona y depende del historial disponible en WhatsApp.',
      );
    } catch (e) {
      report(e);
    } finally {
      actionBusy = false;
    }
  }
  async function saveHistory(event) {
    event.preventDefault();
    if (demo) {
      modal = '';
      tell('Configuración ilustrativa: no se modificó ninguna instancia.');
      return;
    }
    actionBusy = true;
    try {
      await api.request('/session/history', {
        history: Number(retention),
        days_to_sync_history: Number(syncDays),
      });
      historyDisabled = Number(retention) === 0;
      modal = '';
      tell(
        'Historial configurado. Los días de sincronización se aplican en la próxima vinculación.',
      );
      await refreshChats();
    } catch (e) {
      report(e);
    } finally {
      actionBusy = false;
    }
  }
  async function fetchQR() {
    if (!api) return;
    const current = api;
    const result = await current.request('/session/qr');
    if (current !== api) return;
    qr = /^data:image\/(png|jpeg);base64,/.test(result.QRCode || '')
      ? result.QRCode
      : '';
    if (result.passkeyPending)
      tell(
        'WhatsApp solicita una passkey. Completá el proceso desde el dashboard de WuzAPI.',
      );
  }
  async function pair() {
    if (demo) return;
    actionBusy = true;
    try {
      await api.request('/session/connect', {
        Immediate: true,
      });
      qr = '';
      tell('Conexión iniciada. El QR aparecerá cuando WhatsApp esté listo.');
    } catch (e) {
      report(e);
    } finally {
      actionBusy = false;
    }
  }
  async function openDestination(event) {
    event.preventDefault();
    error = '';
    let jid = newDestination.trim();
    const version = epoch;
    actionBusy = true;
    try {
      if (!jid.includes('@')) {
        if (!/^\+?[0-9]+$/.test(jid))
          throw new Error('Ingresá un número internacional o un JID completo.');
        const digits = jid.replace(/^\+/, '');
        if (demo) jid = digits + '@s.whatsapp.net';
        else {
          const result = await api.request('/user/check', { Phone: [digits] });
          if (version !== epoch) return;
          const user = (field(result, 'Users') || []).find(
            (user) => field(user, 'IsInWhatsapp') && field(user, 'JID'),
          );
          if (!user)
            throw new Error(
              'WhatsApp no pudo resolver ese número. Si ya tenés su JID, ingresalo completo.',
            );
          jid = field(user, 'JID');
          const previous = contacts[digits + '@s.whatsapp.net'];
          if (previous) contacts = { ...contacts, [jid]: previous };
        }
      }
      if (!/^[^\s@]+@[^\s@]+$/.test(jid))
        throw new Error('El JID está incompleto.');
      modal = '';
      mode = 'chats';
      tab = 'all';
      newDestination = '';
      selectChat(jid);
    } catch (e) {
      if (version === epoch) report(e);
    } finally {
      if (version === epoch) actionBusy = false;
    }
  }
  async function createGroup(event) {
    event.preventDefault();
    if (demo) {
      tell('Conectá tu instancia para crear un grupo.');
      return;
    }
    actionBusy = true;
    try {
      const people = participants
        .split(/[,\n]/)
        .map((value) => value.trim())
        .filter(Boolean);
      const result = await api.request('/group/create', {
        name: groupName.trim(),
        participants: people,
      });
      await refreshDirectory();
      modal = '';
      groupName = '';
      participants = '';
      if (field(result, 'JID')) await selectChat(field(result, 'JID'));
      tell('Grupo creado.');
    } catch (e) {
      report(e);
    } finally {
      actionBusy = false;
    }
  }
  function showDialog(node) {
    node.showModal();
    return {
      destroy() {
        node.close();
      },
    };
  }
  function setMode(value) {
    mode = value;
    search = '';
    tab = 'all';
  }
  onMount(() => {
    serverURL = location.origin;
    try {
      const saved = JSON.parse(
        sessionStorage.getItem('wuzapi-client') || 'null',
      );
      if (saved) {
        serverURL = saved.serverURL;
        token = saved.token;
        remember = true;
      }
    } catch {
      sessionStorage.removeItem('wuzapi-client');
    }
  });
  onDestroy(() => {
    epoch++;
    clearTimeout(timer);
    api?.close();
    avatars.close();
    revokePreviews();
  });
</script>

<svelte:window
  onkeydown={(event) => {
    if (event.key === 'Escape') {
      modal = '';
      infoOpen = false;
    }
  }}
/>

{#if !authenticated}
  <main class="welcome">
    <section class="welcome-story">
      <a class="brand" href="../"
        ><span class="brand-mark">w<span>·</span></span><span
          >WuzAPI <small>WEB CLIENT</small></span
        ></a
      >
      <div class="welcome-copy">
        <span class="eyebrow">TUS CONVERSACIONES, EN CASA</span>
        <h1>Un espacio para<br />seguir en contacto.</h1>
        <p>
          Chats, grupos y notas de voz.<br />Tu WhatsApp, conectado a tu propia
          instancia de WuzAPI.
        </p>
        <div class="welcome-tags">
          <span><Icon name="chat" size={16} />Conversaciones</span><span
            ><Icon name="history" size={16} />Historial</span
          ><span><Icon name="mic" size={16} />Audio</span>
        </div>
      </div>
      <p class="welcome-foot">
        <Icon name="lock" size={15} />Tu token conecta el navegador directamente
        con tu servidor.
      </p>
    </section>
    <section class="login-panel">
      <div class="login-card">
        <div class="mini-label">HOLA DE NUEVO</div>
        <h2>Conectá tu instancia</h2>
        <p>Usá el token de un usuario de WuzAPI.</p>
        <form onsubmit={connect}>
          <label
            >URL del servidor<input
              type="url"
              bind:value={serverURL}
              required
              placeholder="https://wuzapi.example.com"
              autocomplete="url"
            /></label
          >
          <label
            >Token de usuario<input
              type="password"
              bind:value={token}
              required
              placeholder="Ingresá tu token"
              autocomplete="off"
            /></label
          >
          <label class="checkbox"
            ><input type="checkbox" bind:checked={remember} />Recordar en esta
            pestaña</label
          >
          {#if error}<p class="inline-error" role="alert">{error}</p>{/if}
          <button class="primary full" type="submit" disabled={connecting}
            >{connecting ? 'Conectando…' : 'Abrir conversaciones'}<Icon
              name="arrow"
              size={18}
            /></button
          >
        </form>
        <div class="divider"><span>o conocé el cliente</span></div>
        <button class="secondary full" onclick={startDemo}
          >Explorar demo <span class="subtle">Sin conexión</span></button
        >
        <small class="login-note"
          >No necesitás un token de administrador. Si tu WhatsApp todavía no
          está vinculado, podrás escanear el QR al ingresar.</small
        >
      </div>
      <span class="login-version">WuzAPI Web · Svelte · Autoalojado</span>
    </section>
  </main>
{:else}
  <main class="workspace" class:chat-open={!!selected}>
    <nav class="rail" aria-label="Navegación principal">
      <a class="rail-brand" href="../" title="WuzAPI">w<span>·</span></a>
      <div class="rail-links">
        <button
          class:active={mode === 'chats'}
          class="rail-button"
          title="Chats"
          aria-label="Chats"
          onclick={() => setMode('chats')}
          ><Icon name="chat" size={23} />{#if unreadCount}<span class="rail-dot"
            ></span>{/if}</button
        >
        <button
          class:active={mode === 'contacts'}
          class="rail-button"
          title="Contactos"
          aria-label="Contactos"
          onclick={() => setMode('contacts')}
          ><Icon name="person" size={23} /></button
        >
        <button
          class:active={mode === 'groups'}
          class="rail-button"
          title="Grupos"
          aria-label="Grupos"
          onclick={() => setMode('groups')}
          ><Icon name="users" size={23} /></button
        >
      </div>
      <div class="rail-bottom">
        <button
          class="rail-button"
          title="Configuración de historial"
          aria-label="Configuración de historial"
          onclick={() => (modal = 'settings')}
          ><Icon name="settings" size={23} /></button
        ><button
          class="profile"
          title="Cerrar el cliente"
          aria-label="Cerrar el cliente"
          onclick={logout}>{(status.name || 'W')[0].toUpperCase()}</button
        >
      </div>
    </nav>
    <aside class="inbox">
      <div class="inbox-header">
        <div>
          <span class="mini-label">TU ESPACIO</span>
          <h1>
            {mode === 'contacts'
              ? 'Contactos'
              : mode === 'groups'
                ? 'Grupos'
                : 'Conversaciones'}<span class="count">{directory.length}</span>
          </h1>
        </div>
        <button
          class="icon-button new-chat"
          title={mode === 'groups' ? 'Crear grupo' : 'Nueva conversación'}
          aria-label={mode === 'groups' ? 'Crear grupo' : 'Nueva conversación'}
          onclick={() => (modal = mode === 'groups' ? 'group' : 'new')}
          ><Icon name="plus" /></button
        >
      </div>
      <label class="search-box"
        ><Icon name="search" size={18} /><input
          bind:value={search}
          placeholder={mode === 'contacts'
            ? 'Buscar un contacto'
            : 'Buscar una conversación'}
          aria-label="Buscar conversaciones"
        /><kbd>⌕</kbd></label
      >
      {#if mode === 'chats'}<div class="filters">
          <button class:chosen={tab === 'all'} onclick={() => (tab = 'all')}
            >Todos</button
          ><button
            class:chosen={tab === 'unread'}
            onclick={() => (tab = 'unread')}
            >Nuevos {#if unreadCount}<span>{unreadCount}</span>{/if}</button
          ><button
            class:chosen={tab === 'groups'}
            onclick={() => (tab = 'groups')}>Grupos</button
          >
        </div>{:else}<p class="directory-note">
          {mode === 'contacts'
            ? 'La agenda sincronizada de tu WhatsApp.'
            : 'Todos los grupos a los que pertenecés.'}
        </p>{/if}
      <div class="conversation-list">
        {#each filteredChats as chat (chat.chat_jid)}
          <button
            class="conversation"
            class:selected={selected === chat.chat_jid}
            onclick={() => selectChat(chat.chat_jid)}
          >
            <Avatar
              jid={chat.chat_jid}
              name={nameFor(chat.chat_jid)}
              group={chat.chat_jid.endsWith('@g.us')}
            />
            <span class="conversation-copy"
              ><span class="conversation-top"
                ><strong>{nameFor(chat.chat_jid)}</strong><time
                  >{shortTime(chat.timestamp)}</time
                ></span
              ><span class="conversation-preview"
                >{#if chat.sender_jid === 'me'}<Icon
                    name="check"
                    size={14}
                  />{/if}<span>{preview(chat)}</span
                >{#if locallyUnread[chat.chat_jid]}<span class="unread-dot"
                  ></span>{/if}</span
              ></span
            >
          </button>
        {:else}<div class="list-empty">
            <Icon name="search" size={26} /><strong
              >{search
                ? 'Sin resultados'
                : 'Todavía no hay conversaciones'}</strong
            >
            <p>
              {historyDisabled
                ? 'Activá el historial para guardar tus mensajes.'
                : 'Abrí un contacto o iniciá una conversación.'}
            </p>
          </div>{/each}
        {#if moreChats && mode === 'chats'}<button
            class="text-button load-chats"
            onclick={() => refreshChats(true).catch(report)}
            >Cargar más conversaciones</button
          >{/if}
      </div>
      <footer class="inbox-footer">
        <span class="status-dot" class:offline={!online}></span><span
          >{demo
            ? 'Demo · Solo lectura'
            : online
              ? 'Conectado a tu instancia'
              : 'WhatsApp desconectado'}</span
        ><button
          class="icon-button"
          title="Actualizar contactos y chats"
          aria-label="Actualizar contactos y chats"
          onclick={async () => {
            if (demo) {
              tell('Estás viendo datos de demostración.');
              return;
            }
            try {
              await refreshDirectory();
              await refreshChats();
            } catch (e) {
              report(e);
            }
          }}><Icon name="refresh" size={15} /></button
        >
      </footer>
    </aside>
    <section class="chat-pane" aria-label="Conversación">
      {#if demo}<div class="demo-bar">
          <span>ESTÁS EN LA DEMO</span>Explorá la interfaz con conversaciones de
          ejemplo.<button onclick={logout}
            >Conectar mi instancia <Icon name="arrow" size={14} /></button
          >
        </div>{/if}
      {#if error}<div class="banner error" role="alert">
          <Icon name="info" size={18} /><span>{error}</span><button
            class="icon-button"
            aria-label="Cerrar error"
            onclick={() => (error = '')}><Icon name="close" size={16} /></button
          >
        </div>{/if}
      {#if notice}<div class="banner notice" role="status">
          <Icon name="check" size={18} /><span>{notice}</span><button
            class="icon-button"
            aria-label="Cerrar aviso"
            onclick={() => (notice = '')}
            ><Icon name="close" size={16} /></button
          >
        </div>{/if}
      {#if historyDisabled}<div class="banner">
          <Icon name="history" size={18} /><span
            >El historial está desactivado. Activá el almacenamiento para ver y
            recibir conversaciones en este cliente.</span
          ><button class="text-button" onclick={() => (modal = 'settings')}
            >Configurar</button
          >
        </div>{/if}
      {#if !online && !demo}<div class="banner">
          <Icon name="info" size={18} /><span
            >Tu sesión de WhatsApp no está conectada.</span
          ><button class="text-button" onclick={() => (modal = 'pair')}
            >Vincular</button
          >
        </div>{/if}
      {#if selected}
        <header class="chat-header">
          <button
            class="icon-button mobile-back"
            title="Volver"
            aria-label="Volver a conversaciones"
            onclick={() => {
              drafts[selected] = draft;
              selected = '';
              recording = false;
              chatEpoch++;
            }}><Icon name="back" /></button
          >
          <Avatar
            jid={selected}
            priority
            name={nameFor(selected)}
            group={selected.endsWith('@g.us')}
            small
          />
          <button class="chat-heading" onclick={() => (infoOpen = !infoOpen)}
            ><strong>{nameFor(selected)}</strong><span
              >{selected.endsWith('@g.us')
                ? `${(field(selectedGroup, 'Participants') || []).length} participantes · Grupo`
                : selected.endsWith('@lid')
                  ? 'Cuenta identificada por LID'
                  : 'Conversación de WhatsApp'}</span
            ></button
          >
          <div class="chat-actions">
            <button
              class="icon-button"
              title="Buscar en mensajes cargados"
              aria-label="Buscar en mensajes cargados"
              onclick={() => {
                showMessageSearch = !showMessageSearch;
                messageSearch = '';
              }}><Icon name="search" /></button
            ><button
              class="icon-button"
              title="Recuperar historial del teléfono"
              aria-label="Recuperar historial del teléfono"
              onclick={recoverHistory}
              disabled={actionBusy || historyDisabled}
              ><Icon name="history" /></button
            ><button
              class="icon-button"
              title="Información del chat"
              aria-label="Información del chat"
              onclick={() => (infoOpen = !infoOpen)}
              ><Icon name="info" /></button
            >
          </div>
        </header>
        {#if showMessageSearch}<label class="message-search"
            ><Icon name="search" size={18} /><input
              bind:value={messageSearch}
              placeholder="Buscar en los mensajes cargados"
              aria-label="Buscar en los mensajes cargados"
            /><button
              class="icon-button"
              aria-label="Cerrar búsqueda"
              onclick={() => {
                showMessageSearch = false;
                messageSearch = '';
              }}><Icon name="close" size={16} /></button
            ></label
          >{/if}
        <div class="chat-body">
          <div
            class="timeline"
            bind:this={timeline}
            role="log"
            aria-label="Mensajes"
          >
            <div class="timeline-inner">
              {#if loading}<div class="timeline-note">
                  Cargando mensajes…
                </div>{/if}
              {#if older}<button
                  class="older-button"
                  disabled={fetchingOlder}
                  onclick={() => loadMessages(true)}
                  ><Icon name="history" size={15} />{fetchingOlder
                    ? 'Cargando…'
                    : 'Cargar mensajes anteriores'}</button
                >{/if}
              <p class="privacy-note">
                <Icon name="lock" size={12} />Tus mensajes se consultan
                directamente en tu servidor WuzAPI.
              </p>
              {#each visibleMessages as message, i (message.message_id)}
                {#if i === 0 || day(visibleMessages[i - 1].timestamp) !== day(message.timestamp)}<div
                    class="date-divider"
                  >
                    <span>{day(message.timestamp)}</span>
                  </div>{/if}
                <div class="message-row" class:outgoing={message.own}>
                  {#if selected.endsWith('@g.us') && !message.own}<Avatar
                      jid={message.sender_jid}
                      name={contactName(message.sender_jid, contacts)}
                      small
                    />{/if}
                  <article class="bubble" class:deleted={message.deleted}>
                    {#if selected.endsWith('@g.us') && !message.own}<strong
                        class="sender-name"
                        >{contactName(message.sender_jid, contacts)}</strong
                      >{/if}
                    {#if message.quoted_message_id && !['edit', 'delete'].includes(message.message_type)}<div
                        class="quote"
                      >
                        En respuesta a {message.quoted_message_id}
                      </div>{/if}
                    {#if !message.deleted && ['audio', 'image', 'video', 'document', 'sticker'].includes(message.message_type)}
                      {#key message.message_id}<Media
                          {message}
                          {api}
                          onplayed={markPlayed}
                        />{/key}
                    {/if}
                    {#if message.text}<p>
                        {message.text}
                      </p>{:else if !['audio', 'image', 'video', 'document', 'sticker'].includes(message.message_type)}<p
                        class="unsupported"
                      >
                        Mensaje de tipo {message.message_type}
                      </p>{/if}
                    <div class="message-meta">
                      {#if message.edited}<span>editado</span>{/if}<time
                        >{time(message.timestamp)}</time
                      >{#if message.own}<span
                          class="sent-check"
                          title="Enviado; no implica confirmación de entrega"
                          ><Icon name="check" size={14} /></span
                        >{/if}
                    </div>
                    {#if message.reaction}<span class="reaction"
                        >{message.reaction}</span
                      >{/if}
                  </article>
                </div>
              {:else}{#if !loading}<div class="chat-empty">
                    <span class="empty-orbit"
                      ><Icon name="chat" size={30} /></span
                    >
                    <h3>La conversación empieza acá</h3>
                    <p>
                      Escribí un mensaje o recuperá el historial disponible en
                      el teléfono.
                    </p>
                  </div>{/if}{/each}
            </div>
          </div>
          {#if infoOpen}<aside class="chat-info">
              <div class="info-title">
                <strong>Información</strong><button
                  class="icon-button"
                  aria-label="Cerrar información"
                  onclick={() => (infoOpen = false)}
                  ><Icon name="close" size={18} /></button
                >
              </div>
              <Avatar
                jid={selected}
                priority
                name={nameFor(selected)}
                group={selected.endsWith('@g.us')}
              />
              <h3>{nameFor(selected)}</h3>
              <code>{selected}</code>{#if selectedGroup}<p>
                  {field(selectedGroup, 'Topic') || 'Sin descripción'}
                </p>
                <h4>Participantes</h4>
                {#each field(selectedGroup, 'Participants') || [] as participant}<div
                    class="participant"
                  >
                    <Avatar
                      jid={field(participant, 'JID') || ''}
                      name={contactName(
                        field(participant, 'JID') || '',
                        contacts,
                      )}
                      small
                    /><span
                      >{contactName(
                        field(participant, 'JID') || '',
                        contacts,
                      )}{#if field(participant, 'IsAdmin')}<small
                          >Administrador</small
                        >{/if}</span
                    >
                  </div>{/each}{/if}<button
                class="secondary full"
                onclick={markRead}
                ><Icon name="check" size={16} />Marcar mensajes leídos</button
              ><button
                class="secondary full"
                onclick={recoverHistory}
                disabled={actionBusy}
                ><Icon name="history" size={16} />Solicitar historial</button
              >
            </aside>{/if}
        </div>
        <footer class="composer-area">
          {#if recording}{#key selected}<Recorder
                onsend={sendVoice}
                oncancel={() => (recording = false)}
              />{/key}
          {:else}<form class="composer" onsubmit={sendText}>
              <input
                type="file"
                class="visually-hidden"
                bind:this={attachmentInput}
                onchange={sendAttachment}
                aria-label="Adjuntar archivo"
              />
              <button
                type="button"
                class="icon-button"
                title="Adjuntar imagen, video, audio o documento"
                aria-label="Adjuntar archivo"
                disabled={sending || !online}
                onclick={() => attachmentInput.click()}
                ><Icon name="clip" size={23} /></button
              >
              <textarea
                bind:value={draft}
                placeholder="Escribí un mensaje…"
                aria-label="Mensaje"
                rows="1"
                disabled={!online}
                onkeydown={(event) => {
                  if (
                    event.key === 'Enter' &&
                    !event.shiftKey &&
                    !event.isComposing
                  ) {
                    event.preventDefault();
                    sendText();
                  }
                }}></textarea>
              {#if draft.trim()}<button
                  class="send-button"
                  title="Enviar mensaje"
                  aria-label="Enviar mensaje"
                  disabled={sending || !online}
                  type="submit"><Icon name="send" size={20} /></button
                >{:else}<button
                  type="button"
                  class="icon-button"
                  title="Grabar nota de voz"
                  aria-label="Grabar nota de voz"
                  disabled={sending || !online}
                  onclick={() => (recording = true)}
                  ><Icon name="mic" size={22} /></button
                >{/if}
            </form>{/if}
          <div class="composer-caption">
            <span
              >{sending
                ? 'Enviando…'
                : readOnlyHint ||
                  'Enter para enviar · Shift + Enter para una nueva línea'}</span
            ><span>{demo ? 'DEMO' : 'WUZAPI'}</span>
          </div>
        </footer>
      {:else}
        <div class="workspace-empty">
          <div class="empty-illustration">
            <span class="illustration-circle one"></span><span
              class="illustration-circle two"
            ></span>
            <div class="illustration-card">
              <Icon name="chat" size={42} /><span></span><span></span>
            </div>
            <div class="illustration-mini">
              <Icon name="mic" size={24} /><i></i><i></i><i></i><i></i><i></i>
            </div>
          </div>
          <span class="eyebrow">UN POCO MÁS CERCA</span>
          <h2>Una conversación.<br />Muchas posibilidades.</h2>
          <p>
            Elegí un chat, encontrá a un contacto<br />o retomá la charla con tu
            equipo.
          </p>
          <button class="primary" onclick={() => (modal = 'new')}
            ><Icon name="plus" size={18} />Nueva conversación</button
          ><small
            ><Icon name="lock" size={13} />Conectado directamente a tu WuzAPI</small
          >
        </div>
      {/if}
    </section>
  </main>
{/if}

{#if modal}
  <div class="modal-backdrop">
    <dialog
      use:showDialog
      aria-labelledby="modal-title"
      class="modal"
      oncancel={(event) => {
        event.preventDefault();
        modal = '';
      }}
    >
      <button
        class="icon-button modal-close"
        aria-label="Cerrar diálogo"
        onclick={() => (modal = '')}><Icon name="close" /></button
      >
      {#if modal === 'settings'}
        <span class="eyebrow">TU INSTANCIA</span>
        <h2 id="modal-title">Historial y sincronización</h2>
        <p>
          El cliente muestra los mensajes guardados por WuzAPI. No modifica tus
          webhooks.
        </p>
        <form onsubmit={saveHistory}>
          <label
            >Mensajes guardados por conversación<input
              type="number"
              min="0"
              max="100000"
              bind:value={retention}
              required
            /></label
          ><label
            >Días solicitados al vincular el teléfono<input
              type="number"
              min="0"
              max="365"
              bind:value={syncDays}
              required
            /></label
          >
          <p class="form-note">
            Guardar 0 mensajes desactiva el historial. Los días se aplican en la
            próxima vinculación; WhatsApp decide cuánto historial entrega. La
            actualización automática ocurre cada 3,5 segundos mientras la
            pestaña está visible.
          </p>
          <button class="primary full" disabled={actionBusy}
            >Guardar configuración</button
          >
        </form>
        <button class="text-button disconnect" onclick={logout}
          ><Icon name="logout" size={16} />Cerrar este cliente (sin desvincular
          WhatsApp)</button
        >
      {:else if modal === 'new'}
        <span class="eyebrow">SEGUIMOS EN CONTACTO</span>
        <h2 id="modal-title">Nueva conversación</h2>
        <p>
          Ingresá un teléfono internacional o el JID completo de un contacto.
        </p>
        <form onsubmit={openDestination}>
          <label
            >Teléfono o JID<input
              bind:value={newDestination}
              placeholder="5491155554444 o 123456789@lid"
              required
            /></label
          >
          <p class="form-note">
            Conservá el sufijo @lid si lo recibiste de WhatsApp. Los usernames y
            BSUIDs de Cloud API no se resuelven desde este campo.
          </p>
          <button class="primary full" disabled={actionBusy}
            >{actionBusy ? 'Resolviendo…' : 'Abrir conversación'}<Icon
              name="arrow"
              size={17}
            /></button
          >
        </form>
        <button
          class="text-button"
          onclick={() => {
            modal = '';
            setMode('contacts');
          }}>Ver mis contactos</button
        >
      {:else if modal === 'group'}
        <span class="eyebrow">UN LUGAR PARA EL EQUIPO</span>
        <h2 id="modal-title">Crear un grupo</h2>
        <form onsubmit={createGroup}>
          <label
            >Nombre del grupo<input
              bind:value={groupName}
              required
              maxlength="100"
              placeholder="Mi equipo"
            /></label
          ><label
            >Participantes<textarea
              bind:value={participants}
              required
              placeholder="5491155554444, 123456789@lid"
              rows="4"></textarea></label
          >
          <p class="form-note">
            Separá los números o JIDs con comas o saltos de línea.
          </p>
          <button class="primary full" disabled={actionBusy}>Crear grupo</button
          >
        </form>
      {:else if modal === 'pair'}
        <span class="eyebrow">VINCULÁ TU TELÉFONO</span>
        <h2 id="modal-title">Tu WhatsApp, conectado</h2>
        <p>
          Abrí WhatsApp → Dispositivos vinculados → Vincular un dispositivo.
        </p>
        {#if qr}<img
            class="qr-code"
            src={qr}
            alt="Código QR para vincular WhatsApp"
          />{:else}<div class="qr-placeholder">
            <Icon name="chat" size={40} /><span
              >Iniciá la conexión para obtener un QR.</span
            >
          </div>{/if}
        <button class="primary full" onclick={pair} disabled={actionBusy}
          >{actionBusy ? 'Conectando…' : 'Conectar y mostrar QR'}</button
        >
        <p class="form-note">
          Si WhatsApp solicita una passkey, completá la vinculación en el
          dashboard de WuzAPI. La sincronización puede tardar unos minutos.
        </p>
        <button class="text-button" onclick={() => (modal = 'settings')}
          >Configurar historial antes de vincular</button
        >
      {/if}
      {#if error}<p class="inline-error modal-error" role="alert">
          {error}
        </p>{/if}
    </dialog>
  </div>
{/if}
