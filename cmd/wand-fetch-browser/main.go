// Command wand-fetch-browser downloads the Managed browser into wand's browser
// cache ahead of time, for container image builds and offline bundles, and
// prints the path of the browser binary. It is the Target Chrome from Chrome
// for Testing unless the flags or the WAND_BROWSER_* environment variables
// say otherwise.
package main

import (
	"flag"
	"fmt"

	"github.com/headlesslab/wand/lib/launcher"
	"github.com/headlesslab/wand/lib/utils"
)

var (
	source = flag.String("source", "", `the Browser source: "chrome" for Chrome for Testing (the default) or "chromium" for a Chromium trunk build`)
	binary = flag.String("binary", "", `the Chrome for Testing binary: "chrome" (the default) or "chrome-headless-shell"`)
)

func main() {
	flag.Parse()

	b := launcher.NewBrowser()

	if *source != "" {
		b.Source = launcher.Source(*source)
	}

	if *binary != "" {
		b.Binary = launcher.Binary(*binary)
	}

	p, err := b.Get()
	utils.E(err)

	fmt.Println(p)
}
