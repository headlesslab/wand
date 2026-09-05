# Import go-rod as a snapshot, not a history-preserving fork

wand takes over go-rod/rod, whose code has been frozen since December 2024 with a maintainer who has been contacted and is not continuing. We copy the code from a single upstream commit into a fresh history instead of merging upstream's git history, because wand is a new community project that wants a clean break and a history that starts with its own decisions. We accept that `git blame` stops at the snapshot and that upstream patches must be applied as patches rather than cherry-picked by hash. Upstream's MIT notice is retained, and the snapshot commit is recorded in the repo so provenance stays traceable (the mechanism is decided separately).

The snapshot is upstream commit `393ac0d60b53f3c4a9b2a6504d250cbada55b546` (2024-12-07), upstream's last code commit, rather than the last tagged release v0.116.2 (2024-07-12): the eight code commits between them are fixes the baseline would otherwise have to harvest as patches, and nothing after `393ac0d` touches code. Provenance lives in `NOTICE` (attribution and the snapshot hash), in `LICENSE` (upstream's MIT text with both copyright lines), and in the import commit message (the list of upstream paths left out). The import lands as two commits, a byte-identical snapshot minus those exclusions and then the rename, so the snapshot can be diffed against upstream at any time.

## Considered Options

- **Hard fork preserving upstream history**: keeps blame and cherry-pick, but ties wand's history to upstream's and reads as "go-rod under a new name".
- **Clean-room rewrite**: full freedom, but months without a usable library and nothing to prototype API changes against.
