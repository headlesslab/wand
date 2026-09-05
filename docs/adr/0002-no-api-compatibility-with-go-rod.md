# No API compatibility promise with go-rod

wand is positioned as a new community project rather than a drop-in replacement for go-rod: the module is `github.com/headlesslab/wand`, the root package is `wand`, and no compatibility contract with go-rod's API is offered. The baseline release keeps go-rod's method surface in practice, but only because redesign is deferred, not as a promise; this frees the later API modernization from carrying go-rod's shape. Users migrating from go-rod must at minimum change import paths and the package qualifier.
