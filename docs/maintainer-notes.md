# Maintainer notes

Procedures a maintainer of wand and its Satellite modules runs by hand. Each section names the tool that does the work and what it leaves for a human.

## Repository settings

Every headlesslab repository carries the same settings bundle (spec #33, section 16). One idempotent script applies it through `gh api`, reads every setting before writing, and reports what it changed, so a new repository needs one command and drift is one re-run away.

```sh
go run ./internal/tools/repo-settings [-app <slug>] [-check <context>]... [-dry-run] <owner/repo>...
```

It needs the GitHub CLI logged in as a user with admin access to every repository listed (`gh auth status`; a classic token needs the `repo` scope). The bundle, in the order the script applies it:

| Setting | What the script sets | Where it shows in the repository settings |
| --- | --- | --- |
| Secret scanning | On | Advanced Security → Secret Protection |
| Push protection | On (needs secret scanning first) | Advanced Security → Secret Protection |
| Dependabot alerts | On | Advanced Security → Dependabot |
| Dependabot security updates | On (needs the alerts first) | Advanced Security → Dependabot |
| Private vulnerability reporting | On | Advanced Security |
| Immutable releases | On: a published release's tag and assets are locked; title, notes and the pre-release and latest markers stay editable | General → Releases |
| Actions SHA pinning | Required: every action reference must be a full-length commit SHA; the other Actions permissions are left as they are | Actions → General |
| Ruleset `main` | Targets the default branch. Rules: no deletion, no force push, every change through a pull request (no approvals required), and the `-check` contexts required green before a merge. With no `-check` the status-check rule is left out. | Rules → Rulesets |
| Ruleset `v*` | Targets `refs/tags/v*`. Creating, moving and deleting such a tag is restricted to the bypass actors, so a published tag never changes (ADR-0008). | Rules → Rulesets |

Both rulesets list the repository admin role as a bypass actor with mode "always", and the GitHub App from `-app` beside it once it exists. A bypass actor can still push to `main` directly and cut a `v*` tag by hand, which is how the satellites are released; the Gates bind everyone else, Dependabot included. Rulesets inherited from the organisation are ignored.

### The repositories today

Run each line from the wand checkout. The required checks are the check names the reusable workflow in `headlesslab/.github` produces; a satellite that calls it with `cross-platform: true` (today only `fetch`) gets two more.

```sh
go run ./internal/tools/repo-settings headlesslab/wand

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

wand's `main` ruleset requires no checks yet: its Gates (spec #33, section 13) land with the CI tickets, and each of those tickets adds its check names to the wand line above and re-runs it. A check named in `-check` that no workflow reports would block every merge, so add a check only after its workflow has run once on `main`.

A second run reports `no changes` for every repository. `-dry-run` prints what a run would change, writes nothing, and exits 1 when anything differs; use it to check for drift after a settings change made by hand.

### What stays human

- The GitHub App and organisation-wide two-factor authentication (#58). Once the App exists and is installed on the repositories, re-run every line above with `-app <slug>` so the App becomes a bypass actor of both rulesets; that is the only way the Roll and the release workflow can push to `main` and create tags.
- Organisation-level settings and rulesets: the script touches repositories only.
- Turning a setting off: the script only switches things on and creates or updates the two rulesets. Anything else is a hand change in the repository settings, which the next run reports as drift and reverts.
