package kernel

import (
	"context"
	"encoding/binary"
	"io"
)

// posixFunctions returns the env module: the POSIX functions wasi-libc
// declares but does not define. A guest built against those headers imports
// each one it calls.
func posixFunctions() map[string]hostFunc {
	return map[string]hostFunc{
		// Identity.
		"getpid":  {"i", func(_ context.Context, p *Process, _ []uint64) uint64 { return uint64(p.pid) }},
		"getppid": {"i", func(_ context.Context, p *Process, _ []uint64) uint64 { return uint64(p.ppid) }},
		"getpgrp": {"i", func(_ context.Context, p *Process, _ []uint64) uint64 { return uint64(p.pgid) }},
		"setsid": {"i", func(ctx context.Context, p *Process, _ []uint64) uint64 {
			return p.posixResult(ctx, p.sysSetsid())
		}},
		"setpgid": {"iii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysSetpgid(int32(a[0]), int32(a[1])))
		}},
		"setpgrp": {"i", func(ctx context.Context, p *Process, _ []uint64) uint64 {
			return p.posixResult(ctx, p.sysSetpgid(0, 0))
		}},
		"getpgid": {"ii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysGetpgid(int32(a[0])))
		}},
		"getsid": {"ii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysGetsid(int32(a[0])))
		}},

		// Credentials.
		"getuid":  {"i", func(_ context.Context, p *Process, _ []uint64) uint64 { return uint64(p.uids.real) }},
		"geteuid": {"i", func(_ context.Context, p *Process, _ []uint64) uint64 { return uint64(p.uids.eff) }},
		"getgid":  {"i", func(_ context.Context, p *Process, _ []uint64) uint64 { return uint64(p.gids.real) }},
		"getegid": {"i", func(_ context.Context, p *Process, _ []uint64) uint64 { return uint64(p.gids.eff) }},
		"getresuid": {"iiii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.writeRes(p.uids, uint32(a[0]), uint32(a[1]), uint32(a[2])))
		}},
		"getresgid": {"iiii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.writeRes(p.gids, uint32(a[0]), uint32(a[1]), uint32(a[2])))
		}},
		"setuid": {"ii", func(_ context.Context, p *Process, a []uint64) uint64 {
			p.uids.set(uint32(a[0]))
			return 0
		}},
		"setgid": {"ii", func(_ context.Context, p *Process, a []uint64) uint64 {
			p.gids.set(uint32(a[0]))
			return 0
		}},
		"seteuid": {"ii", func(_ context.Context, p *Process, a []uint64) uint64 {
			p.uids.setRes(idKeep, uint32(a[0]), idKeep)
			return 0
		}},
		"setegid": {"ii", func(_ context.Context, p *Process, a []uint64) uint64 {
			p.gids.setRes(idKeep, uint32(a[0]), idKeep)
			return 0
		}},
		"setreuid": {"iii", func(_ context.Context, p *Process, a []uint64) uint64 {
			p.uids.setRe(uint32(a[0]), uint32(a[1]))
			return 0
		}},
		"setregid": {"iii", func(_ context.Context, p *Process, a []uint64) uint64 {
			p.gids.setRe(uint32(a[0]), uint32(a[1]))
			return 0
		}},
		"setresuid": {"iiii", func(_ context.Context, p *Process, a []uint64) uint64 {
			p.uids.setRes(uint32(a[0]), uint32(a[1]), uint32(a[2]))
			return 0
		}},
		"setresgid": {"iiii", func(_ context.Context, p *Process, a []uint64) uint64 {
			p.gids.setRes(uint32(a[0]), uint32(a[1]), uint32(a[2]))
			return 0
		}},
		"getgroups": {"iii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysGetgroups(uint32(a[0]), uint32(a[1])))
		}},
		"setgroups": {"iii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysSetgroups(uint32(a[0]), uint32(a[1])))
		}},
		"getlogin_r": {"iii", func(_ context.Context, p *Process, a []uint64) uint64 {
			p.inst.Memory().Write(uint32(a[0]), []byte("root\x00"))
			return 0
		}},

		// Process control. vfork is fork, which Linux permits.
		"vfork":        {"i", forkFunc},
		"fork":         {"i", forkFunc},
		"_exit":        {"vi", exitFunc},
		"__posix_exit": {"vi", exitFunc},
		"abort": {"v", func(_ context.Context, p *Process, _ []uint64) uint64 {
			p.abort()
			return 0
		}},
		"execve": {"iiii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysExecve(ctx, uint32(a[0]), uint32(a[1]), uint32(a[2])))
		}},
		"execv": {"iii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			mem := p.inst.Memory()
			path, args := readCString(mem, uint32(a[0])), readArgvFromMemory(mem, uint32(a[1]))
			return p.posixResult(ctx, p.exec(ctx, path, args, nil))
		}},
		"execvp": {"iii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			mem := p.inst.Memory()
			file, args := readCString(mem, uint32(a[0])), readArgvFromMemory(mem, uint32(a[1]))
			return p.posixResult(ctx, p.posixExecvp(ctx, file, args))
		}},

		// The execl family passes its arguments after the second as varargs,
		// which wasm32 passes as a pointer to the list.
		"execl": {"iiii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			mem := p.inst.Memory()
			args, _ := readExeclArgs(mem, uint32(a[1]), uint32(a[2]))
			return p.posixResult(ctx, p.exec(ctx, readCString(mem, uint32(a[0])), args, nil))
		}},
		"execlp": {"iiii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			mem := p.inst.Memory()
			args, _ := readExeclArgs(mem, uint32(a[1]), uint32(a[2]))
			return p.posixResult(ctx, p.posixExecvp(ctx, readCString(mem, uint32(a[0])), args))
		}},
		"execle": {"iiii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			// Read the arguments, then envp after their null. A null envp
			// gives the program an empty environment.
			mem := p.inst.Memory()
			args, envpAt := readExeclArgs(mem, uint32(a[1]), uint32(a[2]))
			envp, _ := mem.ReadUint32Le(envpAt)
			env := readEnvpFromMemory(mem, envp)
			if env == nil {
				env = map[string]string{}
			}
			return p.posixResult(ctx, p.exec(ctx, readCString(mem, uint32(a[0])), args, env))
		}},
		"waitpid": {"iiii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			// ECHILD with WNOHANG reports that no child changed state, so a
			// reap loop ends without an error.
			options := uint32(a[2])
			result := p.sysWait4(ctx, int32(a[0]), uint32(a[1]), options, 0)
			if int32(result) == -ECHILD && options&wnohang != 0 {
				return 0
			}
			return p.posixResult(ctx, result)
		}},
		"wait": {"ii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysWait4(ctx, -1, uint32(a[0]), 0, 0))
		}},
		"pause": {"i", func(ctx context.Context, p *Process, _ []uint64) uint64 {
			return p.posixResult(ctx, p.suspendWithMask(ctx, p.sigMask))
		}},

		// Signals.
		"kill": {"iii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysKill(int32(a[0]), uint32(a[1])))
		}},
		"raise": {"ii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysKill(int32(p.pid), uint32(a[0])))
		}},
		"killpg": {"iii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysKill(-int32(a[0]), uint32(a[1])))
		}},
		"sigaction": {"iiii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysRtSigaction(uint32(a[0]), uint32(a[1]), uint32(a[2])))
		}},
		"signal": {"iii", func(_ context.Context, p *Process, a []uint64) uint64 {
			signum := uint32(a[0])
			if signum == 0 || signum >= 32 {
				return fail
			}
			return uint64(p.setSigAction(signum, &sigAction{handler: uint32(a[1])}).handler)
		}},
		"sigprocmask": {"iiii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysRtSigprocmask(uint32(a[0]), uint32(a[1]), uint32(a[2])))
		}},
		"sigsuspend": {"ii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysRtSigsuspend(ctx, uint32(a[0])))
		}},
		"sigpending": {"ii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysRtSigpending(uint32(a[0])))
		}},
		"sigaltstack": {"iii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysSigaltstack(uint32(a[0]), uint32(a[1])))
		}},
		"setitimer": {"iiii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysSetitimer(uint32(a[0]), uint32(a[1]), uint32(a[2])))
		}},
		"getitimer": {"iii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysGetitimer(uint32(a[0]), uint32(a[1])))
		}},
		"alarm": {"ii", func(_ context.Context, p *Process, a []uint64) uint64 {
			return p.sysAlarm(uint32(a[0]))
		}},
		"sigtimedwait": {"iiii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysSigtimedwait(ctx, uint32(a[0]), uint32(a[1]), uint32(a[2])))
		}},
		"sigwait": {"iii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			// sigwait returns the errno rather than setting it.
			sig := p.sysSigtimedwait(ctx, uint32(a[0]), 0, 0)
			if int32(sig) < 0 {
				return uint64(-int32(sig))
			}
			p.inst.Memory().WriteUint32Le(uint32(a[1]), uint32(sig))
			return 0
		}},

		// A sigset_t is 64 bits, bit n-1 standing for signal n.
		"sigemptyset": {"ii", func(_ context.Context, p *Process, a []uint64) uint64 {
			p.inst.Memory().WriteUint64Le(uint32(a[0]), 0)
			return 0
		}},
		"sigfillset": {"ii", func(_ context.Context, p *Process, a []uint64) uint64 {
			p.inst.Memory().WriteUint64Le(uint32(a[0]), ^uint64(0))
			return 0
		}},
		"sigaddset": {"iii", func(_ context.Context, p *Process, a []uint64) uint64 {
			mem := p.inst.Memory()
			set, _ := mem.ReadUint64Le(uint32(a[0]))
			mem.WriteUint64Le(uint32(a[0]), set|1<<(uint32(a[1])-1))
			return 0
		}},
		"sigdelset": {"iii", func(_ context.Context, p *Process, a []uint64) uint64 {
			mem := p.inst.Memory()
			set, _ := mem.ReadUint64Le(uint32(a[0]))
			mem.WriteUint64Le(uint32(a[0]), set&^(1<<(uint32(a[1])-1)))
			return 0
		}},
		"sigismember": {"iii", func(_ context.Context, p *Process, a []uint64) uint64 {
			set, _ := p.inst.Memory().ReadUint64Le(uint32(a[0]))
			return set >> (uint32(a[1]) - 1) & 1
		}},
		"sigisemptyset": {"ii", func(_ context.Context, p *Process, a []uint64) uint64 {
			set, _ := p.inst.Memory().ReadUint64Le(uint32(a[0]))
			if set != 0 {
				return 0
			}
			return 1
		}},

		// The signal headers declare SIG_IGN and SIG_ERR as functions.
		"__SIG_IGN": returns("vi", 0),
		"__SIG_ERR": returns("vi", 0),

		// Non-local jumps.
		"setjmp": {"ii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			p.setjmp(ctx, uint32(a[0]))
			return 0
		}},
		"sigsetjmp": {"iii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			p.setjmp(ctx, uint32(a[0]))
			return 0
		}},
		"_setjmp": {"ii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			p.setjmp(ctx, uint32(a[0]))
			return 0
		}},
		"longjmp":    {"vii", longjmpFunc},
		"_longjmp":   {"vii", longjmpFunc},
		"siglongjmp": {"vii", longjmpFunc},

		// Terminal control.
		"tcgetattr": {"iii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			t := p.fdTerminal(uint32(a[0]))
			if t == nil {
				return p.posixResult(ctx, negErrno(ENOTTY))
			}
			t.writeWasiTermios(p.inst.Memory(), uint32(a[1]))
			return 0
		}},
		"tcsetattr": {"iiii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			t := p.fdTerminal(uint32(a[0]))
			if t == nil {
				return p.posixResult(ctx, negErrno(ENOTTY))
			}
			t.readWasiTermios(p.inst.Memory(), uint32(a[2]))
			return 0
		}},
		"tcgetpgrp": {"ii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysTcgetpgrp(uint32(a[0])))
		}},
		"tcsetpgrp": {"iii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysTcsetpgrp(uint32(a[0]), int32(a[1])))
		}},
		// The console has no queues to discard and no flow control to stop.
		"tcflush": returns("iii", 0),
		"tcflow":  returns("iii", 0),
		"cfgetispeed": {"ii", func(_ context.Context, p *Process, a []uint64) uint64 {
			speed, _ := p.inst.Memory().ReadUint32Le(uint32(a[0]) + termiosIspeed)
			return uint64(speed)
		}},
		"cfgetospeed": {"ii", func(_ context.Context, p *Process, a []uint64) uint64 {
			speed, _ := p.inst.Memory().ReadUint32Le(uint32(a[0]) + termiosOspeed)
			return uint64(speed)
		}},
		"cfsetispeed": {"iii", func(_ context.Context, p *Process, a []uint64) uint64 {
			p.inst.Memory().WriteUint32Le(uint32(a[0])+termiosIspeed, uint32(a[1]))
			return 0
		}},
		"cfsetospeed": {"iii", func(_ context.Context, p *Process, a []uint64) uint64 {
			p.inst.Memory().WriteUint32Le(uint32(a[0])+termiosOspeed, uint32(a[1]))
			return 0
		}},
		"ttyname": {"ii", func(ctx context.Context, p *Process, _ []uint64) uint64 {
			// The kernel owns no guest memory to hold the name.
			p.setErrno(ctx, wasiErrno(ENOSYS))
			return 0
		}},
		"ttyname_r": {"iiii", func(_ context.Context, p *Process, a []uint64) uint64 {
			return uint64(p.ttyname(uint32(a[0]), uint32(a[1]), uint32(a[2])))
		}},
		"__wasivm_ioctl": {"iiii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysIoctl(uint32(a[0]), uint32(a[1]), uint32(a[2])))
		}},

		// Pseudo-terminals. The C library's ptsname reads the index with
		// TIOCGPTN.
		"posix_openpt": {"ii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			flags := uint32(a[0])
			return p.posixResult(ctx, p.openPtmx(oRDWR|kernelStatusFlags(flags), flags&guestOCLOEXEC != 0))
		}},
		"grantpt": {"ii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			_, errno := p.fdMaster(uint32(a[0]))
			return p.posixResult(ctx, errno)
		}},
		"unlockpt": {"ii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			pt, errno := p.fdMaster(uint32(a[0]))
			if errno == 0 {
				pt.locked = false
			}
			return p.posixResult(ctx, errno)
		}},

		// Memory files and file mappings. The C library's mmap, msync and
		// munmap call the __wasivm imports for a file mapping.
		"memfd_create": {"iii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysMemfdCreate(uint32(a[0]), uint32(a[1])))
		}},
		"__wasivm_mmap": {"iiiiiij", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysMmap(uint32(a[0]), uint32(a[1]), uint32(a[2]), uint32(a[3]), uint32(a[4]), int64(a[5])))
		}},
		"__wasivm_msync": {"iii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysMsync(uint32(a[0]), uint32(a[1])))
		}},
		"__wasivm_munmap": {"iii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysMunmap(uint32(a[0]), uint32(a[1])))
		}},

		// Files. __posix_fcntl3 replaces wasi-libc's fcntl, which rejects
		// F_DUPFD.
		"dup": {"ii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysDup(uint32(a[0])))
		}},
		"dup2": {"iii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysDup2(uint32(a[0]), uint32(a[1])))
		}},
		"__posix_fcntl3": {"iiii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysFcntl(uint32(a[0]), uint32(a[1]), uint32(a[2])))
		}},
		"dup3": {"iiii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			// dup3 refuses to duplicate a descriptor onto itself and takes
			// only O_CLOEXEC.
			flags := uint32(a[2])
			if a[0] == a[1] || flags&^guestOCLOEXEC != 0 {
				return p.posixResult(ctx, negErrno(EINVAL))
			}
			ret := p.sysDup2(uint32(a[0]), uint32(a[1]))
			if ret == a[1] {
				p.setCloexec(uint32(ret), flags != 0)
			}
			return p.posixResult(ctx, ret)
		}},
		"pipe": {"ii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysPipe2(uint32(a[0]), 0))
		}},
		"pipe2": {"iii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysPipe2(uint32(a[0]), uint32(a[1])))
		}},

		// Event, timer, signal and epoll files. The C library's
		// timerfd_create passes its clock's Linux number.
		"eventfd": {"iii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysEventfd(uint32(a[0]), uint32(a[1])))
		}},
		"signalfd": {"iiii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysSignalfd(int32(a[0]), uint32(a[1]), uint32(a[2])))
		}},
		"__wasivm_timerfd_create": {"iii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysTimerfdCreate(uint32(a[0]), uint32(a[1])))
		}},
		"timerfd_settime": {"iiiii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysTimerfdSettime(uint32(a[0]), uint32(a[1]), uint32(a[2]), uint32(a[3])))
		}},
		"timerfd_gettime": {"iii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysTimerfdGettime(uint32(a[0]), uint32(a[1])))
		}},
		"epoll_create1": {"ii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysEpollCreate1(uint32(a[0])))
		}},
		"epoll_ctl": {"iiiii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysEpollCtl(uint32(a[0]), uint32(a[1]), uint32(a[2]), uint32(a[3])))
		}},
		"epoll_wait": {"iiiii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysEpollWait(ctx, uint32(a[0]), uint32(a[1]), int32(a[2]), int32(a[3])))
		}},
		"epoll_pwait": {"iiiiii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysEpollPwait(ctx, uint32(a[0]), uint32(a[1]), int32(a[2]), int32(a[3]), uint32(a[4])))
		}},
		"umask": {"ii", func(_ context.Context, p *Process, a []uint64) uint64 {
			old := p.umask
			p.umask = uint32(a[0]) & 0o777
			return uint64(old)
		}},
		"chdir": {"ii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysChdir(uint32(a[0])))
		}},
		"fchdir": {"ii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysFchdir(uint32(a[0])))
		}},
		"__wasivm_getcwd": {"iii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysGetcwd(uint32(a[0]), uint32(a[1])))
		}},
		"flock": {"iii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysFlock(uint32(a[0]), uint32(a[1])))
		}},
		"fstat": {"iii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysFstat(uint32(a[0]), uint32(a[1])))
		}},
		"__wasilibc_nocwd_fstatat": {"iiiii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysFstatat(uint32(a[0]), uint32(a[1]), uint32(a[2]), uint32(a[3])))
		}},
		"__posix_chmod": {"iii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysFchmodat(atFDCWD, uint32(a[0]), uint32(a[1]), 0))
		}},
		"__posix_fchmod": {"iii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysFchmod(uint32(a[0]), uint32(a[1])))
		}},
		"__posix_fchmodat": {"iiiii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysFchmodat(uint32(a[0]), uint32(a[1]), uint32(a[2]), uint32(a[3])))
		}},
		"sendfile": {"iiiii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysSendfile(ctx, uint32(a[0]), uint32(a[1]), uint32(a[2]), uint32(a[3])))
		}},
		"mkfifo": {"iii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysMknodat(atFDCWD, uint32(a[0]), uint32(a[1])&0o7777|sIFIFO, 0))
		}},
		"mknod": {"iiij", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysMknodat(atFDCWD, uint32(a[0]), uint32(a[1]), uint32(a[2])))
		}},
		"getxattr": {"iiiii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysGetxattr(uint32(a[0]), uint32(a[1]), uint32(a[2]), uint32(a[3])))
		}},
		"listxattr": {"iiii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysListxattr(uint32(a[0]), uint32(a[1]), uint32(a[2])))
		}},
		"setxattr": {"iiiiii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysSetxattr(uint32(a[0]), uint32(a[1]), uint32(a[2]), uint32(a[3]), uint32(a[4])))
		}},
		"fchown": returns("iiii", 0),
		"chown":  returns("iiii", 0),
		"lchown": returns("iiii", 0),
		"chroot": returns("ii", 0),
		"sync":   returns("v", 0),

		// Mounts.
		"mount": {"iiiiii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			// Read the strings, of which data may be null.
			mem := p.inst.Memory()
			source, target, fstype := readCString(mem, uint32(a[0])), readCString(mem, uint32(a[1])), readCString(mem, uint32(a[2]))
			var data string
			if a[4] != 0 {
				data = readCString(mem, uint32(a[4]))
			}
			return p.posixResult(ctx, p.sysMount(source, target, fstype, uint32(a[3]), data))
		}},
		"umount": {"ii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysUmount2(readCString(p.inst.Memory(), uint32(a[0]))))
		}},
		"umount2": {"iii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysUmount2(readCString(p.inst.Memory(), uint32(a[0]))))
		}},

		// Resources and time.
		"getrlimit": {"iii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysGetrlimit(uint32(a[0]), uint32(a[1])))
		}},
		"setrlimit": {"iii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysSetrlimit(uint32(a[0]), uint32(a[1])))
		}},
		"getpriority": {"iii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysGetpriority(uint32(a[0]), uint32(a[1])))
		}},
		"setpriority": {"iiii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysSetpriority(uint32(a[0]), uint32(a[1]), uint32(a[2])))
		}},
		"nice": {"ii", func(_ context.Context, p *Process, a []uint64) uint64 {
			// Linux clamps the new value to the nice range and returns it.
			p.nice = min(max(p.nice+int32(a[0]), -20), 19)
			return uint64(uint32(p.nice))
		}},
		"clock_settime": {"iii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysClockSettime(uint32(a[0]), uint32(a[1])))
		}},
		"settimeofday": {"iii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysSettimeofday(uint32(a[0])))
		}},
		"reboot": {"ii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysReboot(uint32(a[0])))
		}},
		"sysinfo": {"ii", func(_ context.Context, p *Process, a []uint64) uint64 {
			p.sysSysinfo(uint32(a[0]))
			return 0
		}},
		"tzset": returns("v", 0),

		// System names.
		"uname": {"ii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysUname(uint32(a[0])))
		}},
		"sethostname": {"iii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysSetUTSName(&p.kernel.hostname, uint32(a[0]), uint32(a[1])))
		}},
		"setdomainname": {"iii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysSetUTSName(&p.kernel.domainname, uint32(a[0]), uint32(a[1])))
		}},

		// Sockets. Only Unix stream sockets exist. wasi-libc implements
		// accept, send, recv and shutdown over the WASI calls.
		"socket": {"iiii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysSocket(uint32(a[0]), uint32(a[1]), uint32(a[2])))
		}},
		"socketpair": {"iiiii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysSocketpair(uint32(a[0]), uint32(a[1]), uint32(a[2]), uint32(a[3])))
		}},
		"bind": {"iiii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysBind(uint32(a[0]), uint32(a[1]), uint32(a[2])))
		}},
		"listen": {"iii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysListen(uint32(a[0])))
		}},
		"connect": {"iiii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysConnect(uint32(a[0]), uint32(a[1]), uint32(a[2])))
		}},
		"getsockname": {"iiii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysSockname(uint32(a[0]), uint32(a[1]), uint32(a[2]), false))
		}},
		"getpeername": {"iiii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysSockname(uint32(a[0]), uint32(a[1]), uint32(a[2]), true))
		}},
		"setsockopt": {"iiiiii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysSetsockopt(uint32(a[0])))
		}},
		"getsockopt": {"iiiiii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysGetsockopt(uint32(a[0]), uint32(a[1]), uint32(a[2]), uint32(a[3]), uint32(a[4])))
		}},
		"sendto": {"iiiiiii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysSendto(ctx, uint32(a[0]), uint32(a[1]), uint32(a[2]), uint32(a[3]), uint32(a[5])))
		}},
		"recvfrom": {"iiiiiii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysRecvfrom(ctx, uint32(a[0]), uint32(a[1]), uint32(a[2]), uint32(a[3]), uint32(a[5])))
		}},
		"sendmsg": {"iiii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysSendmsg(ctx, uint32(a[0]), uint32(a[1]), uint32(a[2])))
		}},
		"recvmsg": {"iiii", func(ctx context.Context, p *Process, a []uint64) uint64 {
			return p.posixResult(ctx, p.sysRecvmsg(ctx, uint32(a[0]), uint32(a[1]), uint32(a[2])))
		}},
		"getaddrinfo":  returns("iiiii", fail),
		"freeaddrinfo": returns("vi", 0),
		"gai_strerror": returns("ii", 0),

		// Dynamic loading is unsupported.
		"dlopen":  returns("iii", 0),
		"dlclose": returns("ii", 0),
		"dlsym":   returns("iii", 0),
		"dlerror": returns("i", 0),
	}
}

// sysSysinfo writes the system's statistics as a wasm32 struct sysinfo to
// ptr: the uptime, memory in bytes and process count. The kernel has no swap
// and does not average the load.
func (p *Process) sysSysinfo(ptr uint32) {
	// Clear the struct up to its reserved tail.
	k := p.kernel
	mem := p.inst.Memory()
	mem.Write(ptr, make([]byte, sysinfoReserved))

	// Fill the uptime, memory, process count and memory unit.
	used := min(k.memUsed(), memTotal)
	mem.WriteUint32Le(ptr, uint32(k.uptime().Seconds()))
	mem.WriteUint32Le(ptr+16, memTotal)
	mem.WriteUint32Le(ptr+20, uint32(memTotal-used))
	mem.Write(ptr+40, binary.LittleEndian.AppendUint16(nil, uint16(len(k.procs.filter(nil)))))
	mem.WriteUint32Le(ptr+52, 1)
}

// sysinfoReserved is the offset of the wasm32 struct sysinfo's reserved
// tail, after the uptime, loads, memory and swap sizes, process count,
// high memory sizes and memory unit.
const sysinfoReserved = 56

// forkFunc starts a child process that runs beside the caller.
func forkFunc(ctx context.Context, p *Process, _ []uint64) uint64 {
	return p.posixResult(ctx, p.posixFork(ctx))
}

// exitFunc ends the process with the status in its argument.
func exitFunc(_ context.Context, p *Process, a []uint64) uint64 {
	p.posixExit(uint32(a[0]))
	return 0
}

// longjmpFunc jumps back to the setjmp call for the jmp_buf in its first
// argument, which then returns the second argument, or 1 when that is 0.
func longjmpFunc(ctx context.Context, p *Process, a []uint64) uint64 {
	p.longjmp(ctx, uint32(a[0]), max(uint32(a[1]), 1))
	return 0
}

// abort raises SIGABRT and ends the process as SIGABRT would.
func (p *Process) abort() {
	p.sysKill(int32(p.pid), SIGABRT)
	p.posixExit(128 + SIGABRT)
}

// setjmp records the execution state for the jmp_buf at buf: the engine
// snapshot, the C stack between the stack pointer and the heap base, and the
// signal handlers running. It writes the state's id in the buffer's first
// word, where longjmp finds it.
func (p *Process) setjmp(ctx context.Context, buf uint32) {
	// Without snapshots, longjmp cannot return here.
	snapshotter := p.kernel.runtime.Snapshotter(ctx)
	if snapshotter == nil {
		return
	}

	// Copy the C stack, which a longjmp must restore along with the engine
	// state.
	entry := jmpBufEntry{
		snapshot:     snapshotter.Snapshot(),
		buf:          buf,
		handlerDepth: p.handlerDepth,
	}
	var heapBase uint32
	if g := p.inst.ExportedGlobal(guestStackPointer); g != nil {
		entry.sp = uint32(g.Get())
	}
	if g := p.inst.ExportedGlobal("__heap_base"); g != nil {
		heapBase = uint32(g.Get())
	}
	if entry.sp > 0 && heapBase > entry.sp {
		if data, ok := p.inst.Memory().Read(entry.sp, heapBase-entry.sp); ok {
			entry.cstack = append([]byte(nil), data...)
		}
	}
	p.inst.Memory().WriteUint32Le(buf, p.jmpBufs.record(entry))
}

// longjmp restores the state setjmp recorded in the jmp_buf at buf, making
// that setjmp return val. A jump without a live setjmp aborts the process.
func (p *Process) longjmp(ctx context.Context, buf, val uint32) {
	// Find the state.
	id, _ := p.inst.Memory().ReadUint32Le(buf)
	entry, ok := p.jmpBufs.jump(id)
	if !ok {
		_, _ = io.WriteString(p.kernel.console.stderr, "longjmp: no matching setjmp\n")
		p.abort()
		return
	}

	// Restore the C stack, the handler depth and the engine state.
	if g := p.inst.ExportedGlobal(guestStackPointer); g != nil {
		g.Set(uint64(entry.sp))
	}
	p.inst.Memory().Write(entry.sp, entry.cstack)
	p.handlerDepth = entry.handlerDepth
	entry.snapshot.Restore(ctx, []uint64{uint64(val)})
}

// posixResult converts a Linux syscall result to the C convention: a
// negative errno becomes -1 with errno set in the guest.
func (p *Process) posixResult(ctx context.Context, result uint64) uint64 {
	if int32(result) >= 0 {
		return result
	}
	p.setErrno(ctx, wasiErrno(uint32(-int32(result))))
	return fail
}

// setErrno sets the guest's errno to the WASI errno value. A guest that does
// not export __errno_location keeps its errno.
func (p *Process) setErrno(ctx context.Context, errno uint32) {
	// Find errno, then set it.
	fn := p.inst.ExportedFunction("__errno_location")
	if fn == nil {
		return
	}
	results, err := fn.Call(ctx)
	if err != nil || len(results) == 0 {
		return
	}
	p.inst.Memory().WriteUint32Le(uint32(results[0]), errno)
}
