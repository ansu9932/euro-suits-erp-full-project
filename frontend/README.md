# Euro Suits ERP frontend

SvelteKit / Svelte / TypeScript frontend for the full application in the parent directory. See the root README for integrated setup.

- Install: `npm ci`
- Development: `npm run dev`
- Type checks: `npm run check`
- Node build: `npm run build`
- Cloudflare Pages build: `npm run build:cloudflare`

Cloudflare configuration uses example origins. Supply your own deployment project and backend/public origins. API access is enforced by the Go backend; never place service credentials in frontend code.

UI changes should preserve the navy/teal identity, readable labels, persistent task controls, desktop/tablet layouts, reduced-motion support and immediate business actions. Reuse Anime.js through `src/lib/motion.ts` with cleanup when components are destroyed.
