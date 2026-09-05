// Package main ...
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
