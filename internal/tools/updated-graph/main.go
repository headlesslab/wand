// Package main moves every module in the graph as far forward as the Go floor
// allows, and says which ones could not come the whole way, which is what the
// Nightly's updated-graph job runs before it runs the suite (spec #33, section
// 13; ADR-0006; ticket #61):
//
//	go run ./internal/tools/updated-graph
//
// `go get -u ./...` is what a user runs and what this stands in for, but it
// takes the newest version there is, and with GOTOOLCHAIN=local a module that
// has raised its own Go floor above wand's then fails the command outright.
// One such module — golang.org/x/sys has wanted Go 1.25 since v0.42.0 — would
// stop the job before a single test ran, so the question the job exists to ask
// (does wand's own code survive its dependencies moving?) would go unanswered
// for as long as wand's floor stayed where it is. ADR-0006 asks for the suite
// on the updated graph, so the update stops at the floor rather than at the
// first module that has outrun it.
//
// A module held back is not a failure. It is printed, annotated and put in the
// job summary, because what to do about it — take it and raise the floor, or
// stay — is a decision, not a bump (docs/maintainer-notes.md, "The Nightly").
// This exits non-zero only when the go command itself refuses something.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// commander runs one go command and gives back its stdout. A function so that
// the tests can watch what would run with no go command and no network.
type commander func(args ...string) ([]byte, error)

func main() {
	out := os.Stdout

	changes, err := plan(command, out)
	if err != nil {
		fail(err)
	}

	if err := apply(command, changes, out); err != nil {
		fail(err)
	}

	if err := report(changes, out, os.Getenv("GITHUB_STEP_SUMMARY")); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "updated-graph:", err)
	os.Exit(1)
}

// command runs the go command, with its stderr going to this process's, since
// what it says there is the whole diagnosis when it refuses.
func command(args ...string) ([]byte, error) {
	cmd := exec.Command("go", args...)
	cmd.Stderr = os.Stderr

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go %s: %w", strings.Join(args, " "), err)
	}

	return out, nil
}

// module is one entry of `go list -m -json`, of which this reads the version it
// is at, the newest version there is, and, for the main module, the Go floor.
type module struct {
	Path      string `json:"Path"`
	Version   string `json:"Version"`
	Main      bool   `json:"Main"`
	GoVersion string `json:"GoVersion"`
	GoMod     string `json:"GoMod"`
	Update    *struct {
		Version string `json:"Version"`
	} `json:"Update"`
}

// change is one module's move: to the newest version there is, or to the
// newest the floor can take, or nowhere at all.
type change struct {
	path string

	// from is where the module is now, to where it can go (empty when nothing
	// newer fits) and newest the newest version there is.
	from   string
	to     string
	newest string

	// wants is the Go version the newest asks for, when that is what held the
	// module back; empty when the module went the whole way.
	wants string
}

// held reports whether the module could not reach the newest version there is.
func (c change) held() bool {
	return c.wants != ""
}

// plan is every move the graph can make: the modules with a newer version,
// each taken as far as the floor allows.
func plan(run commander, out io.Writer) ([]change, error) {
	modules, err := list(run, "-m", "-u", "-json", "all")
	if err != nil {
		return nil, err
	}

	var floor string
	for _, m := range modules {
		if m.Main {
			floor = m.GoVersion

			break
		}
	}
	if floor == "" {
		return nil, errors.New("no main module in the graph, so there is no Go floor to hold the update to")
	}

	_, _ = fmt.Fprintf(out, "the Go floor is %s\n", floor)

	var changes []change
	for _, m := range modules {
		if m.Main || m.Update == nil {
			continue
		}

		c, err := furthest(run, m, floor)
		if err != nil {
			return nil, err
		}
		changes = append(changes, c)
	}

	return changes, nil
}

// furthest is how far one module can move: the newest version whose own go
// directive the floor can build, looked for from the newest down, so that a
// module whose directive does not only ever rise is still answered exactly.
func furthest(run commander, m module, floor string) (change, error) {
	c := change{path: m.Path, from: m.Version, newest: m.Update.Version}

	versions, err := above(run, m.Path, m.Version)
	if err != nil {
		return change{}, err
	}

	for i := len(versions) - 1; i >= 0; i-- {
		wants, err := directive(run, m.Path, versions[i])
		if err != nil {
			return change{}, err
		}

		if fits(wants, floor) {
			c.to = versions[i]

			return c, nil
		}

		// The newest is what the report names, and it is the one whose Go
		// version says why nothing above here moved.
		if versions[i] == c.newest {
			c.wants = wants
		}
	}

	return c, nil
}

// list runs a `go list -m -json` of some shape and decodes the objects it
// writes, which are concatenated rather than in an array.
func list(run commander, args ...string) ([]module, error) {
	out, err := run(append([]string{"list"}, args...)...)
	if err != nil {
		return nil, err
	}

	var modules []module
	decoder := json.NewDecoder(strings.NewReader(string(out)))
	for {
		var m module
		if err := decoder.Decode(&m); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			return nil, fmt.Errorf("go list -m: %w", err)
		}
		modules = append(modules, m)
	}

	return modules, nil
}

// above is the tagged versions of a module newer than the one in the graph, in
// the order the go command gives them, which is oldest first.
func above(run commander, path, version string) ([]string, error) {
	out, err := run("list", "-m", "-versions", path)
	if err != nil {
		return nil, err
	}

	// The line is the module path and then its versions.
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return nil, fmt.Errorf("go list -m -versions %s answered nothing", path)
	}

	var newer []string
	for _, v := range fields[1:] {
		if v != version {
			newer = append(newer, v)

			continue
		}

		// Everything before the version in the graph is older than it.
		newer = nil
	}

	return newer, nil
}

// directive is the Go version one version of a module asks for, read from the
// go.mod the module cache holds. `go list -m -json path@version` fetches that
// file and the version's metadata, and nothing else: no zip and no extraction,
// so a walk down a module's versions costs a request each.
//
// A go.mod with no go line is older than the directive itself, which the go
// command reads as 1.16, and every floor wand could have is above that.
func directive(run commander, path, version string) (string, error) {
	out, err := run("list", "-m", "-json", path+"@"+version)
	if err != nil {
		return "", err
	}

	var m module
	if err := json.Unmarshal(out, &m); err != nil {
		return "", fmt.Errorf("go list -m -json %s@%s: %w", path, version, err)
	}

	file, err := os.ReadFile(m.GoMod)
	if err != nil {
		return "", fmt.Errorf("the go.mod of %s@%s: %w", path, version, err)
	}

	for _, line := range strings.Split(string(file), "\n") {
		if rest, has := strings.CutPrefix(strings.TrimSpace(line), "go "); has {
			return strings.TrimSpace(rest), nil
		}
	}

	return "1.16", nil
}

// fits reports whether a toolchain at floor can build a module that asks for
// wants. Both are dotted numbers, and a part nobody wrote is a zero.
func fits(wants, floor string) bool {
	want, have := parts(wants), parts(floor)
	for i := 0; i < len(want) || i < len(have); i++ {
		w, h := at(want, i), at(have, i)
		if w != h {
			return w < h
		}
	}

	return true
}

func parts(version string) []int {
	var out []int
	for _, field := range strings.Split(version, ".") {
		n, err := strconv.Atoi(field)
		if err != nil {
			break
		}
		out = append(out, n)
	}

	return out
}

func at(version []int, i int) int {
	if i < len(version) {
		return version[i]
	}

	return 0
}

// apply moves the modules that can move, in one go get, so that the version
// arithmetic happens once over the whole set rather than once per module.
func apply(run commander, changes []change, out io.Writer) error {
	args := []string{"get"}
	for _, c := range changes {
		if c.to != "" {
			args = append(args, c.path+"@"+c.to)
		}
	}

	if len(args) == 1 {
		_, _ = fmt.Fprintln(out, "nothing in the graph has a newer version the Go floor can take")

		return nil
	}

	_, err := run(args...)

	return err
}

// report says what moved and what did not, on stdout, as annotations the run's
// own page carries, and in the job summary, so that a module held back is read
// without a red job to make somebody read it.
func report(changes []change, out io.Writer, summaryFile string) error {
	var summary strings.Builder
	summary.WriteString("### The graph at its newest, as far as the Go floor allows\n\n")
	summary.WriteString("| module | was | now | newest | held back by |\n| --- | --- | --- | --- | --- |\n")

	for _, c := range changes {
		now, held := c.to, "—"
		if now == "" {
			now = c.from
		}
		if c.held() {
			held = "`go " + c.wants + "`"

			_, _ = fmt.Fprintf(out, "held back: %s stays at %s; %s is the newest the floor can take, and %s wants go %s\n",
				c.path, now, now, c.newest, c.wants)
			_, _ = fmt.Fprintf(out, "::notice::%s could not go past %s: its %s wants go %s, above this module's own floor\n",
				c.path, now, c.newest, c.wants)
		} else {
			_, _ = fmt.Fprintf(out, "moved: %s %s -> %s\n", c.path, c.from, c.to)
		}

		_, _ = fmt.Fprintf(&summary, "| `%s` | %s | %s | %s | %s |\n", c.path, c.from, now, c.newest, held)
	}

	if summaryFile == "" {
		return nil
	}

	file, err := os.OpenFile(summaryFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	_, err = file.WriteString(summary.String())

	return err
}
