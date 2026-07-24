package proc

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// KillTreeOnExit places the current process into a Job Object configured with
// KILL_ON_JOB_CLOSE. Every descendant spawned afterwards (MCP servers, ConPTY
// shells, git, npx→cmd→node chains, ...) joins the job automatically, and the
// whole tree is killed by the OS the moment this process dies — clean exit,
// crash, or taskkill alike. This is the single-point fix for orphaned child
// processes; call it once, first thing in main.
//
// Best effort by design: if job creation/assignment fails (e.g. an ancient
// Windows that can't nest jobs when a parent already put us in one), the app
// must still run — we just lose the guarantee. Returns whether the guarantee
// is in place.
func KillTreeOnExit() bool {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return false
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	if _, err := windows.SetInformationJobObject(
		job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)),
	); err != nil {
		_ = windows.CloseHandle(job)
		return false
	}
	if err := windows.AssignProcessToJobObject(job, windows.CurrentProcess()); err != nil {
		_ = windows.CloseHandle(job)
		return false
	}
	// The job handle is deliberately never closed: KILL_ON_JOB_CLOSE fires
	// when the last handle closes, which must be exactly when this process
	// exits and the OS reclaims it.
	return true
}
