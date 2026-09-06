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
| Chrome 152.0.7977.82（[Chrome for Testing](https://googlechromelabs.github.io/chrome-for-testing/)） | ✅ | ❌ | ✅ | ✅ | ✅ | ✅ |
| chrome-headless-shell 152.0.7977.82 | ✅ | ❌ | ✅ | ✅ | ✅ | ✅ |
| Chromium 1668623（[主干构建](https://commondatastorage.googleapis.com/chromium-browser-snapshots/index.html)） | ✅ | ❌ | ✅ | ✅ | ✅ | ✅ |

协议：[devtools-protocol r1666840](https://github.com/ChromeDevTools/devtools-protocol/tree/v0.0.1666840)。支持窗口：Chrome 149 至 152，即 Target Chrome 及其之前的三个稳定里程碑。
<!-- prettier-ignore-end -->

<!-- pins:end -->

## 路线图

- **Baseline release（基线版本）**：将 go-rod 快照改名为 `github.com/headlesslab/wand`，在当前 Chrome 上构建并通过测试，更新协议层、浏览器获取方式和依赖链。进行中。
- **API modernization（API 现代化）**：计划中。
- **Stealth**：计划中。
