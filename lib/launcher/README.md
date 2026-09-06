# Overview

A lib helps to find, launch or download the browser. You can also use it as a standalone lib without wand.

## Managed browser

When no browser binary is set, the launcher downloads a Managed browser once and caches it:

- **Browser source**: Chrome for Testing at the Target Chrome by default (`chrome`, or `chrome-headless-shell` on request), or a Chromium trunk build at the Companion Chromium. Both are pinned in `lib/launcher/pins`, hashes included, and every download is verified against those hashes before extraction, whichever host served it. A version or revision of your own has no recorded hash, so its download is not verified, and the log says so.
- **Download hosts**: Google's buckets and npmmirror, probed concurrently; the first to answer serves the download and the others are fallbacks. A Download host of your own is a URL template with `{version}`, `{platform}` and `{archive}` placeholders, such as `https://mirror.example/chrome-for-testing/{version}/{platform}/{archive}`.
- **Cache**: `wand/browser` under the user cache directory (`$XDG_CACHE_HOME` or `~/.cache` on Linux, `~/Library/Caches` on macOS, `%LocalAppData%` on Windows), one subdirectory per browser: `chrome-<version>`, `chrome-headless-shell-<version>` or `chromium-<revision>`.
- **Platforms**: Chrome for Testing builds for linux64, linux-arm64, mac-x64, mac-arm64, win32 and win64, and Chromium trunk builds for all of those but linux-arm64; the table in the [root README](../../README.md#browsers) shows which of them the current pins have an archive for. A platform without one, and Windows on arm64, Linux on loong64 or Linux with the musl C library (Alpine) in any case, has nothing to download; there, point `WAND_BROWSER_BIN` or `Launcher.Bin()` at a System browser.

In code, `Launcher.Source`, `Launcher.Binary`, `Launcher.Version`, `Launcher.Revision` and `Launcher.Hosts` choose the browser and the hosts; `launcher.NewBrowser()` is the same download helper on its own. The environment variables set the defaults for a whole process:

| Variable              | Values                                        |
| --------------------- | --------------------------------------------- |
| `WAND_BROWSER_SOURCE` | `chrome` (default) or `chromium`              |
| `WAND_BROWSER_BINARY` | `chrome` (default) or `chrome-headless-shell` |
| `WAND_BROWSER_HOSTS`  | URL templates separated by commas             |
| `WAND_BROWSER_CACHE`  | the cache directory                           |

To download ahead of time, for a container image or an offline bundle:

```sh
go run github.com/headlesslab/wand/cmd/wand-fetch-browser [-source chromium] [-binary chrome-headless-shell]
```

## Orphan guard

A browser `Launcher.Launch()` starts does not outlive the wand process, however that process dies: no helper process, no dropped binary, nothing written at launch. The switch is `Launcher.Leakless(bool)`, on in `launcher.New()` and off in `launcher.NewUserMode()`, so that a browser you keep working in survives your program.

- **Windows**: the browser joins a job object that the kernel closes, and so kills, with the wand process.
- **Linux**: the browser is started with the parent-death signal `SIGKILL`, from a goroutine that holds its thread for the browser's lifetime.
- **Every POSIX platform, macOS included**: the browser is started with `--remote-debugging-pipe` on descriptors 3 and 4 that wand opens and never speaks on, the Pipe tether; a Chromium of 89 or later exits by itself when they close. CDP stays on the WebSocket that `Launch()` returns. The flag is passed only with its descriptors, so `Launcher.FormatArgs()` never lists it: a command you build from those arguments yourself has no guard.

With the guard off, or on a browser that rejects the pipe, `Launcher.Kill()` and `Launcher.Cleanup()` still kill the browser's process group on the way out; a wand process that dies hard leaves such a browser running.

Under `Launcher.XVFB()` the tether reaches the browser through xvfb-run all the same; xvfb-run and its Xvfb server are outside it, so a wand process that dies hard leaves the Xvfb server behind.
