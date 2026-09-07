# Overview

This is a sample project to demonstrate how to use wand to setup an end-to-end testing (e2e testing) project.
The test cases will run in parallel.

Use standard Go commands to test the project, such as run `go test` to execute all tests.

The app under test is the calculator under `app/`, which `TestMain` serves on a port the operating system picks.
Point `app` at your own build output, or at a server you start yourself, to test your own app the same way.

## Debugging

Run the tests with the `-wand` flag to watch what they do: `go test -wand=show,trace,slow=1s` puts the browser
on screen, draws every input wand sends and slows each action down to a second. `-wand=devtools` opens DevTools
in each new tab, and `-wand=monitor` serves a page showing every tab. The options are documented with
[`lib/defaults`](../../defaults).
