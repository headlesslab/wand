# Anchor the Go floor to openEuler's current LTS

wand must build with the distro-packaged Go on the domestic platforms its users deploy to, and openEuler LTS is the only such platform whose packaged Go is both current enough to build upstream's code and published on x86_64, aarch64 and loongarch64. We therefore declare `go 1.21` (openEuler 24.03 LTS's Go) in `go.mod`, add no `toolchain` directive, and raise the floor only when a new openEuler LTS ships a newer Go, never above what that LTS ships natively. This forgoes Go 1.22–1.24 language features (range-over-int, range-over-func, generic type aliases) for as long as 24.03 is the current LTS, and it leaves Kylin V10 (Go 1.13–1.15) and openEuler 22.03 (Go 1.17) to cross-compilation or tarball installs, since going lower would mean rewriting upstream's generics and forking fetchup and got.

## Considered Options

- **Two latest Go majors** (the charting-time preference): no domestic platform builds natively; every build needs a toolchain download or a tarball.
- **Go 1.17 or lower**: covers openEuler 22.03 and Kylin V10 natively, at the cost of rewriting upstream's generic code and forking fetchup and got, which all declare 1.21.
