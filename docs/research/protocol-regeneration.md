# CDP protocol regeneration: options and drift

- Date: 2026-09-05
- Issue: [#8](https://github.com/headlesslab/wand/issues/8) (research ticket under the wayfinder map [#1](https://github.com/headlesslab/wand/issues/1))
- Status: facts and options only. No decision is made here.

## Question (verbatim from #8)

> Upstream generates `lib/proto` by launching a local Chromium and reading `/json/protocol` (`lib/proto/generate`), last done 2024-07-01 against Chromium r1321438. What are the options for bringing the protocol layer current, and what has actually changed?
>
> Options to compare: (a) the same live-browser approach against current Chrome stable; (b) generating from ChromeDevTools/devtools-protocol's `browser_protocol.json` + `js_protocol.json` pinned by tag or commit; (c) anything else in use by comparable projects (chromedp's cdproto, playwright). For each: reproducibility, pinning, CI needs, diffability.
>
> Then the drift: which domains, methods, events, and types were removed, renamed, or deprecated between the r1321438 schema and current Chrome stable's, and which of those rod's core (browser / page / element / hijack / input / lib/js) actually calls. Note how usesigil/rod's 2026-07-10 "regenerate latest protocol from tip" commit handled it.

## 1. Baseline: how upstream generates `lib/proto` today

Source: `go-rod/rod@main` (code frozen 2024-12-07), `lib/proto/generate/{main.go,schema.go,patch.go,utils.go}`.

- **Schema acquisition** (`utils.go`, `getSchema`): `launcher.New().Bin(launcher.NewBrowser().MustGet())` downloads the Chromium snapshot for `launcher.RevisionDefault` (= `1321438`, `lib/launcher/revision.go`, committed 2024-07-01) from `HostGoogle` / `HostNPM` / `HostPlaywright` (all `chromium-browser-snapshots` mirrors), launches it, rewrites the DevTools WebSocket URL to `http://.../json/protocol`, reads the JSON and writes a pretty-printed copy to `tmp/proto.json` (`tmp/` is git-ignored, so the schema snapshot is never committed).
  - https://github.com/go-rod/rod/blob/main/lib/proto/generate/utils.go
  - https://github.com/go-rod/rod/blob/main/lib/launcher/revision.go
  - https://github.com/go-rod/rod/blob/main/lib/launcher/browser.go
- **Schema patching** (`patch.go`): seven hand edits are applied before code generation: enum for `Target.TargetInfo.type`; enum for `Page.lifecycleEvent.name`; skip `Input.TimeSinceEpoch`, `Network.TimeSinceEpoch`, `Network.MonotonicTime` (replaced by hand-written types in `a_patch.go`); retype `Network.Cookie.expires` to `TimeSinceEpoch`; make `Input.dispatchMouseEvent.deltaX/deltaY` required; make `Fetch.fulfillRequest.body` required.
  - https://github.com/go-rod/rod/blob/main/lib/proto/generate/patch.go
- **Output** (`main.go`): `cleanup()` deletes every `lib/proto/*.go` not prefixed `a_`, then writes one file per domain (`lib/proto/<snake_case_domain>.go`, 52 files at r1321438), `lib/proto/definitions.go` (`const Version = "v1.3"` and a `types` map of 1339 generated struct names to `reflect.Type`) and `lib/proto/definitions_test.go`. Hand-written code lives in `a_interface.go`, `a_patch.go`, `a_utils.go` (+ tests) and survives regeneration. Post-processing shells out to `gofumpt -w`, `go run golang.org/x/tools/cmd/goimports@latest -w` and `go run github.com/ysmood/golangci-lint@latest -- run --fix`; `gofumpt` is installed by `go run ./lib/utils/setup`.
  - https://github.com/go-rod/rod/blob/main/lib/proto/generate/main.go
  - https://github.com/go-rod/rod/blob/main/lib/utils/setup/main.go
- **Entry point**: `browser.go` carries `//go:generate go run ./lib/utils/setup`, `//go:generate go run ./lib/proto/generate`, `//go:generate go run ./lib/js/generate`, `//go:generate go run ./lib/assets/generate`, `//go:generate go run ./lib/utils/lint`.
  - https://github.com/go-rod/rod/blob/main/browser.go
- **CI**: `test-linux.yml` runs `go generate` on every push and on a daily cron, so the pinned browser is downloaded, launched and the protocol regenerated on every CI run, but nothing is committed. `check-revision.yml` runs monthly (`go run ./lib/launcher/revision` then `go generate`) and also has no commit or PR step, which is why it has produced no commit since 2024-07.
  - https://github.com/go-rod/rod/blob/main/.github/workflows/test-linux.yml
  - https://github.com/go-rod/rod/blob/main/.github/workflows/check-revision.yml
- **Regeneration history** (`lib/proto` commit log): 2023-03-01, 2023-04-01, 2023-05-01, 2023-10-01, 2024-04-01, 2024-05-05 "update chromium revision/version", and 2024-07-01 `5098fbe0` "update revision" (the last regeneration). The only later `lib/proto` commit, 2024-12-07 `393ac0d6`, edits hand-written `a_patch.go` (cookie `expires`), not generated code.
  - https://github.com/go-rod/rod/commits/main/lib/proto
- **What r1321438 is**: Chromium main branch positions are 1313161 for M127 and 1331488 for M128 (chromiumdash), so r1321438 is a tip-of-tree build between the M127 and M128 branch points (mid-2024 Chromium 128 canary).
  - https://chromiumdash.appspot.com/fetch_milestones?mstone=127
  - https://chromiumdash.appspot.com/fetch_milestones?mstone=128
- **Reference schema used below**: the `ChromeDevTools/devtools-protocol` rolls that bracket r1321438 are `98a6075f` "Roll protocol to r1319565" (2024-06-26) and `f9caf879` "Roll protocol to r1323165" (2024-07-04). Their merged domain/command/event/type sets are identical (52 domains, 1330 items), and all 1339 struct keys in upstream `lib/proto/definitions.go` exist in that set (0 missing). The r1323165 JSON is therefore used as "the r1321438 schema" in section 3.
  - https://github.com/ChromeDevTools/devtools-protocol/commit/98a6075f
  - https://github.com/ChromeDevTools/devtools-protocol/commit/f9caf879

## 2. What "current Chrome stable" is on 2026-09-05

| Channel / source | Version | Milestone | Chromium main branch position | Source |
|---|---|---|---|---|
| Stable, Windows (chromiumdash, released ~2026-09-03) | 153.0.8010.27 | 153 | 1681091 | https://chromiumdash.appspot.com/fetch_releases?channel=Stable&platform=Windows&num=2 |
| Stable, Linux (chromiumdash) | 152.0.7977.82 | 152 | 1669021 | https://chromiumdash.appspot.com/fetch_releases?channel=Stable&platform=Linux&num=2 |
| Chrome for Testing last-known-good Stable (2026-09-04T22:19Z) | 152.0.7977.82 (r1669021) | 152 | 1669021 | https://googlechromelabs.github.io/chrome-for-testing/last-known-good-versions.json |
| CfT Beta / Dev / Canary | 154.0.8037.0 (r1689415) / 155.0.8040.2 (r1691159) / 155.0.8043.0 (r1692319) | 154 / 155 / 155 | | same |
| `chromium-browser-snapshots` `LAST_CHANGE` (what `lib/launcher/revision` tracks) | Win_x64 1692911, Linux_x64 1692927 | tip | | https://storage.googleapis.com/chromium-browser-snapshots/Win_x64/LAST_CHANGE |

M153 is in the middle of its stable rollout (Windows already, Linux and CfT still M152). The devtools-protocol rolls closest to the two branch points are:

| Milestone | Branch position | Nearest roll | Commit | Tag |
|---|---|---|---|---|
| M152 | 1669021 | r1669207 (2026-07-28) | `6fe72ec7` | `v0.0.1669207` |
| M153 | 1681091 | r1681094 (2026-08-18) | `0539a3c0` | `v0.0.1681094` |
| tip (2026-09-04) | | r1692173 | `90778954` | `v0.0.1692173` |

Source: https://github.com/ChromeDevTools/devtools-protocol/commits/master

## 3. Options for regenerating (facts, no decision)

### Summary table

| | (a) Live browser (upstream approach) against Chrome stable | (b) `ChromeDevTools/devtools-protocol` JSON pinned by tag/commit | (c1) chromedp `cdproto-gen` / `cdproto` | (c2) Playwright |
|---|---|---|---|---|
| Source of truth | `/json/protocol` served by the launched binary | `json/browser_protocol.json` + `json/js_protocol.json`, generated from Chromium main PDL by a bot | `browser_protocol.pdl` from `chromium.googlesource.com` at a Chromium git ref + `js_protocol.pdl` from V8 at the ref named in Chromium `DEPS` | `/json/protocol` of Playwright's own Chromium build |
| Pin | Browser revision/version (`RevisionDefault` today) | Commit SHA, tag `v0.0.<chromium-rev>`, or npm `devtools-protocol@0.0.<rev>` | Chromium version tag + V8 tag (e.g. `153.0.7991.2_15.3.31`) recorded in the commit message | `browsers.json` `revision` + `browserVersion` (e.g. r1244 = 154.0.8037.0) |
| Mapping to a Chrome release | Direct if the launched binary is a stable build (CfT or installed Chrome); snapshots are tip-of-tree, not stable | Indirect: roll at/after `chromium_main_branch_position` of the milestone (chromiumdash); no per-milestone tag; release-branch cherry-picks are not represented | Direct: any Chromium version tag can be given as the ref | Indirect: Playwright's builds are CfT-based dev/beta snapshots |
| Reproducible | Yes while the binary stays downloadable (snapshot buckets and CfT keep old builds) | Yes (git content) | Yes (git content of Chromium/V8) | Yes (Playwright CDN) |
| CI needs | Download (~150-200 MB) + launch a browser; upstream CI already does this on every run | `curl` two files (~1.6 MB); no browser | Fetch two PDL files from googlesource; Go toolchain; no browser | Browser download + launch |
| Diffability | Only the generated Go diff; the schema JSON is not committed (`tmp/`) | `changelog.md` per roll, `git diff` of JSON/PDL between tags, protocol viewer site | Generated Go diff per "Updating to" commit; combined PDL dumped next to output | `protocol.d.ts` diff in each roll commit |
| Known gotchas | Needs a launcher path for stable builds (current launcher only knows snapshot revisions) | JSON lowers `binary` to `string` (`--map_binary_to_string=true`); two files must be merged | Generator repo `main` last changed 2020-07 | TS-only output; not a Go project |

### (a) Live browser against current Chrome stable

- The upstream launcher downloads only from `chromium-browser-snapshots` mirrors (`HostGoogle`, `HostNPM`, `HostPlaywright`) by revision number; these are per-commit tip-of-tree builds, not stable-channel builds. `lib/launcher/revision` chooses the largest revision present for every OS directory on the npmmirror listing, again unrelated to any release channel.
  - https://github.com/go-rod/rod/blob/main/lib/launcher/browser.go
  - https://github.com/go-rod/rod/blob/main/lib/launcher/revision/main.go
- A stable-channel binary is available as Chrome for Testing (`last-known-good-versions.json`, `known-good-versions-with-downloads.json`; each entry carries `version` and `revision`) or as a locally installed Chrome, which the generator could use via `launcher.New().Bin(path)`.
  - https://googlechromelabs.github.io/chrome-for-testing/
- What is pinned is a single number (browser revision or CfT version) that ties browser and protocol together; the schema itself is not committed, so a schema diff between two generations requires regenerating both.
- The live JSON keeps `binary` as a distinct type (Playwright's generator handles `binary` explicitly; usesigil's comment in `utils.go` states the same), so no base64 workaround is needed.
- Playwright uses exactly this approach (see c2).

### (b) devtools-protocol JSON pinned by tag or commit

- Repository: https://github.com/ChromeDevTools/devtools-protocol (default branch `master`, 1541 stars, BSD-3). Files: `json/browser_protocol.json` (1,398,282 bytes at r1692173), `json/js_protocol.json` (181,783 bytes), `pdl/browser_protocol.pdl`, `pdl/js_protocol.pdl`, `types/*.d.ts`, `changelog.md` (1,632,569 bytes).
- Production (`scripts/update-to-latest.sh`): reads the newest Chromium `main` commit and its `Cr-Commit-Position`, sparse-checks-out `third_party/blink/public/devtools_protocol/` and `DEPS` from Chromium, reads `v8_revision` from `DEPS`, sparse-checks-out `include/js_protocol.pdl` from V8, converts PDL to JSON with `scripts/inspector_protocol/convert_protocol_to_json.py --map_binary_to_string=true`, commits "Roll protocol to r<rev>", regenerates `changelog.md`, sets `package.json` version `0.0.<rev>`, tags `v0.0.<rev>`, pushes. Only runs to completion when there is a diff.
  - https://github.com/ChromeDevTools/devtools-protocol/blob/master/scripts/update-to-latest.sh
- Schedule: `.github/workflows/update.yml` cron `20 4 * * *` (daily 04:20 UTC) plus `workflow_dispatch`; `publish-on-tag.yml` publishes to npm with provenance on every tag. Observed frequency: 15 roll commits in August 2026 (2026-08-01..08-31).
  - https://github.com/ChromeDevTools/devtools-protocol/blob/master/.github/workflows/update.yml
  - https://github.com/ChromeDevTools/devtools-protocol/blob/master/.github/workflows/publish-on-tag.yml
- Versioning: one tag per roll, `v0.0.<chromium-rev>` (e.g. `v0.0.1692173`); historical milestone-style tags `v0.1`, `v0.2`, `v0.8`, `v1.0` exist but are not maintained. `package.json` `version` = `0.0.1692173` at HEAD.
  - https://github.com/ChromeDevTools/devtools-protocol/tags
  - https://www.npmjs.com/package/devtools-protocol
- Correspondence to Chrome versions: rolls follow Chromium `main`, so a milestone maps to the roll whose revision is at or just above the milestone's `chromium_main_branch_position` (see table in section 2). Protocol edits cherry-picked to a release branch after the branch point are not visible in this repo.
- Merging: the two JSON files each carry their own `domains` array; the browser's `/json/protocol` is the concatenation (usesigil merges them exactly this way).
- `binary` handling: the JSON is produced with `--map_binary_to_string=true`, so PDL `binary` becomes JSON `"type": "string"`; only fields that have a description carry the marker text "Encoded as a base64 string when passed over JSON". usesigil's generator restores `[]byte` from that marker and from an explicit list of 21 marker-less fields. The `pdl/` files in the same repo keep `binary`, so consuming the PDL instead of the JSON avoids this (cdproto-gen parses PDL).
- Changelog/diffability: `changelog.md` is regenerated on every roll (`scripts/generate-changelog.mjs`), and the same data drives https://chromedevtools.github.io/devtools-protocol/.

### (c1) chromedp: `cdproto-gen` + `cdproto`

- Generator: https://github.com/chromedp/cdproto-gen. `util/util.go` constants: `ChromiumBase = https://chromium.googlesource.com/chromium/src`, `ChromiumURL = <base>/+/%s/third_party/blink/public/devtools_protocol/browser_protocol.pdl`, `ChromiumDeps = <base>/+/%s/DEPS`, `V8Base = https://chromium.googlesource.com/v8/v8`, `V8URL = <base>/+/%s/include/js_protocol.pdl`. `main.go` flags: `-chromium` (Chromium protocol version/ref), `-v8` (V8 ref, otherwise read from Chromium `DEPS`), `-latest`, `-pdl` (local file), `-cache`, `-ttl` (24h). The combined PDL is written as `<chromium>_<v8>.pdl` into the cache and dumped into the output directory. The `main` branch's last commit is `99c9ca13` 2020-07-09 (repo `pushed_at` 2026-07-04 refers to other branches: `old`, `wip2`, a dependabot branch).
  - https://github.com/chromedp/cdproto-gen/blob/main/util/util.go
  - https://github.com/chromedp/cdproto-gen/blob/main/main.go
- Generated package: https://github.com/chromedp/cdproto. Commits are titled "Updating to <chromium-version>_<v8-version> definitions" and authored manually by kenshaw; eight in the last 12 months: 2026-03-21 (148.0.7744.1_14.8.86), 03-28 (148.0.7760.1_14.8.148), 04-05 (148.0.7773.1_14.8.178), 04-27 (149.0.7811.5_14.9.192), 07-04 (152.0.7930.1_15.2.11), 07-14 (152.0.7948.1_15.2.63), 07-19 (152.0.7960.1_15.2.100), 08-04 (153.0.7991.2_15.3.31; 8 files, +148/-268). The versions used are early-branch (dev-channel-like) tags, not stable ones. `cdproto`'s only workflow is `build.yml` (`go build ./...`); it has no tags.
  - https://github.com/chromedp/cdproto/commits/master
  - https://github.com/chromedp/cdproto/blob/master/.github/workflows/build.yml
- The generator skips deprecated domains/commands and applies "fixups" (spelling, redirects); the README lists the skip log.
  - https://github.com/chromedp/cdproto-gen/blob/main/README.md

### (c2) Playwright

- `utils/roll_browser.js <browser> <revision> <version>` updates `packages/playwright-core/browsers.json`, downloads Playwright's build, checks the downloaded version string against `<version>`, then calls `protocolGenerator.generateProtocol(browserName, executablePath)`.
  - https://github.com/microsoft/playwright/blob/main/utils/roll_browser.js
- `utils/protocol-types-generator/index.js` (`generateChromiumProtocol`) launches the binary with `--remote-debugging-port=9339`, opens `http://localhost:9339/json/protocol` in a page, parses the JSON, and writes TypeScript to `packages/playwright-core/src/server/chromium/protocol.d.ts`. It does not use the devtools-protocol repo.
  - https://github.com/microsoft/playwright/blob/main/utils/protocol-types-generator/index.js
- Pin: `browsers.json` (`chromium` revision `1244`, `browserVersion` `154.0.8037.0`, title "Chrome for Testing"; same for `chromium-headless-shell`). Each roll lands as one commit, e.g. `1d6fc5f0` "feat(chromium): roll to r1244 (#42545)" 2026-09-04, changing `browsers.json` (+4/-4), `protocol.d.ts` (+142/-341) and the mirrored `types/protocol.d.ts`.
  - https://github.com/microsoft/playwright/blob/main/packages/playwright-core/browsers.json
  - https://github.com/microsoft/playwright/commit/1d6fc5f0

## 4. Protocol drift: r1321438 schema vs current Chrome stable

Method: a throwaway Go tool (scratch only, not committed) loaded the merged `browser_protocol.json` + `js_protocol.json` for r1323165 (= r1321438 entity set, see section 1), r1669207 (M152), r1681094 (M153) and r1692173 (tip), diffed domains / commands / events / types and their parameters, and mapped every `proto.*` symbol referenced by rod's root package and `lib/*` back to its CDP entity using a port of the generator's `symbol()` naming function.

### 4.1 Totals

| | M152 (r1669207) | M153 (r1681094) | tip (r1692173) |
|---|---|---|---|
| Domains | 52 -> 58 | 52 -> 58 | 52 -> 58 |
| Items (commands+events+types) | 1330 -> 1510 | 1330 -> 1504 | 1330 -> 1497 |
| Removed commands / events / types | 13 / 11 / 40 | 19 / 12 / 44 | 26 / 13 / 49 |
| Newly deprecated commands / events | 5 / 1 | 6 / 1 | 6 / 1 |
| Added commands / events / types | 83 / 52 / 109 | 88 / 52 / 109 | 92 / 51 / 112 |
| Protocol `version` | 1.3 | 1.3 | 1.3 |

Domain level (identical for all three targets): removed `Database` (experimental); added `Ads`, `BluetoothEmulation`, `CrashReportContext`, `DigitalCredentials`, `FileSystem`, `SmartCardEmulation`, `WebMCP` (all experimental). The generator would delete `lib/proto/database.go` and create `ads.go`, `bluetooth_emulation.go`, `crash_report_context.go`, `digital_credentials.go`, `file_system.go`, `smart_card_emulation.go`, `web_mcp.go`, which is exactly the file set added/removed by usesigil's commit (section 5).

### 4.2 Drift that touches rod core

Scope of "rod core": `proto.*` identifiers in upstream `browser.go`, `page.go`, `element.go`, `hijack.go`, `input.go`, `query.go`, `page_eval.go`, `states.go`, `must.go`, `context.go`, `dev_helpers.go` (170 unique symbols; 11 of them are hand-written helpers in `lib/proto/a_*.go` such as `Client`, `Event`, `Request`, `Sessionable`, `PatternToReg`, `CookiesToParams`) plus `lib/*` outside `lib/proto` (19 symbols, all in `lib/devices` and `lib/input`). `lib/js` contains only injected JavaScript (`helper.js`) and references no CDP entity.

Result against M153 (identical against M152; tip adds one more optional parameter, noted below):

- **Removed or renamed entities used by core: 0.** Every command, event, type and enum constant that rod core references still exists under the same CDP name, and every enum constant value used by core is still in its enum.
- **Entities used by core whose shape changed: 19 root-package symbols + 1 `lib/*` symbol** (parameter/property level):

| rod symbol | CDP entity | change r1321438 -> M153 | effect on generated Go |
|---|---|---|---|
| `proto.NetworkCookie` | type `Network.Cookie` | property `sameParty` removed | field `SameParty` disappears; rod does not read it (`CookiesToParams` copies Name, Value, Domain, Path, Secure, HTTPOnly, SameSite, Expires, Priority) |
| `proto.NetworkCookieParam` | type `Network.CookieParam` | property `sameParty` removed | field `SameParty` disappears; not set by rod |
| `proto.NetworkSetBlockedURLs` | command `Network.setBlockedURLs` | param `urls` newly deprecated and now optional; `urlPatterns` (array of `BlockPattern`) added | `Urls []string` gains `omitempty`; `page.go:167` `proto.NetworkSetBlockedURLs{Urls: urls}` still compiles |
| `proto.EmulationSetDeviceMetricsOverride` (also `lib/devices`) | command `Emulation.setDeviceMetricsOverride` | param `displayFeature` newly deprecated; optional `scrollbarType`, `screenOrientationLockEmulation` added (tip also adds optional `viewportMeta`) | `lib/devices/device.go` `MetricsEmulation()` does not set `DisplayFeature` |
| `proto.PageJavascriptDialogOpening` | event `Page.javascriptDialogOpening` | param `frameId` added (required) | additive for an event consumer |
| `proto.PageJavascriptDialogClosed` | event `Page.javascriptDialogClosed` | param `frameId` added (required) | additive |
| `proto.DOMGetOuterHTML` | command `DOM.getOuterHTML` | optional `includeShadowDOM` added | additive |
| `proto.DOMNode` | type `DOM.Node` | optional `isScrollable`, `affectedByStartingStyles`, `adoptedStyleSheets`, `adProvenance` added | additive |
| `proto.EmulationSetGeolocationOverride` | command `Emulation.setGeolocationOverride` | optional `altitude`, `altitudeAccuracy`, `heading`, `speed` added | additive |
| `proto.NetworkEnable` | command `Network.enable` | optional `reportDirectSocketTraffic`, `enableDurableMessages` added | additive |
| `proto.NetworkRequestWillBeSent` | event `Network.requestWillBeSent` | optional `renderBlockingBehavior` added | additive |
| `proto.NetworkResourceType` | type `Network.ResourceType` | enum value `FedCM` added | additive |
| `proto.PageEnable` | command `Page.enable` | optional `enableFileChooserOpenedEvent` added | additive |
| `proto.PageNavigate` | command `Page.navigate` | optional return `isDownload` added | additive |
| `proto.PageSetInterceptFileChooserDialog` | command `Page.setInterceptFileChooserDialog` | optional `cancel` added | additive |
| `proto.RuntimeRemoteObject` | type `Runtime.RemoteObject` | `subtype` enum value `trustedtype` added | additive |
| `proto.TargetCreateTarget` | command `Target.createTarget` | optional `left`, `top`, `windowState`, `hidden`, `focus` added | additive |
| `proto.TargetTargetInfo` / `proto.TargetTargetInfoTypePage` | type `Target.TargetInfo` | optional `parentId`, `parentFrameId`, `embedderData` added | additive; the `type` enum is still supplied by `patch.go`, not by the schema |

- **Generator patch targets** (`patch.go`): `Target.TargetInfo.type`, `Page.lifecycleEvent.name`, `Input.TimeSinceEpoch`, `Network.TimeSinceEpoch`, `Network.MonotonicTime`, `Network.Cookie.expires`, `Input.dispatchMouseEvent.deltaX/deltaY`, `Fetch.fulfillRequest.body` all still exist in r1681094, so the patches still apply.
- **Hand-written `a_*.go` references** (`DOMGetContentQuadsResult`, `DOMQuad`, `DOMRect`, `FetchRequestPattern`, `InputDispatchKeyEvent`, `InputTouchPoint`, `NetworkCookie`, `NetworkCookieParam`, `NetworkDataReceived`, `PageEnable`, `PageLifecycleEventName*`, `TargetSessionID`, `TargetTargetInfoType*`) all still exist.
- Consistent with this, usesigil's regeneration against tip (section 5) changed only files under `lib/proto/` and required no edit to root-package code.

### 4.3 Drift that does not touch rod core

Removed between r1321438 and M153 (r1681094), grouped by domain; "added" entries are the entities now present in the same or another domain where the diff makes a replacement visible (facts from the two schemas, not a statement of intent):

- `Database` (whole domain, experimental): commands `disable`, `enable`, `executeSQL`, `getDatabaseTableNames`; event `addDatabase`; types `Database`, `DatabaseId`, `Error`.
- `Network` legacy request interception (removed in M153, still present in M152): commands `setRequestInterception` (deprecated), `continueInterceptedRequest` (deprecated), `getResponseBodyForInterception`, `takeResponseBodyForInterceptionAsStream`; event `requestIntercepted` (deprecated); types `InterceptionId`, `InterceptionStage`, `RequestPattern`. Also removed in M153: `setAcceptedEncodings`, `clearAcceptedEncodingsOverride`, type `ContentEncoding`; events `subresourceWebBundleInnerResponseError`, `subresourceWebBundleInnerResponseParsed`, `subresourceWebBundleMetadataError`, `subresourceWebBundleMetadataReceived`; type `PrivateNetworkRequestPolicy`. rod's `hijack.go` uses only the `Fetch` domain (`FetchEnable`, `FetchDisable`, `FetchRequestPaused`, `FetchContinueRequest`, `FetchFulfillRequest`, `FetchFailRequest`, `FetchAuthRequired`, `FetchContinueWithAuth`, `FetchRequestPattern`, `FetchHeaderEntry`) plus `Network` enums, none of which changed.
- `CSS`: type `StyleSheetId` removed from `CSS`; `DOM.StyleSheetId` now exists and `CSS` fields reference `DOM.StyleSheetId` (generated Go name changes from `CSSStyleSheetID` to `DOMStyleSheetID`). Types `CSSFontPaletteValuesRule`, `CSSPositionFallbackRule` (deprecated) removed.
- `Media`: event `playersCreated` removed; event `playerCreated` and type `Player` added.
- `Page`: command `getAdScriptId` and type `AdScriptId` removed; `Page.getAdScriptAncestry` and `Network.AdScriptIdentifier` / `Network.AdAncestry` added. Type `AutoResponseMode` removed.
- `Audits`: command `checkContrast`; types `LowTextContrastIssueDetails`, `AttributionReportingIssueDetails`, `AttributionReportingIssueType`.
- `ServiceWorker`: command `inspectWorker`.
- `Storage`: commands `getInterestGroupDetails`, `sendPendingAttributionReports`, `setAttributionReportingLocalTestingMode`, `setAttributionReportingTracking`, `setInterestGroupAuctionTracking`, `setInterestGroupTracking`; events `attributionReportingSourceRegistered`, `attributionReportingTriggerRegistered`, `interestGroupAccessed`, `interestGroupAuctionEventOccurred`, `interestGroupAuctionNetworkRequestCreated`; 19 `AttributionReporting*` types, `InterestGroupAccessType`, `InterestGroupAuctionEventType`, `InterestGroupAuctionFetchType`, `InterestGroupAuctionId`, `SignedInt64AsBase10`, `UnsignedInt128AsBase16`, `UnsignedInt64AsBase10`; type `SharedStorageAccessType` removed while `SharedStorageAccessMethod` and `SharedStorageAccessScope` were added. At tip (r1692173) but not yet in M153, the shared-storage entry commands are also gone with no replacement in any domain: `clearSharedStorageEntries`, `deleteSharedStorageEntry`, `getSharedStorageEntries`, `getSharedStorageMetadata`, `resetSharedStorageBudget`, `setSharedStorageEntry`, `setSharedStorageTracking`, event `sharedStorageAccessed`, types `SharedStorageAccessParams`, `SharedStorageEntry`, `SharedStorageMetadata`, `SharedStorageReportingMetadata`, `SharedStorageUrlWithMetadata`.
- `SystemInfo`: type `ImageDecodeAcceleratorCapability`.

Newly deprecated (flag `deprecated: true` added) between r1321438 and M153: commands `Browser.grantPermissions`, `CSS.setContainerQueryText`, `Debugger.setScriptSource` (M153 and tip only, not M152), `Network.emulateNetworkConditions` (`emulateNetworkConditionsByRule` added), `Overlay.setShowWebVitals`, `Storage.getStorageKeyForFrame` (`getStorageKey` added); event `Debugger.breakpointResolved`. None is referenced by rod core.

Added items per domain (M153): Ads 3, Audits 22, BluetoothEmulation 27, Browser 1, CSS 20, CrashReportContext 2, DOM 9, Debugger 2, DigitalCredentials 2, Emulation 23, Extensions 9, FileSystem 4, Inspector 1, Media 2, Memory 2, Network 47, Overlay 7, Page 7, Preload 1, SmartCardEmulation 35, Storage 5, Target 3, Tracing 1, WebAuthn 2, WebMCP 12.

Per-roll detail for any of the above is in https://github.com/ChromeDevTools/devtools-protocol/blob/master/changelog.md.

## 5. How usesigil/rod handled it (commit `18e1ba00`, 2026-07-10)

"Update protocol generator and regenerate latest protocol from tip" - https://github.com/usesigil/rod/commit/18e1ba00

- **Scope**: 65 files, +12,034 / -5,882, all under `lib/proto/`. Added `ads.go`, `bluetooth_emulation.go`, `crash_report_context.go`, `digital_credentials.go`, `file_system.go`, `smart_card_emulation.go`, `web_mcp.go`; removed `database.go`; `definitions.go` +1,535 / -1,340. No root-package file was touched, and the only `a_*.go` change is a one-line formatting edit in `a_patch.go`.
- **Generator change** (`lib/proto/generate/utils.go`): `getSchema` no longer launches a browser. It downloads `https://raw.githubusercontent.com/ChromeDevTools/devtools-protocol/master/json/browser_protocol.json` and `.../js_protocol.json`, concatenates the two `domains` arrays, still writes `tmp/proto.json`, and panics on a non-200 status. The URLs point at `master`, so the input is not pinned; the commit does not record which roll (`r<rev>`) was consumed.
- **`binary` restoration**: in `typeName()`, a `"type": "string"` whose description contains "Encoded as a base64 string when passed over JSON" becomes `[]byte`; in `patch.go`, a hard-coded list of 21 marker-less fields (BluetoothEmulation, Network `directTCPSocketChunk*`, `DirectUDPMessage.data`, `PostDataEntry.bytes`, `Page.getManifestIcons.primaryIcon`, SmartCardEmulation, `Storage.setProtectedAudienceKAnonymity.hashes`, WebAuthn `credentialId`s) is retyped to `binary` (including `items` for arrays). The comment attributes the need to the repo JSON lowering pdl `binary` to `string`, consistent with `--map_binary_to_string=true` in the upstream roll script.
- **Browser pin untouched**: `lib/launcher/revision.go` in usesigil/rod still says `RevisionDefault = 1321438`, so their protocol layer (2026-07 tip) and default browser (2024-06 snapshot) are now generated from different Chromium revisions.
- **Follow-ups**: the next commits, `d29dffda` "Key lookup and typing context fixes" (`input.go`, `lib/input/keyboard.go`) and `9214ba2f` "Revert WaitNavigation changes" (`must.go`, `page.go`), do not mention the protocol and touch no `lib/proto` file.
  - https://github.com/usesigil/rod/commits/main

## 6. Sources

- go-rod/rod generator: https://github.com/go-rod/rod/blob/main/lib/proto/generate/main.go , https://github.com/go-rod/rod/blob/main/lib/proto/generate/utils.go , https://github.com/go-rod/rod/blob/main/lib/proto/generate/patch.go , https://github.com/go-rod/rod/blob/main/lib/proto/generate/schema.go
- go-rod/rod launcher and revision: https://github.com/go-rod/rod/blob/main/lib/launcher/revision.go , https://github.com/go-rod/rod/blob/main/lib/launcher/browser.go , https://github.com/go-rod/rod/blob/main/lib/launcher/revision/main.go
- go-rod/rod CI: https://github.com/go-rod/rod/blob/main/.github/workflows/test-linux.yml , https://github.com/go-rod/rod/blob/main/.github/workflows/check-revision.yml
- go-rod/rod history: https://github.com/go-rod/rod/commits/main/lib/proto
- ChromeDevTools/devtools-protocol: https://github.com/ChromeDevTools/devtools-protocol , https://github.com/ChromeDevTools/devtools-protocol/blob/master/scripts/update-to-latest.sh , https://github.com/ChromeDevTools/devtools-protocol/blob/master/.github/workflows/update.yml , https://github.com/ChromeDevTools/devtools-protocol/blob/master/.github/workflows/publish-on-tag.yml , https://github.com/ChromeDevTools/devtools-protocol/tags , https://github.com/ChromeDevTools/devtools-protocol/blob/master/changelog.md
- Snapshots used for the diff: https://github.com/ChromeDevTools/devtools-protocol/commit/98a6075f (r1319565), https://github.com/ChromeDevTools/devtools-protocol/commit/f9caf879 (r1323165), https://github.com/ChromeDevTools/devtools-protocol/commit/6fe72ec7 (r1669207), https://github.com/ChromeDevTools/devtools-protocol/commit/0539a3c0 (r1681094), https://github.com/ChromeDevTools/devtools-protocol/commit/90778954 (r1692173)
- Chrome release data: https://chromiumdash.appspot.com/fetch_releases?channel=Stable&platform=Windows&num=2 , https://chromiumdash.appspot.com/fetch_releases?channel=Stable&platform=Linux&num=2 , https://chromiumdash.appspot.com/fetch_milestones?mstone=127 , https://chromiumdash.appspot.com/fetch_milestones?mstone=128 , https://chromiumdash.appspot.com/fetch_milestones?mstone=152 , https://chromiumdash.appspot.com/fetch_milestones?mstone=153
- Chrome for Testing: https://googlechromelabs.github.io/chrome-for-testing/last-known-good-versions.json
- Chromium snapshots: https://storage.googleapis.com/chromium-browser-snapshots/Win_x64/LAST_CHANGE , https://storage.googleapis.com/chromium-browser-snapshots/Linux_x64/LAST_CHANGE
- chromedp: https://github.com/chromedp/cdproto-gen/blob/main/util/util.go , https://github.com/chromedp/cdproto-gen/blob/main/main.go , https://github.com/chromedp/cdproto-gen/blob/main/README.md , https://github.com/chromedp/cdproto/commits/master , https://github.com/chromedp/cdproto/commit/e85f50db
- Playwright: https://github.com/microsoft/playwright/blob/main/utils/roll_browser.js , https://github.com/microsoft/playwright/blob/main/utils/protocol-types-generator/index.js , https://github.com/microsoft/playwright/blob/main/packages/playwright-core/browsers.json , https://github.com/microsoft/playwright/commit/1d6fc5f0
- usesigil/rod: https://github.com/usesigil/rod/commit/18e1ba00 , https://github.com/usesigil/rod/blob/main/lib/launcher/revision.go , https://github.com/usesigil/rod/commits/main
- Background: https://github.com/headlesslab/wand/issues/2 (upstream state survey)
