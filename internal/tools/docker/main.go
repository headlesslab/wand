// Package main is wand's image build script: it builds the two container
// images from the checkout it is run in, proves what the image Gate proves,
// and pushes nothing. Publishing the tags is the release workflow's, so this
// tool never logs in to a registry (spec #33, section 14; ticket #55).
//
//	go run ./internal/tools/docker [-image name] [-platform p] [-suite]
//
// It writes the .dockerignore, builds the runtime image and then the :dev
// image on top of the one it has just built, runs the manager and Chrome out
// of the runtime image and holds the version Chrome reports to the Target
// Chrome pin, and with -suite runs wand's whole suite and the Zero leftover
// check inside the :dev image against this checkout. Run it from the module
// root, as go generate runs the generators.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/headlesslab/wand/internal/devutil"
	"github.com/headlesslab/wand/lib/launcher/pins"
	"github.com/headlesslab/wand/lib/utils"
)

func main() {
	opts := options{}

	flag.StringVar(&opts.image, "image", "ghcr.io/headlesslab/wand",
		"the name to build the runtime image under; the development image takes the same name at :dev")
	flag.StringVar(&opts.platform, "platform", "",
		"the platform to build for, such as linux/arm64; empty builds for the machine's own, as every image job does")
	flag.BoolVar(&opts.suite, "suite", false,
		"run wand's whole suite and the Zero leftover check inside the development image after the build")
	flag.Parse()

	wd, err := os.Getwd()
	utils.E(err)
	opts.dir = wd

	utils.E(devutil.DockerIgnore(opts.dir))

	devutil.ExecLine(true, "", opts.buildRuntime()...)
	devutil.ExecLine(true, "", opts.buildDev()...)

	// The manager is the image's entrypoint, so a manager that cannot start
	// is an image that cannot serve; -h exits 0 and launches nothing.
	devutil.ExecLine(true, "", opts.run(opts.image, "wand-manager", "-h")...)

	// The Target Chrome is a pin and not a lookup, which is what makes the
	// linux/amd64 and the linux/arm64 image carry the same version: each
	// image job holds its own to the same pin.
	out := devutil.ExecLine(true, "", opts.run(opts.image, "chrome", "--version")...)
	if got := chromeVersion(out); got != pins.ChromeVersion {
		utils.E(fmt.Errorf("the browser in %s is Chrome %q, not the Target Chrome pin %q",
			opts.image, got, pins.ChromeVersion))
	}

	if opts.suite {
		devutil.ExecLine(true, "", opts.runSuite()...)
	}
}

// options is one run of the script.
type options struct {
	image    string
	platform string
	suite    bool
	dir      string
}

// dev is the development image's name: the runtime image's at :dev, the tag
// the release workflow publishes it under.
func (o options) dev() string {
	return o.image + ":dev"
}

// buildRuntime builds the runtime image from docker/Dockerfile.
func (o options) buildRuntime() []string {
	return append(o.build("docker/Dockerfile", o.image), ".")
}

// buildDev builds the development image on top of the runtime image just
// built, rather than on whatever the registry holds under that name.
func (o options) buildDev() []string {
	return append(o.build("docker/dev.Dockerfile", o.dev()), "--build-arg", "base="+o.image, ".")
}

// build is the common head of the two builds. Nothing is pushed and nothing
// is pulled that a digest does not name.
func (o options) build(dockerfile, tag string) []string {
	args := []string{"docker", "build", "--file", dockerfile, "--tag", tag}
	if o.platform != "" {
		args = append(args, "--platform", o.platform)
	}

	return args
}

// run is one command in a throw-away container of image.
func (o options) run(image string, cmd ...string) []string {
	return o.runWith(nil, image, cmd...)
}

// runWith is run with docker flags of its own between the platform and the
// image name.
func (o options) runWith(dockerArgs []string, image string, cmd ...string) []string {
	args := []string{"docker", "run", "--rm"}
	if o.platform != "" {
		args = append(args, "--platform", o.platform)
	}

	args = append(args, dockerArgs...)

	return append(append(args, image), cmd...)
}

// runSuite runs the suite over this checkout, mounted into the development
// image, so that what the container tests is the tree the script was started
// from and the CDP logs of a failed test land back on the host for the image
// job to upload.
func (o options) runSuite() []string {
	mount := filepath.ToSlash(o.dir) + ":" + workdir

	return o.runWith([]string{"--volume", mount, "--workdir", workdir}, o.dev(), "bash", "-c", suite)
}

// workdir is where runSuite mounts the checkout inside the container, the
// working directory the development image sets.
const workdir = "/wand"

// suite is what the development image runs: wand's whole suite through the
// ci-test wrapper, then the Zero leftover check in the same PID namespace,
// so that a browser the suite left behind is found while it is still there.
// Examples reach the public internet until #52 lands, hence -run=^Test, as
// in every Tier 1 job.
const suite = `go run ./internal/tools/ci-test -race -count=1 -run=^Test ./...
status=$?
go run ./internal/tools/zero-leftover || status=1
exit $status
`

// chromeVersion is the version a browser reports for --version, the last
// field of a line such as "Google Chrome for Testing 153.0.8010.12".
func chromeVersion(out string) string {
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return ""
	}

	return fields[len(fields)-1]
}
