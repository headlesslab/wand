# Generate the protocol layer from devtools-protocol, pinned below the Target Chrome's branch point

Upstream regenerated `lib/proto` by downloading and launching a pinned Chromium snapshot and reading its `/json/protocol`, which needs a browser in CI, leaves no diffable schema behind, and had drifted to a mid-2024 tip-of-tree build. wand instead generates `lib/proto` from `ChromeDevTools/devtools-protocol`'s `json/browser_protocol.json` and `json/js_protocol.json` at the tag `v0.0.<rev>`, where `<rev>` is the largest roll not above the Target Chrome's branch position and the Target Chrome is whatever Chrome for Testing's Stable channel serves; the launcher's default browser is pinned to the same Target Chrome, and both move together, once per Chrome stable milestone, through an automated pull request. This is Puppeteer's mechanism (its `update_browser_revision` script and `ensure-correct-devtools-protocol-package` check), chosen because a roll below the branch point is guaranteed to be in that release while one above may carry unreleased changes, and because a 1.6 MB download reproduces the schema without a browser.

## Considered Options

- **Live browser `/json/protocol`** (upstream, Playwright): exact match to a binary and native `binary` types, but CI must download and launch a browser, the schema is never committed so two generations cannot be diffed, and the launcher would first need a Chrome for Testing download path.
- **devtools-protocol PDL** at the same tag: keeps `binary`, but upstream's generator parses JSON, so a PDL parser would be new code.
- **chromedp's `cdproto-gen`**: fetches PDL from `chromium.googlesource.com` at a Chrome version tag, so it pins an exact release, but needs a PDL parser plus V8 `DEPS` resolution, skips deprecated entities, and its generator has not changed since 2020.
- **Tip-of-tree JSON from `master`** (usesigil/rod): no pin, roll not recorded, and the protocol ran two years ahead of the pinned browser.

## Consequences

- The JSON lowers PDL `binary` to `string`; the generator restores `[]byte` from the "Encoded as a base64 string when passed over JSON" description marker plus a hand-kept list of marker-less fields, and fails when that count disagrees with the `binary` occurrences in the same tag's PDL files.
- Generated code mirrors the pinned schema exactly: experimental and deprecated entities are kept (deprecated ones carry Go's `// Deprecated:` marker), removed entities disappear without compatibility stubs, and each regeneration ships a symbol-level summary of removed, renamed and deprecated Go identifiers.
- Generated code is committed and the schema is not; CI regenerates from the pin and fails on any diff, and re-derives the roll from the Target Chrome's branch position, so the pin cannot silently drift.
- A Target Chrome exists only once Chrome for Testing's Stable channel serves it, so wand lags the first platform's stable rollout by days; older browsers get the same generated code on a best-effort basis.
