# wand

[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/headlesslab/wand/badge)](https://scorecard.dev/viewer/?uri=github.com/headlesslab/wand)

A Chrome DevTools Protocol driver for browser automation and web scraping in Go.

wand descends from a snapshot of [go-rod](https://github.com/go-rod/rod) (commit `393ac0d`, 2024-12-07) and is maintained independently by headlesslab. See [NOTICE](NOTICE) for attribution.

Moving an existing go-rod program over is an import swap and a short list of behaviour changes; [the migration guide](docs/migrating-from-go-rod.md) has both, with a Known limitations section.

[中文说明](README.zh-CN.md)

## Install

```sh
go get github.com/headlesslab/wand
```

wand needs no driver binary. It drives a browser over the DevTools Protocol, and it finds the Chrome you already have before it downloads anything; see [Browsers](#browsers).

```go
package main

import (
	"fmt"

	"github.com/headlesslab/wand"
)

func main() {
	browser := wand.New().MustConnect()
	defer browser.MustClose()

	page := browser.MustPage("https://go.dev").MustWaitStable()

	fmt.Println(page.MustElement("h1").MustText())
}
```

Two commands go with the library:

```sh
go install github.com/headlesslab/wand/cmd/wand-manager@latest        # launch browsers on another machine
go install github.com/headlesslab/wand/cmd/wand-fetch-browser@latest  # download the browser ahead of time
```

The runnable examples in [`examples_test.go`](examples_test.go) drive pages of this repository served on a loopback port, so `go test -run Example` needs no network. [`lib/examples`](lib/examples) holds standalone programs, several of which drive live websites.

## Browsers

wand is aligned to one Chrome stable at a time, the Target Chrome: the protocol layer is generated for it and the Managed browser download is pinned to it. A Companion Chromium trunk build is the second Managed browser, for deployments that cannot accept Google Chrome's terms. ✅ marks the platforms where a Managed browser archive exists; on the others wand uses a browser already installed. Every number in the table moves with each [Roll](docs/maintainer-notes.md#the-roll), once per Chrome stable milestone.

<!-- pins:begin -->
<!-- prettier-ignore-start -->
| Managed browser | Linux x64 | Linux arm64 | macOS x64 | macOS arm64 | Windows x86 | Windows x64 |
| :--- | :---: | :---: | :---: | :---: | :---: | :---: |
| Chrome 153.0.8010.12 ([Chrome for Testing](https://googlechromelabs.github.io/chrome-for-testing/)) | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| chrome-headless-shell 153.0.8010.12 | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Chromium 1680292 ([trunk build](https://commondatastorage.googleapis.com/chromium-browser-snapshots/index.html)) | ✅ | ❌ | ✅ | ✅ | ✅ | ✅ |

Protocol: [devtools-protocol r1680125](https://github.com/ChromeDevTools/devtools-protocol/tree/v0.0.1680125). Support window: Chrome 150 to 153, the Target Chrome and the three stable milestones before it.
<!-- prettier-ignore-end -->

<!-- pins:end -->

The Target Chrome is what CI tests; the three milestones before it are best-effort. Nothing is enforced in code: a browser of another major version is launched anyway, with one line through the browser's logger naming both versions.

### Browser resolution

`Launch` takes the first browser it finds, and downloads one only when it finds none:

1. `Launcher.Bin()` in code
2. the `-wand=bin=<path>` flag ([`lib/defaults`](lib/defaults))
3. `WAND_BROWSER_BIN`
4. a System browser: Google Chrome, Chromium or Microsoft Edge at the paths `LookPath` searches on this OS
5. the Managed browser already in the cache
6. a download of the Managed browser, hash-verified against [`lib/launcher/pins`](lib/launcher/pins), from Google's buckets or npmmirror, whichever answers first

Switch the download off with `WAND_BROWSER_DOWNLOAD=0` or `Launcher.Download(false)`; then, and on a platform with nothing to download, the launch fails with `launcher.ErrNoBrowser` and an error listing every step tried.

| Variable                | Values                                        | In code                                    |
| ----------------------- | --------------------------------------------- | ------------------------------------------ |
| `WAND_BROWSER_BIN`      | path of the browser to launch                 | `Launcher.Bin()`; the flag is `-wand=bin=` |
| `WAND_BROWSER_CACHE`    | the browser cache directory                   | `Browser.RootDir`                          |
| `WAND_BROWSER_HOSTS`    | URL templates separated by commas             | `Launcher.Hosts()`                         |
| `WAND_BROWSER_DOWNLOAD` | `0` switches the download off                 | `Launcher.Download()`                      |
| `WAND_BROWSER_SOURCE`   | `chrome` (default) or `chromium`              | `Launcher.Source()`                        |
| `WAND_BROWSER_BINARY`   | `chrome` (default) or `chrome-headless-shell` | `Launcher.Binary()`                        |

Precedence is the option set in code, then the flag, then the variable, then discovery. The cache is `wand/browser` under the user cache directory (`$XDG_CACHE_HOME` or `~/.cache` on Linux, `~/Library/Caches` on macOS, `%LocalAppData%` on Windows). The [launcher's README](lib/launcher/README.md) has the whole story.

Domestic browsers are not in the discovery list: none of lbrowser, 奇安信可信浏览器, 360 安全浏览器信创版, UOS 浏览器 or 红莲花 is documented as accepting `--remote-debugging-port`. If yours does, name it yourself with `WAND_BROWSER_BIN` or `Launcher.Bin()`. On UOS and Kylin, browsers installed from the app store live under `/opt/apps/<app id>/files/`; on Loongnix, `dpkg -L lbrowser` names lbrowser's executable.

## Platforms

| Support tier | What wand promises                                                                                       | Platforms                                                     |
| ------------ | -------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------- |
| Tier 1       | built and tested with a real browser in CI, with `-race`, on every pull request and every push to `main` | `linux/amd64`, `linux/arm64`, `windows/amd64`, `darwin/arm64` |
| Tier 2       | built and vetted in CI, never tested with a browser; runtime best-effort                                 | `darwin/amd64`, `windows/arm64`, `linux/loong64`              |
| Tier 3       | no promise                                                                                               | everything else                                               |

The Go floor is **`go 1.21`**, with no `toolchain` directive, and it is anchored to the newest Go the current openEuler LTS ships natively on every architecture it supports ([ADR-0003](docs/adr/0003-go-floor-anchored-to-openeuler-lts.md)). It moves only when a new LTS raises it.

That makes the Domestic platform build paths:

- **openEuler 24.03 LTS**, on x86_64, aarch64 and loongarch64, builds wand with its own packaged Go — 1.21.4 on GA through SP2, 1.24.6 on SP3 and SP4 — and needs no toolchain download.
- **openEuler 22.03 LTS (Go 1.17.3), Kylin V10 (Go 1.13–1.15) and UOS 20** are below the floor. Either cross-compile from a machine with Go 1.21 or later, or install a Go tarball from [golang.google.cn](https://golang.google.cn/dl/), which serves every architecture below.

  ```sh
  GOOS=linux GOARCH=loong64 go build ./...
  ```

- **`linux/loong64` means new-world (ABI 2.0) distributions only**: openEuler 22.03 LTS and 24.03 LTS SP1 and later. Kylin V10 and UOS 20 loong64 builds are old-world, are not targets, and an official Go binary crashes on them before `main`. Official Go has shipped `linux/loong64` binaries since Go 1.21.

No Managed browser archive exists for `windows/arm64`, `linux/loong64` or musl systems such as Alpine, so wand never downloads there; point `WAND_BROWSER_BIN` at a browser you installed.

## Container image

`ghcr.io/headlesslab/wand` is `ubuntu:noble` with the Target Chrome inside it and `wand-manager`, the remote-launch server, as its entrypoint, for `linux/amd64` and `linux/arm64` in one manifest. The browser it carries is the pinned one above, downloaded and hash-verified at build time, so a container runs the browser wand is tested on. Its tags are published by a release and are not there yet; until then, build it from a checkout:

```sh
docker build -t ghcr.io/headlesslab/wand -f docker/Dockerfile .
docker run --rm -p 7317:7317 ghcr.io/headlesslab/wand
```

A wand program anywhere then launches its browsers in that container:

```go
l := launcher.MustNewManaged("ws://127.0.0.1:7317")
browser := wand.New().Client(l.MustClient()).MustConnect()
```

See [`lib/examples/launch-managed`](lib/examples/launch-managed) for the whole program.

`docker run --rm ghcr.io/headlesslab/wand chrome --version` prints the Chrome inside, and `xvfb-run` is there for a visible browser.

Every published manifest carries a build-provenance attestation and an SPDX SBOM attestation in Sigstore's public log:

```sh
gh attestation verify oci://ghcr.io/headlesslab/wand:v0.1.0 -R headlesslab/wand
```

`go run ./internal/tools/docker` builds that image and the `:dev` one on top of it, which adds the Go and Node toolchains, and checks both the way the image Gate does; nothing is pushed. Add `-suite` to run wand's whole suite inside the `:dev` image, where `utils.InContainer` holds and the launcher passes `--no-sandbox`.

Behind a restricted network, a Go module proxy and an Ubuntu mirror are build arguments:

```sh
docker build -t ghcr.io/headlesslab/wand -f docker/Dockerfile \
  --build-arg goproxy=https://goproxy.cn,direct \
  --build-arg apt_mirror=https://your-mirror.example/ubuntu .
```

## Releases

One release per Chrome stable milestone, cut after the [Roll](docs/maintainer-notes.md#the-roll) that moves the pins, with bug-fix patches in between. [`versions.json`](versions.json) takes one row per release — the wand version beside the Target Chrome, Protocol roll and Companion Chromium it carried — appended by the release workflow, so you can match a browser fleet to a wand version. It is empty until the first release.

Until 1.0 a minor may break and a patch never does, so `go get -u=patch` is always safe ([ADR-0008](docs/adr/0008-fresh-v0-series-one-milestone-release-per-chrome-stable.md)).

## Roadmap

- **Baseline release**: the go-rod snapshot renamed to `github.com/headlesslab/wand`, built and tested against current Chrome, with the protocol layer, browser acquisition and dependency chain brought current. In progress.
- **API modernization**: planned. It will change the API in a later minor.
- **Stealth**: planned. wand ships no evasion code today.

No dates and no ordering promises.
