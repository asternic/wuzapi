// Status broadcasts share the history store but are not conversations.
// Match the protocol JID, never a contact's display name or all broadcasts.
export function isStatusJID(jid) {
  return jid === 'status@broadcast';
}

export function field(object, key) {
  if (!object || typeof object !== 'object') return undefined;
  const target = key.replaceAll('_', '').toLowerCase();
  const found = Object.keys(object).find(
    (k) => k.replaceAll('_', '').toLowerCase() === target,
  );
  return found === undefined ? undefined : object[found];
}
export function storedEvent(row) {
  try {
    return JSON.parse(row.data_json || '{}');
  } catch {
    return {};
  }
}
export function unwrapMessage(value) {
  let message = value || {};
  for (let i = 0; i < 8; i++) {
    const wrapper = [
      'ephemeralMessage',
      'viewOnceMessage',
      'viewOnceMessageV2',
      'viewOnceMessageV2Extension',
      'documentWithCaptionMessage',
    ]
      .map((key) => field(message, key))
      .find(Boolean);
    if (!wrapper) break;
    message = field(wrapper, 'message') || message;
  }
  return message;
}
export function accountJID(jid = '') {
  return jid.replace(/[.:][^@]*(?=@)/, '');
}
export function normalizeMessage(row, ownJID = '') {
  const event = storedEvent(row);
  const info = field(event, 'Info') || {};
  const message = unwrapMessage(
    field(event, 'Message') || field(event, 'RawMessage'),
  );
  const eventTime = field(info, 'Timestamp');
  const timestamp =
    eventTime &&
    Number.isFinite(+new Date(eventTime)) &&
    +new Date(eventTime) > 0
      ? new Date(eventTime).toISOString()
      : row.timestamp;
  const kind = row.message_type;
  const media = field(message, `${kind}Message`);
  const protocol = field(message, 'protocolMessage');
  const edited = unwrapMessage(field(protocol, 'editedMessage'));
  const text =
    field(edited, 'conversation') ||
    field(field(edited, 'extendedTextMessage'), 'text') ||
    row.text_content ||
    '';
  const own =
    row.sender_jid === 'me' ||
    field(info, 'IsFromMe') === true ||
    (!!ownJID && accountJID(row.sender_jid) === accountJID(ownJID));
  return {
    ...row,
    timestamp,
    text: /^:(audio|image|video|document|sticker):$/.test(text) ? '' : text,
    own,
    media,
    info,
    event,
    message,
    target:
      row.quoted_message_id ||
      field(field(protocol, 'key'), 'ID') ||
      field(field(field(message, 'reactionMessage'), 'key'), 'ID'),
    filename: field(media, 'fileName') || 'Archivo adjunto',
  };
}
export function mergeMessages(previous, incoming) {
  const byID = new Map(previous.map((row) => [row.message_id, row]));
  for (const row of incoming)
    byID.set(row.message_id, { ...byID.get(row.message_id), ...row });
  return [...byID.values()].sort(
    (a, b) => new Date(a.timestamp) - new Date(b.timestamp) || a.id - b.id,
  );
}
export function displayMessages(rows, ownJID) {
  const normalized = rows.map((row) => normalizeMessage(row, ownJID));
  const messages = new Map(
    normalized
      .filter(
        (row) => !['edit', 'delete', 'reaction'].includes(row.message_type),
      )
      .map((row) => [row.message_id, { ...row }]),
  );
  for (const row of normalized) {
    const target = messages.get(row.target);
    if (!target) continue;
    if (row.message_type === 'delete') {
      target.deleted = true;
      target.text = 'Mensaje eliminado';
      target.media = null;
      target.media_link = '';
    }
    if (row.message_type === 'edit' && !target.deleted) {
      target.text = row.text;
      target.edited = true;
    }
    if (row.message_type === 'reaction') target.reaction = row.text;
  }
  return [...messages.values()].sort(
    (a, b) => new Date(a.timestamp) - new Date(b.timestamp) || a.id - b.id,
  );
}
export function safeMediaURL(value) {
  if (typeof value !== 'string') return '';
  if (/^data:(audio|image|video)\/[a-z0-9.+-]+(?:;[^,]*)?;base64,/i.test(value))
    return value;
  try {
    const url = new URL(value);
    return ['http:', 'https:'].includes(url.protocol) &&
      !url.username &&
      !url.password
      ? url.href
      : '';
  } catch {
    return '';
  }
}
export function downloadPayload(media) {
  return Object.fromEntries(
    [
      'URL',
      'DirectPath',
      'MediaKey',
      'Mimetype',
      'FileEncSHA256',
      'FileSHA256',
      'FileLength',
    ]
      .map((key) => [key, field(media, key)])
      .filter(([, value]) => value !== undefined),
  );
}
export function contactName(jid, contacts = {}) {
  const contact = contacts[jid];
  return (
    field(contact, 'FullName') ||
    field(contact, 'PushName') ||
    field(contact, 'FirstName') ||
    jid.split('@')[0]
  );
}
export function preview(chat) {
  const names = {
    audio: 'Nota de voz',
    image: 'Foto',
    sticker: 'Sticker',
    video: 'Video',
    document: 'Documento',
    delete: 'Mensaje eliminado',
    reaction: 'Reacción',
    edit: 'Mensaje editado',
  };
  return chat.text_content && !chat.text_content.startsWith(':')
    ? chat.text_content
    : names[chat.message_type] || 'Sin mensajes';
}
