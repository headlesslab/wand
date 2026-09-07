// Package main is wand's image build script: it builds the two container
// images from the checkout it is run in, proves what the image Gate proves,
// and either keeps them on this machine or pushes them by digest for the
// release workflow to publish (spec #33, section 14; tickets #55 and #60).
//
//	go run ./internal/tools/docker [-suite] [-push <file>]
//
// It writes the .dockerignore, builds the runtime image and then the :dev
// image on top of the one it has just built, runs the manager and Chrome out
// of the runtime image and holds the version Chrome reports to the Target
// Chrome pin, and with -suite runs wand's whole suite and the Zero leftover
// check inside the :dev image against this checkout. Each image is built for
// the machine's own platform, as the image jobs build theirs. Run it from the
// module root, as go generate runs the generators.
//
// With -push it builds the same two images with buildx and pushes them under
// no tag at all, writing the two manifest digests to the file named. A tag of
// the release is a manifest of both architectures, which the release workflow
// makes from the digests of its two image jobs once both have pushed; this
// tool creates no tag and logs in to no registry, which the workflow does
// before it runs this. Pushing needs a builder that can export an image, so
// the workflow makes a docker-container one first.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

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
	push := flag.String("push", "",
		"push both images by digest instead of keeping them here, writing the two digests as JSON to this file")
	flag.Parse()

	dir, err := os.Getwd()
	utils.E(err)

	utils.E(devutil.DockerIgnore(dir))

	if *push != "" {
		// The suite runs against a checkout mounted into an image on this
		// machine, and -push keeps none: asking for both is asking for two
		// different runs.
		if *suite {
			utils.E(errors.New("-suite runs the suite in an image built here, which -push does not keep"))
		}

		publish(*push)

		return
	}

	devutil.ExecLine(true, "", buildRuntime()...)
	devutil.ExecLine(true, "", buildDev()...)

	prove(image)

	if *suite {
		devutil.ExecLine(true, "", runSuite(dir)...)
	}
}

// prove runs the manager and the browser out of the runtime image at ref. The
// manager is the image's entrypoint, so a manager that cannot start is an
// image that cannot serve; -h exits 0 and launches nothing. The Target Chrome
// is a pin and not a lookup, which is what makes the linux/amd64 and the
// linux/arm64 image carry the same version: each image job holds its own to
// the same pin.
func prove(ref string) {
	devutil.ExecLine(true, "", run(nil, ref, "wand-manager", "-h")...)

	out := devutil.ExecLine(true, "", run(nil, ref, "chrome", "--version")...)
	if got := reportedVersion(out); got != pins.ChromeVersion {
		utils.E(fmt.Errorf("the browser in %s is Chrome %q, not the Target Chrome pin %q",
			ref, got, pins.ChromeVersion))
	}
}

// publish builds both images for this runner's platform, pushes each under no
// tag, proves the runtime one by pulling back the digest that was pushed, and
// writes the two digests to the file the release workflow reads them from.
func publish(out string) {
	dir := filepath.Dir(out)
	utils.E(os.MkdirAll(dir, 0o755))

	runtimeMeta := filepath.Join(dir, "metadata-runtime.json")
	devMeta := filepath.Join(dir, "metadata-dev.json")

	devutil.ExecLine(true, "", pushRuntime(runtimeMeta)...)
	runtimeDigest, err := digest(runtimeMeta)
	utils.E(err)

	// The development image is built on the runtime image just pushed, by the
	// digest that push gave: the builder that exports to a registry holds no
	// image of its own for a tag to name.
	devutil.ExecLine(true, "", pushDev(image+"@"+runtimeDigest, devMeta)...)
	devDigest, err := digest(devMeta)
	utils.E(err)

	prove(image + "@" + runtimeDigest)

	body, err := json.Marshal(digests{Runtime: runtimeDigest, Dev: devDigest})
	utils.E(err)
	utils.E(os.WriteFile(out, append(body, '\n'), 0o600))

	fmt.Println(string(body))
}

// digests are the two manifests one architecture's image job pushed, which the
// release workflow gathers from both jobs into one manifest per tag.
type digests struct {
	Runtime string `json:"runtime"`
	Dev     string `json:"dev"`
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

// pushRuntime builds the runtime image and pushes it by digest.
func pushRuntime(metadata string) []string {
	args := []string{"docker", "buildx", "build", "--file", "docker/Dockerfile"}

	return append(append(args, byDigest(metadata)...), ".")
}

// pushDev builds the development image on the runtime image at base and
// pushes it by digest.
func pushDev(base, metadata string) []string {
	args := []string{
		"docker", "buildx", "build", "--file", "docker/dev.Dockerfile",
		"--build-arg", "base=" + base,
	}

	return append(append(args, byDigest(metadata)...), ".")
}

// byDigest is the buildx output that pushes an image under no tag: the
// registry holds it by its digest alone, which is what the release workflow
// gathers into one manifest per tag once both architectures have pushed. The
// metadata file is where buildx writes the digest it pushed.
func byDigest(metadata string) []string {
	return []string{
		"--output", "type=image,name=" + image + ",push-by-digest=true,name-canonical=true,push=true",
		"--metadata-file", metadata,
	}
}

// digest is the manifest digest buildx recorded in its metadata file.
func digest(metadata string) (string, error) {
	raw, err := os.ReadFile(metadata)
	if err != nil {
		return "", err
	}

	m := struct {
		Digest string `json:"containerimage.digest"`
	}{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return "", fmt.Errorf("%s: %w", metadata, err)
	}

	if !strings.HasPrefix(m.Digest, "sha256:") {
		return "", fmt.Errorf("%s names no image digest: %s", metadata, strings.TrimSpace(string(raw)))
	}

	return m.Digest, nil
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
