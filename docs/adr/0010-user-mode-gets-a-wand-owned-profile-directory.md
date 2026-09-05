# Give user mode a wand-owned profile directory instead of Chrome's default profile

Upstream's `launcher.NewUserMode()` launches the system browser with `--remote-debugging-port` and no `--user-data-dir`, so it attaches to the browser's default profile and with it the user's existing logins and extensions. Since Chrome 136 (May 2025) branded Google Chrome ignores `--remote-debugging-port` and `--remote-debugging-pipe` on the default user-data directory and exits with "DevTools remote debugging requires a non-default data directory"; the run against Chrome 152 (#14) reproduced it in 1.4 s, and upstream's own `TestLaunchUserMode` never exercises the path, so the suite stayed green while the feature was broken (rod #1189, #1184). Chrome for Testing keeps the old behaviour, but user mode exists precisely to drive the browser the user already has. wand's `NewUserMode()` therefore defaults `--user-data-dir` to `os.UserConfigDir()/wand/user-mode`, a persistent directory wand owns, so logins, extensions and settings survive between runs while staying separate from Chrome's own profile; `Launcher.UserDataDir()` overrides it, and a caller who points it at Chrome's real profile directory gets Chrome's refusal unchanged. Puppeteer and Playwright made the same move: neither offers a launch on the default profile. We accept that a user mode session no longer sees the sign-ins of the user's daily browser, which was the feature's original appeal and which no CDP client can offer on Chrome 136 and later.

## Considered Options

- **Keep the default profile and fail with a clearer error**: a better failure, not a fix; every user mode launch on current Chrome fails.
- **Copy the default profile into a wand directory on first run**: races the profile lock of a running Chrome, cookies under Windows app-bound encryption stay unreadable, and neither Puppeteer nor Playwright does it.
- **Recommend Chrome for Testing for user mode**: exempt from the rule, but a fresh browser with no logins defeats the purpose no less than a new directory does, and adds a 200 MB download.

## Consequences

- The directory follows the platform convention (`%AppData%\wand\user-mode` on Windows, `$XDG_CONFIG_HOME` or `~/.config/wand/user-mode` on Linux, `~/Library/Application Support/wand/user-mode` on macOS) and is not under the browser cache of ADR-0005, because a profile is user data, not a re-downloadable artefact.
- `NewUserMode()` keeps the orphan guard off by default (`Leakless(false)` semantics, ADR-0007), so a browser the user keeps working in is not killed when the wand process exits.
- The confirmed fix for rod #1189/#1184 is a unit test on the flag set plus a launch test against the branded Google Chrome present on GitHub's `ubuntu-latest`, `windows-latest` and `macos-latest` runners; Chrome for Testing would not reproduce the failure, and `ubuntu-24.04-arm` ships no Chrome, so the test skips there with a reason.
- The migration guide records the change; a go-rod user who relied on the default profile can set `UserDataDir()` explicitly and is refused by Chrome 136 and later exactly as before.
