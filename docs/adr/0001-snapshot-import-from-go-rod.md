# Import go-rod as a snapshot, not a history-preserving fork

wand takes over go-rod/rod, whose code has been frozen since December 2024 with a maintainer who has been contacted and is not continuing. We copy the code from a single upstream commit into a fresh history instead of merging upstream's git history, because wand is a new community project that wants a clean break and a history that starts with its own decisions. We accept that `git blame` stops at the snapshot and that upstream patches must be applied as patches rather than cherry-picked by hash. Upstream's MIT notice is retained, and the snapshot commit is recorded in the repo so provenance stays traceable (the mechanism is decided separately).

## Considered Options

- **Hard fork preserving upstream history**: keeps blame and cherry-pick, but ties wand's history to upstream's and reads as "go-rod under a new name".
- **Clean-room rewrite**: full freedom, but months without a usable library and nothing to prototype API changes against.
