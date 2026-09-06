// Package main prints the three pins of lib/launcher/pins for the release
// workflow, which heads every release with them and appends them to
// versions.json, so no workflow parses Go source (ADR-0008, ADR-0009):
//
//	go run ./internal/tools/print-pins        # Chrome 152.0.7977.82, protocol r1666840, Chromium 1668000
//	go run ./internal/tools/print-pins -json  # {"chrome":"152.0.7977.82","protocol":1666840,"chromium":1668000}
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/headlesslab/wand/lib/launcher/pins"
)

func main() {
	asJSON := flag.Bool("json", false, "print the pins as one JSON object instead of one line of text")
	flag.Parse()

	out, err := render(*asJSON)
	if err != nil {
		fmt.Fprintln(os.Stderr, "print-pins:", err)
		os.Exit(1)
	}
	fmt.Println(out)
}

// render is the Target Chrome, the Protocol roll and the Companion Chromium,
// as one line of text or as one JSON object.
func render(asJSON bool) (string, error) {
	if !asJSON {
		return fmt.Sprintf("Chrome %s, protocol r%d, Chromium %d",
			pins.ChromeVersion, pins.ProtocolRoll, pins.ChromiumPosition), nil
	}

	out, err := json.Marshal(struct {
		Chrome   string `json:"chrome"`
		Protocol int    `json:"protocol"`
		Chromium int    `json:"chromium"`
	}{pins.ChromeVersion, pins.ProtocolRoll, pins.ChromiumPosition})
	if err != nil {
		return "", err
	}
	return string(out), nil
}
