# Maintainer notes

Procedures a maintainer of wand and its Satellite modules runs by hand. Each section names the tool that does the work and what it leaves for a human.

## Repository settings

Every headlesslab repository carries the same settings bundle (spec #33, section 16). One idempotent script applies it through `gh api`, reads every setting before writing, and reports what it changed, so a new repository needs one command and drift is one re-run away.

```sh
go run ./internal/tools/repo-settings [-app <slug>] [-check <context>]... [-dry-run] <owner/repo>...
```

It needs the GitHub CLI logged in as a user with admin access to every repository listed (`gh auth status`; a classic token needs the `repo` scope). The bundle, in the order the script applies it:

| Setting                         | What the script sets                                                                                                                                                                                                                     | Where it shows in the repository settings |
| ------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------- |
| Secret scanning                 | On                                                                                                                                                                                                                                       | Advanced Security → Secret Protection     |
| Push protection                 | On (needs secret scanning first)                                                                                                                                                                                                         | Advanced Security → Secret Protection     |
| Dependabot alerts               | On                                                                                                                                                                                                                                       | Advanced Security → Dependabot            |
| Dependabot security updates     | On (needs the alerts first)                                                                                                                                                                                                              | Advanced Security → Dependabot            |
| Private vulnerability reporting | On                                                                                                                                                                                                                                       | Advanced Security                         |
| Immutable releases              | On: a published release's tag and assets are locked; title, notes and the pre-release and latest markers stay editable                                                                                                                   | General → Releases                        |
| Actions SHA pinning             | Required: every action reference must be a full-length commit SHA; the other Actions permissions are left as they are                                                                                                                    | Actions → General                         |
| Ruleset `main`                  | Targets the default branch. Rules: no deletion, no force push, every change through a pull request (no approvals required), and the `-check` contexts required green before a merge. With no `-check` the status-check rule is left out. | Rules → Rulesets                          |
| Ruleset `v*`                    | Targets `refs/tags/v*`. Creating, moving and deleting such a tag is restricted to the bypass actors, so a published tag never changes (ADR-0008).                                                                                        | Rules → Rulesets                          |

Both rulesets list the repository admin role as a bypass actor with mode "always", and the GitHub App from `-app` beside it once it exists. A bypass actor can still push to `main` directly and cut a `v*` tag by hand, which is how the satellites are released; the Gates bind everyone else, Dependabot included. Rulesets inherited from the organisation are ignored.

### The repositories today

Run each line from the wand checkout. The Gates are the check names the reusable workflow in `headlesslab/.github` produces; a satellite that calls it with `cross-platform: true` (today only `fetch`) gets two more. A check is matched by name from any source, so the flag carries no app id.

```sh
go run ./internal/tools/repo-settings \
  -check "Tier 1 linux/amd64 (Go stable)" \
  -check "Generate (zero diff)" \
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

wand's `main` ruleset requires the two jobs of `.github/workflows/gate.yml`: the test job, added while #71 (ticket #36) was open because that pull request was the only branch reporting the check, and the generate job (ticket #42), added to the line above by its pull request and applied by re-running the line once the job had reported; the remaining Gates (spec #33, section 13) land with the CI tickets, and each of those tickets adds its check names to the wand line above and re-runs it. A check named in `-check` that no workflow reports would block every merge, so add a Gate only once a branch reports it, and prefer one that has already run on `main`.

A second run reports `no changes` for every repository. `-dry-run` prints what a run would change, writes nothing, and exits 1 when anything differs; use it to check for drift after a settings change made by hand.

### What stays human

- The GitHub App and organisation-wide two-factor authentication (#58). Once the App exists and is installed on the repositories, re-run every line above with `-app <slug>` so the App becomes a bypass actor of both rulesets; that is the only way the Roll and the release workflow can push to `main` and create tags. `-app` also takes the numeric App ID from the App's settings page, for a private App the apps endpoint does not show to the token.
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

The generator downloads the tag's `json/browser_protocol.json` and `json/js_protocol.json` and merges them (the same content a browser serves at `/json/protocol`), applies upstream's patches, and restores `[]byte` for the fields the JSON lowered to `string`: from the "Encoded as a base64 string when passed over JSON" marker in a field's description, and from the hand-kept list `binaryFields` in `lib/proto/generate/patch.go` for the fields that have no description. It then counts the `binary` occurrences in the tag's PDL files and refuses to write anything when that count differs from the `[]byte` fields it would generate; the message lists both sides, so the fix is to add the missing field to the list, or to drop from it a field the roll removed (a listed field the schema lacks is an error on its own). Every generated file under `lib/proto` is replaced (the `a_` files are hand-written and stay), formatted with the pinned golangci-lint, and the Protocol roll is written as `proto.Roll` beside `proto.Version`; the `lib/proto` suite holds it equal to the pins. Deprecated entities and fields carry Go's `Deprecated:` paragraph, so `staticcheck` flags their use; experimental ones are generated like any other; entities the roll removed are gone, with no stub.

The run ends with a summary of the removed, renamed and newly deprecated Go identifiers, printed and written to `tmp/proto-summary.md`; it goes into the Roll pull request and the release notes. Running the generator again on a committed tree prints an empty summary and changes nothing, which is what the generate Gate checks. The `-schema` checkout must be at the pinned tag: the generator reads its `package.json` version and refuses any other roll.

Known limitation, kept on purpose (spec #33, section 4; rod #1196): an optional boolean is a plain `bool` with `omitempty`, so a `false` that differs from Chrome's default is never sent. Changing the field types is API modernization.

On Windows an editor's language server that holds the freshly written files can make the formatting step fail with "a file with a user-mapped section open"; the generator retries three times, and running it in a copy of the tree the editor does not watch always works.

## The generate Gate and the tools it pins

`go generate` from the module root runs, in this order: the setup tool (`go mod download`, `npm ci` for the Node tools, `.dockerignore`), the Roll tool's `-check`, the protocol generator, the JS helper generator, the assets generator, the devices generator, then the lint tool (cspell, eslint, prettier, `golangci-lint fmt` and `run`, the `Must` prefix rule, and a clean `git status`). The generate job of `.github/workflows/gate.yml` runs the same steps on linux/amd64 and fails on any drift, so the committed generated code is always what the pinned inputs give (spec #33, section 13).

Nothing in that chain resolves a version at run time: the Node tools (cspell, eslint with its html plugin, prettier, uglify-js) are named at exact versions in `internal/tools/package.json` and installed from `internal/tools/package-lock.json` with `npm ci`; golangci-lint is run through `go run` at the version `internal/devutil/tools.go` pins, and its formatters (gofmt, gofumpt, goimports, gci) run at the versions its own module pins, so that one line moves them all. Node must be on `PATH` locally; the Gate installs it with `actions/setup-node`. To move a tool, change the version in `package.json` and run `npm install --prefix internal/tools` for the lockfile, or change the line in `tools.go`, then run `go generate` and commit whatever it reformats.

The repository's `.golangci.yml` is upstream's configuration migrated to the v2 schema, the way `headlesslab/.github` did for the Satellite modules, with the linters newer than upstream's set that would restyle the Snapshot disabled and each reason written beside the name; turning one on is a change of its own, once the upstream pull requests are harvested.
