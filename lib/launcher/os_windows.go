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

// startGuarded starts the browser under the Orphan guard, in libuv's order:
// created suspended, assigned to a kill-on-close job object, then resumed,
// so that no instant of its life is outside the job. Windows passes no
// pipe; the job object is the whole guard, whatever the browser. A browser
// the guard cannot take is killed, and the launch fails rather than run
// unguarded.
func (l *Launcher) startGuarded(cmd *exec.Cmd) error {
	job, err := newKillOnCloseJob()
	if err != nil {
		return err
	}

	cmd.SysProcAttr.CreationFlags |= windows.CREATE_SUSPENDED

	err = cmd.Start()
	if err != nil {
		_ = windows.CloseHandle(job)
		return err
	}

	// cmd.Process holds a handle to the browser, so its PID cannot be
	// recycled under these two calls.
	err = assignToJob(job, cmd.Process.Pid)
	if err == nil {
		err = resume(cmd.Process.Pid)
	}
	if err != nil {
		_ = cmd.Process.Kill()
		go func() { _ = cmd.Wait() }()
		_ = windows.CloseHandle(job)
		return fmt.Errorf("[launcher] the Orphan guard could not take the browser: %w", err)
	}

	l.guard.job = job
	return nil
}

// newKillOnCloseJob creates a job object whose processes the kernel kills
// when its last handle closes. No breakaway is allowed, so the browser's own
// children join the job too.
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

// assignToJob puts the process in the job, through a handle of its own since
// os.Process does not lend the one it holds.
func assignToJob(job windows.Handle, pid int) error {
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return err
	}
	defer func() { _ = windows.CloseHandle(process) }()

	return windows.AssignProcessToJobObject(job, process)
}

// resume the one thread of a process created suspended. os.StartProcess
// closes the thread handle CreateProcess returned, so the thread is found
// again through a snapshot.
func resume(pid int) error {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return err
	}
	defer func() { _ = windows.CloseHandle(snapshot) }()

	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	for err = windows.Thread32First(snapshot, &entry); err == nil; err = windows.Thread32Next(snapshot, &entry) {
		if entry.OwnerProcessID != uint32(pid) {
			continue
		}

		thread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
		if err != nil {
			return err
		}
		_, err = windows.ResumeThread(thread)
		_ = windows.CloseHandle(thread)
		return err
	}

	return fmt.Errorf("no thread of process %d: %w", pid, err)
}

func terminateProcess(pid int) {
	handle, err := syscall.OpenProcess(syscall.PROCESS_TERMINATE, true, uint32(pid))
	if err != nil {
		return
	}

	_ = syscall.TerminateProcess(handle, 0)
	_ = syscall.CloseHandle(handle)
}
