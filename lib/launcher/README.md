# Overview

A lib helps to find, launch or download the browser. You can also use it as a standalone lib without wand.

## Managed browser

When no browser binary is set, the launcher downloads a Managed browser once and caches it:

- **Browser source**: Chrome for Testing at the Target Chrome by default (`chrome`, or `chrome-headless-shell` on request), or a Chromium trunk build at the Companion Chromium. Both are pinned in `lib/launcher/pins`, hashes included, and every download is verified against those hashes before extraction, whichever host served it. A version or revision of your own has no recorded hash, so its download is not verified, and the log says so.
- **Download hosts**: Google's buckets and npmmirror, probed concurrently; the first to answer serves the download and the others are fallbacks. A private mirror is a URL template with `{version}`, `{platform}` and `{archive}` placeholders, such as `https://mirror.example/chrome-for-testing/{version}/{platform}/{archive}`.
- **Cache**: `wand/browser` under the user cache directory (`$XDG_CACHE_HOME` or `~/.cache` on Linux, `~/Library/Caches` on macOS, `%LocalAppData%` on Windows), one subdirectory per browser: `chrome-<version>`, `chrome-headless-shell-<version>` or `chromium-<revision>`.
- **Platforms**: the Chrome for Testing archives exist for linux64, linux-arm64, mac-x64, mac-arm64, win32 and win64, the Chromium trunk builds for all of those but linux-arm64. Windows on arm64, Linux on loong64 and Linux with the musl C library (Alpine) have nothing to download; there, point `WAND_BROWSER_BIN` or `Launcher.Bin()` at an installed browser.

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
