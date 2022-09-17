package gitroot

import (
	"os/exec"
	"strings"
)

// FindRepoRoot uses the Git Cli to find the git root dir.
func FindRepoRoot() (string, error) {
	path, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(path)), nil
}
