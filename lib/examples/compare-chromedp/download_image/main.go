// Package main ...
package main

import (
	"log"
	"os"

	"github.com/headlesslab/wand"
)

func main() {
	u := "https://avatars.githubusercontent.com/u/33149672"

	browser := wand.New().MustConnect()

	page := browser.MustPage(u).MustWaitLoad()

	b, err := page.GetResource(u)
	if err != nil {
		log.Fatal(err)
	}

	if err := os.WriteFile("download.png", b, 0o644); err != nil {
		log.Fatal(err)
	}
	log.Print("wrote download.png")
}
