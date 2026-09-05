// Package main ...
package main

import (
	"fmt"

	"github.com/headlesslab/wand"
	"github.com/headlesslab/wand/lib/launcher"
)

func main() {
	l := launcher.New()

	// For more info: https://pkg.go.dev/github.com/headlesslab/wand/lib/launcher
	u := l.MustLaunch()

	browser := wand.New().ControlURL(u).MustConnect()

	page := browser.MustPage("http://example.com").MustWaitStable()

	fmt.Println(page.MustInfo().Title)
}
