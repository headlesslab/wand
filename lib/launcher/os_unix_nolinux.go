//go:build !windows && !linux

package launcher

import (
	"os/exec"
)

// startCmd starts the browser. The platform has no parent-death signal, so
// the Orphan guard here is the Pipe tether alone.
func (l *Launcher) startCmd(cmd *exec.Cmd) error {
	return cmd.Start()
}
