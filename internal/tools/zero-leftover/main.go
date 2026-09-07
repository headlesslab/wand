// Package main is the Zero leftover step every Tier 1 job ends with: it
// waits up to 30 s for the browsers a run launched to be gone, then fails on
// any chrome or chromium process still running and on anything still under
// launcher.DefaultUserDataDirPrefix, the directory the launcher makes its
// temporary user data directories in (spec #33, section 12; ticket #54).
//
//	go run ./internal/tools/zero-leftover [-wait 30s]
//
// The wait covers a browser still exiting when the suite's process has
// ended: Launcher.Cleanup gives a browser ten seconds before killing it, and
// a crash handler may outlive its browser by a moment. What is left after
// the wait is listed on stderr, process by process and directory by
// directory, so the log names the leftover rather than a bare failure.
// Nothing is killed or removed: a leftover on a hosted runner is a bug in a
// test or in the launcher, and the red step is what shows it. Any browser
// on the machine counts, a developer's own included: the tool is written for
// a runner that is the job's alone.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/headlesslab/wand/lib/launcher"
)

func main() {
	wait := flag.Duration("wait", 30*time.Second, "how long to wait for the last browser to be gone")
	flag.Parse()

	os.Exit(run(context.Background(), os.Stdout, os.Stderr, options{
		wait:   *wait,
		poll:   time.Second,
		list:   listProcesses,
		prefix: launcher.DefaultUserDataDirPrefix,
	}))
}

// options is what a run works with; the tests give a lister and a prefix of
// their own.
type options struct {
	wait   time.Duration
	poll   time.Duration
	list   func(context.Context) ([]process, error)
	prefix string
}

// process is one running process: its id and the name of its executable,
// which macOS reports as a path.
type process struct {
	pid  int
	name string
}

func (p process) String() string {
	return fmt.Sprintf("pid %d %s", p.pid, p.name)
}

// run polls until nothing is left or the wait is over, and says so on out;
// what is wrong goes to errOut. The exit status is 0 when the machine is
// clean and 1 when something is left or cannot be looked at.
func run(ctx context.Context, out, errOut io.Writer, opts options) int {
	deadline := time.Now().Add(opts.wait)
	for {
		processes, dirs, err := leftover(ctx, opts)
		if err != nil {
			_, _ = fmt.Fprintf(errOut, "zero-leftover: %v\n", err)
			return 1
		}
		if len(processes) == 0 && len(dirs) == 0 {
			_, _ = fmt.Fprintf(out, "zero-leftover: no browser process, nothing under %s\n", opts.prefix)
			return 0
		}
		if !time.Now().Before(deadline) {
			report(errOut, opts.wait, processes, dirs)
			return 1
		}
		time.Sleep(opts.poll)
	}
}

// leftover is every browser process running and every entry under the
// prefix, right now.
func leftover(ctx context.Context, opts options) ([]process, []string, error) {
	processes, err := opts.list(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("listing processes: %w", err)
	}

	entries, err := os.ReadDir(opts.prefix)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, nil, fmt.Errorf("reading %s: %w", opts.prefix, err)
	}
	dirs := []string{}
	for _, e := range entries {
		dirs = append(dirs, filepath.Join(opts.prefix, e.Name()))
	}

	return browsers(processes), dirs, nil
}

// browsers keeps the processes whose executable is a browser: chrome or
// chromium, and their helpers, which carry one of the two names as well
// (chrome_crashpad_handler, Google Chrome for Testing Helper (Renderer)).
func browsers(processes []process) []process {
	kept := []process{}
	for _, p := range processes {
		name := strings.ToLower(filepath.Base(p.name))
		if strings.Contains(name, "chrome") || strings.Contains(name, "chromium") {
			kept = append(kept, p)
		}
	}
	return kept
}

// report lists what is left after the wait, one per line.
func report(out io.Writer, wait time.Duration, processes []process, dirs []string) {
	_, _ = fmt.Fprintf(out, "zero-leftover: %s and %s left after %s:\n",
		plural(len(processes), "browser process", "browser processes"),
		plural(len(dirs), "directory", "directories"), wait)
	for _, p := range processes {
		_, _ = fmt.Fprintf(out, "  %s\n", p)
	}
	for _, dir := range dirs {
		_, _ = fmt.Fprintf(out, "  %s\n", dir)
	}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
