// Renders SVG files to PNG with headless Chrome (transparent background kept).
// Usage: node render.mjs <in.svg>:<out.png>:<size> ...
import { spawn } from 'node:child_process';
import { readFileSync, writeFileSync } from 'node:fs';
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const jobs = process.argv.slice(2).map((a) => { const [i, o, s] = a.split(':'); return { i, o, s: +s }; });
const chrome = spawn('/Applications/Google Chrome.app/Contents/MacOS/Google Chrome', [
  '--headless=new', '--remote-debugging-port=9334', '--user-data-dir=/tmp/athlix-render-' + process.pid,
  '--no-first-run', 'about:blank'], { stdio: 'ignore' });
try {
  for (let i = 0; i < 50; i++) { try { await (await fetch('http://127.0.0.1:9334/json/version')).json(); break; } catch { await sleep(200); } }
  const t = await (await fetch('http://127.0.0.1:9334/json/new?about:blank', { method: 'PUT' })).json();
  const ws = new WebSocket(t.webSocketDebuggerUrl);
  await new Promise((r) => ws.addEventListener('open', r));
  let id = 0; const pend = new Map();
  ws.addEventListener('message', (m) => { const x = JSON.parse(m.data); if (x.id && pend.has(x.id)) { pend.get(x.id)(x); pend.delete(x.id); } });
  const send = (method, params = {}) => new Promise((r) => { const n = ++id; pend.set(n, r); ws.send(JSON.stringify({ id: n, method, params })); });
  await send('Page.enable');
  await send('Emulation.setDefaultBackgroundColorOverride', { color: { r: 0, g: 0, b: 0, a: 0 } });
  for (const { i, o, s } of jobs) {
    await send('Emulation.setDeviceMetricsOverride', { width: s, height: s, deviceScaleFactor: 1, mobile: false });
    const svg = readFileSync(i, 'utf8').replace('width="1024" height="1024"', `width="${s}" height="${s}"`);
    const html = `<html><body style="margin:0;background:transparent">${svg}</body></html>`;
    await send('Page.navigate', { url: 'data:text/html;base64,' + Buffer.from(html).toString('base64') });
    await sleep(300);
    const r = await send('Page.captureScreenshot', { format: 'png', clip: { x: 0, y: 0, width: s, height: s, scale: 1 } });
    writeFileSync(o, Buffer.from(r.result.data, 'base64'));
  }
  ws.close();
  console.log(`rendered ${jobs.length}`);
} finally { chrome.kill(); }
