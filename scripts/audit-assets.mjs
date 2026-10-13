#!/usr/bin/env node
// Asset & link audit for the Euro Suits ERP frontend.
// Crawls SSR-rendered pages, extracts every href/src reference and verifies
// each one resolves. Usage: node scripts/audit-assets.mjs [baseUrl]
const BASE = process.argv[2] || 'http://127.0.0.1:3000';
const PAGES = ['/', '/signin', '/exports', '/admin', '/finance', '/md', '/developer', '/sampling', '/privacy', '/guide.html'];
const seen = new Set();
const results = [];

async function status(url) {
 try {
  const res = await fetch(url, { method: 'GET', redirect: 'manual' });
  const body = res.headers.get('content-type')?.includes('text/html') ? await res.text() : null;
  return { status: res.status, type: res.headers.get('content-type'), body, size: res.headers.get('content-length') };
 } catch (e) { return { status: 0, error: String(e) }; }
}

for (const page of PAGES) {
 const url = BASE + page;
 const { status: code, body, type } = await status(url);
 if (code !== 200) { results.push({ page, code, type }); continue; }
 results.push({ page, code: 200, type });
 if (!body) continue;
 const refs = [...body.matchAll(/(?:href|src)="([^"#][^"]*)"/g)].map(m => m[1])
  .filter(r => !r.startsWith('data:') && !r.startsWith('mailto:') && !r.startsWith('http') && !r.startsWith('cid:'));
 for (const ref of [...new Set(refs)]) {
  const abs = ref.startsWith('/') ? BASE + ref : new URL(ref, url).href;
  if (seen.has(abs)) continue;
  seen.add(abs);
  const r = await status(abs);
  results.push({ asset: ref, from: page, code: r.status, size: r.size });
 }
}

let bad = 0;
for (const r of results) {
 if (r.asset) {
  const ok = r.code === 200;
  if (!ok) bad++;
  console.log(`${ok ? '  OK ' : 'FAIL '} ${r.code} ${r.asset} (from ${r.from})${r.size ? ' ' + r.size + 'B' : ''}`);
 } else {
  console.log(`${r.code === 200 ? 'PAGE ' : 'PAGE-FAIL '}${r.code} ${r.page} ${r.type || ''}`);
  if (r.code !== 200) bad++;
 }
}
console.log(bad ? `\n${bad} broken reference(s) found.` : '\nAll references resolve.');
process.exit(bad ? 1 : 0);
