//go:build linux || darwin

package uninstall

import (
	"fmt"
	"os"
	"runtime"
	"slices"
)

// unixProtected are filesystem roots whose removal would be catastrophic. A
// planned path equal to one of these is refused.
var unixProtected = []string{
	"/", "/bin", "/boot", "/dev", "/etc", "/home", "/lib", "/lib64",
	"/opt", "/proc", "/root", "/run", "/sbin", "/srv", "/sys", "/tmp",
	"/usr", "/var",
}

func platformProtectedPath(clean string) bool {
	return slices.Contains(unixProtected, clean)
}

// manualServiceUnit returns the location of a native service unit vmbench
// might have been manually configured with, or "" when the platform has
// none. vmbench never installs a service itself; the check only informs the
// operator.
func manualServiceUnit() string {
	if runtime.GOOS == "darwin" {
		return "/Library/LaunchDaemons/io.cloudapp.vmbench.plist"
	}
	return "/etc/systemd/system/vmbench.service"
}

// removeExecutable deletes the running binary. On Linux and macOS deleting
// an in-use executable only unlinks the directory entry; the inode is
// reclaimed when the process exits, so the command finishes normally.
func removeExecutable(path string) (removed bool, problems []string) {
	err := os.Remove(path)
	switch {
	case err == nil:
		return true, nil
	case os.IsNotExist(err):
		return false, nil
	default:
		return false, []string{fmt.Sprintf("%s: remove binary: %v", path, err)}
	}
}
