package main

import (
	"os"
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

	// Nothing a build without -push runs reaches a registry: the tags are the
	// release workflow's.
	for _, args := range [][]string{buildRuntime(), buildDev(), run(nil, image), runSuite("/src")} {
		line := strings.Join(args, " ")
		for _, word := range []string{"push", "login", "--load", "--output"} {
			g.Desc("%s", line).False(strings.Contains(line, word))
		}
	}
}

func TestPushCreatesNoTag(t *testing.T) {
	g := setup(t)

	// Both images are pushed under no tag at all: the registry holds each by
	// its digest, and a tag of the release is the manifest of both
	// architectures that the release workflow makes from them.
	g.Eq(pushRuntime("tmp/metadata-runtime.json"), []string{
		"docker", "buildx", "build", "--file", "docker/Dockerfile",
		"--output", "type=image,name=ghcr.io/headlesslab/wand,push-by-digest=true,name-canonical=true,push=true",
		"--metadata-file", "tmp/metadata-runtime.json", ".",
	})

	// The development image is built on the runtime image just pushed, by the
	// digest that push gave: a builder exporting to a registry holds no image
	// of its own for a tag to name.
	g.Eq(pushDev("ghcr.io/headlesslab/wand@sha256:a", "tmp/metadata-dev.json"), []string{
		"docker", "buildx", "build", "--file", "docker/dev.Dockerfile",
		"--build-arg", "base=ghcr.io/headlesslab/wand@sha256:a",
		"--output", "type=image,name=ghcr.io/headlesslab/wand,push-by-digest=true,name-canonical=true,push=true",
		"--metadata-file", "tmp/metadata-dev.json", ".",
	})

	for _, args := range [][]string{pushRuntime("m.json"), pushDev("base", "m.json")} {
		g.Desc("%s", strings.Join(args, " ")).False(strings.Contains(strings.Join(args, " "), "--tag"))
	}
}

func TestTheDigestBuildxPushed(t *testing.T) {
	g := setup(t)

	dir := g.Testable.(*testing.T).TempDir()
	metadata := filepath.Join(dir, "metadata.json")

	g.E(os.WriteFile(metadata, []byte(`{
		"containerimage.digest": "sha256:1a2b3c4d",
		"image.name": "ghcr.io/headlesslab/wand"
	}`), 0o600))

	got, err := digest(metadata)
	g.E(err)
	g.Eq(got, "sha256:1a2b3c4d")

	_, err = digest(filepath.Join(dir, "missing.json"))
	g.Err(err)

	// A build that pushed nothing leaves a file with no digest in it, which is
	// a release that must not go on to make a manifest of it.
	g.E(os.WriteFile(metadata, []byte(`{"image.name": "ghcr.io/headlesslab/wand"}`), 0o600))
	_, err = digest(metadata)
	g.Err(err)
	g.Has(err.Error(), "names no image digest")

	g.E(os.WriteFile(metadata, []byte("not json"), 0o600))
	_, err = digest(metadata)
	g.Err(err)
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
