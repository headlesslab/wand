# wand

A Chrome DevTools Protocol driver for browser automation and web scraping in Go. wand descends from a snapshot of go-rod's code and is maintained independently by headlesslab.

## Language

### Lineage

**Upstream**:
The go-rod/rod repository whose code wand was copied from. Its code has been frozen since December 2024; wand does not track it.
_Avoid_: origin, parent, original, fork source

**Snapshot**:
The single upstream commit wand's initial code was copied from, imported without git history.
_Avoid_: fork point, base commit, import commit

### Roadmap

**Baseline release**:
wand's first release: the snapshot renamed to the wand module and made to work on current Chrome, with no API redesign.
_Avoid_: v1, MVP, first version, migration release

**API modernization**:
The effort, after the baseline release, to bring wand's API to Playwright/Puppeteer-level ergonomics. Not part of the baseline release.
_Avoid_: v2, rewrite, new API

**Stealth**:
Client-side evasions injected into a page so a site's bot-detection scripts see a human-looking browser. Not part of the baseline release; a later effort of its own.
_Avoid_: anti-bot, anti-bot-detection, anti-detection, undetected

### Platforms

**Domestic platform**:
A Chinese-market OS and CPU combination wand targets: Kylin, UOS, or openEuler on x86-64 (Hygon, Zhaoxin), ARM64 (Phytium, Kunpeng), or LoongArch new-world (Loongson 3A5000 / 3A6000, `linux/loong64`). On loong64 only new-world (ABI 2.0) distributions qualify, which today means openEuler 22.03 LTS and later; Kylin V10 and UOS 20 loong64 builds are old-world and out. Users both build on it with the distro's own Go and deploy cross-compiled binaries to it.
_Avoid_: Chinese platform, localized platform, xinchuang, 国产化 left untranslated in English docs

**Go floor**:
The Go version `go.mod` declares. Anchored to the newest Go the current openEuler LTS ships natively on every architecture it supports; it moves only when a new LTS raises it.
_Avoid_: minimum Go, Go baseline, the go directive

**Support tier**:
How much wand promises for a `GOOS/GOARCH`. Tier 1: built and tested with a real browser in CI. Tier 2: cross-compiled in CI, runtime best-effort. Tier 3: no promise.
_Avoid_: supported platform (without a tier), first-class, best-effort (as a bare label)

### Chrome alignment

**Target Chrome**:
The single Chrome stable version wand is aligned to: the protocol layer is generated for it and the launcher's default managed browser is pinned to it, and both move together. It is whatever Chrome for Testing's Stable channel serves at the time of the last roll.
_Avoid_: pinned browser, default revision, browser version (as a bare label)

**Protocol roll**:
The devtools-protocol revision wand's protocol layer is generated from: the newest roll at or below the Target Chrome's branch point.
_Avoid_: schema version, protocol revision, RevisionDefault

**Chromium trunk build**:
A Chromium binary from Google's continuous-build archive, identified by the trunk commit position it was built from; it corresponds to no shipped Chrome release.
_Avoid_: Chromium snapshot (Snapshot is the go-rod commit), revision (as a bare label), nightly

**Companion Chromium**:
The Chromium trunk build wand pins alongside the Target Chrome: the newest position at or below the Target Chrome's branch point that exists for every platform in the launcher's Chromium table, rolled together with the Target Chrome.
_Avoid_: Target Chromium, Chromium pin, default revision, RevisionDefault

### Browser acquisition

**System browser**:
A CDP-capable browser already installed on the host outside wand's browser cache, found by the launcher's discovery or named by the user.
_Avoid_: local browser, installed browser, user browser

**Managed browser**:
A browser wand downloaded into its browser cache at a pinned version: the Target Chrome by default, or the Companion Chromium.
_Avoid_: downloaded browser, auto-downloaded browser, bundled browser

**Browser resolution**:
The launcher's ordered search for a browser binary: an explicit path, then a system browser, then a managed browser, downloading one only when nothing earlier is found.
_Avoid_: browser lookup, LookPath (as a name for the whole order), auto-download (as a name for the whole order)

**Browser source**:
The archive family a managed browser is downloaded from: Chrome for Testing by default, or Chromium trunk builds.
_Avoid_: channel, provider, host (that is where an archive is fetched from)

**Download host**:
A URL template that serves managed-browser archives; wand ships Google's buckets and npmmirror and probes every configured host concurrently.
_Avoid_: mirror (as the generic term), CDN, registry
