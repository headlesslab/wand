package main

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/headlesslab/wand/lib/launcher/pins"
	"github.com/ysmood/got"
)

var setup = got.Setup(nil)

func TestRender(t *testing.T) {
	g := setup(t)

	line, err := render(false)
	g.E(err)
	g.Eq(line, fmt.Sprintf("Chrome %s, protocol r%d, Chromium %d",
		pins.ChromeVersion, pins.ProtocolRoll, pins.ChromiumPosition))

	out, err := render(true)
	g.E(err)
	g.Eq(out, fmt.Sprintf(`{"chrome":%q,"protocol":%d,"chromium":%d}`,
		pins.ChromeVersion, pins.ProtocolRoll, pins.ChromiumPosition))

	var decoded struct {
		Chrome   string
		Protocol int
		Chromium int
	}
	g.E(json.Unmarshal([]byte(out), &decoded))
	g.Eq(decoded.Chrome, pins.ChromeVersion)
	g.Eq(decoded.Protocol, pins.ProtocolRoll)
	g.Eq(decoded.Chromium, pins.ChromiumPosition)
}
