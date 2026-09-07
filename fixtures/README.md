# Overview

The images under this folder were rendered from go-rod's `fixtures/design.sketch`, which is not part of wand's snapshot (see ADR-0001 and `NOTICE`).

`examples/` holds the pages the runnable examples in `examples_test.go` drive. They are served on an ephemeral loopback port, so every example executes offline and its output never changes under it (spec #33, section 12).
