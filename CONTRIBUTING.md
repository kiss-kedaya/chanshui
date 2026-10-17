# Contributing

## Development setup

Requirements: Go 1.22+, Node.js 20/22 and npm.

```bash
cd web
npm ci
npm run build
cd ..

go test -race ./...
go vet ./...
```

On Windows, `scripts/start-local.ps1` builds and starts the local server.

## Before opening a pull request

- `gofmt` all Go files.
- Run `go test -race ./...` and `go vet ./...`.
- Run frontend `npm test`, `npm run typecheck` and `npm run build`.
- Check mobile layouts at 320, 360, 390 and 430 CSS pixels; verify orange/mint and light/dark themes.
- Keep API keys, cookies, job tokens, feedback contacts, databases, `.env` files and generated `.artifacts/` out of the commit.
- Add or update tests when behavior changes. For scoring or protocol changes, include a regression case and note whether old reports remain valid.

## Fingerprint and capture contributions

- Capture only endpoints you own or are authorized to test.
- Do not commit the API key or any request/response header containing it.
- Record the endpoint, model request name, capture time, sample counts and validation result in a short note under `docs/`.
- Treat a dated baseline as a reference for that capture, not as proof of an official model identity.
- If a gateway returns a different dated version, do not merge it into an older baseline to make attribution look cleaner.

## Commit style

Use short conventional commits, for example:

```text
fix: keep cache read ratio when write is absent
feat: add dated deepseek fingerprint baseline
docs: add self-hosting security checklist
```

## Security

Do not report vulnerabilities in a public issue. Follow `SECURITY.md`.

By contributing, you agree that your code contribution is licensed under the repository's `LICENSE`. Third-party data and model names remain subject to their own terms; see `THIRD_PARTY_NOTICES.md`.
