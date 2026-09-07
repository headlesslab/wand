# Overview

This is a sample project to demonstrate how to use wand to setup an end-to-end testing (e2e testing) project.
The test cases will run in parallel.

Use standard Go commands to test the project, such as run `go test` to execute all tests.

The app under test is the calculator under `app/`, which `TestMain` serves on a port the operating system picks.
Point `app` at your own build output, or at a server you start yourself, to test your own app the same way.

## Debugging

Same as go-rod's tutorial here: [See what's under the hood](https://go-rod.github.io/#/get-started/README?id=see-what39s-under-the-hood)
