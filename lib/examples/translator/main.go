// Package main ...
package main

import (
	"flag"
	"fmt"
	"log"
	"strings"

	"github.com/headlesslab/wand"
)

func main() {
	flag.Parse()

	// get the commandline arguments
	source := strings.TrimSpace(strings.Join(flag.Args(), " "))
	if source == "" {
		log.Fatal("usage: go run main.go -- 'This is the phrase to translate to Spanish.'")
	}

	browser := wand.New().MustConnect()

	page := browser.MustPage("https://translate.google.com/?sl=auto&tl=es&op=translate")

	el := page.MustElement(`textarea[aria-label="Source text"]`)

	// MustWaitRequestIdle takes excludes, so this names the requests the wait
	// ignores: the account calls the page keeps making would otherwise hold it
	// open. They are regexps matched against a request's URL, not prefixes, so
	// the host is anchored and its dots escaped, or the pattern would also
	// ignore https://accounts-google-com.example/ and any URL that merely
	// carries this one in a query string.
	wait := page.MustWaitRequestIdle(`^https://accounts\.google\.com/`)
	el.MustInput(source)
	wait()

	result := page.MustElement("[role=region] span[lang]").MustText()

	fmt.Println(result)
}
