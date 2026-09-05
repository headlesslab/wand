# Upstream's test suite against current Chrome stable: what breaks

- **Date:** 2026-09-05
- **Ticket:** headlesslab/wand#14 (wayfinder task ticket, child of map #1) - https://github.com/headlesslab/wand/issues/14
- **Context tickets:** headlesslab/wand#7 (harvest catalog), #8 (protocol regeneration), #10 (dependency chain), #2 (upstream survey)
- **Status:** task output, facts only. Nothing here is a decision; the acceptance-criteria ticket (#15) decides.

## Question (verbatim from #14)

> What actually breaks when upstream's code meets today's Chrome?
>
> Check out go-rod/rod at its last code commit (2024-12-07) with its pinned dependencies, run `go build ./...` and the test suite against a current Chrome stable (system-installed, not the pinned download), and record: build failures, test failures grouped by cause (protocol change, launcher, dependency, flaky), and whether the Chrome >= 136 `NewUserMode()` failure (upstream #1189) reproduces.

## Answer in one screen

Against Chrome stable **152.0.7977.76** on Windows 11, Upstream at the Snapshot commit `393ac0d`:

| What | Result |
|---|---|
| `go build ./...`, `go vet ./...` (pinned `go.sum`) | **pass** on Go 1.23.7 and 1.22.12; on Go 1.21.13 only with `GOWORK=off` (upstream's `go.work` says `go 1.22`, `go.mod` says `go 1.21`) |
| Root package (217 tests, upstream's Windows CI job) | **209 pass, 7 skip, 1 fail**, identical across 4 full runs |
| Failures caused by Chrome 152 (protocol change) | **1**: `TestPromiseLeak`. Chrome now fails an evaluation interrupted by navigation with `Inspected target navigated or closed` instead of `Execution context was destroyed.`, so `cdp.ErrCtxDestroyed` no longer matches. 5/5 fail on 152, 5/5 pass on Chromium 128. |
| Chrome >= 136 `NewUserMode()` (upstream #1189) | **reproduces**: Chrome prints `DevTools remote debugging requires a non-default data directory. Specify this using --user-data-dir.` and exits within 1.4 s; rod returns `[launcher] Failed to get the debug url: ...`. Upstream's own `TestLaunchUserMode` does not exercise this path and stays green. |
| Failures caused by dependencies | `leakless`: Windows Defender blocked the extracted `leakless.exe` once (`Trojan:Win32/Kepavll!rfn`), which killed the whole root test binary. The identical re-extracted binary ran unblocked in every later run. `use-node@latest` failed one `lib/utils` test on first install, then passed. |
| Failures independent of the Chrome version | `lib/cdp` `TestCrash` (Windows socket error is `WSAECONNRESET`, test expects `io.EOF`; fails on 128 and 152); `lib/launcher` `TestTestOpen` (Go 1.23 `os.Process` change; passes on Go 1.22.12) |
| Flaky | `TestBrowserCrash` hung once (2 min) in the Chromium 128 full run and killed the binary; 6/6 pass in isolation. `TestUseNode` failed once. |
| Did not reproduce | Windows `Page.PDF` hang (upstream #1193): `TestPagePDF` passed in every run on both browsers. New-Headless semantics (Chrome 132+): rod's bare `--headless` runs the suite green. |

Everything below is the evidence.

## Environment

| Item | Value |
|---|---|
| Upstream commit | `393ac0d60b53f3c4a9b2a6504d250cbada55b546` (2024-12-07, "feat: map session cookie expires to zero time value #1159"), the Snapshot commit per ADR-0001 |
| Dependencies | as pinned: fetchup v0.2.3, goob v0.4.0, got v0.40.0, gotrace v0.6.0, gson v0.7.3, leakless v0.9.0, gop v0.2.0 (indirect); `go.sum` untouched |
| Go | 1.23.7 windows/amd64 for all test runs; 1.22.12 and 1.21.13 pulled via `GOTOOLCHAIN` for build checks; `GOFLAGS` empty; `GOPROXY=https://goproxy.cn,direct` |
| OS / hardware | Windows 11 Pro 10.0.26200, 28 logical CPUs, 31.8 GB RAM |
| Browser under test | Google Chrome stable **152.0.7977.76**, `C:\Program Files\Google\Chrome\Application\chrome.exe`, system install, not running while tests ran |
| Control browser | upstream's pinned download, Chromium r1321438 = **128.0.6568.0**, already cached at `%APPDATA%\rod\browser\chromium-1321438` from 2024-12; the harness `init()` only validated it, no download happened |
| Antivirus | Windows Defender 4.18.26080.3, signatures 1.459.55.0 (2026-09-05), the only registered AV product |
| Not available | no C toolchain, so `-race` (upstream's Linux job) could not run; `jq` absent, `go test -json` parsed with Python |

## Method

- **Checkout**: `git clone go-rod/rod`, `git checkout 393ac0d`. Workspace mode as upstream ships it (`go.work` uses `.`, `lib/examples/custom-websocket`, `lib/examples/e2e-testing`, `lib/utils/check-issue`).
- **Build**: `go build ./...` and `go vet ./...` at the root (workspace mode covers the three extra modules).
- **Pointing the suite at the system Chrome**: the harness has no environment variable for this. `launcher.New()` seeds `flags.Bin` from `defaults.Bin`, and `lib/defaults` fills it only from the `-rod=...` CLI flag it scans in `os.Args` (`lib/defaults/defaults.go`, `ResetWith`/`parseFlag`). So every browser run was `go test ... <pkg> -args "-rod=bin=C:\Program Files\Google\Chrome\Application\chrome.exe" -timeout-each=2m`, mirroring upstream's Windows job (`test-other-platforms.yml`: `go run ./lib/utils/ci-test -timeout-each=2m -run=^Test`, which is `go test` plus `GODEBUG=tracebackancestors=100`). Runs without the `-rod=bin` flag use the cached Chromium 128 and serve as the control.
- **Packages**: upstream's Linux job tests `. ./lib/utils ./lib/proto ./lib/cdp ./lib/defaults ./lib/devices ./lib/launcher ./lib/input` (with `-race`); the Windows and macOS jobs test only the root package. All of those packages plus `lib/utils/check-issue` were run here.
- **Runs**: root package 4 full runs on Chrome 152 (3 with the harness patch below, 1 without) and 1 full run on Chromium 128; every failure then repeated in isolation (`-count=5` or `-count=3`) on both browsers. `lib/cdp` and `lib/launcher` were run **one test per `go test` invocation** so that a panic in one test cannot hide the others' results (upstream's suite panics on `MustLaunch` failures); each of those loops ran against both browsers.
- **Harness patch** (the only source change): `setup_test.go` `newTester()` gained `.Leakless(false)` after the first full run died on the Defender block described under Finding 4, and the three `lib/cdp` test files got `launcher.New().Leakless(false)` for their patched pass. Both unpatched and patched results are reported. Nothing else was modified.
- **Not run**: `go generate` (it regenerates `lib/proto` from the live browser, regenerates JS/assets, and runs `lib/utils/lint`; running it would change the Snapshot), the lint toolchain (`cspell`/`eslint`/`prettier` via `npx`, `ysmood/golangci-lint@latest`), `check-cov`, and `-race`.
- **Discarded data**: one batch where several suites ran concurrently exhausted the machine (hundreds of leaked Chrome processes, `fatal error: out of memory allocating heap arena map`, `go tool compile` exiting with `0xc0000142`). Everything from that batch was thrown away and rerun sequentially with a browser sweep between runs. The numbers below come only from clean sequential runs.
- **Sources for Chrome-side facts**: Google's remote-debugging change post (2025-03-17) https://developer.chrome.com/blog/remote-debugging-port ; Chromium `content/browser/devtools/devtools_session.cc` (`kTargetClosedMessage`); Puppeteer `packages/puppeteer-core/src/cdp/ExecutionContext.ts` (`rewriteError`); Go 1.23 `src/os/exec.go` (`handlePersistentRelease`).

## Results by package

"152" = Chrome 152 via `-rod=bin`; "128" = cached Chromium r1321438 without the flag. Root-package numbers are from full runs; `lib/cdp` and `lib/launcher` from one-test-per-invocation loops.

| Package | Tests | Chrome 152 | Chromium 128 | Failing tests and cause |
|---|---|---|---|---|
| `.` (rod) | 217 | 209 pass / 7 skip / **1 fail**, 4 of 4 runs | 207 pass / 7 skip / 0 fail, but `TestBrowserCrash` hung 2 min and the `TimeoutEach` panic killed the binary before `TestPageNavigation` and `TestElementTracing` reported | `TestPromiseLeak`: protocol change (152 only). `TestBrowserCrash`: flaky hang (128 run only) |
| `lib/cdp` | 12 | 11 pass / **1 fail** (leakless on and off) | 11 pass / 1 fail | `TestCrash`: Windows socket semantics, both browsers |
| `lib/launcher` | 23 | 20 pass / **3 fail** | 22 pass / 1 fail | `TestTestOpen`: Go 1.23 (both). `TestManaged`, `TestLaunchClient`: rod-manager rejects a `Bin` outside `%APPDATA%\rod\browser` (artefact of `-rod=bin`, pass on 128) |
| `lib/utils` | 32 | 31 pass / 1 fail first run; 32 pass on rerun | n/a (no browser) | `TestUseNode`: first-run `use-node@latest` install, flaky |
| `lib/proto`, `lib/defaults`, `lib/devices`, `lib/input` | 1 / 2 / 1 / 5 | all pass (also on Go 1.21.13) | n/a | |
| `lib/utils/check-issue` (workspace module) | 1 | pass | n/a | |

Skipped by upstream in the root package (7): `TestPageNavigateErr` ("TODO: This test on Windows is flaky"), `TestHijackMockWholeResponseNoBody` ("Because of flaky test result"), `TestNativeDrag`, `TestOldBrowser`, `TestFonts`, `TestBinarySize`, `TestLintIgnore`.

## Finding 1: build

`go build ./...` and `go vet ./...` pass with the pinned dependency graph on Go 1.23.7 (7.7 s cold) and Go 1.22.12. On Go 1.21.13, `go build ./...` stops with `go: go.work requires go >= 1.22 (running go 1.21.13; GOTOOLCHAIN=go1.21.13)`; with `GOWORK=off` the root module builds, vets, and its browser-free packages (`lib/proto`, `lib/defaults`, `lib/devices`, `lib/input`) pass on 1.21.13. So the upstream survey's "the fetchup pin breaks builds" (#2) describes the upgrade path (`fetchup.New` changed signature in v0.4.0, see #7), not the pinned build: as committed, the Snapshot compiles.

## Finding 2: the one Chrome-caused test failure, `TestPromiseLeak` (protocol change)

`page_eval_test.go:149` starts a 1 s `Runtime.evaluate` promise and navigates the page 0.3 s later, expecting the evaluation to fail with `cdp.ErrCtxDestroyed`:

```
page_eval_test.go:161:
    &cdp.Error{Code: -32000, Message: "Inspected target navigated or closed", Data: ""}
    ⦗should in chain of⦘
    &cdp.Error{Code: -32000, Message: "Execution context was destroyed.", Data: ""}
--- FAIL: TestPromiseLeak (0.65s)
```

- Deterministic: 5/5 fail on Chrome 152 (`-count=5 -run 'TestPromiseLeak$'`), 5/5 pass on Chromium 128, and it is the only failure in all four full runs on 152.
- `cdp.ErrCtxDestroyed` (`lib/cdp/error.go:44`) matches on the message text `Execution context was destroyed.`. `Inspected target navigated or closed` is Chromium's session-level `kTargetClosedMessage` in `content/browser/devtools/devtools_session.cc`, annotated "Clients match against this error message verbatim (http://crbug.com/1001678)". Chrome 152 fails the pending command at the session level when the navigation happens; Chromium 128 let V8's context-destroyed path fail it. (A plausible mechanism is RenderDocument giving every navigation a new RenderFrameHost; not verified here.)
- Blast radius inside rod: `grep` shows no non-test code matches `ErrCtxDestroyed`; the core retries on `ErrCtxNotFound` (`page_eval.go:130`, `query.go:283`), `ErrSearchSessionNotFound` (`query.go:284`) and `ErrNotAttachedToActivePage` (`page.go:328`). So on 152 the behaviour change is (a) this test and (b) any user code that does `errors.Is(err, cdp.ErrCtxDestroyed)`. Puppeteer's `ExecutionContext.ts` `rewriteError` already treats `Cannot find context with specified id` and `Inspected target navigated or closed` as the same "Execution context was destroyed" condition.

## Finding 3: Chrome >= 136 `NewUserMode()` reproduces (launcher)

Program (in the checkout, `go run`): `launcher.NewUserMode().Context(ctx20s).Logger(w).Launch()` with `Bin` left to `LookPath()`, which resolved `C:\Program Files\Google\Chrome\Application\chrome.exe`. Chrome was not running.

```
bin:  C:\Program Files\Google\Chrome\Application\chrome.exe
args: [--no-startup-window --remote-debugging-port=37712]

DevTools remote debugging requires a non-default data directory. Specify this using --user-data-dir.
Created TensorFlow Lite XNNPACK delegate for CPU.

--- result ---
elapsed=1.376s
url=""
err=[launcher] Failed to get the debug url:
DevTools remote debugging requires a non-default data directory. Specify this using --user-data-dir.
Created TensorFlow Lite XNNPACK delegate for CPU.
```

- Same message as upstream #1189 (reported 2025-05-17 against Chrome 136 on Linux). Chrome exits by itself, so the failure surfaces through `Launcher.getURL()`'s `<-l.exit` branch as `parser.Err()`, within 1.4 s, no hang, and no Chrome process is left behind (0 `chrome.exe` afterwards). Google's rule (blog post of 2025-03-17): since Chrome 136 `--remote-debugging-port`/`--remote-debugging-pipe` are ignored for the default user-data directory; a non-default `--user-data-dir` is required; for automation Google recommends Chrome for Testing.
- **Upstream's suite does not catch this.** `lib/launcher/launcher_test.go` `TestLaunchUserMode` sets `.Bin("")` and `.Revision(launcher.RevisionDefault)`, so `getBin()` falls through to the downloaded Chromium 128 and launches it headless with `--remote-debugging-port=58472`; the second `launcher.NewUserMode().RemoteDebuggingPort(port).MustLaunch()` in the same test finds that port already answering and never starts a browser. It passed here against both browsers. `TestUserModeErr` only checks non-existent binaries. Result: green suite, broken feature.
- Corollary from the same code: `NewUserMode()` is the only launcher path without a `--user-data-dir`; `launcher.New()` always sets one (`%TEMP%\rod\user-data\<random>`), which is why the rest of the suite is unaffected by the Chrome 136 rule.

## Finding 4: dependencies

### `leakless` v0.9.0 (Windows Defender)

The very first full run of the root package (unpatched harness) died in 2.7 s:

```
panic: fork/exec C:\Users\...\AppData\Local\Temp\leakless-amd64-adb80298fa6a3af7ced8b1c9b5f18007\leakless.exe:
  Operation did not complete successfully because the file contains a virus or potentially unwanted software.
  (syscall.Errno 225)
  github.com/go-rod/rod/lib/launcher.(*Launcher).MustLaunch  lib/launcher/launcher.go:411
  github.com/go-rod/rod_test.newTester                       setup_test.go:92
  github.com/go-rod/rod.Pool[...].MustGet                    must.go:1169
```

- Defender's record: `Get-MpThreatDetection` shows one detection at 18:51:37, process `rod.test.exe`, resource `leakless.exe`, ThreatID 2147939874 = **`Trojan:Win32/Kepavll!rfn`** (severity 5). The file was removed.
- Not deterministic here: leakless re-extracted the identical binary (`GetLeaklessBin()`, sha256 `1D11BEC16A63D85A386C47FB97914AD13318F7FDCBF399C5A895CF642307F115`, 2,147,840 bytes) about 3 minutes later and it executed without complaint in every subsequent leakless-enabled run, including a probe that deleted the extracted file and ran `TestLaunch` twice, and a later full unpatched root run (209/7/1, same as patched). Only one detection is on record for the whole session.
- Impact when it does hit: `launcher.New()` enables leakless by default, `newTester()` uses `MustLaunch()`, and the first `MustLaunch` panic inside the tester pool takes the entire test binary down (4 tests marked failed, the rest never ran). Nothing in the harness can turn leakless off from the outside (`lib/defaults` has no option for it), hence the one-line `.Leakless(false)` patch.
- What leakless was protecting: `TestMain` closes the pooled browsers only when `m.Run()` returns 0 (`setup_test.go:44-52`, `os.Exit(code)` comes first). Every failing run therefore depends on leakless to reap 28 browsers. With leakless disabled, each failing full run left 28 browser main processes (about 200 `chrome.exe` including renderers) and their `%TEMP%\rod\user-data\*` directories behind; 116 such directories had accumulated by the end of this session. With leakless enabled the reaping works but is not instant: after a failing run all 28 pooled browsers were still alive 3 s after the test binary exited, and all 128 `chrome.exe` processes were gone 20 s later without any external kill.

### `use-node@latest` (`lib/utils`)

`TestUseNode` runs `go run github.com/ysmood/use-node@latest -p v20` and asserts that `npx` then resolves inside the downloaded `v20` tree. On the first run on this machine (9.6 s, the install of `v20.20.2` into `%APPDATA%\use-node`) `exec.LookPath("npx")` still resolved the system Node's `F:\Program Files\nodejs\npx.cmd` (Node v20.15.0) and the test failed; two immediate reruns passed. Unpinned `@latest`, first-run timing, and a system Node on `PATH` are the ingredients; nothing to do with Chrome.

### Download hosts for the pinned browser (context)

`HEAD` on upstream's three hosts for r1321438 `Win_x64/chrome-win.zip`: Google `storage.googleapis.com` 200 (267,483,258 bytes), `registry.npmmirror.com` 200 (same size), Playwright `playwright.azureedge.net/builds/chromium/1321438/chromium-linux-arm64.zip` 400 and the hard-coded `RevisionPlaywright = 1124` also 400 (confirms #21). The harness never downloaded because the cached copy validated.

## Finding 5: Windows-only failures independent of the Chrome version

### `lib/cdp` `TestCrash`

Fails identically on Chromium 128 and Chrome 152 (5 runs each, leakless on and off):

```
client_test.go:171:
    &net.OpError{Op: "read", Net: "tcp", ..., Err: &os.SyscallError{Syscall: "wsarecv", Err: syscall.Errno(10054)}}
    ⦗not ==⦘
    &errors.errorString{s: "EOF"}
```

After `Browser.crash`, the test expects the hand-rolled WebSocket client to return `io.EOF` for the in-flight `Runtime.evaluate`. Windows closes the socket with `WSAECONNRESET` (10054) instead. Upstream's CI runs `lib/cdp` only in the Linux job, so this never showed there.

### Upstream's own Windows skips

Seven tests skip in every run (list under Results by package); two of them carry explicit "flaky on Windows" reasons.

## Finding 6: Go 1.23 toolchain, independent of Chrome

`lib/launcher/private_test.go:163` `TestTestOpen` stubs `openExec` to return an `exec.Cmd` with `Process = &os.Process{}`; `launcher.Open()` then calls `Process.Release()`.

```
panic: handlePersistentRelease called in invalid mode     (Go 1.23.7, src/os/exec.go:196)
ok    github.com/go-rod/rod/lib/launcher    0.149s          (GOTOOLCHAIN=go1.22.12)
```

Go 1.23 gave `os.Process` an internal mode (pid vs handle) and a zero-value `Process` has neither; upstream's CI pins Go 1.22 and never saw it. It fails on both browsers because no browser is involved.

## Finding 7: artefacts of pointing the suite at a system Chrome

`TestManaged` and `TestLaunchClient` (`lib/launcher/private_test.go`) fail only with `-rod=bin=<system Chrome>`:

```
panic: websocket bad handshake: 400 Bad Request. [rod-manager] not allowed rod-bin path:
  C:\Program Files\Google\Chrome\Application\chrome.exe (use --allow-all to disable the protection)
```

`manager.go:131-147` allows `flags.Bin` only under `DefaultBrowserDir` (`%APPDATA%\rod\browser`) unless `--allow-all`. Both tests pass without the flag. Relevant beyond the suite: any change of the default browser location (for example a Chrome for Testing directory) moves this whitelist too.

## Finding 8: flaky

- `TestBrowserCrash` (`browser_test.go:164`): in the single full Chromium 128 run it hung until `TimeoutEach` (2 min) fired: `panic: [rod_test.TimeoutEach] TestBrowserCrash timeout after 2m0s`, with the goroutine dump pointing at `leakless.(*Launcher).serve`; the panic killed the binary, so `TestPageNavigation` and `TestElementTracing` have no result in that run. In isolation it passed 3/3 on 128 and 3/3 on 152, and it passed in all four full 152 runs (including the leakless-enabled one). One hang in six full runs.
- `TestUseNode`: see Finding 4.
- `TestWaitInvisible` failed once (5.6 s) only inside the discarded resource-exhaustion batch and never in a clean run; recorded for completeness, not counted.

## Finding 9: did not reproduce

- **Windows `Page.PDF` hang** (upstream #1193, harvest catalog row 23, suggested fix `--disable-features=PrintCompositorLPAC`): `TestPagePDF` (`page_test.go:889`, `PDF` + `MustPDF` on `fixtures/click.html`) passed in every run on both browsers. The suite's fixture is tiny; the report's conditions (specific Windows sandbox configuration) were not otherwise exercised.
- **New Headless**: rod's default flag is bare `--headless`, which on Chrome 152 is new Headless (old Headless left the Chrome binary in 132). 209 tests pass under it; no test depended on old-Headless behaviour.
- **Protocol removals**: no test failed on a missing CDP command or event; consistent with #8's finding that none of the 178 schema symbols rod's core uses was removed between r1321438 and M153.

## Example tests and the e2e-testing module

Upstream runs these only in the nightly `check-examples.yml` (`go test -run Example ./...` after `go run ./lib/utils/get-browser`), not on push. Here each root-package `Example` ran in its own `go test` invocation with a 60 s timeout against Chrome 152 (each example launches its own browser through `rod.New().MustConnect()`, leakless on).

| Result | Examples |
|---|---|
| pass (6) | `Example_wait_for_animation`, `Example_wait_for_request`, `Example_handle_events`, `Example_hijack_requests`, `Example_states`, `ExamplePage_pool` |
| timeout after 60 s, live site (2) | `Example_basic` (navigates `github.com` and searches), `Example_search` (`github.com`) |
| fail, live-site drift (1) | `Example_customize_retry_strategy`: `github.com`'s search input is now `name="source"`, the example wants `type` (same class as upstream PR #1227, classified "drop" in #7) |
| not executed (15) | no `// Output:` comment, so `go test` only compiles them: `Example_disable_headless_to_debug`, `Example_context_and_timeout`, `Example_context_and_EachEvent`, `Example_error_handling`, `Example_page_screenshot`, `Example_page_scroll_screenshot`, `Example_page_pdf`, `Example_race_selectors`, `Example_customize_browser_launch`, `Example_direct_cdp`, `Example_download_file`, `Example_eval_reuse_remote_object`, `ExampleBrowser_pool`, `Example_load_extension`, `Example_log_cdp_traffic` |

`lib/cdp` `ExampleClient` passes. The `lib/examples/e2e-testing` workspace module (`TestAdd`, `TestMultiple`, against a local fixture) passes against Chrome 152 in 1.7 s. None of the three non-passing examples involves Chrome behaviour; all three depend on `github.com` reachability and page content from this network.

## What this leaves for #15 (facts, not decisions)

- "Builds and passes its tests against current Chrome stable" is, on Windows, one assertion away for the root package (`TestPromiseLeak`) once the harness can start browsers; the other red tests are Windows socket semantics (`TestCrash`), Go 1.23 (`TestTestOpen`), and the rod-manager whitelist (`TestManaged`, `TestLaunchClient` under `-rod=bin`).
- The Chrome >= 136 `NewUserMode()` breakage is real, immediate (no hang), and invisible to upstream's suite; a fix needs a test that launches a real non-Chromium-download browser in user mode, or a unit test on the flag set.
- The harness's only knob for the browser under test is the `-rod=bin` CLI flag; there is no environment variable, and `init()` still validates (and would download) the pinned Chromium even when `-rod=bin` is given.
- Cleanup of pooled browsers on a failing run is delegated entirely to leakless (`TestMain` exits before `Cleanup` when any test fails), so the leakless decision in #11 is also a test-harness decision.
- `go.work` (go 1.22) and `go.mod` (go 1.21) disagree; Go 1.21 builds only with `GOWORK=off`.
- Linux and macOS were not exercised here; upstream's Linux job additionally runs `-race` and the six `lib/*` packages.
