const {test} = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const fs = require('node:fs');
const path = require('node:path');

function setup() {
  const elements = new Map();
  function element() {
    const classes = new Set();
    return {value: '', checked: false, disabled: false, textContent: '', children: [], style: {},
      classList: {add: (...names) => names.forEach(n => classes.add(n)), remove: (...names) => names.forEach(n => classes.delete(n)), contains: n => classes.has(n)},
      appendChild(child) {this.children.push(child);}, addEventListener() {}};
  }
  const document = {addEventListener() {}, getElementById(id) {if (!elements.has(id)) elements.set(id,element()); return elements.get(id);}, createElement: element, createTextNode(text) {return {textContent: text};}};
  const context = vm.createContext({document, window: {location: {origin: 'https://api.example'}}, console, Headers, setTimeout, clearTimeout,
    localStorage: {getItem: () => 'session-token'}, fetch: async () => {throw new Error('unexpected fetch');}});
  vm.runInContext(fs.readFileSync(path.join(__dirname,'../static/dashboard/js/app.js'),'utf8'),context);
  vm.runInContext("generateMessageUUID = () => 'TEST-ID'; getLocalStorageItem = () => 'session-token';",context);
  return {context,document};
}

test('mention-all sends group flag and clears it when recipient changes', async () => {
  const {context:c,document:d}=setup();const phone=d.getElementById('messagesendphone');const box=d.getElementById('messageMentionAll');
  phone.value='123-456@g.us';c.updateMentionAllAvailability();assert.equal(box.disabled,false);box.checked=true;
  d.getElementById('messagesendtext').value='Please read';
  let sent;c.fetch=async (url,options) => {sent={url,options};return {json:async()=>({success:true,data:{Id:'TEST-ID'}})};};
  await c.sendTextMessage();assert.equal(sent.url,'https://api.example/chat/send/text');assert.equal(sent.options.headers.token,'session-token');assert.equal(JSON.parse(sent.options.body).MentionAll,true);
  phone.value='5491112345678';c.updateMentionAllAvailability();assert.equal(box.checked,false);assert.equal(box.disabled,true);
  await c.sendTextMessage();assert.equal(JSON.parse(sent.options.body).MentionAll,false);
  box.checked=true;await assert.rejects(c.sendTextMessage(),/requires a group/);
  box.checked=false;d.getElementById('messagesendtext').value=' ';await assert.rejects(c.sendTextMessage(),/recipient and message/);
});

test('business lookup preserves LID, safely renders data and restores loading state',async()=>{
  const {context:c,document:d}=setup();d.getElementById('userinfoinput').value='123@lid';
  const malicious='<img src=x onerror=alert(1)>';
  let request;c.fetch=async(url,options)=>{request={url,options};return {ok:true,json:async()=>({success:true,data:{JID:'123@lid',Address:malicious,Email:'support@example.com'}})};};
  await c.doBusinessProfile();assert.equal(request.url,'https://api.example/user/business-profile');assert.equal(JSON.parse(request.options.body).Phone,'123@lid');assert.equal(request.options.headers.token,'session-token');
  const container=d.getElementById('businessProfileContainer');assert.equal(container.children[1].children[1].textContent,malicious);assert.equal(container.children[1].innerHTML,undefined);
  assert.equal(d.getElementById('getBusinessProfile').disabled,false);assert.equal(container.classList.contains('hidden'),false);
});

test('business lookup handles invalid input, nonbusiness accounts and network failures',async()=>{
  const {context:c,document:d}=setup();const input=d.getElementById('userinfoinput');const output=d.getElementById('businessProfileContainer');
  input.value='123@g.us';await c.doBusinessProfile();assert.match(output.textContent,/individual/);
  input.value='123';c.fetch=async()=>({ok:false,json:async()=>({success:false,error:'Not a business account'})});await c.doBusinessProfile();assert.match(output.textContent,/Not a business account/);
  c.fetch=async()=>{throw new Error('offline');};await c.doBusinessProfile();assert.match(output.textContent,/try again/);assert.equal(d.getElementById('getBusinessProfile').disabled,false);
});

test('stale business results cannot overwrite another account lookup',async()=>{
  const {context:c,document:d}=setup();d.getElementById('userinfoinput').value='123';
  let finish;c.fetch=()=>new Promise(resolve=>{finish=resolve;});const pending=c.doBusinessProfile();
  d.getElementById('userinfoinput').value='456';c.resetBusinessProfile();
  finish({ok:true,json:async()=>({success:true,data:{Address:'Old account'}})});await pending;
  assert.equal(d.getElementById('businessProfileContainer').textContent,'');assert.equal(d.getElementById('businessProfileContainer').classList.contains('hidden'),true);
});
