// Package main is the Roll tool: it computes everything lib/launcher/pins
// holds and rewrites that package, so the launcher, the protocol generator,
// the Roll and the release workflow all read the same numbers.
//
//	go run ./lib/launcher/pins/generate [<version>]
//	go run ./lib/launcher/pins/generate -check
//
// With no argument the Target Chrome is Chrome for Testing's last-known-good
// Stable; with a version argument it is that version (a Security roll, or a
// Roll forced by hand). From its branch position the tool derives the Protocol
// roll, the largest devtools-protocol v0.0.<rev> tag not above the position,
// and the Companion Chromium, the newest Chromium trunk position at or below
// it whose archive exists for all five bucket prefixes. It then downloads
// every Managed browser archive from Google's origin bucket, never from a
// mirror, records each SHA-256 and rewrites lib/launcher/pins/pins.go. An
// archive Google does not serve is reported and the exit status is 1; what was
// verified is still written, so the gap shows in the diff instead of hiding.
//
// -check rewrites nothing: it re-derives the Protocol roll from the recorded
// branch position and re-renders the file from the recorded pins, and fails
// when either differs from what is committed. That is the generate Gate's
// zero-diff check for this package (ADR-0004, ADR-0005, ADR-0009).
package main

import (
	"bytes"
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

// pinsFile is the file the Roll writes, relative to the module root, where
// every generator of this repository runs from.
const pinsFile = "lib/launcher/pins/pins.go"

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

	s := newSources()

	if opts.check {
		return check(ctx, s)
	}
	return roll(ctx, s, opts.version)
}

func check(ctx context.Context, s *sources) int {
	file, err := os.ReadFile(pinsFile)
	if err != nil {
		return fail(err)
	}
	if err := s.check(ctx, current(), file); err != nil {
		return fail(err)
	}

	fmt.Printf("pins: %s is what the Roll writes; branch position %d derives protocol r%d\n",
		pinsFile, pins.ChromePosition, pins.ProtocolRoll)
	return 0
}

func roll(ctx context.Context, s *sources, version string) int {
	p, missing, err := s.roll(ctx, version)
	if err != nil {
		return fail(err)
	}
	out, err := render(p)
	if err != nil {
		return fail(err)
	}
	if err := os.WriteFile(pinsFile, out, 0o644); err != nil {
		return fail(err)
	}

	fmt.Printf("pins: wrote %s for Chrome %s (branch position %d, protocol r%d, Chromium %d)\n",
		pinsFile, p.ChromeVersion, p.ChromePosition, p.ProtocolRoll, p.ChromiumPosition)

	if len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "pins: %d archive(s) missing from Google's bucket:\n", len(missing))
		for _, u := range missing {
			fmt.Fprintln(os.Stderr, "  "+u)
		}
		return 1
	}
	return 0
}

func fail(err error) int {
	fmt.Fprintln(os.Stderr, "pins:", err)
	return 1
}

type options struct {
	check   bool
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
	case len(rest) > 1:
		return options{}, errors.New("at most one version may be given")
	case len(rest) == 1 && opts.check:
		return options{}, errors.New("-check takes no version: it checks the pins as committed")
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
		"write nothing; fail unless the committed pins re-derive and re-render to the same bytes")
	return fs
}

func usage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "usage: go run ./lib/launcher/pins/generate [-check] [<version>]")
	fs := flagSet(&options{})
	fs.SetOutput(w)
	fs.PrintDefaults()
}

// current is what the pins package holds at build time.
func current() Pins {
	return Pins{
		ChromeVersion:    pins.ChromeVersion,
		ChromePosition:   pins.ChromePosition,
		ProtocolRoll:     pins.ProtocolRoll,
		ChromiumPosition: pins.ChromiumPosition,
		ChromeSHA256:     pins.ChromeSHA256,
		ChromiumSHA256:   pins.ChromiumSHA256,
	}
}

// normalize undoes the CRLF a Windows checkout with autocrlf applies, so the
// bytes compare to what the Roll writes.
func normalize(file []byte) []byte {
	return bytes.ReplaceAll(file, []byte("\r\n"), []byte("\n"))
}
