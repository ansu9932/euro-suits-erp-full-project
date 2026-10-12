# Publication notes

Prepared on 12 October 2026 from the existing Euro Suits ERP source. Frontend snapshot: `194c6904e434ecb200a96d1af4a3e6077cb8cc33`. Backend includes schema and migrations through version 26. This repository starts with fresh history.

Included: application source, source assets, dependency lock files, embedded catalogs, synthetic fixtures, migration SQL, deployment helper source, and test/tooling source.

Excluded: all `.env` and private credential files, SSH keys, service tokens, certificates, local or production databases/backups, customer uploads, generated account manifests, emails, recordings, spreadsheets, screenshots, operational evidence and historical handover reports. Those are not required to distribute application source.

Public-copy adjustments:

- Local setup generates a random password instead of the former fixed local-only default.
- The WhatsApp test recipient starts empty.
- Cloudflare bindings and host-specific deployment examples use example hostnames.
- Deployment helpers ask for the developer email and Meta identifiers instead of embedding them.
- A frontend test uses generic company names.
- Stale/private operational documentation is replaced by public setup and architecture notes.

These packaging adjustments affect only this exported copy. The active production source and deployment repository were not modified. Source inventory and secret-pattern checks were performed; application tests and deployment actions were not part of this publication.
