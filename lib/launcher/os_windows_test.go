//go:build windows

package launcher

import (
	"testing"

	"golang.org/x/sys/windows"
)

// stillActive is the exit code GetExitCodeProcess reports for a running process.
const stillActive = 259

func configureDriver(string) {}

func processAlive(pid int) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer func() { _ = windows.CloseHandle(h) }()

	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	return code == stillActive
}

// TestGuardJobObject: the browser joins a kill-on-close job object whose only
// handle the driver holds, so a driver killed with TerminateProcess takes the
// browser with it.
func TestGuardJobObject(t *testing.T) {
	g := setup(t)

	pid, driver := startDriver(t, "on")
	g.True(processAlive(pid))

	killDriver(t, driver)
	g.True(waitGone(pid, guardBound))
}
