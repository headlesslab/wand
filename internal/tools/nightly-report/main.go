// Package main is the Nightly's issue opener: it reads the jobs of one
// workflow run and, for every job that went red, opens an issue keyed by that
// job's name or comments on the open one (spec #33, section 13; ticket #61):
//
//	go run ./internal/tools/nightly-report -repo headlesslab/wand -run 42
//
// A Nightly gates nothing, so nobody watches it; the issue is how a red job
// reaches a maintainer. Keying it by the job name means the linux/arm64 suite
// failing four nights running is one issue with four comments rather than four
// issues, and that a second job going red is its own issue rather than a
// comment nobody reads.
//
// Two tokens, because they can do different things: the run's own GITHUB_TOKEN
// reads the jobs, which is an Actions permission the automation App does not
// have, and the App's token opens the issues, so that they come from the
// automation identity rather than from github-actions[bot] (#58). They arrive
// as GH_TOKEN and APP_TOKEN, and neither is ever printed.
//
// The tool exits 0 when it has reported, whatever it found: the red belongs to
// the jobs it reports on, and a red reporter would say only that reporting
// failed. It exits 1 when it could not report at all.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// label is what a new issue carries, so that it lands in triage like any other
// issue nobody has read yet (docs/agents/triage-labels.md).
const label = "needs-triage"

// runner runs one gh command with one token and gives back its stdout. It is
// a function so that the tests can watch what the tool would run without a gh
// or a network anywhere near them.
type runner func(token string, args ...string) ([]byte, error)

// options are the run to report on and where to report it.
type options struct {
	repo   string
	run    string
	server string
	read   string
	write  string
	dryRun bool
}

func main() {
	opts := options{
		server: env("GITHUB_SERVER_URL", "https://github.com"),
		read:   os.Getenv("GH_TOKEN"),
		write:  os.Getenv("APP_TOKEN"),
	}
	flag.StringVar(&opts.repo, "repo", os.Getenv("GITHUB_REPOSITORY"), "the repository the run belongs to, as owner/name")
	flag.StringVar(&opts.run, "run", os.Getenv("GITHUB_RUN_ID"), "the workflow run to report on")
	flag.BoolVar(&opts.dryRun, "dry-run", false, "print the issues that would be opened or commented on, and open nothing")
	flag.Parse()

	if err := report(gh, opts, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "nightly-report:", err)
		os.Exit(1)
	}
}

func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}

	return fallback
}

// gh runs the gh command line with one token, which reaches it as GH_TOKEN
// and reaches nothing else: the inherited GH_TOKEN and GITHUB_TOKEN are
// dropped rather than shadowed, so a call always uses the token it was given.
func gh(token string, args ...string) ([]byte, error) {
	cmd := exec.Command("gh", args...)
	cmd.Stderr = os.Stderr

	cmd.Env = []string{"GH_TOKEN=" + token}
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if name != "GH_TOKEN" && name != "GITHUB_TOKEN" {
			cmd.Env = append(cmd.Env, kv)
		}
	}

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gh %s: %w", strings.Join(args, " "), err)
	}

	return out, nil
}

// job is one job of a workflow run, as the Actions API gives it.
type job struct {
	Name       string `json:"name"`
	Conclusion string `json:"conclusion"`
	URL        string `json:"html_url"`
	Steps      []struct {
		Name       string `json:"name"`
		Conclusion string `json:"conclusion"`
	} `json:"steps"`
}

// isRed reports whether a job's conclusion is one a maintainer has to answer
// for. A job that was cancelled or skipped is not: cancelling is somebody
// already watching, and a skip is the workflow's own decision, such as the
// image scan before the first release exists.
func (j job) isRed() bool {
	return j.Conclusion == "failure" || j.Conclusion == "timed_out"
}

// failedSteps are the steps of a red job that went red themselves, which is
// the one line of a Nightly log worth carrying into the issue.
func (j job) failedSteps() []string {
	var names []string
	for _, s := range j.Steps {
		if s.Conclusion == "failure" || s.Conclusion == "timed_out" {
			names = append(names, s.Name)
		}
	}

	return names
}

// report is the whole tool: the red jobs of the run, each turned into an issue
// or a comment on the issue it already has.
func report(run runner, opts options, out io.Writer) error {
	if opts.repo == "" || opts.run == "" {
		return errors.New("both -repo and -run are needed")
	}

	jobs, err := runJobs(run, opts)
	if err != nil {
		return err
	}

	var red []job
	for _, j := range jobs {
		if j.isRed() {
			red = append(red, j)
		}
	}

	if len(red) == 0 {
		_, _ = fmt.Fprintf(out, "all %d jobs of run %s were green: nothing to report\n", len(jobs), opts.run)

		return nil
	}

	// One job's issue failing must not swallow the next job's, so every red
	// job is reported and the errors are answered with together.
	var errs []error
	for _, j := range red {
		if err := raise(run, opts, j, out); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// runJobs are the jobs of the run, in the order the Actions API lists them, read with the
// run's own token: the App has no Actions permission (#58).
func runJobs(run runner, opts options) ([]job, error) {
	// --jq gives one job object per line, across every page, so nothing here
	// depends on how gh joins the pages of an object response.
	out, err := run(opts.read, "api", "--paginate", "--jq", ".jobs[]",
		fmt.Sprintf("repos/%s/actions/runs/%s/jobs?per_page=100", opts.repo, opts.run))
	if err != nil {
		return nil, err
	}

	var jobs []job
	decoder := json.NewDecoder(bytes.NewReader(out))
	for {
		var j job
		if err := decoder.Decode(&j); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			return nil, fmt.Errorf("the jobs of run %s: %w", opts.run, err)
		}
		jobs = append(jobs, j)
	}

	if len(jobs) == 0 {
		return nil, fmt.Errorf("run %s of %s has no jobs, so it is not the run to report on", opts.run, opts.repo)
	}

	return jobs, nil
}

// raise opens the issue of one red job, or comments on the one already open.
func raise(run runner, opts options, j job, out io.Writer) error {
	title := issueTitle(j)

	number, err := openIssue(run, opts, title)
	if err != nil {
		return err
	}

	body := issueBody(opts, j)

	if opts.dryRun {
		what := "would open an issue"
		if number != "" {
			what = "would comment on issue #" + number
		}
		_, _ = fmt.Fprintf(out, "%s: %s\n\n%s\n\n", what, title, body)

		return nil
	}

	if number != "" {
		if _, err := run(opts.write, "issue", "comment", number, "--repo", opts.repo, "--body", body); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "commented on issue #%s: %s\n", number, title)

		return nil
	}

	if _, err := run(opts.write, "issue", "create", "--repo", opts.repo,
		"--title", title, "--label", label, "--body", body); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "opened an issue: %s\n", title)

	return nil
}

// issueTitle is the key: one issue per job name, so that the same job failing on
// four nights is one issue with four comments.
func issueTitle(j job) string {
	return fmt.Sprintf("Nightly: %s failed", j.Name)
}

// openIssue is the number of the open issue with that exact title, or the empty
// string when there is none. The search is a filter GitHub applies loosely, so
// the exact title is what decides, not what the search answered.
func openIssue(run runner, opts options, title string) (string, error) {
	out, err := run(opts.write, "issue", "list", "--repo", opts.repo, "--state", "open",
		"--search", `in:title "`+title+`"`, "--json", "number,title")
	if err != nil {
		return "", err
	}

	var found []struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
	}
	if err := json.Unmarshal(out, &found); err != nil {
		return "", fmt.Errorf("the open issues of %s: %w", opts.repo, err)
	}

	for _, issue := range found {
		if issue.Title == title {
			return fmt.Sprint(issue.Number), nil
		}
	}

	return "", nil
}

// issueBody says which run and which job, which steps went red, and what the issue
// is for, so that a maintainer reading it a week later needs nothing else to
// start.
func issueBody(opts options, j job) string {
	run := fmt.Sprintf("%s/%s/actions/runs/%s", opts.server, opts.repo, opts.run)

	steps := "the job went red before any step did"
	if failed := j.failedSteps(); len(failed) > 0 {
		steps = "`" + strings.Join(failed, "`, `") + "`"
	}

	where := j.URL
	if where == "" {
		where = run
	}

	return strings.Join([]string{
		fmt.Sprintf("The Nightly job **%s** went red: %s", j.Name, where),
		fmt.Sprintf("Run: %s", run),
		fmt.Sprintf("Red steps: %s", steps),
		"A Nightly blocks no merge and no release (CONTEXT.md, Nightly), so this issue is the whole alarm. It is keyed by the job's name: the same job failing again comments here rather than opening a second issue, and a different job gets one of its own. Close it once the job is green again, and split anything it turns out to be into its own issue.",
	}, "\n\n")
}
