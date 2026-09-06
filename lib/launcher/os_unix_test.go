//go:build !windows

package launcher

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"testing"
)

func processAlive(pid int) bool {
	// Signal zero probes for the process; a permission error is a process.
	// A zombie counts until its parent, init once the driver is gone, has
	// reaped it.
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// TestTetherErr: a Pipe tether that cannot be opened fails the launch before
// any process starts, and a pipe opened before the failing one is closed
// again.
func TestTetherErr(t *testing.T) {
	g := setup(t)

	defer func() { newPipe = os.Pipe }()

	for _, failAt := range []int{1, 2} {
		calls := 0
		var opened []*os.File
		newPipe = func() (*os.File, *os.File, error) {
			calls++
			if calls == failAt {
				return nil, nil, errors.New("no pipe")
			}
			r, w, err := os.Pipe()
			opened = append(opened, r, w)
			return r, w, err
		}

		_, err := New().Bin("not-exists").Launch()
		g.Desc("pipe %d", failAt).Eq(err.Error(), "no pipe")

		for _, f := range opened {
			g.Desc("pipe %d", failAt).Err(f.Close())
		}
	}
}

// TestLaunchXVFB: under xvfb-run, which closes descriptor 3 for the command
// it runs, the Pipe tether still reaches the browser as descriptors 3 and 4,
// so a Chrome of 113 or later starts instead of aborting for want of them.
func TestLaunchXVFB(t *testing.T) {
	g := setup(t)

	if _, err := exec.LookPath("xvfb-run"); err != nil {
		g.Skip("xvfb-run is not installed")
	}

	l := New().XVFB("-a")
	defer func() {
		l.Kill()
		l.Cleanup()
	}()

	u, err := l.Launch()
	g.E(err)
	g.Regex(`\Aws://.+\z`, u)
}
