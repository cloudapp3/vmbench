//go:build windows

package uninstall

import (
	"fmt"
	"os/exec"
	"slices"
	"strings"
)

// windowsProtected are filesystem roots whose removal would be catastrophic.
var windowsProtected = []string{
	`C:\`, `C:\Windows`, `C:\Program Files`, `C:\Program Files (x86)`,
	`C:\ProgramData`, `C:\Users`,
}

func platformProtectedPath(clean string) bool {
	return slices.ContainsFunc(windowsProtected, func(p string) bool {
		return strings.EqualFold(clean, p)
	})
}

// manualServiceUnit returns "" — vmbench on Windows runs from the console,
// and any service registration is manual, so there is no conventional unit
// location to check.
func manualServiceUnit() string { return "" }

// removeExecutable cannot delete the running .exe directly on Windows. It
// spawns a detached cmd that waits ~2s (long enough for this process to
// exit) and then deletes the binary.
func removeExecutable(path string) (removed bool, problems []string) {
	cmd := exec.Command("cmd", "/c", `ping -n 3 127.0.0.1 >nul & del /q "`+path+`"`)
	if err := cmd.Start(); err != nil {
		return false, []string{fmt.Sprintf("schedule binary deletion: %v", err)}
	}
	_ = cmd.Process.Release()
	return true, nil
}
