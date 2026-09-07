# Maintainer notes

Procedures a maintainer of wand and its Satellite modules runs by hand. Each section names the tool that does the work and what it leaves for a human.

## Repository settings

Every headlesslab repository carries the same settings bundle (spec #33, section 16). One idempotent script applies it through `gh api`, reads every setting before writing, and reports what it changed, so a new repository needs one command and drift is one re-run away.

```sh
go run ./internal/tools/repo-settings [-app <slug>] [-check <context>]... [-dry-run] <owner/repo>...
```

It needs the GitHub CLI logged in as a user with admin access to every repository listed (`gh auth status`; a classic token needs the `repo` scope). The bundle, in the order the script applies it:

| Setting                         | What the script sets                                                                                                                                                                                                                                                                                                                                                                 | Where it shows in the repository settings |
| ------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ----------------------------------------- |
| Secret scanning                 | On                                                                                                                                                                                                                                                                                                                                                                                   | Advanced Security → Secret Protection     |
| Push protection                 | On (needs secret scanning first)                                                                                                                                                                                                                                                                                                                                                     | Advanced Security → Secret Protection     |
| Dependabot alerts               | On                                                                                                                                                                                                                                                                                                                                                                                   | Advanced Security → Dependabot            |
| Dependabot security updates     | On (needs the alerts first)                                                                                                                                                                                                                                                                                                                                                          | Advanced Security → Dependabot            |
| Private vulnerability reporting | On                                                                                                                                                                                                                                                                                                                                                                                   | Advanced Security                         |
| CodeQL default setup            | Configured for Go, default query suite: GitHub runs the analysis from a workflow of its own, on every pull request, on a push to the default branch and weekly. A setup that does not scan Go is drift and is rewritten; a language the maintainer added beside Go is left alone. One just configured names no language until its first analysis has run, and reads as `configured`. | Advanced Security → Code scanning         |
| Immutable releases              | On: a published release's tag and assets are locked; title, notes and the pre-release and latest markers stay editable                                                                                                                                                                                                                                                               | General → Releases                        |
| Actions SHA pinning             | Required: every action reference must be a full-length commit SHA; the other Actions permissions are left as they are                                                                                                                                                                                                                                                                | Actions → General                         |
| Ruleset `main`                  | Targets the default branch. Rules: no deletion, no force push, every change through a pull request (no approvals required), CodeQL's verdict on the pull request (no new alert of high security severity or worse, none at error level), and the `-check` contexts required green before a merge. With no `-check` the status-check rule is left out.                                | Rules → Rulesets                          |
| Ruleset `v*`                    | Targets `refs/tags/v*`. Creating, moving and deleting such a tag is restricted to the bypass actors, so a published tag never changes (ADR-0008).                                                                                                                                                                                                                                    | Rules → Rulesets                          |

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
  -check "Image linux/amd64" \
  -check "Image linux/arm64" \
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

wand's `main` ruleset requires the fourteen jobs of `.github/workflows/gate.yml`: the linux/amd64 stable job, added while #71 (ticket #36) was open because that pull request was the only branch reporting the check; the generate job (ticket #42), added to the line above by its pull request and applied by re-running the line once the job had reported; the other six Tier 1 jobs and the three Tier 2 jobs (ticket #54), added the same way; the dependency review job (ticket #56); and the two image jobs (ticket #55). `govulncheck` needs no name of its own: it is a step of the linux/amd64 stable job, already required, as Trivy is of the image jobs. Every Gate spec #33, section 13 asks for is now named. A check named in `-check` that no workflow reports would block every merge, so add a Gate only once a branch reports it, and prefer one that has already run on `main`. The same holds for the code scanning rule the bundle writes: it names CodeQL, and a tool name code scanning does not report would block every merge just as surely. A new check also blocks every pull request that is already open, because GitHub does not re-run one when its base moves: a branch older than the workflow that reports the check never will report it, and the check sits expected for good. So a run that adds a check ends by updating every open pull request's branch (`gh pr update-branch <number>`, the Update branch button), Dependabot's included, which gives each one a run from a merge with the new workflow. Observed 2026-09-07: the two image checks of #55 went into the ruleset as soon as they had reported on `main`, which left #96 and #98 blocked on checks that could not appear until their branches were updated.

A second run reports `no changes` for every repository. `-dry-run` prints what a run would change, writes nothing, and exits 1 when anything differs; use it to check for drift after a settings change made by hand.

### What stays human

- The GitHub App and organisation-wide two-factor authentication (#58), which [The automation identity](#the-automation-identity) below walks through end to end. Its last step re-runs every line above with `-app <slug>`, so the App joins the admin role in the bypass list of both rulesets; that is the only way the Roll and the release workflow can push to `main` and create tags. `-app` also takes the numeric App ID from the App's settings page, for a private App the apps endpoint does not show to the token.
- The alerts CodeQL's first analysis finds. The ruleset holds a pull request to what it adds, so the Snapshot's own findings block nothing, but the rc ships with none open (#15): each is fixed, or dismissed in the Security tab with a reason, until the count is zero. Check there once the first analysis of `main` has finished, a few minutes after the setup is switched on.
- **The dependency graph**, at Advanced Security → Dependency graph, or organisation-wide for every repository at once. It has no REST endpoint, so the script cannot set it and cannot report it as drift either. It is off on a repository of an organisation whose `dependency_graph_enabled_for_new_repositories` is false, which is how `headlesslab/wand` was created, and two things in the bundle are inert without it: the Dependency review Gate reds with "Dependency review is not supported on this repository", and Dependabot alerts, which the script switches on and GitHub reports as on, have no graph to raise an alert against. Enable it first, then re-run the failed Gate job.
- Organisation-level settings and rulesets: the script touches repositories only.
- Turning a setting off: the script only switches things on and creates or updates the two rulesets. Anything else is a hand change in the repository settings, which the next run reports as drift and reverts.

## The automation identity

The Roll, the release workflow and the Nightly issue opener act as a GitHub App the organisation owns, never as a personal access token (spec #33, section 15, whose "Automation identity" this is; #58). A token minted per run from the App's private key lives an hour, belongs to no person, expires on nobody's leaving, and is the bypass actor that lets those workflows push to `main` and create a `v*` tag while the rulesets bind everyone else.

Creating an App, holding its private key, installing it and changing an organisation setting are things only an organisation owner can do: GitHub exposes no REST endpoint for any of them, so no agent and no token can stand in. What follows is therefore written for a human, in the order the dependencies force. The App must exist and be installed before its secrets are worth anything, the secrets must exist before a run can mint a token, and the settings script cannot name a bypass actor that does not exist yet.

The identity, as it stands:

| Property                      | Value                                                                           |
| ----------------------------- | ------------------------------------------------------------------------------- |
| Name                          | `headlesslab-automation`                                                        |
| App ID                        | `4859989`                                                                       |
| Owner                         | the `headlesslab` organisation, installable on this account only                |
| Repository permissions        | contents: read and write, issues: read and write, pull requests: read and write |
| Installed on                  | every repository of the organisation                                            |
| Secrets on `headlesslab/wand` | `AUTOMATION_APP_CLIENT_ID`, `AUTOMATION_APP_PRIVATE_KEY`                        |

The two secret names are what #59, #60 and #61 write into their workflows, so they are settled here rather than in whichever of those lands first. The Client ID rather than the App ID, because `actions/create-github-app-token` deprecated its `app-id` input in v3 and warns on every use; a secret rather than the variable its README suggests, because #58 asks for both halves of the identity to be repository secrets and a value only workflows read is no worse for being one. The App ID is not stored anywhere: it is read off the App's page, and it is what appears in a ruleset's bypass list.

### 1. Create the App

Open <https://github.com/organizations/headlesslab/settings/apps/new> and fill in:

- **GitHub App name**: `headlesslab-automation`. The name is unique across GitHub; if it is taken, pick another and carry that one through the rest of these steps.
- **Homepage URL**: `https://github.com/headlesslab/wand`.
- **Webhook**: clear the **Active** checkbox. The App is a token source, not a listener.
- **Repository permissions**: Contents **Read and write**, Issues **Read and write**, Pull requests **Read and write**. Metadata read-only comes with them and cannot be cleared. Nothing else: an organisation permission or an account permission the App never uses is a permission a leaked key would carry.
- **Where can this GitHub App be installed**: **Only on this account**.

Create it, then read three values off the App's General page: the **Client ID** (`Iv23li…`) for step 4's secret, the **App ID** (a number) for the bypass list step 8 reads back, and the **slug**, the last segment of the page's URL (`.../settings/apps/<slug>`), for step 8's `-app`.

The same App can be created from a manifest instead, which is what #58 did: a local page posts a JSON description to `https://github.com/organizations/headlesslab/settings/apps/new?state=<nonce>`, GitHub shows its own confirmation page, and the redirect afterwards carries a one-shot code that `POST /app-manifests/<code>/conversions` turns into the App, private key included, once. It saves the form-filling and lets the key go straight from the response into `gh secret set`, never touching a disk. Two things to know before writing one: the manifest's `hook_attributes` block makes its own `url` mandatory as soon as it is present, so an App with no webhook omits the block rather than setting `active: false`; and the conversion response is the only copy of the key there will ever be.

### 2. Generate the private key

On the same page, **Private keys** → **Generate a private key**. The browser downloads a `.pem`. Save it outside any checkout: the working tree ignores `*.pem` as a backstop, but a key that never enters a repository cannot be committed by accident. (From a manifest the key arrives in the conversion response instead, and this step does not happen.)

GitHub keeps only the public half, so the file is the only copy. A lost or leaked key is replaced by generating a second one, storing it (step 4) and deleting the old one on this page; a rotation is those three actions and no others.

### 3. Install it on the organisation

App page → **Install App** → **headlesslab** → **All repositories** → **Install**.

All repositories rather than wand alone: the five satellites carry the same two rulesets and the same bypass list (section [Repository settings](#repository-settings) above), and a repository created later is covered without another visit here. An installation grants nothing on its own; the permissions of step 1 are the ceiling. Read it back with `gh api orgs/headlesslab/installations --jq '.installations[] | "\(.app_slug) \(.repository_selection)"'`, which must say `all`.

### 4. Store the two secrets

From a shell logged in as an admin of wand:

```sh
gh secret set AUTOMATION_APP_CLIENT_ID --repo headlesslab/wand --body '<the Client ID>'
gh secret set AUTOMATION_APP_PRIVATE_KEY --repo headlesslab/wand < ~/headlesslab-automation.private-key.pem
```

Only wand carries them. The satellites are released by hand and mint no token; what they need from the App is an installation and a place in the bypass list, neither of which is a secret.

### 5. Prove a minted token

The proof is a workflow that lives on one branch and never reaches `main`. Neither `gate.yml` nor `scorecard.yml` runs on a push to a branch other than `main` — the Gate's other trigger is `pull_request` and Scorecard's is a weekly schedule — so pushing this branch with no pull request open runs this workflow and nothing else.

```sh
git switch -c chore/app-token-proof
cat > .github/workflows/app-token-proof.yml <<'YAML'
name: App token proof

on:
  push:
    branches:
      - chore/app-token-proof

# The job's own GITHUB_TOKEN can do nothing at all, so an issue that opens
# and closes proves the App's token and only the App's token.
permissions: {}

jobs:
  proof:
    runs-on: ubuntu-latest
    timeout-minutes: 5
    steps:
      - uses: actions/create-github-app-token@bcd2ba49218906704ab6c1aa796996da409d3eb1 # v3.2.0
        id: token
        with:
          client-id: ${{ secrets.AUTOMATION_APP_CLIENT_ID }}
          private-key: ${{ secrets.AUTOMATION_APP_PRIVATE_KEY }}

      - name: Open and close an issue as the App
        env:
          GH_TOKEN: ${{ steps.token.outputs.token }}
          REPO: ${{ github.repository }}
        run: |
          url=$(gh issue create --repo "$REPO" \
            --title 'App token proof' \
            --body 'Opened by the automation identity to prove its token (#58). Closed by the same run and deleted afterwards.')
          echo "opened $url"
          n=${url##*/}
          gh issue close "$n" --repo "$REPO" --comment 'Closed by the same run.'
          gh api "repos/$REPO/issues/$n" \
            --jq '"state=\(.state) user=\(.user.login) type=\(.user.type)"'
YAML
git add .github/workflows/app-token-proof.yml
git commit -m 'App token proof (#58)'
git push -u origin chore/app-token-proof
```

The action is pinned to a full-length commit SHA because the Actions policy the settings bundle writes rejects any reference that is not one; a tag here fails the run before a step of it starts. Dependabot will not move this pin the way it moves the ones in `.github/workflows`: its `github-actions` entry covers workflow files, and this is prose. A future reader should check the action's own releases rather than trust the version comment.

A run takes a few seconds to register, so ask for it in a loop rather than reading the list once:

```sh
until id=$(gh run list --branch chore/app-token-proof --limit 1 --json databaseId --jq '.[0].databaseId') && [ -n "$id" ]; do sleep 3; done
gh run watch "$id" --exit-status
```

The last line of the log is the proof: `state=closed user=headlesslab-automation[bot] type=Bot`. The REST spelling of a bot login is `<slug>[bot]`; `gh issue view --json author` spells the same account `app/<slug>`, so a check written against `gh`'s spelling and a check written against REST's disagree on a run that worked. A login naming anything but the App means the token came from somewhere else. `Bad credentials` means the private key and the Client ID belong to different Apps, or the `.pem` reached the secret without its trailing newline; `Resource not accessible by integration` means the App is not installed on wand, or step 1's issues permission is missing.

### 6. Remove the proof

The workflow and the issue both go, so the App is the only thing this exercise leaves behind:

```sh
git switch main
git branch -D chore/app-token-proof
git push origin --delete chore/app-token-proof
gh issue delete <n> --yes
```

### 7. Require two-factor authentication

There is no API for this: `two_factor_requirement_enabled` is a field of `GET /orgs/{org}` and not a parameter of the `PATCH`. Check first from the shell, though, because the switch removes every member and outside collaborator who has not enabled 2FA, and each of them has to be invited back by hand:

```sh
gh api 'orgs/headlesslab/members?filter=2fa_disabled' --jq '.[].login'
gh api 'orgs/headlesslab/outside_collaborators?filter=2fa_disabled' --jq '.[].login'
```

Both must print nothing. Then open <https://github.com/organizations/headlesslab/settings/security>, tick **Require two-factor authentication for everyone in the headlesslab organisation**, and confirm. The App is unaffected: it is not a member, has no password, and its credential is the private key of step 2, which is why the automation is an App and not somebody's account.

```sh
gh api orgs/headlesslab --jq .two_factor_requirement_enabled   # true
```

### 8. Re-run the settings script with the App

Each of the three command lines of [The repositories today](#the-repositories-today) is run again with `-app headlesslab-automation` inserted after the package path and every `-check` left exactly as it stands there. Do not shorten the check list: the bundle writes the required checks from what `-check` gives it, so a line run with fewer of them silently drops the rest as required checks of `main`.

Add `-dry-run` first. On a bundle that is otherwise already applied the two rulesets are the only changes it prints, and it exits 1 saying so:

```
ruleset main                     differs -> up to date (dry run)
ruleset v*                       differs -> up to date (dry run)
```

Drop `-dry-run` to write. If the slug does not resolve, pass the numeric App ID of step 1 instead: `GET /apps/<slug>` shows a private App only to a token allowed to see it.

The bypass list is what to read back, and it is the last acceptance criterion of #58:

```sh
gh api repos/headlesslab/wand/rulesets --jq '.[] | "\(.id) \(.name)"'
gh api repos/headlesslab/wand/rulesets/<id> --jq .bypass_actors
```

Both rulesets must list `{"actor_id": 4859989, "actor_type": "Integration", "bypass_mode": "always"}` beside the admin role, and every satellite repository the same. Until they do, the Roll's pull request and the release workflow's tag are refused by the very rules they exist to be trusted through.

Observed 2026-09-07, the run these steps were written from: the App was created from a manifest as App ID 4859989, installed across the organisation with `repository_selection: all`, and its proof opened and closed issue #101 of wand, which was then deleted with the branch. The settings script wrote the bypass actor into both rulesets of wand and of all five satellites; on all five satellites it also turned on the CodeQL default setup, which had not reached them since #56 added it to the bundle.

## The Roll

The Roll (spec #33, section 15) moves the Target Chrome, the Protocol roll, the Companion Chromium and every managed-browser archive hash together. `.github/workflows/roll.yml` runs it on a schedule and opens the pull request; the tool underneath is the same one, and is run by hand for a Roll taken outside the schedule or to inspect one before it opens:

```sh
go run ./lib/launcher/pins/generate                 # Chrome for Testing's current Stable
go run ./lib/launcher/pins/generate 153.0.8010.27   # that version instead (a Security roll)
go run ./lib/launcher/pins/generate -if-newer       # that Stable, but only past the pinned milestone
go run ./lib/launcher/pins/generate -render         # rewrite the outputs from the committed pins, no download
go run ./lib/launcher/pins/generate -check          # what the generate Gate runs
```

The tool reads Chrome for Testing's version JSON for the Target Chrome and its branch position, lists the tags of `ChromeDevTools/devtools-protocol` through `git ls-remote` for the Protocol roll (the largest `v0.0.<rev>` not above the branch position), lists the Chromium trunk build bucket for the Companion Chromium (the newest position at or below the branch position whose archive exists under all five prefixes, searching an ever wider window below the position until one is found), then downloads every managed-browser archive, twelve Chrome for Testing ones and five Chromium ones (about 2.5 GB), from Google's bucket only and hashes each as it streams; nothing is kept on disk. It rewrites `lib/launcher/pins/pins.go` and the browser table between the `<!-- pins:begin -->` and `<!-- pins:end -->` markers of `README.md` and `README.zh-CN.md`, and prints the three pins. Running it again for the same version gives no diff. `-render` rewrites the same outputs from the committed pins without downloading anything, for when the table's layout or a README's prose changes between two Rolls.

`-if-newer` is the schedule's form, and the only one that decides for itself whether to roll at all: it reads the last-known-good Stable, compares its milestone with the committed Target Chrome's, and writes nothing at all unless Stable has reached a higher milestone. So it costs one HTTP request on every day between two milestones, and downloads the 2.5 GB only on the day there is a Roll. An equal milestone is not newer: one Milestone release per Chrome stable milestone (ADR-0008), so a move inside a milestone is a forced version, never the schedule's doing.

When Google serves no archive for one of the six Chrome for Testing platforms (linux-arm64 exists from 153.0.8001.0 on), the tool still writes what it verified, lists the missing archives and exits 1, so the gap is visible in the diff rather than hidden; a Roll pull request is not opened from such a run.

`-check` writes nothing: it re-derives the Protocol roll from the committed branch position and fails on a mismatch, and it re-renders every output from the committed values and fails when the bytes differ, which catches a stale roll and any drift in formatting, order or a README table. It cannot tell a hand-edited hash from a downloaded one; the reviewed Roll pull request is what vouches for the hashes (ADR-0005). `go generate` runs it before the protocol generator, so the generate Gate covers this package; like the protocol generator, that step needs the network (one `git ls-remote`).

The release workflow reads the pins through the printer, never by parsing source:

```sh
go run ./internal/tools/print-pins         # Chrome <version>, protocol r<roll>, Chromium <position>
go run ./internal/tools/print-pins -json   # {"chrome":"<version>","protocol":<roll>,"chromium":<position>}
```

### The Roll workflow

`.github/workflows/roll.yml` runs daily at 05:17 UTC and on `workflow_dispatch`, with two inputs: a `version` that forces that version, and a `dry-run` that computes and prints everything and opens nothing.

```sh
gh workflow run roll.yml                                   # what the schedule does
gh workflow run roll.yml -f dry-run=true                   # compute and print, open nothing
gh workflow run roll.yml -f version=153.0.8010.27          # a milestone taken early, or a Security roll
```

It mints a token from the automation identity first, so a rejected token fails before 2.5 GB is downloaded, and it checks out with that token, because GitHub starts no workflow from a `GITHUB_TOKEN` push: the Gate has to run on a Roll, and the App's push is what makes it run. Then the Roll tool with `-if-newer` or the forced version, the protocol generator for the new Protocol roll, `-check` as the generate Gate will run it, and a pull request titled `roll to Chrome <version> (protocol r<rev>)`, labelled `roll`, whose body carries the three pins and the generator's symbol-level summary.

The `roll` label is a repository label of its own, created by hand when this workflow landed; `gh pr create` fails outright on a label that does not exist, so a rebuilt repository needs `gh label create roll` before the first Roll. It has a section of its own in the release notes, through `.github/release.yml`.

Two states end the run green without a pull request: the pins unchanged, which is every day between two milestones, and a Roll whose pull request is already open, which is every day between the Roll and its merge. Without the second check the schedule would push the same branch and fail every day until the first Roll merged.

A failure — the token rejected, an archive Google does not serve, a protocol that does not re-derive — opens **The Roll failed** and comments on that same issue on each later failure, rather than opening one a day. `GITHUB_TOKEN` opens it, since the App token is one of the things that may have failed. Close it once the Roll is green again.

### The Security roll

A Chrome fix for a vulnerability exploited in the wild ships as a patch release (spec #33, section 15; user story 50), and its Roll moves the patch number inside the milestone, which the schedule will never do on its own:

1. Take the version from the Chrome releases blog, and check that its milestone is the one already pinned. A milestone move is an ordinary Roll and a minor release, not this procedure.
2. `gh workflow run roll.yml -f version=<version>`. The pull request carries only the patch number, the branch position, the Protocol roll if the roll moved, the Companion Chromium and the hashes; the protocol summary is usually empty within a milestone.
3. Review the hash diff, which is the thing being approved (ADR-0005), and merge once the Gate is green.
4. Cut a patch release from `main` with the release workflow. A patch never breaks (ADR-0008), and a Security roll changes no exported symbol, so `go get -u=patch` carries the fix to everyone.

### What stays human

- Deciding to roll early: the schedule waits for Chrome for Testing to promote the milestone to Stable. Taking a milestone before that, or a Security roll, is a `workflow_dispatch` with a version.
- Reviewing the Roll: the reviewed pull request is the trust anchor for every managed-browser hash (ADR-0005), so its reviewer reads the hash diff as the thing being approved. The workflow opens it and merges nothing.

## The release

Cutting a release is one `workflow_dispatch` of `.github/workflows/release.yml`, run from `main` (spec #33, sections 14 and 15; ADR-0008; ticket #60):

```sh
gh workflow run release.yml -f version=v0.1.0-rc.1                # the release candidate
gh workflow run release.yml -f version=v0.1.0                     # the release it is promoted to
gh workflow run release.yml -f version=v0.1.1 -f dry-run=true     # all of it, published to throwaway tags and deleted
```

What is released is `main`'s head at the moment of the dispatch, by SHA, so a `main` that moves while the run works cannot slip into it. Nothing else in the repository creates a tag: merging a Roll opens no release, and there is no version constant in the code, so a release is named by what is typed here and nowhere else.

The run validates before it writes anything: the version's shape (`vX.Y.Z`, or `vX.Y.Z-rc.N` with the candidate numbered from 1) and that it is above every version already tagged, every check the `main` ruleset requires green on that very commit (a skipped one counts, which is what the dependency review Gate is on a push to `main`), the pins re-derived and re-rendered (`-check`), and the protocol regenerated to the committed bytes. Then it works in the order of what cannot be undone: both architectures build the image and push it under no tag at all; the two digests become one manifest per tag, each with its attestations; the `versions.json` row is committed on `main`; and the GitHub Release is created last, which is what creates the tag. So a run that fails leaves at worst an untagged manifest in the registry and, past the commit, a row on `main` that the release it names will carry.

The commit, the tag and the Release are the automation identity's (#58): the `main` and `v*` rulesets bind everyone but their bypass actors, and `GITHUB_TOKEN` is not one.

### The Release body

The body is the three pins on the first line, the Release preamble under it when the release has one, and then the pull-request list GitHub generates from the labels of `.github/release.yml`: the Roll first in a section of its own, then enhancements, bug fixes, documentation, dependencies and everything else. `-rc.N` is marked a pre-release, and `latest` moves only for a release that is not a candidate.

A Release preamble is `docs/releases/<tag>.md`, committed through a reviewed pull request before the dispatch and read from the tagged commit; a release with nothing to say beyond its pins and its pull requests has none. Two placeholders are filled in, and any other `{{...}}` fails the release rather than reaching the Release page:

| Placeholder | What it becomes                                                                                          |
| ----------- | -------------------------------------------------------------------------------------------------------- |
| `{{rc}}`    | the candidate's number, and for a release that is not a candidate the last candidate of the same version |
| `{{date}}`  | the day the release is cut, `YYYY-MM-DD` in UTC                                                          |

### versions.json

The root `versions.json` maps every release to the three pins it carried, one row per release, newest last, and there is no changelog file. The workflow appends the row through the pins printer (`go run ./internal/tools/print-pins -json`), so nothing parses Go source, and writes the bytes prettier writes, since the generate Gate runs over the commit it makes.

### The image

`ghcr.io/headlesslab/wand` is published only by this workflow, never by the image Gate. Each architecture is built natively on its own runner and pushed by digest, under no tag; the two digests then become one manifest per tag, `vX.Y.Z` or `vX.Y.Z-rc.N` (plus `latest` when it is not a candidate) for the runtime image and `dev` for the development one. A published tag is never rebuilt in place.

Every manifest published carries a build-provenance attestation and an SPDX SBOM attestation, pushed to the registry beside it and recorded in Sigstore's public log, and the run itself verifies each tag before it goes on:

```sh
gh attestation verify oci://ghcr.io/headlesslab/wand:v0.1.0 -R headlesslab/wand
```

The bill of materials is read off the registry by Trivy, which resolves a multi-architecture manifest to the runner's own, so what it lists is the linux/amd64 image; both architectures are built from one Dockerfile and one base image digest, so the package set is the same and the architecture is not.

A dry run does all of this under throwaway tags (`dry-run-<run id>-...`), verifies them and deletes them at the end, and creates no commit, no tag and no Release. The untagged manifests it pushes stay, as every image's do until a tag names them.

### Promotion

`v0.1.0` is cut from the last candidate once at least 14 days have passed since `rc.1`, at least 7 since the latest candidate, and no issue labelled `release-blocking` is open. Any change makes `rc.N+1` instead, a Roll landing mid-soak included. That label is a repository label of its own, created by hand when this workflow landed, so a rebuilt repository needs `gh label create release-blocking` before its first soak.

The workflow releases `main`'s head, so at promotion `main` must still be the candidate's commit plus the promotion's own documentation. What the promotion adds is `docs/releases/v0.1.0.md`, the same preamble under a promotion header; nothing else may have landed:

```sh
git diff v0.1.0-rc.1..main -- . ':!docs/releases' ':!versions.json'   # empty, or it is rc.N+1
```

Afterwards the candidate's own notes get a one-line banner pointing at `v0.1.0`, which immutable releases still allow, and no other Release page is edited.

### A bad release

A tag is never deleted or moved, so a release that should not have shipped is retracted rather than removed (ADR-0008):

1. Open a pull request adding a `retract` directive to `go.mod`, naming the version and why in one line:

   ```go
   retract v0.1.1 // the manager refused every launch on linux/arm64
   ```

   The directive reaches consumers through the release that carries it, so it ships with the fix, not on its own.

2. Cut the fixed patch with this workflow. `latest` moves to it, and `go get -u=patch` carries it to everyone (ADR-0008).

   This works while the bad release is the newest one, which is what a release found bad normally is: the workflow releases `main`'s head and refuses a version that is not above every tag published, so a patch for an older minor has nowhere to be cut from. Fixing one would take a release branch, which the baseline has none of; until it does, the fix goes out on top.

3. Mark the bad Release withdrawn: edit its notes to open with one line naming the release that replaces it, and clear its latest marker. Immutable releases lock a published tag and its assets; the title, the notes and the pre-release and latest markers stay editable, which is what makes this possible.

Nothing is published over the bad release's image tags either; the fixed patch publishes its own.

### What stays human

- **Deciding the version and dispatching it.** The workflow validates a version, it does not choose one: a Roll makes the next minor, a fix the next patch (ADR-0008).
- **The Release preamble**, when the release has something to say: a reviewed pull request adding `docs/releases/<tag>.md` before the dispatch.
- **The soak**, its `release-blocking` label and the promotion decision above.
- **The package's visibility.** GHCR creates a package private on its first push and exposes no REST endpoint for the setting, so the first release is followed by making `ghcr.io/headlesslab/wand` public and linking it to the repository, in the package's settings.
- **A dry run's throwaway tags**, when `GITHUB_TOKEN` was not allowed to delete a package version: the run says which tags are still there, and they go in the package's settings.

## The Nightly

Once a night at 02:41 UTC, and on demand, `.github/workflows/nightly.yml` proves `main` again on everything a Gate cannot ask of a pull request. Nothing in it is a required check: a red job blocks no merge and no release, and reaches a maintainer as an issue instead (spec #33, section 13; ADR-0008; ticket #61).

| Job                                                             | Where                      | What it proves                                                                                                                                         | What a red usually is                                                                                                         |
| --------------------------------------------------------------- | -------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------- |
| the seven `Tier 1 rerun …` jobs                                 | the Gate's four platforms  | the Gate's own matrix, rerun with no commit under it, from the same steps (`.github/actions/tier1`)                                                    | a flake, or a runner image that moved under a suite nobody changed                                                            |
| `System browser ubuntu-latest`, `System browser windows-11-arm` | those two runners          | Browser resolution's discovery, on the browser the runner image ships, which every Gate job bypasses with `WAND_BROWSER_BIN`                           | the runner image dropped its Chrome, or discovery stopped finding one; on windows/arm64 this is the only place the suite runs |
| `Examples`                                                      | `ubuntu-latest`            | every `Example` function of the root suite and the whole `lib/examples/e2e-testing` module, each on a browser of its own                               | an example that drifted from its `// Output:` comment, or one that started reaching for a public host                         |
| `Updated graph (go get -u)`                                     | `ubuntu-latest`, Go 1.21.x | every module moved as far forward as the Go floor allows, then the suite on that graph (ADR-0006)                                                      | wand's own code breaking on a dependency that moved; a module that could not come is a notice and a summary row, not a red    |
| `Support window`                                                | `ubuntu-latest`            | the suite on the oldest Chrome wand claims to work with: the Target Chrome's milestone less three, at Chrome for Testing's last known good build of it | something Chrome changed in the three milestones since; best-effort support, so this is news rather than a fault              |
| `govulncheck on main`                                           | `ubuntu-latest`            | the vulnerability database against code nobody has changed                                                                                             | an advisory published since the last pull request that reaches wand's own code                                                |
| `Trivy on the published images`                                 | `ubuntu-latest`            | `ghcr.io/headlesslab/wand:latest` and `:dev` as the registry holds them, which no Gate ever sees                                                       | a base image that grew a fixable CRITICAL after the release was cut; the answer is a Dependabot digest bump and a patch       |
| `Report`                                                        | `ubuntu-latest`            | —                                                                                                                                                      | the App token: with it rejected every issue the run would have opened is gone, so this job opens one about itself             |

No job here may ever carry a Gate job's name, which is why every Tier 1 entry says "rerun". A job's name is its check run's name, the Nightly runs on the very commit the Gate ran on, and the release workflow reads that commit's required checks back through `check-runs?filter=latest`, which keeps the newest check run per name: a Nightly job sharing a Gate job's name would replace the Gate's answer with its own, and a red one would hold a release that no Nightly is allowed to hold.

The Tier 1 matrix appears twice, once here and once in `gate.yml`, but its steps do not: they are a composite action under `.github/actions/tier1`, so the browser pin assertion, what the suite is run with and the coverage arithmetic have one place to be fixed. That action uses no other action on purpose — Dependabot's `github-actions` updater reads `.github/workflows` and a root `action.yml` and nothing else, so a SHA pinned there would be a pin nothing moves; the checkout, the toolchain, the artifact upload and the zero-leftover step stay in each workflow, where Dependabot sees them.

### The issue a red job opens

`internal/tools/nightly-report` runs as the last job, whatever the jobs above it did. For every job that ended in `failure` or `timed_out` it opens one issue titled `Nightly: <job name> failed`, labelled `needs-triage`, or comments on the open issue with exactly that title. So the same job failing on four nights is one issue with four comments, a second job going red is an issue of its own, and a job that was cancelled or skipped — the image scan before the first release exists — is not red at all.

Two tokens, because they can do different things: the run's own `GITHUB_TOKEN` reads the jobs of the run, an Actions permission the automation App does not have, and the App's token opens the issues, so they come from the automation identity rather than from `github-actions[bot]` (section [The automation identity](#the-automation-identity)). The reporter is green once it has reported — the red belongs to the jobs it reports on — and a reporter that cannot report gets the notice the Roll gives itself, opened with `GITHUB_TOKEN` because the App token is one of the things that may have failed.

Closing the issue is a human's to do, once the job is green again: nothing here closes one, and a Nightly that goes green says so by opening nothing.

### Running one by hand

```sh
gh workflow run nightly.yml --repo headlesslab/wand
```

The reporter alone, against any run of any workflow, writing nothing:

```sh
GH_TOKEN=$(gh auth token) go run ./internal/tools/nightly-report -repo headlesslab/wand -run <run id> -dry-run
```

Locally, one job at a time, from the module root:

```sh
# The Support window: the Chrome, then the suite on it, as the job runs it.
version=$(go run ./internal/tools/support-window)
export WAND_BROWSER_BIN=$(go run ./cmd/wand-fetch-browser -version "$version")
go run ./internal/tools/ci-test -race -count=1 -run=^Test ./...

# The examples, which the Gate's -run=^Test leaves out.
go run ./internal/tools/ci-test -count=1 -timeout=40m -run '^Example' ./...
go run ./internal/tools/ci-test -count=1 ./lib/examples/e2e-testing

# The graph at its newest the floor allows, on a checkout you do not mind rewriting.
GOWORK=off go run ./internal/tools/updated-graph && GOWORK=off go test -run '^Test' ./...
```

A version other than the Target Chrome has no pinned archive hash, so `wand-fetch-browser -version` says the download is unverified and takes it over TLS anyway; the Nightly narrows that to Google's own bucket with `WAND_BROWSER_HOSTS`, never a mirror (ADR-0005).

### What stays human

- **Closing a Nightly issue**, and splitting whatever it turns out to be into an issue of its own.
- **The pull request an `Updated graph` run prompts.** `go get -u` is run to find out, never to commit: `go.mod` moves through a reviewed pull request (section [Dependabot](#dependabot)). `internal/tools/updated-graph` is what runs there rather than the bare `go get -u ./...`, because that command takes the newest version there is and, with `GOTOOLCHAIN=local`, one module that has raised its own Go floor above wand's then fails it outright — before a single test has run. The tool moves every module as far as the floor allows, so the suite runs on the updated graph, and names the ones held back with the Go version they wanted. Taking one of those means moving the floor, which ADR-0003 anchors to openEuler LTS: a decision, not a bump.
- **Reading a red `Support window`.** The three milestones below the Target Chrome are best-effort, not tested support: what breaks there is written down, among the migration guide's known limitations when it affects users, and holds no release.

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

| Gate                | Where it runs                                                    | What reds it                                                                                                                                                                                           |
| ------------------- | ---------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `govulncheck ./...` | a step of the `Tier 1 linux/amd64 (Go stable)` job               | a vulnerable function of a dependency or of the standard library that wand's own code can reach                                                                                                        |
| Dependency review   | the `Dependency review` job, on the pull request event alone     | in what the pull request adds to the dependency graph: an advisory of high severity or worse, or a licence outside MIT, BSD-2-Clause, BSD-3-Clause, Apache-2.0, ISC, MPL-2.0 and Go's own patent grant |
| CodeQL              | GitHub's own workflow, from the default setup the bundle applies | an alert the pull request adds, of high security severity or worse or at error level; the `main` ruleset's code scanning rule is what holds the merge                                                  |

`go run ./internal/tools/govulncheck` is the same scan a developer runs, at the version `internal/devutil` pins; pass arguments to reach one package instead of `./...`. It reads the Go vulnerability database, so it needs the network, and x/vuln's Go floor is far above wand's, so with `GOTOOLCHAIN=local` only the stable job can build it: hence one scan, on that job. Source mode is what makes it quiet enough to gate on — a vulnerability in a package wand imports but never calls into is reported and passes. An advisory published against unchanged code reds the next run; the Nightly rerun on `main` (section [The Nightly](#the-nightly)) is what surfaces one the same morning rather than at the next pull request.

Dependency review reads GitHub's dependency graph, which is a repository setting with no REST endpoint and so is not in the bundle: with the graph off the job reds in five seconds with "Dependency review is not supported on this repository", whatever the pull request contains. See "What stays human" above.

Widening the licence allowlist is a reviewed change to `gate.yml`. It has been widened once, by `LicenseRef-scancode-google-patent-license-golang`, which is not a seventh licence: it is how Go's `PATENTS` file is scanned, every `golang.org/x` module and Go itself carry it beside a BSD-3-Clause `LICENSE`, and the scan gives the two as one conjunction, so a check that knows only `BSD-3-Clause` refuses the pair. It grants a patent licence rather than restricting anything, wand's graph has carried it since the snapshot, and without it `golang.org/x/sys` could not be upgraded at all — nor could the hand pull request the Nightly's `go get -u` prompts (#61). `gosec` stays disabled in `.golangci.yml`, as upstream had it: CodeQL is the source-analysis Gate.

### Dependabot

`.github/dependabot.yml`, all weekly: `github-actions` at the root (the SHA pins of every workflow), `docker` on `docker/` (the base image digests), and `npm` on `internal/tools/` as one group, so a week's linter updates arrive as one pull request with one resolved lockfile. Go modules get no version pull request at all — the limit of zero says so in the file — while Dependabot security updates, which the settings bundle turns on, are not subject to that limit and open one as soon as an advisory matches. Everything else in `go.mod` moves through a hand pull request, prompted by a satellite release or by the Nightly `go get -u ./...` (section [The Nightly](#the-nightly)).

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
- **Pooled testers**: the root suite keeps one browser per parallel test (`go test -parallel`, GOMAXPROCS by default), each behind a `MockClient` that can stub one CDP call, or the first few calls of one method. A failing `Must` call on a pooled browser, its pages or their elements fails the test and ends it (inside a `g.Panic` block it panics, as the block expects), so a red test never kills the binary and the run keeps its cleanup and its coverage. A test that failed, or that hit `-timeout-each` (one minute), has its browser closed and replaced, so nothing it left in a browser reaches another test; a test still running one `-timeout-each` after its browser was closed ends the run, as the `go test` timeout would, after the other browsers are gone.
- **Zero leftover**: when the run ends, whatever its outcome, every pooled browser is closed and its user data directory under `os.TempDir()/wand/user-data` removed. Tests that launch a browser of their own do the same through the harness helpers (`g.launch` in the root suite, `launch` in `lib/cdp`, `stop` in `lib/launcher`); a browser `wand.New().MustConnect()` launched itself is cleaned up by `Browser.Close`, and a launch that fails removes the temporary directory it made up. `Launcher.Cleanup` is bounded: a browser still running ten seconds after the call is killed, and a directory a helper process still holds is retried for as long. Every Tier 1 job ends, whatever its outcome, with `go run ./internal/tools/zero-leftover`, which waits up to 30 s and fails on any chrome or chromium process still running or anything still under `launcher.DefaultUserDataDirPrefix`, listing what it found. It counts every browser on the machine, so on a developer machine it is meaningful only with no browser of your own open. The one directory a test leaves on purpose is the User mode profile, `launcher.DefaultUserModeDir` under the user's configuration directory: `TestUserModeBrandedChrome` launches on it, since the fix it proves is that very default, and kills its browser without `Cleanup`, which would remove a profile that is the developer's own; every other test that launches User mode names a temporary directory. The examples clean up after themselves too: each one closes the browser it launched, and the two that launch through the launcher by hand call Cleanup, so an examples run leaves no directory behind either (#52).
- **Running locally**: run one browser suite at a time (`go test .`, `go test ./lib/cdp`, `go test ./lib/launcher`). The ci-test wrapper, `go run ./internal/tools/ci-test <go test arguments>`, sets `GODEBUG=tracebackancestors=100` for the leak checker and is what the Gate runs. Pass `-run '^Test'` as the Gate does: the root suite's default pattern runs the examples too, each of which launches a browser of its own outside the pool, and its default `-timeout` of five minutes (`got.DefaultFlags`) kills the binary from outside, past every cleanup, so a run it ends leaves the pooled browsers' directories behind. The root suite writes one CDP log per test under `tmp/cdp-log/<run>/<tester>/`, kept for a failed test (the Tier 1 jobs upload the directory) and removed for a passed one.
- **What stays skipped**: only environment guards. `TestFonts` runs in a container only, `TestBinarySize` outside Windows and containers only, `TestProfileDir` with `-test-profile-dir` only, `TestLaunchXVFB` where `xvfb-run` is installed, `TestUserModeBrandedChrome` (the Confirmed fix for rod #1189, ticket #47) where `LookPath` finds branded Google Chrome, which alone refuses remote debugging on its default profile, and on Linux only with a display or `xvfb-run`; the Gate requires its PASS line on `ubuntu-latest`, `windows-latest` and `macos-latest`, the three runners that ship one. No test is skipped for flakiness, listens on a fixed port, or reaches the public internet: the pooled browsers, and the ones `g.launch` starts, run with `--host-resolver-rules` that resolve no host but loopback, so a test that tries fails on every machine. The examples name no public host either (#52): they drive the pages under `fixtures/examples`, served on a port the OS picks, so `go test -run Example ./...` is green with the network away, and `TestExamplesOffline` reads the three example files and fails on an example that both runs and names a public host, so one added later cannot put the set back on the internet. The Managed browser download is the one network access, taken only when no browser is found, before any browser starts.
