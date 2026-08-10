//go:build unix

package spacewave_cli

import "syscall"

const aptDebOpenFlag = syscall.O_NONBLOCK
