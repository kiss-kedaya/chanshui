# Open-source checklist

Use this before changing repository visibility. Once a repository is public, commits and release assets may be cached, forked or indexed even if it is made private again.

## Code and data

- [ ] `LICENSE` matches the intended license. This repository currently uses MIT; switch before publishing if you want AGPL-3.0 or another license.
- [ ] `README.md`, `SECURITY.md`, `CONTRIBUTING.md` and `THIRD_PARTY_NOTICES.md` are current.
- [ ] `git status` is clean and the release tag points at the intended commit.
- [ ] No `.env`, API key, database, feedback export or local `.local/` file is tracked.
- [ ] Fingerprint/capture data has provenance and no credentials. Review `THIRD_PARTY_NOTICES.md` and any upstream data terms.
- [ ] Decide whether internal plans under `docs/superpowers/`, `docs/local-verification-*` and old release history should stay public.

## Git history

The current tree is clean, but old commits may contain deployment details. Before publishing:

```bash
git log --all -S 'CHANSHUI_ADMIN_TOKEN=' --oneline
git log --all -S '<ORIGIN_IP>' --oneline
git log --all -S 'sk-' --oneline
```

`sk-` hits in this repository are test placeholders. The old deployment IP can still appear in history. If it must not be public, rewrite history with a backup or publish from a fresh repository; do not run `git filter-repo` without taking a full backup and confirming the new remote.

## GitHub settings

- [ ] Change visibility only after the history review.
- [ ] Enable Secret scanning and push protection.
- [ ] Enable Dependabot alerts and security updates.
- [ ] Enable Code Scanning / CodeQL. The workflow currently sets `upload: false`; set it to `true` after the repository is public and Code Scanning is available.
- [ ] Protect `main`: require the CI workflow and no force pushes.
- [ ] Set a repository description, homepage and topics.

## Release

- [ ] Tag a release from the reviewed commit.
- [ ] Confirm the release assets, `checksums.txt` and release notes.
- [ ] Confirm the Docker CI job and the normal Go/Next CI job pass.
- [ ] Do not put feedback exports or database files into release assets.

## Credentials

- [ ] Rotate the admin token if it was ever pasted into a shared channel or committed. The value is not part of this repository; only its location is documented.
- [ ] Review the deployment host and WAF/CDN configuration separately. Open-sourcing code does not imply publishing private infrastructure details.
