import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';
// allowedHosts lets sandboxed preview proxies (*.e2b.app) reach the dev server;
// localhost remains allowed by default. The /api proxy targets the Go backend,
// or scripts/mock-api.mjs for UI-only testing without the backend stack.
export default defineConfig({plugins:[sveltekit()],server:{port:5173,strictPort:true,allowedHosts:['.e2b.app'],proxy:{'/api':{target:process.env.ERP_API_TARGET || 'http://127.0.0.1:8081'}}}});
