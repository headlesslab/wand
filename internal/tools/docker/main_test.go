package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/headlesslab/wand/lib/launcher/pins"
	"github.com/ysmood/got"
)

var setup = got.Setup(nil)

func TestBuildsNothingPushed(t *testing.T) {
	g := setup(t)

	g.Eq(buildRuntime(), []string{
		"docker", "build", "--file", "docker/Dockerfile", "--tag", "ghcr.io/headlesslab/wand", ".",
	})

	// The development image is built on the runtime image just built, not on
	// whatever the registry holds under that name.
	g.Eq(buildDev(), []string{
		"docker", "build", "--file", "docker/dev.Dockerfile", "--tag", "ghcr.io/headlesslab/wand:dev",
		"--build-arg", "base=ghcr.io/headlesslab/wand", ".",
	})

	// Nothing this script runs reaches a registry: the tags are the release
	// workflow's.
	for _, args := range [][]string{buildRuntime(), buildDev(), run(nil, image), runSuite("/src")} {
		line := strings.Join(args, " ")
		for _, word := range []string{"push", "login", "--load", "--output"} {
			g.Desc("%s", line).False(strings.Contains(line, word))
		}
	}
}

func TestRunSuiteMountsTheCheckout(t *testing.T) {
	g := setup(t)

	// The checkout is mounted where the development image works, under the
	// separator docker takes, which is what filepath.ToSlash is for on a
	// Windows working directory (there, and only there, it rewrites the one
	// the operating system gave).
	g.Eq(runSuite(filepath.FromSlash("/src/wand")), []string{
		"docker", "run", "--rm",
		"--volume", "/src/wand:/wand", "--workdir", "/wand",
		"ghcr.io/headlesslab/wand:dev", "bash", "-c", suite,
	})

	// The Zero leftover check runs in the container the suite ran in, and
	// the suite's own status is what the run exits with.
	g.Has(suite, "./internal/tools/ci-test -race -count=1 -run=^Test ./...")
	g.Has(suite, "./internal/tools/zero-leftover")
	g.Has(suite, "exit $status")
}

func TestReportedVersion(t *testing.T) {
	g := setup(t)

	g.Eq(reportedVersion("Google Chrome for Testing "+pins.ChromeVersion+"\n"), pins.ChromeVersion)
	g.Eq(reportedVersion("Chromium 153.0.0.0 snap\n"), "153.0.0.0")
	g.Eq(reportedVersion("Chromium 153.0\n"), "")
	g.Eq(reportedVersion("   \n"), "")
}
