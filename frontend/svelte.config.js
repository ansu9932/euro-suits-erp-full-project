import nodeAdapter from '@sveltejs/adapter-node';
import cloudflareAdapter from '@sveltejs/adapter-cloudflare';
const cloudflare = process.env.DEPLOY_TARGET === 'cloudflare';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';
export default {
 preprocess: vitePreprocess(),
 kit: {
  adapter: cloudflare ? cloudflareAdapter() : nodeAdapter(),
  csp: {mode:'auto', directives: {
   'default-src':['self'],
   'script-src':['self','https://challenges.cloudflare.com'],
   'style-src':['self','unsafe-inline'],
   'img-src':['self','data:'],
   'font-src':['self'],
   'connect-src':['self'],
   'frame-src':['https://challenges.cloudflare.com'],
   'frame-ancestors':['none'],
   'object-src':['none'],
   'base-uri':['self'],
   'form-action':['self']
  }}
 }
};
