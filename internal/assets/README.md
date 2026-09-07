# Assets

The static files wand embeds: the mouse pointer the visual trace draws, and the two pages the monitor
server serves. `assets.go` is generated from the HTML beside it by `go generate` (see `generate/`).

The package is internal, so it is not part of wand's API; go-rod exported the same constants from
`lib/assets`. Copy what you need if you were using them (see [the migration guide](../../docs/migrating-from-go-rod.md#3-symbols-that-left-the-public-api)).
