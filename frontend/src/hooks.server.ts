import type {Handle} from '@sveltejs/kit';
import {proxyApi} from '$lib/server/edge-proxy.mjs';
export const handle: Handle = async ({event, resolve}) => {
 if (event.platform?.env && event.url.pathname.startsWith('/api/')) {
  return proxyApi(event.request,event.platform.env,event.getClientAddress());
 }
 const response = await resolve(event);
 response.headers.set('X-Content-Type-Options','nosniff');
 response.headers.set('X-Frame-Options','DENY');
 response.headers.set('Referrer-Policy','same-origin');
 response.headers.set('Permissions-Policy','camera=(), microphone=(), geolocation=()');
 return response;
};
