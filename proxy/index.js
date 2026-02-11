#!/usr/bin/env node
const WebSocket = require('ws');
const fs = require('fs');
const path = require('path');

const PROXY_PORT = process.env.ONEBOT_PROXY_PORT ? Number(process.env.ONEBOT_PROXY_PORT) : 3938;
const UPSTREAM = process.env.ONEBOT_UPSTREAM_URL || 'ws://101.34.19.31:13888/onebot/v11/ws';
const BLOCKED_FILE = process.env.BLOCKED_FILE || path.join(__dirname, 'blocked_groups.json');
const FILTER_FILE = process.env.FILTER_FILE || path.join(__dirname, 'filter_rules.json');

let blocked = new Set();
let filterRules = new Map(); // groupId -> [RegExp]

function loadBlocked(){
  try{
    const data = fs.readFileSync(BLOCKED_FILE, 'utf8');
    const arr = JSON.parse(data || '[]');
    blocked = new Set(arr.map(String));
    console.log('[onebot-proxy] blocked groups loaded:', Array.from(blocked));
  }catch(e){
    console.error('[onebot-proxy] load blocked error', e.message);
  }
}

function loadFilterRules(){
  try{
    const raw = fs.readFileSync(FILTER_FILE, 'utf8');
    const obj = JSON.parse(raw || '{}');
    const map = new Map();
    if (obj && typeof obj === 'object'){
      for (const gid of Object.keys(obj)){
        try{
          const arr = obj[gid] || [];
          const regexes = (Array.isArray(arr) ? arr : []).map(s => new RegExp(s));
          map.set(String(gid), regexes);
        }catch(e){
          console.error('[onebot-proxy] invalid regex in filter rules for', gid, e.message);
        }
      }
    }
    filterRules = map;
    console.log('[onebot-proxy] filter rules loaded for groups:', Array.from(filterRules.keys()));
  }catch(e){
    console.error('[onebot-proxy] load filter rules error', e.message);
  }
}

loadBlocked();
loadFilterRules();

fs.watchFile(BLOCKED_FILE, {interval:1000}, () => {
  console.log('[onebot-proxy] blocked file changed, reloading');
  loadBlocked();
});
fs.watchFile(FILTER_FILE, {interval:1000}, () => {
  console.log('[onebot-proxy] filter file changed, reloading');
  loadFilterRules();
});

const wss = new WebSocket.Server({port: PROXY_PORT});
console.log(`[onebot-proxy] Listening on ws://127.0.0.1:${PROXY_PORT}  -> upstream: ${UPSTREAM}`);

wss.on('connection', (client, req) => {
  console.log('[onebot-proxy] client connected:', req.socket.remoteAddress || req.connection.remoteAddress);
  const upstream = new WebSocket(UPSTREAM);

  upstream.on('open', () => {
    console.log('[onebot-proxy] connected to upstream', UPSTREAM);
  });

  upstream.on('message', (data) => {
    // forward upstream -> client
    if (client.readyState === WebSocket.OPEN) client.send(data);
  });

  upstream.on('close', (code, reason) => {
    console.log('[onebot-proxy] upstream closed', code, reason?.toString());
    if (client.readyState === WebSocket.OPEN) client.close();
  });

  upstream.on('error', (err) => {
    console.error('[onebot-proxy] upstream error', err && err.message);
  });

  client.on('message', (data) => {
    // filter messages from NapCat -> bot
    let drop = false;
    try {
      const obj = JSON.parse(data.toString());
      // OneBot v11 event push (post_type: 'message')
      if (obj && obj.post_type === 'message' && obj.message_type === 'group') {
        const gid = obj.group_id || (obj.data && obj.data.group_id) || (obj.extra && obj.extra.group_id);
        if (gid && blocked.has(String(gid))) {
          drop = true;
          console.log('[onebot-proxy] drop group message for fully blocked group', gid);
        } else if (gid) {
          // check per-group regex rules: if any pattern matches, drop
          const rules = filterRules.get(String(gid));
          if (rules && rules.length) {
            const text = extractTextFromOneBotMessage(obj);
            for (const re of rules) {
              try{
                if (re.test(String(text))) {
                  drop = true;
                  console.log('[onebot-proxy] drop group message by rule', gid, re);
                  break;
                }
              }catch(e){
                console.error('[onebot-proxy] regex test error', e.message);
              }
            }
          }
        }
      }
    } catch(e){
      // not JSON or cannot parse — just forward
    }
    if (!drop) {
      if (upstream.readyState === WebSocket.OPEN) upstream.send(data);
    }
  });

  function extractTextFromOneBotMessage(event){
    // common fields: message (string or array), raw_message
    if (!event) return '';
    if (typeof event.message === 'string') return event.message;
    if (Array.isArray(event.message)){
      try{
        return event.message.map(el => {
          if (!el) return '';
          if (typeof el === 'string') return el;
          if (el.type === 'text') return el.text || el.data?.text || '';
          // common other shapes
          return el.text || el.data?.text || el.content?.text || '';
        }).join('');
      }catch(e){
        return '';
      }
    }
    if (typeof event.raw_message === 'string') return event.raw_message;
    // fallback
    return '';
  }

  client.on('close', () => {
    console.log('[onebot-proxy] client disconnected');
    if (upstream.readyState === WebSocket.OPEN) upstream.close();
  });

  client.on('error', (err) => {
    console.error('[onebot-proxy] client error', err && err.message);
  });
});

process.on('unhandledRejection', (r) => console.error('[onebot-proxy] unhandledRejection', r));
process.on('uncaughtException', (e) => console.error('[onebot-proxy] uncaughtException', e && e.stack));
