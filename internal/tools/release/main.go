// Package main is the version arithmetic and the release body of the release
// workflow: it validates the version it is given against the tags already
// published, appends that release's row to versions.json, and writes the head
// of the GitHub Release body — the three pins on the first line, and the
// Release preamble of docs/releases/<tag>.md under it when the release has one
// (spec #33, section 15; ADR-0008; ticket #60):
//
//	go run ./internal/tools/release -version v0.1.0-rc.1 -pins pins.json -body body.md
//
// The pins arrive as the JSON the print-pins tool writes, so nothing here
// parses Go source, and GitHub appends its own label-grouped pull-request list
// to the body this writes. It prints one JSON object describing the release
// and changes nothing outside versions.json and the body file; every error is
// a release the workflow must not cut.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	// versionsFile maps every release to the three pins it carries (ADR-0008),
	// and releasesDir holds the optional Release preamble of a release, one
	// file per tag.
	versionsFile = "versions.json"
	releasesDir  = "docs/releases"
)

// options are the flags, which name the release and where the two files the
// workflow reads back are.
type options struct {
	version string
	pins    string
	body    string
}

func main() {
	opts := options{}
	flag.StringVar(&opts.version, "version", "", "the release to cut: vX.Y.Z, or vX.Y.Z-rc.N for a release candidate")
	flag.StringVar(&opts.pins, "pins", "", "the file holding the JSON of `go run ./internal/tools/print-pins -json`")
	flag.StringVar(&opts.body, "body", "", "where to write the head of the GitHub Release body")
	flag.Parse()

	tags, err := publishedTags()
	if err != nil {
		fail(err)
	}

	out, err := cut(".", opts, tags, time.Now().UTC())
	if err != nil {
		fail(err)
	}

	fmt.Println(out)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "release:", err)
	os.Exit(1)
}

// publishedTags is every tag of the checkout that looks like a version, which
// is what a release is validated against. The workflow checks out the whole
// history for it.
func publishedTags() ([]string, error) {
	out, err := exec.Command("git", "tag", "--list", "v*").Output()
	if err != nil {
		return nil, fmt.Errorf("git tag: %w", err)
	}

	return strings.Fields(string(out)), nil
}

// cut is the whole tool: it validates version against the tags already
// published, appends the release's row to the versions file under dir and
// writes the head of the release body, and answers with the JSON the workflow
// reads its decisions from.
func cut(dir string, opts options, tags []string, now time.Time) (string, error) {
	if opts.pins == "" || opts.body == "" {
		return "", errors.New("-pins and -body are required")
	}

	rel, err := parse(opts.version)
	if err != nil {
		return "", err
	}

	previous, err := after(rel, tags)
	if err != nil {
		return "", err
	}

	p, err := readPins(opts.pins)
	if err != nil {
		return "", err
	}

	preamble, err := readPreamble(dir, rel, tags, now)
	if err != nil {
		return "", err
	}

	// The body first, since it is written outside the checkout: a failure
	// here then leaves no row behind in versions.json for the next run to
	// commit.
	if err := os.WriteFile(opts.body, []byte(head(p, preamble)), 0o600); err != nil {
		return "", err
	}

	if err := writeVersions(dir, rowOf(rel.String(), p)); err != nil {
		return "", err
	}

	out, err := json.Marshal(summary{
		Tag:        rel.String(),
		Prerelease: rel.rc > 0,
		Previous:   previous,
		Preamble:   preamble != "",
	})

	return string(out), err
}

// summary is what the workflow reads back: the tag to create, whether GitHub
// marks it a pre-release, the release it follows (empty for the first one) and
// whether a Release preamble went into the body.
type summary struct {
	Tag        string `json:"tag"`
	Prerelease bool   `json:"prerelease"`
	Previous   string `json:"previous"`
	Preamble   bool   `json:"preamble"`
}

// release is a version this workflow cuts: vX.Y.Z, or vX.Y.Z-rc.N for a
// release candidate of it.
type release struct {
	major, minor, patch int
	rc                  int // 0 when the version is not a release candidate
}

// versionPattern is the whole grammar of a wand version (ADR-0008): three
// numbers with no leading zero, and a release candidate numbered from 1. Any
// other pre-release or build metadata semantic versioning allows is not a
// release wand makes.
var versionPattern = regexp.MustCompile(`^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-rc\.([1-9]\d*))?$`)

func parse(version string) (release, error) {
	m := versionPattern.FindStringSubmatch(version)
	if m == nil {
		return release{}, fmt.Errorf(
			"%q is not a version this workflow cuts: vX.Y.Z, or vX.Y.Z-rc.N with the candidate numbered from 1",
			version)
	}

	n := [4]int{}
	for i, field := range m[1:] {
		if field == "" {
			continue
		}

		v, err := strconv.Atoi(field)
		if err != nil {
			return release{}, fmt.Errorf("%q: %w", version, err)
		}
		n[i] = v
	}

	return release{major: n[0], minor: n[1], patch: n[2], rc: n[3]}, nil
}

func (r release) String() string {
	if r.rc > 0 {
		return fmt.Sprintf("v%d.%d.%d-rc.%d", r.major, r.minor, r.patch, r.rc)
	}

	return fmt.Sprintf("v%d.%d.%d", r.major, r.minor, r.patch)
}

// order is the release as the four numbers precedence compares in turn. The
// last one makes a release candidate come before the release it leads to:
// rc.1 before rc.2 before v0.1.0 itself.
func (r release) order() [4]int {
	last := r.rc
	if last == 0 {
		last = math.MaxInt
	}

	return [4]int{r.major, r.minor, r.patch, last}
}

// above reports whether r comes after other in semantic version precedence.
func (r release) above(other release) bool {
	a, b := r.order(), other.order()
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}

	return false
}

// after checks the release against the tags already published and answers with
// the one it follows, empty when it is the first. Tags are never moved
// (ADR-0008), so a release that is not above every published one is a mistake
// the workflow stops at rather than a tag it fails to create later.
func after(rel release, tags []string) (string, error) {
	var top release
	found := false

	for _, tag := range tags {
		published, err := parse(tag)
		if err != nil {
			continue // a tag this workflow did not cut
		}

		if !found || published.above(top) {
			top, found = published, true
		}
	}

	switch {
	case !found:
		return "", nil
	case rel.above(top):
		return top.String(), nil
	default:
		return "", fmt.Errorf("%s is not above the last release %s", rel, top)
	}
}

// pins are the three the release carries, as print-pins writes them.
type pins struct {
	Chrome   string `json:"chrome"`
	Protocol int    `json:"protocol"`
	Chromium int    `json:"chromium"`
}

func readPins(path string) (pins, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return pins{}, err
	}

	p := pins{}
	if err := json.Unmarshal(raw, &p); err != nil {
		return pins{}, fmt.Errorf("%s: %w", path, err)
	}

	if p.Chrome == "" || p.Protocol == 0 || p.Chromium == 0 {
		return pins{}, fmt.Errorf("%s holds no three pins: %s", path, strings.TrimSpace(string(raw)))
	}

	return p, nil
}

// row is one entry of versions.json: a release and the three pins it carried,
// the version first, as the file reads. Not the pins struct embedded, which
// would have to be declared above Version and would write every row with its
// version last.
type row struct {
	Version  string `json:"version"`
	Chrome   string `json:"chrome"`
	Protocol int    `json:"protocol"`
	Chromium int    `json:"chromium"`
}

// rowOf is the release's own row, out of the pins the printer gave.
func rowOf(version string, p pins) row {
	return row{Version: version, Chrome: p.Chrome, Protocol: p.Protocol, Chromium: p.Chromium}
}

func writeVersions(dir string, r row) error {
	path := filepath.Join(dir, versionsFile)

	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	out, err := appendRow(raw, r)
	if err != nil {
		return err
	}

	return os.WriteFile(path, out, 0o600)
}

// appendRow adds the release to the versions file. The bytes it writes are the
// ones prettier writes — two spaces of indent, one trailing newline — because
// the generate Gate runs prettier over the commit this release makes.
func appendRow(raw []byte, r row) ([]byte, error) {
	rows := []row{}
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, fmt.Errorf("%s: %w", versionsFile, err)
	}

	for _, published := range rows {
		if published.Version == r.Version {
			return nil, fmt.Errorf("%s already holds %s", versionsFile, r.Version)
		}
	}

	out, err := json.MarshalIndent(append(rows, r), "", "  ")
	if err != nil {
		return nil, err
	}

	return append(out, '\n'), nil
}

// readPreamble is docs/releases/<tag>.md with its placeholders filled in, and
// the empty string when the release has none, which is what a Milestone
// release with nothing to say beyond its pins and its pull requests looks
// like (ADR-0008).
func readPreamble(dir string, rel release, tags []string, now time.Time) (string, error) {
	path := filepath.Join(dir, releasesDir, rel.String()+".md")

	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	} else if err != nil {
		return "", err
	}

	text, err := substitute(string(raw), candidate(rel, tags), now.Format(time.DateOnly))
	if err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}

	return text, nil
}

// candidate is the release candidate number a preamble's {{rc}} names: the
// release's own, and for a release that is not a candidate the last candidate
// published of that version, which is the one it promotes (ADR-0008). It is 0
// when there is none, which makes {{rc}} an error rather than a blank.
func candidate(rel release, tags []string) int {
	if rel.rc > 0 {
		return rel.rc
	}

	last := 0
	for _, tag := range tags {
		published, err := parse(tag)
		if err != nil {
			continue
		}

		same := published.major == rel.major && published.minor == rel.minor && published.patch == rel.patch
		if same && published.rc > last {
			last = published.rc
		}
	}

	return last
}

// placeholder is anything in a preamble that looks like one, so that a
// misspelled or unsupported name fails the release rather than reaching the
// Release page as it stands.
var placeholder = regexp.MustCompile(`{{[^{}]*}}`)

// substitute fills in the two placeholders a Release preamble may carry:
// {{rc}}, the release candidate number, and {{date}}, the day the release is
// cut.
func substitute(text string, rc int, date string) (string, error) {
	unknown := []string{}

	out := placeholder.ReplaceAllStringFunc(text, func(name string) string {
		switch {
		case name == "{{date}}":
			return date
		case name == "{{rc}}" && rc > 0:
			return strconv.Itoa(rc)
		case name == "{{rc}}":
			unknown = append(unknown, name+" (this release has no candidate to name)")
		default:
			unknown = append(unknown, name)
		}

		return name
	})

	if len(unknown) > 0 {
		return "", fmt.Errorf("the Release preamble holds %s; only {{rc}} and {{date}} are filled in",
			strings.Join(unknown, ", "))
	}

	return out, nil
}

// head is the body a release is created with: the three pins on the first
// line, and the Release preamble under it when the release has one. GitHub
// appends its label-grouped pull-request list to it, which is why this ends
// with a newline and nothing else.
func head(p pins, preamble string) string {
	line := fmt.Sprintf("Target Chrome `%s`, Protocol roll `r%d`, Companion Chromium `%d`.\n",
		p.Chrome, p.Protocol, p.Chromium)

	if strings.TrimSpace(preamble) == "" {
		return line
	}

	return line + "\n" + strings.TrimSpace(preamble) + "\n"
}
