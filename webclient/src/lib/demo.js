const now = Date.now();
const stamp = (minutes) => new Date(now - minutes * 60000).toISOString();
const row = (id, chat, sender, text, minutes, type = 'text', extra = {}) => ({
  id,
  chat_jid: chat,
  sender_jid: sender,
  message_id: `demo-${id}`,
  timestamp: stamp(minutes),
  message_type: type,
  text_content: text,
  ...extra,
});
export const demoContacts = {
  '5491155550101@s.whatsapp.net': { FullName: 'Lucía Fernández' },
  '5491155550102@s.whatsapp.net': { FullName: 'Santiago Méndez' },
  '5491155550103@s.whatsapp.net': { FullName: 'Valentina Ruiz' },
  '5491155550104@s.whatsapp.net': { FullName: 'Mateo Torres' },
};
export const demoGroups = [
  {
    JID: '120363000000001@g.us',
    Name: 'Equipo de producto',
    Topic: 'Ideas, avances y un poco de café.',
    Participants: [
      { JID: '5491155550101@s.whatsapp.net' },
      { JID: '5491155550102@s.whatsapp.net' },
    ],
  },
  { JID: '120363000000002@g.us', Name: 'Comunidad WuzAPI', Participants: [] },
];
export const demoMessages = [
  row(
    1,
    '5491155550101@s.whatsapp.net',
    '5491155550101@s.whatsapp.net',
    '¡Buen día! ¿Cómo viene el nuevo cliente?',
    38,
  ),
  row(
    2,
    '5491155550101@s.whatsapp.net',
    'me',
    'Ya tenemos las conversaciones, los contactos y todo el historial en un solo lugar.',
    36,
  ),
  row(
    3,
    '5491155550101@s.whatsapp.net',
    '5491155550101@s.whatsapp.net',
    'Me encanta que también se pueda usar con grupos 🙌',
    34,
  ),
  row(
    4,
    '5491155550101@s.whatsapp.net',
    'me',
    'Sí, y podés recuperar mensajes anteriores directamente desde el teléfono.',
    31,
  ),
  row(
    5,
    '5491155550101@s.whatsapp.net',
    '5491155550101@s.whatsapp.net',
    '',
    27,
    'audio',
  ),
  row(
    6,
    '5491155550101@s.whatsapp.net',
    'me',
    'Perfecto. Después probamos las notas de voz con una sesión real.',
    24,
  ),
  row(
    7,
    '5491155550101@s.whatsapp.net',
    '5491155550101@s.whatsapp.net',
    '¡Dale! Te escribo cuando esté lista ☕',
    2,
  ),
  row(
    8,
    '120363000000001@g.us',
    '5491155550102@s.whatsapp.net',
    'Les dejé las últimas novedades del proyecto.',
    8,
  ),
  row(
    9,
    '5491155550102@s.whatsapp.net',
    '5491155550102@s.whatsapp.net',
    'Gracias por la ayuda, quedó funcionando.',
    19,
  ),
  row(
    10,
    '5491155550103@s.whatsapp.net',
    'me',
    'Nos vemos mañana a las 10.',
    65,
  ),
  row(
    11,
    '120363000000002@g.us',
    '5491155550101@s.whatsapp.net',
    'Bienvenidos a la comunidad 👋',
    130,
  ),
  row(
    12,
    '5491155550104@s.whatsapp.net',
    '5491155550104@s.whatsapp.net',
    '¿Pudiste revisar el documento?',
    180,
  ),
];
export const demoChats = Object.values(
  Object.groupBy(demoMessages, (row) => row.chat_jid),
).map((rows) => rows.at(-1));
