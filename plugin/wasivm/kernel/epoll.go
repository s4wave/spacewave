package kernel

import (
	"context"
	"encoding/binary"
	"time"
)

// epoll constants as the guest's headers number them: Linux's values.
const (
	epollIn      = 0x001
	epollOut     = 0x004
	epollHup     = 0x010
	epollRdhup   = 0x2000
	epollOneshot = 1 << 30
	epollET      = 1 << 31

	epollCtlAdd = 1
	epollCtlDel = 2
	epollCtlMod = 3

	guestEpollCloexec = 0o2000000
)

// epollEventSize is the size of a wasm32 struct epoll_event: a 32-bit events
// field, then the 64-bit data at offset 8.
const epollEventSize = 16

// sysEpollCreate1 opens an epoll file with an empty interest list.
func (p *Process) sysEpollCreate1(flags uint32) uint64 {
	if flags&^guestEpollCloexec != 0 {
		return negErrno(EINVAL)
	}
	of := &OpenFile{path: "anon_inode:[eventpoll]", flags: oRDWR, epoll: &epollSet{}}
	return uint64(p.installFD(of, flags&guestEpollCloexec != 0))
}

// sysEpollCtl adds, modifies or removes descriptor fd on the interest list of
// epoll file epfd, reading the events and data from the struct epoll_event
// at eventPtr. Only a file that reports readiness changes can be added.
func (p *Process) sysEpollCtl(epfd, op, fd, eventPtr uint32) uint64 {
	// Find the set and the file.
	ep, ok1 := p.fds[epfd]
	of, ok2 := p.fds[fd]
	switch {
	case !ok1 || !ok2:
		return negErrno(EBADF)
	case ep.epoll == nil || ep == of:
		return negErrno(EINVAL)
	}
	set := ep.epoll
	i := set.find(fd, of)

	// Read the event of an add or modify.
	var events uint32
	var data uint64
	if op != epollCtlDel {
		ev, ok := p.inst.Memory().Read(eventPtr, epollEventSize)
		if !ok {
			return negErrno(EFAULT)
		}
		events, data = binary.LittleEndian.Uint32(ev), binary.LittleEndian.Uint64(ev[8:])
	}

	// Apply the operation.
	switch op {
	case epollCtlAdd:
		_, _, readChange := of.readiness(false)
		_, _, writeChange := of.readiness(true)
		switch {
		case i >= 0:
			return negErrno(EEXIST)
		case readChange == nil && writeChange == nil:
			return negErrno(EPERM)
		}
		set.entries = append(set.entries, &epollEntry{fd: fd, of: of, events: events, data: data})
	case epollCtlMod:
		if i < 0 {
			return negErrno(ENOENT)
		}
		set.entries[i] = &epollEntry{fd: fd, of: of, events: events, data: data}
	case epollCtlDel:
		if i < 0 {
			return negErrno(ENOENT)
		}
		set.entries = append(set.entries[:i], set.entries[i+1:]...)
	default:
		return negErrno(EINVAL)
	}
	return 0
}

// sysEpollWait writes up to maxevents events of epoll file epfd to eventsPtr
// and returns their count. With no events it waits for an event, for timeout
// milliseconds unless timeout is negative. A caught signal ends the wait
// with EINTR.
func (p *Process) sysEpollWait(ctx context.Context, epfd, eventsPtr uint32, maxevents, timeout int32) uint64 {
	// Find the set.
	ep, ok := p.fds[epfd]
	switch {
	case !ok:
		return negErrno(EBADF)
	case ep.epoll == nil || maxevents <= 0:
		return negErrno(EINVAL)
	}

	// Start the clock.
	var timerCh <-chan time.Time
	if timeout > 0 {
		timer := time.NewTimer(time.Duration(timeout) * time.Millisecond)
		defer timer.Stop()
		timerCh = timer.C
	}

	// Wait until an event happens.
	ctx, release := p.interruptible(ctx)
	defer release()
	for {
		// Report the events that happened.
		events, changes := ep.epoll.poll(int(maxevents))
		if len(events) != 0 || timeout == 0 {
			return p.writeEpollEvents(eventsPtr, events)
		}

		// Wait for a change, the clock or a caught signal.
		switch waitAny(ctx, timerCh, changes) {
		case waitDone:
			return negErrno(EINTR)
		case waitTimer:
			return 0
		}
	}
}

// sysEpollPwait runs epoll_wait with the signal set at maskPtr as the
// signal mask, installed and restored with the wait as one step, so a signal
// it unblocks ends the wait. A zero maskPtr keeps the mask.
func (p *Process) sysEpollPwait(ctx context.Context, epfd, eventsPtr uint32, maxevents, timeout int32, maskPtr uint32) uint64 {
	if maskPtr != 0 {
		mask, ok := p.inst.Memory().ReadUint64Le(maskPtr)
		if !ok {
			return negErrno(EFAULT)
		}
		old, _ := p.swapSigMask(mask)
		defer p.restoreSigMask(old)
	}
	return p.sysEpollWait(ctx, epfd, eventsPtr, maxevents, timeout)
}

// writeEpollEvents writes events to eventsPtr as struct epoll_event and
// returns their count.
func (p *Process) writeEpollEvents(eventsPtr uint32, events []epollEvent) uint64 {
	buf := make([]byte, len(events)*epollEventSize)
	for i, ev := range events {
		binary.LittleEndian.PutUint32(buf[i*epollEventSize:], ev.events)
		binary.LittleEndian.PutUint64(buf[i*epollEventSize+8:], ev.data)
	}
	if !p.inst.Memory().Write(eventsPtr, buf) {
		return negErrno(EFAULT)
	}
	return uint64(len(events))
}
