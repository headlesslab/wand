# wand

[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/headlesslab/wand/badge)](https://scorecard.dev/viewer/?uri=github.com/headlesslab/wand)

Go 语言的 Chrome DevTools Protocol 驱动，用于浏览器自动化与网页抓取。

wand 源自 [go-rod](https://github.com/go-rod/rod) 的一份代码快照（提交 `393ac0d`，2024-12-07），由 headlesslab 独立维护。署名信息见 [NOTICE](NOTICE)。

把现有的 go-rod 程序迁过来，只需替换导入路径，再加上少数几处行为差异；[迁移指南](docs/migrating-from-go-rod.md)（英文）两者都列全了，并附有一节已知限制。

[English](README.md)

## 安装

```sh
go get github.com/headlesslab/wand
```

wand 不需要任何驱动程序：它通过 DevTools Protocol 直接驱动浏览器，并且会先找你已经装好的 Chrome，找不到才下载，详见[浏览器](#浏览器)。

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

库之外还有两个命令：

```sh
go install github.com/headlesslab/wand/cmd/wand-manager@latest        # 在另一台机器上启动浏览器
go install github.com/headlesslab/wand/cmd/wand-fetch-browser@latest  # 预先下载浏览器
```

[`examples_test.go`](examples_test.go) 里的可运行示例驱动的是本仓库自带的页面，由本地回环端口提供，因此 `go test -run Example` 不需要联网。[`lib/examples`](lib/examples) 下则是一些独立的程序，其中若干会访问真实网站。

## 浏览器

wand 一次只对齐一个 Chrome 稳定版，即 Target Chrome：协议层为它生成，Managed browser 的下载也固定在它上面。Companion Chromium（Chromium 主干构建）是第二种 Managed browser，供无法接受 Google Chrome 条款的部署使用。✅ 表示该平台存在 Managed browser 归档，其余平台使用已安装的浏览器。表中所有数字随每次 [Roll](docs/maintainer-notes.md#the-roll) 一起移动，每个 Chrome 稳定里程碑一次。

<!-- pins:begin -->
<!-- prettier-ignore-start -->
| Managed browser | Linux x64 | Linux arm64 | macOS x64 | macOS arm64 | Windows x86 | Windows x64 |
| :--- | :---: | :---: | :---: | :---: | :---: | :---: |
| Chrome 153.0.8010.12（[Chrome for Testing](https://googlechromelabs.github.io/chrome-for-testing/)） | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| chrome-headless-shell 153.0.8010.12 | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Chromium 1680292（[主干构建](https://commondatastorage.googleapis.com/chromium-browser-snapshots/index.html)） | ✅ | ❌ | ✅ | ✅ | ✅ | ✅ |

协议：[devtools-protocol r1680125](https://github.com/ChromeDevTools/devtools-protocol/tree/v0.0.1680125)。支持窗口：Chrome 150 至 153，即 Target Chrome 及其之前的三个稳定里程碑。
<!-- prettier-ignore-end -->

<!-- pins:end -->

CI 测试的是 Target Chrome，它之前的三个里程碑属于尽力而为。代码里不做任何强制：主版本号不同的浏览器照样启动，只会通过浏览器的 logger 打一行日志，同时写明两个版本号。

### 浏览器解析（Browser resolution）

`Launch` 依次查找，取第一个找到的，全部落空才下载：

1. 代码里的 `Launcher.Bin()`
2. `-wand=bin=<path>` 标志（见 [`lib/defaults`](lib/defaults)）
3. `WAND_BROWSER_BIN`
4. System browser：当前系统常见路径下的 Google Chrome、Chromium 或 Microsoft Edge
5. 缓存中已有的 Managed browser
6. 下载 Managed browser：按 [`lib/launcher/pins`](lib/launcher/pins) 中的哈希校验，从 Google 的存储桶和 npmmirror 中先响应的一方获取

用 `WAND_BROWSER_DOWNLOAD=0` 或 `Launcher.Download(false)` 可以关掉下载；关掉之后，以及在没有归档可下载的平台上，启动会以 `launcher.ErrNoBrowser` 失败，错误信息里列出尝试过的每一步。

| 环境变量                | 取值                                       | 对应的代码选项                        |
| ----------------------- | ------------------------------------------ | ------------------------------------- |
| `WAND_BROWSER_BIN`      | 要启动的浏览器可执行文件路径               | `Launcher.Bin()`；标志是 `-wand=bin=` |
| `WAND_BROWSER_CACHE`    | 浏览器缓存目录                             | `Browser.RootDir`                     |
| `WAND_BROWSER_HOSTS`    | 逗号分隔的 URL 模板                        | `Launcher.Hosts()`                    |
| `WAND_BROWSER_DOWNLOAD` | `0` 表示关闭下载                           | `Launcher.Download()`                 |
| `WAND_BROWSER_SOURCE`   | `chrome`（默认）或 `chromium`              | `Launcher.Source()`                   |
| `WAND_BROWSER_BINARY`   | `chrome`（默认）或 `chrome-headless-shell` | `Launcher.Binary()`                   |

优先级是：代码里设置的选项、标志、环境变量、自动查找。缓存位于用户缓存目录下的 `wand/browser`（Linux 是 `$XDG_CACHE_HOME` 或 `~/.cache`，macOS 是 `~/Library/Caches`，Windows 是 `%LocalAppData%`）。完整说明见[启动器的 README](lib/launcher/README.md)（英文）。

国产浏览器不在查找列表里：lbrowser、奇安信可信浏览器、360 安全浏览器信创版、UOS 浏览器、红莲花，都没有任何文档说明它们接受 `--remote-debugging-port`。如果你手上的那一款支持，用 `WAND_BROWSER_BIN` 或 `Launcher.Bin()` 直接指过去即可。在 UOS 和麒麟上，应用商店装的浏览器位于 `/opt/apps/<app id>/files/` 下；在 Loongnix 上，`dpkg -L lbrowser` 会列出 lbrowser 的可执行文件。

## 平台

| Support tier | wand 的承诺                                               | 平台                                                          |
| ------------ | --------------------------------------------------------- | ------------------------------------------------------------- |
| Tier 1       | 每次推送和 PR 都在 CI 上用真实浏览器带 `-race` 构建并测试 | `linux/amd64`、`linux/arm64`、`windows/amd64`、`darwin/arm64` |
| Tier 2       | 在 CI 上构建并 vet，从不用浏览器测试；运行时尽力而为      | `darwin/amd64`、`windows/arm64`、`linux/loong64`              |
| Tier 3       | 不作任何承诺                                              | 其余一切                                                      |

Go floor 是 **`go 1.21`**，不带 `toolchain` 指令，锚定在当前 openEuler LTS 在其支持的每种架构上原生提供的最新 Go 版本上（[ADR-0003](docs/adr/0003-go-floor-anchored-to-openeuler-lts.md)）。只有新的 LTS 抬高它时它才会动。

于是 Domestic platform（国产平台）的构建路径是：

- **openEuler 24.03 LTS**，在 x86_64、aarch64 和 loongarch64 上，用自带的 Go 即可构建 wand（GA 到 SP2 是 1.21.4，SP3 和 SP4 是 1.24.6），不需要下载工具链。
- **openEuler 22.03 LTS（Go 1.17.3）、麒麟 V10（Go 1.13–1.15）和 UOS 20** 都低于 Go floor。要么在装有 Go 1.21 及以上的机器上交叉编译，要么从 [golang.google.cn](https://golang.google.cn/dl/) 装一份 Go 压缩包，那里提供下面涉及的每一种架构。

  ```sh
  GOOS=linux GOARCH=loong64 go build ./...
  ```

- **`linux/loong64` 只指新世界（ABI 2.0）发行版**：openEuler 22.03 LTS 以及 24.03 LTS SP1 及之后的版本。麒麟 V10 和 UOS 20 的 loong64 版本属于旧世界，不是目标平台，官方 Go 编出的二进制在上面连 `main` 都跑不到就会崩。官方 Go 从 1.21 起提供 `linux/loong64` 的二进制发行版。

`windows/arm64`、`linux/loong64` 以及 Alpine 这类 musl 系统都没有对应的 Managed browser 归档，wand 在这些平台上从不下载；请用 `WAND_BROWSER_BIN` 指向你自己安装的浏览器。

## 容器镜像

`ghcr.io/headlesslab/wand` 以 `ubuntu:noble` 为基础，内含 Target Chrome，入口点是远程启动服务 `wand-manager`，`linux/amd64` 与 `linux/arm64` 合并为同一个 manifest。镜像中的浏览器就是上表固定的那一个，在构建时下载并校验哈希，因此容器里跑的正是 wand 测试所用的浏览器。它的标签由发布流程推送，目前尚未发布；在此之前请从检出的代码构建：

```sh
docker build -t ghcr.io/headlesslab/wand -f docker/Dockerfile .
docker run --rm -p 7317:7317 ghcr.io/headlesslab/wand
```

之后任何地方的 wand 程序都可以把浏览器启动在该容器里：

```go
l := launcher.MustNewManaged("ws://127.0.0.1:7317")
browser := wand.New().Client(l.MustClient()).MustConnect()
```

完整程序见 [`lib/examples/launch-managed`](lib/examples/launch-managed)。

`docker run --rm ghcr.io/headlesslab/wand chrome --version` 会打印镜像内的 Chrome 版本；镜像里也带了 `xvfb-run`，可用于运行有界面的浏览器。

每一个发布出去的 manifest 都带有构建来源（provenance）证明和 SPDX SBOM 证明，记录在 Sigstore 的公开日志里：

```sh
gh attestation verify oci://ghcr.io/headlesslab/wand:v0.1.0 -R headlesslab/wand
```

`go run ./internal/tools/docker` 会构建该镜像，并在其之上构建附带 Go 与 Node 工具链的 `:dev` 镜像，然后按 image Gate 的方式检查两者；它不会推送任何东西。加上 `-suite` 可在 `:dev` 镜像内运行 wand 的完整测试套件，那里 `utils.InContainer` 成立，启动器会传入 `--no-sandbox`。

在受限网络下，Go 模块代理与 Ubuntu 镜像源都是构建参数：

```sh
docker build -t ghcr.io/headlesslab/wand -f docker/Dockerfile \
  --build-arg goproxy=https://goproxy.cn,direct \
  --build-arg apt_mirror=https://your-mirror.example/ubuntu .
```

## 发布

每个 Chrome 稳定里程碑发一个版本，在移动 pins 的 [Roll](docs/maintainer-notes.md#the-roll) 合并之后切出，其间按需发补丁版本。[`versions.json`](versions.json) 记录了每个版本各自携带的 Target Chrome、Protocol roll 和 Companion Chromium，可以用它把浏览器版本和 wand 版本对上。

1.0 之前的规则是：minor 版本可能破坏兼容，patch 版本从不破坏，因此 `go get -u=patch` 永远是安全的（[ADR-0008](docs/adr/0008-fresh-v0-series-one-milestone-release-per-chrome-stable.md)）。

## 路线图

- **Baseline release（基线版本）**：将 go-rod 快照改名为 `github.com/headlesslab/wand`，在当前 Chrome 上构建并通过测试，更新协议层、浏览器获取方式和依赖链。进行中。
- **API modernization（API 现代化）**：计划中。它会在之后的某个 minor 版本里改变 API。
- **Stealth**：计划中。wand 目前不含任何反检测代码。

没有时间表，也不承诺先后顺序。
