// Package main ...
package main

import (
	"fmt"

	"github.com/headlesslab/wand"
)

func main() {
	wand.New().MustConnect().MustPage("https://www.google.com/").MustWaitLoad().MustPDF("sample.pdf")
	fmt.Println("wrote sample.pdf")
}
