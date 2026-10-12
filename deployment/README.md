# Deployment helper source

Sanitized copies of provisioning helpers are provided for completeness. They are not wired to the live ERP and were not executed while publishing this repository.

`hostinger-clean/` contains initial production configuration, account creation, certificate-hook and optional backup helpers. They assume Ubuntu, Docker Compose, application root `/opt/euro-suits-erp` and a Compose wrapper named `dc`. Copy the required helpers to that application root only after reviewing their paths and configuration. These helpers are for deliberately provisioned environments; do not run initial setup over an existing installation.

All actual passwords, developer email, Meta account IDs and API credentials must be supplied privately. Replace `erp.example.com` and `api.example.com` with your own hostnames. `finish-private-setup.py` is historical optional provisioning source, including a backup cron; its presence does not enable or expand backups.

`cloudflare/compose.edge.yaml` expects an operator-provided `deploy/nginx.origin.conf` and origin certificates. The included bootstrap Nginx file is only for ACME validation and returns 404 elsewhere. Configure the final authenticated/HTTPS origin separately for your deployment. Never commit certificates or provider secrets.

Frontend deployments and backend/migration deployments are separate. A Git push to this public snapshot has no production deployment connection.
