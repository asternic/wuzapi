import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  normalizeMessage,
  mergeMessages,
  displayMessages,
  safeMediaURL,
  downloadPayload,
  isStatusJID,
} from './model.js';

test('live and synchronized messages retain LID identity and media download metadata', () => {
  const media = {
    URL: 'https://example.test/audio',
    directPath: '/encrypted',
    mediaKey: 'AQID',
    mimetype: 'audio/ogg',
    fileLength: 123,
  };
  const row = {
    id: 1,
    message_id: 'a',
    sender_jid: '777@lid',
    chat_jid: '777@lid',
    message_type: 'audio',
    text_content: ':audio:',
    data_json: JSON.stringify({
      Info: { IsFromMe: false },
      Message: { ephemeralMessage: { message: { audioMessage: media } } },
    }),
  };
  const result = normalizeMessage(row, '123@s.whatsapp.net');
  assert.equal(result.chat_jid, '777@lid');
  assert.equal(result.own, false);
  assert.equal(result.text, '');
  assert.deepEqual(downloadPayload(result.media), {
    URL: media.URL,
    DirectPath: '/encrypted',
    MediaKey: 'AQID',
    Mimetype: 'audio/ogg',
    FileLength: 123,
  });
});
test('own messages are recognized from legacy history, linked devices, and event metadata', () => {
  for (const row of [
    { sender_jid: 'me' },
    { sender_jid: '123:8@s.whatsapp.net' },
    { sender_jid: '777@lid', data_json: '{"Info":{"IsFromMe":true}}' },
  ]) {
    assert.equal(normalizeMessage(row, '123@s.whatsapp.net').own, true);
  }
});
test('polling deduplicates messages without discarding older pages or a local attachment preview', () => {
  const first = { id: 1, message_id: 'a', timestamp: '2026-10-20T10:00:00Z' };
  const pending = {
    id: Number.MAX_SAFE_INTEGER,
    message_id: 'b',
    timestamp: '2026-10-20T10:01:00Z',
    previewSource: 'blob:local',
  };
  const persisted = {
    id: 2,
    message_id: 'b',
    timestamp: '2026-10-20T10:01:00Z',
  };
  assert.deepEqual(mergeMessages([first, pending], [persisted]), [
    first,
    { ...pending, ...persisted },
  ]);
});
test('edits and deletions update their target rather than appearing as new chat bubbles', () => {
  const base = {
    id: 1,
    message_id: 'a',
    message_type: 'text',
    text_content: 'Before',
  };
  const edit = {
    id: 2,
    message_id: 'e',
    message_type: 'edit',
    quoted_message_id: 'a',
    text_content: 'After',
  };
  let result = displayMessages([base, edit]);
  assert.equal(result.length, 1);
  assert.equal(result[0].text, 'After');
  assert.equal(result[0].edited, true);
  result = displayMessages([
    base,
    edit,
    { id: 3, message_id: 'd', message_type: 'delete', quoted_message_id: 'a' },
  ]);
  assert.equal(result.length, 1);
  assert.equal(result[0].deleted, true);
});
test('unsafe media links cannot become navigation URLs', () => {
  for (const value of [
    'javascript:alert(1)',
    'file:///etc/passwd',
    'data:text/html;base64,AAAA',
    'https://user:secret@example.test',
  ])
    assert.equal(safeMediaURL(value), '');
  assert.equal(
    safeMediaURL('https://example.test/photo.jpg'),
    'https://example.test/photo.jpg',
  );
  assert.equal(
    safeMediaURL('data:audio/ogg;base64,AAAA'),
    'data:audio/ogg;base64,AAAA',
  );
});
test('malformed legacy event JSON does not prevent rendering a history row', () => {
  assert.equal(
    normalizeMessage({ data_json: '{bad', text_content: 'Hello' }).text,
    'Hello',
  );
});

test('recovered history displays the original WhatsApp date rather than the database import time', () => {
  const rows = [
    {
      id: 1,
      message_id: 'newer',
      timestamp: '2026-10-20T10:00:00Z',
      data_json: JSON.stringify({
        Info: { Timestamp: '2026-10-19T10:00:00Z' },
      }),
    },
    {
      id: 2,
      message_id: 'older',
      timestamp: '2026-10-20T10:01:00Z',
      data_json: JSON.stringify({
        Info: { Timestamp: '2026-10-18T10:00:00Z' },
      }),
    },
  ];
  const messages = displayMessages(rows);
  assert.deepEqual(
    messages.map((m) => m.message_id),
    ['older', 'newer'],
  );
  assert.equal(messages[0].timestamp, '2026-10-18T10:00:00.000Z');
});

test('status broadcasts are excluded without hiding contacts, groups or broadcast lists', () => {
  assert.equal(isStatusJID('status@broadcast'), true);
  for (const jid of [
    '123@s.whatsapp.net',
    '456@lid',
    '123@g.us',
    '123@broadcast',
    'status@s.whatsapp.net',
    '123@newsletter',
    '',
    undefined,
  ]) {
    assert.equal(isStatusJID(jid), false, String(jid));
  }
  const chats = [
    { chat_jid: 'status@broadcast' },
    { chat_jid: '456@lid', name: 'status' },
  ];
  assert.deepEqual(
    chats.filter((chat) => !isStatusJID(chat.chat_jid)),
    [chats[1]],
  );
});
