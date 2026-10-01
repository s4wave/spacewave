package link

// Path is how a link reaches its remote peer.
type Path int

const (
	// PathUnknown means the link does not report its path or has not chosen
	// one yet.
	PathUnknown Path = iota
	// PathDirect means packets travel between the two hosts without a relay.
	PathDirect
	// PathRelay means packets travel through a relay server, such as TURN.
	PathRelay
)

// String returns the path's lower-case name.
func (p Path) String() string {
	switch p {
	case PathDirect:
		return "direct"
	case PathRelay:
		return "relay"
	default:
		return "unknown"
	}
}

// PathLink is a Link or MountedLink that reports its current path.
type PathLink interface {
	// GetPath returns how the link reaches its remote peer now. The path can
	// change while the link is open.
	GetPath() Path
}

// GetPath returns the current path of l, or PathUnknown when l does not report
// one.
func GetPath(l any) Path {
	if pl, ok := l.(PathLink); ok {
		return pl.GetPath()
	}
	return PathUnknown
}
