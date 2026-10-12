// Server-only bridge: browser credentials never leave the approved ERP backend.
export async function proxyApi(request, config, clientIP, send = fetch) {
 const url = new URL(request.url);
 const fail = (status, error) => Response.json({error}, {status, headers:{'Cache-Control':'private, no-store'}});
 if (!url.pathname.startsWith('/api/')) return fail(404, 'Not found.');
 if (url.origin !== config.ERP_PUBLIC_ORIGIN && url.pathname !== '/api/health') return fail(403, 'Use the official Euro Suits ERP address to sign in.');
 if (!config.ERP_PROXY_SECRET || !clientIP) return fail(503, 'The secure API connection is not configured.');
 let target;
 try { target = new URL(config.ERP_BACKEND_ORIGIN); } catch { return fail(503, 'The secure API connection is not configured.'); }
 if (target.protocol !== 'https:' || target.username || target.password || target.origin === url.origin) return fail(503, 'The secure API connection is not configured.');
 target.pathname = url.pathname;
 target.search = url.search;
 const headers = new Headers(request.headers);
 for (const key of [...headers.keys()]) {
  if (key.startsWith('cf-') || key.startsWith('x-forwarded-') || key.startsWith('x-erp-') || ['host','forwarded','x-real-ip','connection','transfer-encoding','content-length'].includes(key)) headers.delete(key);
 }
 headers.set('X-ERP-Proxy-Key',config.ERP_PROXY_SECRET);
 headers.set('X-ERP-Client-IP',clientIP);
 // Preserve Origin, cookies, CSRF and idempotency headers. Never follow redirects
 // with private credentials, cache API responses, or buffer upload/download bodies.
 const upstreamRequest = new Request(target, {
  method:request.method, headers,
  body:['GET','HEAD'].includes(request.method)?undefined:request.body,
  duplex:'half', redirect:'manual', signal:AbortSignal.timeout(40000)
 });
 try {
  const upstream = await send(upstreamRequest,{cache:'no-store'});
  const response = new Response(upstream.body,upstream);
  response.headers.set('Cache-Control','private, no-store');
  response.headers.delete('CDN-Cache-Control');
  response.headers.delete('Cloudflare-CDN-Cache-Control');
  return response;
 } catch { return fail(502, 'The ERP service is temporarily unavailable. Please try again.'); }
}
