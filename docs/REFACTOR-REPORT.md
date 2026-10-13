# Euro Suits ERP — Frontend Audit & Refactor Report

Date: 2026-10-12 · Branch: `arena/c11fb5bd-euro-suits-erp-full-project` · Scope: `frontend/` (SvelteKit UI)

---

## 1. Asset & CSS integration audit

**Method.** Added a repeatable audit tool, `scripts/audit-assets.mjs`. It crawls every SSR-rendered
route (`/`, `/signin`, `/exports`, `/admin`, `/finance`, `/md`, `/developer`, `/sampling`,
`/privacy`, `/guide.html`), extracts every `href`/`src` reference (stylesheets, fonts, images,
inter-page links) and verifies each resolves with HTTP 200.

**Result: no broken asset links.** All CSS bundles, `logo.png`, both font families
(DM Sans 400–700, Manrope 400–800, woff + woff2), `/guide.html`, `/privacy` and all 19 portal
sign-in routes resolve correctly — before **and** after the refactor. Run it any time with:

```bash
cd frontend && npm run build && (PORT=3100 node build &) && node scripts/audit-assets.mjs http://127.0.0.1:3100
```

**Integration problems found & fixed**

| Problem | Fix |
|---|---|
| Two global theme layers fighting with `!important` + `:has()` specificity: `readability.css` (loaded on **every** page, including the marketing landing) vs `dashboard.css` (workspace theme). The landing page was forced into `font-size:16px!important` paragraphs and had to defend itself with doubled `.es-site.es-site` selectors. | `readability.css` deleted. Its still-relevant rules (sticky dialog headers, compact ETF/ESF material dialogs, dialog title band) merged into `dashboard.css`, scoped to the workspace. Landing page is now self-contained (ships exactly 2 stylesheets). |
| Scattered, duplicated CSS imports: `interactions.css` re-imported in 3 components although `+layout.svelte` already loads it globally; `app.css`/`company.css` imported mid-script after other CSS in inconsistent order. | One ordered import block at the top of each workspace shell: base (`app.css`) → module (`company.css`) → theme (`dashboard.css`). Redundant imports removed. |
| `app.html` was a single minified line with a redundant `<div style="display:contents">` body wrapper, a stale `theme-color` (`#15283d`), and no apple-touch-icon. | Reformatted; wrapper removed; `theme-color` aligned to the actual nav (`#142b40`); `apple-touch-icon` added. |
| `exports` page used `.875rem` for the account label, which silently depended on the deleted forced `:root{font-size:16px!important}`. | Converted to px (`13px`). |

## 2. UI/UX polish & modernization

- **Single design-token source.** `app.css` `:root` now defines the palette/radius/motion tokens
  (`--primary:#087e78`, `--nav:#142b40`, `--border:#dce5eb`, `--radius-*`, `--motion-fast`, …);
  `dashboard.css` redefines them only inside `body:has(.erp-workspace)`. The two layers previously
  carried slightly different hard-coded values for the same concepts.
- **Typography fixed.** 21 declarations used synthesized font weights (`450/550/650/750`) that no
  loaded font file provides. Normalized across **all** components to the real weights browsers
  were already resolving to (`500/600/700/800`) — crisper rendering, zero visual regression.
- **Button states completed.** Added `text-button:disabled`, a proper
  `.button.secondary.danger` (used by Delete company / Delete user) with hover state; focus rings
  unified on the `--focus` token.
- **Responsive bug fixed (the big one).** Below 761px **all three workspace shells were broken**:
  the theme's `position:fixed` sidebar + `margin-left:var(--sidebar-width)` out-specificied the
  mobile rules in `app.css`/`company.css`, squeezing content to a strip next to an off-canvas or
  invisible sidebar. `dashboard.css` now owns a `≤760px` block: `--sidebar-width:0`,
  company/developer navigation collapses into a horizontal scrolling bar, operations keeps its
  drawer, top bar/content padding tightened.
- **Consistent dialogs.** One navy sticky title-band rule for all six dialog/drawer headers
  (previously duplicated with conflicting colors `#172b3e` vs `#142b40` in two files).

## 3. Whitespace & layout cleanup

- **Padding bloat removed:** `.panel-header` fixed `min-height:73px` dropped;
  `.company-content` bottom padding 120px → 48px; `.company-empty` 70/80px → 48/24px;
  asymmetric paddings normalized (`29px 32px 15px` → `30px 32px 48px`, metric margins,
  page-heading rhythm aligned to theme values).
- **Dead overrides removed:** ~2/3 of `DeveloperWorkspace.svelte`'s `<style>` block (sidebar,
  top bar, content rules the theme already overrides at higher specificity), plus the
  theme-overridden shell rules in `company.css`'s `≤1100px`/`≤760px` blocks.
- **`prefers-reduced-motion` deduplicated:** four copy-pasted blocks (app/company/dashboard/
  interactions) reduced to the one global block in `interactions.css`.
- **Readability:** all minified single-line stylesheets (`app.css`, `company.css`,
  `materials.css`, `interactions.css`, landing files) reformatted to multi-line — ~4,000 lines of
  now-reviewable CSS, zero behaviour change.
- **`!important` war demilitarized:** 70 `!important` declarations removed, 44 net. Kept only
  where they still earn their place (`.es-primary` color vs `.es-site a{color:inherit}`,
  reduced-motion guards, theme layer overrides).

## 4. Files modified

| File | Change |
|---|---|
| `frontend/src/app.html` | Reformatted; removed empty wrapper div; theme-color + apple-touch-icon |
| `frontend/src/app.css` | Tokens, weight normalization, padding/margin cleanup, button states, formatted |
| `frontend/src/dashboard.css` | Sole workspace theme layer; merged dialog rules; **new ≤760px mobile block** |
| `frontend/src/readability.css` | **Deleted** (merged into dashboard.css) |
| `frontend/src/company.css` | Dead shell overrides pruned, padding normalized, weights, formatted |
| `frontend/src/materials.css`, `interactions.css` | Formatted (materials); reduced-motion dedupe (interactions) |
| `frontend/src/lib/landing/{landing,story,motion}.css` | `.es-site.es-site` hacks + 40+ defensive `!important`s removed |
| `frontend/src/routes/+layout.svelte` | readability.css import removed |
| `frontend/src/routes/exports/+page.svelte` | rem → px fix |
| `frontend/src/lib/{Operations,Company,Developer}Workspace.svelte` | Import consolidation; DeveloperWorkspace dead styles removed |
| `frontend/src/lib/{Enquiry,Export,ExportBankEmail,ExportReconciliation,Performance}Workspace.svelte` | Font-weight normalization |
| `scripts/audit-assets.mjs` | **New** — repeatable asset/link audit tool |

## Verification performed

- `npm run build` — clean ✓
- `svelte-check` — **0 errors, 0 warnings** ✓
- `npm run test:proxy` — 3/3 pass ✓
- `scripts/audit-assets.mjs` — all references resolve ✓
- Bundle inspection: no `readability.css` remnants in any output chunk; mobile block, sticky
  headers and tokens confirmed present in the workspace bundle ✓

## Suggested manual QA (backend not available in this environment)

1. **Phones:** open Operations, Company Setup and Developer workspaces ≤760px — first release
   where all three lay out correctly.
2. Sign-in screens: inputs now follow the intended login block (48px tall, `#edf3f7` fill).
3. ETF/ESF material dialogs: compact mint surface + sticky headers.
4. Landing page on a fresh cache (no forced global typography anymore).
