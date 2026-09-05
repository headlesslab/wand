# Replace upstream's ysmood dependencies with headlesslab satellite modules

Upstream's whole third-party graph is seven MIT modules by its own author: three frozen since 2022 (`goob`, `gson`, `gotrace`), a test framework (`got`, with `gop`) that ships in every user binary because `page.go` uses `got/lib/lcs` and that caps `got` at v0.42.4 since v0.43.0 deleted `lib/lcs`, and two active ones (`fetchup`, `got`) whose newer versions break the build, the most-upvoted problem in upstream's tracker (#1195, #1203, #1237). wand removes every `ysmood/*` module from its runtime graph: `goob`, `gson`, `got/lib/lcs` and `gotrace` are snapshot-imported the way ADR-0001 imported rod (verbatim at the pinned tag, tests included, a LICENSE carrying Yad Smood's and headlesslab's copyright lines, a NOTICE naming the upstream tag and commit) into four satellite modules named for what they do, `github.com/headlesslab/eventbus`, `lazyjson`, `seqdiff` and `leakcheck`; `fetchup` is replaced by a new generic archive fetcher, `github.com/headlesslab/fetch` (concurrent host probe with the first responder winning, SHA-256 verified before extraction, zip only, a `gofrs/flock` file lock around download and extraction, extraction into a temporary directory renamed into place), with all Chrome knowledge staying in wand's launcher; `got` stays as a test-only dependency at its latest version, `gop` following it. Each satellite is one repository versioned v0.x on its own with no breaking changes without a major bump, and wand pins exact versions. wand's shipped code may otherwise use mainstream third-party libraries where they earn their place (`golang.org/x/sys`, `gofrs/flock` inside `fetch`).

## Considered Options

- **Keep everything pinned**: the frozen modules are dependency-free and stable, but `go get -u` breaks the build, `got` and `gop` ship in every binary, and no version of any of the seven has a maintainer who is continuing rod.
- **Internalize into the wand module** (runZeroInc/go-rod inlined four of them under `pkg/`): clears the graph in one repository, but ties `gson`'s public type to wand's release line and hides the copies from reuse by other headlesslab projects; the driver chose separate modules for that reuse and for independent versioning.
- **Replace `gson` with `json.RawMessage` or `tidwall/gjson`**: changes ten exported signatures and 27 protocol struct fields, an API redesign that belongs to API modernization.
- **Replace `got` with testify**: 1,782 calls across 29 files and about 12k test lines plus a change to the `lib/proto` test generator, for no user-visible gain, since test dependencies do not ship.
- **Upgrade `fetchup` to v0.5.3** (upstream PRs #1201, #1231, #1236): keeps a single-maintainer module that has already broken its API once, and its `Fetch()` extracts before wand could verify a hash.
- **GitHub forks preserving history** for the satellites: rejected for consistency with ADR-0001; the frozen upstreams have no later fixes to sync.

## Consequences

- Users who imported `github.com/ysmood/gson` change that one import path to `github.com/headlesslab/lazyjson`; identifiers (`JSON`, `Int`, `New`, ...) are unchanged. Other upstream identifiers survive the copy too (`seqdiff` keeps `YadLCS`); renames are each satellite's own later change.
- `go get -u ./...` on wand yields a graph that builds; CI runs the suite on both the pinned graph and the updated one.
- `got`'s version ceiling disappears; the `check-cov` CI step is pinned to the same `got` version instead of `@latest`.
- The three upstream dependency-bump PRs are superseded rather than harvested.
- Order of work in the baseline: satellites created and tagged v0.1.0, then the mechanical import swap in wand under the unchanged suite, then the `got`/`gop` upgrade, then the download rewrite on `fetch` together with the browser-acquisition changes (ADR-0005), then the orphan guard (ADR-0007).
- `leakless.LockPort` is no longer available for the download lock; `fetch` owns it.
