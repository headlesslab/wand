package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ysmood/got"
)

var setup = got.Setup(nil)

// day is the day the tests cut their releases on, which is what {{date}}
// becomes.
var day = time.Date(2026, time.September, 8, 11, 30, 0, 0, time.UTC)

func TestVersionGrammar(t *testing.T) {
	g := setup(t)

	for _, version := range []string{"v0.1.0", "v0.1.0-rc.1", "v1.20.3", "v0.0.1-rc.12"} {
		r, err := parse(version)
		g.Desc("%s", version).E(err)
		g.Eq(r.String(), version)
	}

	// Everything else is not a release wand makes: no prefix, a leading zero,
	// a candidate numbered from 0, and every other pre-release semantic
	// versioning would allow.
	for _, version := range []string{
		"", "0.1.0", "v0.1", "v0.1.0.1", "v00.1.0", "v0.01.0", "v0.1.0-rc.0", "v0.1.0-rc",
		"v0.1.0-rc.1.2", "v0.1.0-beta.1", "v0.1.0+build.1", "V0.1.0", " v0.1.0",
	} {
		_, err := parse(version)
		g.Desc("%q", version).Err(err)
	}

	g.Has(errorOf(g, "v0.1.0-beta.1"), "is not a version this workflow cuts")

	// A number no int holds is a version this grammar shapes but Go cannot
	// count with.
	_, err := parse("v99999999999999999999.0.0")
	g.Err(err)
}

// errorOf is the message parse gives a version it refuses.
func errorOf(g got.G, version string) string {
	_, err := parse(version)
	g.Err(err)

	return err.Error()
}

func TestPrecedence(t *testing.T) {
	g := setup(t)

	// In precedence order, lowest first: a candidate comes before the release
	// it leads to, and every field before the one after it.
	ordered := []string{
		"v0.1.0-rc.1", "v0.1.0-rc.2", "v0.1.0-rc.10", "v0.1.0",
		"v0.1.1-rc.1", "v0.1.1", "v0.2.0", "v1.0.0",
	}

	for i, low := range ordered {
		lower, err := parse(low)
		g.E(err)

		for j, high := range ordered {
			higher, err := parse(high)
			g.E(err)
			g.Desc("%s above %s", high, low).Eq(higher.above(lower), j > i)
		}
	}
}

func TestTheReleaseFollowsTheLastOne(t *testing.T) {
	g := setup(t)

	published := []string{"v0.1.0-rc.1", "v0.1.0-rc.2", "v0.1.0", "snapshot", "v0.2"}

	// The tags a release is measured against are the ones this workflow cut;
	// anything else in the repository is not a version and is passed over.
	previous, err := after(release{minor: 2}, published)
	g.E(err)
	g.Eq(previous, "v0.1.0")

	// The first release of a repository with no tag at all follows nothing.
	previous, err = after(release{minor: 1, rc: 1}, nil)
	g.E(err)
	g.Eq(previous, "")

	// Tags are never moved (ADR-0008), so a version at or below the last one
	// stops the release here rather than at the tag GitHub would refuse.
	_, err = after(release{minor: 1}, published)
	g.Err(err)
	g.Eq(err.Error(), "v0.1.0 is not above the last release v0.1.0")

	_, err = after(release{minor: 1, rc: 3}, published)
	g.Err(err)
	g.Has(err.Error(), "v0.1.0-rc.3 is not above")
}

func TestThePinsComeFromThePrinter(t *testing.T) {
	g := setup(t)

	dir := g.Testable.(*testing.T).TempDir()
	path := filepath.Join(dir, "pins.json")

	g.E(os.WriteFile(path, []byte(`{"chrome":"153.0.8010.12","protocol":1671234,"chromium":1670000}`), 0o600))

	p, err := readPins(path)
	g.E(err)
	g.Eq(p, pins{Chrome: "153.0.8010.12", Protocol: 1671234, Chromium: 1670000})

	_, err = readPins(filepath.Join(dir, "missing.json"))
	g.Err(err)

	// A file that is not the printer's answer, and one that is JSON but holds
	// none of the three pins, are both a release cut on a pin that is not
	// there.
	g.E(os.WriteFile(path, []byte("Chrome 153.0.8010.12"), 0o600))
	_, err = readPins(path)
	g.Err(err)

	g.E(os.WriteFile(path, []byte(`{"chrome":"153.0.8010.12","protocol":1671234}`), 0o600))
	_, err = readPins(path)
	g.Err(err)
	g.Has(err.Error(), "holds no three pins")
}

func TestVersionsFileGrowsByOneRow(t *testing.T) {
	g := setup(t)

	first := rowOf("v0.1.0-rc.1", pins{Chrome: "153.0.8010.12", Protocol: 1671234, Chromium: 1670000})

	// The bytes are the ones prettier writes, since the generate Gate runs
	// prettier over the commit the release makes.
	out, err := appendRow([]byte("[]\n"), first)
	g.E(err)
	g.Eq(string(out), `[
  {
    "version": "v0.1.0-rc.1",
    "chrome": "153.0.8010.12",
    "protocol": 1671234,
    "chromium": 1670000
  }
]
`)

	second := rowOf("v0.1.0", pins{Chrome: "153.0.8010.12", Protocol: 1671234, Chromium: 1670000})
	grown, err := appendRow(out, second)
	g.E(err)

	rows := []row{}
	g.E(json.Unmarshal(grown, &rows))
	g.Len(rows, 2)
	g.Eq(rows[0], first)
	g.Eq(rows[1], second)

	// A release already in the file is one this workflow has cut before.
	_, err = appendRow(grown, second)
	g.Err(err)
	g.Has(err.Error(), "already holds v0.1.0")

	_, err = appendRow([]byte("{}"), first)
	g.Err(err)
}

func TestPreamblePlaceholders(t *testing.T) {
	g := setup(t)

	out, err := substitute("release candidate {{rc}} of {{date}}\n", 2, "2026-09-08")
	g.E(err)
	g.Eq(out, "release candidate 2 of 2026-09-08\n")

	// A misspelled or unsupported placeholder reaches the Release page as it
	// stands, so it fails the release instead.
	_, err = substitute("wand {{version}}", 1, "2026-09-08")
	g.Err(err)
	g.Has(err.Error(), "{{version}}")

	// {{rc}} in a release with no candidate to name is the same mistake.
	_, err = substitute("promoted from rc.{{rc}}", 0, "2026-09-08")
	g.Err(err)
	g.Has(err.Error(), "no candidate to name")

	// Prose with no placeholder at all is carried over untouched.
	out, err = substitute("wand v0.1.0 is here.\n", 0, "2026-09-08")
	g.E(err)
	g.Eq(out, "wand v0.1.0 is here.\n")
}

func TestTheCandidateAPreambleNames(t *testing.T) {
	g := setup(t)

	published := []string{"v0.1.0-rc.1", "v0.1.0-rc.2", "v0.2.0-rc.1", "main"}

	// A candidate names itself, whatever else is published.
	g.Eq(candidate(release{minor: 1, rc: 1}, published), 1)

	// The release it is promoted to names the last candidate of its own
	// version, which is the one it promotes.
	g.Eq(candidate(release{minor: 1}, published), 2)

	// A release with no candidate before it has none to name.
	g.Eq(candidate(release{minor: 3}, published), 0)
}

func TestTheBodyStartsWithThePins(t *testing.T) {
	g := setup(t)

	p := pins{Chrome: "153.0.8010.12", Protocol: 1671234, Chromium: 1670000}

	// The first line is the three pins; GitHub appends its label-grouped
	// pull-request list to whatever body the release is created with.
	g.Eq(head(p, ""), "Target Chrome `153.0.8010.12`, Protocol roll `r1671234`, Companion Chromium `1670000`.\n")

	g.Eq(head(p, "\n\nwand is here.\n\n"),
		"Target Chrome `153.0.8010.12`, Protocol roll `r1671234`, Companion Chromium `1670000`.\n"+
			"\nwand is here.\n")
}

func TestCutARelease(t *testing.T) {
	g := setup(t)

	dir := checkout(g)
	opts := options{version: "v0.1.0-rc.1", pins: filepath.Join(dir, "pins.json"), body: filepath.Join(dir, "body.md")}

	out, err := cut(dir, opts, []string{"v0.0.1"}, day)
	g.E(err)
	g.Eq(out, `{"tag":"v0.1.0-rc.1","prerelease":true,"previous":"v0.0.1","preamble":true}`)

	// The row is appended to the file, and the body is the pins line and the
	// preamble with its placeholders filled in.
	g.Has(read(g, filepath.Join(dir, "versions.json")), `"version": "v0.1.0-rc.1"`)
	g.Eq(read(g, opts.body),
		"Target Chrome `153.0.8010.12`, Protocol roll `r1671234`, Companion Chromium `1670000`.\n"+
			"\nwand v0.1.0, release candidate 1, cut on 2026-09-08.\n")
}

func TestCutAReleaseWithNoPreamble(t *testing.T) {
	g := setup(t)

	dir := checkout(g)
	opts := options{version: "v0.2.0", pins: filepath.Join(dir, "pins.json"), body: filepath.Join(dir, "body.md")}

	// A Milestone release normally has nothing to say beyond its pins and its
	// pull requests, so it carries no preamble file and the body is the pins
	// line alone.
	out, err := cut(dir, opts, []string{"v0.1.0-rc.1"}, day)
	g.E(err)
	g.Eq(out, `{"tag":"v0.2.0","prerelease":false,"previous":"v0.1.0-rc.1","preamble":false}`)
	g.Eq(read(g, opts.body), "Target Chrome `153.0.8010.12`, Protocol roll `r1671234`, Companion Chromium `1670000`.\n")
}

func TestAReleaseTheWorkflowMustNotCut(t *testing.T) {
	g := setup(t)

	dir := checkout(g)
	opts := options{version: "v0.1.0-rc.1", pins: filepath.Join(dir, "pins.json"), body: filepath.Join(dir, "body.md")}

	// Each of these leaves versions.json as it was: the version is not one
	// this workflow cuts, it is not above the last release, the pins file is
	// not there, and the flags naming the two files are missing.
	for _, bad := range []options{
		{version: "v0.1", pins: opts.pins, body: opts.body},
		{version: "v0.0.1", pins: opts.pins, body: opts.body},
		{version: opts.version, pins: filepath.Join(dir, "gone.json"), body: opts.body},
		{version: opts.version, pins: opts.pins},
	} {
		_, err := cut(dir, bad, []string{"v0.0.1"}, day)
		g.Desc("%v", bad).Err(err)
		g.Eq(read(g, filepath.Join(dir, "versions.json")), "[]\n")
	}

	// A preamble whose placeholders cannot be filled in is a release that
	// stops here too, before anything is written.
	g.E(os.WriteFile(filepath.Join(dir, "docs", "releases", "v0.3.0.md"), []byte("{{rc}}\n"), 0o600))
	_, err := cut(dir, options{version: "v0.3.0", pins: opts.pins, body: opts.body}, nil, day)
	g.Err(err)
	g.Has(err.Error(), filepath.Join("docs", "releases", "v0.3.0.md"))
	g.Eq(read(g, filepath.Join(dir, "versions.json")), "[]\n")
}

// checkout is a tree holding what the tool reads: the versions file seeded as
// an empty array, the pins the printer would have written and one Release
// preamble, of v0.1.0-rc.1.
func checkout(g got.G) string {
	dir := g.Testable.(*testing.T).TempDir()

	g.E(os.MkdirAll(filepath.Join(dir, "docs", "releases"), 0o700))
	g.E(os.WriteFile(filepath.Join(dir, "versions.json"), []byte("[]\n"), 0o600))
	g.E(os.WriteFile(filepath.Join(dir, "pins.json"),
		[]byte(`{"chrome":"153.0.8010.12","protocol":1671234,"chromium":1670000}`), 0o600))
	g.E(os.WriteFile(filepath.Join(dir, "docs", "releases", "v0.1.0-rc.1.md"),
		[]byte("wand v0.1.0, release candidate {{rc}}, cut on {{date}}.\n"), 0o600))

	return dir
}

func read(g got.G, path string) string {
	raw, err := os.ReadFile(path)
	g.E(err)

	return string(raw)
}
