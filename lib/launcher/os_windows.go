//go:build windows

package launcher

import (
	"fmt"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// guard is the Orphan guard's hold on a launched browser: the job object the
// browser and its children belong to, set to kill them when its last handle
// closes. wand holds the only handle, so the kernel closes the job with the
// wand process, however it dies.
type guard struct {
	job windows.Handle
}

func (g *guard) release() {
	if g.job != 0 {
		_ = windows.CloseHandle(g.job)
		g.job = 0
	}
}

func killGroup(pid int) {
	terminateProcess(pid)
}

func (l *Launcher) osSetupCmd(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

// start the browser, with the Orphan guard when guarded: the browser joins a
// kill-on-close job object as soon as it has started. Windows passes no pipe;
// the job object is the whole guard, whatever the browser.
func (l *Launcher) start(cmd *exec.Cmd, guarded bool) error {
	if !guarded {
		return cmd.Start()
	}

	job, err := newKillOnCloseJob()
	if err != nil {
		return err
	}

	err = cmd.Start()
	if err != nil {
		_ = windows.CloseHandle(job)
		return err
	}

	err = assignToJob(job, cmd.Process.Pid)
	if err != nil {
		_ = cmd.Process.Kill()
		go func() { _ = cmd.Wait() }()
		_ = windows.CloseHandle(job)
		return fmt.Errorf("[launcher] the browser could not join the Orphan guard's job object: %w", err)
	}

	l.guard.job = job
	return nil
}

func newKillOnCloseJob() (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, err
	}

	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE

	_, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)))
	if err != nil {
		_ = windows.CloseHandle(job)
		return 0, err
	}

	return job, nil
}

func assignToJob(job windows.Handle, pid int) error {
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return err
	}
	defer func() { _ = windows.CloseHandle(process) }()

	return windows.AssignProcessToJobObject(job, process)
}

func terminateProcess(pid int) {
	handle, err := syscall.OpenProcess(syscall.PROCESS_TERMINATE, true, uint32(pid))
	if err != nil {
		return
	}

	_ = syscall.TerminateProcess(handle, 0)
	_ = syscall.CloseHandle(handle)
}
