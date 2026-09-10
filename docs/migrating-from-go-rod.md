# Migrating from go-rod

wand's code is a snapshot of [go-rod](https://github.com/go-rod/rod) at commit `393ac0d` (2024-12-07), renamed and brought current; see [`NOTICE`](../NOTICE) for attribution. The baseline release redesigns nothing, so in practice go-rod's method surface is wand's: `Browser`, `Page`, `Element`, the `Must*` family, `lib/proto`, `lib/launcher`, `lib/cdp` and the rest all keep their names, receivers and behaviour unless this page says otherwise. That is not a compatibility promise. ADR-0002 positions wand as a new project rather than a drop-in replacement for go-rod, and the surface survives only because the redesign is deferred to the API modernization.

That makes the move an import swap plus a short list of behaviour changes. This page has all of both, and a [Known limitations](#known-limitations) section listing what wand does _not_ fix, so you can decide in one sitting.

Two things worth knowing before you start:

- **The API will change in a later minor.** The API modernization is the effort after the baseline; it is not in this release and it has no date, but it will not keep go-rod's method names for ever. See the [Roadmap](../README.md#roadmap).
- **`go get -u=patch` is always safe.** Under wand's v0 rule a minor may break and a patch never does (ADR-0008).

## At a glance

| #                                                                | Change                                                                                        | Kind                           |
| ---------------------------------------------------------------- | --------------------------------------------------------------------------------------------- | ------------------------------ |
| [1](#1-the-import-prefix)                                        | `github.com/go-rod/rod` → `github.com/headlesslab/wand`, package `rod` → `wand`               | compile error until fixed      |
| [2](#2-gson-becomes-lazyjson)                                    | `github.com/ysmood/gson` → `github.com/headlesslab/lazyjson`                                  | compile error until fixed      |
| [3](#3-symbols-that-left-the-public-api)                         | `lib/assets`, seven `lib/utils` helpers, the revision and host constants, `defaults.LockPort` | compile error if you used them |
| [4](#4-the-two-commands)                                         | `lib/launcher/rod-manager` → `cmd/wand-manager`; new `cmd/wand-fetch-browser`                 | command path                   |
| [5](#5-the-browser-cache-moved-and-a-system-browser-comes-first) | System browser first; cache under `os.UserCacheDir()/wand/browser`                            | runtime behaviour              |
| [6](#6-leakless-is-gone-the-orphan-guard-replaces-it)            | No guard binary, no guard process; `Leakless()` keeps its name                                | runtime behaviour              |
| [7](#7-user-mode-has-a-profile-directory-of-its-own)             | `NewUserMode()` uses `os.UserConfigDir()/wand/user-mode`                                      | runtime behaviour              |
| [8](#8--rod-becomes--wand)                                       | `-rod=` → `-wand=`, `DISABLE_ROD_FLAG` → `DISABLE_WAND_FLAG`, plus `WAND_BROWSER_*`           | flags and environment          |
| [9](#9-cdperrctxdestroyed-matches-a-second-message)              | Chrome 152's "Inspected target navigated or closed" now matches                               | runtime behaviour              |
| [10](#10-the-container-image)                                    | `ghcr.io/go-rod/rod` → `ghcr.io/headlesslab/wand`, one multi-arch manifest                    | deployment                     |
| [11](#11-navigations-return-once-the-document-has-committed)     | `Navigate`, `NavigateBack` and `NavigateForward` return once the document has committed       | runtime behaviour              |

## 1. The import prefix

`github.com/go-rod/rod` becomes `github.com/headlesslab/wand`, and the root package is `wand` rather than `rod`. Every subpackage keeps its path below the prefix, so `lib/proto`, `lib/launcher`, `lib/launcher/flags`, `lib/cdp`, `lib/input`, `lib/devices`, `lib/js`, `lib/utils` and `lib/defaults` are a prefix swap and nothing more.

```go
import (
    "github.com/headlesslab/wand"
    "github.com/headlesslab/wand/lib/launcher"
    "github.com/headlesslab/wand/lib/proto"
)

browser := wand.New().MustConnect()
```

The mechanical part:

```sh
grep -rl 'github.com/go-rod/rod' --include='*.go' . |
  xargs sed -i 's|github.com/go-rod/rod|github.com/headlesslab/wand|g'
go mod edit -droprequire github.com/go-rod/rod
go mod tidy
```

What is left is the identifier: `rod.Browser`, `rod.New`, `rod.ErrElementNotFound` and friends are `wand.` now. If your tree names the package in many places and you would rather not touch them all today, alias the import and nothing else changes:

```go
import rod "github.com/headlesslab/wand"
```

## 2. `gson` becomes `lazyjson`

`github.com/ysmood/gson` is [`github.com/headlesslab/lazyjson`](https://github.com/headlesslab/lazyjson), a snapshot of gson with every identifier unchanged: `lazyjson.JSON`, `lazyjson.New`, `lazyjson.NewFrom`, `lazyjson.Int` and the rest are gson's, same shape and same methods. So an exported API of your own that hands out a `gson.JSON` only changes an import path.

```sh
grep -rl 'github.com/ysmood/gson' --include='*.go' . |
  xargs sed -i 's|github.com/ysmood/gson|github.com/headlesslab/lazyjson|g'
go mod edit -droprequire github.com/ysmood/gson
go mod tidy
```

The alias trick works here too: `import gson "github.com/headlesslab/lazyjson"`.

It surfaces wherever go-rod's did — `Element.Property`, `HijackRequest.JSONBody`, `proto` fields such as `RuntimeRemoteObject.Value`, `proto.DOMDescribeNode.Depth`.

This is part of a wider change you do not otherwise see: **no `ysmood/*` module is in wand's runtime dependency graph.** It is five Satellite modules headlesslab maintains — `eventbus`, `lazyjson`, `seqdiff`, `leakcheck` and `fetch` — plus `golang.org/x/sys`, and no test framework is linked into your binary. `go get -u ./...` on a project that depends on wand keeps building, which is what broke on go-rod when fetchup and got changed their APIs (rod #1203, #1195, #1202, #1237).

## 3. Symbols that left the public API

Everything below is dev-only code that go-rod exported because its generators lived in the same module. wand moved it under `internal/`, so the compiler tells you at once if you used any of it.

| Gone from      | What                                                                | Where it went                                                                              |
| -------------- | ------------------------------------------------------------------- | ------------------------------------------------------------------------------------------ |
| `lib/assets`   | the whole package: `Monitor`, `MonitorPage`, `MousePointer`         | `internal/assets`; copy the constant if you embedded one                                   |
| `lib/utils`    | `Exec`, `ExecLine`, `S`, `EscapeGoString`, `ReadString`, `TestEnvs` | `internal/devutil`, unchanged                                                              |
| `lib/utils`    | `UseNode`                                                           | gone; `internal/devutil` installs and runs the Node tools its own way                      |
| `lib/launcher` | `RevisionDefault`, `RevisionPlaywright`                             | `lib/launcher/pins`: `pins.ChromeVersion`, `pins.ChromiumPosition`, `pins.ProtocolRoll`    |
| `lib/launcher` | `Host`, `HostGoogle`, `HostNPM`, `HostPlaywright`                   | `launcher.DefaultHosts(source)`, which returns URL templates; Playwright's host is dropped |
| `lib/defaults` | `LockPort`                                                          | gone with leakless; the download lock is a file lock inside `fetch`                        |

The rest of `lib/utils` — `Sleeper`, `Retry`, `BackoffSleeper`, `MustToJSON`, `Dump`, `OutputFile` and so on — is untouched. go-rod's `lib/utils/shell` moved to `internal/tools/shell`, but it was `package main` there too, so nothing could import it and nothing breaks.

`lib/launcher/flags` keeps every identifier, but the wand-owned flags carry `wand-` values now: `flags.Bin` is `"wand-bin"`, `flags.Leakless` is `"wand-leakless"`, and so on for `WorkingDir`, `Env`, `XVFB`, `Preferences` and `KeepUserDataDir` (`flags.Download` is new). Code that uses the constants needs no change; code that passes the string literal to `Launcher.Set` does. A remote launch carries these names over the wire, so pair a wand client with a `wand-manager`, not with a `rod-manager`.

`launcher.Browser`, the download helper, kept its name and its `Context`, `RootDir`, `Logger` and `HTTPClient` fields, and gained `Source`, `Binary` and `Version` beside `Revision`. Its `Hosts` is `[]string` of URL templates now instead of `[]Host`, and `LockPort` is gone.

## 4. The two commands

`go install` names a binary after the last element of its path, so both commands carry the `wand-` prefix and live under `cmd/`:

| go-rod                                           | wand                                                 |
| ------------------------------------------------ | ---------------------------------------------------- |
| `github.com/go-rod/rod/lib/launcher/rod-manager` | `github.com/headlesslab/wand/cmd/wand-manager`       |
| —                                                | `github.com/headlesslab/wand/cmd/wand-fetch-browser` |

```sh
go install github.com/headlesslab/wand/cmd/wand-manager@latest
go install github.com/headlesslab/wand/cmd/wand-fetch-browser@latest
```

`wand-manager` is go-rod's remote-launch manager under a new name: same flags, same protocol, `launcher.MustNewManaged("ws://...")` on the client side. Its binary-path protection follows the new browser cache, so a client that asks for a `Bin` outside it is refused unless the manager runs with `--allow-all`.

`wand-fetch-browser` is new. It downloads the Managed browser ahead of time and prints its path on stdout, for a Docker build or an offline bundle; go-rod had no command for this.

## 5. The browser cache moved, and a System browser comes first

Two changes here, and the second is the one you will notice.

**A browser you already have is used.** go-rod's `launcher.New()` went straight to its pinned Chromium download unless you named a binary; only `NewUserMode()` looked for an installed browser, through `LookPath()`. wand's Browser resolution takes the first of:

1. `Launcher.Bin()` in code
2. the `-wand=bin=<path>` flag
3. `WAND_BROWSER_BIN`
4. a System browser — Google Chrome, Chromium or Microsoft Edge at the paths `LookPath` searches on this OS
5. the Managed browser already in the cache
6. a download of the Managed browser

So a developer machine with Chrome on it downloads nothing, and a fresh container downloads the Target Chrome once. The download is the fallback, never the default, and `WAND_BROWSER_DOWNLOAD=0` or `Launcher.Download(false)` switches it off; then, and on a platform with nothing to download, the launch fails with `launcher.ErrNoBrowser` and an error naming every step it tried. A browser of a different major version than the Target Chrome is launched anyway, with one line through the browser's logger naming both versions.

**The cache directory changed**, and go-rod's is not reused:

|         | go-rod                     | wand                                                    |
| ------- | -------------------------- | ------------------------------------------------------- |
| Linux   | `$HOME/.cache/rod/browser` | `$XDG_CACHE_HOME`, else `~/.cache`, then `wand/browser` |
| macOS   | `$HOME/.cache/rod/browser` | `~/Library/Caches/wand/browser`                         |
| Windows | `%APPDATA%\rod\browser`    | `%LocalAppData%\wand\browser`                           |

That is `os.UserCacheDir()`, which on Windows is the local profile rather than the roaming one, so roaming profiles stop carrying browsers between machines. `WAND_BROWSER_CACHE` or `Browser.RootDir` overrides it. Subdirectories are `chrome-<version>`, `chrome-headless-shell-<version>` and `chromium-<revision>`. Your old `rod/browser` directory is not read and not deleted; remove it by hand when you are done with go-rod.

Every download is verified against a SHA-256 recorded in `lib/launcher/pins` before extraction, whichever host served it, and the hosts — Google's buckets and npmmirror by default — are probed concurrently, so the first to answer serves it and there is no timeout to wait through behind a restricted network.

The six variables, all new (go-rod had none):

| Variable                | Values                                        | In code                                    |
| ----------------------- | --------------------------------------------- | ------------------------------------------ |
| `WAND_BROWSER_BIN`      | path of the browser to launch                 | `Launcher.Bin()`; the flag is `-wand=bin=` |
| `WAND_BROWSER_CACHE`    | the browser cache directory                   | `Browser.RootDir`                          |
| `WAND_BROWSER_HOSTS`    | URL templates separated by commas             | `Launcher.Hosts()`                         |
| `WAND_BROWSER_DOWNLOAD` | `0` switches the download off                 | `Launcher.Download()`                      |
| `WAND_BROWSER_SOURCE`   | `chrome` (default) or `chromium`              | `Launcher.Source()`                        |
| `WAND_BROWSER_BINARY`   | `chrome` (default) or `chrome-headless-shell` | `Launcher.Binary()`                        |

Precedence is the option set in code, then the flag, then the variable, then discovery. The whole story is in the [launcher's README](../lib/launcher/README.md).

## 6. leakless is gone; the Orphan guard replaces it

go-rod kept a launched browser from outliving its process by dropping a prebuilt [leakless](https://github.com/ysmood/leakless) binary into the temp directory and running it as a guard process. Windows Defender, 360 and Tencent PC Manager flagged it intermittently, Falco alerted on it in Kubernetes, it existed for five targets only, and its port lock could hang (rod #865, #1214, #1232, #720, #1229).

wand's Orphan guard writes nothing and starts nothing. The guarantee is the same — a browser `Launcher.Launch()` started does not outlive the wand process, however that process dies — and it is kept by the operating system:

- **Windows**: the browser joins a kill-on-close job object that the kernel closes with the wand process.
- **Linux**: the browser is started with the parent-death signal `SIGKILL`, from a goroutine that holds its OS thread for the browser's lifetime.
- **Every POSIX platform, macOS included**: the browser is started with `--remote-debugging-pipe` on descriptors 3 and 4 that wand opens and never speaks on — the Pipe tether — and Chromium 89 and later exits by itself when they close. CDP still runs over the WebSocket `Launch()` returns.

**`Launcher.Leakless(bool)` keeps its name, its place and its defaults**: on in `launcher.New()`, off in `launcher.NewUserMode()`. So your launcher configuration compiles unchanged and means the same thing — it is now the switch for the guard above rather than for the helper process.

Two details worth knowing:

- `Launcher.FormatArgs()` never lists `--remote-debugging-pipe`, because the flag is only valid with its two descriptors open. A command you assemble from those arguments and run yourself therefore has no guard.
- Under `Launcher.XVFB()` the tether reaches the browser through `xvfb-run`, but `xvfb-run` and its Xvfb server sit outside it, so a wand process that dies hard leaves the Xvfb server behind.

With the guard off, or on a browser that rejects the pipe, `Launcher.Kill()` and `Launcher.Cleanup()` still kill the browser's process group on the way out, exactly as before.

## 7. User mode has a profile directory of its own

Since Chrome 136, branded Google Chrome refuses remote debugging on its default profile and exits with "DevTools remote debugging requires a non-default data directory". `launcher.NewUserMode()` pointed at exactly that directory, so it has been broken on every current Chrome (rod #1189, #1184).

wand's `NewUserMode()` defaults `--user-data-dir` to a persistent directory wand owns, `launcher.DefaultUserModeDir`:

|         | Path                                                        |
| ------- | ----------------------------------------------------------- |
| Linux   | `$XDG_CONFIG_HOME`, else `~/.config`, then `wand/user-mode` |
| macOS   | `~/Library/Application Support/wand/user-mode`              |
| Windows | `%AppData%\wand\user-mode`                                  |

That is `os.UserConfigDir()`, and the directory is made at the first launch. `Launcher.UserDataDir()` still overrides it, and pointing it at Chrome's real profile gets Chrome's refusal unchanged — wand does not work around it.

**Your existing sign-ins do not carry over.** Sign in once in the wand profile and the session is there on the next run, which is the point of the preset. The browser itself comes from Browser resolution like any other, so it is the Chrome you have installed unless you say otherwise, and wand passes `--no-first-run` so branded Chrome on Linux and macOS does not hold the launch on a modal first-run dialog.

The Orphan guard stays off in User mode, so the browser outlives your program; a launch finds a browser already listening on the port before starting another. After a browser the launcher started exits, `Cleanup()` removes the profile like any user data directory and `Kill()` leaves it; a launcher that found a browser already listening owns neither, and both calls leave both alone.

## 8. `-rod=` becomes `-wand=`

The `lib/defaults` flag is renamed, options and syntax unchanged:

```sh
go run main.go -wand=show
go run main.go -wand show,trace,slow=1s,monitor
go run main.go --wand="slow=1s,dir=path/has /space,monitor=:9223"
```

`DISABLE_ROD_FLAG` is `DISABLE_WAND_FLAG`. An unknown option still panics, with `unknown wand env option:` in the message.

The flag is the middle of three configuration surfaces: an option set in code beats it, and it beats the `WAND_BROWSER_*` variables of [section 5](#5-the-browser-cache-moved-and-a-system-browser-comes-first), which beat the launcher's own discovery. Only `bin` has both a flag option and a variable.

## 9. `cdp.ErrCtxDestroyed` matches a second message

Chrome 152 changed the CDP error message for an evaluation interrupted by navigation from "Execution context was destroyed." to "Inspected target navigated or closed", so `errors.Is(err, cdp.ErrCtxDestroyed)` stopped matching on go-rod.

`cdp.ErrCtxDestroyed` matches both messages now, so the check you already wrote works across the whole Support window:

```go
if errors.Is(err, cdp.ErrCtxDestroyed) {
    // the page navigated out from under the evaluation
}
```

`cdp.ErrCtxNotFound` is a separate sentinel and is not merged into it. Comparing `err.Message` yourself is what breaks — use `errors.Is`.

Beside it, the CDP client reports a connection the browser closed as `io.EOF` on every Tier 1 OS, Windows included, where the raw connection-reset error used to surface instead. One sentinel, everywhere.

## 10. The container image

`ghcr.io/go-rod/rod` is `ghcr.io/headlesslab/wand`. It is `ubuntu:noble` with the Target Chrome inside — the same binary CI tested, hash-verified at build time — and `wand-manager` as its command:

```sh
docker run --rm -p 7317:7317 ghcr.io/headlesslab/wand
```

Tags mirror release tags (`vX.Y.Z`, `vX.Y.Z-rc.N`, and `latest` for the newest non-candidate), and each is **one manifest covering `linux/amd64` and `linux/arm64`**, each architecture built natively on its own runner, so the same tag works on nodes of either. `:dev` adds the Go and Node toolchains for running a suite inside the container. A published tag is never rebuilt in place.

Every published manifest carries a build-provenance attestation and an SPDX SBOM attestation in Sigstore's public log:

```sh
gh attestation verify oci://ghcr.io/headlesslab/wand:v0.1.0 -R headlesslab/wand
```

## 11. Navigations return once the document has committed

go-rod's `Page.Navigate` returns as soon as Chrome answers `Page.navigate`, which it does once the navigation is ready to commit, before the renderer has committed the new document; `NavigateBack` and `NavigateForward` return as soon as their script has run. A command bound for the renderer that is sent in between, `SetViewport` or `Emulate` say, is held by Chrome until the commit and can be lost there with no answer, a hang that only the page's context ends (#120).

wand's `Navigate`, `NavigateBack` and `NavigateForward` return once the navigation has committed: `Page.frameNavigated` where it loads a document of its own, `Page.navigatedWithinDocument` where it stays in the current one. That is the point Playwright calls `commit`, and the one Puppeteer requires before any lifecycle event; `Reload` already waited for it. What they do not wait for is unchanged: the load event is still `WaitLoad`'s, and `WaitNavigation` still takes a lifecycle event, so nothing that already waits waits twice. A navigation that turns into a download commits nothing and is a `NavigationError`; `NavigateBack` at the first entry and `NavigateForward` at the last return at once. As with every wait in wand, the bound is the page's context: `Page.Timeout`.

## Known limitations

These are real, documented, and **not** fixed by the baseline release. Each is upstream behaviour wand carried over knowingly.

| Limitation                                                                                          | Detail                                                                                                                                                                                                                                                                                                                                                                               |
| --------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Optional booleans in `lib/proto` (rod [#1196](https://github.com/go-rod/rod/issues/1196))           | Each generated struct uses `bool` with `omitempty`, so a `false` that differs from Chrome's default is never sent — `FetchRelatives: false` does not reach the browser. Changing the fields to `*bool` is an API change and belongs to the API modernization.                                                                                                                        |
| OOPIF `Element.Frame()` and `ControlURL()` (rod [#1234](https://github.com/go-rod/rod/issues/1234)) | `Element.Frame()` reaches an out-of-process iframe only with site isolation disabled, and callers that connect through `ControlURL()` hit the same gap. Proper OOPIF support is API work.                                                                                                                                                                                            |
| Ubuntu 24.04 needs `NoSandbox` (rod [#1070](https://github.com/go-rod/rod/issues/1070))             | Ubuntu 24.04 restricts unprivileged user namespaces, so a launch fails with "No usable sandbox!". `launcher.New().NoSandbox(true)` is the confirmed workaround.                                                                                                                                                                                                                      |
| Alpine's Chromium hangs (rod [#1114](https://github.com/go-rod/rod/issues/1114))                    | Alpine 3.20's Chromium package hangs on `Target.createTarget` some of the time; `--disable-gpu` helps. It is the browser build, not wand. wand's own image is `ubuntu:noble`.                                                                                                                                                                                                        |
| No Managed browser on windows/arm64, loong64 or musl                                                | No Chrome for Testing or Chromium trunk archive exists for these, so nothing is downloadable. The error names your platform; point `WAND_BROWSER_BIN` or `Launcher.Bin()` at a browser you installed.                                                                                                                                                                                |
| new-Headless semantics (rod [#818](https://github.com/go-rod/rod/issues/818))                       | Since Chrome 132 a bare `--headless` **is** new Headless, and the old headless rendering is gone from the Chrome binary. `Launcher.HeadlessNew()` is kept and means the same thing. For the old behaviour, ask for the `chrome-headless-shell` binary (`WAND_BROWSER_BINARY=chrome-headless-shell` or `Launcher.Binary()`). `--remote-debugging-address` is ignored in new Headless. |

## Domestic browsers

wand's discovery list holds Google Chrome, Chromium and Microsoft Edge only. The Chinese-market browsers — lbrowser, 奇安信可信浏览器, 360 安全浏览器信创版, UOS 浏览器, 红莲花 — are not in it, because no vendor documentation was found stating that any of them accepts `--remote-debugging-port`. If yours does, name it yourself:

```sh
export WAND_BROWSER_BIN=/opt/apps/<app-id>/files/<executable>
```

On UOS and Kylin, browsers installed from the app store live under `/opt/apps/<app id>/files/`. lbrowser ships as a Loongnix package whose file list names its executable (`dpkg -L lbrowser`).

## If something is wrong

Open an issue at [headlesslab/wand](https://github.com/headlesslab/wand/issues). A report that names the Chrome version, the platform and what go-rod did instead is the useful kind.
