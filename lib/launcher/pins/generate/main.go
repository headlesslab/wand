// Package main is the Roll tool: it computes everything lib/launcher/pins
// holds and rewrites every place the pins appear, so the launcher, the
// protocol generator, the Roll, the release workflow and the READMEs all
// carry the same numbers.
//
//	go run ./lib/launcher/pins/generate [<version>]
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
	version string
}

// chromeVersion is the four-number form Chrome for Testing publishes.
var chromeVersion = regexp.MustCompile(`^\d+\.\d+\.\d+\.\d+$`)

func parseArgs(args []string) (options, error) {
	var opts options
	fs := flagSet(&opts)
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}

	rest := fs.Args()
	switch {
	case opts.check && opts.render:
		return options{}, errors.New("-check and -render exclude each other")
	case len(rest) > 1:
		return options{}, errors.New("at most one version may be given")
	case len(rest) == 1 && opts.check:
		return options{}, errors.New("-check takes no version: it checks the pins as committed")
	case len(rest) == 1 && opts.render:
		return options{}, errors.New("-render takes no version: it rewrites the pins as committed")
	case len(rest) == 1:
		if !chromeVersion.MatchString(rest[0]) {
			return options{}, fmt.Errorf("%q is not a Chrome version of the form 152.0.7977.82", rest[0])
		}
		opts.version = rest[0]
	}

	return opts, nil
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
	return fs
}

func usage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "usage: go run ./lib/launcher/pins/generate [-check | -render | <version>]")
	fs := flagSet(&options{})
	fs.SetOutput(w)
	fs.PrintDefaults()
}
