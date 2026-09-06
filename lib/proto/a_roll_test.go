package proto_test

import (
	"github.com/headlesslab/wand/lib/launcher/pins"
	"github.com/headlesslab/wand/lib/proto"
)

// Roll holds the generated package to the pins: the protocol layer is
// generated from the Protocol roll the pins name, and a Roll that moved the
// pins without regenerating fails here before the generate Gate's
// zero-diff check has to (ADR-0004).
func (t T) Roll() {
	t.Eq(proto.Roll, pins.ProtocolRoll)
}
