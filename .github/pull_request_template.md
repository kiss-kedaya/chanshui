## Summary

What changed and why?

## Verification

- [ ] `go test -race ./...`
- [ ] `go vet ./...`
- [ ] `cd web && npm test`
- [ ] `cd web && npm run typecheck`
- [ ] `cd web && npm run build`
- [ ] UI checked at 320 / 360 / 390 / 430 px when frontend changed

## Security

- [ ] No API keys, cookies, job tokens, feedback contacts, database files or local `.env` files are included.
- [ ] New fingerprint/capture data includes provenance and does not contain credentials.
