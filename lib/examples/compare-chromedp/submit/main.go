// Package main ...
package main

import (
	"log"
	"strings"

	"github.com/headlesslab/wand"
	"github.com/headlesslab/wand/lib/input"
)

// This example demonstrates how to fill out and submit a form.
func main() {
	page := wand.New().MustConnect().MustPage("https://github.com/search")

	page.MustElement(`input[name=q]`).MustWaitVisible().MustInput("chromedp").MustType(input.Enter)

	res := page.MustElementR("a", "chromedp").MustParent().MustParent().MustNext().MustText()

	log.Printf("got: `%s`", strings.TrimSpace(res))
}
