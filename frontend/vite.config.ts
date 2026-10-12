import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';
export default defineConfig({plugins:[sveltekit()],server:{port:5173,strictPort:true,proxy:{'/api':{target:process.env.ERP_API_TARGET || 'http://127.0.0.1:8081'}}}});
