# wand

Go 语言的 Chrome DevTools Protocol 驱动，用于浏览器自动化与网页抓取。

wand 源自 [go-rod](https://github.com/go-rod/rod) 的一份代码快照（提交 `393ac0d`，2024-12-07），由 headlesslab 独立维护。署名信息见 [NOTICE](NOTICE)。

[English](README.md)

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

## 容器镜像

`ghcr.io/headlesslab/wand` 以 `ubuntu:noble` 为基础，内含 Target Chrome，入口点是远程启动服务 `wand-manager`，提供 `linux/amd64` 与 `linux/arm64` 两种架构。镜像中的浏览器就是上表固定的那一个，在构建时下载并校验哈希，因此容器里跑的正是 wand 测试所用的浏览器。它的标签由发布流程推送，目前尚未发布；在此之前请从检出的代码构建：

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

`docker run --rm ghcr.io/headlesslab/wand chrome --version` 会打印镜像内的 Chrome 版本；镜像里也带了 `xvfb-run`，可用于有界面模式。

`go run ./internal/tools/docker` 会构建该镜像，并在其之上构建附带 Go 与 Node 工具链的 `:dev` 镜像，然后按 CI 的方式检查两者；它不会推送任何东西。加上 `-suite` 可在 `:dev` 镜像内运行 wand 的完整测试套件，那里 `utils.InContainer` 成立，启动器会传入 `--no-sandbox`。

在受限网络下，Go 模块代理与 Ubuntu 镜像源都是构建参数：

```sh
docker build -t ghcr.io/headlesslab/wand -f docker/Dockerfile \
  --build-arg goproxy=https://goproxy.cn,direct \
  --build-arg apt_mirror=https://your-mirror.example/ubuntu .
```

## 路线图

- **Baseline release（基线版本）**：将 go-rod 快照改名为 `github.com/headlesslab/wand`，在当前 Chrome 上构建并通过测试，更新协议层、浏览器获取方式和依赖链。进行中。
- **API modernization（API 现代化）**：计划中。
- **Stealth**：计划中。
