// Package main applies the headlesslab repository settings bundle to one or
// more GitHub repositories through `gh api` and reports what it changed, so a
// new repository needs one command and drift is one re-run away:
//
//	go run ./internal/tools/repo-settings [-app <slug>] [-check <context>]... [-dry-run] <owner/repo>...
//
// The bundle is: secret scanning with push protection, Dependabot alerts and
// security updates, private vulnerability reporting, immutable releases, the
// Actions policy requiring full-length SHA pins, the "main" ruleset (the given
// status checks required, no direct or force pushes, admins and the App as
// bypass actors) and the "v*" tag ruleset (creation, update and deletion
// restricted to the bypass actors). Every setting is read before it is
// written, so a run on a repository that already matches reports no changes.
// With -dry-run nothing is written and the exit status is 1 when anything
// would change.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

func main() {
	opts, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}

	bin, err := exec.LookPath("gh")
	if err != nil {
		fail(errors.New("gh not found on PATH: install the GitHub CLI and run `gh auth login`"))
	}
	c := &client{api: ghAPI{bin: bin}}

	appID := 0
	if opts.app != "" {
		appID, err = lookupApp(c, opts.app)
		if err != nil {
			fail(err)
		}
	}

	changes, err := run(c, opts.repos, bundle(opts.checks, appID), opts.dryRun, os.Stdout)
	if err != nil {
		fail(err)
	}
	if opts.dryRun && changes > 0 {
		os.Exit(1)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "repo-settings:", err)
	os.Exit(1)
}

const usage = `usage: go run ./internal/tools/repo-settings [-app <slug>] [-check <context>]... [-dry-run] <owner/repo>...

  -app <slug>       GitHub App made a bypass actor of the main and v* rulesets
  -check <context>  status check required on main; repeat for each check
  -dry-run          report what would change, write nothing, exit 1 on drift`

type options struct {
	repos  []string
	checks []string
	app    string
	dryRun bool
}

func parseArgs(args []string) (options, error) {
	var opts options
	fs := flag.NewFlagSet("repo-settings", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Var((*list)(&opts.checks), "check", "status check required on main; repeat for each check")
	fs.StringVar(&opts.app, "app", "", "GitHub App slug made a bypass actor of the rulesets")
	fs.BoolVar(&opts.dryRun, "dry-run", false, "report what would change without writing")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}

	opts.repos = fs.Args()
	if len(opts.repos) == 0 {
		return options{}, errors.New("at least one repository is required")
	}
	for _, repo := range opts.repos {
		if strings.Count(repo, "/") != 1 || strings.HasPrefix(repo, "/") || strings.HasSuffix(repo, "/") {
			return options{}, fmt.Errorf("repository %q is not of the form owner/name", repo)
		}
	}
	return opts, nil
}

// list is a repeatable string flag.
type list []string

func (l *list) String() string { return strings.Join(*l, ", ") }

func (l *list) Set(v string) error {
	*l = append(*l, v)
	return nil
}

// lookupApp resolves a GitHub App slug to the id rulesets name it by.
func lookupApp(c *client, slug string) (int, error) {
	var app struct {
		ID int `json:"id"`
	}
	if err := c.do(methodGet, "apps/"+slug, nil, &app); err != nil {
		return 0, fmt.Errorf("looking up GitHub App %q: %w", slug, err)
	}
	if app.ID == 0 {
		return 0, fmt.Errorf("looking up GitHub App %q: no id in the response", slug)
	}
	return app.ID, nil
}

// run applies every setting to every repository, printing one line per
// setting and a count per repository, and returns the number of settings that
// changed (or, in a dry run, would change). Each write is read back, so a
// setting the API silently ignored is an error rather than a change reported
// on every run.
func run(c *client, repos []string, settings []setting, dryRun bool, out io.Writer) (int, error) {
	r := report{out}
	total := 0
	for _, repo := range repos {
		r.printf("%s\n", repo)
		changes := 0
		for _, s := range settings {
			changed, err := apply(c, repo, s, dryRun, r)
			if err != nil {
				return total, fmt.Errorf("%s: %s: %w", repo, s.name, err)
			}
			if changed {
				changes++
			}
		}
		total += changes
		r.printf("%s: %s\n\n", repo, count(changes, dryRun))
	}

	noun := "repositories"
	if len(repos) == 1 {
		noun = "repository"
	}
	suffix := ""
	if dryRun {
		suffix = " (dry run)"
	}
	r.printf("%d %s, %s%s\n", len(repos), noun, count(total, dryRun), suffix)
	return total, nil
}

func apply(c *client, repo string, s setting, dryRun bool, r report) (bool, error) {
	current, ok, err := s.read(c, repo)
	if err != nil {
		return false, err
	}
	if ok {
		r.printf("  %-32s %s\n", s.name, current)
		return false, nil
	}
	if dryRun {
		r.printf("  %-32s %s -> %s (dry run)\n", s.name, current, s.want)
		return true, nil
	}

	if err := s.write(c, repo); err != nil {
		return false, err
	}
	after, ok, err := s.read(c, repo)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, fmt.Errorf("still %q after applying %q", after, s.want)
	}
	r.printf("  %-32s %s -> %s\n", s.name, current, after)
	return true, nil
}

// report is the run's output; a failed write to it is not worth failing the run for.
type report struct {
	w io.Writer
}

func (r report) printf(format string, args ...any) {
	_, _ = fmt.Fprintf(r.w, format, args...)
}

func count(n int, dryRun bool) string {
	switch {
	case n == 0:
		return "no changes"
	case dryRun:
		return fmt.Sprintf("%d changes pending", n)
	case n == 1:
		return "1 change"
	}
	return fmt.Sprintf("%d changes", n)
}
