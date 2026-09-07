// Package main is wand's image build script: it builds the two container
// images from the checkout it is run in, proves what the image Gate proves,
// and pushes nothing. Publishing the tags, which is where the two
// architectures meet in one manifest and take their attestations, is the
// release workflow's (ticket #60), so this tool never logs in to a registry
// (spec #33, section 14; ticket #55).
//
//	go run ./internal/tools/docker [-suite]
//
// It writes the .dockerignore, builds the runtime image and then the :dev
// image on top of the one it has just built, runs the manager and Chrome out
// of the runtime image and holds the version Chrome reports to the Target
// Chrome pin, and with -suite runs wand's whole suite and the Zero leftover
// check inside the :dev image against this checkout. Each image is built for
// the machine's own platform, as the image jobs build theirs. Run it from
// the module root, as go generate runs the generators.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/headlesslab/wand/internal/devutil"
	"github.com/headlesslab/wand/lib/launcher/pins"
	"github.com/headlesslab/wand/lib/utils"
)

const (
	// image is the runtime image's name and dev the development image's, the
	// two names the release workflow publishes under.
	image = "ghcr.io/headlesslab/wand"
	dev   = image + ":dev"

	// workdir is where runSuite mounts the checkout inside the container,
	// which is also the working directory the development image sets.
	workdir = "/wand"
)

func main() {
	suite := flag.Bool("suite", false,
		"run wand's whole suite and the Zero leftover check inside the development image after the build")
	flag.Parse()

	dir, err := os.Getwd()
	utils.E(err)

	utils.E(devutil.DockerIgnore(dir))

	devutil.ExecLine(true, "", buildRuntime()...)
	devutil.ExecLine(true, "", buildDev()...)

	// The manager is the image's entrypoint, so a manager that cannot start
	// is an image that cannot serve; -h exits 0 and launches nothing.
	devutil.ExecLine(true, "", run(nil, image, "wand-manager", "-h")...)

	// The Target Chrome is a pin and not a lookup, which is what makes the
	// linux/amd64 and the linux/arm64 image carry the same version: each
	// image job holds its own to the same pin.
	out := devutil.ExecLine(true, "", run(nil, image, "chrome", "--version")...)
	if got := reportedVersion(out); got != pins.ChromeVersion {
		utils.E(fmt.Errorf("the browser in %s is Chrome %q, not the Target Chrome pin %q",
			image, got, pins.ChromeVersion))
	}

	if *suite {
		devutil.ExecLine(true, "", runSuite(dir)...)
	}
}

// buildRuntime builds the runtime image from docker/Dockerfile.
func buildRuntime() []string {
	return []string{"docker", "build", "--file", "docker/Dockerfile", "--tag", image, "."}
}

// buildDev builds the development image on top of the runtime image just
// built, rather than on whatever the registry holds under that name.
func buildDev() []string {
	return []string{
		"docker", "build", "--file", "docker/dev.Dockerfile", "--tag", dev,
		"--build-arg", "base=" + image, ".",
	}
}

// run is one command in a throw-away container of image, with docker flags
// of its own before the image name.
func run(dockerArgs []string, image string, cmd ...string) []string {
	args := append([]string{"docker", "run", "--rm"}, dockerArgs...)

	return append(append(args, image), cmd...)
}

// runSuite runs the suite over the checkout at dir, mounted into the
// development image, so that what the container tests is the tree the script
// was started from and the CDP logs of a failed test land back on the host
// for the image job to upload. A Windows path reaches docker under the
// separator it takes.
func runSuite(dir string) []string {
	mount := filepath.ToSlash(dir) + ":" + workdir

	return run([]string{"--volume", mount, "--workdir", workdir}, dev, "bash", "-c", suite)
}

// suite is what the development image runs: wand's whole suite through the
// ci-test wrapper, then the Zero leftover check in the same PID namespace,
// so that a browser the suite left behind is found while it is still there.
// The examples stay out with -run=^Test, as in every Tier 1 job: each
// launches a browser of its own outside the tester pool, and they belong to
// the examples Nightly (#61).
const suite = `go run ./internal/tools/ci-test -race -count=1 -run=^Test ./...
status=$?
go run ./internal/tools/zero-leftover || status=1
exit $status
`

// dottedVersion is a browser version as --version reports it, four numbers
// deep, as in "Google Chrome for Testing 153.0.8010.12".
var dottedVersion = regexp.MustCompile(`\b\d+(?:\.\d+){3}\b`)

// reportedVersion is the version in the answer a browser gives --version,
// and the empty string when the answer holds none.
func reportedVersion(out string) string {
	return dottedVersion.FindString(out)
}
