# Maintainer notes

Procedures a maintainer of wand and its Satellite modules runs by hand. Each section names the tool that does the work and what it leaves for a human.

## Repository settings

Every headlesslab repository carries the same settings bundle (spec #33, section 16). One idempotent script applies it through `gh api`, reads every setting before writing, and reports what it changed, so a new repository needs one command and drift is one re-run away.

```sh
go run ./internal/tools/repo-settings [-app <slug>] [-check <context>]... [-dry-run] <owner/repo>...
```

It needs the GitHub CLI logged in as a user with admin access to every repository listed (`gh auth status`; a classic token needs the `repo` scope). The bundle, in the order the script applies it:

| Setting                         | What the script sets                                                                                                                                                                                                                                                                                                                                  | Where it shows in the repository settings |
| ------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------- |
| Secret scanning                 | On                                                                                                                                                                                                                                                                                                                                                    | Advanced Security → Secret Protection     |
| Push protection                 | On (needs secret scanning first)                                                                                                                                                                                                                                                                                                                      | Advanced Security → Secret Protection     |
| Dependabot alerts               | On                                                                                                                                                                                                                                                                                                                                                    | Advanced Security → Dependabot            |
| Dependabot security updates     | On (needs the alerts first)                                                                                                                                                                                                                                                                                                                           | Advanced Security → Dependabot            |
| Private vulnerability reporting | On                                                                                                                                                                                                                                                                                                                                                    | Advanced Security                         |
| CodeQL default setup            | Configured for Go, default query suite: GitHub runs the analysis from a workflow of its own, on every pull request, on a push to the default branch and weekly. A setup configured for another set of languages is drift and is rewritten.                                                                                                            | Advanced Security → Code scanning         |
| Immutable releases              | On: a published release's tag and assets are locked; title, notes and the pre-release and latest markers stay editable                                                                                                                                                                                                                                | General → Releases                        |
| Actions SHA pinning             | Required: every action reference must be a full-length commit SHA; the other Actions permissions are left as they are                                                                                                                                                                                                                                 | Actions → General                         |
| Ruleset `main`                  | Targets the default branch. Rules: no deletion, no force push, every change through a pull request (no approvals required), CodeQL's verdict on the pull request (no new alert of high security severity or worse, none at error level), and the `-check` contexts required green before a merge. With no `-check` the status-check rule is left out. | Rules → Rulesets                          |
| Ruleset `v*`                    | Targets `refs/tags/v*`. Creating, moving and deleting such a tag is restricted to the bypass actors, so a published tag never changes (ADR-0008).                                                                                                                                                                                                     | Rules → Rulesets                          |

Both rulesets list the repository admin role as a bypass actor with mode "always", and the GitHub App from `-app` beside it once it exists. A bypass actor can still push to `main` directly and cut a `v*` tag by hand, which is how the satellites are released; the Gates bind everyone else, Dependabot included. Rulesets inherited from the organisation are ignored.

### The repositories today

Run each line from the wand checkout. The Gates are the check names the reusable workflow in `headlesslab/.github` produces; a satellite that calls it with `cross-platform: true` (today only `fetch`) gets two more. A check is matched by name from any source, so the flag carries no app id.

```sh
go run ./internal/tools/repo-settings \
  -check "Tier 1 linux/amd64 (Go stable)" \
  -check "Tier 1 linux/amd64 (Go 1.21)" \
  -check "Tier 1 linux/arm64 (Go stable)" \
  -check "Tier 1 linux/arm64 (Go 1.21)" \
  -check "Tier 1 windows/amd64 (Go stable)" \
  -check "Tier 1 darwin/arm64 (Go stable)" \
  -check "Tier 1 linux/amd64 (Companion Chromium)" \
  -check "Tier 2 darwin/amd64" \
  -check "Tier 2 windows/arm64" \
  -check "Tier 2 linux/loong64" \
  -check "Generate (zero diff)" \
  -check "Dependency review" \
  headlesslab/wand

go run ./internal/tools/repo-settings \
  -check "go / test (ubuntu-latest, floor)" \
  -check "go / test (ubuntu-latest, stable)" \
  -check "go / lint" \
  -check "go / govulncheck" \
  headlesslab/eventbus headlesslab/lazyjson headlesslab/seqdiff headlesslab/leakcheck

go run ./internal/tools/repo-settings \
  -check "go / test (ubuntu-latest, floor)" \
  -check "go / test (ubuntu-latest, stable)" \
  -check "go / test (windows-latest, stable)" \
  -check "go / test (macos-latest, stable)" \
  -check "go / lint" \
  -check "go / govulncheck" \
  headlesslab/fetch
```

wand's `main` ruleset requires the twelve jobs of `.github/workflows/gate.yml`: the linux/amd64 stable job, added while #71 (ticket #36) was open because that pull request was the only branch reporting the check; the generate job (ticket #42), added to the line above by its pull request and applied by re-running the line once the job had reported; the other six Tier 1 jobs and the three Tier 2 jobs (ticket #54), added the same way; and the dependency review job (ticket #56). `govulncheck` needs no name of its own: it is a step of the linux/amd64 stable job, already required. The remaining Gate (spec #33, section 13: the in-container run of #55) lands with its ticket, and adds its check names to the wand line above and re-runs it. A check named in `-check` that no workflow reports would block every merge, so add a Gate only once a branch reports it, and prefer one that has already run on `main`. The same holds for the code scanning rule the bundle writes: it names CodeQL, and a tool name code scanning does not report would block every merge just as surely.

A second run reports `no changes` for every repository. `-dry-run` prints what a run would change, writes nothing, and exits 1 when anything differs; use it to check for drift after a settings change made by hand.

### What stays human

- The GitHub App and organisation-wide two-factor authentication (#58). Once the App exists and is installed on the repositories, re-run every line above with `-app <slug>` so the App becomes a bypass actor of both rulesets; that is the only way the Roll and the release workflow can push to `main` and create tags. `-app` also takes the numeric App ID from the App's settings page, for a private App the apps endpoint does not show to the token.
- The alerts CodeQL's first analysis finds. The ruleset holds a pull request to what it adds, so the Snapshot's own findings block nothing, but the rc ships with none open (#15): each is fixed, or dismissed in the Security tab with a reason, until the count is zero. Check there once the first analysis of `main` has finished, a few minutes after the setup is switched on.
- **The dependency graph**, at Advanced Security → Dependency graph, or organisation-wide for every repository at once. It has no REST endpoint, so the script cannot set it and cannot report it as drift either. It is off on a repository of an organisation whose `dependency_graph_enabled_for_new_repositories` is false, which is how `headlesslab/wand` was created, and two things in the bundle are inert without it: the Dependency review Gate reds with "Dependency review is not supported on this repository", and Dependabot alerts, which the script switches on and GitHub reports as on, have no graph to raise an alert against. Enable it first, then re-run the failed Gate job.
- Organisation-level settings and rulesets: the script touches repositories only.
- Turning a setting off: the script only switches things on and creates or updates the two rulesets. Anything else is a hand change in the repository settings, which the next run reports as drift and reverts.

## The Roll

The Roll (spec #33, section 15) moves the Target Chrome, the Protocol roll, the Companion Chromium and every managed-browser archive hash together. Until its workflow exists, and for a Security roll, the Roll tool is run by hand from the module root:

```sh
go run ./lib/launcher/pins/generate                 # Chrome for Testing's current Stable
go run ./lib/launcher/pins/generate 153.0.8010.27   # that version instead (a Security roll)
go run ./lib/launcher/pins/generate -render         # rewrite the outputs from the committed pins, no download
go run ./lib/launcher/pins/generate -check          # what the generate Gate runs
```

The tool reads Chrome for Testing's version JSON for the Target Chrome and its branch position, lists the tags of `ChromeDevTools/devtools-protocol` through `git ls-remote` for the Protocol roll (the largest `v0.0.<rev>` not above the branch position), lists the Chromium trunk build bucket for the Companion Chromium (the newest position at or below the branch position whose archive exists under all five prefixes, searching an ever wider window below the position until one is found), then downloads every managed-browser archive, twelve Chrome for Testing ones and five Chromium ones (about 2.5 GB), from Google's bucket only and hashes each as it streams; nothing is kept on disk. It rewrites `lib/launcher/pins/pins.go` and the browser table between the `<!-- pins:begin -->` and `<!-- pins:end -->` markers of `README.md` and `README.zh-CN.md`, and prints the three pins. Running it again for the same version gives no diff. `-render` rewrites the same outputs from the committed pins without downloading anything, for when the table's layout or a README's prose changes between two Rolls.

When Google serves no archive for one of the six Chrome for Testing platforms (linux-arm64 exists from 153.0.8001.0 on), the tool still writes what it verified, lists the missing archives and exits 1, so the gap is visible in the diff rather than hidden; a Roll pull request is not opened from such a run.

`-check` writes nothing: it re-derives the Protocol roll from the committed branch position and fails on a mismatch, and it re-renders every output from the committed values and fails when the bytes differ, which catches a stale roll and any drift in formatting, order or a README table. It cannot tell a hand-edited hash from a downloaded one; the reviewed Roll pull request is what vouches for the hashes (ADR-0005). `go generate` runs it before the protocol generator, so the generate Gate covers this package; like the protocol generator, that step needs the network (one `git ls-remote`).

The release workflow reads the pins through the printer, never by parsing source:

```sh
go run ./internal/tools/print-pins         # Chrome <version>, protocol r<roll>, Chromium <position>
go run ./internal/tools/print-pins -json   # {"chrome":"<version>","protocol":<roll>,"chromium":<position>}
```

### What stays human

- Deciding to roll: the tool computes and downloads, it opens no pull request. The reviewed Roll pull request is the trust anchor for every managed-browser hash (ADR-0005), so its reviewer reads the hash diff as the thing being approved.
- Running the protocol generator for the new Protocol roll (below) and putting its symbol-level summary in the pull request, until the Roll workflow does both.

## The protocol layer

`lib/proto` is generated from `ChromeDevTools/devtools-protocol` at the Protocol roll the pins name, with no browser involved (ADR-0004). After a Roll, or when the generator itself changes, run it from the module root:

```sh
go run ./lib/proto/generate                 # tag v0.0.<ProtocolRoll> from GitHub
go run ./lib/proto/generate -schema <dir>   # a checkout of that tag instead, offline
```

The generator downloads the tag's `json/browser_protocol.json` and `json/js_protocol.json` and merges them (the same content a browser serves at `/json/protocol`), applies upstream's patches, and restores `[]byte` for the fields the JSON lowered to `string`: from the "Encoded as a base64 string when passed over JSON" marker in a field's description, and from the hand-kept list `binaryFields` in `lib/proto/generate/patch.go` for the fields that have no description. It then counts the `binary` occurrences in the tag's PDL files and refuses to write anything when that count differs from the `[]byte` fields it would generate; the message lists both sides, so the fix is to add the missing field to the list, or to drop from it a field the roll removed (a listed field the schema lacks is an error on its own). Every generated file under `lib/proto` is replaced (the `a_` files are hand-written and stay), formatted with the pinned golangci-lint, and the Protocol roll is written as `proto.ProtocolRoll` beside `proto.Version`; the `lib/proto` suite holds it equal to the pins. Deprecated entities and fields carry Go's `Deprecated:` paragraph, so `staticcheck` flags their use; experimental ones are generated like any other; entities the roll removed are gone, with no stub.

The run ends with a summary of the removed, renamed and newly deprecated Go identifiers, printed and written to `tmp/proto-summary.md`; it goes into the Roll pull request and the release notes. Running the generator again on a committed tree prints an empty summary and changes nothing, which is what the generate Gate checks. The `-schema` checkout must be at the pinned tag: the generator reads its `package.json` version and refuses any other roll.

Known limitation, kept on purpose (spec #33, section 4; rod #1196): an optional boolean is a plain `bool` with `omitempty`, so a `false` that differs from Chrome's default is never sent. Changing the field types is API modernization.

On Windows an editor's language server that holds the freshly written files can make the formatting step fail with "a file with a user-mapped section open"; running the generator in a copy of the tree the editor does not watch avoids it.

## The generate Gate and the tools it pins

`go generate` from the module root runs, in this order: the setup tool (`go mod download`, `npm ci` for the Node tools, `.dockerignore`), the Roll tool's `-check`, the protocol generator, the JS helper generator, the assets generator, the devices generator, then the lint tool (cspell, eslint, prettier, `golangci-lint fmt` and `run`, the `Must` prefix rule, and a clean `git status`). The generate job of `.github/workflows/gate.yml` runs the same steps on linux/amd64 and fails on any drift, so the committed generated code is always what the pinned inputs give (spec #33, section 13).

Nothing in that chain resolves a version at run time: the Node tools (cspell, eslint with its html plugin, prettier, uglify-js) are named at exact versions in `internal/tools/package.json` and installed from `internal/tools/package-lock.json` with `npm ci`; golangci-lint is run through `go run` at the version `internal/devutil/tools.go` pins, and its formatters (gofmt, gofumpt, goimports, gci) run at the versions its own module pins, so that one line moves them all. Node must be on `PATH` locally; the Gate installs it with `actions/setup-node`. To move a tool, change the version in `package.json` and run `npm install --prefix internal/tools` for the lockfile, or change the line in `tools.go`, then run `go generate` and commit whatever it reformats.

The repository's `.golangci.yml` is upstream's configuration migrated to the v2 schema, the way `headlesslab/.github` did for the Satellite modules, with the linters newer than upstream's set that would restyle the Snapshot disabled and each reason written beside the name; turning one on is a change of its own, once the upstream pull requests are harvested.

## The security Gates, Dependabot and Scorecard

What a pull request must survive besides the tests, what moves the pins nobody moves by hand, and what wand publishes about itself (spec #33, section 16; ticket #56, on the security posture decision #31).

### On a pull request

| Gate                | Where it runs                                                    | What reds it                                                                                                                                                                    |
| ------------------- | ---------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `govulncheck ./...` | a step of the `Tier 1 linux/amd64 (Go stable)` job               | a vulnerable function of a dependency or of the standard library that wand's own code can reach                                                                                 |
| Dependency review   | the `Dependency review` job, on the pull request event alone     | in what the pull request adds to the dependency graph: an advisory of high severity or worse, or a licence outside MIT, BSD-2-Clause, BSD-3-Clause, Apache-2.0, ISC and MPL-2.0 |
| CodeQL              | GitHub's own workflow, from the default setup the bundle applies | an alert the pull request adds, of high security severity or worse or at error level; the `main` ruleset's code scanning rule is what holds the merge                           |

`go run ./internal/tools/govulncheck` is the same scan a developer runs, at the version `internal/devutil` pins; pass arguments to reach one package instead of `./...`. It reads the Go vulnerability database, so it needs the network, and x/vuln's Go floor is far above wand's, so with `GOTOOLCHAIN=local` only the stable job can build it: hence one scan, on that job. Source mode is what makes it quiet enough to gate on — a vulnerability in a package wand imports but never calls into is reported and passes. An advisory published against unchanged code reds the next run; the Nightly rerun on `main` (#61) is what surfaces one the same morning rather than at the next pull request.

Dependency review reads GitHub's dependency graph, which is a repository setting with no REST endpoint and so is not in the bundle: with the graph off the job reds in five seconds with "Dependency review is not supported on this repository", whatever the pull request contains. See "What stays human" above.

Widening the licence allowlist is a reviewed change to `gate.yml`. `gosec` stays disabled in `.golangci.yml`, as upstream had it: CodeQL is the source-analysis Gate.

### Dependabot

`.github/dependabot.yml`, all weekly: `github-actions` at the root (the SHA pins of every workflow), `docker` on `docker/` (the base image digests), and `npm` on `internal/tools/` as one group, so a week's linter updates arrive as one pull request with one resolved lockfile. Go modules get no version pull request at all — the limit of zero says so in the file — while Dependabot security updates, which the settings bundle turns on, are not subject to that limit and open one as soon as an advisory matches. Everything else in `go.mod` moves through a hand pull request, prompted by a satellite release or by the Nightly `go get -u ./...`.

Every Dependabot pull request runs the full Gate and merges like any other; it gets no secrets, which nothing in the Gate needs.

### Scorecard

`.github/workflows/scorecard.yml` runs weekly and on every push to `main`. It publishes to the OpenSSF API, which is what the badge and the public dataset read, and uploads its SARIF to code scanning, where a failing check reads as an alert beside CodeQL's. It is neither a Gate nor a Nightly: it blocks no merge and opens no issue, and a check that scores badly is read, not fixed by reflex.

The badge markdown, for both READMEs (#62):

```markdown
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/headlesslab/wand/badge)](https://scorecard.dev/viewer/?uri=github.com/headlesslab/wand)
```

Scorecard cannot move into `gate.yml`: the API reads the workflow file at the analysed commit and refuses results from one with a workflow-level `env` or `defaults`, which the Gate has, or from a job that runs anything but the handful of actions it allows. Branch-Protection scores from the `main` ruleset, which the run's own token may read; classic branch protection would have wanted an admin token, and no personal access token exists.

## The test suite

How the suites choose their browser, what they print, and what a red run means (spec #33, section 12; ticket #53).

- **Browser**: every suite launches through `launcher.New()`, so Browser resolution chooses the browser: `-wand=bin=`, then `WAND_BROWSER_BIN`, then a System browser, then the Managed browser, downloaded when missing. There is no test-only knob. The Gate pins the browser with `WAND_BROWSER_BIN`, set from the pre-fetch command's stdout; a developer machine runs on its installed Chrome unless the variable says otherwise. The one test that launches another browser is `TestUserModeBrandedChrome`, which passes the branded Google Chrome `LookPath` finds to `Launcher.Bin()`, because the Chrome 136 refusal it proves fixed exists in no other browser. The root suite resolves once in `TestMain` through `Launcher.ResolveBin()`, fails there with the resolved path when that browser does not start, and prints `browser: <path> (<product>), <n> pooled testers` at the top of every run. Each Tier 1 job runs the root package with no test (`go test -run '^$' -v .`) to get that one line before the suite, and holds it to the pins the printer gives: the product's version equals the Target Chrome, or the path holds `chromium-<position>` for the Companion Chromium, whose trunk build has a position rather than a version (#54).
- **Pooled testers**: the root suite keeps one browser per parallel test (`go test -parallel`, GOMAXPROCS by default), each behind a `MockClient` that can stub one CDP call. A failing `Must` call on a pooled browser, its pages or their elements fails the test and ends it (inside a `g.Panic` block it panics, as the block expects), so a red test never kills the binary and the run keeps its cleanup and its coverage. A test that failed, or that hit `-timeout-each` (one minute), has its browser closed and replaced, so nothing it left in a browser reaches another test; a test still running one `-timeout-each` after its browser was closed ends the run, as the `go test` timeout would, after the other browsers are gone.
- **Zero leftover**: when the run ends, whatever its outcome, every pooled browser is closed and its user data directory under `os.TempDir()/wand/user-data` removed. Tests that launch a browser of their own do the same through the harness helpers (`g.launch` in the root suite, `launch` in `lib/cdp`, `stop` in `lib/launcher`); a browser `wand.New().MustConnect()` launched itself is cleaned up by `Browser.Close`, and a launch that fails removes the temporary directory it made up. `Launcher.Cleanup` is bounded: a browser still running ten seconds after the call is killed, and a directory a helper process still holds is retried for as long. Every Tier 1 job ends, whatever its outcome, with `go run ./internal/tools/zero-leftover`, which waits up to 30 s and fails on any chrome or chromium process still running or anything still under `launcher.DefaultUserDataDirPrefix`, listing what it found. It counts every browser on the machine, so on a developer machine it is meaningful only with no browser of your own open. The one directory a test leaves on purpose is the User mode profile, `launcher.DefaultUserModeDir` under the user's configuration directory: `TestUserModeBrandedChrome` launches on it, since the fix it proves is that very default, and kills its browser without `Cleanup`, which would remove a profile that is the developer's own; every other test that launches User mode names a temporary directory. The examples still leave their directories behind until #52 rewrites them.
- **Running locally**: run one browser suite at a time (`go test .`, `go test ./lib/cdp`, `go test ./lib/launcher`). The ci-test wrapper, `go run ./internal/tools/ci-test <go test arguments>`, sets `GODEBUG=tracebackancestors=100` for the leak checker and is what the Gate runs. Pass `-run '^Test'` as the Gate does: the root suite's default pattern runs the examples too, which reach the internet, and its default `-timeout` of five minutes (`got.DefaultFlags`) kills the binary from outside, past every cleanup, so a run it ends leaves the pooled browsers' directories behind. The root suite writes one CDP log per test under `tmp/cdp-log/<run>/<tester>/`, kept for a failed test (the Tier 1 jobs upload the directory) and removed for a passed one.
- **What stays skipped**: only environment guards. `TestFonts` runs in a container only, `TestBinarySize` outside Windows and containers only, `TestProfileDir` with `-test-profile-dir` only, `TestLaunchXVFB` where `xvfb-run` is installed, `TestUserModeBrandedChrome` (the Confirmed fix for rod #1189, ticket #47) where `LookPath` finds branded Google Chrome, which alone refuses remote debugging on its default profile, and on Linux only with a display or `xvfb-run`; the Gate requires its PASS line on `ubuntu-latest`, `windows-latest` and `macos-latest`, the three runners that ship one. No test is skipped for flakiness, listens on a fixed port, or reaches the public internet: the pooled browsers, and the ones `g.launch` starts, run with `--host-resolver-rules` that resolve no host but loopback, so a test that tries fails on every machine. The Managed browser download is the one network access, taken only when no browser is found, before any browser starts.
