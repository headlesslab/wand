//go:build !windows && !linux

package launcher

import (
	"testing"
)

func configureDriver(string) {}

// TestGuardTether: the platform has no parent-death signal, so the Pipe
// tether is the whole guard: the kernel closes the driver's end of the pipe
// with the driver and the browser exits on the EOF.
func TestGuardTether(t *testing.T) {
	g := setup(t)

	pid, driver := startDriver(t, "on")
	g.True(processAlive(pid))

	killDriver(t, driver)
	g.True(waitGone(pid, guardBound))
}
