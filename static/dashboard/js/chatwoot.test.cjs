const fs = require('fs');
const path = require('path');

const html = fs.readFileSync(path.join(__dirname, '..', 'index.html'), 'utf8');
const app = fs.readFileSync(path.join(__dirname, 'app.js'), 'utf8');

function requireText(source, text) {
  if (!source.includes(text)) {
    throw new Error(`missing ${text}`);
  }
}

function functionBody(name) {
  const start = app.indexOf(`async function ${name}()`);
  const end = app.indexOf('\nasync function ', start + 1);
  if (start === -1) throw new Error(`missing function ${name}`);
  return app.slice(start, end === -1 ? app.length : end);
}

requireText(html, 'id="chatwootConfig"');
requireText(html, 'id="modalChatwootConfig"');
requireText(html, 'id="chatwootToken" type="password"');
requireText(html, 'id="chatwootWebhookUrl"');
requireText(html, 'id="saveChatwootConfig"');
requireText(html, 'id="disableChatwootConfig"');
requireText(app, 'function loadChatwootConfig()');
requireText(app, 'function saveChatwootConfig()');
requireText(app, 'function disableChatwootConfig()');
requireText(app, "'/chatwoot/config'");
requireText(app, "'/chatwoot/test'");
requireText(app, "'/chatwoot/status'");
requireText(functionBody('saveChatwootConfig'), "'/chatwoot/config'");
requireText(functionBody('testChatwootConnection'), "'/chatwoot/test'");
