# wand

A Chrome DevTools Protocol driver for browser automation and web scraping in Go. wand descends from a snapshot of go-rod's code and is maintained independently by headlesslab.

## Language

### Lineage

**Upstream**:
The go-rod/rod repository whose code wand was copied from. Its code has been frozen since December 2024; wand does not track it.
_Avoid_: origin, parent, original, fork source

**Snapshot**:
The single upstream commit wand's initial code was copied from, imported without git history.
_Avoid_: fork point, base commit, import commit

### Roadmap

**Baseline release**:
wand's first release: the snapshot renamed to the wand module and made to work on current Chrome, with no API redesign.
_Avoid_: v1, MVP, first version, migration release

**API modernization**:
The effort, after the baseline release, to bring wand's API to Playwright/Puppeteer-level ergonomics. Not part of the baseline release.
_Avoid_: v2, rewrite, new API
