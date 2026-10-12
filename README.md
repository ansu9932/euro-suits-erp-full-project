# Euro Suits ERP

Full application source for the Euro Suits garment manufacturing ERP: the SvelteKit frontend, Go API, PostgreSQL migrations, background jobs, deployment templates and test tooling.

This public repository is a source snapshot. It is separate from the private production deployment repository and has no automatic connection to the live ERP.

## Stack

- Go 1.27 / Chi API
- SvelteKit 2 / Svelte 5 / TypeScript; Anime.js 4.5.0
- PostgreSQL 18 and Redis
- Docker Compose and Nginx; optional Cloudflare Pages frontend

## Included modules

Company setup, KYC attachments and renewal reminders; users, designations and scoped permissions; garment operations and production planning; material and sample management; enquiry, costing, approvals and booking; export invoices, JV, receipts and banking, reconciliation and eBRC; developer administration, audit, diagnostics and notification integrations.

Feature availability is controlled by server-side permissions. The source also contains work that may be disabled or pending business rollout; its presence does not imply that a feature is enabled in production.

## Source layout

| Directory | Contents |
| --- | --- |
| `backend/` | Go API, workers, embedded catalogs, schema and migrations through v26 |
| `frontend/` | Dashboard UI, public pages, assets, Cloudflare API proxy and frontend tests |
| `deploy/` | Nginx templates used by Docker Compose |
| `deployment/` | Sanitized production provisioning and edge deployment helper source |
| `scripts/` | Database role provisioning, backup and security helpers |
| `tests/` | Integration, workflow, access and regression test source |
| `testing/` | Optional isolated review environment tools; private generated data excluded |
| `docs/` | Public architecture and publication notes |

## Fresh local installation

Requires Docker with Compose and Python 3. Run from the repository root against a new, empty local workspace:

```sh
python3 setup-local.py
docker compose -f compose.yaml -f compose.dev.yaml up -d postgres redis
python3 scripts/provision_database.py
docker compose build migrate api web
docker compose run --rm migrate initialize-company-workspace
docker compose -f compose.yaml -f compose.dev.yaml up -d
```

Open **http://localhost:8080**. The setup script generates a random local password and writes it to the private `LOCAL-ACCESS.txt` file. Local role accounts are `finance`, `md` and `admin`. Existing `.env` files are retained; initialization refuses a populated company workspace. Never run initialization or reset commands against an existing production database.

For an existing local installation, use `docker compose -f compose.yaml -f compose.dev.yaml up -d --build`. Keep the original encryption key and database volumes to retain access to stored documents.

SMTP, WhatsApp, developer email verification and external services require your own private configuration. Defaults do not contain service credentials. Do not configure real recipients when using test environments.

## Frontend development

```sh
cd frontend
npm ci
npm run dev
```

The dev server runs on port 5173 and proxies `/api` to `http://127.0.0.1:8081` unless `ERP_API_TARGET` is set. Run a separately configured local backend with a matching application origin. The Compose installation above provides the integrated application on port 8080.

`npm run check`, `npm run build` and the files under `frontend/tests/` are available for validation. Integration tools under `tests/` require disposable databases and their documented environment settings; do not point them at production. Publication of this snapshot did not rerun the application test suites.

## Deployment

The frontend supports Node and Cloudflare adapters. `npm run build:cloudflare` selects Cloudflare; configure your own project and origins in `frontend/wrangler.jsonc`. Example hostnames deliberately do not point to live infrastructure.

The backend is deployed independently with Docker Compose. See `deployment/README.md` before using its provisioning helpers. This repository does not include live DNS, SSH keys, provider tokens, database credentials or active server configuration.

## Public snapshot scope

See [publication notes](docs/PUBLICATION.md). Private databases, uploaded documents, employee accounts, secret configuration, historical deployment evidence and original Git history are excluded. Synthetic fixture data remains in source for isolated development; demo seeding is disabled by default and is forbidden in production.
