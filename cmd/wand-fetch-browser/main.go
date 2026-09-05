// Command wand-fetch-browser downloads the managed browser into wand's browser
// cache ahead of time, for container image builds and offline bundles, and
// prints the path of the browser binary.
package main

import (
	"fmt"

	"github.com/headlesslab/wand/lib/launcher"
	"github.com/headlesslab/wand/lib/utils"
)

func main() {
	p, err := launcher.NewBrowser().Get()
	utils.E(err)

	fmt.Println(p)
}
