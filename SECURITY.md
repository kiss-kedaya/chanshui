# Security Policy

## Supported versions

Security fixes target the latest GitHub Release and the current `main` branch. Older releases may not receive backports.

## Reporting a vulnerability

Please use **GitHub Security Advisories** for this repository. Do not open a public issue for a vulnerability, leaked credential, or a bypass that could affect a deployed instance.

Include:

- affected version or commit;
- reproduction steps and impact;
- whether the issue is reachable anonymously or requires the admin token;
- relevant logs, with API keys, cookies, job tokens and feedback contacts removed.

## Sensitive data boundaries

- `CHANSHUI_ADMIN_TOKEN` protects `/admin` and the private feedback list. Generate it with `openssl rand -hex 24` (48 hex characters), keep it in a root-only environment file, and rotate it if it is exposed. The admin login endpoint is rate-limited.
- Upstream API keys are used for the current request only. They are not stored in SQLite, reports, logs or share links.
- Job operation keys are returned once at creation. The server stores only a SHA-256 digest; public report IDs are read-only.
- Reports expire after 24 hours by default. Feedback is kept separately and is not deleted by report expiry.
- Feedback contacts are private. The only readers are `/admin` and the offline `feedback-export` tool. Never serve an export from the web root.

## Deployment checklist

- Terminate TLS at a reverse proxy and do not expose the application port directly.
- Put a WAF / rate limit in front of `/api/*` and `/admin`.
- Keep the SQLite data directory outside the static web root; restrict it to the service user.
- Set a strong admin token, or leave it unset to disable the admin route.
- Do not enable `-allow-private` in production.
- Back up the database and test restore before upgrades.
- Use the blue/green procedure in `docs/deploy/rolling-upgrade.md`; do not restart a single instance while jobs are running.

## Known boundaries

- The default deployment is a single SQLite instance. It is not a distributed queue or a multi-tenant isolation boundary.
- Public API rate limiting and the public audit counter are in-process; they reset on restart and are not shared between blue/green instances.
- Fingerprint and behavioral attribution are evidence tools, not cryptographic proof of a model's identity.
- The project is provided without a warranty; see `LICENSE`.
