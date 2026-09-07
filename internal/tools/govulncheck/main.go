// Package main is the vulnerability scan of the Gate's linux/amd64 Go stable
// job, and the same command to run before pushing:
//
//	go run ./internal/tools/govulncheck [<govulncheck arguments>]
//
// It runs govulncheck at the version internal/devutil pins, in source mode,
// over ./... when given no argument of its own: every package of the module,
// stopping at the two nested example modules. A vulnerable function of a
// dependency or of the standard library that wand's code can reach fails the
// scan; one nothing reaches is reported and does not (spec #33, section 16;
// ticket #56).
//
// x/vuln declares a Go floor well above wand's, so with the GOTOOLCHAIN=local
// every Gate job sets, only the Go stable job can build it: hence one scan,
// on that job. The scan reads the Go vulnerability database, so it needs the
// network, and a fresh advisory against unchanged code reds it on the next
// run, which is the point.
//
// The exit status is govulncheck's own, so the step's log ends with the
// report that failed it rather than with a Go stack trace.
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/headlesslab/wand/internal/devutil"
)

func main() {
	targets := os.Args[1:]
	if len(targets) == 0 {
		targets = []string{"./..."}
	}

	fmt.Println("govulncheck:", devutil.Govulncheck)

	cmd := exec.Command("go", append([]string{"run", devutil.Govulncheck}, targets...)...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr

	err := cmd.Run()
	if err == nil {
		return
	}

	var exit *exec.ExitError
	if errors.As(err, &exit) {
		os.Exit(exit.ExitCode())
	}

	fmt.Fprintln(os.Stderr, "govulncheck:", err)
	os.Exit(1)
}
