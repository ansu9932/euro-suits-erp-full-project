# Architecture

The SvelteKit frontend uses the Go API for authenticated records and actions. A same-origin gateway or Cloudflare edge proxy routes `/api` requests to the backend. PostgreSQL stores business records and migrations; Redis supports sessions and background coordination. Files are stored through the application's encrypted attachment workflows.

The API owns authentication, CSRF protection, scope and feature permissions, approval transitions, audit recording and version-based concurrent-update protection. UI visibility complements server authorization rather than replacing it. Migration commands run with an owner connection; the API uses its restricted database role.

Schemas and incremental migrations are embedded in `backend/internal/app`. Keep their sequence intact. Initial workspace/account commands and optional demo seeding are distinct from normal runtime. Demo seeding must remain disabled in production.

Operational notifications run through backend workers and private provider configuration. Keep isolated test environments disconnected from real employee recipients. The Spooly frontend mascot is separate from notification delivery and real AI/backend integrations.
