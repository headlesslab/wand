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

wand's `main` ruleset requires the minimal Gate of `.github/workflows/gate.yml`, added while #71 (ticket #36) was open because that pull request was the only branch reporting the check; the remaining Gates (spec #33, section 13) land with the CI tickets, and each of those tickets adds its check names to the wand line above and re-runs it. A check named in `-check` that no workflow reports would block every merge, so add a Gate only once a branch reports it, and prefer one that has already run on `main`.

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
go run ./lib/launcher/pins/generate -check          # what the generate Gate runs
```

The tool reads Chrome for Testing's version JSON for the Target Chrome and its branch position, lists the tags of `ChromeDevTools/devtools-protocol` through `git ls-remote` for the Protocol roll (the largest `v0.0.<rev>` not above the branch position), lists the Chromium snapshots bucket for the Companion Chromium (the newest position at or below the branch position whose archive exists under all five prefixes), then downloads every managed-browser archive, twelve Chrome for Testing ones and five Chromium ones (about 2.5 GB), from Google's bucket only and hashes each as it streams; nothing is kept on disk. It rewrites `lib/launcher/pins/pins.go` and prints the three pins. Running it again for the same version gives no diff.

When Google serves no archive for one of the six Chrome for Testing platforms (linux-arm64 exists from 153.0.8001.0 on), the tool still writes what it verified, lists the missing archives and exits 1, so the gap is visible in the diff rather than hidden; a Roll pull request is not opened from such a run.

`-check` writes nothing: it re-derives the Protocol roll from the committed branch position and re-renders the file from the committed values, and fails on either difference. `go generate` runs it before the protocol generator, so the generate Gate catches a hand edit and a stale roll.

The release workflow reads the pins through the printer, never by parsing source:

```sh
go run ./internal/tools/print-pins         # Chrome <version>, protocol r<roll>, Chromium <position>
go run ./internal/tools/print-pins -json   # {"chrome":"<version>","protocol":<roll>,"chromium":<position>}
```

### What stays human

- Deciding to roll: the tool computes and downloads, it opens no pull request. The reviewed Roll pull request is the trust anchor for every managed-browser hash (ADR-0005), so its reviewer reads the hash diff as the thing being approved.
- Regenerating the protocol layer for the new Protocol roll and reading its symbol-level summary, until the Roll workflow does both.
