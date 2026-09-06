# Browser acquisition strategies for the launcher

- Date: 2026-09-05
- Ticket: headlesslab/wand#12 (research, child of #1; blocks #13)
- Author: research pass over primary sources only (upstream source, bucket listings, JSON indexes, HEAD requests). No browser binaries were downloaded; availability was checked with HEAD requests and index listings.

## Question (verbatim from #12)

> Upstream's launcher downloads Chromium snapshot r1321438 from Google's snapshot storage, with NPM and Playwright mirrors as fallbacks. Compare the strategies for the baseline release: (a) keep Chromium snapshots with a bumped revision; (b) Chrome for Testing (Google's versioned builds, pinned by Chrome version); (c) system-installed browser only, download optional; (d) Playwright's browser builds.
>
> For each: platform coverage (linux / mac / windows; amd64 / arm64), Alpine / musl viability (upstream #1114), download hosts and mirror availability for users behind restricted networks (mainland China included), version-pinning granularity, and what runZeroInc/go-rod's switch to Chrome for Testing looked like.
>
> Output: a comparison on a `research/` branch. Do not decide.

This document states facts and options. It does not recommend.

---

## 1. Current state: upstream `go-rod/rod` `lib/launcher` (main, code frozen 2024-12-07)

Source files: `browser.go`, `revision.go`, `revision/main.go`, `launcher.go`, `utils.go`, `lib/defaults/defaults.go`, `.github/workflows/check-revision.yml`, `lib/docker/Dockerfile`, `lib/utils/get-browser/main.go`.
Base URL for all of them: https://raw.githubusercontent.com/go-rod/rod/main/

### 1.1 Platform table and hosts (`lib/launcher/browser.go`)

`hostConf` is keyed by `runtime.GOOS + "_" + runtime.GOARCH` and has exactly five entries:

| GOOS_GOARCH     | bucket prefix | zip name           |
| --------------- | ------------- | ------------------ |
| `darwin_amd64`  | `Mac`         | `chrome-mac.zip`   |
| `darwin_arm64`  | `Mac_Arm`     | `chrome-mac.zip`   |
| `linux_amd64`   | `Linux_x64`   | `chrome-linux.zip` |
| `windows_386`   | `Win`         | `chrome-win.zip`   |
| `windows_amd64` | `Win_x64`     | `chrome-win.zip`   |

There is no `linux_arm64` and no `windows_arm64` entry; on those platforms `hostConf` is the zero value, so `HostGoogle`/`HostNPM` produce URLs with empty prefix and zip name.

Three `Host func(revision int) string` implementations, in the default order `[]Host{HostGoogle, HostNPM, HostPlaywright}`:

| Host             | URL template                                                                                                                                                                                                                                                      |
| ---------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `HostGoogle`     | `https://storage.googleapis.com/chromium-browser-snapshots/<prefix>/<revision>/<zip>`                                                                                                                                                                             |
| `HostNPM`        | `https://registry.npmmirror.com/-/binary/chromium-browser-snapshots/<prefix>/<revision>/<zip>`                                                                                                                                                                    |
| `HostPlaywright` | `https://playwright.azureedge.net/builds/chromium/<rev>/chromium-linux-arm64.zip`, where `rev = RevisionPlaywright` only when `GOOS=linux && GOARCH=arm64`; on every other platform `rev` is the Chromium revision, and the URL still names the `linux-arm64` zip |

`Browser.Download()` hands all candidate URLs to `fetchup.New(dir, urls...)`, which "will race downloading a TCP packet from each host and use the fastest host", then `fetchup.StripFirstDir(dir)`. On failure the error points to https://go-rod.github.io/#/compatibility?id=os.

Cache layout: `DefaultBrowserDir` is `%APPDATA%\rod\browser` on Windows and `$HOME/.cache/rod/browser` on darwin/linux; `Browser.Dir()` is `<RootDir>/chromium-<revision>`; `BinPath()` is `Chromium.app/Contents/MacOS/Chromium` (darwin), `chrome` (linux), `chrome.exe` (windows) under that dir. `Browser.Get()` holds `leakless.LockPort(LockPort)` (default `defaults.LockPort = 2978`; the field comment says 2968) and calls `Validate()`, which executes the binary with `--headless --no-sandbox --use-mock-keychain --disable-dev-shm-usage --disable-gpu --dump-dom about:blank` and treats an output containing `error while loading shared libraries` as a _valid_ binary (missing OS dependencies are not the downloader's problem).

### 1.2 Pinned revision (`lib/launcher/revision.go`, `revision/main.go`, `check-revision.yml`)

```go
const RevisionDefault = 1321438
const RevisionPlaywright = 1124
```

The generator (`go run ./lib/launcher/revision`) lists directories on `https://registry.npmmirror.com/-/binary/chromium-browser-snapshots/` (skipping `Win`), parses every revision directory name per platform, and picks the _largest revision present on all platforms_ (`largestCommonRevision`); it fails if the result is `< 969819`. `RevisionPlaywright` comes from `npm show playwright version` followed by `browsers.json` of that Playwright tag (`browsers.0.revision`). `check-revision.yml` runs this monthly (`cron: '0 0 1 * *'`) followed by `go generate`; per #2 it has produced no commit in two years.

What r1321438 is: the bucket's `Linux_x64/1321438/REVISIONS` file reports `got_revision_cp: refs/heads/main@{#1321438}` and `got_v8_revision_cp: refs/heads/12.8.168@{#1}`; the zips carry `Last-Modified: Sun, 30 Jun 2024`. chromiumdash gives M127's branch point as position 1313161 (branch 6533) and M128's as 1331488 (branch 6613), so r1321438 sits between them, i.e. a Chromium 128.0.65xx.0 canary-era trunk build. Chrome for Testing's known-good list for 128 runs from 128.0.6534.0 (r1313756) to 128.0.6613.137 (r1331488).
Sources: https://commondatastorage.googleapis.com/chromium-browser-snapshots/Linux_x64/1321438/REVISIONS · https://chromiumdash.appspot.com/fetch_milestones?mstone=127 · https://chromiumdash.appspot.com/fetch_milestones?mstone=128 · https://googlechromelabs.github.io/chrome-for-testing/known-good-versions-with-downloads.json

### 1.3 How the binary is chosen at launch (`launcher.go`, `defaults.go`)

- `launcher.New()` sets `flags.Bin = defaults.Bin`. `defaults.Bin` is filled only from the `-rod=bin=<path>` CLI flag (parsed from `os.Args` in `defaults.ResetWith`; disable with env `DISABLE_ROD_FLAG`). There is no environment variable for the binary path.
- If `Bin` is empty, `getBin()` calls `l.browser.Get()`, i.e. **download**. `launcher.New()` does _not_ call `LookPath()`; only `launcher.NewUserMode()` does (`bin, _ := LookPath()`).
- `Launcher.Bin(path)` documents: "if the path is not empty the auto download will be disabled". `Launcher.Revision(rev)` sets `browser.Revision`.
- Docs (custom-launch.md): "By default, the launcher will automatically download and use a statically versioned browser so that the browser behavior is consistent." and show `launcher.New().Bin(path)` / `launcher.LookPath()` as the opt-out.
- `inContainer` (`utils.InContainer`) adds `--no-sandbox` automatically.

Sources: https://raw.githubusercontent.com/go-rod/rod/main/lib/launcher/launcher.go · https://raw.githubusercontent.com/go-rod/rod/main/lib/defaults/defaults.go · https://raw.githubusercontent.com/go-rod/go-rod.github.io/main/custom-launch.md

### 1.4 `LookPath()` search list (system browsers)

| GOOS    | candidates (in order)                                                                                                                                                                                                                                                                                                                                         |
| ------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| darwin  | `/Applications/Google Chrome.app/…/Google Chrome`, `/Applications/Chromium.app/…/Chromium`, `/Applications/Microsoft Edge.app/…/Microsoft Edge`, `/Applications/Google Chrome Canary.app/…`, `/usr/bin/google-chrome-stable`, `/usr/bin/google-chrome`, `/usr/bin/chromium`, `/usr/bin/chromium-browser`                                                      |
| linux   | `chrome`, `google-chrome`, `/usr/bin/google-chrome`, `microsoft-edge`, `/usr/bin/microsoft-edge`, `chromium`, `chromium-browser`, `google-chrome-stable` (added 2024-11-25 for NixOS, #1136), `/usr/bin/google-chrome-stable`, `/usr/bin/chromium`, `/usr/bin/chromium-browser`, `/snap/bin/chromium`, `/data/data/com.termux/files/usr/bin/chromium-browser` |
| openbsd | `chrome`, `chromium`                                                                                                                                                                                                                                                                                                                                          |
| windows | `chrome`, `edge` on `PATH`, then `%ProgramFiles%`, `%ProgramFiles(x86)%`, `%LocalAppData%` × `Google\Chrome\Application\chrome.exe`, `Chromium\Application\chrome.exe`, `Microsoft\Edge\Application\msedge.exe`                                                                                                                                               |

Each candidate goes through `exec.LookPath`. No env var, no registry lookup, no `Chrome for Testing.app`, no Homebrew prefix, no `/opt/google/chrome`.
Source: https://raw.githubusercontent.com/go-rod/rod/main/lib/launcher/browser.go · https://github.com/go-rod/rod/commit/73907a8616

### 1.5 Docker image and docs on platforms

- `lib/docker/Dockerfile` builds on the `golang` image, runs `go run ./lib/utils/get-browser` (which is `launcher.NewBrowser().Get()`), copies `/root/.cache/rod` into `ubuntu:noble`, and installs glibc-era deps (`libnss3 libxss1 libasound2t64 libxtst6 libgtk-3-0 libgbm1 ca-certificates`, fonts, `tzdata`, `dumb-init`, `xvfb`). The image is glibc, not musl.
- compatibility.md: "You should be able to compile and run Rod seamlessly on all main platforms that Golang supports. … On some platforms, you might need to install the browser manually, Rod can't guarantee the auto-downloaded browser will always work." For Alpine it documents `apk add chromium`. "Each version of Rod only guarantees to work with its `launcher.DefaultRevision` of the browser."

Sources: https://raw.githubusercontent.com/go-rod/rod/main/lib/docker/Dockerfile · https://raw.githubusercontent.com/go-rod/rod/main/lib/utils/get-browser/main.go · https://raw.githubusercontent.com/go-rod/go-rod.github.io/main/compatibility.md

### 1.6 Upstream #1114 (Alpine)

"Chromium version in Alpine 3.20 is causing timeout issues" — open since 2024-09-19, 7 thumbs-up, 2 comments (a bot asking for the version and one "Any updates?"). The reporter launches the **system** Alpine Chromium via `launcher.New().Bin(browserPath).Set("no-sandbox")`; page creation hangs 10–30 % of the time; `--disable-gpu` helps. It cites Puppeteer's troubleshooting page, which says "Chrome does not support Alpine out of the box" and "The current Chromium version in Alpine 3.20 is causing timeout issues with Puppeteer. Downgrading to Alpine 3.19 fixes the issue." The issue is about the musl Chromium _package_, not about the downloader; none of the download strategies below apply to it directly.
Sources: https://github.com/go-rod/rod/issues/1114 · https://pptr.dev/troubleshooting

---

## 2. Comparison table

All facts as observed 2026-09-05. "Verified" means HEAD 200 or present in an index listing today.

| Dimension                            | (a) Chromium snapshots (bumped revision)                                                                                                  | (b) Chrome for Testing                                                                                                            | (c) System browser only, download optional                                                                       | (d) Playwright browser builds                                                                                                                         |
| ------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------- |
| linux/amd64                          | yes (`Linux_x64`)                                                                                                                         | yes (`linux64`)                                                                                                                   | whatever is installed                                                                                            | yes — now re-hosted CfT (`builds/cft/<ver>/linux64/`)                                                                                                 |
| linux/arm64                          | **no** (only stale `Linux_ARM_Cross-Compile`, LAST_CHANGE 270195, 32-bit)                                                                 | **Beta/Dev/Canary only**: `linux-arm64` since 153.0.8001.0; Stable 152 has none (HEAD 404)                                        | yes if distro ships one (Alpine, Debian, Ubuntu do)                                                              | yes — Playwright-built `builds/chromium/<pwrev>/chromium-linux-arm64.zip` (+ headless-shell), still published at r1243                                |
| darwin/amd64                         | yes (`Mac`)                                                                                                                               | yes (`mac-x64`)                                                                                                                   | yes                                                                                                              | yes (CfT re-host)                                                                                                                                     |
| darwin/arm64                         | yes (`Mac_Arm`)                                                                                                                           | yes (`mac-arm64`)                                                                                                                 | yes                                                                                                              | yes (CfT re-host)                                                                                                                                     |
| windows/amd64                        | yes (`Win_x64`)                                                                                                                           | yes (`win64`)                                                                                                                     | yes                                                                                                              | yes (CfT re-host, `win64` only)                                                                                                                       |
| windows/386                          | yes (`Win`)                                                                                                                               | yes (`win32`)                                                                                                                     | yes                                                                                                              | no                                                                                                                                                    |
| windows/arm64                        | yes (`Win_Arm64`, verified at r1321438 and current)                                                                                       | no (runZeroInc runs `win64` under emulation)                                                                                      | yes (native Chrome/Edge exist)                                                                                   | no                                                                                                                                                    |
| Alpine / musl                        | no — glibc builds                                                                                                                         | no — glibc builds                                                                                                                 | **only viable path**: `apk add chromium` (152.0.7977.64-r0 on edge, x86_64 + aarch64; 142.0.7444.59-r0 on v3.22) | no — glibc builds                                                                                                                                     |
| Primary host                         | `storage.googleapis.com` / `commondatastorage.googleapis.com` bucket `chromium-browser-snapshots`                                         | `storage.googleapis.com` bucket `chrome-for-testing-public`; index on `googlechromelabs.github.io`                                | n/a                                                                                                              | `cdn.playwright.dev` → Microsoft ESRP CDN (`playwright.download.prss.microsoft.com`); legacy `playwright.azureedge.net` still redirects               |
| Mirror reachable from mainland China | npmmirror mirrors `Linux_x64, Mac, Mac_Arm, Win, Win_x64` (not `Win_Arm64`, no `LAST_CHANGE`), ~1 day behind                              | npmmirror `chrome-for-testing/` mirrors all 2506 versions incl. `linux-arm64`, ~21 h behind                                       | n/a (OS package mirrors)                                                                                         | npmmirror `playwright/builds/` partial: `chromium/` up to r1243, `cft/` only 8 versions (latest 153.0.8010.12), `chromium-headless-shell/` only r1155 |
| Pinning granularity                  | integer trunk commit position (e.g. 1321438); **not** a Chrome version; per-platform sets differ                                          | full Chrome version `MAJOR.MINOR.BUILD.PATCH` (+ its `revision`), or channel, or milestone, or `MAJOR.MINOR.BUILD`                | none (whatever is installed); version drift is the user's                                                        | Playwright's own integer roll number (1124, 1202, 1244 …) which maps to one `browserVersion` per Playwright release                                   |
| Programmable metadata                | `LAST_CHANGE` text per platform; `REVISIONS` JSON per dir; bucket XML listing (`?delimiter=/&prefix=`); chromiumdash for version→position | 8 JSON endpoints (known-good / last-known-good / per-build / per-milestone, each ± downloads) + per-version JSON + text endpoints | n/a                                                                                                              | `browsers.json` on a git tag/branch only; no index of all revisions                                                                                   |
| Oldest available                     | bucket history to 2010s; upstream tool refuses `< 969819`                                                                                 | 113.0.5672.0 (r1121455); `chrome-headless-shell` since 120.0.6098.0                                                               | n/a                                                                                                              | whatever is still on the CDN; npmmirror `builds/chromium/` has 64 revisions                                                                           |
| Binary licence                       | Chromium, BSD-3-Clause                                                                                                                    | Google Chrome build → Google Chrome ToS; CfT tooling repo Apache-2.0; no separate CfT licence text published                      | depends on what is installed (Chromium BSD / Chrome ToS / Edge EULA)                                             | Playwright tooling Apache-2.0; binaries: CfT (Chrome) since 1.57 on x64/mac/win; Chromium (BSD) on linux-arm64                                        |
| Who else uses it                     | `@puppeteer/browsers` `chromium` browser type (maps `LINUX_ARM` to `Linux_x64`)                                                           | Puppeteer default since CfT launch; Playwright ≥ 1.57; runZeroInc/go-rod                                                          | chromedp (never downloads)                                                                                       | Playwright only                                                                                                                                       |

---

## 3. Strategy (a): Chromium snapshots with a bumped revision

**What it is.** Google's continuous-build archive. chromium.org: "Chromium builds are made available on a best-effort basis, and are built from arbitrary revisions that don't necessarily map to user-facing Chrome releases." The latest build per platform is "mentioned in the `LAST_CHANGE` file"; finding an older build for a given Chrome version requires resolving a branch base position first.
Source: https://www.chromium.org/getting-involved/download-chromium/

**Bucket layout (verified).** Top-level prefixes in `chromium-browser-snapshots` include `Linux_x64/`, `Mac/`, `Mac_Arm/`, `Win/`, `Win_x64/`, `Win_Arm64/`, `Linux/`, `Linux_ARM_Cross-Compile/`, `Android/`, `Android_Arm64/`, `AndroidDesktop_*`, `Arm/`, `Linux_ChromiumOS*/`, `lacros*/`, `*Git/`, `*_rel/`, `experimental/`, `tmp/`. A revision directory (e.g. `Linux_x64/1692927/`) contains `REVISIONS`, `chrome-linux.zip`, `chromedriver_linux64.zip`, `content-shell.zip`, `devtools-frontend.zip`; `Win_Arm64/<rev>/` contains `chrome-win.zip`, `chromedriver_win64.zip`, `mini_installer.exe`, etc.
Source: https://commondatastorage.googleapis.com/chromium-browser-snapshots/?delimiter=/&prefix=

**`LAST_CHANGE` on 2026-09-05.**

| prefix                    | LAST_CHANGE                |
| ------------------------- | -------------------------- |
| `Linux_x64`               | 1692927                    |
| `Mac`                     | 1692917                    |
| `Mac_Arm`                 | 1692927                    |
| `Win`                     | 1692910                    |
| `Win_x64`                 | 1692911                    |
| `Win_Arm64`               | 1692910                    |
| `Linux` (32-bit)          | 382086 (stale)             |
| `Linux_ARM_Cross-Compile` | 270195 (stale, 32-bit ARM) |

Because the per-platform heads differ, a revision that exists for one platform may be missing for another; this is why upstream's generator computes the largest _common_ revision. There is **no Linux arm64 directory** in this bucket. `Win_Arm64` exists and had r1321438 (`chrome-win.zip`), so Windows-on-ARM could be added to the current scheme with one more `hostConf` row.
Source: `https://commondatastorage.googleapis.com/chromium-browser-snapshots/<prefix>/LAST_CHANGE`

**r1321438 availability (HEAD, Google).** `Linux_x64/chrome-linux.zip` 190,157,449 B; `Mac/chrome-mac.zip` 149,328,540 B; `Mac_Arm/chrome-mac.zip` 135,605,548 B; `Win/chrome-win.zip` 244,547,786 B; `Win_x64/chrome-win.zip` 267,483,258 B — all 200, `Last-Modified: 30 Jun 2024`. `Linux/1321438/chrome-linux.zip` is 404.

**Mirror: npmmirror (`registry.npmmirror.com/-/binary/chromium-browser-snapshots/`).** JSON directory listing API. Top level has exactly `Linux_x64/, Mac/, Mac_Arm/, Win/, Win_x64/` — no `Win_Arm64`, and `LAST_CHANGE` files are not mirrored (`[NOT_FOUND]`). `Linux_x64/` lists 132,388 revision directories; the newest is 1692111 (synced 2026-09-04T02:03Z), versus Google's 1692927, i.e. roughly one day / ~800 positions behind. r1321438 is present for all five platforms and redirects (302) to `https://cdn.npmmirror.com/binaries/chromium-browser-snapshots/<prefix>/1321438/<zip>` with byte sizes identical to Google's. Upstream switched to this mirror in commit e0d0a7eea5 ("use registry.npmmirror.com to download the chromium", 2022-02-08).
Sources: https://registry.npmmirror.com/-/binary/chromium-browser-snapshots/ · https://registry.npmmirror.com/-/binary/chromium-browser-snapshots/Linux_x64/ · https://github.com/go-rod/rod/commit/e0d0a7eea5

**Version pinning.** The unit is a trunk commit position. Mapping to a Chrome version requires chromiumdash (`fetch_releases?channel=Stable&platform=Linux&num=1` → `{"version":"152.0.7977.82","chromium_main_branch_position":1669021}`; `fetch_milestones?mstone=N` → branch point), and a position that is a _branch point_ is not guaranteed to have a snapshot on every platform (snapshots are built from arbitrary trunk revisions, not release tags). Snapshot revisions therefore pin "a trunk build near Chrome N canary", never a shipped Chrome version.
Sources: https://chromiumdash.appspot.com/fetch_releases?channel=Stable&platform=Linux&num=1 · https://chromiumdash.appspot.com/fetch_milestones?mstone=128

**Alpine / musl.** The zips are glibc builds (upstream's Docker image installs Ubuntu deps around them; `Validate()` explicitly tolerates "error while loading shared libraries"). Not usable on musl without a compatibility layer; not what #1114 is about.

**Licence.** Chromium source and binaries are BSD-3-Clause ("Redistribution and use in source and binary forms, with or without modification, are permitted…").
Source: https://chromium.googlesource.com/chromium/src/+/main/LICENSE

**Other consumers.** `@puppeteer/browsers` still offers a `chromium` browser type against the same bucket (`https://storage.googleapis.com/chromium-browser-snapshots`, folders `Linux_x64`, `Mac_Arm`, `Mac`, `Win`, `Win_x64`; `LINUX_ARM` is mapped to `Linux_x64`, i.e. no arm64) and reads `LAST_CHANGE` for "latest".
Source: https://raw.githubusercontent.com/puppeteer/puppeteer/main/packages/browsers/src/browser-data/chromium.ts

---

## 4. Strategy (b): Chrome for Testing (CfT)

**What it is.** "Chrome for Testing is a Chrome flavor that specifically targets web app testing and automation use cases." It exists because regular Chrome auto-updates and Google does not publish versioned Chrome downloads for users; CfT is "a versioned binary that's as close to regular Chrome as possible without negatively affecting the testing use case", "made available for every Chrome release" across "all channels (Stable, Beta, Dev, and Canary)". Caveat from the same post: "Chrome for Testing has been created purely for browser automation and testing purposes. It should only be used to consume trustworthy content." ChromeDriver releases are integrated into the same infrastructure.
Source: https://developer.chrome.com/blog/chrome-for-testing/

**Download URL pattern (verified).** `https://storage.googleapis.com/chrome-for-testing-public/<version>/<platform>/<binary>-<platform>.zip`, e.g. `…/152.0.7977.82/linux64/chrome-linux64.zip` (194,031,103 B, `Last-Modified: 02 Sep 2026`) and `…/152.0.7977.82/linux64/chrome-headless-shell-linux64.zip` (119,454,769 B). Executable inside: `chrome-linux64/chrome`, `chrome-win64/chrome.exe`, `chrome-mac-arm64/Google Chrome for Testing.app/Contents/MacOS/Google Chrome for Testing` (paths per Puppeteer's and Playwright's registries).

**JSON API endpoints** (README of GoogleChromeLabs/chrome-for-testing):

| Endpoint                                                        | Purpose                                                             |
| --------------------------------------------------------------- | ------------------------------------------------------------------- |
| `known-good-versions.json` / `-with-downloads.json`             | "The versions for which all CfT assets are available for download." |
| `last-known-good-versions.json` / `-with-downloads.json`        | latest such version per channel (Stable/Beta/Dev/Canary)            |
| `latest-patch-versions-per-build.json` / `-with-downloads.json` | latest patch for each `MAJOR.MINOR.BUILD`                           |
| `latest-versions-per-milestone.json` / `-with-downloads.json`   | latest version per milestone                                        |
| `<version>.json` (e.g. `123.0.6309.0.json`)                     | per-version download list                                           |
| text endpoints                                                  | plain-text equivalents of the three "latest" lists                  |

"The set of 'all CfT assets' for a given Chrome version is a matrix of supported binaries × platforms." Every entry carries both `version` and `revision` (trunk position), so a CfT pin also yields the snapshot-style revision for free.
Source: https://github.com/GoogleChromeLabs/chrome-for-testing#json-api-endpoints

**Observed data (index timestamp 2026-09-04T22:19Z).**

- Channels: Stable 152.0.7977.82 (r1669021), Beta 154.0.8037.0 (r1689415), Dev 155.0.8040.2 (r1691159), Canary 155.0.8043.0 (r1692319).
- `known-good-versions-with-downloads.json`: 5.06 MB, 2,504 versions from 113.0.5672.0 (r1121455) to 155.0.8043.0. Binaries: `chrome`, `chromedriver`, `chrome-headless-shell` (first appears at 120.0.6098.0). Versions per major recently: 151→67, 152→70, 153→39, 154→23, 155→7. `latest-patch-versions-per-build.json` has 1,955 builds; milestones 113–155.
- Platforms: `linux64`, `mac-arm64`, `mac-x64`, `win32`, `win64`, and `linux-arm64` — README: "`linux-arm64` (supported since v153.0.8001.0)". In the data, `linux-arm64` first appears at 153.0.8001.0 (r1676824) and exists for 42 versions across 153–155. **Stable (152) has no `linux-arm64`** (HEAD on `…/152.0.7977.82/linux-arm64/chrome-linux-arm64.zip` → 404); Beta `…/154.0.8037.0/linux-arm64/chrome-linux-arm64.zip` → 200, 196,474,766 B. Puppeteer's registry encodes the same cut-over: `LINUX_ARM` → `linux64` when `buildId < 153.0.8001.0`, else `linux-arm64`. No Windows arm64 platform exists.
- The CfT landing page reports every listed download URL as HTTP 200 at its last check.

Sources: https://googlechromelabs.github.io/chrome-for-testing/last-known-good-versions-with-downloads.json · https://googlechromelabs.github.io/chrome-for-testing/known-good-versions-with-downloads.json · https://googlechromelabs.github.io/chrome-for-testing/ · https://raw.githubusercontent.com/puppeteer/puppeteer/main/packages/browsers/src/browser-data/chrome.ts

**Mirror: npmmirror (`registry.npmmirror.com/-/binary/chrome-for-testing/`).** 2,506 version directories (Google's known-good list has 2,504), each with the platform subdirectories, including `linux-arm64/` for 154.0.8037.0. For 152.0.7977.82/linux64 it holds `chrome-linux64.zip` (194,031,103 B), `chrome-headless-shell-linux64.zip` (119,454,769 B), `chromedriver-linux64.zip`, mirrored 2026-09-03T20:26Z — about 21 hours after Google's `Last-Modified`. The JSON _index_ itself (`googlechromelabs.github.io`) is GitHub Pages and is not mirrored there; Playwright's `cdn.playwright.dev/builds/cft/<version>/…` also re-hosts the CfT zips (see §6).
Source: https://registry.npmmirror.com/-/binary/chrome-for-testing/

**Version pinning.** Any of: exact `MAJOR.MINOR.BUILD.PATCH`; `MAJOR.MINOR.BUILD` (→ latest patch); milestone `MAJOR` (→ latest version); channel name (→ moving target). Only versions in `known-good-versions` are guaranteed to have every binary on every platform.

**Alpine / musl.** Same as (a): glibc builds; Puppeteer's troubleshooting page (the reference #1114 cites) says "Chrome does not support Alpine out of the box"; runZeroInc's launcher header says "systems using MUSL instead of GLIBC (like Alpine Linux) require OS packages as well".

**Licence / terms.** CfT binaries are Google Chrome builds. Neither the CfT landing page nor the blog post publishes a licence for the binaries; the tooling repo is Apache-2.0. Google Chrome's terms page states "Most source code for Chrome is available free of charge under open source software license agreements at chrome://credits" and contains an AVC patent clause limiting the codec to "personal, non-commercial" use; it does not mention Chrome for Testing. This differs from Chromium snapshots, which are BSD-3-Clause in their entirety.
Sources: https://developer.chrome.com/blog/chrome-for-testing/ · https://raw.githubusercontent.com/GoogleChromeLabs/chrome-for-testing/main/LICENSE · https://www.google.com/chrome/terms/

---

## 5. Strategy (c): system-installed browser only, download optional

**What upstream already has.** `LookPath()` (table in §1.4), `Launcher.Bin(path)`, `-rod=bin=` flag, `NewUserMode()` (the only preset that calls `LookPath`). Auto-download is the default of `launcher.New()`; the docs describe `Bin`/`LookPath` as the manual override. There is no env var for the binary path and no "prefer system, download only if missing" mode.

**Comparators.**

- chromedp never downloads; `findExecPath()` tries, on Unix, `headless_shell`, `headless-shell`, `chromium`, `chromium-browser`, `google-chrome`, `google-chrome-stable`, `google-chrome-beta`, `google-chrome-unstable`, `/usr/bin/google-chrome`…; on darwin `/Applications/Chromium.app/…` and `/Applications/Google Chrome.app/…`; on Windows `chrome`, `chrome.exe`, `%ProgramFiles(x86)%`/`%ProgramFiles%` Chrome, `%USERPROFILE%\AppData\Local\Google\Chrome\…`, `…\Chromium\…`.
  Source: https://raw.githubusercontent.com/chromedp/chromedp/master/allocate.go
- Puppeteer: env `PUPPETEER_EXECUTABLE_PATH`, `PUPPETEER_SKIP_DOWNLOAD`, `PUPPETEER_CACHE_DIR`, `PUPPETEER_BROWSER`, `PUPPETEER_TMP_DIR`; cache `~/.cache/puppeteer`. Its Alpine recipe is `apk add chromium nss freetype harfbuzz ca-certificates ttf-freefont` + `PUPPETEER_EXECUTABLE_PATH=/usr/bin/chromium-browser`.
  Sources: https://raw.githubusercontent.com/puppeteer/puppeteer/main/packages/puppeteer/src/getConfiguration.ts · https://pptr.dev/troubleshooting · https://pptr.dev/guides/configuration
- runZeroInc/go-rod: search is `GetDefaultSystemChromiumDirs(os) × GetDefaultSystemChromiumExecutables(os)`. Dirs: darwin `/Applications, /usr/bin, /usr/local/bin, /opt/homebrew/bin`; linux `/opt/google/chrome{,-beta,-canary,-unstable}, /usr/bin, /usr/local/bin, /data/data/com.termux/files/usr/bin, /opt/microsoft/msedge`; BSDs by name; Windows `%LocalAppData%, %ProgramFiles%, %ProgramFiles(x86)%, %ProgramW6432%` × `Google\Chrome{, Beta, Canary, SxS, Dev}\Application`, `Chromium\Application`, `Microsoft\Edge\Application`. Executables include `Google Chrome for Testing.app/…` on macOS and `chrome.exe, chromium.exe, msedge.exe` on Windows. Options: `WithUseSystemChromium(bool)`, `WithUseChromiumPath(path)`, `WithUseAutomaticInstall(bool)`; cache dir from `ROD_BROWSER_CACHE`, then `XDG_CACHE_HOME`, then `$HOME/.cache`. Order in `ResolveChromiumPaths`: explicit path → cache (`LATEST.txt`) → system paths (if enabled) → download (if enabled).
  Source: https://raw.githubusercontent.com/runZeroInc/go-rod/main/lib/launcher/browser.go

**Platform coverage.** Anything that has a CDP-speaking browser installed, including the gaps of every download strategy: linux/arm64 Stable, windows/arm64 native Chrome/Edge, Alpine/musl, BSDs, Termux, NixOS.

**Alpine / musl.** The only strategy that works on musl: Alpine's `chromium` package is 152.0.7977.64-r0 on `edge` for both `x86_64` and `aarch64` (BSD-3-Clause, built 2026-08-28) and 142.0.7444.59-r0 on `v3.22`. #1114 (Alpine 3.20 Chromium hanging) is a property of that package/version, and the remedy in the Puppeteer note it cites is to change Alpine version or add `--disable-gpu`.
Sources: https://pkgs.alpinelinux.org/package/edge/community/x86_64/chromium · https://pkgs.alpinelinux.org/package/edge/community/aarch64/chromium · https://pkgs.alpinelinux.org/package/v3.22/community/x86_64/chromium

**Mirrors / restricted networks.** Not applicable to the library; the OS package mirrors (Alpine, Debian, Ubuntu, Homebrew, winget) are the user's concern.

**Version pinning.** None. The library's `lib/proto` is generated against one browser version (#2: r1321438 / Chrome 128 era); the installed browser is whatever the OS has (#2 notes Chrome ≥ 136 already breaks `NewUserMode()`). Upstream docs: "Each version of Rod only guarantees to work with its `launcher.DefaultRevision` of the browser."

**Licence.** Whatever the user installed.

---

## 6. Strategy (d): Playwright's browser builds

**What Playwright ships today (main, 2026-09-04).** `packages/playwright-core/browsers.json`: `chromium` revision `1244`, `browserVersion` `154.0.8037.0`, `title` **"Chrome for Testing"**; `chromium-headless-shell` r1244 "Chrome Headless Shell"; `firefox` r1543 (155.0); `webkit` r2360 (26.6); plus `ffmpeg`, `winldd`, `android`.
Source: https://raw.githubusercontent.com/microsoft/playwright/main/packages/playwright-core/browsers.json

**Playwright's Chromium is now CfT.** Release notes 1.57: "Playwright now runs on Chrome for Testing builds rather than Chromium. Headed mode uses `chrome`; headless mode uses `chrome-headless-shell`." and, at that release, "On Arm64 Linux, Playwright continues to use Chromium." Release 1.49 introduced the `chromium-headless-shell` split and the `'chromium'` channel for new headless. In current `registry/index.ts` the `chromium` download map uses `cftUrl('linux64/chrome-linux64.zip')`, `cftUrl('linux-arm64/chrome-linux-arm64.zip')`, `cftUrl('mac-x64/…')`, `cftUrl('mac-arm64/…')`, `cftUrl('win64/chrome-win64.zip')` for `ubuntu22.04/24.04/26.04`, `debian12/13`, `mac14/15/26`, `win64`; `undefined` (unsupported) for `ubuntu18.04/20.04`, `debian11`, `mac10.x–13`; there is no win32 or windows-arm64 key. `cftUrl` resolves to path `builds/cft/<browserVersion>/<suffix>` with the single mirror `https://cdn.playwright.dev`.
Sources: https://playwright.dev/docs/release-notes · https://raw.githubusercontent.com/microsoft/playwright/main/packages/playwright-core/src/server/registry/index.ts

**Hosts.** `PLAYWRIGHT_CDN_MIRRORS = ['https://cdn.playwright.dev/dbazure/download/playwright' (ESRP CDN), 'https://playwright.download.prss.microsoft.com/dbazure/download/playwright', 'https://cdn.playwright.dev']`. Overrides: `PLAYWRIGHT_DOWNLOAD_HOST`, and per-browser `PLAYWRIGHT_CHROMIUM_DOWNLOAD_HOST` / `PLAYWRIGHT_FIREFOX_DOWNLOAD_HOST` / `PLAYWRIGHT_WEBKIT_DOWNLOAD_HOST` (docs: "take precedence over `PLAYWRIGHT_DOWNLOAD_HOST`"). Observed redirects: `cdn.playwright.dev/dbazure/download/playwright/…` → 307 → `playwright.download.prss.microsoft.com/…`; `cdn.playwright.dev/builds/cft/154.0.8037.0/linux64/chrome-linux64.zip` → 307 → `storage.googleapis.com/chrome-for-testing-public/154.0.8037.0/linux64/chrome-linux64.zip` (i.e. the "bucket-direct" mirror is Google's bucket); upstream go-rod's `playwright.azureedge.net/builds/chromium/<rev>/chromium-linux-arm64.zip` still answers 307 to the same ESRP host. The ESRP gateway returned HTTP 400 (`X-DSGatewayServiceAPI-ErrorCode: 20012`, "GatewayServiceFileDetails Response is not in success state") to every HEAD and 1-byte range request from this network, so file existence on the Microsoft CDN could not be confirmed here; existence was confirmed indirectly via npmmirror's copies below.
Sources: registry `index.ts` (above) · https://playwright.dev/docs/browsers

**Linux arm64 is the one place Playwright still builds Chromium itself.** Legacy revision-numbered paths remain in use for arm64: npmmirror's copy of `playwright/builds/chromium/1243/` holds `chromium-linux-arm64.zip` (207,794,482 B) and `chromium-headless-shell-linux-arm64.zip` (116,442,984 B), mirrored 2026-09-01; `1237/` likewise (2026-08-16). Upstream go-rod's `RevisionPlaywright = 1124` directory on npmmirror holds all five legacy platforms (`chromium-linux-arm64.zip` 175,235,700 B, `chromium-linux.zip`, `chromium-mac-arm64.zip`, `chromium-mac.zip`, `chromium-win64.zip`). runZeroInc's `RevisionPlaywright = 1202` is not on npmmirror.
Source: https://registry.npmmirror.com/-/binary/playwright/builds/chromium/

**Mirror coverage on npmmirror (`-/binary/playwright/builds/`).** Directories: `android, cft, chromium-headless-shell, chromium-tip-of-tree, chromium-with-symbols, chromium, driver, ffmpeg, firefox-beta, firefox, webkit, winldd`. `chromium/` has 64 revision dirs, newest 1243 (current is 1244); `cft/` has only 8 versions (`149.0.7827.55 … 153.0.8010.12`; 154.0.8037.0 absent; 153.0.8010.12 has `linux64, mac-arm64, mac-x64, win64` only); `chromium-headless-shell/` has a single revision (1155). Coverage is therefore partial and lags.
Source: https://registry.npmmirror.com/-/binary/playwright/builds/

**Version pinning.** Playwright's integer roll number, incremented per roll (`feat(chromium): roll to r1244 (#42545)`, 2026-09-04; r1243 on 2026-08-28; r1241 on 2026-08-20 …). The number is Playwright-specific; the mapping to a Chrome version is only recorded in `browsers.json` at a given tag (`https://raw.githubusercontent.com/microsoft/playwright/v<version>/packages/playwright-core/browsers.json`, which is what upstream's generator reads). There is no JSON index of all Playwright revisions.
Source: https://api.github.com/repos/microsoft/playwright/commits?path=packages/playwright-core/browsers.json (via `gh api`)

**Platform coverage / OS support.** Playwright's system requirements: "Debian 12 / 13, Ubuntu 22.04 / 24.04 / 26.04 (x86-64 or arm64)", "macOS 14 (Sonoma) or later", "Windows 11+, Windows Server 2019+ or Windows Subsystem for Linux (WSL)". No win32, no windows-arm64, no Alpine.
Source: https://playwright.dev/docs/intro

**Alpine / musl.** Not supported (glibc builds; Alpine not in the OS list).

**Licence.** Playwright is Apache-2.0. The `chromium` artefacts it serves are CfT (Google Chrome) on x64/mac/win since 1.57, and Playwright-built Chromium (BSD) on linux-arm64; `chromium-headless-shell` follows the same split.
Sources: https://raw.githubusercontent.com/microsoft/playwright/main/LICENSE · https://playwright.dev/docs/release-notes

---

## 7. What runZeroInc/go-rod's switch to Chrome for Testing looked like

Repository self-description: "a stripped-down fork of @ysmood's go-rod/rod" with, among other changes, "Switch to Chrome for Testing for most platforms, fallback to Puppeteer builds for Linux on ARM64" (the code actually uses Playwright's build, see below), "Removal of leakless and automatic file drops", "Replacement of panic-based error handling with error return parameters", "Removal of artifacts created by `go generate`". "The focus of this fork is almost entirely on screenshots."
Source: https://github.com/runZeroInc/go-rod (README via `gh api repos/runZeroInc/go-rod/readme`)

**The switch commit: `27e2c031e0` "start rework of go-rod downloader" (2025-12-07), +444/−212 over 8 files (`browser.go` +314/−83).** Removed: `type Host func(revision int) string`, `hostConf`, `HostGoogle`, `HostNPM`, `HostPlaywright`, the `Hosts []Host` field, and the `launcher.Browser{Hosts, Revision}` construction. Added:

- `const ChromeForTestingLatestDownloadsURL = "https://googlechromelabs.github.io/chrome-for-testing/last-known-good-versions-with-downloads.json"` and a typed struct for its `channels.{Stable,Beta,Dev,Canary}.downloads.{chrome,chromedriver,chrome-headless-shell}[]{platform,url}`.
- `ResolveChromeForTestingPlatform(os, arch)`: `darwin/arm64→mac-arm64`, `darwin/amd64→mac-x64`, `linux/amd64→linux64`, `windows/amd64→win64`, `windows/386→win32`; everything else → `""`.
- `const PlaywrightBrowserMetaURL = "https://raw.githubusercontent.com/microsoft/playwright/refs/heads/main/packages/playwright-core/browsers.json"` and `const PlaywrightLinuxArm64URL = "https://playwright.azureedge.net/builds/chromium/%s/chromium-linux-arm64.zip"`.
- `ResolveLatestDownloadURL(os, arch) (url, revision, error)`: for a CfT platform, fetch the index and return the **Stable** channel's `chrome` download for that platform plus its `revision`; for `linux/arm64`, fetch Playwright's `browsers.json` from `main` and format the arm64 URL with the `chromium` entry's revision; otherwise `unsupported platform`.
- `NewBrowser(srcOS, srcArch) (*Browser, error)` (error-returning; `Revision` set from the resolved metadata, not a constant).
- `revision.go` bumped `RevisionDefault 1321438 → 1536371` and `RevisionPlaywright 1124 → 1202`; the constants still exist in the fork but `browser.go` no longer references them.
- Same day: "omit fetchup" (`9bce88de8c`), "omit leakless", "omit manager".

Source: `gh api repos/runZeroInc/go-rod/commits/27e2c031e0` · https://github.com/runZeroInc/go-rod/commit/27e2c031e0

**Follow-up commits touching `lib/launcher` (selection).** `bd12524773` (2025-12-25) "update LookPath to use ResolveChromePathsFromSystem" (−42 lines of hard-coded paths); `ca1df3e585` (2025-12-26) "rip out edge, emulate x64 on windows arm64", reverted the next day in `657bf41a80`, yet current code keeps `windows/arm64 → win64` with the comment "Windows ARM64 uses the x86_64 binary via emulation" and `resolveLatestDownloadURL` rewrites `arch = "amd64"` for it; `066be16fad` "chrome -> chromium for consistency"; `c29c18f4db` "hold the LATEST.txt lock for the duration"; `77249b3c8c` "retry unzip for temporary errors"; `6ecfb3078b` "skip --version validation, as it breaks on windows"; `029132a20c` "export launcher.Browser for caller use"; 2026-01: sandbox/non-root user handling, Windows ACLs, launch timeout; 2026-04/06: orphan-process cleanup, `pdeathsig`, `ForceClose`.
Source: `gh api "repos/runZeroInc/go-rod/commits?path=lib/launcher&per_page=100"`

**Resulting design (main, `lib/launcher/browser.go`, 1,186 lines).**

- No pinned version: `DownloadAndInstall()` resolves the _current_ Stable CfT version at run time (`ResolveLatestDownloadURLWithCache`, in-process cache `chromiumDownloadURLCacheTimeout = 24h`), reads the installed revision from `<CacheDir>/LATEST.txt`, and upgrades when `latest > installed`. Locks: `flock` on `LATEST.txt.lock` and on `chromium-<rev>.lock`. Directory naming keeps upstream's `chromium-<revision>` (the CfT `revision` field).
- Cache dir: `ROD_BROWSER_CACHE` → `XDG_CACHE_HOME/<suffix>` → `$HOME/.cache/<suffix>`.
- Extraction guarded by `MaxPackageFileSize = 1 GiB`, `MaxPackageTotalSize = 2 GiB`, `MaxMetadataSize = 256 MiB`, `MaxMetadataTimeout = 10 s`, three unzip retries.
- Header comment: "Google's Chromium-for-Testing (CfT) project provides builds for macOS (Intel and ARM), Windows (x86 and x64), and Linux (x64) … Playwright offers Chromium builds for Linux on ARM … Other platforms should use operating-specific packages to install Chromium or Chrome. Note that systems using MUSL instead of GLIBC (like Alpine Linux) require OS packages as well." A `TODO` considers driving chromedriver instead (link to VibiumDev/vibium `installer.go`).
- Observations against today's data: the fork predates CfT's `linux-arm64` (153+), so it still routes arm64 through Playwright's `builds/chromium/<rev>/chromium-linux-arm64.zip`, which Playwright continues to publish (r1243 confirmed via npmmirror); it uses the _old_ host `playwright.azureedge.net` (still redirecting) rather than the mirrors in Playwright's current registry; it reads `browsers.json` from Playwright's `main` branch, so the arm64 build tracks Playwright's unreleased roll, not a release; it consumes only the `chrome` binary, never `chrome-headless-shell`; and it has no mirror/fallback host (npmmirror was removed with `HostNPM`).

Source: https://raw.githubusercontent.com/runZeroInc/go-rod/main/lib/launcher/browser.go

---

## 8. Cross-cutting facts

- **Every downloadable option is glibc.** (a), (b) and (d) all ship glibc-linked Linux binaries; only (c) with a distro package covers musl. #1114 is a musl-package bug report and is unaffected by the downloader choice.
- **linux/arm64 today:** (a) none; (b) Beta/Dev/Canary only — every known-good 153+ version has it, Stable is still 152; (d) Playwright-built Chromium at Playwright roll numbers; (c) distro packages (Alpine aarch64 152.0.7977.64-r0, etc.).
- **windows/arm64 today:** only (a) (`Win_Arm64` snapshot prefix, not mirrored on npmmirror) and (c); (b) and (d) have no native artefact (runZeroInc runs `win64` under emulation).
- **Mirrors for mainland China:** npmmirror covers (a) for the five classic prefixes (~1 day lag), (b) completely (~21 h lag, all platforms), (d) partially and stale. The CfT JSON index lives on GitHub Pages and is not mirrored by npmmirror; Playwright's `browsers.json` lives on `raw.githubusercontent.com`.
- **Chrome-version pinning is only native to (b).** (a) pins trunk positions that don't map to shipped versions (chromiumdash can approximate); (d) pins Playwright roll numbers whose Chrome version is recorded per Playwright tag; (c) pins nothing.
- **Headless shell:** (b) publishes `chrome-headless-shell-<platform>.zip` (~119 MB vs ~194 MB for `chrome` on linux64, 152.0.7977.82) since 120.0.6098.0; (d) publishes `chromium-headless-shell` (Playwright 1.49+); (a) has `content-shell.zip` but no headless shell.
- **Two existing Go implementations to study:** upstream's host-race (`fetchup`) with a generated common revision, and runZeroInc's runtime "latest Stable" resolver with `LATEST.txt` + `flock`. Neither pins an exact Chrome version in code.

---

## 9. Sources

Upstream go-rod/rod

- https://raw.githubusercontent.com/go-rod/rod/main/lib/launcher/browser.go
- https://raw.githubusercontent.com/go-rod/rod/main/lib/launcher/revision.go
- https://raw.githubusercontent.com/go-rod/rod/main/lib/launcher/revision/main.go
- https://raw.githubusercontent.com/go-rod/rod/main/lib/launcher/launcher.go
- https://raw.githubusercontent.com/go-rod/rod/main/lib/launcher/utils.go
- https://raw.githubusercontent.com/go-rod/rod/main/lib/defaults/defaults.go
- https://raw.githubusercontent.com/go-rod/rod/main/.github/workflows/check-revision.yml
- https://raw.githubusercontent.com/go-rod/rod/main/lib/docker/Dockerfile
- https://raw.githubusercontent.com/go-rod/rod/main/lib/utils/get-browser/main.go
- https://github.com/go-rod/rod/commit/e0d0a7eea5 (npmmirror host, 2022-02-08)
- https://github.com/go-rod/rod/commit/73907a8616 (NixOS path, 2024-11-25)
- https://github.com/go-rod/rod/issues/1114
- https://raw.githubusercontent.com/go-rod/go-rod.github.io/main/compatibility.md
- https://raw.githubusercontent.com/go-rod/go-rod.github.io/main/custom-launch.md
- https://github.com/headlesslab/wand/issues/2 (prior research: revision/protocol dating, fork survey)

Chromium snapshots

- https://www.chromium.org/getting-involved/download-chromium/
- https://commondatastorage.googleapis.com/chromium-browser-snapshots/?delimiter=/&prefix=
- https://commondatastorage.googleapis.com/chromium-browser-snapshots/<prefix>/LAST_CHANGE (Linux_x64, Mac, Mac_Arm, Win, Win_x64, Win_Arm64, Linux, Linux_ARM_Cross-Compile)
- https://commondatastorage.googleapis.com/chromium-browser-snapshots/Linux_x64/1321438/REVISIONS
- https://storage.googleapis.com/chromium-browser-snapshots/<prefix>/1321438/<zip> (HEAD)
- https://registry.npmmirror.com/-/binary/chromium-browser-snapshots/ and per-platform listings
- https://chromiumdash.appspot.com/fetch_releases?channel=Stable&platform=Linux&num=1
- https://chromiumdash.appspot.com/fetch_milestones?mstone=127 and ?mstone=128
- https://chromium.googlesource.com/chromium/src/+/main/LICENSE
- https://raw.githubusercontent.com/puppeteer/puppeteer/main/packages/browsers/src/browser-data/chromium.ts

Chrome for Testing

- https://googlechromelabs.github.io/chrome-for-testing/
- https://github.com/GoogleChromeLabs/chrome-for-testing#json-api-endpoints
- https://googlechromelabs.github.io/chrome-for-testing/last-known-good-versions-with-downloads.json
- https://googlechromelabs.github.io/chrome-for-testing/known-good-versions-with-downloads.json
- https://googlechromelabs.github.io/chrome-for-testing/latest-patch-versions-per-build.json
- https://googlechromelabs.github.io/chrome-for-testing/latest-versions-per-milestone.json
- https://developer.chrome.com/blog/chrome-for-testing/
- https://raw.githubusercontent.com/GoogleChromeLabs/chrome-for-testing/main/LICENSE
- https://www.google.com/chrome/terms/
- https://storage.googleapis.com/chrome-for-testing-public/152.0.7977.82/linux64/chrome-linux64.zip (HEAD) and related HEADs for 152/154 linux-arm64
- https://registry.npmmirror.com/-/binary/chrome-for-testing/
- https://raw.githubusercontent.com/puppeteer/puppeteer/main/packages/browsers/src/browser-data/chrome.ts

System browsers / Alpine

- https://raw.githubusercontent.com/chromedp/chromedp/master/allocate.go
- https://raw.githubusercontent.com/puppeteer/puppeteer/main/packages/puppeteer/src/getConfiguration.ts
- https://pptr.dev/troubleshooting
- https://pptr.dev/guides/configuration
- https://pkgs.alpinelinux.org/package/edge/community/x86_64/chromium
- https://pkgs.alpinelinux.org/package/edge/community/aarch64/chromium
- https://pkgs.alpinelinux.org/package/v3.22/community/x86_64/chromium

Playwright

- https://raw.githubusercontent.com/microsoft/playwright/main/packages/playwright-core/browsers.json
- https://raw.githubusercontent.com/microsoft/playwright/main/packages/playwright-core/src/server/registry/index.ts
- https://playwright.dev/docs/browsers
- https://playwright.dev/docs/intro (system requirements)
- https://playwright.dev/docs/release-notes (1.49, 1.57)
- https://raw.githubusercontent.com/microsoft/playwright/main/LICENSE
- https://registry.npmmirror.com/-/binary/playwright/builds/ (and `chromium/`, `cft/`, `chromium-headless-shell/` sublistings)
- HEAD/redirect checks on https://cdn.playwright.dev/… and https://playwright.azureedge.net/… (ESRP gateway answered 400 to HEAD/range from this network)

runZeroInc/go-rod

- https://github.com/runZeroInc/go-rod (README)
- https://github.com/runZeroInc/go-rod/commit/27e2c031e0
- https://github.com/runZeroInc/go-rod/commit/bd12524773
- https://github.com/runZeroInc/go-rod/commit/ca1df3e585 and https://github.com/runZeroInc/go-rod/commit/657bf41a80
- https://raw.githubusercontent.com/runZeroInc/go-rod/main/lib/launcher/browser.go
- https://raw.githubusercontent.com/runZeroInc/go-rod/main/lib/launcher/revision.go
- `gh api "repos/runZeroInc/go-rod/commits?path=lib/launcher&per_page=100"` (pages 1–3)
