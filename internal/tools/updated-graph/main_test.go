package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ysmood/got"
)

var setup = got.Setup(nil)

// fake is a go command that answers from a table and remembers what it was
// asked, so that no test here runs one or reaches a proxy.
type fake struct {
	answers map[string]string
	fails   map[string]error
	calls   []string
}

func (f *fake) run(args ...string) ([]byte, error) {
	key := strings.Join(args, " ")
	f.calls = append(f.calls, key)

	if err := f.fails[key]; err != nil {
		return nil, err
	}

	out, has := f.answers[key]
	if !has {
		return nil, fmt.Errorf("no answer for: go %s", key)
	}

	return []byte(out), nil
}

// graph is the x/sys situation of 2026-09: the module is eight minor versions
// behind, the newest wants a Go far above wand's floor, and the last one the
// floor can take is in between.
func graph(g got.G, current string) *fake {
	dir := g.Testable.(*testing.T).TempDir()

	directives := map[string]string{
		"v0.22.0": "1.18", "v0.29.0": "1.18", "v0.30.0": "1.18",
		"v0.31.0": "1.23.0", "v0.35.0": "1.23.0",
		"v0.42.0": "1.25.0", "v0.47.0": "1.25.0",
	}

	f := &fake{answers: map[string]string{
		"list -m -u -json all": `{"Path":"github.com/headlesslab/wand","Main":true,"GoVersion":"1.21.0"}
			{"Path":"golang.org/x/sys","Version":"` + current + `","Update":{"Version":"v0.47.0"}}`,
		"list -m -versions golang.org/x/sys": "golang.org/x/sys v0.22.0 v0.29.0 v0.30.0 v0.31.0 v0.35.0 v0.42.0 v0.47.0",
	}}

	for version, directive := range directives {
		mod := filepath.Join(dir, version+".mod")
		g.E(os.WriteFile(mod, []byte("module golang.org/x/sys\n\ngo "+directive+"\n"), 0o600))

		key := "list -m -json golang.org/x/sys@" + version
		f.answers[key] = fmt.Sprintf(`{"Path":"golang.org/x/sys","Version":%q,"GoMod":%q}`, version, mod)
	}

	f.answers["get golang.org/x/sys@v0.30.0"] = ""

	return f
}

func TestTheFloorIsWhereTheUpdateStops(t *testing.T) {
	g := setup(t)

	f := graph(g, "v0.22.0")

	changes, err := plan(f.run, &strings.Builder{})
	g.E(err)
	g.Len(changes, 1)

	// v0.30.0 is the last one declaring a go the 1.21.0 floor can build, and
	// v0.47.0 is what held the module back from going further.
	c := changes[0]
	g.Eq(c.path, "golang.org/x/sys")
	g.Eq(c.from, "v0.22.0")
	g.Eq(c.to, "v0.30.0")
	g.Eq(c.newest, "v0.47.0")
	g.Eq(c.wants, "1.25.0")
	g.True(c.held())

	// The walk starts at the newest and stops at the first that fits, so the
	// three versions below v0.30.0 are never asked about.
	for _, version := range []string{"v0.29.0", "v0.22.0"} {
		g.Desc("%s not probed", version).
			False(strings.Contains(strings.Join(f.calls, "\n"), "golang.org/x/sys@"+version))
	}
}

func TestAModuleAlreadyAtTheFloorsCeilingStaysPut(t *testing.T) {
	g := setup(t)

	// This is where the bump of this pull request leaves x/sys: nothing above
	// v0.30.0 declares a go 1.21.0 can build.
	changes, err := plan(graph(g, "v0.30.0").run, &strings.Builder{})
	g.E(err)
	g.Len(changes, 1)
	g.Eq(changes[0].to, "")
	g.Eq(changes[0].wants, "1.25.0")
	g.True(changes[0].held())
}

func TestAModuleThatGoesTheWholeWay(t *testing.T) {
	g := setup(t)

	f := graph(g, "v0.22.0")
	dir := g.Testable.(*testing.T).TempDir()
	mod := filepath.Join(dir, "newest.mod")
	g.E(os.WriteFile(mod, []byte("module golang.org/x/sys\n\ngo 1.21.0\n"), 0o600))
	f.answers["list -m -json golang.org/x/sys@v0.47.0"] = `{"GoMod":"` + filepath.ToSlash(mod) + `"}`

	changes, err := plan(f.run, &strings.Builder{})
	g.E(err)
	g.Eq(changes[0].to, "v0.47.0")
	g.Eq(changes[0].wants, "")
	g.False(changes[0].held())
}

func TestOneGoGetForTheWholeSet(t *testing.T) {
	g := setup(t)

	f := &fake{answers: map[string]string{"get a@v2 c@v4": ""}}
	out := &strings.Builder{}

	// The module that could not move is not named: go get would take it to
	// where it already is, and the argument would say the opposite of what
	// the report says.
	g.E(apply(f.run, []change{
		{path: "a", from: "v1", to: "v2"},
		{path: "b", from: "v3", wants: "1.25.0"},
		{path: "c", from: "v3", to: "v4"},
	}, out))
	g.Eq(f.calls, []string{"get a@v2 c@v4"})

	// Nothing to move is not a go get at all.
	quiet := &fake{}
	out = &strings.Builder{}
	g.E(apply(quiet.run, []change{{path: "b", from: "v3", wants: "1.25.0"}}, out))
	g.Len(quiet.calls, 0)
	g.Has(out.String(), "nothing in the graph has a newer version")
}

func TestTheReportSaysWhatCouldNotCome(t *testing.T) {
	g := setup(t)

	summary := filepath.Join(g.Testable.(*testing.T).TempDir(), "summary.md")
	out := &strings.Builder{}

	g.E(report([]change{
		{path: "golang.org/x/sys", from: "v0.30.0", newest: "v0.47.0", wants: "1.25.0"},
		{path: "github.com/ysmood/got", from: "v0.43.0", to: "v0.44.0", newest: "v0.44.0"},
	}, out, summary))

	// A module held back is said out loud and annotated, so that it is read
	// without a red job to make somebody read it.
	g.Has(out.String(), "held back: golang.org/x/sys stays at v0.30.0")
	g.Has(out.String(), "::notice::golang.org/x/sys could not go past v0.30.0")
	g.Has(out.String(), "moved: github.com/ysmood/got v0.43.0 -> v0.44.0")

	written := g.Read(summary).String()
	g.Has(written, "| `golang.org/x/sys` | v0.30.0 | v0.30.0 | v0.47.0 | `go 1.25.0` |")
	g.Has(written, "| `github.com/ysmood/got` | v0.43.0 | v0.44.0 | v0.44.0 | — |")

	// No summary to write to is what a developer's machine looks like.
	g.E(report(nil, &strings.Builder{}, ""))
}

func TestVersionsAboveTheOneInTheGraph(t *testing.T) {
	g := setup(t)

	f := &fake{answers: map[string]string{
		"list -m -versions m": "m v1 v2 v3 v4",
	}}

	newer, err := above(f.run, "m", "v2")
	g.E(err)
	g.Eq(newer, []string{"v3", "v4"})

	// A version the list does not carry — a pseudo-version, or one retracted
	// since — leaves every tagged version to consider.
	newer, err = above(f.run, "m", "v0.0.0-20220412211240-33da011f77ad")
	g.E(err)
	g.Eq(newer, []string{"v1", "v2", "v3", "v4"})

	// A module with no tagged version at all.
	f.answers["list -m -versions m"] = "m"
	newer, err = above(f.run, "m", "v2")
	g.E(err)
	g.Len(newer, 0)
}

func TestTheGoDirectiveOfAVersion(t *testing.T) {
	g := setup(t)

	dir := g.Testable.(*testing.T).TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		g.E(os.WriteFile(p, []byte(body), 0o600))

		return p
	}

	f := &fake{answers: map[string]string{
		"list -m -json m@v1": `{"GoMod":"` + filepath.ToSlash(write("v1.mod", "module m\n\ngo 1.21.0\n")) + `"}`,
		// A go.mod older than the go directive itself, which the go command
		// reads as 1.16.
		"list -m -json m@v2": `{"GoMod":"` + filepath.ToSlash(write("v2.mod", "module m\n")) + `"}`,
		"list -m -json m@v3": `{"GoMod":"` + filepath.ToSlash(dir) + `/gone.mod"}`,
	}}

	wants, err := directive(f.run, "m", "v1")
	g.E(err)
	g.Eq(wants, "1.21.0")

	wants, err = directive(f.run, "m", "v2")
	g.E(err)
	g.Eq(wants, "1.16")

	// A go.mod the module cache does not hold, and an answer that is not the
	// JSON go list writes.
	_, err = directive(f.run, "m", "v3")
	g.Err(err)

	f.answers["list -m -json m@v4"] = "not json"
	_, err = directive(f.run, "m", "v4")
	g.Err(err)
}

func TestWhatAFloorCanBuild(t *testing.T) {
	g := setup(t)

	for _, c := range []struct {
		wants, floor string
		fits         bool
	}{
		{"1.18", "1.21.0", true},
		{"1.21", "1.21.0", true},
		{"1.21.0", "1.21.0", true},
		{"1.21.1", "1.21.0", false},
		{"1.23.0", "1.21.0", false},
		{"1.25.0", "1.21.0", false},
		{"1.16", "1.21.0", true},
		{"1.9", "1.21.0", true},
		// A directive with something that is not a number in it stops being
		// read there, which leaves the parts before it deciding.
		{"1.21rc1", "1.21.0", true},
	} {
		g.Desc("go %s on a %s floor", c.wants, c.floor).Eq(fits(c.wants, c.floor), c.fits)
	}
}

func TestPlanRefusals(t *testing.T) {
	g := setup(t)

	// No main module means no floor to hold the update to.
	f := &fake{answers: map[string]string{"list -m -u -json all": `{"Path":"m","Version":"v1"}`}}
	_, err := plan(f.run, &strings.Builder{})
	g.Err(err)
	g.Has(err.Error(), "no Go floor")

	// The go command refusing, and answering something that is not its JSON.
	f = &fake{fails: map[string]error{"list -m -u -json all": errors.New("go list: offline")}}
	_, err = plan(f.run, &strings.Builder{})
	g.Err(err)

	f = &fake{answers: map[string]string{"list -m -u -json all": "not json"}}
	_, err = plan(f.run, &strings.Builder{})
	g.Err(err)

	// A module whose versions cannot be listed.
	f = graph(g, "v0.22.0")
	delete(f.answers, "list -m -versions golang.org/x/sys")
	_, err = plan(f.run, &strings.Builder{})
	g.Err(err)
}
