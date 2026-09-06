//go:build linux

package launcher

import (
	"testing"
)

func configureDriver(mode string) {
	if mode == "tether" {
		parentDeathSignal = 0
	}
}

// TestGuardPdeathsig: the browser is started with the parent-death signal
// from a thread the driver holds, so a driver killed with SIGKILL takes the
// browser with it at once.
func TestGuardPdeathsig(t *testing.T) {
	g := setup(t)

	pid, driver := startDriver(t, "on")
	g.True(processAlive(pid))

	killDriver(t, driver)
	g.True(waitGone(pid, guardBound))
}

// TestGuardTether: with the parent-death signal disabled in the driver, the
// Pipe tether alone brings the browser down, since the kernel closes the
// driver's end of the pipe and the browser exits on the EOF.
func TestGuardTether(t *testing.T) {
	g := setup(t)

	pid, driver := startDriver(t, "tether")
	g.True(processAlive(pid))

	killDriver(t, driver)
	g.True(waitGone(pid, guardBound))
}
