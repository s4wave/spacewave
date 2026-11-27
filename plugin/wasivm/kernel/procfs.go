package kernel

import (
	"fmt"
	"maps"
	"math"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"
)

// clockTicks is the rate of the tick counts /proc reports, Linux's USER_HZ.
const clockTicks = 100

// memTotal is the memory size the kernel reports. Guests share the host's
// memory without a fixed size, so the value is nominal.
const memTotal = 2 << 30

// pageSize is the page size /proc counts memory in.
const pageSize = 4096

// newProcFS creates the root of a proc filesystem: a directory for each
// process, self linking to the calling process's directory, and files that
// describe the system.
func (k *Kernel) newProcFS() *VNode {
	// Add the system files.
	v := k.vfs
	root := v.newDir(0o555)
	files := map[string]func() []byte{
		"cmdline":     func() []byte { return []byte("console=hvc0\n") },
		"cpuinfo":     func() []byte { return []byte("processor\t: 0\nmodel name\t: WebAssembly\n\n") },
		"filesystems": k.procFilesystems,
		"loadavg":     k.procLoadavg,
		"meminfo":     k.procMeminfo,
		"mounts":      k.procMounts,
		"stat":        k.procStat,
		"uptime":      k.procUptime,
		"version":     func() []byte { return []byte("Linux version " + utsRelease + " (spacewave)\n") },
	}
	for name, gen := range files {
		v.mkGenFile(root, name, gen)
	}

	// List the processes at each use.
	self := v.mkSymlink(root, "self", "")
	root.list = func(dir *VNode) {
		k.listProcs(dir)
		if k.caller != nil {
			self.setSymlink(strconv.Itoa(k.caller.pid))
		}
	}
	return root
}

// listProcs updates the proc root dir to hold a directory for each process.
func (k *Kernel) listProcs(dir *VNode) {
	// Drop the directories of reaped processes.
	for name := range dir.children {
		if pid, err := strconv.Atoi(name); err == nil && k.procs.get(pid) == nil {
			delete(dir.children, name)
			dir.nlink--
		}
	}

	// Add the new process directories.
	for _, p := range k.procs.filter(nil) {
		name := strconv.Itoa(p.pid)
		if dir.children[name] == nil {
			link(dir, name, k.newProcDir(p.pid))
			dir.nlink++
		}
	}
}

// newProcDir creates the /proc directory of the process pid. Its files show
// the process holding pid when the files open. An exec can replace that
// process. Its task directory lists the process's one thread, whose ID is
// pid, with the same files, as Linux lists each thread of a process.
func (k *Kernel) newProcDir(pid int) *VNode {
	// Fill the process directory.
	v := k.vfs
	dir := v.newDir(0o555)
	k.fillProcDir(dir, pid)

	// Add the task directory and its one thread.
	task := v.newDir(0o555)
	link(dir, "task", task)
	dir.nlink++
	thread := v.newDir(0o555)
	link(task, strconv.Itoa(pid), thread)
	task.nlink++
	k.fillProcDir(thread, pid)
	return dir
}

// fillProcDir adds to dir the files and links that describe the process pid.
func (k *Kernel) fillProcDir(dir *VNode, pid int) {
	// Add the files.
	v := k.vfs
	files := map[string]func(p *Process) []byte{
		"cmdline": procCmdline,
		"comm":    func(p *Process) []byte { return []byte(p.comm() + "\n") },
		"environ": procEnviron,
		"stat":    k.procPidStat,
		"statm":   procStatm,
		"status":  k.procStatus,
	}
	for name, render := range files {
		v.mkGenFile(dir, name, func() []byte {
			if p := k.procs.get(pid); p != nil {
				return render(p)
			}
			return nil
		})
	}
	v.mkGenFile(dir, "mounts", k.procMounts)

	// Add the links, which follow the process at each use.
	links := map[string]*VNode{}
	for _, name := range []string{"cwd", "exe", "root"} {
		links[name] = v.mkSymlink(dir, name, "")
	}
	fds := v.mkDir(dir, "fd")
	fds.list = func(fds *VNode) {
		if p := k.procs.get(pid); p != nil {
			k.listFDs(fds, p)
		}
	}
	dir.list = func(*VNode) {
		if p := k.procs.get(pid); p != nil {
			links["cwd"].setSymlink(p.cwd)
			links["exe"].setSymlink(p.exe)
			links["root"].setSymlink("/")
		}
	}
}

// listFDs fills the /proc/<pid>/fd directory dir with a link for each of
// p's descriptors to the file it has open.
func (k *Kernel) listFDs(dir *VNode, p *Process) {
	clear(dir.children)
	for fd, of := range p.fds {
		target := of.path
		switch {
		case of.sock != nil:
			target = fmt.Sprintf("socket:[%d]", of.sock.ino)
		case of.pipe() != nil:
			target = fmt.Sprintf("pipe:[%d]", of.pipe().ino)
		}
		k.vfs.mkSymlink(dir, strconv.FormatUint(uint64(fd), 10), target)
	}
}

// comm returns the process's command name, the base name of its program.
func (p *Process) comm() string {
	if len(p.args) == 0 {
		return path.Base(p.exe)
	}
	return path.Base(p.args[0])
}

// state returns the process's state letter: R for the caller, Z for a
// process that exited and S for one waiting its turn or blocked.
func (k *Kernel) state(p *Process) byte {
	switch {
	case p.exited():
		return 'Z'
	case p == k.caller:
		return 'R'
	}
	return 'S'
}

// procCmdline renders /proc/<pid>/cmdline: the arguments, each ending in a
// NUL.
func procCmdline(p *Process) []byte {
	var out []byte
	for _, arg := range p.args {
		out = append(append(out, arg...), 0)
	}
	return out
}

// procEnviron renders /proc/<pid>/environ: the environment entries, each
// ending in a NUL.
func procEnviron(p *Process) []byte {
	var out []byte
	for _, key := range slices.Sorted(maps.Keys(p.env)) {
		out = append(append(out, key+"="+p.env[key]...), 0)
	}
	return out
}

// procStatm renders /proc/<pid>/statm, the memory use in pages.
func procStatm(p *Process) []byte {
	pages := p.memSize / pageSize
	return fmt.Appendf(nil, "%d %d 0 0 0 %d 0\n", pages, pages, pages)
}

// procPidStat renders /proc/<pid>/stat in the Linux field order. The kernel
// does not count CPU time or page faults, which read as zero.
func (k *Kernel) procPidStat(p *Process) []byte {
	// Fill the fields up to the resource limit of the resident set.
	ttyNr, tpgid := uint64(0), -1
	if t := p.terminal(); t != nil {
		ttyNr, tpgid = t.node.rdev, t.foreground(p)
	}
	started := p.started.Sub(k.bootTime) * clockTicks / time.Second
	fields := []any{
		p.pid, "(" + p.comm() + ")", string(k.state(p)), p.ppid, p.pgid, p.sid, ttyNr, tpgid,
		0, 0, 0, 0, 0, // flags and page faults
		0, 0, 0, 0, // CPU times
		20 + p.nice, p.nice, 1, 0, int64(started), p.memSize, p.memSize / pageSize,
		uint64(math.MaxUint64), // rsslim
	}

	// Leave the addresses, signal masks and scheduling fields zero, apart
	// from the exit signal, and end with the exit code.
	for range 12 {
		fields = append(fields, 0)
	}
	fields = append(fields, SIGCHLD)
	for range 13 {
		fields = append(fields, 0)
	}
	exitCode := 0
	if p.exited() {
		exitCode = p.exitCode
	}
	fields = append(fields, exitCode)
	return fmt.Appendln(nil, fields...)
}

// procStatus renders /proc/<pid>/status.
func (k *Kernel) procStatus(p *Process) []byte {
	// Read the signal state.
	p.sigMu.Lock()
	pending, blocked := p.sigPending, p.sigMask
	var ignored, caught uint64
	for sig, act := range p.sigActions {
		switch {
		case sig == 0 || act.handler == sigDfl:
		case act.handler == sigIgn:
			ignored |= sigBit(uint32(sig))
		default:
			caught |= sigBit(uint32(sig))
		}
	}
	p.sigMu.Unlock()

	// Render the identity and credentials.
	groups := make([]string, len(p.groups))
	for i, g := range p.groups {
		groups[i] = strconv.FormatUint(uint64(g), 10)
	}
	out := fmt.Appendf(nil, "Name:\t%s\nUmask:\t%04o\nState:\t%c\n", p.comm(), p.umask, k.state(p))
	out = fmt.Appendf(out, "Tgid:\t%d\nPid:\t%d\nPPid:\t%d\nTracerPid:\t0\n", p.pid, p.pid, p.ppid)
	out = fmt.Appendf(out, "Uid:\t%d\t%d\t%d\t%d\n", p.uids.real, p.uids.eff, p.uids.saved, p.uids.fs)
	out = fmt.Appendf(out, "Gid:\t%d\t%d\t%d\t%d\n", p.gids.real, p.gids.eff, p.gids.saved, p.gids.fs)
	out = fmt.Appendf(out, "FDSize:\t%d\nGroups:\t%s\n", len(p.fds), strings.Join(groups, " "))

	// Render the memory and signal state.
	out = fmt.Appendf(out, "VmSize:\t%8d kB\nVmRSS:\t%8d kB\nThreads:\t1\n", p.memSize/1024, p.memSize/1024)
	out = fmt.Appendf(out, "SigPnd:\t%016x\nShdPnd:\t%016x\nSigBlk:\t%016x\n", pending, 0, blocked)
	return fmt.Appendf(out, "SigIgn:\t%016x\nSigCgt:\t%016x\n", ignored, caught)
}

// procFilesystems renders /proc/filesystems, the types mount accepts.
func (k *Kernel) procFilesystems() []byte {
	return []byte("nodev\tproc\nnodev\ttmpfs\nnodev\tramfs\nnodev\tdevpts\nnodev\tsysfs\nnodev\tmqueue\n")
}

// procMounts renders /proc/mounts, the mount table in mount order.
func (k *Kernel) procMounts() []byte {
	var out []byte
	for _, m := range k.mounts {
		out = fmt.Appendf(out, "%s %s %s %s 0 0\n", m.source, m.target, m.fstype, m.options())
	}
	return out
}

// memUsed returns the memory the processes' programs hold.
func (k *Kernel) memUsed() uint64 {
	var used uint64
	for _, p := range k.procs.filter(nil) {
		used += uint64(p.memSize)
	}
	return used
}

// procMeminfo renders /proc/meminfo. The processes' memory is the only
// memory in use.
func (k *Kernel) procMeminfo() []byte {
	free := (memTotal - min(k.memUsed(), memTotal)) / 1024
	return fmt.Appendf(nil, "MemTotal:\t%8d kB\nMemFree:\t%8d kB\nMemAvailable:\t%8d kB\n"+
		"Buffers:\t       0 kB\nCached:\t\t       0 kB\nShmem:\t\t       0 kB\nSReclaimable:\t       0 kB\n"+
		"SwapTotal:\t       0 kB\nSwapFree:\t       0 kB\n", memTotal/1024, free, free)
}

// uptime returns the time since the kernel started.
func (k *Kernel) uptime() time.Duration {
	return time.Since(k.bootTime)
}

// procUptime renders /proc/uptime: the seconds since boot and the seconds
// the CPU idled, which the kernel does not count.
func (k *Kernel) procUptime() []byte {
	return fmt.Appendf(nil, "%.2f 0.00\n", k.uptime().Seconds())
}

// procLoadavg renders /proc/loadavg. The kernel does not average the load,
// which reads as zero.
func (k *Kernel) procLoadavg() []byte {
	return fmt.Appendf(nil, "0.00 0.00 0.00 1/%d %d\n", len(k.procs.filter(nil)), k.procs.lastPID())
}

// procStat renders /proc/stat. The kernel does not count CPU time, so the
// CPU reads as idle since boot.
func (k *Kernel) procStat() []byte {
	idle := int64(k.uptime() * clockTicks / time.Second)
	return fmt.Appendf(nil, "cpu  0 0 0 %d 0 0 0 0 0 0\ncpu0 0 0 0 %d 0 0 0 0 0 0\n"+
		"intr 0\nctxt 0\nbtime %d\nprocesses %d\nprocs_running 1\nprocs_blocked 0\n",
		idle, idle, k.bootTime.Unix(), k.procs.lastPID())
}
