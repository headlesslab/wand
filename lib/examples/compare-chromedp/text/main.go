// Package main ...
package main

import (
	"log"
	"strings"

	"github.com/headlesslab/wand"
)

// This example demonstrates  how to extract text from a specific element.
func main() {
	page := wand.New().MustConnect().MustPage("https://pkg.go.dev/time")

	res := page.MustElement("#pkg-overview").MustParent().MustText()
	log.Println(strings.TrimSpace(res))
}
