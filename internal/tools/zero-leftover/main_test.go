package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ysmood/got"
)

var setup = got.Setup(nil)

// helperEnv marks the copy of this test binary that TestReal runs as a
// process named like a browser.
const helperEnv = "WAND_ZERO_LEFTOVER_HELPER"

// snapshots is a lister that answers each call with the next snapshot and
// repeats the last one for ever.
func snapshots(list ...[]process) func(context.Context) ([]process, error) {
	i := 0
	return func(context.Context) ([]process, error) {
		if i < len(list)-1 {
			i++
			return list[i-1], nil
		}
		return list[len(list)-1], nil
	}
}

// quick is the options of a run that polls fast and gives up soon, on a
// prefix that does not exist.
func quick(list func(context.Context) ([]process, error)) options {
	return options{
		wait:   300 * time.Millisecond,
		poll:   10 * time.Millisecond,
		list:   list,
		prefix: filepath.Join(os.TempDir(), "wand-zero-leftover-none"),
	}
}

func TestClean(t *testing.T) {
	g := setup(t)

	out := &bytes.Buffer{}
	code := run(g.Context(), out, quick(snapshots([]process{{1, "go"}, {2, "zero-leftover"}})))

	g.Eq(code, 0)
	g.Has(out.String(), "zero-leftover: no browser process, nothing under ")
}

func TestProcessGone(t *testing.T) {
	g := setup(t)

	chrome := []process{{7, "chrome"}, {8, "chrome_crashpad_handler"}}
	out := &bytes.Buffer{}
	start := time.Now()
	code := run(g.Context(), out, quick(snapshots(chrome, chrome, chrome, []process{})))

	g.Eq(code, 0)
	g.Has(out.String(), "no browser process")
	// Three polls before the browser was gone: the run waited, and no longer
	// than the wait.
	g.Gte(time.Since(start), 30*time.Millisecond)
	g.Lt(time.Since(start), 300*time.Millisecond)
}

func TestProcessLeft(t *testing.T) {
	g := setup(t)

	out := &bytes.Buffer{}
	start := time.Now()
	code := run(g.Context(), out, quick(snapshots([]process{{42, "chrome"}, {1, "go"}})))

	g.Eq(code, 1)
	g.Gte(time.Since(start), 300*time.Millisecond)
	g.Has(out.String(), "zero-leftover: 1 browser process and 0 directories left after 300ms:\n")
	g.Has(out.String(), "  pid 42 chrome\n")
	g.False(strings.Contains(out.String(), "pid 1 go"))
}

func TestDirLeft(t *testing.T) {
	g := setup(t)

	opts := quick(snapshots([]process{}))
	opts.prefix = t.TempDir()
	dir := filepath.Join(opts.prefix, "abc123")
	g.E(os.Mkdir(dir, 0o755))
	file := filepath.Join(opts.prefix, "stray")
	g.E(os.WriteFile(file, nil, 0o644))

	out := &bytes.Buffer{}
	code := run(g.Context(), out, opts)

	g.Eq(code, 1)
	g.Has(out.String(), "0 browser processes and 2 directories left")
	g.Has(out.String(), "  "+dir+"\n")
	g.Has(out.String(), "  "+file+"\n")
}

func TestDirRemoved(t *testing.T) {
	g := setup(t)

	opts := quick(snapshots([]process{}))
	opts.prefix = t.TempDir()
	dir := filepath.Join(opts.prefix, "abc123")
	g.E(os.Mkdir(dir, 0o755))
	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = os.Remove(dir)
	}()

	out := &bytes.Buffer{}
	code := run(g.Context(), out, opts)

	g.Eq(code, 0)
	g.Has(out.String(), "nothing under "+opts.prefix+"\n")
}

// TestPrefixUnreadable: a prefix that cannot be read, as opposed to one that
// does not exist, fails the run. A NUL byte makes a path every OS refuses.
func TestPrefixUnreadable(t *testing.T) {
	g := setup(t)

	opts := quick(snapshots([]process{}))
	opts.prefix = filepath.Join(t.TempDir(), "nul\x00byte")

	out := &bytes.Buffer{}
	code := run(g.Context(), out, opts)

	g.Eq(code, 1)
	g.Has(out.String(), "zero-leftover: reading "+opts.prefix+": ")
}

func TestListError(t *testing.T) {
	g := setup(t)

	out := &bytes.Buffer{}
	code := run(g.Context(), out, quick(func(context.Context) ([]process, error) {
		return nil, errors.New("ps: exit status 1")
	}))

	g.Eq(code, 1)
	g.Eq(out.String(), "zero-leftover: listing processes: ps: exit status 1\n")
}

func TestBrowsers(t *testing.T) {
	g := setup(t)

	processes := []process{
		{1, "chrome"},
		{2, "chrome_crashpad_handler"},
		{3, "chrome.exe"},
		{4, "/Users/runner/.cache/wand/browser/chrome-1/Google Chrome for Testing.app/Contents/MacOS/Google Chrome for Testing"},
		{5, "Google Chrome for Testing Helper (Renderer)"},
		{6, "Chromium"},
		{7, "chromium-browser"},
		{8, "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"},
		{9, "go"},
		{10, "wand.test"},
		{11, "zero-leftover"},
		{12, "ps"},
		{13, "msedge.exe"},
		{14, "GoogleUpdate.exe"},
	}

	g.Eq(browsers(processes), processes[:8])
	g.Eq(browsers(nil), []process{})
}

func TestReport(t *testing.T) {
	g := setup(t)

	out := &bytes.Buffer{}
	report(out, options{wait: 30 * time.Second}, []process{{1, "chrome"}}, []string{"/tmp/wand/user-data/a"})

	g.Eq(out.String(), "zero-leftover: 1 browser process and 1 directory left after 30s:\n  pid 1 chrome\n  /tmp/wand/user-data/a\n")
}

// TestReal runs a copy of this test binary under a name that makes it a
// browser to the tool, and holds the real lister to it: listed while it
// runs, reported by run, gone once it has exited.
func TestReal(t *testing.T) {
	g := setup(t)

	self, err := os.Executable()
	g.E(err)
	bin := filepath.Join(t.TempDir(), "chrome-fixture"+filepath.Ext(self))
	data, err := os.ReadFile(self)
	g.E(err)
	g.E(os.WriteFile(bin, data, 0o755))

	helper := exec.CommandContext(g.Context(), bin, "-test.run=^TestHelperProcess$")
	helper.Env = append(os.Environ(), helperEnv+"=1")
	stdin, err := helper.StdinPipe()
	g.E(err)
	g.E(helper.Start())
	pid := helper.Process.Pid

	g.True(listed(g, pid))

	// A developer machine may have a browser of its own open, so the run
	// is judged on the helper only.
	out := &bytes.Buffer{}
	opts := quick(listProcesses)
	opts.wait = 0
	g.Eq(run(g.Context(), out, opts), 1)
	g.Has(out.String(), fmt.Sprintf("  pid %d ", pid))
	g.Has(out.String(), "chrome-fixture")

	g.E(stdin.Close())
	g.E(helper.Wait())

	// The process table can lag the exit by a moment.
	for i := 0; i < 100 && listed(g, pid); i++ {
		time.Sleep(50 * time.Millisecond)
	}
	g.False(listed(g, pid))
}

// listed reports whether the real lister shows pid as a browser.
func listed(g got.G, pid int) bool {
	processes, err := listProcesses(g.Context())
	g.E(err)
	for _, p := range browsers(processes) {
		if p.pid == pid {
			return true
		}
	}
	return false
}

// TestHelperProcess is the body of the helper TestReal runs: it lives until
// its standard input ends. As a test of its own it does nothing.
func TestHelperProcess(*testing.T) {
	if os.Getenv(helperEnv) != "1" {
		return
	}
	_, _ = io.Copy(io.Discard, os.Stdin)
	os.Exit(0)
}
