# How Playwright and Puppeteer keep the browser from outliving the driver

- Date: 2026-09-05
- Ticket: headlesslab/wand#11 (research input; child of #1). Background for the `ysmood/leakless` row of `docs/research/dependency-chain.md`.
- Author: research pass over primary sources only — upstream source on GitHub, Chromium source at chromium.googlesource.com, Node.js and libuv source, Go source and issue tracker. Nothing was executed; every claim below is read out of source or official documentation.

## Question (as posed)

> How do **Playwright** and **Puppeteer** make sure the Chrome/Chromium process they launch does not outlive the driver process (no orphaned/leaked browser), on Linux, macOS and Windows? go-rod relies on `ysmood/leakless` (a dropped-to-temp guard executable that kills the browser when the Go parent dies); wand is deciding whether to replace it, so we need the exact mechanisms the two mainstream drivers use, with primary-source citations.
>
> Cover transport (`--remote-debugging-pipe` default or opt-in; does Chromium exit on pipe EOF), normal-exit paths (signal handlers, process-group kill, `taskkill`, temp-dir cleanup), the parent-hard-killed case, whether either project spawns a guard binary, comparators (chromedp, chromedriver, other Chrome switches), and Go feasibility facts.
>
> Output a per-driver x per-platform table with the "parent SIGKILLed" outcome. Do not decide; report facts.

This document states facts. It does not recommend.

---

## Short answers

**(1) Transport.** Playwright always launches Chromium with `--remote-debugging-pipe` and always talks CDP over fds 3/4 — there is no option to turn it off, and passing the flag yourself is rejected. Puppeteer defaults to `--remote-debugging-port=0` (WebSocket) and uses the pipe only when the caller passes `pipe: true` (`@defaultValue false`). Chromium **does** shut the whole browser down when the read end of that pipe reaches EOF: the reader thread treats `read() <= 0` as an error, posts `DevToolsPipeHandler::OnDisconnect`, and the `on_disconnect` closure that Chrome installs is `ChromeDevToolsManagerDelegate::CloseBrowserSoon`, which calls `chrome::ExitIgnoreUnloadHandlers()`; the headless shell installs `HeadlessBrowserImpl::Shutdown`. The same code path runs on Windows (`ReadFile` failure), where the fds are obtained with `_get_osfhandle(3)` / `_get_osfhandle(4)`. There is no equivalent closure for `--remote-debugging-port`: closing a DevTools WebSocket does not stop the browser.

Three qualifiers on that, each load-bearing for a Go driver:

- **It is an M89 feature.** Before Chromium 89 the pipe handler took no disconnect closure and the browser did *not* exit on EOF. The CL that added it says so plainly: "When the browser is controlled remotely over the pipe, and the pipe is disconnected, there is no way left to control the browser. **Every pipe client tries to kill the browser at that point.** Instead, we can just shutdown normally, leaving no core dumps."
- **Only the read end counts.** Chromium `SIG_IGN`s `SIGPIPE` process-wide, and `PipeWriterBase::WriteBytes` on failure only does `LOG(ERROR) << "Could not write into pipe";` — no disconnect, no exit. To make Chrome exit you must close the end *Chrome reads from* (its fd 3). Closing only the parent's read end does nothing.
- **The closure is per-embedder.** Chrome and `chrome-headless-shell` exit; `content_shell` passes `base::OnceClosure()` (null) and keeps running.

**(3) Parent hard-killed (SIGKILL / crash / OOM).** Two independent things save these drivers, and neither is JavaScript:

- **Windows**: libuv — hence Node, hence both drivers — puts every non-`detached` child into a process-wide job object created with `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`, explicitly "so windows will kill it when the parent process dies". Both drivers pass `detached: false` on Windows. So on Windows the browser is killed by the kernel when Node dies, whatever the transport, with no driver code involved.
- **Linux / macOS**: both drivers pass `detached: true`, so the browser is the leader of its own process group *and session* — it receives no signal when Node dies and is reparented to init. The **only** thing that reaps it is pipe EOF. That means Playwright is safe (it always uses the pipe), Puppeteer is safe **only** if the caller passed `pipe: true`, and Puppeteer's default WebSocket configuration **leaks the browser** — Chrome keeps running with its `about:blank` startup window. In every hard-kill case the temp user-data-dir and artifacts directories are leaked on disk, because the cleanup code lives in the JavaScript exit handler that never ran.

**(4) Guard/watchdog process.** Neither project spawns one. Each does a single `child_process.spawn` of the browser executable, plus a synchronous `taskkill` on Windows during teardown. `playwright-core/bin/` contains only `.sh`/`.ps1` install scripts, and `puppeteer-core/src/node/` contains only launchers and transports — no shipped helper binary anywhere in the launch path. The nearest thing to leakless in the Playwright tree is the **language-binding driver**: for the Python/Java/.NET clients, `playwright.sh run-driver` is a Node process that speaks the protocol over its own stdin/stdout and calls `gracefullyProcessExitDoNotHang(0)` when that pipe closes — but it is the protocol transport, not an extra guard process, and it does not exist for the Node.js API.

---

## Per-driver x per-platform table

Rows are the three real configurations. "Parent SIGKILLed" means the driver process is destroyed without running any handler (SIGKILL, hard crash, OOM killer, `TerminateProcess`).

| Configuration | Transport | Spawn flags | Normal close (`browser.close()`) | `process.on('exit')` | SIGINT / SIGTERM / SIGHUP | Parent SIGKILLed — **Linux** | Parent SIGKILLed — **macOS** | Parent SIGKILLed — **Windows** | Temp dirs after hard kill |
|---|---|---|---|---|---|---|---|---|---|
| **Playwright** (`chromium.launch()`, all platforms) | `--remote-debugging-pipe` **always**; CDP `\0`-framed on stdio fd 3/4 | `detached: process.platform !== 'win32'`; `stdio: ['ignore','pipe','pipe','pipe','pipe']` | CDP `Browser.close`, then on timeout `kill()` | always registered: `killProcessAndCleanup()` -> POSIX `process.kill(-pid,'SIGKILL')` / Win `taskkill /pid <pid> /T /F` + synchronous `fs.rmSync` of temp dirs | all three default **true**; SIGINT -> `gracefullyCloseAll()` then `process.exit(130)`, second Ctrl-C force-kills; SIGTERM/SIGHUP -> `gracefullyCloseAll()` | **Browser exits.** fds 3/4 close -> Chromium `read()` returns 0 -> `OnDisconnect` -> `chrome::ExitIgnoreUnloadHandlers()` (Chrome) / `HeadlessBrowserImpl::Shutdown` (headless shell) | same as Linux (identical code path, no `IS_MAC` branch) | **Browser killed twice over**: libuv job object `KILL_ON_JOB_CLOSE` fires immediately; the pipe-EOF path would also fire | **leaked** (`playwright_chromiumdev_profile-*`, `playwright-artifacts-*` in `os.tmpdir()`) |
| **Puppeteer, default** (`pipe` unset -> `false`) | `--remote-debugging-port=0`, WebSocket from the `DevTools listening on ws://...` stderr line | `detached ??= process.platform !== 'win32'`; `stdio: ['pipe','pipe','pipe']` | CDP `Browser.close` via `cdpConnection.closeBrowser()`, then `browserProcess.close()` on error | `#onDriverProcessExit -> this.kill()` -> POSIX `process.kill(-pid,'SIGKILL')` / Win `execSync('taskkill /pid <pid> /T /F')`. **No temp-dir removal in this path** | all three default **true**; SIGINT -> `kill()` + `process.exit(130)`; SIGTERM/SIGHUP -> `close()` (runs the `onExit` hook, which removes a temp profile, then kills) | **Browser LEAKS.** No signal reaches the new session; the WebSocket simply closes and Chromium has no shutdown closure for the HTTP/WS handler; the `about:blank` startup window keeps the browser alive; process reparented to init | same as Linux — leaks | **Browser killed** by the libuv job object | **leaked** (`puppeteer_dev_chrome_profile-*`) |
| **Puppeteer, `pipe: true`** | `--remote-debugging-pipe`; CDP `\0`-framed on fd 3/4 | as above but `stdio: ['pipe','pipe','pipe','pipe','pipe']` | as above | as above | as above | **Browser exits** via pipe EOF, same Chromium path as Playwright | same as Linux | **Browser killed** by the job object (pipe EOF also applies) | **leaked** |

Cross-cutting caveats that apply to every row:

- The Windows job-object row depends on `AssignProcessToJobObject` succeeding. libuv deliberately swallows `ERROR_ACCESS_DENIED` and then "continue[s] without establishing a kill-child-on-parent-exit relationship", which happens when the Node process itself is inside a job that forbids breakaway on a Windows version without nested-job support. libuv also sets `JOB_OBJECT_LIMIT_SILENT_BREAKAWAY_OK`, so only the browser process itself is in the job — Chromium's own child processes are not. They are covered separately by Chromium's sandbox, which puts each target in a job object that also carries `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` and whose handle the browser process holds; `--no-sandbox` (Playwright's default unless `chromiumSandbox: true`) bypasses that.
- On branded Google Chrome for Win/Mac/Linux, `--remote-debugging-pipe` is refused entirely when the default user-data-dir is in use (`NotStartedReason::kDisabledByDefaultUserDataDir`), and by the `DevToolsRemoteDebuggingAllowed` policy. Both drivers always pass an explicit `--user-data-dir`, so they never hit the first gate.
- Every "Browser exits via pipe EOF" cell requires **Chromium M89 or newer** (see 3.4). On M88 and earlier the pipe handler had no disconnect closure and the browser survived.
- Neither driver arms anything for the case where the *browser* is killed and the driver survives; that is handled by the `close`/`exit` event on the child, which is a different problem.

---

## 1. Playwright

Files (all `microsoft/playwright`, branch `main`): `packages/utils/processLauncher.ts`, `packages/playwright-core/src/server/browserType.ts`, `packages/playwright-core/src/server/chromium/chromium.ts`, `packages/playwright-core/src/server/pipeTransport.ts`, `packages/playwright-core/src/cli/driver.ts`.

### 1.1 The pipe is not optional

`ChromiumBrowserType.defaultArgs()` unconditionally appends the flag, and rejects a caller who tries to supply it:

```ts
chromeArguments.push(`--user-data-dir=${userDataDir}`);
chromeArguments.push('--remote-debugging-pipe');
...
if (args.find(arg => arg.startsWith('--remote-debugging-pipe')))
  throw new Error('Playwright manages remote debugging connection itself.');
```

`BrowserType._launchProcess` always passes `stdio: 'pipe'`, which `launchProcess` expands to five entries so that fds 3 and 4 exist, and then binds the transport to `stdio[3]` / `stdio[4]`:

```ts
const stdio: ('ignore' | 'pipe')[] = options.stdio === 'pipe'
  ? ['ignore', 'pipe', 'pipe', 'pipe', 'pipe'] : ['pipe', 'pipe', 'pipe'];
...
if (!this.supportsPipeTransport(options)) {
  transport = await WebSocketTransport.connect(progress, wsEndpoint!);
} else {
  const stdio = launchedProcess.stdio as unknown as [...];
  transport = new PipeTransport(stdio[3], stdio[4]);
}
```

`BrowserType.supportsPipeTransport()` returns `true` by default and is overridden only in `webkit.ts`, `bidi/bidiFirefox.ts` and `bidi/bidiChromium.ts` — never in `chromium/chromium.ts`. So the CDP Chromium path is pipe-only. `server/pipeTransport.ts` writes `JSON.stringify(message)` followed by `'\0'`, matching Chromium's default `kASCIIZ` protocol mode.

`--remote-debugging-port` is used in exactly two places that are not the normal launch: `launchWithSeleniumHub()` (`args.push('--remote-debugging-port=0')`, then CDP over the hub's WebSocket) and `waitForReadyState()`, which only watches for the `DevTools listening on` line if the *user* put a `--remote-debugging-port` in `args`.

### 1.2 Spawn flags and process groups

```ts
const spawnOptions: childProcess.SpawnOptions = {
  // On non-windows platforms, `detached: true` makes child process a leader of a new
  // process group, making it possible to kill child process tree with `.kill(-pid)` command.
  detached: process.platform !== 'win32',
  ...
};
const spawnedProcess = childProcess.spawn(options.command, options.args || [], spawnOptions);
```

### 1.3 Exit and signal handling

`launchProcess` registers `'exit'` unconditionally and the three signals per option (`handleSIGINT`/`handleSIGTERM`/`handleSIGHUP`, all documented as "Defaults to `true`" and defaulted to `true` in `_launchProcess`):

```ts
addProcessHandlerIfNeeded('exit');
if (options.handleSIGINT) addProcessHandlerIfNeeded('SIGINT');
if (options.handleSIGTERM) addProcessHandlerIfNeeded('SIGTERM');
if (options.handleSIGHUP) addProcessHandlerIfNeeded('SIGHUP');
```

- `exit` -> `killProcessAndCleanup()` for every live browser: force-kill plus a synchronous `fs.rmSync(dir, {force: true, recursive: true, maxRetries: 5})` of each temp directory. It must be synchronous because Node's `'exit'` handler cannot await.
- `SIGINT` -> `gracefullyCloseAll()` then `process.exit(130)`. A **second** Ctrl-C removes the handler and force-kills immediately ("This prevents hanging in the case where closing the browser takes a lot of time or is buggy").
- `SIGTERM` / `SIGHUP` -> `gracefullyCloseAll()`.

The force-kill itself is the platform split:

```ts
if (process.platform === 'win32') {
  const taskkillProcess = childProcess.spawnSync(`taskkill /pid ${spawnedProcess.pid} /T /F`, { shell: true, windowsHide: true });
  ...
} else {
  process.kill(-spawnedProcess.pid, 'SIGKILL');
}
```

Graceful close is CDP `Browser.close` over the same transport (`attemptToGracefullyCloseBrowser`); `BrowserProcess.close` is `closeOrKill(timeout)`, which races the graceful close against a timer and falls back to `kill()`.

### 1.4 Temp directories

`_prepareToLaunch` creates `os.tmpdir()/playwright-artifacts-*` and, when no `userDataDir` was supplied, `os.tmpdir()/playwright_${name}dev_profile-*`, and pushes both into `tempDirectories`. They are removed on the child's `'close'` event (async `removeFolders`) and in the `'exit'` handler (sync `rmSync`). Neither runs if the Node process is SIGKILLed.

### 1.5 The language-binding driver (the closest thing to a guard)

For the Python/Java/.NET bindings the client process spawns `playwright.sh run-driver`, a Node process that speaks the Playwright protocol over **its own stdin/stdout** with a length-prefixed `PipeTransport`. When the client dies, that pipe closes:

```ts
transport.onclose = () => {
  dispatcherConnection.onmessage = () => {};
  gracefullyProcessExitDoNotHang(0);
};
// Ignore the SIGINT signal in the driver process so the parent can gracefully close the connection.
// We still will destruct everything (close browsers and exit) when the transport pipe closes.
process.on('SIGINT', () => { /* Keep the process running. */ });
```

`gracefullyProcessExitDoNotHang` calls `gracefullyCloseAll()` and forces `process.exit(code)` after 30 s. This is a *stdin-EOF* guard on the driver, one layer above the browser, and it exists only because the non-JS bindings are out-of-process. It is not spawned for `require('playwright')`.

The same stdin-EOF idea is used by Playwright's MCP server (`process.stdin.on('end', ...)` plus an exit watchdog); see microsoft/playwright#41664 and #41013 for the maintainers' position that a client which neither closes stdio nor signals leaves the server — and its browser — running.

---

## 2. Puppeteer

Files (all `puppeteer/puppeteer`, branch `main`): `packages/browsers/src/launch.ts`, `packages/puppeteer-core/src/node/ChromeLauncher.ts`, `packages/puppeteer-core/src/node/BrowserLauncher.ts`, `packages/puppeteer-core/src/node/PipeTransport.ts`, `packages/puppeteer-core/src/node/LaunchOptions.ts`, `packages/puppeteer-core/src/node/util/fs.ts`.

### 2.1 The pipe is opt-in

`LaunchOptions.pipe` is documented as "Connect to a browser over a pipe instead of a WebSocket. Only supported with Chrome. `@defaultValue false`". `ChromeLauncher.computeLaunchArguments` picks the transport:

```ts
const { ignoreDefaultArgs = false, args = [], pipe = false, debuggingPort, ... } = options;
...
if (!chromeArguments.some(argument => argument.startsWith('--remote-debugging-'))) {
  if (pipe) {
    assert(!debuggingPort, 'Browser should be launched with either pipe or debugging port - not both.');
    chromeArguments.push('--remote-debugging-pipe');
  } else {
    chromeArguments.push(`--remote-debugging-port=${debuggingPort || 0}`);
  }
}
```

`BrowserLauncher.launch` then derives everything from the produced argv — `const usePipe = launchArgs.args.includes('--remote-debugging-pipe');` — and passes `pipe: usePipe` down to `@puppeteer/browsers`' `launch()`. The `Process` constructor turns that into the stdio shape:

```ts
#configureStdio(opts: {pipe: boolean}): Array<'ignore' | 'pipe'> {
  if (opts.pipe) return ['pipe', 'pipe', 'pipe', 'pipe', 'pipe'];
  else return ['pipe', 'pipe', 'pipe'];
}
```

and `createCdpPipeConnection` binds `{3: pipeWrite, 4: pipeRead}` to a `PipeTransport` that frames with `'\0'`. The default path is `createCdpSocketConnection`, which scrapes `CDP_WEBSOCKET_ENDPOINT_REGEX = /^DevTools listening on (ws:\/\/.*)$/` out of the browser's stderr.

`defaultArgs()` always appends `about:blank` when no positional URL was given, so the default browser has a real startup window (contrast with Playwright's `--no-startup-window` for non-persistent launches).

### 2.2 Spawn flags, signals, kill

```ts
opts.pipe ??= false;
opts.handleSIGINT ??= true;
opts.handleSIGTERM ??= true;
opts.handleSIGHUP ??= true;
// On non-windows platforms, `detached: true` makes child process a
// leader of a new process group, making it possible to kill child
// process tree with `.kill(-pid)` command.
opts.detached ??= process.platform !== 'win32';
```

Handlers, all registered through a shared dispatcher so a single `process.on(...)` serves N browsers:

```ts
#onDriverProcessExit = (_code: number) => { this.kill(); };

#onDriverProcessSignal = (signal: string): void => {
  switch (signal) {
    case 'SIGINT':  this.kill(); process.exit(130);
    case 'SIGTERM':
    case 'SIGHUP':  void this.close(); break;
  }
};
```

`kill()` checks `pidExists(pid)` (`process.kill(pid, 0)`), then:

```ts
if (process.platform === 'win32') {
  try { childProcess.execSync(`taskkill /pid ${this.#browserProcess.pid} /T /F`); }
  catch (error) { /* ... */ this.#browserProcess.kill(); }
} else {
  const processGroupId = -this.#browserProcess.pid;
  try { process.kill(processGroupId, 'SIGKILL'); }
  catch (error) { /* ... */ this.#browserProcess.kill('SIGKILL'); }
}
```

The fallbacks are annotated "This delays killing of all child processes of `this.proc` until the main Node.js process dies", and a total failure throws `PROCESS_ERROR_EXPLANATION` ("Puppeteer was unable to kill the process which ran the browser binary...").

`Process.close()` runs the `onExit` hook (once) and then `kill()` if the child has not exited. `BrowserLauncher.closeBrowser()` prefers the graceful route — `cdpConnection.closeBrowser()` (CDP `Browser.close`) followed by `browserProcess.hasClosed()` — and falls back to `browserProcess.close()`; with no CDP connection it waits 5 s before closing.

### 2.3 Temp directory

`ChromeLauncher.computeLaunchArguments` creates the profile with `mkdtemp(await this.getProfilePath())` and marks `isTempUserDataDir`. Removal is wired as the `onExit` hook:

```ts
const onProcessExit = async () => {
  await this.cleanUserDataDir(launchArgs.userDataDir, {isTemp: launchArgs.isTempUserDataDir});
};
```

`cleanUserDataDir` calls `rm(path)` = `fs.promises.rm(path, {force: true, recursive: true, maxRetries: 10, retryDelay: 100})`. The hook runs on the browser's `'exit'` event and inside `Process.close()`. Note that the `process.on('exit')` handler is `this.kill()` only — Puppeteer does **not** do a synchronous temp-dir sweep at driver exit the way Playwright does.

---

## 3. What Chromium actually does with the pipe

Switch definition, `content/public/common/content_switches.cc`:

```cpp
// Enables remote debug over stdio pipes [in=3, out=4] or over the remote pipes
// specified in the 'remote-debugging-io-pipes' switch.
// Optionally, specifies the format for the protocol messages, can be either
// "JSON" (the default) or "CBOR".
const char kRemoteDebuggingPipe[] = "remote-debugging-pipe";
```

The fd numbers are constants on the public API, `content/public/browser/devtools_agent_host.h`:

```cpp
// File descriptor used by DevTools remote debugging pipe handler
// to read and write protocol messages.
static constexpr int kReadFD = 3;
static constexpr int kWriteFD = 4;
...
// Starts remote debugging for browser target for the given fd=3
// for reading and fd=4 for writing remote debugging messages.
static void StartRemoteDebuggingPipeHandler(base::OnceClosure on_disconnect);
```

### 3.1 EOF -> disconnect

`content/browser/devtools/devtools_pipe_handler.cc`, `PipeReaderBase::ReadBytes` (one reader thread doing blocking reads):

```cpp
#if BUILDFLAG(IS_WIN)
      DWORD size_read = 0;
      bool had_error = UNSAFE_TODO(
          !ReadFile(read_handle_, static_cast<char*>(buffer) + bytes_read,
                    size - bytes_read, &size_read, nullptr));
#else
      int size_read = UNSAFE_TODO(read(read_fd_, ...));
      if (size_read < 0 && errno == EINTR) continue;
      bool had_error = size_read <= 0;
#endif
      if (had_error) {
        if (!shutting_down_.IsSet()) {
          LOG(ERROR) << "Connection terminated while reading from pipe";
          GetUIThreadTaskRunner({})->PostTask(
              FROM_HERE, base::BindOnce(&DevToolsPipeHandler::OnDisconnect,
                                        devtools_handler_));
        }
        return 0;
      }
```

`size_read == 0` is exactly the POSIX EOF that the kernel produces once the last writer fd is closed — which is what happens when the parent process dies, however it dies. On Windows the corresponding event is `ReadFile` failing with `ERROR_BROKEN_PIPE`. `ReadLoop()` then also posts `DevToolsPipeHandler::Shutdown`. `OnDisconnect` runs the embedder-supplied closure:

```cpp
void DevToolsPipeHandler::OnDisconnect() {
  if (on_disconnect_) std::move(on_disconnect_).Run();
}
```

### 3.2 The closure: Chrome exits

`chrome/browser/devtools/remote_debugging_server.cc`:

```cpp
void RemoteDebuggingServer::StartPipeHandler() {
  content::DevToolsAgentHost::StartRemoteDebuggingPipeHandler(
      base::BindOnce(&ChromeDevToolsManagerDelegate::CloseBrowserSoon));
}
```

`chrome/browser/devtools/chrome_devtools_manager_delegate.cc`:

```cpp
// static
void ChromeDevToolsManagerDelegate::CloseBrowserSoon() {
  content::GetUIThreadTaskRunner({})->PostTask(
      FROM_HERE, base::BindOnce([]() {
        // Do not keep the application running anymore, we got an explicit
        // request to close.
        AllowBrowserToClose();
        chrome::ExitIgnoreUnloadHandlers();
      }));
}
```

The old headless shell (`chrome-headless-shell` / `chromium-headless-shell`, which is what Playwright launches for `headless: true` and Puppeteer for `headless: 'shell'`) installs its own closure, `headless/lib/browser/headless_devtools.cc`:

```cpp
void StartLocalDevToolsHttpHandler(HeadlessBrowserImpl* browser) {
  HeadlessBrowser::Options* options = browser->options();
  if (options->devtools_pipe_enabled) {
    content::DevToolsAgentHost::StartRemoteDebuggingPipeHandler(
        base::BindOnce(&PostTaskToCloseBrowser, browser->GetWeakPtr()));
  }
  ...
}
```

where `PostTaskToCloseBrowser` posts `HeadlessBrowserImpl::Shutdown`. So both binaries exit on pipe EOF; the mechanism is not headless-specific.

By contrast `StartHttpServer` — the `--remote-debugging-port` path — takes only a socket factory, output dir, frontend dir and mode. There is no disconnect closure anywhere in that path, which is why closing a DevTools WebSocket does not stop the browser.

### 3.3 Platform caveats

- **Windows fd 3/4.** Chromium maps them with the CRT: `read_handle_ = reinterpret_cast<HANDLE>(_get_osfhandle(read_fd));` (and the same for the writer), wrapped in a `ScopedInvalidParameterHandlerOverride` so a bad fd returns `INVALID_HANDLE_VALUE` instead of tripping the CRT's invalid-parameter handler. This only works if the launcher hands the child a CRT fd table with entries 3 and 4 — which is what libuv does by building the MSVCRT `child_stdio_buffer` (`int count; unsigned char crt_flags[]; HANDLE os_handle[]`) and passing it as `StartupInfo.lpReserved2` / `cbReserved2`.
- **Windows alternative: raw handles on the command line.** Since the parent may not be a CRT program, Chromium also accepts `--remote-debugging-io-pipes=<in>,<out>`, "a comma separated list of two pipe handles serialized as unsigned integers", which the browser turns back into fds with `_open_osfhandle`. `StartRemoteDebuggingPipeHandler` consults it before falling back to fds 3/4, and if adoption fails it runs `on_disconnect` immediately.
- **Framing.** `--remote-debugging-pipe` alone means `kASCIIZ` (NUL-separated JSON); `--remote-debugging-pipe=cbor` selects the "Experimental (!) CBOR (RFC 7049) based binary format".
- **Policy / default-profile gate.** In branded Google Chrome on Win/Mac/Linux, `RemoteDebuggingServer::GetInstance` refuses to start the pipe handler at all when the default user data directory is in use (`kDisabledByDefaultUserDataDir`) or when the `DevToolsRemoteDebuggingAllowed` policy is off. Both drivers always pass an explicit `--user-data-dir`, so they clear the first gate; a Go driver that reuses the user's real profile would not. When the handler never starts, there is no EOF watchdog at all.
- **Write side is inert.** `PipeWriterBase::WriteBytes` logs `"Could not write into pipe"` and returns; it never posts `OnDisconnect`. Chromium ignores `SIGPIPE` globally (`content/app/content_main.cc`: `CHECK_NE(SIG_ERR, signal(SIGPIPE, SIG_IGN));`). Only closing Chrome's **read** end (its fd 3) triggers shutdown.
- **The keep-alive.** With `--no-startup-window`/`--headless` plus a remote-debugging switch, `ChromeDevToolsManagerDelegate` holds a `ScopedKeepAlive(KeepAliveOrigin::REMOTE_DEBUGGING)`; `AllowBrowserToClose()` inside `CloseBrowserSoon` is what releases it. That is why a windowless Chrome stays up until the pipe closes and then exits cleanly rather than being terminated.
- **fd 3/4 must actually be open (M113+).** `chrome/app/chrome_main_delegate.cc` and `headless/lib/headless_content_main_delegate.cc` verify the descriptors before opening any other file (`fcntl(fd, F_GETFL) != -1` on POSIX, `_get_osfhandle(fd) != INVALID_HANDLE_VALUE` on Windows, in `components/devtools/devtools_pipe/devtools_pipe.cc`) and abort with `CHROME_RESULT_CODE_UNSUPPORTED_PARAM` / `EXIT_FAILURE` and `LOG(ERROR) << "Remote debugging pipe file descriptors are not open.";`. The check is skipped on Windows when `--remote-debugging-io-pipes` is present. It was added for crbug 40259890, where a launcher that did not open fd 3/4 made Chrome read and write whatever files happened to land there ("will result in file data corruption").
- **Windows teardown.** `ClosePipe` does `CancelIoEx(read_handle_, nullptr)` before `_close(read_fd_)` (POSIX does `shutdown(read_fd_, SHUT_RDWR)`), added because `CloseHandle` blocked indefinitely on a synchronous read.
- **Headless-shell packaging.** Per `headless/README.md`, prebuilt `headless_shell` ships as `chrome-headless-shell` from M118, and as of M132 `--headless=old` has no effect — the shell is no longer inside the Chrome binary.

### 3.4 Version floor and history

The disconnect closure is not ancient. Relevant landings:

| Landed | Milestone | Change |
|---|---|---|
| 2017-10-25 | M64 | `DevToolsPipeHandler` introduced (CL 735515), content_shell only |
| 2018-02-13 | M66 | full Chrome support for `--remote-debugging-pipe` (CL 912247) |
| 2018-03-29 | M67 | headless support (CL 954405) — no separate handler, it calls the same `content::` entry point |
| 2018-12-15 | M74 | `CancelIoEx` on Windows teardown (CL 1379314) |
| 2019-02-12 | M74 | CBOR framing mode (CL 1460182) |
| **2020-11-13** | **M89** (89.0.4325.0) | **CL 2536354 "Disconnected DevToolsPipeHandler closes the browser"** — adds `on_disconnect` and wires `CloseBrowserSoon` / `PostTaskToCloseBrowser` |
| 2021-02-05 | M90 | pipe and port may be used simultaneously (CL 2679787) |
| 2023-03-13 | M113 | fd-open pre-check (CL 4327189, crbug 40259890) |
| 2023-03-16 | M113 | fix for the Mac/Windows regression that pre-check caused for Puppeteer `pipe: true` (CL 4346972, crbug 1425099) |
| 2023-07-10 | M117 | `--remote-debugging-io-pipes` for ChromeDriver on Windows (CL 4628834) |

So: **M89+ means pipe EOF shuts the browser down; M88 and earlier do not, and the launcher must kill the process itself.** No primary-source bug reporting "browser does not exit on pipe EOF" exists after M89; the earlier behaviour was by design, not a defect.

---

## 4. The Node/libuv substrate (why Windows is free for them and not for Go)

Node's `child_process.spawn` is `uv_spawn`; `options.detached` maps to `UV_PROCESS_DETACHED` (`src/process_wrap.cc`: `if (flags & kProcessFlagDetached) options.flags |= UV_PROCESS_DETACHED;`).

libuv `src/win/process.c` creates one process-wide job object lazily and puts every non-detached child in it:

```c
/* Create a job object and set it up to kill all contained processes when
 * it's closed. Since this handle is made non-inheritable and we're not
 * giving it to anyone, we're the only process holding a reference to it.
 * That means that if this process exits it is closed and all the processes
 * it contains are killed. All processes created with uv_spawn that are not
 * spawned with the UV_PROCESS_DETACHED flag are assigned to this job.
 ...
  info.BasicLimitInformation.LimitFlags =
      JOB_OBJECT_LIMIT_BREAKAWAY_OK |
      JOB_OBJECT_LIMIT_SILENT_BREAKAWAY_OK |
      JOB_OBJECT_LIMIT_DIE_ON_UNHANDLED_EXCEPTION |
      JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE;
```

and after `CreateProcessW`:

```c
  /* If the process isn't spawned as detached, assign to the global job object
   * so windows will kill it when the parent process dies. */
  if (!(options->flags & UV_PROCESS_DETACHED)) {
    uv_once(&uv_global_job_handle_init_guard_, uv__init_global_job_handle);
    if (!AssignProcessToJobObject(uv_global_job_handle_, info.hProcess)) {
      /* ... When that happens we just swallow the error and continue without
       * establishing a kill-child-on-parent-exit relationship ... */
      DWORD err = GetLastError();
      if (err != ERROR_ACCESS_DENIED) uv_fatal_error(err, "AssignProcessToJobObject");
    }
  }
```

This is the reason both drivers can afford to do nothing special on Windows: the runtime already gave them `KILL_ON_JOB_CLOSE`. Go's `os/exec` has no equivalent.

Node's `stdio` documentation confirms the fd-3/4 mechanism is a documented, cross-platform API: "Otherwise, the value of `options.stdio` is an array where each index corresponds to an fd in the child... Additional fds can be specified to create additional pipes between the parent and child." And on `detached`: "On non-Windows platforms, if `options.detached` is set to `true`, the child process will be made the leader of a new process group and session."

Chromium's own Windows children are covered by the sandbox, not by libuv's job: `sandbox/win/src/job.cc` sets `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` on the job it creates for each target, and the browser process holds the only handle — so killing the browser process takes its sandboxed children with it.

---

## 5. Comparators

### 5.1 chromedp

`allocate_linux.go` is the whole of chromedp's orphan prevention:

```go
//go:build linux

func allocateCmdOptions(cmd *exec.Cmd) {
	if _, ok := os.LookupEnv("LAMBDA_TASK_ROOT"); ok {
		// do nothing on AWS Lambda
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = new(syscall.SysProcAttr)
	}
	// When the parent process dies (Go), kill the child as well.
	cmd.SysProcAttr.Pdeathsig = syscall.SIGKILL
}
```

`allocate_other.go` is `//go:build !linux` with an empty body. There is no `allocate_windows.go` and no `allocate_darwin.go` (both raw paths 404). So macOS and Windows get nothing at all: no job object, no `CREATE_NEW_PROCESS_GROUP`, no kqueue watcher, no guard.

Other chromedp facts:

- Transport is a WebSocket on a TCP port, never the pipe: `if _, ok := a.initFlags["remote-debugging-port"]; !ok { args = append(args, "--remote-debugging-port=0") }`, then the URL is scraped from the `DevTools listening on` line; `conn.go` is "a gobwas/ws websocket connection", the only `Transport` implementation. The string `--remote-debugging-pipe` does not appear in chromedp.
- Teardown is `exec.CommandContext`, whose documented `Cancel` "invoke[s] the Kill method on its Process" — i.e. a single **positive** pid, not the group, and no `taskkill`. The graceful path is CDP `Browser.close` in `chromedp.Cancel`.
- The temp `chromedp-runner*` user-data-dir is removed by a goroutine after `cmd.Wait()`, so it leaks whenever the Go process dies.
- Setting the public `ModifyCmdFunc` option **replaces** `allocateCmdOptions` (`if a.modifyCmdFunc != nil { a.modifyCmdFunc(cmd) } else { allocateCmdOptions(cmd) }`), silently dropping the Linux `Pdeathsig`. Its doc comment describes the default as "send[ing] SIGKILL to any open browsers when the Go program exits".

### 5.2 chromedriver (Chromium's own `chrome/test/chromedriver`)

- **No job object on Windows.** `base::LaunchOptions` has a `HANDLE job_handle` field ("If non-null, launches the application in that job object"), but `chrome_launcher.cc` never assigns it.
- **No process-group kill on POSIX.** The only `new_process_group` assignment is `if (capabilities.detach) options.new_process_group = true;`, i.e. it is used to let Chrome deliberately *outlive* chromedriver.
- Kill paths are `ChromeDesktopImpl::QuitImpl()` (CDP `Browser.close`, wait 10 s, else `KillProcess`) and `KillProcess`, which is `kill(process.Pid(), SIGKILL)` on a **positive** pid. `~ChromeDesktopImpl()` kills nothing; when `!quit_` it logs "Chrome quit unexpectedly, leaving behind temporary directories for debugging:" and leaks them on purpose. `chromedriver_server.cc` installs no POSIX signal handlers (only `base::AtExitManager`). **Kill chromedriver with SIGKILL and Chrome keeps running.** In the non-detach case an interactive Ctrl-C does reach Chrome, but only because it shares chromedriver's process group by accident, not by design.
- chromedriver *does* support `--remote-debugging-pipe`, and `chrome/test/chromedriver/net/pipe_builder.cc` is the reference implementation of both platform strategies: POSIX `options->fds_to_remap.emplace_back(child_read.get(), kReadFD)`, Windows `--remote-debugging-io-pipes=<in>,<out>` with `options->handles_to_inherit.push_back(...)` and the comment "We use the fact that inherited handles in the child process have the same value and access rights as in the parent process."

### 5.3 go-rod today (for contrast)

`launcher.Launch()`:

```go
if l.Has(flags.Leakless) && leakless.Support() {
	ll = leakless.New()
	cmd = ll.Command(bin, args...)
} else {
	port := l.Get(flags.RemoteDebuggingPort)
	u, err := ResolveURL(port)
	if err == nil { return u, nil }
	cmd = exec.Command(bin, args...)
}
```

`osSetupCmd` adds `SysProcAttr{Setpgid: true}` on unix and `CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP` on Windows; `killGroup` is `syscall.Kill(-pid, SIGKILL)` / `TerminateProcess`. rod uses `--remote-debugging-port`, not the pipe. When leakless is unavailable (`Support()` covers only linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64), the fallback is a bare `exec.Command` with no parent-death mechanism at all.

leakless itself, for the record: `Launcher.Command` does not exec the target — it opens `net.Listen("tcp", "127.0.0.1:0")`, generates a random UID, and execs a guard binary with `(uid, addr, name, args...)`. The guard dials back, spawns the real child, reports its pid, then blocks decoding JSON from the socket; **the socket EOF that the kernel delivers when the parent dies (SIGKILL included) unblocks it and runs the deferred kill**. The kill is `syscall.Kill(-p.Pid, SIGKILL)` after `Setpgid: true` on unix and `exec.Command("taskkill", "/t", "/f", "/pid", ...)` on Windows. The guard binaries are gzip+base64 string literals generated by `cmd/pack` and written to `os.TempDir()/leakless-<GOARCH>-<version>/leakless[.exe]` on first use, under a `127.0.0.1:2978` port lock. The README's reason for using a socket rather than a pid: "after a process exits, a newly created process may have the same PID."

### 5.4 Does Chrome have any other "die with parent" switch?

No. The switch list in `content/public/common/content_switches.cc` has `remote-debugging-pipe`, `remote-debugging-port`, `remote-debugging-io-pipes` (Windows) and `remote-allow-origins`; the disconnect closure is wired only for the pipe. There is no `--parent-process-handle` or equivalent.

`PR_SET_PDEATHSIG` does appear in Chromium, but not where it would help:

```cpp
    if (options.kill_on_parent_death) {
      if (prctl(PR_SET_PDEATHSIG, SIGKILL) != 0) {
        RAW_LOG(ERROR, "prctl(PR_SET_PDEATHSIG) failed");
        _exit(127);
      }
    }
```

`base/process/launch_posix.cc`, driven by `LaunchOptions::kill_on_parent_death`, which is declared only under `#if BUILDFLAG(IS_LINUX) || BUILDFLAG(IS_CHROMEOS)`. A code search for that field across the tree returns four files — `base/process/launch.h`, `base/process/launch_posix.cc`, `remoting/host/linux/linux_process_launcher_delegate.cc`, `base/test/launcher/test_launcher.cc`. **No renderer/GPU launch path sets it**, and nothing lets an external launcher request it: the flag is set by the launching process on itself after `fork()`, so a Go driver would have to set it in its own `pre_exec` hook, which is exactly `SysProcAttr.Pdeathsig`.

How Chromium's own children actually die (browser -> renderer, the reverse of our problem): on POSIX, `ChildThreadImpl::TerminateSelfOnDisconnect()` on Mojo channel error calls `base::Process::TerminateCurrentProcessImmediately(0)`, with the comment "This isn't needed on Windows because there the sandbox's job object terminates child processes automatically"; on Windows, `sandbox/win/src/job.cc` and `CreateUnsandboxedJob()` in `sandbox/policy/win/sandbox_win.cc` give each child a `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` job whose handle the *browser* holds. Both protect children from a browser crash, not the browser from a launcher crash.

The one place Chromium does exactly what wand would need is internal: `chrome/browser/win/isolated_browser/isolated_browser_support.cc` has a stub process create a `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` job, put itself and the isolated browser in it, and deliberately leak the primary job handle so that "when the stub/test process terminates or crashes, Windows kernel automatically closes all process handles, triggering `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` to kill any surviving browser processes". That is chrome.exe launching itself; it is not exposed to external launchers.

---

## 6. Feasibility notes for a Go driver

Facts only; no recommendation.

### 6.1 `cmd.ExtraFiles` does not work on Windows

`os/exec.Cmd`, verbatim:

```go
	// ExtraFiles specifies additional open files to be inherited by the
	// new process. It does not include standard input, standard output, or
	// standard error. If non-nil, entry i becomes file descriptor 3+i.
	//
	// ExtraFiles is not supported on Windows.
	ExtraFiles []*os.File
```

The failure is silent rather than an error: `exec.go` appends `ExtraFiles` into `os.ProcAttr.Files`, and `syscall/exec_windows.go` `StartProcess` only wires `fd[0..2]` into `si.StdInput/StdOutput/StdErr`. Entries 3+ are duplicated as inheritable and put in the `PROC_THREAD_ATTRIBUTE_HANDLE_LIST`, but the child is never told their values and Windows has no fd-number concept, so nothing usable arrives at fd 3/4. On Linux and macOS `ExtraFiles` works as documented and gives Chromium exactly the fd 3/4 it wants.

### 6.2 Windows: `AdditionalInheritedHandles` + `--remote-debugging-io-pipes`

`syscall.SysProcAttr` on Windows (reachable through `cmd.SysProcAttr`) has:

```go
	NoInheritHandles           bool                // if set, no handles are inherited by the new process, not even the standard handles, contained in ProcAttr.Files, nor the ones contained in AdditionalInheritedHandles
	AdditionalInheritedHandles []Handle            // a list of additional handles, already marked as inheritable, that will be inherited by the new process
```

Note "already marked as inheritable" — the caller sets `HANDLE_FLAG_INHERIT` itself (`windows.SetHandleInformation`, or `SecurityAttributes{InheritHandle: 1}` at creation). Because an inherited handle keeps the same numeric value in the child, the chromedriver recipe transfers directly: create the two pipes, mark inheritable, list them in `AdditionalInheritedHandles`, and pass `--remote-debugging-io-pipes=<read>,<write>` on the command line. `golang.org/x/sys/windows` adds no competing field; it contributes `SetHandleInformation`, `HANDLE_FLAG_INHERIT`, `DuplicateHandle`.

### 6.3 `Pdeathsig` and the thread caveat (golang/go#27505)

Current Go documentation, `syscall/exec_linux.go`:

```go
	// Pdeathsig, if non-zero, is a signal that the kernel will send to
	// the child process when the creating thread dies. Note that the signal
	// is sent on thread termination, which may happen before process termination.
	// There are more details at https://go.dev/issue/27505.
	Pdeathsig    Signal
```

golang/go#27505 ("syscall: misleading documentation for linux SysProcAttr.Pdeathsig", opened 2018-09-05, milestone Backlog) is **closed as completed on 2026-08-12 by the original reporter**, with the closing comment "In any case, I think the Linux documentation is sufficient now, so this issue could probably be closed." It was closed as a documentation fix (CL 412114, "syscall: clarify Pdeathsig documentation on Linux / This is a rather large footgun"), **not** as a behaviour change. The mitigation stated in the thread is to call `runtime.LockOSThread` before `cmd.Start` and never unlock or let that goroutine return.

The kernel semantics are the source of the footgun; `PR_SET_PDEATHSIG(2const)`, CAVEATS:

> The "parent" in this case is considered to be the thread that created this process. In other words, the signal will be sent when that thread terminates (via, for example, pthread_exit(3)), rather than after all of the threads in the parent process terminate.

Two further facts from the same page: the parent-death signal setting **is cleared for the child of a `fork(2)`** (so Chrome's own forked subprocesses do not inherit it) and it is cleared on setuid/setgid exec or a binary with file capabilities (relevant to Chrome's setuid sandbox helper). STANDARDS: Linux. FreeBSD has an equivalent (`procctl(PROC_PDEATHSIG_CTL)`), exposed in Go as `Pdeathsig` in `syscall/exec_freebsd.go`.

### 6.4 Process groups and Windows job objects from Go

- `SysProcAttr.Setpgid` / `.Pgid` exist on every unix target (`exec_linux.go`, `exec_libc2.go` for darwin). `syscall.Kill(-pid, sig)` has POSIX-defined semantics: "If *pid* is negative, but not -1, *sig* shall be sent to all processes ... whose process group ID is equal to the absolute value of *pid*" (IEEE Std 1003.1-2024, `kill()`).
- `golang.org/x/sys/windows` has the complete job-object surface: `CreateJobObject`, `AssignProcessToJobObject`, `SetInformationJobObject`, `TerminateJobObject`; the type `JOBOBJECT_EXTENDED_LIMIT_INFORMATION`; the constants `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE = 0x00002000` and `JobObjectExtendedLimitInformation = 9`. That is exactly the combination libuv uses, so the Node-on-Windows behaviour is reproducible in pure Go. Caveat from Chromium's `base/process/launch.h`: a child launched with `CREATE_BREAKAWAY_FROM_JOB` escapes the job.

### 6.5 macOS has no parent-death mechanism

Darwin's `syscall.SysProcAttr` (`syscall/exec_libc2.go`, `//go:build darwin || openbsd`) has no `Pdeathsig` field — only `Chroot, Credential, Ptrace, Setsid, Setpgid, Setctty, Noctty, Ctty, Foreground, Pgid`. The field exists only in `exec_linux.go` and `exec_freebsd.go`. macOS offers no `prctl(PR_SET_PDEATHSIG)` and no `procctl` equivalent.

What macOS does offer is the reverse direction: kqueue `EVFILT_PROC` with `NOTE_EXIT`, per Apple's `kqueue(2)` — "Takes the process ID to monitor as the identifier ... `NOTE_EXIT` — The process has exited." That is usable only from a separate watcher process observing the parent's pid, and carries the pid-reuse race that leakless cites as its reason for using a socket instead.

So on macOS the available options are exactly: (a) a guard process (socket-EOF like leakless, or kqueue `NOTE_EXIT`); (b) a pipe or socket the child itself watches — which is what `--remote-debugging-pipe` provides for free on Chromium M89+, since Chrome exits on pipe EOF; or (c) accept the leak, which is what chromedp and chromedriver do.

---

## Sources

Playwright (`microsoft/playwright`, branch `main`)
- https://github.com/microsoft/playwright/blob/main/packages/utils/processLauncher.ts
- https://github.com/microsoft/playwright/blob/main/packages/playwright-core/src/server/browserType.ts
- https://github.com/microsoft/playwright/blob/main/packages/playwright-core/src/server/chromium/chromium.ts
- https://github.com/microsoft/playwright/blob/main/packages/playwright-core/src/server/pipeTransport.ts
- https://github.com/microsoft/playwright/blob/main/packages/utils/pipeTransport.ts
- https://github.com/microsoft/playwright/blob/main/packages/playwright-core/src/cli/driver.ts
- https://github.com/microsoft/playwright/blob/main/docs/src/api/params.md (`browser-option-handlesigint` / `-handlesigterm` / `-handlesighup`)
- https://github.com/microsoft/playwright/tree/main/packages/playwright-core/bin (install scripts only, no shipped helper binary)
- https://github.com/microsoft/playwright/issues/41013, https://github.com/microsoft/playwright/issues/41664

Puppeteer (`puppeteer/puppeteer`, branch `main`)
- https://github.com/puppeteer/puppeteer/blob/main/packages/browsers/src/launch.ts
- https://github.com/puppeteer/puppeteer/blob/main/packages/puppeteer-core/src/node/ChromeLauncher.ts
- https://github.com/puppeteer/puppeteer/blob/main/packages/puppeteer-core/src/node/BrowserLauncher.ts
- https://github.com/puppeteer/puppeteer/blob/main/packages/puppeteer-core/src/node/PipeTransport.ts
- https://github.com/puppeteer/puppeteer/blob/main/packages/puppeteer-core/src/node/LaunchOptions.ts
- https://github.com/puppeteer/puppeteer/blob/main/packages/puppeteer-core/src/node/util/fs.ts

Chromium (`chromium/src`, branch `main`, via chromium.googlesource.com `?format=TEXT`)
- content/public/common/content_switches.cc (`kRemoteDebuggingPipe`, `kRemoteDebuggingPort`, `kRemoteDebuggingIoPipes`)
- content/public/browser/devtools_agent_host.h (`kReadFD = 3`, `kWriteFD = 4`, `StartRemoteDebuggingPipeHandler`)
- content/browser/devtools/devtools_pipe_handler.h / .cc (`PipeReaderBase::ReadBytes`, `ReadLoop`, `OnDisconnect`, `Shutdown`, `_get_osfhandle`)
- content/browser/devtools/devtools_agent_host_impl.cc (`AdoptHandle`, `AdoptPipes`, `StartRemoteDebuggingPipeHandler`)
- chrome/browser/devtools/remote_debugging_server.cc (`StartPipeHandler`, `IsRemoteDebuggingAllowed`, `kDisabledByDefaultUserDataDir`)
- chrome/browser/devtools/chrome_devtools_manager_delegate.cc (`CloseBrowserSoon` -> `chrome::ExitIgnoreUnloadHandlers`)
- headless/lib/browser/headless_devtools.cc (`PostTaskToCloseBrowser` -> `HeadlessBrowserImpl::Shutdown`)
- sandbox/win/src/job.cc, sandbox/policy/win/sandbox_win.cc (`CreateUnsandboxedJob`), `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`
- base/process/launch.h (`job_handle`, `kill_on_parent_death`, `CREATE_BREAKAWAY_FROM_JOB` note), base/process/launch_posix.cc (`PR_SET_PDEATHSIG`)
- content/app/content_main.cc (`signal(SIGPIPE, SIG_IGN)`), content/child/child_thread_impl.cc (`TerminateSelfOnDisconnect`)
- chrome/app/chrome_main_delegate.cc, headless/lib/headless_content_main_delegate.cc, components/devtools/devtools_pipe/devtools_pipe.cc (fd 3/4 pre-check)
- headless/lib/browser/command_line_handler.cc, headless/lib/browser/headless_browser_impl.cc, headless/README.md
- content/shell/browser/shell_devtools_manager_delegate.cc (null `on_disconnect`)
- chrome/browser/win/isolated_browser/isolated_browser_support.cc
- chrome/test/chromedriver/chrome_launcher.cc, chrome/test/chromedriver/chrome/chrome_desktop_impl.cc, chrome/test/chromedriver/net/pipe_builder.cc
- CLs: https://chromium-review.googlesource.com/c/chromium/src/+/735515 (M64, handler), .../912247 (M66, Chrome), .../954405 (M67, headless), .../1379314 (M74, `CancelIoEx`), .../1460182 (M74, CBOR), **.../2536354 (M89, "Disconnected DevToolsPipeHandler closes the browser")**, .../2679787 (M90), .../4327189 and .../4346972 (M113), .../4628834 (M117, `--remote-debugging-io-pipes`)
- https://issues.chromium.org/issues/40259890 (fd 3/4 corruption when the launcher does not open them), crbug 1425099 (M113 Mac/Windows `pipe: true` regression)

Node.js / libuv
- https://github.com/nodejs/node/blob/main/doc/api/child_process.md (`options.stdio`, `options.detached`)
- https://github.com/nodejs/node/blob/main/src/process_wrap.cc (`kProcessFlagDetached` -> `UV_PROCESS_DETACHED`, `uv_spawn`)
- https://github.com/libuv/libuv/blob/v1.x/src/win/process.c (`uv__init_global_job_handle`, `AssignProcessToJobObject`)
- https://github.com/libuv/libuv/blob/v1.x/src/win/process-stdio.c (`child_stdio_buffer`, `lpReserved2`)

Go
- https://github.com/golang/go/blob/master/src/os/exec/exec.go (`ExtraFiles`, `CommandContext`)
- https://github.com/golang/go/blob/master/src/syscall/exec_windows.go (`SysProcAttr.AdditionalInheritedHandles`, `PROC_THREAD_ATTRIBUTE_HANDLE_LIST`)
- https://github.com/golang/go/blob/master/src/syscall/exec_linux.go (`Pdeathsig`, `Setpgid`)
- https://github.com/golang/go/blob/master/src/syscall/exec_libc2.go (darwin `SysProcAttr`, no `Pdeathsig`)
- https://github.com/golang/go/blob/master/src/syscall/exec_freebsd.go (`Pdeathsig` via `procctl`)
- https://github.com/golang/go/issues/27505 (closed 2026-08-12 as completed, documentation only)
- https://go-review.googlesource.com/c/go/+/412114
- https://github.com/golang/sys/blob/master/windows/syscall_windows.go, .../types_windows.go (job-object API and constants)
- https://man7.org/linux/man-pages/man2/PR_SET_PDEATHSIG.2const.html
- https://pubs.opengroup.org/onlinepubs/9799919799/functions/kill.html
- https://developer.apple.com/library/archive/documentation/System/Conceptual/ManPages_iPhoneOS/man2/kqueue.2.html

Comparators
- https://github.com/chromedp/chromedp/blob/main/allocate_linux.go, allocate_other.go, allocate.go, conn.go, chromedp.go
- https://github.com/ysmood/leakless/blob/main/leakless.go, cmd/leakless/os_unix.go, cmd/leakless/os_windows.go, cmd/pack/main.go, cmd/pack/targets.go
- https://github.com/go-rod/rod/blob/main/lib/launcher/launcher.go, os_unix.go, os_windows.go
