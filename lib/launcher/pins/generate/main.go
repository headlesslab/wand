// Package main is the Roll tool: it computes everything lib/launcher/pins
// holds and rewrites every place the pins appear, so the launcher, the
// protocol generator, the Roll, the release workflow and the READMEs all
// carry the same numbers.
//
//	go run ./lib/launcher/pins/generate [<version>]
//	go run ./lib/launcher/pins/generate -if-newer
//	go run ./lib/launcher/pins/generate -render
//	go run ./lib/launcher/pins/generate -check
//
// With no argument the Target Chrome is Chrome for Testing's last-known-good
// Stable; with a version argument it is that version (a Security roll, or a
// Roll forced by hand). From its branch position the tool derives the Protocol
// roll, the largest devtools-protocol v0.0.<rev> tag not above the position,
// and the Companion Chromium, the newest Chromium trunk build at or below it
// whose archive exists for all five bucket prefixes. It then downloads every
// Managed browser archive from Google's origin bucket, never from another
// Download host, records each SHA-256 and rewrites lib/launcher/pins/pins.go
// and the browser table between the pins markers of README.md and
// README.zh-CN.md. An archive Google does not serve is reported and the exit
// status is 1; what was verified is still written, so the gap shows in the
// diff instead of hiding.
//
// -if-newer is the scheduled Roll's form, and the only one that decides for
// itself whether there is anything to do: it reads the last-known-good Stable
// and rolls to it only when its milestone is above the committed Target
// Chrome's, so a daily schedule costs one request on every day between two
// milestones and downloads the archives on the day there is a Roll. An equal
// milestone is not newer, one Milestone release per Chrome stable milestone
// (ADR-0008), so a move inside a milestone is a version given instead.
//
// -render downloads nothing: it rewrites the same outputs from the pins as
// committed, for when the renderer or a README's prose changes between two
// Rolls.
//
// -check rewrites nothing. It re-derives the Protocol roll from the committed
// branch position and fails on a mismatch, and it re-renders every output from
// the committed values and fails when the bytes differ, so a stale roll and
// any drift in formatting, order or a README table are caught before the next
// Roll's diff carries them. It cannot tell a hand-edited hash from a
// downloaded one: the reviewed Roll pull request is the trust anchor for the
// hashes (ADR-0005). go generate runs it, so the generate Gate's zero-diff
// check covers this package (ADR-0004, ADR-0009).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"regexp"
	"strconv"

	"github.com/headlesslab/wand/lib/launcher/pins"
)

func main() {
	opts, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		usage(os.Stderr)
		os.Exit(2)
	}

	os.Exit(run(opts))
}

// run does the work and returns the exit status, so that main's os.Exit
// happens after the signal handler is released.
func run(opts options) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	r := newRoller()

	switch {
	case opts.check:
		return check(ctx, r)
	case opts.render:
		return rerender(r)
	case opts.ifNewer:
		return rollIfNewer(ctx, r)
	default:
		return roll(ctx, r, opts.version)
	}
}

func check(ctx context.Context, r *roller) int {
	if err := r.check(ctx, current()); err != nil {
		return fail(err)
	}

	fmt.Printf("pins: every output is what the Roll writes; branch position %d derives protocol r%d\n",
		pins.ChromePosition, pins.ProtocolRoll)
	return 0
}

func rerender(r *roller) int {
	if err := r.write(current()); err != nil {
		return fail(err)
	}

	fmt.Printf("pins: rewrote %s for Chrome %s\n", outputNames(), pins.ChromeVersion)
	return 0
}

func roll(ctx context.Context, r *roller, version string) int {
	p, missing, err := r.roll(ctx, version)
	if err != nil {
		return fail(err)
	}
	if err := r.write(p); err != nil {
		return fail(err)
	}

	fmt.Printf("pins: wrote %s for Chrome %s (branch position %d, protocol r%d, Chromium %d)\n",
		outputNames(), p.ChromeVersion, p.ChromePosition, p.ProtocolRoll, p.ChromiumPosition)

	if len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "pins: %d archive(s) missing from Google's bucket:\n", len(missing))
		for _, u := range missing {
			fmt.Fprintln(os.Stderr, "  "+u)
		}
		return 1
	}
	return 0
}

// rollIfNewer is what the scheduled Roll runs: it rolls only once Chrome for
// Testing's Stable has reached a milestone above the Target Chrome's, so that
// a schedule running every day still opens one pull request per Chrome stable
// milestone (ADR-0008) and writes nothing on all the other days. An
// in-milestone move, a Security roll, is a forced version instead.
func rollIfNewer(ctx context.Context, r *roller) int {
	target := pins.ChromeVersion

	version, needed, err := rollNeeded(ctx, r, target)
	if err != nil {
		return fail(err)
	}
	if !needed {
		fmt.Printf("pins: Chrome for Testing Stable is %s, which is no milestone above the Target Chrome %s; nothing to roll\n",
			version, target)
		return 0
	}

	return roll(ctx, r, version)
}

// rollNeeded reads Chrome for Testing's last-known-good Stable and reports
// whether its milestone is above target's. It returns the Stable version
// either way, so that the caller can name it in both outcomes, and it reads
// nothing but the version JSON: the decision costs one request.
func rollNeeded(ctx context.Context, r *roller, target string) (string, bool, error) {
	pinned, err := milestone(target)
	if err != nil {
		return "", false, err
	}

	version, _, err := r.stable(ctx)
	if err != nil {
		return "", false, err
	}

	stable, err := milestone(version)
	if err != nil {
		return "", false, err
	}

	return version, stable > pinned, nil
}

// milestone is the Chrome milestone of a four-number Chrome version: the 153
// of 153.0.8010.12.
func milestone(version string) (int, error) {
	m := chromeVersion.FindStringSubmatch(version)
	if m == nil {
		return 0, notAChromeVersion(version)
	}

	// The pattern admits only digits, so this fails on nothing but a number
	// too large for an int, which no Chrome milestone will ever be.
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, fmt.Errorf("the milestone of the Chrome version %q: %w", version, err)
	}
	return n, nil
}

// outputNames lists the outputs as prose: "a, b and c".
func outputNames() string {
	names := ""
	for i, o := range outputs {
		switch {
		case i == 0:
		case i == len(outputs)-1:
			names += " and "
		default:
			names += ", "
		}
		names += o.path
	}
	return names
}

func fail(err error) int {
	fmt.Fprintln(os.Stderr, "pins:", err)
	return 1
}

type options struct {
	check   bool
	render  bool
	ifNewer bool
	version string
}

// chromeVersion is the four-number form Chrome for Testing publishes, its
// first number captured because that is the milestone.
var chromeVersion = regexp.MustCompile(`^(\d+)\.\d+\.\d+\.\d+$`)

func parseArgs(args []string) (options, error) {
	var opts options
	fs := flagSet(&opts)
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}

	if err := opts.modes(); err != nil {
		return options{}, err
	}
	if err := opts.takeVersion(fs.Args()); err != nil {
		return options{}, err
	}

	return opts, nil
}

// notAChromeVersion is the one wording for a string that is not a Chrome
// version, shared by the argument and the milestone comparison so that the
// two cannot drift apart.
func notAChromeVersion(version string) error {
	return fmt.Errorf("%q is not a Chrome version of the form 152.0.7977.82", version)
}

// modes rejects the flag combinations that ask for two runs at once: each
// flag names a whole run, so no two of them go together.
func (o *options) modes() error {
	switch {
	case o.check && o.render:
		return errors.New("-check and -render exclude each other")
	case o.ifNewer && (o.check || o.render):
		return errors.New("-if-newer rolls or does nothing: it excludes -check and -render")
	}
	return nil
}

// takeVersion reads the one optional version argument, which only a plain run
// accepts: each flag either decides the version for itself or reads the pins
// as committed.
func (o *options) takeVersion(rest []string) error {
	switch {
	case len(rest) > 1:
		return errors.New("at most one version may be given")
	case len(rest) == 0:
		return nil
	case o.check:
		return errors.New("-check takes no version: it checks the pins as committed")
	case o.render:
		return errors.New("-render takes no version: it rewrites the pins as committed")
	case o.ifNewer:
		return errors.New("-if-newer takes no version: a version given is a Roll already decided on")
	case !chromeVersion.MatchString(rest[0]):
		return notAChromeVersion(rest[0])
	}

	o.version = rest[0]
	return nil
}

// flagSet declares the flags on opts. usage prints the same set, so every
// flag is described once.
func flagSet(opts *options) *flag.FlagSet {
	fs := flag.NewFlagSet("pins", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&opts.check, "check", false,
		"write nothing; fail unless the committed pins re-derive and every output re-renders to the same bytes")
	fs.BoolVar(&opts.render, "render", false,
		"download nothing; rewrite every output from the committed pins")
	fs.BoolVar(&opts.ifNewer, "if-newer", false,
		"roll only when Chrome for Testing's Stable is a newer milestone than the Target Chrome; otherwise say so and do nothing")
	return fs
}

func usage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "usage: go run ./lib/launcher/pins/generate [-check | -render | -if-newer | <version>]")
	fs := flagSet(&options{})
	fs.SetOutput(w)
	fs.PrintDefaults()
}
