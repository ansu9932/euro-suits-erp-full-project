# Local UI demo (no backend stack required)

The Company Setup workspace (`/finance`, `/md`, `/admin`) normally needs the Go API,
Postgres and Redis. For **layout / CSS / UX testing** the repo ships a zero-dependency
mock of those API contracts: [`scripts/mock-api.mjs`](../scripts/mock-api.mjs).
It mirrors `backend/internal/app` (login, session, company-setup list/detail/actions,
field & port registers, multipart uploads, reminders) with seeded demo data.

## Run it (two terminals)

```bash
# terminal 1 — mock API on 127.0.0.1:8081
node scripts/mock-api.mjs

# terminal 2 — SvelteKit dev server on http://localhost:5173 (proxies /api to the mock)
cd frontend
npm install        # first time only
npm run dev
```

Open **http://localhost:5173/finance** in your browser.

## Test accounts

| Role | Username | Password | Sees |
|---|---|---|---|
| Finance Head | `finance` | `finance123` | editable drafts, submissions, documents, reminders |
| MD | `md` | `md123` | approval view (approve / request changes) |
| Super Admin | `admin` | `admin123` | everything incl. user access & private KYC |

Turnstile is disabled by the mock (`login-config` returns an empty site key), so
sign-in works with username + password only. No OTP is triggered.

## Seeded scenarios (5 companies)

- **Approved** internal company → read-only summary, privacy banner, full KYC checklist
- **Pending** buyer → submit/approve action bar
- **Changes requested** supplier → amber warning + reviewer note
- **Draft** UK branch → empty-state sections
- **Approved** UK buyer → country-variant fields (VAT), no privacy restriction for MD/Admin

Plus 3 renewal reminders (one overdue → sidebar badge), 6 port registry rows,
7 field definitions (global + one company-scoped), and per-company history timelines.

Writes (save / submit / approve / amend / request changes / delete / uploads /
register adds) mutate in-memory state, so buttons behave live for the session.
**Restarting `mock-api.mjs` restores the pristine seed data** (login sessions survive
restarts via a temp file).

Delete is validated like production: type the exact company name + your password,
otherwise the form shows the 403 error state.

## Notes

- The Vite dev server proxies `/api` to `ERP_API_TARGET` (default `http://127.0.0.1:8081`),
  so pointing it at a real backend is one env var: `ERP_API_TARGET=http://host:8080 npm run dev`.
- Hosted sandbox previews (e.g. `https://5173-<sandbox>.e2b.app`) are gated by the
  platform's traffic-access token and only reachable through the host UI's preview
  panel; use this local setup for unrestricted full-tab testing.
