package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/headlesslab/wand/lib/launcher/pins"
	"github.com/ysmood/got"
)

var setup = got.Setup(nil)

// native is a run on the machine's own platform, as every image job is.
var native = options{image: "ghcr.io/headlesslab/wand", dir: "/src"}

func TestBuildsNothingPushed(t *testing.T) {
	g := setup(t)

	g.Eq(native.buildRuntime(), []string{
		"docker", "build", "--file", "docker/Dockerfile", "--tag", "ghcr.io/headlesslab/wand", ".",
	})

	// The development image is built on the runtime image just built, not on
	// whatever the registry holds under that name.
	g.Eq(native.buildDev(), []string{
		"docker", "build", "--file", "docker/dev.Dockerfile", "--tag", "ghcr.io/headlesslab/wand:dev",
		"--build-arg", "base=ghcr.io/headlesslab/wand", ".",
	})

	for _, args := range [][]string{native.buildRuntime(), native.buildDev(), native.runSuite()} {
		line := strings.Join(args, " ")
		g.False(strings.Contains(line, "push"))
		g.False(strings.Contains(line, "login"))
	}
}

func TestPlatformOnlyWhenAsked(t *testing.T) {
	g := setup(t)

	cross := native
	cross.platform = "linux/arm64"

	g.Has(strings.Join(cross.buildRuntime(), " "), "--platform linux/arm64")
	g.Has(strings.Join(cross.run(cross.image, "chrome", "--version"), " "), "--platform linux/arm64")
	g.False(strings.Contains(strings.Join(native.buildRuntime(), " "), "--platform"))
}

func TestRunSuiteMountsTheCheckout(t *testing.T) {
	g := setup(t)

	// The checkout is mounted where the development image works, under the
	// separator docker takes, which is what filepath.ToSlash is for on a
	// Windows working directory (there, and only there, it rewrites the one
	// the operating system gave).
	local := options{image: "wand", dir: filepath.FromSlash("/src/wand")}

	g.Eq(local.runSuite(), []string{
		"docker", "run", "--rm",
		"--volume", "/src/wand:/wand", "--workdir", "/wand",
		"wand:dev", "bash", "-c", suite,
	})

	// The Zero leftover check runs in the container the suite ran in, and
	// the suite's own status is what the run exits with.
	g.Has(suite, "./internal/tools/ci-test -race -count=1 -run=^Test ./...")
	g.Has(suite, "./internal/tools/zero-leftover")
	g.Has(suite, "exit $status")
}

func TestChromeVersion(t *testing.T) {
	g := setup(t)

	g.Eq(chromeVersion("Google Chrome for Testing "+pins.ChromeVersion+"\n"), pins.ChromeVersion)
	g.Eq(chromeVersion("Chromium 153.0.0.0 snap\n"), "snap")
	g.Eq(chromeVersion("   \n"), "")
}
