// Package plan9fs implements a 9p2000.L filesystem server binding to
// hydra's FSCursor/FSHandle interfaces.
package plan9fs

// 9p2000.L message types.
const (
	// TLERROR is not sent by clients but used for RLERROR responses.
	TLERROR      = 6
	RLERROR      = 7
	TSTATFS      = 8
	RSTATFS      = 9
	TLOPEN       = 12
	RLOPEN       = 13
	TLCREATE     = 14
	RLCREATE     = 15
	TSYMLINK     = 16
	RSYMLINK     = 17
	TMKNOD       = 18
	RMKNOD       = 19
	TREADLINK    = 22
	RREADLINK    = 23
	TGETATTR     = 24
	RGETATTR     = 25
	TSETATTR     = 26
	RSETATTR     = 27
	TXATTRWALK   = 30
	RXATTRWALK   = 31
	TXATTRCREATE = 32
	RXATTRCREATE = 33
	TREADDIR     = 40
	RREADDIR     = 41
	TFSYNC       = 50
	RFSYNC       = 51
	TLOCK        = 52
	RLOCK        = 53
	TGETLOCK     = 54
	RGETLOCK     = 55
	TLINK        = 70
	RLINK        = 71
	TMKDIR       = 72
	RMKDIR       = 73
	TRENAMEAT    = 74
	RRENAMEAT    = 75
	TUNLINKAT    = 76
	RUNLINKAT    = 77
	TVERSION     = 100
	RVERSION     = 101
	TAUTH        = 102
	RAUTH        = 103
	TATTACH      = 104
	RATTACH      = 105
	TFLUSH       = 108
	RFLUSH       = 109
	TWALK        = 110
	RWALK        = 111
	TREAD        = 116
	RREAD        = 117
	TWRITE       = 118
	RWRITE       = 119
	TCLUNK       = 120
	RCLUNK       = 121
	TREMOVE      = 122
	RREMOVE      = 123
)

// QID type flags.
const (
	QidDir     = 0x80
	QidAppend  = 0x40
	QidExcl    = 0x20
	QidAuth    = 0x08
	QidTmpFile = 0x04
	QidSymlink = 0x02
	QidFile    = 0x00
)

// QID is a 13-byte unique file identifier.
type QID struct {
	Type    uint8
	Version uint32
	Path    uint64
}

// 9p2000.L open flags.
const (
	P9DotlRdonly = 0x00000
	P9DotlWronly = 0x00001
	P9DotlRdwr   = 0x00002
	P9DotlCreate = 0x00040
	P9DotlExcl   = 0x00080
	P9DotlNoctty = 0x00100
	P9DotlTrunc  = 0x00200
	P9DotlAppend = 0x00400
)

// GETATTR request mask bits.
const (
	GetattrMode        = 0x00000001
	GetattrNlink       = 0x00000002
	GetattrUid         = 0x00000004
	GetattrGid         = 0x00000008
	GetattrRdev        = 0x00000010
	GetattrAtime       = 0x00000020
	GetattrMtime       = 0x00000040
	GetattrCtime       = 0x00000080
	GetattrIno         = 0x00000100
	GetattrSize        = 0x00000200
	GetattrBlocks      = 0x00000400
	GetattrBtime       = 0x00000800
	GetattrGen         = 0x00001000
	GetattrDataVersion = 0x00002000
	GetattrAll         = 0x00003fff
	GetattrBasic       = 0x000007ff
)

// SETATTR request mask bits.
const (
	SetattrMode     = 0x00000001
	SetattrUid      = 0x00000002
	SetattrGid      = 0x00000004
	SetattrSize     = 0x00000008
	SetattrAtime    = 0x00000010
	SetattrMtime    = 0x00000020
	SetattrCtime    = 0x00000040
	SetattrAtimeSet = 0x00000100
	SetattrMtimeSet = 0x00000200
)

// LOCK types.
const (
	LockTypeRdlck = 0
	LockTypeWrlck = 1
	LockTypeUnlck = 2
)

// LOCK status codes.
const (
	LockSuccess = 0
	LockBlocked = 1
	LockError   = 2
	LockGrace   = 3
)

// UNLINKAT flags.
const (
	AtRemovedir = 0x200
)

// Linux errno values (own definitions for WASM compatibility).
const (
	EPERM     = 1
	ENOENT    = 2
	EINTR     = 4
	EIO       = 5
	EBADF     = 9
	ENOMEM    = 12
	EACCES    = 13
	EEXIST    = 17
	ENOTDIR   = 20
	EINVAL    = 22
	ENOSPC    = 28
	ERANGE    = 34
	ENOSYS    = 38
	ENOTEMPTY = 39
	ENOTSUP   = 95
	EROFS     = 30
)

// headerSize is the size of the 9p message header.
const headerSize = 7

// maxWalkNames is the maximum number of names in a TWALK.
const maxWalkNames = 16

// defaultMsize is the default maximum message size.
const defaultMsize = 65536

// versionString is the 9p2000.L protocol version.
const versionString = "9P2000.L"
