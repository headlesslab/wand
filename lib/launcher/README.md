# Overview

A lib helps to find, launch or download the browser. You can also use it as a standalone lib without wand.

## Browser resolution

`Launch` looks for a browser in this order and takes the first it finds:

1. `Launcher.Bin()` in code.
2. The `-wand=bin=<path>` flag (`lib/defaults`).
3. `WAND_BROWSER_BIN`.
4. A System browser: Google Chrome, Chromium or Microsoft Edge at the paths `LookPath` searches on this OS, which are upstream's plus the Chrome for Testing app bundle and the Homebrew prefixes on macOS and `/opt/google/chrome` and `/opt/microsoft/msedge` on Linux. The first that exists wins; versions are not compared.
5. The Managed browser already in the cache.
6. A download of the Managed browser, unless `WAND_BROWSER_DOWNLOAD=0` or `Launcher.Download(false)` switched it off. Then, and on a platform with nothing to download, the launch fails with `launcher.ErrNoBrowser` and an error that lists every step tried.

An explicit path is launched as it is, and wand never refuses a browser: once `wand.Browser` connects, a browser of another major version than the Target Chrome's gets one line through the browser's logger, with both versions. So a developer with Chrome installed runs wand without any download, a fresh container downloads the Target Chrome once, and CI pins the browser with `WAND_BROWSER_BIN`.

Domestic platform browsers (lbrowser, the Qianxin, 360 and UOS browsers) are not in the discovery list: none is verified to accept `--remote-debugging-port`. If yours does, point `WAND_BROWSER_BIN` or `Launcher.Bin()` at its executable; on UOS and Kylin, browsers from the app store install under `/opt/apps/<app id>/files/`, and lbrowser comes as a package whose file list names the executable.

### Environment variables

The `WAND_BROWSER_*` variables set the defaults for a whole process. Precedence is the option set in code, then the `-wand=` flag, then the variable, then discovery:

| Variable                | Values                                        | In code                                    |
| ----------------------- | --------------------------------------------- | ------------------------------------------ |
| `WAND_BROWSER_BIN`      | path of the browser to launch                 | `Launcher.Bin()`; the flag is `-wand=bin=` |
| `WAND_BROWSER_CACHE`    | the browser cache directory                   | `Browser.RootDir`                          |
| `WAND_BROWSER_HOSTS`    | URL templates separated by commas             | `Launcher.Hosts()`                         |
| `WAND_BROWSER_DOWNLOAD` | `0` switches the download off                 | `Launcher.Download()`                      |
| `WAND_BROWSER_SOURCE`   | `chrome` (default) or `chromium`              | `Launcher.Source()`                        |
| `WAND_BROWSER_BINARY`   | `chrome` (default) or `chrome-headless-shell` | `Launcher.Binary()`                        |

## Managed browser

When Browser resolution reaches its last step, the launcher downloads a Managed browser once and caches it:

- **Browser source**: Chrome for Testing at the Target Chrome by default (`chrome`, or `chrome-headless-shell` on request), or a Chromium trunk build at the Companion Chromium. Both are pinned in `lib/launcher/pins`, hashes included, and every download is verified against those hashes before extraction, whichever host served it. A version or revision of your own has no recorded hash, so its download is not verified, and the log says so.
- **Download hosts**: Google's buckets and npmmirror, probed concurrently; the first to answer serves the download and the others are fallbacks. A Download host of your own is a URL template with `{version}`, `{platform}` and `{archive}` placeholders, such as `https://mirror.example/chrome-for-testing/{version}/{platform}/{archive}`.
- **Cache**: `wand/browser` under the user cache directory (`$XDG_CACHE_HOME` or `~/.cache` on Linux, `~/Library/Caches` on macOS, `%LocalAppData%` on Windows), one subdirectory per browser: `chrome-<version>`, `chrome-headless-shell-<version>` or `chromium-<revision>`.
- **Platforms**: Chrome for Testing builds for linux64, linux-arm64, mac-x64, mac-arm64, win32 and win64, and Chromium trunk builds for all of those but linux-arm64; the table in the [root README](../../README.md#browsers) shows which of them the current pins have an archive for. A platform without one, and Windows on arm64, Linux on loong64 or Linux with the musl C library (Alpine) in any case, has nothing to download; there, point `WAND_BROWSER_BIN` or `Launcher.Bin()` at a System browser.

In code, `Launcher.Source`, `Launcher.Binary`, `Launcher.Version`, `Launcher.Revision` and `Launcher.Hosts` choose the browser and the hosts; `launcher.NewBrowser()` is the same download helper on its own, which `WAND_BROWSER_DOWNLOAD` does not switch off.

To download ahead of time, for a container image or an offline bundle, the command prints the path of the browser binary on stdout and its progress on stderr:

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
