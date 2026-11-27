package kernel

// sysSetsid starts a new session and process group led by p, with no
// controlling terminal, as Linux does. A process group leader cannot start a
// session.
func (p *Process) sysSetsid() uint64 {
	if p.pgid == p.pid {
		return negErrno(EPERM)
	}
	p.sid, p.pgid = p.pid, p.pid
	p.ctty = nil
	return uint64(p.pid)
}

// sysSetpgid moves the process pid, p itself or a child of p, into the
// process group pgid of p's session. Zero selects p for pid and the target's
// own PID for pgid, starting a group.
func (p *Process) sysSetpgid(pid, pgid int32) uint64 {
	// Find the target.
	if pid < 0 || pgid < 0 {
		return negErrno(EINVAL)
	}
	target := p
	if pid != 0 && int(pid) != p.pid {
		target = p.kernel.procs.get(int(pid))
		if target == nil || target.ppid != p.pid {
			return negErrno(ESRCH)
		}
	}

	// Join or start the group. A session leader stays in its group, and the
	// group must already exist in the session unless the target starts it.
	group := int(pgid)
	if group == 0 {
		group = target.pid
	}
	switch {
	case target.sid != p.sid || target.pid == target.sid:
		return negErrno(EPERM)
	case group != target.pid && !p.kernel.groupInSession(group, p.sid):
		return negErrno(EPERM)
	}
	target.pgid = group
	return 0
}

// groupInSession reports whether a process of session sid is in the process
// group pgid.
func (k *Kernel) groupInSession(pgid, sid int) bool {
	members := k.procs.filter(func(t *Process) bool { return t.pgid == pgid && t.sid == sid })
	return len(members) != 0
}

// sysGetpgid returns the process group of the process pid, or of p for 0.
func (p *Process) sysGetpgid(pid int32) uint64 {
	t := p.lookupPID(pid)
	if t == nil {
		return negErrno(ESRCH)
	}
	return uint64(t.pgid)
}

// sysGetsid returns the session of the process pid, or of p for 0.
func (p *Process) sysGetsid(pid int32) uint64 {
	t := p.lookupPID(pid)
	if t == nil {
		return negErrno(ESRCH)
	}
	return uint64(t.sid)
}

// lookupPID returns the process pid, p for 0, or nil.
func (p *Process) lookupPID(pid int32) *Process {
	if pid == 0 {
		return p
	}
	return p.kernel.procs.get(int(pid))
}

// sysTcgetpgrp returns the foreground process group of p's controlling
// terminal on fd. Before any process sets one, every process is in the
// foreground, so it is p's own process group.
func (p *Process) sysTcgetpgrp(fd uint32) uint64 {
	t := p.controllingFD(fd)
	if t == nil {
		return negErrno(ENOTTY)
	}
	return uint64(t.foreground(p))
}

// sysTcsetpgrp makes pgid the foreground process group of p's controlling
// terminal on fd. The foreground process group receives the signals of the
// terminal's control characters.
func (p *Process) sysTcsetpgrp(fd uint32, pgid int32) uint64 {
	t := p.controllingFD(fd)
	switch {
	case t == nil:
		return negErrno(ENOTTY)
	case pgid <= 0:
		return negErrno(EINVAL)
	case !p.kernel.groupInSession(int(pgid), p.sid):
		return negErrno(EPERM)
	}
	t.fgPgid = int(pgid)
	return 0
}

// controllingFD returns the terminal of descriptor fd when it is p's
// controlling terminal, or nil.
func (p *Process) controllingFD(fd uint32) *tty {
	of, ok := p.fds[fd]
	if !ok || of.ptm != nil {
		return nil
	}
	if t := of.terminal(); t != nil && t == p.terminal() {
		return t
	}
	return nil
}
