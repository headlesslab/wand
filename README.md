# wand

A Chrome DevTools Protocol driver for browser automation and web scraping in Go.

wand descends from a snapshot of [go-rod](https://github.com/go-rod/rod) (commit `393ac0d`, 2024-12-07) and is maintained independently by headlesslab. See [NOTICE](NOTICE) for attribution.

[中文说明](README.zh-CN.md)

## Browsers

wand is aligned to one Chrome stable at a time, the Target Chrome: the protocol layer is generated for it and the Managed browser download is pinned to it. A Companion Chromium trunk build is the second Managed browser, for deployments that cannot accept Google Chrome's terms. ✅ marks the platforms where a Managed browser archive exists; on the others wand uses a browser already installed. Every number in the table moves with each [Roll](docs/maintainer-notes.md#the-roll), once per Chrome stable milestone.

<!-- pins:begin -->
<!-- prettier-ignore-start -->
| Managed browser | Linux x64 | Linux arm64 | macOS x64 | macOS arm64 | Windows x86 | Windows x64 |
| :--- | :---: | :---: | :---: | :---: | :---: | :---: |
| Chrome 152.0.7977.82 ([Chrome for Testing](https://googlechromelabs.github.io/chrome-for-testing/)) | ✅ | ❌ | ✅ | ✅ | ✅ | ✅ |
| chrome-headless-shell 152.0.7977.82 | ✅ | ❌ | ✅ | ✅ | ✅ | ✅ |
| Chromium 1668623 ([trunk build](https://commondatastorage.googleapis.com/chromium-browser-snapshots/index.html)) | ✅ | ❌ | ✅ | ✅ | ✅ | ✅ |

Protocol: [devtools-protocol r1666840](https://github.com/ChromeDevTools/devtools-protocol/tree/v0.0.1666840). Support window: Chrome 149 to 152, the Target Chrome and the three stable milestones before it.
<!-- prettier-ignore-end -->

<!-- pins:end -->

## Roadmap

- **Baseline release**: the go-rod snapshot renamed to `github.com/headlesslab/wand`, built and tested against current Chrome, with the protocol layer, browser acquisition and dependency chain brought current. In progress.
- **API modernization**: planned.
- **Stealth**: planned.
