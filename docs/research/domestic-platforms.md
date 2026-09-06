# Domestic Chinese platforms as build and runtime targets

- **Date:** 2026-09-05
- **Ticket:** [headlesslab/wand#21](https://github.com/headlesslab/wand/issues/21) — _Survey domestic Chinese platforms as build and runtime targets_
- **Scope:** facts only, each with a source. No decision is made here. Anything that could not be
  confirmed against a primary source is written as **not found**.

## The question (verbatim from #21)

> ## Question
>
> wand must both **build on** and **run on** the domestic platforms its users work with (see `CONTEXT.md`): Kylin V10, UOS, and openEuler, on x86-64 (Hygon, Zhaoxin), ARM64 (Phytium, Kunpeng), and LoongArch new-world (Loongson 3A5000 / 3A6000, `GOARCH=loong64`). Old-world LoongArch, MIPS64, and sw64 are out.
>
> Facts needed, per distro and architecture where they differ:
>
> - **Go toolchain on the platform.** Which Go version each distro's own repos ship (Kylin V10 SP1 / SP2 / SP3, UOS V20 desktop and server, openEuler 22.03 LTS and 24.03 LTS); whether newer Go is available from the vendor or from golang.google.cn tarballs, and since which Go version the official distribution ships `linux/loong64` binaries. How `GOTOOLCHAIN` auto-download behaves behind mainland networks (proxy.golang.org unreachable; goproxy.cn as the usual mirror): does a `go.mod` `go` directive newer than the system Go fail outright or try to download?
> - **Go port status.** Since which Go version `linux/loong64` is a supported port, and any known limitations (cgo, race detector, plugins, `-buildmode`) that matter to a CDP driver.
> - **Browsers.** Which Chromium-based browsers exist on each platform (Qianxin trusted browser / 奇安信可信浏览器, 360 安全浏览器, UOS 浏览器, 红莲花, distro-packaged Chromium), their Chromium major versions, and whether they accept `--remote-debugging-port` and `--headless`, i.e. whether CDP can attach. Whether Chrome for Testing, Chromium snapshots, or Playwright builds offer any `linux-arm64` or `loong64` artifacts.
> - **Dependencies with per-arch artifacts.** Does `ysmood/leakless` embed prebuilt binaries per GOOS/GOARCH, and which arches does it cover? Any other upstream dependency or `lib/` code with arch-specific paths (launcher platform detection, `lib/utils`).
> - **Verification means.** What GitHub Actions can cover for these targets (cross-compile only; QEMU user-mode for arm64 / loong64; self-hosted runners), and what chromedp and playwright-go do for arm64 / loong64.
>
> Output: facts with sources on a `research/` branch (`docs/research/domestic-platforms.md`). Do not decide.

---

## Summary table: distribution × architecture

"System Go" = the newest `golang` package the distribution's own repositories offer (base +
updates), read from the repository package indexes on 2026-09-05. "Official Go" = whether
[golang.google.cn](https://golang.google.cn/dl/) publishes a tarball for that `GOOS/GOARCH`.

| Distribution / release                                   | Arch            | System Go (distro repos)                                                                                  | Official Go tarball                | Chromium-family browser in distro repos                               | CDP attach            | CI verifiable                           |
| -------------------------------------------------------- | --------------- | --------------------------------------------------------------------------------------------------------- | ---------------------------------- | --------------------------------------------------------------------- | --------------------- | --------------------------------------- |
| Kylin V10 SP1 (server, `NS/V10/V10SP1`)                  | x86_64, aarch64 | **1.13** (`golang-1.13-3.3.ky10`)                                                                         | yes (`linux-amd64`, `linux-arm64`) | none (only `firefox-60.7.0`)                                          | n/a in repo           | cross-compile; arm64 hosted runner      |
| Kylin V10 SP1 (server)                                   | loongarch64     | no `loongarch64` tree in this SP                                                                          | yes (`linux-loong64`, ≥ go1.21)    | n/a                                                                   | n/a                   | cross-compile only                      |
| Kylin V10 SP2 (server)                                   | x86_64, aarch64 | **1.15.7** (base 1.13.15, updates → `1.15.7-60.p01`)                                                      | yes                                | none (only `firefox-79.0`)                                            | n/a in repo           | cross-compile; arm64 hosted runner      |
| Kylin V10 SP3 (server)                                   | x86_64, aarch64 | **1.15.7** (`golang-1.15.7-60.p01.ky10`)                                                                  | yes                                | none (only `firefox-79.0`)                                            | n/a in repo           | cross-compile; arm64 hosted runner      |
| Kylin V10 SP3 (server)                                   | loongarch64     | **1.15.7** (`golang-1.15.7-60.p01.a.ky10.loongarch64`)                                                    | yes (`linux-loong64`, ≥ go1.21)    | none                                                                  | n/a in repo           | cross-compile only (no runner, no QEMU) |
| Kylin V10 desktop (`KYLIN-ALL` suite `10.1`)             | amd64, arm64    | **1.14** (`golang-1.14` 1.14.3; default `golang` = 2:1.13)                                                | yes                                | **`chromium-browser` 83.0.4103.61**                                   | not vendor-documented | cross-compile; arm64 hosted runner      |
| Kylin V10 desktop (`KYLIN-ALL` suite `10.1`)             | loongarch64     | **1.15.6** (`golang-1.15` 1.15.6-1.lnd.9)                                                                 | yes (≥ go1.21)                     | none                                                                  | n/a                   | cross-compile only                      |
| UOS V20 desktop / server                                 | all             | **not found** — `professional-packages.chinauos.com` and `home-packages.chinauos.com` return **HTTP 401** | yes for amd64/arm64/loong64        | **not found** in a first-party repo                                   | not found             | cross-compile only                      |
| _(closest public proxy for UOS 20: deepin 20 `apricot`)_ | amd64 only      | 1.15.9 (`golang-1.15`), default `golang` 2:1.15.1                                                         | —                                  | `chromium` **83.0.4103.116**; `google-chrome-stable` 76 in `non-free` | not vendor-documented | —                                       |
| openEuler 22.03 LTS (GA … SP4)                           | x86_64, aarch64 | **1.17.3** (unchanged across GA, SP1–SP4 and all updates)                                                 | yes                                | **none** (only `firefox`)                                             | n/a in repo           | cross-compile; arm64 hosted runner      |
| openEuler 22.03 LTS (GA only)                            | loongarch64     | **1.17.3** (`golang-1.17.3-18.oe2203.loongarch64`)                                                        | yes (≥ go1.21)                     | none                                                                  | n/a                   | cross-compile only                      |
| openEuler 24.03 LTS (GA … SP2)                           | x86_64, aarch64 | **1.21.4**                                                                                                | yes                                | **none** (only `firefox`)                                             | n/a in repo           | cross-compile; arm64 hosted runner      |
| openEuler 24.03 LTS SP3 / SP4                            | x86_64, aarch64 | **1.24.6** (`golang-1.24` alongside `golang-1.21.4`)                                                      | yes                                | **none**                                                              | n/a in repo           | cross-compile; arm64 hosted runner      |
| openEuler 24.03 LTS SP1 / SP2                            | loongarch64     | **1.21.4**                                                                                                | yes                                | none                                                                  | n/a                   | cross-compile only                      |
| openEuler 24.03 LTS SP3 / SP4                            | loongarch64     | **1.24.6** (SP4 `everything`)                                                                             | yes                                | none                                                                  | n/a                   | cross-compile only                      |

Notes on the table:

- The oldest system Go in scope is **Kylin V10 SP1 server: Go 1.13**; the oldest that is still
  shipping today across a whole current LTS line is **openEuler 22.03 LTS: Go 1.17.3**, unchanged
  from GA through SP4.
- **No target distribution ships a Chromium package on any architecture except Kylin V10 desktop
  (`chromium-browser` 83, amd64 and arm64).** openEuler ships no `chromium`/`chrome` package at
  all in `everything`, `update` or `EPOL/main`, on any of x86_64 / aarch64 / loongarch64.
- "CDP attach" is marked _not vendor-documented_ rather than "no": no vendor page found states
  whether `--remote-debugging-port` / `--headless` are accepted. See §3.

Non-target distributions, included only as evidence about what exists on `loong64` (these are
**not** in scope per #21, and are labelled as context):

| Distribution                    | Arch                                 | System Go                                             | Chromium-family browser                                     |
| ------------------------------- | ------------------------------------ | ----------------------------------------------------- | ----------------------------------------------------------- |
| Loongnix 25 (`loongnix-stable`) | loong64                              | 1.26.0 (`golang-1.26` 1.26.0-1.lnd.1), default 2:1.24 | **`lbrowser` 3.4.2293.2** + **`lbrowserdriver` 3.4.2293.2** |
| openKylin `huanghe`             | loong64, amd64                       | 1.26.0 (`golang-1.26` 1.26.0-ok1)                     | none (only `firefox` 125)                                   |
| deepin 23 `crimson`             | amd64, arm64, loong64, riscv64, i386 | 1.22.12 (`golang-1.22`)                               | none (only `firefox` 131)                                   |

---

## 1. Go toolchain on the platform

### 1.1 Kylin V10 (server line, `update.cs2c.com.cn/NS/V10/`)

The Kylin server repository tree is public and browsable. Its layout is
`NS/V10/<SP>/os/adv/lic/{base,updates}/<arch>/Packages/`, with `<arch>` ∈ {`x86_64`, `aarch64`,
`loongarch64`}. `loongarch64` exists only under `V10SP3`; `V10SP1` and `V10SP2` return 404 for it.

| SP     | repo    | x86_64 / aarch64                                | loongarch64                                         |
| ------ | ------- | ----------------------------------------------- | --------------------------------------------------- |
| V10SP1 | base    | `golang-1.13-3.3.ky10`                          | (no tree)                                           |
| V10SP1 | updates | no `golang` package                             | (no tree)                                           |
| V10SP2 | base    | `golang-1.13.15-1.p01.ky10`                     | (no tree)                                           |
| V10SP2 | updates | `golang-1.15.7-19.p01` … `golang-1.15.7-60.p01` | (no tree)                                           |
| V10SP3 | base    | `golang-1.15.7-9.p01.ky10`                      | `golang-1.15.7-9.p02.a.ky10`                        |
| V10SP3 | updates | `golang-1.15.7-22.p01` … `golang-1.15.7-60.p01` | `golang-1.15.7-24.p01.a` … `golang-1.15.7-60.p01.a` |

So a fully-updated Kylin V10 server, on any of the three architectures, has **Go 1.15.7**. Kylin
V10 SP1 without updates has **Go 1.13**.

Sources:
<https://update.cs2c.com.cn/NS/V10/V10SP1/os/adv/lic/base/x86_64/Packages/> ·
<https://update.cs2c.com.cn/NS/V10/V10SP2/os/adv/lic/updates/x86_64/Packages/> ·
<https://update.cs2c.com.cn/NS/V10/V10SP3/os/adv/lic/base/loongarch64/Packages/> ·
<https://update.cs2c.com.cn/NS/V10/V10SP3/os/adv/lic/updates/loongarch64/Packages/>

### 1.2 Kylin V10 desktop (`archive.kylinos.cn/kylin/KYLIN-ALL`, suite `10.1`)

The desktop archive is Debian-format. Its `Release` file (fetched 2026-09-05) reads:

```
Architectures: i386 arm64 loongarch64 mips64el sw64 amd64 armhf
Codename: 10.1
Components: main restricted multiverse universe
Date: Sat, 11 Apr 2026 14:04:36 +0000
Description: Kylin Desktop 10.1 Desktop V10.1 10.1
Origin: Kylin
```

`main` component contents:

| Arch        | default `golang`   | versioned packages                                                                             |
| ----------- | ------------------ | ---------------------------------------------------------------------------------------------- |
| amd64       | `2:1.13~1kylin2k1` | `golang-1.10` 1.10-1kylin1, `golang-1.13` 1.13.8-1kylin1, `golang-1.14` 1.14.3-2kylin2~20.04.2 |
| arm64       | `2:1.13~1kylin2k1` | same as amd64                                                                                  |
| loongarch64 | `2:1.15~1.lnd.1`   | `golang-1.10`, `golang-1.13`, `golang-1.14`, `golang-1.15` 1.15.6-1.lnd.9                      |

Sources:
<http://archive.kylinos.cn/kylin/KYLIN-ALL/dists/10.1/Release> ·
<http://archive.kylinos.cn/kylin/KYLIN-ALL/dists/10.1/main/binary-amd64/Packages.gz> ·
<http://archive.kylinos.cn/kylin/KYLIN-ALL/dists/10.1/main/binary-arm64/Packages.gz> ·
<http://archive.kylinos.cn/kylin/KYLIN-ALL/dists/10.1/main/binary-loongarch64/Packages.gz>

### 1.3 UOS V20 — **not found**

UOS's own package repositories are credential-gated. On 2026-09-05:

- `https://professional-packages.chinauos.com/desktop-professional/dists/` → **HTTP 401**
- `https://home-packages.chinauos.com/home/dists/` → **HTTP 401**
- `https://professional-packages.chinauos.com/` → HTTP 404
- `https://packages.chinauos.com/`, `https://euler-packages.chinauos.com/` → HTTP 404

The Go version and browser inventory of UOS V20 desktop/server therefore **could not be verified
from a primary source**. The nearest publicly readable base is the deepin community archive that
UOS 20 shares a Debian 10 lineage with, suite `apricot` (amd64 only in the public archive):

- `golang` `2:1.15.1~1`, `golang-go` `2:1.15.1~1`, `golang-1.15` `1.15.9.1-1+dde`,
  `golang-1.11` `1.11.6.1-1+dde`
- `chromium` `83.0.4103.116-1~deb10u3`
- `non-free`: `google-chrome-stable` `76.0.3809.132-1`

This is a _related_ archive, not UOS's; do not read it as a statement about UOS itself.

Sources:
<https://community-packages.deepin.com/deepin/dists/apricot/main/binary-amd64/Packages.gz> ·
<https://community-packages.deepin.com/deepin/dists/apricot/non-free/binary-amd64/Packages.gz>

For contrast, deepin 23 (`beige` / `crimson`, arches amd64, arm64, i386, **loong64**, riscv64)
ships `golang` `2:1.22~3deepin5`, `golang-1.22` `1.22.12-3deepin1`, and **no** `chromium` package
(only `firefox` 131.0.3).
Source: <https://community-packages.deepin.com/beige/dists/crimson/main/binary-loong64/Packages.gz>

### 1.4 openEuler 22.03 LTS and 24.03 LTS

Read from the Tsinghua TUNA mirror of `repo.openeuler.org` (the openEuler origin refused TLS from
this network; TUNA is a byte mirror of the same tree).

| Release       | `everything` (x86_64 / aarch64)                | newest in `update`                             | loongarch64 tree                                        |
| ------------- | ---------------------------------------------- | ---------------------------------------------- | ------------------------------------------------------- |
| 22.03-LTS     | `golang-1.17.3-1.oe2203`                       | `golang-1.17.3-32`                             | yes, `golang-1.17.3-18.oe2203.loongarch64`              |
| 22.03-LTS-SP1 | `golang-1.17.3-12`                             | `golang-1.17.3-37`                             | no (404)                                                |
| 22.03-LTS-SP2 | `golang-1.17.3-19`                             | `golang-1.17.3-32`                             | no (404)                                                |
| 22.03-LTS-SP3 | `golang-1.17.3-26`                             | `golang-1.17.3-46`                             | no (404)                                                |
| 22.03-LTS-SP4 | `golang-1.17.3-34`                             | `golang-1.17.3-49`                             | no (404)                                                |
| 24.03-LTS     | `golang-1.21.4-8.oe2403`                       | `golang-1.21.4-44`                             | no (404)                                                |
| 24.03-LTS-SP1 | `golang-1.21.4-28`                             | `golang-1.21.4-45`                             | **yes**, `golang-1.21.4-28.oe2403sp1.loongarch64`       |
| 24.03-LTS-SP2 | `golang-1.21.4-32`                             | `golang-1.21.4-43`                             | yes, `golang-1.21.4-32`                                 |
| 24.03-LTS-SP3 | `golang-1.21.4-44`                             | `golang-1.21.4-48`, **`golang-1.24-1.24.6-2`** | yes, `golang-1.21.4-44`                                 |
| 24.03-LTS-SP4 | `golang-1.21.4-48`, **`golang-1.24-1.24.6-6`** | `golang-1.21.4-50`, `golang-1.24.2-49`         | yes, both `golang-1.21.4-48` and `golang-1.24-1.24.6-6` |

Key points: **openEuler 22.03 LTS is pinned to Go 1.17.3 for its entire life** — GA through SP4,
base and updates, on every architecture. openEuler 24.03 LTS starts at Go 1.21.4 and adds a
parallel-installable `golang-1.24` (Go 1.24.6) from SP3 onward, including on `loongarch64` in SP4.

Sources (representative; the same path pattern was queried for every release/repo/arch):
<https://repo.openeuler.org/openEuler-22.03-LTS/everything/x86_64/Packages/> ·
<https://mirrors.tuna.tsinghua.edu.cn/openeuler/openEuler-22.03-LTS/everything/loongarch64/Packages/> ·
<https://mirrors.tuna.tsinghua.edu.cn/openeuler/openEuler-24.03-LTS/everything/x86_64/Packages/> ·
<https://mirrors.tuna.tsinghua.edu.cn/openeuler/openEuler-24.03-LTS-SP4/everything/loongarch64/Packages/> ·
<https://mirrors.tuna.tsinghua.edu.cn/openeuler/openEuler-24.03-LTS-SP3/update/x86_64/Packages/>

### 1.5 Official Go tarballs, including `linux-loong64`

`https://go.dev/dl/?mode=json&include=all` (365 releases listed) contains
`goX.Y.Z.linux-loong64.tar.gz` files starting at **`go1.21rc2` / `go1.21.0`**, continuously
through `go1.27.1`. There is **no** `linux-loong64` file for any release before `go1.21rc2`.

`golang.google.cn` — the mainland-accessible mirror of the download site — serves the same set.
Verified directly on 2026-09-05 with range requests:

| URL                                                          | Result                     |
| ------------------------------------------------------------ | -------------------------- |
| `https://golang.google.cn/dl/go1.27.1.linux-loong64.tar.gz`  | HTTP 206, 68 524 216 bytes |
| `https://golang.google.cn/dl/go1.21.0.linux-loong64.tar.gz`  | HTTP 206, 63 913 749 bytes |
| `https://golang.google.cn/dl/go1.20.14.linux-loong64.tar.gz` | **HTTP 404**               |
| `https://golang.google.cn/dl/go1.27.1.linux-arm64.tar.gz`    | HTTP 206, 67 009 954 bytes |

`https://golang.google.cn/dl/?mode=json` lists current stable releases `go1.27.1` and `go1.26.8`,
each with linux arches `386, amd64, arm64, armv6l, loong64, mips, mips64, mips64le, mipsle, ppc64,
ppc64le, riscv64, s390x`.

So: **`linux/loong64` has been an officially distributed binary release since Go 1.21**, even
though the port itself dates to Go 1.19 (see §2).

Sources: <https://go.dev/dl/?mode=json&include=all> · <https://golang.google.cn/dl/?mode=json> ·
<https://golang.google.cn/dl/>

### 1.6 `GOTOOLCHAIN` behaviour behind mainland networks

From the Go toolchain documentation:

> "Starting in Go 1.21, the Go distribution consists of a `go` command and a bundled Go toolchain,
> which is the standard library as well as the compiler, assembler, and other tools."

> "In standard Go toolchains, the `$GOROOT/go.env` file sets the default `GOTOOLCHAIN=auto`"

> "When a command encounters a module requiring a newer Go version and `GOTOOLCHAIN` permits
> running different toolchains (it is one of the `auto` or `path` forms), the `go` command chooses
> and switches to an appropriate newer toolchain to continue executing the current command."

> "These toolchains are packaged as special modules with module path `golang.org/toolchain` and
> version `v0.0.1-go_VERSION_._GOOS_-_GOARCH_`. Toolchains are downloaded like any other module,
> meaning that toolchain downloads can be proxied by setting `GOPROXY` and have their checksums
> checked by the Go checksum database."

> "toolchain downloads fail for lack of verification if `GOSUMDB=off`. `GOPRIVATE` and `GONOSUMDB`
> patterns do not apply to the toolchain downloads."

> "When `GOTOOLCHAIN` is set to `local`, the `go` command always runs the bundled Go toolchain."

Behaviour before Go 1.21, quoted from the same page:

> "Before Go 1.21, Go toolchains treated the `go` line as an advisory requirement: if builds
> succeeded the toolchain assumed everything worked, and if not it printed a note about the
> potential version mismatch. Go 1.21 changed the `go` line to be a mandatory requirement instead.
> This behavior is partly backported to earlier language versions: Go 1.19 releases starting at Go
> 1.19.13 and Go 1.20 releases starting at Go 1.20.8, refuse to load workspaces or modules
> declaring version Go 1.22 or later."

**Consequence for the oldest targets.** `GOTOOLCHAIN` does not exist before Go 1.21. On Kylin V10
(Go 1.13 / 1.15.7) and openEuler 22.03 LTS (Go 1.17.3), a `go.mod` `go` directive newer than the
installed toolchain triggers **no download attempt at all**: those toolchains predate the
mechanism, so the build either succeeds (if the source happens to compile) or fails with the
version-mismatch note, per the quote above.

**Does goproxy.cn carry toolchain modules?** Yes. Verified live on 2026-09-05:

| Request                                                                             | Result                                                                             |
| ----------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------- |
| `GET https://goproxy.cn/golang.org/toolchain/@v/list`                               | HTTP 200, list beginning `v0.0.1-go1.2.2.darwin-386` …                             |
| `GET https://goproxy.cn/golang.org/toolchain/@v/v0.0.1-go1.24.0.linux-amd64.info`   | HTTP 200 `{"Version":"v0.0.1-go1.24.0.linux-amd64","Time":"2025-02-10T23:33:55Z"}` |
| `GET https://goproxy.cn/golang.org/toolchain/@v/v0.0.1-go1.24.0.linux-arm64.info`   | HTTP 200                                                                           |
| `GET https://goproxy.cn/golang.org/toolchain/@v/v0.0.1-go1.24.0.linux-loong64.info` | HTTP 200                                                                           |
| `GET https://goproxy.cn/sumdb/sum.golang.org/supported`                             | HTTP 200                                                                           |
| `GET https://goproxy.cn/sumdb/sum.golang.org/latest`                                | HTTP 200, signed tree head `— sum.golang.org …`                                    |

So with `GOPROXY=https://goproxy.cn,direct`, a Go ≥ 1.21 toolchain on a mainland network can both
download the `golang.org/toolchain` module for `linux-amd64`, `linux-arm64` **and `linux-loong64`**,
and satisfy the mandatory checksum-database verification through the proxy's `/sumdb/` endpoint.
With the default `GOPROXY=https://proxy.golang.org,direct` and that host unreachable, the download
step is what fails; the failure is a download error, not a silent fallback.

Sources: <https://go.dev/doc/toolchain> · <https://goproxy.cn/> and the URLs in the table above.

---

## 2. Go port status for `linux/loong64`

**Added in Go 1.19.** From the Go 1.19 release notes, verbatim:

> "Go 1.19 adds support for the Loongson 64-bit architecture LoongArch on Linux (`GOOS=linux`,
> `GOARCH=loong64`). The implemented ABI is LP64D. Minimum kernel version supported is 5.19.
>
> Note that most existing commercial Linux distributions for LoongArch come with older kernels,
> with a historical incompatible system call ABI. Compiled binaries will not work on these systems,
> even if statically linked. Users on such unsupported systems are limited to the
> distribution-provided Go package."

That second paragraph is the "old world" problem stated by the Go project itself, and it is why
the distro-provided `golang` package matters on LoongArch even when official tarballs exist.

**Minimum requirements**, from the Go wiki:

> "For loong64, kernel 5.19 and later versions work fine."

> "the Go compiler always generated Loong64 binaries that could be executed any processor cored by
> LA364, LA464, LA664 or later."

with the processor mapping given there: **LA464** = "loongson-3A5000/3C5000/3D5000", **LA664** =
"loongson-3A6000/3C6000" — i.e. both CPUs named in #21 are covered.

**Feature support, read from the Go source of record** (`src/internal/platform` at `master`,
2026-09-05):

| Capability                    | `linux/loong64` | `linux/arm64`                | Source symbol                                           |
| ----------------------------- | --------------- | ---------------------------- | ------------------------------------------------------- |
| cgo                           | **yes**         | yes                          | `distInfo`: `{"linux","loong64"}: {CgoSupported: true}` |
| first-class port              | **no**          | **yes** (`FirstClass: true`) | same map                                                |
| broken port                   | no              | no                           | same map                                                |
| race detector                 | **yes**         | yes                          | `RaceDetectorSupported`                                 |
| msan                          | yes             | yes                          | `MSanSupported`                                         |
| asan                          | yes             | yes                          | `ASanSupported`                                         |
| fuzz coverage instrumentation | yes             | yes                          | `FuzzInstrumented`                                      |
| `-buildmode=c-archive`        | yes             | yes                          | `BuildModeSupported`                                    |
| `-buildmode=c-shared`         | yes             | yes                          | `BuildModeSupported`                                    |
| `-buildmode=pie`              | yes             | yes                          | `BuildModeSupported`                                    |
| `-buildmode=plugin`           | yes             | yes                          | `BuildModeSupported`                                    |
| `-buildmode=shared`           | **no**          | yes                          | `BuildModeSupported`                                    |
| internally-linked PIE         | yes             | yes                          | `InternalLinkPIESupported`                              |

The one entry that is _absent_ for `loong64` and present for `arm64` is `FirstClass`. The Go
porting policy defines a first-class port by two properties — **"Broken builds block releases"**
and **"Installation is documented at https://go.dev/doc/install"** — and lists exactly eight such
ports: `darwin/amd64`, `darwin/arm64`, `linux/386`, `linux/amd64`, `linux/arm`, `linux/arm64`,
`windows/386`, `windows/amd64`. `linux/loong64` is not among them, so a `loong64` regression does
not block a Go release. (The same page notes "All Linux first class ports are for systems using
glibc only"; Kylin, UOS and openEuler are all glibc systems.) The concrete build-time limitation
for `loong64` is `-buildmode=shared`, which a CDP driver does not use. cgo, the race detector and
`-buildmode=plugin` are all available.

Sources:
<https://go.dev/doc/go1.19> ·
<https://go.dev/wiki/MinimumRequirements> ·
<https://github.com/golang/go/blob/master/src/internal/platform/supported.go> ·
<https://github.com/golang/go/blob/master/src/internal/platform/zosarch.go> ·
<https://go.dev/wiki/PortingPolicy#first-class-ports>

---

## 3. Browsers

### 3.1 Distribution-packaged Chromium

| Distribution                                                                            | Arch                         | Package                           | Version                                                                                                                                                                             |
| --------------------------------------------------------------------------------------- | ---------------------------- | --------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Kylin V10 desktop `10.1`, `universe`                                                    | amd64                        | `chromium-browser`                | **83.0.4103.61-0kylin0.18.04.1k1**                                                                                                                                                  |
| Kylin V10 desktop `10.1`, `universe`                                                    | arm64                        | `chromium-browser`                | **83.0.4103.61-0kylin0.18.04.1k1**                                                                                                                                                  |
| Kylin V10 desktop `10.1`, `universe`                                                    | loongarch64                  | —                                 | **none**                                                                                                                                                                            |
| Kylin V10 server SP1/SP2/SP3                                                            | all                          | —                                 | **none**; a substring search of the whole `V10SP3` `base` x86_64 package index for `chrom` returns only `libchromaprint`, `mathjax-winchrome-fonts` and `texlive-context-chromato*` |
| openEuler 22.03-LTS-SP4, 24.03-LTS, 24.03-LTS-SP4 (`everything`, `update`, `EPOL/main`) | x86_64, aarch64, loongarch64 | —                                 | **none**; only `firefox` (102 / 115 / 128 / 140). A substring search of `openEuler-24.03-LTS-SP4/everything/x86_64/Packages/` for `chromium` returns nothing                        |
| deepin 20 `apricot` (UOS 20's public Debian-10-era sibling)                             | amd64                        | `chromium`                        | 83.0.4103.116-1~deb10u3                                                                                                                                                             |
| deepin 23 `crimson`                                                                     | amd64, arm64, loong64        | —                                 | none                                                                                                                                                                                |
| Loongnix 25 `loongnix-stable` _(context, not a target)_                                 | loong64                      | **`lbrowser` / `lbrowserdriver`** | **3.4.2293.2-1.stable**                                                                                                                                                             |

Sources:
<http://archive.kylinos.cn/kylin/KYLIN-ALL/dists/10.1/universe/binary-amd64/Packages.gz> ·
<http://archive.kylinos.cn/kylin/KYLIN-ALL/dists/10.1/universe/binary-arm64/Packages.gz> ·
<http://archive.kylinos.cn/kylin/KYLIN-ALL/dists/10.1/universe/binary-loongarch64/Packages.gz> ·
<https://update.cs2c.com.cn/NS/V10/V10SP3/os/adv/lic/base/x86_64/Packages/> ·
<https://mirrors.tuna.tsinghua.edu.cn/openeuler/openEuler-24.03-LTS-SP4/everything/x86_64/Packages/> ·
<https://pkg.loongnix.cn/loongnix/25/dists/loongnix-stable/main/binary-loong64/Packages.gz>

The `lbrowser` package stanza is worth recording because it is the only Chromium-family browser
found for `loong64` in any repository surveyed:

```
Package: lbrowser
Version: 3.4.2293.2-1.stable
Architecture: loong64
Maintainer: LBrowser Team <browser@loongson.cn>
Installed-Size: 378015
Depends: … libgbm1 (>= 8.1~0), libgtk-3-0 (>= 3.9.10) | libgtk-4-1, libnspr4, libnss3, …
Filename: pool/main/l/lbrowser/lbrowser_3.4.2293.2-1.stable_loong64.deb
Size: 100355388
Description: The web browser from the Loongnix Community
```

and next to it, in the same index, `lbrowserdriver` at the same version — a ChromeDriver-shaped
companion. Loongson's own documentation describes lbrowser as "基于 chromium 内核" (built on the
Chromium engine) and compatible with "龙芯、鲲鹏、兆芯、飞腾" processors and "UOS、麒麟" operating
systems, but **does not state which Chromium major version** it corresponds to, and its
documentation tree (安装更新 / 使用手册 / 常见问题 / 技术支持 / 发行注记) contains **no page on
command-line switches, automation, or the driver**. Latest release listed there is 3.4.2293.2,
dated 2026-06-08.
Sources: <https://docs.loongnix.cn/lbrowser/> · <https://docs.loongnix.cn/lbrowser/Release_notes/>

### 3.2 Vendor browsers

| Browser                           | Chromium major                                           | Evidence                                                    | Platforms named by the vendor                                                                                           |
| --------------------------------- | -------------------------------------------------------- | ----------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------- |
| 奇安信可信浏览器 先锋版 (Qianxin) | **132**                                                  | Qianxin news, 2024-12-25                                    | "已正式上架麒麟应用商店、统信 UOS 应用商店" (Kylin and UOS app stores)                                                  |
| 奇安信可信浏览器 (earlier)        | 110                                                      | Qianxin news 2023-01-16 / Kylin news                        | 银河麒麟操作系统, Kylin app store                                                                                       |
| 360 安全浏览器 V10（信创版）      | **95**                                                   | 360 product page: "360 安全浏览器搭载 Chromium 95 内核版本" | "全面支持信创主流 CPU、操作系统" — no specific CPU/OS list on the page                                                  |
| 红莲花 (海泰方圆)                 | "最新的 Chromium 内核" — **no version number published** | vendor marketing page                                       | claims adaptation to 龙芯 (MIPS and LoongArch), 飞腾, 兆芯, 鲲鹏, 海光, 申威 and to UOS / 麒麟 / 中科方德 / 普华 / 深度 |
| UOS 浏览器 (统信浏览器)           | **not found**                                            | —                                                           | —                                                                                                                       |

**Whether CDP can attach to any of these: not found.** No first-party documentation was located
for 奇安信可信浏览器, 360 安全浏览器信创版, UOS 浏览器, 红莲花, or lbrowser that mentions
`--remote-debugging-port`, `--headless`, DevTools remote debugging, or an automation interface.
The existence of `lbrowserdriver` alongside `lbrowser` is the only concrete signal in that
direction, and it is a WebDriver-shaped artifact, not a documented CDP endpoint.

For reference, what "CDP can attach" requires, from the protocol's own documentation: the browser
is started with `--remote-debugging-port=9222`, and the client reads `webSocketDebuggerUrl` from
`/json/version` (or the target list at `/json` / `/json/list`) and connects over WebSocket.

One version-related fact that bears on every old Chromium above: since **Chrome 132.0.6793.0**,
"the old Headless mode is only available as a standalone binary named `chrome-headless-shell`";
headless was reworked in Chrome 112.

Sources:
<https://www.qianxin.com/news/detail?news_id=12893> ·
<https://www.kylinos.cn/about/news/1943512216392765442.html> ·
<https://www.360.net/product-center/Endpoint-Security/v10> ·
<https://browser.360.net/gc/index.html> ·
<https://www.haitaichina.com/hlhxcllqglhxcllq/index.htm> ·
<https://chromedevtools.github.io/devtools-protocol/> ·
<https://developer.chrome.com/docs/chromium/headless>

### 3.3 Chrome for Testing, Chromium snapshots, Playwright builds

**Chrome for Testing.** The full `known-good-versions-with-downloads.json` (2 504 versions on
2026-09-05) was parsed. First appearance of each platform:

| Artifact                | `linux64`    | `linux-arm64`    |
| ----------------------- | ------------ | ---------------- |
| `chrome`                | 113.0.5672.0 | **153.0.8001.0** |
| `chrome-headless-shell` | 120.0.6098.0 | **153.0.8001.0** |
| `chromedriver`          | 115.0.5763.0 | **153.0.8001.0** |

42 versions carry `chrome` for `linux-arm64`, from 153.0.8001.0 to 155.0.8043.0. **Zero versions
carry any `loong` platform.** In `last-known-good-versions-with-downloads.json`, the **Stable**
channel (152.0.7977.82, revision 1669021) offers only `linux64, mac-arm64, mac-x64, win32, win64`
— **no `linux-arm64`**; Beta (154.0.8037.0), Dev (155.0.8040.2) and Canary (155.0.8043.0) do offer
`linux-arm64`. So `linux-arm64` Chrome for Testing exists today only on the pre-stable channels.

Sources:
<https://googlechromelabs.github.io/chrome-for-testing/known-good-versions-with-downloads.json> ·
<https://googlechromelabs.github.io/chrome-for-testing/last-known-good-versions-with-downloads.json>

**Chromium snapshots.** The top-level prefixes in the `chromium-browser-snapshots` GCS bucket are:

```
Android/ AndroidDesktop_arm64/ AndroidDesktop_x64/ Android_Arm64/ Arm/ Linux/ LinuxGit/
LinuxGit_x64/ Linux_ARM_Cross-Compile/ Linux_ChromiumOS/ Linux_ChromiumOS_Full/ Linux_x64/
Mac/ MacGit/ Mac_Arm/ Win/ WinGit/ Win_Arm64/ Win_x64/ …
```

There is **no `Linux_ARM64` prefix and no LoongArch prefix**. This is exactly why upstream's
`hostConf` map has no `linux_arm64` entry (see §4).
Source: <https://commondatastorage.googleapis.com/chromium-browser-snapshots/?delimiter=/&prefix=>

**Playwright builds.** Playwright's stated system requirements are:

> "Debian 12 / 13, Ubuntu 22.04 / 24.04 / 26.04 (x86-64 or arm64)."

Its CDN does publish `chromium-linux-arm64.zip`. Probed on 2026-09-05 via
`https://cdn.playwright.dev/dbazure/download/playwright/builds/chromium/<build>/<file>` (the host
`playwright.azureedge.net` that upstream uses 307-redirects to
`playwright.download.prss.microsoft.com`):

| build                                      | `chromium-linux.zip` | `chromium-linux-arm64.zip`    | `chromium-loong64.zip` |
| ------------------------------------------ | -------------------- | ----------------------------- | ---------------------- |
| **1124** (upstream's `RevisionPlaywright`) | 206                  | **400 — absent** (checked 3×) | 400                    |
| 1128                                       | —                    | 206                           | 400                    |
| 1133                                       | —                    | 206                           | 400                    |
| 1139, 1140, 1155, 1169, 1181, 1200         | 206                  | 206                           | 400                    |

So Playwright arm64 Chromium builds exist, **but not for the exact revision go-rod pins**, and
there is no LoongArch artifact at any revision probed.
Sources: <https://playwright.dev/docs/intro> · the CDN URLs above.

---

## 4. Dependencies with per-architecture artifacts

### 4.1 `ysmood/leakless`

`leakless` embeds its guard executable as gzip+base64 blobs in generated Go files, one per target.
The repository (`main`, last pushed 2024-07-29) contains exactly five:

| File                   | Size (bytes of Go source) |
| ---------------------- | ------------------------- |
| `bin_amd64_linux.go`   | 1 273 300                 |
| `bin_arm64_linux.go`   | 1 185 796                 |
| `bin_amd64_darwin.go`  | 1 316 321                 |
| `bin_arm64_darwin.go`  | 1 302 261                 |
| `bin_amd64_windows.go` | 1 256 602                 |

and `cmd/pack/targets.go` states the list verbatim:

```go
var targets = []utils.Target{
	"linux/amd64",
	"linux/arm64",
	"darwin/amd64",
	"darwin/arm64",
	"windows/amd64",
}
```

Lookup key is `GOARCH + "_" + GOOS` (`pkg/utils/target.go`, `Target.BinName`). **Coverage is
therefore: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64 — and nothing else.
No `loong64`, no `linux/386`, no `linux/arm`.**

`leakless.go` exposes the escape hatch:

```go
// Support returns true if the OS is supported by leakless.
func Support() bool {
	_, has := leaklessBinaries[utils.GetTarget().BinName()]
	return has
}
```

and `GetLeaklessBin()` would panic (via `utils.E`) on an unsupported target — but it is never
reached, because upstream guards the call. In `lib/launcher/launcher.go`:

```go
if l.Has(flags.Leakless) && leakless.Support() {
	ll = leakless.New()
	cmd = ll.Command(bin, args...)
} else {
	…
	cmd = exec.Command(bin, args...)
}
```

**Consequence on `loong64`: the build succeeds and the launch succeeds, but silently without the
zombie-process guard.** The readme documents the supported workaround: fork leakless, edit
`targets.go`, `go generate`, and use a `replace` directive.

Sources:
<https://github.com/ysmood/leakless/blob/main/cmd/pack/targets.go> ·
<https://github.com/ysmood/leakless/blob/main/pkg/utils/target.go> ·
<https://github.com/ysmood/leakless/blob/main/leakless.go> ·
<https://github.com/ysmood/leakless/blob/main/readme.md> ·
<https://github.com/go-rod/rod/blob/main/lib/launcher/launcher.go>

### 4.2 Arch-specific code paths in upstream `lib/`

A code search over `go-rod/rod` for `GOARCH` returns exactly one file: `lib/launcher/browser.go`.
It contains three arch-sensitive constructs.

**(a) `hostConf` — the download URL table.** Verbatim:

```go
var hostConf = map[string]struct {
	urlPrefix string
	zipName   string
}{
	"darwin_amd64":  {"Mac", "chrome-mac.zip"},
	"darwin_arm64":  {"Mac_Arm", "chrome-mac.zip"},
	"linux_amd64":   {"Linux_x64", "chrome-linux.zip"},
	"windows_386":   {"Win", "chrome-win.zip"},
	"windows_amd64": {"Win_x64", "chrome-win.zip"},
}[runtime.GOOS+"_"+runtime.GOARCH]
```

There is **no `linux_arm64` and no `linux_loong64` key**. On those platforms `hostConf` is the zero
value, so `HostGoogle` and `HostNPM` format URLs with an empty path segment and empty archive
name — they cannot resolve to a real artifact. (§3.3 explains why: the snapshot bucket has no
`Linux_ARM64` prefix to point at.)

**(b) `HostPlaywright` — the arm64-only fallback.** Verbatim:

```go
func HostPlaywright(revision int) string {
	rev := RevisionPlaywright
	if !(runtime.GOOS == "linux" && runtime.GOARCH == "arm64") {
		rev = revision
	}
	return fmt.Sprintf(
		"https://playwright.azureedge.net/builds/chromium/%d/chromium-linux-arm64.zip",
		rev,
	)
}
```

with `lib/launcher/revision.go`:

```go
const RevisionDefault = 1321438
const RevisionPlaywright = 1124  // "RevisionPlaywright for arm linux."
```

This is the only path by which upstream can obtain a browser on `linux/arm64`, and **as of
2026-09-05 that exact URL returns HTTP 400 — Playwright build 1124 no longer carries a
`chromium-linux-arm64.zip`** (§3.3). Neighbouring builds do. On `loong64` the function is
irrelevant: it always formats an `arm64` file name.

**(c) OS-keyed (not arch-keyed) maps** — `DefaultBrowserDir`, `Browser.BinPath` and `LookPath` are
all keyed on `runtime.GOOS` only, so they behave identically on every Linux arch. `LookPath`'s
Linux candidate list is `chrome, google-chrome, /usr/bin/google-chrome, microsoft-edge,
/usr/bin/microsoft-edge, chromium, chromium-browser, google-chrome-stable,
/usr/bin/google-chrome-stable, /usr/bin/chromium, /usr/bin/chromium-browser, /snap/bin/chromium,
/data/data/com.termux/files/usr/bin/chromium-browser` — it contains **no domestic browser path**
(no `lbrowser`, no Qianxin, no 360, no UOS browser).

`Browser.Validate()` additionally runs the found binary with `--headless --no-sandbox
--use-mock-keychain --disable-dev-shm-usage --disable-gpu --dump-dom about:blank` and requires the
output to contain `<html><head></head><body></body></html>`, treating "error while loading shared
libraries" as acceptable.

`lib/utils` contains no `GOARCH` branch. `lib/launcher/os_unix.go` / `os_windows.go` split on OS,
not arch.

Sources: <https://github.com/go-rod/rod/blob/main/lib/launcher/browser.go> ·
<https://github.com/go-rod/rod/blob/main/lib/launcher/revision.go>

---

## 5. Verification means

### 5.1 GitHub Actions

**arm64:** GitHub-hosted arm64 runners are generally available for public repositories as of
**2025-08-07**, with the labels `ubuntu-24.04-arm`, `ubuntu-22.04-arm` and `windows-11-arm`:

> "Developers can take advantage of the performance benefits of using arm64 processors or run their
> multi-architecture builds at no cost."

> "These runners are only available in public repositories and will not work in private
> repositories."

That public-repository-only restriction was lifted on **2026-01-29**: "Linux and Windows arm64
standard GitHub-hosted runners are now supported in all repositories", same three labels.

**loong64: no hosted runner exists.** Nor is QEMU user-mode emulation available out of the box:
`tonistiigi/binfmt` — the image behind `docker/setup-qemu-action` — documents its supported list as

```
"supported": ["linux/amd64","linux/arm64","linux/riscv64","linux/ppc64le","linux/s390x","linux/386","linux/arm/v7","linux/arm/v6"],
"emulators": ["qemu-aarch64","qemu-arm","qemu-i386","qemu-ppc64le","qemu-riscv64","qemu-s390x"]
```

with **no LoongArch entry**. Verifying `loong64` in Actions therefore requires either a
self-hosted runner or a hand-built QEMU/binfmt setup.

What is available for free on any runner is **cross-compilation**: `linux/loong64` is a recognised
target of `go tool dist list` since Go 1.19 (§2), so `GOOS=linux GOARCH=loong64 go build` is a
build-only check that runs anywhere.

Sources:
<https://github.blog/changelog/2025-08-07-arm64-hosted-runners-for-public-repositories-are-now-generally-available/> ·
<https://github.blog/changelog/2026-01-29-arm64-standard-runners-are-now-available-in-private-repositories/> ·
<https://github.com/tonistiigi/binfmt> ·
<https://docs.github.com/actions/reference/github-hosted-runners-reference>

### 5.2 What chromedp does

`.github/workflows/test.yml` is the whole of chromedp's CI, verbatim in the parts that matter:

```yaml
strategy:
  matrix:
    go-version: [oldstable, stable]
runs-on: ubuntu-latest
```

**No arm64 and no loong64 job.** chromedp does not download browsers itself; its companion image
`chromedp/headless-shell` is however multi-arch. The Docker Hub manifest for tag `stable`
(last updated 2026-08-11) lists exactly two images:

```
linux amd64  150339807 bytes
linux arm64 (v8)  146143842 bytes
```

Its README states: "The headless-shell project provides a multi-arch container image … Multi-arch
images for Chrome's `stable`, `beta`, and `dev` channels are pushed daily."

Sources:
<https://github.com/chromedp/chromedp/blob/main/.github/workflows/test.yml> ·
<https://github.com/chromedp/docker-headless-shell> ·
<https://hub.docker.com/v2/repositories/chromedp/headless-shell/tags/stable>

### 5.3 What playwright-go does

`.github/workflows/build.yml`:

```yaml
strategy:
  fail-fast: false
  matrix:
    os: [ubuntu-latest, windows-latest, macos-latest]
    browser: [chromium, firefox, webkit]
    go: ['stable', 'oldstable']
```

**No arm64 and no loong64 job.** playwright-go relies on the upstream Playwright driver, whose own
stated support is "Debian 12 / 13, Ubuntu 22.04 / 24.04 / 26.04 (x86-64 or arm64)" — arm64 yes,
LoongArch not mentioned.

Sources:
<https://github.com/playwright-community/playwright-go/blob/main/.github/workflows/build.yml> ·
<https://playwright.dev/docs/intro>

---

## What could not be established

- **UOS V20 desktop and server: system Go version, packaged browser inventory, architecture list.**
  UOS's package repositories require credentials (HTTP 401). No first-party alternative was found.
- **The Chromium major version behind lbrowser, UOS 浏览器, and 红莲花.** None of the three vendors
  publishes it in the documentation surveyed.
- **Whether any domestic vendor browser accepts `--remote-debugging-port` / `--headless`.** No
  vendor documentation or first-party statement was found either way.
- **Loongnix / openKylin / deepin figures in this document are context, not targets.** They are
  included only as evidence about what exists on `loong64` and are labelled as such.

---

## Sources

Go project

- <https://go.dev/doc/toolchain>
- <https://go.dev/doc/go1.19>
- <https://go.dev/doc/go1.20>
- <https://go.dev/wiki/MinimumRequirements>
- <https://go.dev/wiki/PortingPolicy#first-class-ports>
- <https://go.dev/dl/?mode=json&include=all>
- <https://golang.google.cn/dl/> · <https://golang.google.cn/dl/?mode=json>
- <https://github.com/golang/go/blob/master/src/internal/platform/supported.go>
- <https://github.com/golang/go/blob/master/src/internal/platform/zosarch.go>
- <https://goproxy.cn/>

Distribution repositories

- <https://update.cs2c.com.cn/NS/V10/> (Kylin V10 server, SP1 / SP2 / SP3, `base` and `updates`, x86_64 / aarch64 / loongarch64)
- <http://archive.kylinos.cn/kylin/KYLIN-ALL/dists/10.1/> (Kylin V10 desktop, `Release`, `main` and `universe`, amd64 / arm64 / loongarch64)
- <https://repo.openeuler.org/> and its mirror <https://mirrors.tuna.tsinghua.edu.cn/openeuler/> (22.03-LTS GA–SP4, 24.03-LTS GA–SP4; `everything`, `update`, `EPOL/main`; x86_64 / aarch64 / loongarch64)
- <https://professional-packages.chinauos.com/desktop-professional/dists/> and <https://home-packages.chinauos.com/home/dists/> (UOS — HTTP 401)
- <https://community-packages.deepin.com/deepin/dists/apricot/> and <https://community-packages.deepin.com/beige/dists/crimson/>
- <https://pkg.loongnix.cn/loongnix/25/dists/loongnix-stable/> (context)
- <https://mirrors.nju.edu.cn/openkylin/dists/huanghe/> (context)

Browsers

- <https://www.qianxin.com/news/detail?news_id=12893> (Chromium 132, 2024-12-25)
- <https://www.qianxin.com/news/detail?news_id=7916> (Chromium 110)
- <https://www.kylinos.cn/about/news/1943512216392765442.html> (Kylin app store, 2023-01-16)
- <https://www.360.net/product-center/Endpoint-Security/v10> (Chromium 95)
- <https://browser.360.net/gc/index.html>
- <https://www.haitaichina.com/hlhxcllqglhxcllq/index.htm> (红莲花)
- <https://docs.loongnix.cn/lbrowser/> · <https://docs.loongnix.cn/lbrowser/Release_notes/>
- <https://chromedevtools.github.io/devtools-protocol/>
- <https://developer.chrome.com/docs/chromium/headless>
- <https://googlechromelabs.github.io/chrome-for-testing/known-good-versions-with-downloads.json>
- <https://googlechromelabs.github.io/chrome-for-testing/last-known-good-versions-with-downloads.json>
- <https://commondatastorage.googleapis.com/chromium-browser-snapshots/?delimiter=/&prefix=>
- <https://playwright.dev/docs/intro>
- `https://cdn.playwright.dev/dbazure/download/playwright/builds/chromium/<build>/chromium-linux-arm64.zip` and `https://playwright.azureedge.net/builds/chromium/1124/chromium-linux-arm64.zip`

Dependencies and upstream code

- <https://github.com/ysmood/leakless> (`cmd/pack/targets.go`, `pkg/utils/target.go`, `leakless.go`, `readme.md`, `bin_*.go`)
- <https://github.com/go-rod/rod/blob/main/lib/launcher/browser.go>
- <https://github.com/go-rod/rod/blob/main/lib/launcher/revision.go>
- <https://github.com/go-rod/rod/blob/main/lib/launcher/launcher.go>

CI

- <https://github.blog/changelog/2025-08-07-arm64-hosted-runners-for-public-repositories-are-now-generally-available/>
- <https://github.blog/changelog/2026-01-29-arm64-standard-runners-are-now-available-in-private-repositories/>
- <https://docs.github.com/actions/reference/github-hosted-runners-reference>
- <https://github.com/tonistiigi/binfmt>
- <https://github.com/chromedp/chromedp/blob/main/.github/workflows/test.yml>
- <https://github.com/chromedp/docker-headless-shell>
- <https://hub.docker.com/v2/repositories/chromedp/headless-shell/tags/stable>
- <https://github.com/playwright-community/playwright-go/blob/main/.github/workflows/build.yml>
