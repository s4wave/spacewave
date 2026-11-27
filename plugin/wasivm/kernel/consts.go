package kernel

// Linux signal numbers (asm-generic).
const (
	// SIGHUP reports a hangup of the controlling terminal.
	SIGHUP = 1

	// SIGINT interrupts the foreground process: ^C.
	SIGINT = 2

	// SIGQUIT quits the foreground process: ^\.
	SIGQUIT = 3

	// SIGABRT aborts a process.
	SIGABRT = 6

	// SIGKILL kills a process and cannot be caught.
	SIGKILL = 9

	// SIGPIPE reports a write to a pipe with no reader.
	SIGPIPE = 13

	// SIGALRM reports the expiry of ITIMER_REAL.
	SIGALRM = 14

	// SIGTERM asks a process to terminate.
	SIGTERM = 15

	// SIGCHLD reports a child that changed state.
	SIGCHLD = 17

	// SIGSTOP stops a process and cannot be caught.
	SIGSTOP = 19

	// SIGTSTP stops the foreground process from the terminal: ^Z.
	SIGTSTP = 20

	// SIGWINCH reports a terminal window size change.
	SIGWINCH = 28
)

// rt_sigprocmask "how" values.
const (
	sigBlock   = 0
	sigUnblock = 1
	sigSetmask = 2
)

// guestSaSiginfo is the sigaction flag asking for a handler with siginfo.
const guestSaSiginfo = 0x4

// Signal dispositions below which a sigaction handler is a guest function.
const (
	// sigDfl selects the signal's default action.
	sigDfl = 0
	// sigIgn ignores the signal.
	sigIgn = 1
)

// sigTerminateMask is the sigset of signals whose default action ends the
// process. The default action of the other signals the kernel sends is to
// ignore them.
const sigTerminateMask = uint64(1)<<(SIGHUP-1) |
	uint64(1)<<(SIGINT-1) |
	uint64(1)<<(SIGQUIT-1) |
	uint64(1)<<(SIGKILL-1) |
	uint64(1)<<(SIGPIPE-1) |
	uint64(1)<<(SIGALRM-1) |
	uint64(1)<<(SIGTERM-1)

// sigKillStopMask is the sigset of signals that can never be blocked
// (SIGKILL and SIGSTOP), in Linux sigset bit layout (bit signo-1).
const sigKillStopMask = uint64(1)<<(SIGKILL-1) | uint64(1)<<(SIGSTOP-1)

// sigaltstack ss_flags bits.
const (
	ssOnstack = 1
	ssDisable = 2
)

// Priority selectors.
const (
	prioProcess = 0
	prioPgrp    = 1
	prioUser    = 2
)

// Clock ids (asm-generic).
const (
	clockRealtime         = 0
	clockMonotonic        = 1
	clockProcessCPUTimeID = 2
	clockThreadCPUTimeID  = 3
	clockMonotonicRaw     = 4
	clockRealtimeCoarse   = 5
	clockMonotonicCoarse  = 6
	clockBoottime         = 7
)

// Interval timer ids.
const (
	itimerReal    = 0
	itimerVirtual = 1
	itimerProf    = 2
)

// Open flags (octal, asm-generic).
const (
	oRDONLY    = 0o0
	oWRONLY    = 0o1
	oRDWR      = 0o2
	oCREAT     = 0o100
	oEXCL      = 0o200
	oNOCTTY    = 0o400
	oTRUNC     = 0o1000
	oAPPEND    = 0o2000
	oNONBLOCK  = 0o4000
	oDIRECTORY = 0o200000
	oCLOEXEC   = 0o2000000
)

// Fcntl commands.
const (
	fDUPFD         = 0
	fGETFD         = 1
	fSETFD         = 2
	fGETFL         = 3
	fSETFL         = 4
	fDUPFD_CLOEXEC = 0x406
	fADD_SEALS     = 0x409
	fGET_SEALS     = 0x40a
	fdCLOEXEC      = 1
)

// Termios flag bits (asm-generic).
const (
	icrnl  = 0x0100 // ICRNL: map CR to NL on input
	opost  = 0x0001 // OPOST: enable output processing
	onlcr  = 0x0004 // ONLCR: map NL to CR-NL on output
	isig   = 0x0001 // ISIG: enable signals
	icanon = 0x0002 // ICANON: canonical (line) mode
	echo   = 0x0008 // ECHO: echo input characters
	echoe  = 0x0010 // ECHOE: echo erase as backspace, space, backspace
)

// File type constants (mode bits).
const (
	sIFMT   = 0o170000
	sIFDIR  = 0o040000
	sIFCHR  = 0o020000
	sIFBLK  = 0o060000
	sIFREG  = 0o100000
	sIFIFO  = 0o010000
	sIFLNK  = 0o120000
	sIFSOCK = 0o140000
)

// Open file status flags as the guest's headers number them: wasi-libc uses
// the WASI file descriptor flags and rights bits.
const (
	guestOAPPEND   = 0x1
	guestONONBLOCK = 0x4
	guestORDONLY   = 0x04000000
	guestOWRONLY   = 0x10000000
	// guestOCLOEXEC is O_CLOEXEC, which wasi-libc defines as zero. The
	// WasiVM headers give it a private bit of the WASI descriptor flags,
	// which open passes to path_open.
	guestOCLOEXEC = 0x200
)

// Socket constants as the guest's headers number them.
const (
	guestAFUNIX       = 3
	guestSockStream   = 6
	guestSockNonblock = 0x4000
	guestSockCloexec  = 0x2000
	guestMsgWaitall   = 0x2
	guestMsgCtrunc    = 0x8
	guestMsgDontwait  = 0x40
	guestMsgNosignal  = 0x4000
	// guestMsgCmsgCloexec is MSG_CMSG_CLOEXEC.
	guestMsgCmsgCloexec = 0x40000000
	// guestSolSocket is SOL_SOCKET, which wasi-libc numbers apart from
	// Linux.
	guestSolSocket = 0x7fffffff
	guestScmRights = 1
	// The SOL_SOCKET option names, which wasi-libc numbers as Linux does.
	guestSoType       = 3
	guestSoError      = 4
	guestSoPeercred   = 17
	guestSoAcceptconn = 30
	guestSoProtocol   = 38
	guestSoDomain     = 39
)

// flock operations (asm-generic).
const (
	lockSH = 1
	lockEX = 2
	lockNB = 4
	lockUN = 8
)

// xattr flags.
const (
	xattrCreate  = 1
	xattrReplace = 2
)

// Special constants.
const (
	atFDCWD           = 0xFFFFFF9C // -100 as uint32
	atSymlinkNofollow = 0x100
	atREMOVEDIR       = 0x200
	atEMPTY_PATH      = 0x1000
	wnohang           = 1

	// wasiAtSymlinkNofollow is wasi-libc's AT_SYMLINK_NOFOLLOW, which its
	// own lstat passes. Linux leaves the bit unused.
	wasiAtSymlinkNofollow = 0x1
)

// Resource limit ids (getrlimit/setrlimit/prlimit64, asm-generic). Only the
// ids needing a finite default are named. rlimitNlimits bounds the table.
const (
	rlimitStack   = 3
	rlimitNofile  = 7
	rlimitNlimits = 16

	// rlimInfinity is RLIM_INFINITY: the limit is unbounded.
	rlimInfinity = ^uint64(0)
)
