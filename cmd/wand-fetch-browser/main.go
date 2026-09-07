// Command wand-fetch-browser downloads the Managed browser into wand's browser
// cache ahead of time, for container image builds and offline bundles, and
// prints the path of the browser binary, alone on stdout so that a script can
// take it, with the download's progress on stderr. It is the Target Chrome
// from Chrome for Testing unless the flags or the WAND_BROWSER_* environment
// variables say otherwise; WAND_BROWSER_DOWNLOAD does not apply, since
// downloading is what the command is for.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/headlesslab/wand/lib/launcher"
	"github.com/headlesslab/wand/lib/utils"
)

var (
	source  = flag.String("source", "", `the Browser source: "chrome" for Chrome for Testing (the default) or "chromium" for a Chromium trunk build`)
	binary  = flag.String("binary", "", `the Chrome for Testing binary: "chrome" (the default) or "chrome-headless-shell"`)
	version = flag.String("version", "", "the Chrome for Testing version, the Target Chrome by default; any other version has no pinned archive hash, so it is verified by nothing but the transport")
)

func main() {
	flag.Parse()

	b := launcher.NewBrowser()
	b.Logger = log.New(os.Stderr, "[wand-fetch-browser] ", log.LstdFlags)

	if *source != "" {
		b.Source = launcher.Source(*source)
	}

	if *binary != "" {
		b.Binary = launcher.Binary(*binary)
	}

	// The pins hold one Chrome for Testing version, the Target Chrome, so a
	// version given here downloads without a hash to check it against; the
	// Support-window Nightly (#61) is what asks for one, and its browser is
	// read from Chrome for Testing over TLS rather than approved by a Roll.
	if *version != "" {
		b.Version = *version
	}

	p, err := b.Get()
	utils.E(err)

	fmt.Println(p)
}
