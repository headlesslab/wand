package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/ysmood/got"
)

var setup = got.Setup(nil)

// opts is a run of the repository, with two tokens that are told apart by
// their names alone.
var opts = options{
	repo:   "headlesslab/wand",
	run:    "42",
	server: "https://github.com",
	read:   "the run's token",
	write:  "the App's token",
}

// call is one gh command the tool ran, with the token it ran under.
type call struct {
	token string
	args  []string
}

// what a call is about: "api", "issue list", "issue create" or "issue
// comment", which is what a test answers and asserts on.
func (c call) what() string {
	if c.args[0] != "issue" {
		return c.args[0]
	}

	return strings.Join(c.args[:2], " ")
}

// fake is a gh that answers from a table and remembers what it was asked.
type fake struct {
	answers map[string]string
	fails   map[string]error
	calls   []call
}

func (f *fake) run(token string, args ...string) ([]byte, error) {
	c := call{token: token, args: args}
	f.calls = append(f.calls, c)

	if err := f.fails[c.what()]; err != nil {
		return nil, err
	}

	return []byte(f.answers[c.what()]), nil
}

// only is the single call of one kind, which the test asserts there is exactly
// one of.
func (f *fake) only(g got.G, what string) call {
	var found []call
	for _, c := range f.calls {
		if c.what() == what {
			found = append(found, c)
		}
	}

	g.Desc("one %q call", what).Len(found, 1)

	return found[0]
}

// count of the calls of one kind.
func (f *fake) count(what string) int {
	n := 0
	for _, c := range f.calls {
		if c.what() == what {
			n++
		}
	}

	return n
}

// jobsOf is what the Actions API gives through gh's --jq: one job object per
// line.
func jobsOf(lines ...string) string {
	return strings.Join(lines, "\n") + "\n"
}

const (
	redSuite = `{"name":"Tier 1 linux/arm64 (Go stable)","conclusion":"failure",` +
		`"html_url":"https://github.com/headlesslab/wand/actions/runs/42/job/7",` +
		`"steps":[{"name":"go build","conclusion":"success"},{"name":"The suite","conclusion":"failure"},` +
		`{"name":"Zero leftover","conclusion":"failure"}]}`

	redExamples = `{"name":"Examples","conclusion":"failure",` +
		`"html_url":"https://github.com/headlesslab/wand/actions/runs/42/job/8",` +
		`"steps":[{"name":"The examples","conclusion":"failure"}]}`

	greenGovulncheck = `{"name":"govulncheck on main","conclusion":"success",` +
		`"html_url":"https://github.com/headlesslab/wand/actions/runs/42/job/9",` +
		`"steps":[{"name":"govulncheck","conclusion":"success"}]}`

	skippedTrivy = `{"name":"Trivy on the published image","conclusion":"skipped",` +
		`"html_url":"https://github.com/headlesslab/wand/actions/runs/42/job/10","steps":[]}`
)

func TestGreenRunOpensNothing(t *testing.T) {
	g := setup(t)

	// A skipped job is green here: the image scan skips itself cleanly until
	// the first release exists, and a cancelled job is somebody watching.
	f := &fake{answers: map[string]string{"api": jobsOf(greenGovulncheck, skippedTrivy,
		`{"name":"Support window","conclusion":"cancelled","steps":[]}`)}}

	out := &strings.Builder{}
	g.E(report(f.run, opts, out))

	g.Has(out.String(), "all 3 jobs of run 42 were green")
	g.Eq(f.count("issue list"), 0)
	g.Eq(f.count("issue create"), 0)
	g.Eq(f.count("issue comment"), 0)
}

func TestARedJobOpensItsIssue(t *testing.T) {
	g := setup(t)

	f := &fake{answers: map[string]string{
		"api":        jobsOf(redSuite, greenGovulncheck),
		"issue list": "[]",
	}}

	out := &strings.Builder{}
	g.E(report(f.run, opts, out))

	created := f.only(g, "issue create")
	g.Has(strings.Join(created.args, " "), "--title Nightly: Tier 1 linux/arm64 (Go stable) failed")
	g.Has(strings.Join(created.args, " "), "--label "+label)

	// The body carries the job, its own URL, the run and the steps that went
	// red, so that the issue is readable without opening anything.
	body := created.args[len(created.args)-1]
	g.Has(body, "Tier 1 linux/arm64 (Go stable)")
	g.Has(body, "https://github.com/headlesslab/wand/actions/runs/42/job/7")
	g.Has(body, "https://github.com/headlesslab/wand/actions/runs/42")
	g.Has(body, "`The suite`, `Zero leftover`")

	g.Has(out.String(), "opened an issue: Nightly: Tier 1 linux/arm64 (Go stable) failed")
}

func TestTheSecondNightCommentsOnTheOpenIssue(t *testing.T) {
	g := setup(t)

	f := &fake{answers: map[string]string{
		"api":        jobsOf(redSuite),
		"issue list": `[{"number":123,"title":"Nightly: Tier 1 linux/arm64 (Go stable) failed"}]`,
	}}

	out := &strings.Builder{}
	g.E(report(f.run, opts, out))

	g.Eq(f.count("issue create"), 0)
	commented := f.only(g, "issue comment")
	g.Eq(commented.args[2], "123")
	g.Has(out.String(), "commented on issue #123")
}

func TestOnlyTheExactTitleIsTheSameIssue(t *testing.T) {
	g := setup(t)

	// GitHub's search matches loosely, so an issue about the same job under
	// another title, and the same title on another job, are both somebody
	// else's issue: this job's is opened.
	f := &fake{answers: map[string]string{
		"api": jobsOf(redSuite),
		"issue list": `[{"number":7,"title":"Tier 1 linux/arm64 (Go stable) failed"},` +
			`{"number":8,"title":"Nightly: Tier 1 linux/amd64 (Go stable) failed"}]`,
	}}

	g.E(report(f.run, opts, &strings.Builder{}))

	g.Eq(f.count("issue comment"), 0)
	g.Eq(f.count("issue create"), 1)
}

func TestOneIssuePerJob(t *testing.T) {
	g := setup(t)

	f := &fake{answers: map[string]string{
		"api":        jobsOf(redSuite, redExamples, greenGovulncheck),
		"issue list": "[]",
	}}

	g.E(report(f.run, opts, &strings.Builder{}))

	g.Eq(f.count("issue create"), 2)

	var titles []string
	for _, c := range f.calls {
		if c.what() == "issue create" {
			titles = append(titles, c.args[5])
		}
	}
	g.Eq(titles, []string{
		"Nightly: Tier 1 linux/arm64 (Go stable) failed",
		"Nightly: Examples failed",
	})
}

func TestATimedOutJobIsRed(t *testing.T) {
	g := setup(t)

	f := &fake{answers: map[string]string{
		"api":        jobsOf(`{"name":"Support window","conclusion":"timed_out","steps":[]}`),
		"issue list": "[]",
	}}

	g.E(report(f.run, opts, &strings.Builder{}))

	created := f.only(g, "issue create")
	body := created.args[len(created.args)-1]

	// No step of its own went red, and the job has no URL of its own either,
	// so the body says so and points at the run.
	g.Has(body, "the job went red before any step did")
	g.Has(body, "https://github.com/headlesslab/wand/actions/runs/42")
}

func TestTheJobsAndTheIssuesUseDifferentTokens(t *testing.T) {
	g := setup(t)

	f := &fake{answers: map[string]string{
		"api":        jobsOf(redSuite),
		"issue list": "[]",
	}}

	g.E(report(f.run, opts, &strings.Builder{}))

	// The Actions API with the run's own token, since the App has no Actions
	// permission; every issue with the App's, so that the issue comes from
	// the automation identity (#58).
	g.Eq(f.only(g, "api").token, opts.read)
	g.Eq(f.only(g, "issue list").token, opts.write)
	g.Eq(f.only(g, "issue create").token, opts.write)
}

func TestADryRunOpensNothing(t *testing.T) {
	g := setup(t)

	dry := opts
	dry.dryRun = true

	f := &fake{answers: map[string]string{
		"api":        jobsOf(redSuite),
		"issue list": "[]",
	}}

	out := &strings.Builder{}
	g.E(report(f.run, dry, out))

	g.Eq(f.count("issue create"), 0)
	g.Eq(f.count("issue comment"), 0)
	g.Has(out.String(), "would open an issue: Nightly: Tier 1 linux/arm64 (Go stable) failed")
	g.Has(out.String(), "`The suite`, `Zero leftover`")
}

func TestReportRefusals(t *testing.T) {
	g := setup(t)

	// Neither the repository nor the run may be guessed.
	for _, o := range []options{{run: "42"}, {repo: "headlesslab/wand"}} {
		g.Err(report((&fake{}).run, o, &strings.Builder{}))
	}

	// A run with no job at all is not the run to report on, whatever the
	// reason: a run number nobody made, a token that may not read it.
	g.Err(report((&fake{answers: map[string]string{"api": ""}}).run, opts, &strings.Builder{}))

	// Something that is not the JSON the Actions API documents.
	g.Err(report((&fake{answers: map[string]string{"api": "<html>rate limited</html>"}}).run, opts, &strings.Builder{}))

	// gh itself failing, on each of the three calls in turn.
	for _, what := range []string{"api", "issue list", "issue create"} {
		f := &fake{
			answers: map[string]string{"api": jobsOf(redSuite), "issue list": "[]"},
			fails:   map[string]error{what: errors.New("gh: " + what)},
		}
		g.Desc("%s fails", what).Err(report(f.run, opts, &strings.Builder{}))
	}

	// An issue list that is not a list.
	f := &fake{answers: map[string]string{"api": jobsOf(redSuite), "issue list": "not json"}}
	g.Err(report(f.run, opts, &strings.Builder{}))
}

func TestOneJobFailingToReportLeavesTheNextReported(t *testing.T) {
	g := setup(t)

	// The App's token is rejected for the first issue; the second job must
	// still be tried, and the run must still end in an error.
	f := &fake{
		answers: map[string]string{"api": jobsOf(redSuite, redExamples), "issue list": "[]"},
		fails:   map[string]error{"issue create": errors.New("gh: 403")},
	}

	err := report(f.run, opts, &strings.Builder{})
	g.Err(err)
	g.Eq(f.count("issue create"), 2)
	g.Eq(strings.Count(err.Error(), "gh: 403"), 2)
}
