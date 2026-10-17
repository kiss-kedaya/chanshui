# Third-Party Notices

This project contains code, model references and captured model outputs from several sources. The MIT license in `LICENSE` applies to this project's code; it does not relicense third-party data, model names or dependencies.

## ModelTrace numeric baselines

Some numeric candidate baselines are derived from the ModelTrace data referenced in `data_ref/provenance.json` (commit `d4131b30243dfa05e70180b5eedde742103f1d73` at the time of writing). Check the upstream repository's license and terms before redistributing or using the data commercially.

## DeepSeek samples

The dated `DeepSeek-V4.1-Flash-0910` and `DeepSeek-V4.1-Flash-1017` baselines were captured through `https://api.deepseek.com` with the request name `deepseek-flash`. Each baseline records 36 training samples and 12 independent validation samples. The dated names are capture labels, not an official version claim by the API. See `docs/deepseek-fingerprint-2026-10-05.md` and the capture files under `data_ref/`.

## Grok samples

The Grok 4.6/4.7 numeric baselines were captured through a third-party gateway, not an official xAI endpoint. They are reference samples for the model names returned by that gateway and are not independent proof of model identity. See `docs/grok-fingerprints-2026-10-05.md` and `data_ref/captures/`.

## Dependencies

Go dependencies are listed in `go.mod` / `go.sum`; frontend dependencies are listed in `web/package.json` / `web/package-lock.json`. Each dependency remains under its own license. Review those licenses before distributing a bundled release.

## Trademarks

Claude, GPT, OpenAI, DeepSeek, Grok, xAI, Gemini, GLM, Unity, Unreal and other names are trademarks of their respective owners. This project is not affiliated with or endorsed by those owners.

## No warranty

Model fingerprints, cache measurements, tool/web checks and scores are evidence, not guarantees of a provider's identity, quality or legality. See `LICENSE` and `SECURITY.md`.
