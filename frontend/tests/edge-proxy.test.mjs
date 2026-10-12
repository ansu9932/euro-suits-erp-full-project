import {test} from 'node:test';
import assert from 'node:assert/strict';
import {proxyApi} from '../src/lib/server/edge-proxy.mjs';
const config={ERP_PUBLIC_ORIGIN:'https://erp.example.com',ERP_BACKEND_ORIGIN:'https://origin.example.com',ERP_PROXY_SECRET:'unit-test-secret'};
test('streams body, preserves session and CSRF, replaces forged IP, never follows redirects',async()=>{
 const req=new Request('https://erp.example.com/api/upload?order=3',{method:'POST',headers:{cookie:'erp_session=example','x-csrf-token':'csrf','idempotency-key':'unique','x-erp-client-ip':'attacker','x-erp-proxy-key':'attacker','x-real-ip':'attacker',origin:'https://erp.example.com'},body:'sample-file'});
 const response=await proxyApi(req,config,'203.0.113.9',async(r,options)=>{
  assert.equal(r.url,'https://origin.example.com/api/upload?order=3');assert.equal(r.redirect,'manual');
  assert.equal(r.headers.get('cookie'),'erp_session=example');assert.equal(r.headers.get('x-csrf-token'),'csrf');assert.equal(r.headers.get('idempotency-key'),'unique');assert.equal(r.headers.get('origin'),config.ERP_PUBLIC_ORIGIN);
  assert.equal(r.headers.get('x-erp-client-ip'),'203.0.113.9');assert.equal(r.headers.get('x-erp-proxy-key'),config.ERP_PROXY_SECRET);assert.equal(r.headers.get('x-real-ip'),null);assert.equal(await r.text(),'sample-file');assert.equal(options.cache,'no-store');
  return new Response('response',{headers:{'Set-Cookie':'erp_session=new; Secure; HttpOnly; SameSite=Strict','Cache-Control':'public','CDN-Cache-Control':'max-age=500'}});
 });
 assert.match(response.headers.get('set-cookie'),/HttpOnly/);assert.equal(response.headers.get('cache-control'),'private, no-store');assert.equal(response.headers.get('cdn-cache-control'),null);assert.equal(await response.text(),'response');
});
test('preview hosts cannot reach private APIs and incomplete configuration fails closed',async()=>{
 const noFetch=()=>{throw Error('must not reach backend')};
 assert.equal((await proxyApi(new Request('https://preview.pages.dev/api/session'),config,'203.0.113.9',noFetch)).status,403);
 assert.equal((await proxyApi(new Request('https://erp.example.com/api/session'),{...config,ERP_PROXY_SECRET:''},'203.0.113.9',noFetch)).status,503);
 assert.equal((await proxyApi(new Request('https://erp.example.com/api/session'),{...config,ERP_BACKEND_ORIGIN:'http://origin.example.com'},'203.0.113.9',noFetch)).status,503);
});
test('backend failure returns generic error without private details',async()=>{
 const response=await proxyApi(new Request('https://erp.example.com/api/session'),config,'203.0.113.9',async()=>{throw Error(config.ERP_PROXY_SECRET)});
 assert.equal(response.status,502);assert.ok(!(await response.text()).includes(config.ERP_PROXY_SECRET));
});
