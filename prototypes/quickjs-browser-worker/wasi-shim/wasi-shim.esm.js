/* eslint-disable */
// wasi_defs.ts
var FD_STDIN = 0;
var FD_STDOUT = 1;
var FD_STDERR = 2;
var CLOCKID_REALTIME = 0;
var CLOCKID_MONOTONIC = 1;
var CLOCKID_PROCESS_CPUTIME_ID = 2;
var CLOCKID_THREAD_CPUTIME_ID = 3;
var ERRNO_SUCCESS = 0;
var ERRNO_2BIG = 1;
var ERRNO_ACCES = 2;
var ERRNO_ADDRINUSE = 3;
var ERRNO_ADDRNOTAVAIL = 4;
var ERRNO_AFNOSUPPORT = 5;
var ERRNO_AGAIN = 6;
var ERRNO_ALREADY = 7;
var ERRNO_BADF = 8;
var ERRNO_BADMSG = 9;
var ERRNO_BUSY = 10;
var ERRNO_CANCELED = 11;
var ERRNO_CHILD = 12;
var ERRNO_CONNABORTED = 13;
var ERRNO_CONNREFUSED = 14;
var ERRNO_CONNRESET = 15;
var ERRNO_DEADLK = 16;
var ERRNO_DESTADDRREQ = 17;
var ERRNO_DOM = 18;
var ERRNO_DQUOT = 19;
var ERRNO_EXIST = 20;
var ERRNO_FAULT = 21;
var ERRNO_FBIG = 22;
var ERRNO_HOSTUNREACH = 23;
var ERRNO_IDRM = 24;
var ERRNO_ILSEQ = 25;
var ERRNO_INPROGRESS = 26;
var ERRNO_INTR = 27;
var ERRNO_INVAL = 28;
var ERRNO_IO = 29;
var ERRNO_ISCONN = 30;
var ERRNO_ISDIR = 31;
var ERRNO_LOOP = 32;
var ERRNO_MFILE = 33;
var ERRNO_MLINK = 34;
var ERRNO_MSGSIZE = 35;
var ERRNO_MULTIHOP = 36;
var ERRNO_NAMETOOLONG = 37;
var ERRNO_NETDOWN = 38;
var ERRNO_NETRESET = 39;
var ERRNO_NETUNREACH = 40;
var ERRNO_NFILE = 41;
var ERRNO_NOBUFS = 42;
var ERRNO_NODEV = 43;
var ERRNO_NOENT = 44;
var ERRNO_NOEXEC = 45;
var ERRNO_NOLCK = 46;
var ERRNO_NOLINK = 47;
var ERRNO_NOMEM = 48;
var ERRNO_NOMSG = 49;
var ERRNO_NOPROTOOPT = 50;
var ERRNO_NOSPC = 51;
var ERRNO_NOSYS = 52;
var ERRNO_NOTCONN = 53;
var ERRNO_NOTDIR = 54;
var ERRNO_NOTEMPTY = 55;
var ERRNO_NOTRECOVERABLE = 56;
var ERRNO_NOTSOCK = 57;
var ERRNO_NOTSUP = 58;
var ERRNO_NOTTY = 59;
var ERRNO_NXIO = 60;
var ERRNO_OVERFLOW = 61;
var ERRNO_OWNERDEAD = 62;
var ERRNO_PERM = 63;
var ERRNO_PIPE = 64;
var ERRNO_PROTO = 65;
var ERRNO_PROTONOSUPPORT = 66;
var ERRNO_PROTOTYPE = 67;
var ERRNO_RANGE = 68;
var ERRNO_ROFS = 69;
var ERRNO_SPIPE = 70;
var ERRNO_SRCH = 71;
var ERRNO_STALE = 72;
var ERRNO_TIMEDOUT = 73;
var ERRNO_TXTBSY = 74;
var ERRNO_XDEV = 75;
var ERRNO_NOTCAPABLE = 76;
var RIGHTS_FD_DATASYNC = 1 << 0;
var RIGHTS_FD_READ = 1 << 1;
var RIGHTS_FD_SEEK = 1 << 2;
var RIGHTS_FD_FDSTAT_SET_FLAGS = 1 << 3;
var RIGHTS_FD_SYNC = 1 << 4;
var RIGHTS_FD_TELL = 1 << 5;
var RIGHTS_FD_WRITE = 1 << 6;
var RIGHTS_FD_ADVISE = 1 << 7;
var RIGHTS_FD_ALLOCATE = 1 << 8;
var RIGHTS_PATH_CREATE_DIRECTORY = 1 << 9;
var RIGHTS_PATH_CREATE_FILE = 1 << 10;
var RIGHTS_PATH_LINK_SOURCE = 1 << 11;
var RIGHTS_PATH_LINK_TARGET = 1 << 12;
var RIGHTS_PATH_OPEN = 1 << 13;
var RIGHTS_FD_READDIR = 1 << 14;
var RIGHTS_PATH_READLINK = 1 << 15;
var RIGHTS_PATH_RENAME_SOURCE = 1 << 16;
var RIGHTS_PATH_RENAME_TARGET = 1 << 17;
var RIGHTS_PATH_FILESTAT_GET = 1 << 18;
var RIGHTS_PATH_FILESTAT_SET_SIZE = 1 << 19;
var RIGHTS_PATH_FILESTAT_SET_TIMES = 1 << 20;
var RIGHTS_FD_FILESTAT_GET = 1 << 21;
var RIGHTS_FD_FILESTAT_SET_SIZE = 1 << 22;
var RIGHTS_FD_FILESTAT_SET_TIMES = 1 << 23;
var RIGHTS_PATH_SYMLINK = 1 << 24;
var RIGHTS_PATH_REMOVE_DIRECTORY = 1 << 25;
var RIGHTS_PATH_UNLINK_FILE = 1 << 26;
var RIGHTS_POLL_FD_READWRITE = 1 << 27;
var RIGHTS_SOCK_SHUTDOWN = 1 << 28;
var Iovec = class _Iovec {
  buf;
  buf_len;
  static read_bytes(view, ptr) {
    const iovec = new _Iovec();
    iovec.buf = view.getUint32(ptr, true);
    iovec.buf_len = view.getUint32(ptr + 4, true);
    return iovec;
  }
  static read_bytes_array(view, ptr, len) {
    const iovecs = [];
    for (let i = 0; i < len; i++) {
      iovecs.push(_Iovec.read_bytes(view, ptr + 8 * i));
    }
    return iovecs;
  }
};
var Ciovec = class _Ciovec {
  buf;
  buf_len;
  static read_bytes(view, ptr) {
    const iovec = new _Ciovec();
    iovec.buf = view.getUint32(ptr, true);
    iovec.buf_len = view.getUint32(ptr + 4, true);
    return iovec;
  }
  static read_bytes_array(view, ptr, len) {
    const iovecs = [];
    for (let i = 0; i < len; i++) {
      iovecs.push(_Ciovec.read_bytes(view, ptr + 8 * i));
    }
    return iovecs;
  }
};
var WHENCE_SET = 0;
var WHENCE_CUR = 1;
var WHENCE_END = 2;
var FILETYPE_UNKNOWN = 0;
var FILETYPE_BLOCK_DEVICE = 1;
var FILETYPE_CHARACTER_DEVICE = 2;
var FILETYPE_DIRECTORY = 3;
var FILETYPE_REGULAR_FILE = 4;
var FILETYPE_SOCKET_DGRAM = 5;
var FILETYPE_SOCKET_STREAM = 6;
var FILETYPE_SYMBOLIC_LINK = 7;
var Dirent = class {
  d_next;
  d_ino;
  d_namlen;
  d_type;
  dir_name;
  constructor(next_cookie, d_ino, name, type) {
    const encoded_name = new TextEncoder().encode(name);
    this.d_next = next_cookie;
    this.d_ino = d_ino;
    this.d_namlen = encoded_name.byteLength;
    this.d_type = type;
    this.dir_name = encoded_name;
  }
  head_length() {
    return 24;
  }
  name_length() {
    return this.dir_name.byteLength;
  }
  write_head_bytes(view, ptr) {
    view.setBigUint64(ptr, this.d_next, true);
    view.setBigUint64(ptr + 8, this.d_ino, true);
    view.setUint32(ptr + 16, this.dir_name.length, true);
    view.setUint8(ptr + 20, this.d_type);
  }
  write_name_bytes(view8, ptr, buf_len) {
    view8.set(
      this.dir_name.slice(0, Math.min(this.dir_name.byteLength, buf_len)),
      ptr
    );
  }
};
var ADVICE_NORMAL = 0;
var ADVICE_SEQUENTIAL = 1;
var ADVICE_RANDOM = 2;
var ADVICE_WILLNEED = 3;
var ADVICE_DONTNEED = 4;
var ADVICE_NOREUSE = 5;
var FDFLAGS_APPEND = 1 << 0;
var FDFLAGS_DSYNC = 1 << 1;
var FDFLAGS_NONBLOCK = 1 << 2;
var FDFLAGS_RSYNC = 1 << 3;
var FDFLAGS_SYNC = 1 << 4;
var Fdstat = class {
  fs_filetype;
  fs_flags;
  fs_rights_base = 0n;
  fs_rights_inherited = 0n;
  constructor(filetype, flags) {
    this.fs_filetype = filetype;
    this.fs_flags = flags;
  }
  write_bytes(view, ptr) {
    view.setUint8(ptr, this.fs_filetype);
    view.setUint16(ptr + 2, this.fs_flags, true);
    view.setBigUint64(ptr + 8, this.fs_rights_base, true);
    view.setBigUint64(ptr + 16, this.fs_rights_inherited, true);
  }
};
var FSTFLAGS_ATIM = 1 << 0;
var FSTFLAGS_ATIM_NOW = 1 << 1;
var FSTFLAGS_MTIM = 1 << 2;
var FSTFLAGS_MTIM_NOW = 1 << 3;
var OFLAGS_CREAT = 1 << 0;
var OFLAGS_DIRECTORY = 1 << 1;
var OFLAGS_EXCL = 1 << 2;
var OFLAGS_TRUNC = 1 << 3;
var Filestat = class {
  dev = 0n;
  ino;
  filetype;
  nlink = 0n;
  size;
  atim = 0n;
  mtim = 0n;
  ctim = 0n;
  constructor(ino, filetype, size) {
    this.ino = ino;
    this.filetype = filetype;
    this.size = size;
  }
  write_bytes(view, ptr) {
    view.setBigUint64(ptr, this.dev, true);
    view.setBigUint64(ptr + 8, this.ino, true);
    view.setUint8(ptr + 16, this.filetype);
    view.setBigUint64(ptr + 24, this.nlink, true);
    view.setBigUint64(ptr + 32, this.size, true);
    view.setBigUint64(ptr + 40, this.atim, true);
    view.setBigUint64(ptr + 48, this.mtim, true);
    view.setBigUint64(ptr + 56, this.ctim, true);
  }
};
var EVENTTYPE_CLOCK = 0;
var EVENTTYPE_FD_READ = 1;
var EVENTTYPE_FD_WRITE = 2;
var EVENTRWFLAGS_FD_READWRITE_HANGUP = 1 << 0;
var SUBCLOCKFLAGS_SUBSCRIPTION_CLOCK_ABSTIME = 1 << 0;
var Subscription = class _Subscription {
  constructor(userdata, eventtype, clockid, timeout, precision, flags, fd) {
    this.userdata = userdata;
    this.eventtype = eventtype;
    this.clockid = clockid;
    this.timeout = timeout;
    this.precision = precision;
    this.flags = flags;
    this.fd = fd;
  }
  static read_bytes(view, ptr) {
    const userdata = view.getBigUint64(ptr, true);
    const eventtype = view.getUint8(ptr + 8);
    if (eventtype === EVENTTYPE_CLOCK) {
      return new _Subscription(
        userdata,
        eventtype,
        view.getUint32(ptr + 16, true),
        // clockid
        view.getBigUint64(ptr + 24, true),
        // timeout
        view.getBigUint64(ptr + 32, true),
        // precision
        view.getUint16(ptr + 40, true),
        // flags
        0
        // fd (not used)
      );
    }
    return new _Subscription(
      userdata,
      eventtype,
      0,
      // clockid (not used)
      0n,
      // timeout (not used)
      0n,
      // precision (not used)
      0,
      // flags (not used)
      view.getUint32(ptr + 16, true)
      // fd
    );
  }
  static size() {
    return 48;
  }
};
var Event = class {
  constructor(userdata, error, eventtype, nbytes = 0n, rwflags = 0) {
    this.userdata = userdata;
    this.error = error;
    this.eventtype = eventtype;
    this.nbytes = nbytes;
    this.rwflags = rwflags;
  }
  write_bytes(view, ptr) {
    view.setBigUint64(ptr, this.userdata, true);
    view.setUint16(ptr + 8, this.error, true);
    view.setUint8(ptr + 10, this.eventtype);
    if (this.eventtype === EVENTTYPE_FD_READ || this.eventtype === EVENTTYPE_FD_WRITE) {
      view.setBigUint64(ptr + 16, this.nbytes, true);
      view.setUint16(ptr + 24, this.rwflags, true);
    }
  }
  static size() {
    return 32;
  }
};
var SIGNAL_NONE = 0;
var SIGNAL_HUP = 1;
var SIGNAL_INT = 2;
var SIGNAL_QUIT = 3;
var SIGNAL_ILL = 4;
var SIGNAL_TRAP = 5;
var SIGNAL_ABRT = 6;
var SIGNAL_BUS = 7;
var SIGNAL_FPE = 8;
var SIGNAL_KILL = 9;
var SIGNAL_USR1 = 10;
var SIGNAL_SEGV = 11;
var SIGNAL_USR2 = 12;
var SIGNAL_PIPE = 13;
var SIGNAL_ALRM = 14;
var SIGNAL_TERM = 15;
var SIGNAL_CHLD = 16;
var SIGNAL_CONT = 17;
var SIGNAL_STOP = 18;
var SIGNAL_TSTP = 19;
var SIGNAL_TTIN = 20;
var SIGNAL_TTOU = 21;
var SIGNAL_URG = 22;
var SIGNAL_XCPU = 23;
var SIGNAL_XFSZ = 24;
var SIGNAL_VTALRM = 25;
var SIGNAL_PROF = 26;
var SIGNAL_WINCH = 27;
var SIGNAL_POLL = 28;
var SIGNAL_PWR = 29;
var SIGNAL_SYS = 30;
var RIFLAGS_RECV_PEEK = 1 << 0;
var RIFLAGS_RECV_WAITALL = 1 << 1;
var ROFLAGS_RECV_DATA_TRUNCATED = 1 << 0;
var SDFLAGS_RD = 1 << 0;
var SDFLAGS_WR = 1 << 1;
var PREOPENTYPE_DIR = 0;
var PrestatDir = class {
  pr_name;
  constructor(name) {
    this.pr_name = new TextEncoder().encode(name);
  }
  write_bytes(view, ptr) {
    view.setUint32(ptr, this.pr_name.byteLength, true);
  }
};
var Prestat = class _Prestat {
  tag;
  inner;
  static dir(name) {
    const prestat = new _Prestat();
    prestat.tag = PREOPENTYPE_DIR;
    prestat.inner = new PrestatDir(name);
    return prestat;
  }
  write_bytes(view, ptr) {
    view.setUint32(ptr, this.tag, true);
    this.inner.write_bytes(view, ptr + 4);
  }
};

// fd.ts
var Fd = class {
  fd_allocate(_offset, _len) {
    return ERRNO_NOTSUP;
  }
  fd_close() {
    return 0;
  }
  fd_fdstat_get() {
    return { ret: ERRNO_NOTSUP, fdstat: null };
  }
  fd_fdstat_set_flags(_flags) {
    return ERRNO_NOTSUP;
  }
  fd_fdstat_set_rights(_fs_rights_base, _fs_rights_inheriting) {
    return ERRNO_NOTSUP;
  }
  fd_filestat_get() {
    return { ret: ERRNO_NOTSUP, filestat: null };
  }
  fd_filestat_set_size(_size) {
    return ERRNO_NOTSUP;
  }
  fd_filestat_set_times(_atim, _mtim, _fst_flags) {
    return ERRNO_NOTSUP;
  }
  fd_pread(_size, _offset) {
    return { ret: ERRNO_NOTSUP, data: new Uint8Array() };
  }
  fd_prestat_get() {
    return { ret: ERRNO_NOTSUP, prestat: null };
  }
  fd_pwrite(_data, _offset) {
    return { ret: ERRNO_NOTSUP, nwritten: 0 };
  }
  fd_read(_size) {
    return { ret: ERRNO_NOTSUP, data: new Uint8Array() };
  }
  fd_readdir_single(_cookie) {
    return { ret: ERRNO_NOTSUP, dirent: null };
  }
  fd_seek(_offset, _whence) {
    return { ret: ERRNO_NOTSUP, offset: 0n };
  }
  fd_sync() {
    return 0;
  }
  fd_tell() {
    return { ret: ERRNO_NOTSUP, offset: 0n };
  }
  fd_write(_data) {
    return { ret: ERRNO_NOTSUP, nwritten: 0 };
  }
  path_create_directory(_path) {
    return ERRNO_NOTSUP;
  }
  path_filestat_get(_flags, _path) {
    return { ret: ERRNO_NOTSUP, filestat: null };
  }
  path_filestat_set_times(_flags, _path, _atim, _mtim, _fst_flags) {
    return ERRNO_NOTSUP;
  }
  path_link(_path, _inode, _allow_dir) {
    return ERRNO_NOTSUP;
  }
  path_unlink(_path) {
    return { ret: ERRNO_NOTSUP, inode_obj: null };
  }
  path_lookup(_path, _dirflags) {
    return { ret: ERRNO_NOTSUP, inode_obj: null };
  }
  path_open(_dirflags, _path, _oflags, _fs_rights_base, _fs_rights_inheriting, _fd_flags) {
    return { ret: ERRNO_NOTDIR, fd_obj: null };
  }
  path_readlink(_path) {
    return { ret: ERRNO_NOTSUP, data: null };
  }
  path_remove_directory(_path) {
    return ERRNO_NOTSUP;
  }
  path_rename(_old_path, _new_fd, _new_path) {
    return ERRNO_NOTSUP;
  }
  path_unlink_file(_path) {
    return ERRNO_NOTSUP;
  }
  /**
   * Poll for I/O readiness. Override this in subclasses that support polling.
   * @param eventtype EVENTTYPE_FD_READ or EVENTTYPE_FD_WRITE
   * @returns PollResult indicating readiness
   */
  fd_poll(_eventtype) {
    return { ready: false, nbytes: 0n, flags: 0 };
  }
};
var Inode = class _Inode {
  ino;
  constructor() {
    this.ino = _Inode.issue_ino();
  }
  // NOTE: ino 0 is reserved for the root directory
  static next_ino = 1n;
  static issue_ino() {
    return _Inode.next_ino++;
  }
  static root_ino() {
    return 0n;
  }
};

// wasi.ts
var WASIProcExit = class extends Error {
  constructor(code) {
    super("exit with exit code " + code);
    this.code = code;
  }
};
var WASI = class {
  #freeFds = [];
  args = [];
  env = [];
  fds = [];
  inst;
  debug = false;
  wasiImport;
  /// Start a WASI command
  start(instance) {
    this.inst = instance;
    try {
      instance.exports._start();
      return 0;
    } catch (e) {
      if (e instanceof WASIProcExit) {
        return e.code;
      }
      throw e;
    }
  }
  /// Initialize a WASI reactor
  initialize(instance) {
    this.inst = instance;
    if (instance.exports._initialize) {
      instance.exports._initialize();
    }
  }
  log(...args) {
    if (this.debug) {
      console.log("[WASI]", ...args);
    }
  }
  constructor(args, env, fds, options = {}) {
    this.debug = options.debug ?? false;
    this.args = args;
    this.env = env;
    this.fds = fds;
    const self = this;
    this.wasiImport = {
      args_sizes_get(argc, argv_buf_size) {
        const buffer = new DataView(self.inst.exports.memory.buffer);
        buffer.setUint32(argc, self.args.length, true);
        let buf_size = 0;
        for (const arg of self.args) {
          buf_size += arg.length + 1;
        }
        buffer.setUint32(argv_buf_size, buf_size, true);
        self.log("args_sizes_get", self.args.length, buf_size);
        return 0;
      },
      args_get(argv, argv_buf) {
        const buffer = new DataView(self.inst.exports.memory.buffer);
        const buffer8 = new Uint8Array(self.inst.exports.memory.buffer);
        for (let i = 0; i < self.args.length; i++) {
          buffer.setUint32(argv, argv_buf, true);
          argv += 4;
          const arg = new TextEncoder().encode(self.args[i]);
          buffer8.set(arg, argv_buf);
          buffer.setUint8(argv_buf + arg.length, 0);
          argv_buf += arg.length + 1;
        }
        return 0;
      },
      environ_sizes_get(environ_count, environ_size) {
        const buffer = new DataView(self.inst.exports.memory.buffer);
        buffer.setUint32(environ_count, self.env.length, true);
        let buf_size = 0;
        for (const environ of self.env) {
          buf_size += new TextEncoder().encode(environ).length + 1;
        }
        buffer.setUint32(environ_size, buf_size, true);
        return 0;
      },
      environ_get(environ, environ_buf) {
        const buffer = new DataView(self.inst.exports.memory.buffer);
        const buffer8 = new Uint8Array(self.inst.exports.memory.buffer);
        for (let i = 0; i < self.env.length; i++) {
          buffer.setUint32(environ, environ_buf, true);
          environ += 4;
          const e = new TextEncoder().encode(self.env[i]);
          buffer8.set(e, environ_buf);
          buffer.setUint8(environ_buf + e.length, 0);
          environ_buf += e.length + 1;
        }
        return 0;
      },
      clock_res_get(id, res_ptr) {
        let resolutionValue;
        switch (id) {
          case CLOCKID_MONOTONIC: {
            resolutionValue = 5000n;
            break;
          }
          case CLOCKID_REALTIME: {
            resolutionValue = 1000000n;
            break;
          }
          default:
            return ERRNO_NOSYS;
        }
        const view = new DataView(self.inst.exports.memory.buffer);
        view.setBigUint64(res_ptr, resolutionValue, true);
        return ERRNO_SUCCESS;
      },
      clock_time_get(id, _precision, time) {
        const buffer = new DataView(self.inst.exports.memory.buffer);
        if (id === CLOCKID_REALTIME) {
          buffer.setBigUint64(time, BigInt((/* @__PURE__ */ new Date()).getTime()) * 1000000n, true);
        } else if (id == CLOCKID_MONOTONIC) {
          let monotonic_time;
          try {
            monotonic_time = BigInt(Math.round(performance.now() * 1e6));
          } catch {
            monotonic_time = 0n;
          }
          buffer.setBigUint64(time, monotonic_time, true);
        } else {
          buffer.setBigUint64(time, 0n, true);
        }
        return 0;
      },
      fd_advise(fd, _offset, _len, _advice) {
        if (self.fds[fd] != void 0) {
          return ERRNO_SUCCESS;
        }
        return ERRNO_BADF;
      },
      fd_allocate(fd, offset, len) {
        if (self.fds[fd] != void 0) {
          return self.fds[fd].fd_allocate(offset, len);
        }
        return ERRNO_BADF;
      },
      fd_close(fd) {
        if (self.fds[fd] != void 0) {
          const ret = self.fds[fd].fd_close();
          self.fds[fd] = void 0;
          self.#freeFds.push(fd);
          return ret;
        }
        return ERRNO_BADF;
      },
      fd_datasync(fd) {
        if (self.fds[fd] != void 0) {
          return self.fds[fd].fd_sync();
        }
        return ERRNO_BADF;
      },
      fd_fdstat_get(fd, fdstat_ptr) {
        if (self.fds[fd] != void 0) {
          const { ret, fdstat } = self.fds[fd].fd_fdstat_get();
          if (fdstat != null) {
            fdstat.write_bytes(new DataView(self.inst.exports.memory.buffer), fdstat_ptr);
          }
          return ret;
        }
        return ERRNO_BADF;
      },
      fd_fdstat_set_flags(fd, flags) {
        if (self.fds[fd] != void 0) {
          return self.fds[fd].fd_fdstat_set_flags(flags);
        }
        return ERRNO_BADF;
      },
      fd_fdstat_set_rights(fd, fs_rights_base, fs_rights_inheriting) {
        if (self.fds[fd] != void 0) {
          return self.fds[fd].fd_fdstat_set_rights(fs_rights_base, fs_rights_inheriting);
        }
        return ERRNO_BADF;
      },
      fd_filestat_get(fd, filestat_ptr) {
        if (self.fds[fd] != void 0) {
          const { ret, filestat } = self.fds[fd].fd_filestat_get();
          if (filestat != null) {
            filestat.write_bytes(new DataView(self.inst.exports.memory.buffer), filestat_ptr);
          }
          return ret;
        }
        return ERRNO_BADF;
      },
      fd_filestat_set_size(fd, size) {
        if (self.fds[fd] != void 0) {
          return self.fds[fd].fd_filestat_set_size(size);
        }
        return ERRNO_BADF;
      },
      fd_filestat_set_times(fd, atim, mtim, fst_flags) {
        if (self.fds[fd] != void 0) {
          return self.fds[fd].fd_filestat_set_times(atim, mtim, fst_flags);
        }
        return ERRNO_BADF;
      },
      fd_pread(fd, iovs_ptr, iovs_len, offset, nread_ptr) {
        const buffer = new DataView(self.inst.exports.memory.buffer);
        const buffer8 = new Uint8Array(self.inst.exports.memory.buffer);
        if (self.fds[fd] != void 0) {
          const iovecs = Iovec.read_bytes_array(buffer, iovs_ptr, iovs_len);
          let nread = 0;
          for (const iovec of iovecs) {
            const { ret, data } = self.fds[fd].fd_pread(iovec.buf_len, offset);
            if (ret != ERRNO_SUCCESS) {
              buffer.setUint32(nread_ptr, nread, true);
              return ret;
            }
            buffer8.set(data, iovec.buf);
            nread += data.length;
            offset += BigInt(data.length);
            if (data.length != iovec.buf_len) {
              break;
            }
          }
          buffer.setUint32(nread_ptr, nread, true);
          return ERRNO_SUCCESS;
        }
        return ERRNO_BADF;
      },
      fd_prestat_get(fd, buf_ptr) {
        const buffer = new DataView(self.inst.exports.memory.buffer);
        if (self.fds[fd] != void 0) {
          const { ret, prestat } = self.fds[fd].fd_prestat_get();
          if (prestat != null) {
            prestat.write_bytes(buffer, buf_ptr);
          }
          return ret;
        }
        return ERRNO_BADF;
      },
      fd_prestat_dir_name(fd, path_ptr, path_len) {
        if (self.fds[fd] != void 0) {
          const { ret, prestat } = self.fds[fd].fd_prestat_get();
          if (prestat == null) {
            return ret;
          }
          const prestat_dir_name = prestat.inner.pr_name;
          const buffer8 = new Uint8Array(self.inst.exports.memory.buffer);
          buffer8.set(prestat_dir_name.slice(0, path_len), path_ptr);
          return prestat_dir_name.byteLength > path_len ? ERRNO_NAMETOOLONG : ERRNO_SUCCESS;
        }
        return ERRNO_BADF;
      },
      fd_pwrite(fd, iovs_ptr, iovs_len, offset, nwritten_ptr) {
        const buffer = new DataView(self.inst.exports.memory.buffer);
        const buffer8 = new Uint8Array(self.inst.exports.memory.buffer);
        if (self.fds[fd] != void 0) {
          const iovecs = Ciovec.read_bytes_array(buffer, iovs_ptr, iovs_len);
          let nwritten = 0;
          for (const iovec of iovecs) {
            const data = buffer8.slice(iovec.buf, iovec.buf + iovec.buf_len);
            const { ret, nwritten: nwritten_part } = self.fds[fd].fd_pwrite(data, offset);
            if (ret != ERRNO_SUCCESS) {
              buffer.setUint32(nwritten_ptr, nwritten, true);
              return ret;
            }
            nwritten += nwritten_part;
            offset += BigInt(nwritten_part);
            if (nwritten_part != data.byteLength) {
              break;
            }
          }
          buffer.setUint32(nwritten_ptr, nwritten, true);
          return ERRNO_SUCCESS;
        }
        return ERRNO_BADF;
      },
      fd_read(fd, iovs_ptr, iovs_len, nread_ptr) {
        const buffer = new DataView(self.inst.exports.memory.buffer);
        const buffer8 = new Uint8Array(self.inst.exports.memory.buffer);
        if (self.fds[fd] != void 0) {
          const iovecs = Iovec.read_bytes_array(buffer, iovs_ptr, iovs_len);
          let nread = 0;
          for (const iovec of iovecs) {
            const { ret, data } = self.fds[fd].fd_read(iovec.buf_len);
            if (ret != ERRNO_SUCCESS) {
              buffer.setUint32(nread_ptr, nread, true);
              return ret;
            }
            buffer8.set(data, iovec.buf);
            nread += data.length;
            if (data.length != iovec.buf_len) {
              break;
            }
          }
          buffer.setUint32(nread_ptr, nread, true);
          return ERRNO_SUCCESS;
        }
        return ERRNO_BADF;
      },
      fd_readdir(fd, buf, buf_len, cookie, bufused_ptr) {
        const buffer = new DataView(self.inst.exports.memory.buffer);
        const buffer8 = new Uint8Array(self.inst.exports.memory.buffer);
        if (self.fds[fd] != void 0) {
          let bufused = 0;
          for (; ; ) {
            const { ret, dirent } = self.fds[fd].fd_readdir_single(cookie);
            if (ret != 0) {
              buffer.setUint32(bufused_ptr, bufused, true);
              return ret;
            }
            if (dirent == null) {
              break;
            }
            if (buf_len - bufused < dirent.head_length()) {
              bufused = buf_len;
              break;
            }
            const head_bytes = new ArrayBuffer(dirent.head_length());
            dirent.write_head_bytes(new DataView(head_bytes), 0);
            buffer8.set(
              new Uint8Array(head_bytes).slice(0, Math.min(head_bytes.byteLength, buf_len - bufused)),
              buf
            );
            buf += dirent.head_length();
            bufused += dirent.head_length();
            if (buf_len - bufused < dirent.name_length()) {
              bufused = buf_len;
              break;
            }
            dirent.write_name_bytes(buffer8, buf, buf_len - bufused);
            buf += dirent.name_length();
            bufused += dirent.name_length();
            cookie = dirent.d_next;
          }
          buffer.setUint32(bufused_ptr, bufused, true);
          return 0;
        }
        return ERRNO_BADF;
      },
      fd_renumber(fd, to) {
        if (self.fds[fd] != void 0 && self.fds[to] != void 0) {
          const ret = self.fds[to].fd_close();
          if (ret != 0) {
            return ret;
          }
          self.fds[to] = self.fds[fd];
          self.fds[fd] = void 0;
          self.#freeFds.push(fd);
          return 0;
        }
        return ERRNO_BADF;
      },
      fd_seek(fd, offset, whence, offset_out_ptr) {
        const buffer = new DataView(self.inst.exports.memory.buffer);
        if (self.fds[fd] != void 0) {
          const { ret, offset: offset_out } = self.fds[fd].fd_seek(offset, whence);
          buffer.setBigInt64(offset_out_ptr, offset_out, true);
          return ret;
        }
        return ERRNO_BADF;
      },
      fd_sync(fd) {
        if (self.fds[fd] != void 0) {
          return self.fds[fd].fd_sync();
        }
        return ERRNO_BADF;
      },
      fd_tell(fd, offset_ptr) {
        const buffer = new DataView(self.inst.exports.memory.buffer);
        if (self.fds[fd] != void 0) {
          const { ret, offset } = self.fds[fd].fd_tell();
          buffer.setBigUint64(offset_ptr, offset, true);
          return ret;
        }
        return ERRNO_BADF;
      },
      fd_write(fd, iovs_ptr, iovs_len, nwritten_ptr) {
        const buffer = new DataView(self.inst.exports.memory.buffer);
        const buffer8 = new Uint8Array(self.inst.exports.memory.buffer);
        if (self.fds[fd] != void 0) {
          const iovecs = Ciovec.read_bytes_array(buffer, iovs_ptr, iovs_len);
          let nwritten = 0;
          for (const iovec of iovecs) {
            const data = buffer8.slice(iovec.buf, iovec.buf + iovec.buf_len);
            const { ret, nwritten: nwritten_part } = self.fds[fd].fd_write(data);
            if (ret != ERRNO_SUCCESS) {
              buffer.setUint32(nwritten_ptr, nwritten, true);
              return ret;
            }
            nwritten += nwritten_part;
            if (nwritten_part != data.byteLength) {
              break;
            }
          }
          buffer.setUint32(nwritten_ptr, nwritten, true);
          return ERRNO_SUCCESS;
        }
        return ERRNO_BADF;
      },
      path_create_directory(fd, path_ptr, path_len) {
        const buffer8 = new Uint8Array(self.inst.exports.memory.buffer);
        if (self.fds[fd] != void 0) {
          const path = new TextDecoder("utf-8").decode(buffer8.slice(path_ptr, path_ptr + path_len));
          return self.fds[fd].path_create_directory(path);
        }
        return ERRNO_BADF;
      },
      path_filestat_get(fd, flags, path_ptr, path_len, filestat_ptr) {
        const buffer = new DataView(self.inst.exports.memory.buffer);
        const buffer8 = new Uint8Array(self.inst.exports.memory.buffer);
        if (self.fds[fd] != void 0) {
          const path = new TextDecoder("utf-8").decode(buffer8.slice(path_ptr, path_ptr + path_len));
          const { ret, filestat } = self.fds[fd].path_filestat_get(flags, path);
          if (filestat != null) {
            filestat.write_bytes(buffer, filestat_ptr);
          }
          return ret;
        }
        return ERRNO_BADF;
      },
      path_filestat_set_times(fd, flags, path_ptr, path_len, atim, mtim, fst_flags) {
        const buffer8 = new Uint8Array(self.inst.exports.memory.buffer);
        if (self.fds[fd] != void 0) {
          const path = new TextDecoder("utf-8").decode(buffer8.slice(path_ptr, path_ptr + path_len));
          return self.fds[fd].path_filestat_set_times(flags, path, atim, mtim, fst_flags);
        }
        return ERRNO_BADF;
      },
      path_link(old_fd, old_flags, old_path_ptr, old_path_len, new_fd, new_path_ptr, new_path_len) {
        const buffer8 = new Uint8Array(self.inst.exports.memory.buffer);
        if (self.fds[old_fd] != void 0 && self.fds[new_fd] != void 0) {
          const old_path = new TextDecoder("utf-8").decode(buffer8.slice(old_path_ptr, old_path_ptr + old_path_len));
          const new_path = new TextDecoder("utf-8").decode(buffer8.slice(new_path_ptr, new_path_ptr + new_path_len));
          const { ret, inode_obj } = self.fds[old_fd].path_lookup(old_path, old_flags);
          if (inode_obj == null) {
            return ret;
          }
          return self.fds[new_fd].path_link(new_path, inode_obj, false);
        }
        return ERRNO_BADF;
      },
      path_open(fd, dirflags, path_ptr, path_len, oflags, fs_rights_base, fs_rights_inheriting, fd_flags, opened_fd_ptr) {
        const buffer = new DataView(self.inst.exports.memory.buffer);
        const buffer8 = new Uint8Array(self.inst.exports.memory.buffer);
        if (self.fds[fd] != void 0) {
          const path = new TextDecoder("utf-8").decode(buffer8.slice(path_ptr, path_ptr + path_len));
          self.log("path_open", path);
          const { ret, fd_obj } = self.fds[fd].path_open(
            dirflags,
            path,
            oflags,
            fs_rights_base,
            fs_rights_inheriting,
            fd_flags
          );
          if (ret != 0) {
            return ret;
          }
          const opened_fd = (() => {
            if (self.#freeFds.length > 0) {
              const fd2 = self.#freeFds.pop();
              self.fds[fd2] = fd_obj;
              return fd2;
            }
            self.fds.push(fd_obj);
            return self.fds.length - 1;
          })();
          buffer.setUint32(opened_fd_ptr, opened_fd, true);
          return 0;
        }
        return ERRNO_BADF;
      },
      path_readlink(fd, path_ptr, path_len, buf_ptr, buf_len, nread_ptr) {
        const buffer = new DataView(self.inst.exports.memory.buffer);
        const buffer8 = new Uint8Array(self.inst.exports.memory.buffer);
        if (self.fds[fd] != void 0) {
          const path = new TextDecoder("utf-8").decode(buffer8.slice(path_ptr, path_ptr + path_len));
          const { ret, data } = self.fds[fd].path_readlink(path);
          if (data != null) {
            const data_buf = new TextEncoder().encode(data);
            if (data_buf.length > buf_len) {
              buffer.setUint32(nread_ptr, 0, true);
              return ERRNO_BADF;
            }
            buffer8.set(data_buf, buf_ptr);
            buffer.setUint32(nread_ptr, data_buf.length, true);
          }
          return ret;
        }
        return ERRNO_BADF;
      },
      path_remove_directory(fd, path_ptr, path_len) {
        const buffer8 = new Uint8Array(self.inst.exports.memory.buffer);
        if (self.fds[fd] != void 0) {
          const path = new TextDecoder("utf-8").decode(buffer8.slice(path_ptr, path_ptr + path_len));
          return self.fds[fd].path_remove_directory(path);
        }
        return ERRNO_BADF;
      },
      path_rename(fd, old_path_ptr, old_path_len, new_fd, new_path_ptr, new_path_len) {
        const buffer8 = new Uint8Array(self.inst.exports.memory.buffer);
        if (self.fds[fd] != void 0 && self.fds[new_fd] != void 0) {
          const old_path = new TextDecoder("utf-8").decode(buffer8.slice(old_path_ptr, old_path_ptr + old_path_len));
          const new_path = new TextDecoder("utf-8").decode(buffer8.slice(new_path_ptr, new_path_ptr + new_path_len));
          let { ret, inode_obj } = self.fds[fd].path_unlink(old_path);
          if (inode_obj == null) {
            return ret;
          }
          ret = self.fds[new_fd].path_link(new_path, inode_obj, true);
          if (ret != ERRNO_SUCCESS) {
            if (self.fds[fd].path_link(old_path, inode_obj, true) != ERRNO_SUCCESS) {
              throw "path_link should always return success when relinking an inode back to the original place";
            }
          }
          return ret;
        }
        return ERRNO_BADF;
      },
      path_symlink(old_path_ptr, old_path_len, fd, _new_path_ptr, _new_path_len) {
        const buffer8 = new Uint8Array(self.inst.exports.memory.buffer);
        if (self.fds[fd] != void 0) {
          new TextDecoder("utf-8").decode(buffer8.slice(old_path_ptr, old_path_ptr + old_path_len));
          return ERRNO_NOTSUP;
        }
        return ERRNO_BADF;
      },
      path_unlink_file(fd, path_ptr, path_len) {
        const buffer8 = new Uint8Array(self.inst.exports.memory.buffer);
        if (self.fds[fd] != void 0) {
          const path = new TextDecoder("utf-8").decode(buffer8.slice(path_ptr, path_ptr + path_len));
          return self.fds[fd].path_unlink_file(path);
        }
        return ERRNO_BADF;
      },
      /**
       * poll_oneoff - Poll for events on multiple subscriptions.
       *
       * This implementation supports:
       * - Clock subscriptions (sleep)
       * - FD_READ subscriptions (poll for readable data)
       * - FD_WRITE subscriptions (poll for writeable)
       *
       * For fd subscriptions, we call fd_poll() on the Fd object.
       */
      poll_oneoff(in_ptr, out_ptr, nsubscriptions, nevents_ptr) {
        if (nsubscriptions === 0) {
          return ERRNO_INVAL;
        }
        const buffer = new DataView(self.inst.exports.memory.buffer);
        const subscriptions = [];
        for (let i = 0; i < nsubscriptions; i++) {
          subscriptions.push(Subscription.read_bytes(buffer, in_ptr + i * Subscription.size()));
        }
        self.log("poll_oneoff: subscriptions", subscriptions);
        const events = [];
        let clockTimeout = null;
        let clockUserdata = 0n;
        for (const s of subscriptions) {
          if (s.eventtype === EVENTTYPE_CLOCK) {
            let getNow;
            if (s.clockid === CLOCKID_MONOTONIC) {
              getNow = () => BigInt(Math.round(performance.now() * 1e6));
            } else if (s.clockid === CLOCKID_REALTIME) {
              getNow = () => BigInt((/* @__PURE__ */ new Date()).getTime()) * 1000000n;
            } else {
              events.push(new Event(s.userdata, ERRNO_INVAL, s.eventtype));
              continue;
            }
            const endTime = (s.flags & SUBCLOCKFLAGS_SUBSCRIPTION_CLOCK_ABSTIME) !== 0 ? s.timeout : getNow() + s.timeout;
            if (clockTimeout === null || endTime < clockTimeout) {
              clockTimeout = endTime;
              clockUserdata = s.userdata;
            }
          } else if (s.eventtype === EVENTTYPE_FD_READ || s.eventtype === EVENTTYPE_FD_WRITE) {
            if (self.fds[s.fd] == void 0) {
              events.push(new Event(s.userdata, ERRNO_BADF, s.eventtype));
              continue;
            }
            const pollResult = self.fds[s.fd].fd_poll(s.eventtype);
            self.log("poll_oneoff: fd", s.fd, "poll result", pollResult);
            if (pollResult.ready) {
              events.push(new Event(s.userdata, ERRNO_SUCCESS, s.eventtype, pollResult.nbytes, pollResult.flags));
            }
          } else {
            events.push(new Event(s.userdata, ERRNO_NOTSUP, s.eventtype));
          }
        }
        if (events.length === 0 && clockTimeout !== null) {
          const getNow = subscriptions.find((s) => s.eventtype === EVENTTYPE_CLOCK)?.clockid === CLOCKID_REALTIME ? () => BigInt((/* @__PURE__ */ new Date()).getTime()) * 1000000n : () => BigInt(Math.round(performance.now() * 1e6));
          while (clockTimeout > getNow()) {
            for (const s of subscriptions) {
              if (s.eventtype === EVENTTYPE_FD_READ || s.eventtype === EVENTTYPE_FD_WRITE) {
                if (self.fds[s.fd] != void 0) {
                  const pollResult = self.fds[s.fd].fd_poll(s.eventtype);
                  if (pollResult.ready) {
                    events.push(
                      new Event(s.userdata, ERRNO_SUCCESS, s.eventtype, pollResult.nbytes, pollResult.flags)
                    );
                  }
                }
              }
            }
            if (events.length > 0) {
              break;
            }
          }
          if (events.length === 0) {
            events.push(new Event(clockUserdata, ERRNO_SUCCESS, EVENTTYPE_CLOCK));
          }
        }
        for (let i = 0; i < events.length; i++) {
          events[i].write_bytes(buffer, out_ptr + i * Event.size());
        }
        buffer.setUint32(nevents_ptr, events.length, true);
        self.log("poll_oneoff: returning", events.length, "events");
        return ERRNO_SUCCESS;
      },
      proc_exit(exit_code) {
        throw new WASIProcExit(exit_code);
      },
      proc_raise(sig) {
        throw "raised signal " + sig;
      },
      sched_yield() {
      },
      random_get(buf, buf_len) {
        const buffer8 = new Uint8Array(self.inst.exports.memory.buffer).subarray(buf, buf + buf_len);
        if ("crypto" in globalThis && (typeof SharedArrayBuffer === "undefined" || !(self.inst.exports.memory.buffer instanceof SharedArrayBuffer))) {
          for (let i = 0; i < buf_len; i += 65536) {
            crypto.getRandomValues(buffer8.subarray(i, i + 65536));
          }
        } else {
          for (let i = 0; i < buf_len; i++) {
            buffer8[i] = Math.random() * 256 | 0;
          }
        }
      },
      sock_recv(_fd, _ri_data, _ri_flags) {
        throw "sockets not supported";
      },
      sock_send(_fd, _si_data, _si_flags) {
        throw "sockets not supported";
      },
      sock_shutdown(_fd, _how) {
        throw "sockets not supported";
      },
      sock_accept(_fd, _flags) {
        throw "sockets not supported";
      }
    };
  }
};

// fs_mem.ts
var OpenFile = class extends Fd {
  file;
  file_pos = 0n;
  constructor(file) {
    super();
    this.file = file;
  }
  fd_allocate(offset, len) {
    if (this.file.size > offset + len) {
    } else {
      const new_data = new Uint8Array(Number(offset + len));
      new_data.set(this.file.data, 0);
      this.file.data = new_data;
    }
    return ERRNO_SUCCESS;
  }
  fd_fdstat_get() {
    return { ret: 0, fdstat: new Fdstat(FILETYPE_REGULAR_FILE, 0) };
  }
  fd_filestat_set_size(size) {
    if (this.file.size > size) {
      this.file.data = new Uint8Array(this.file.data.buffer.slice(0, Number(size)));
    } else {
      const new_data = new Uint8Array(Number(size));
      new_data.set(this.file.data, 0);
      this.file.data = new_data;
    }
    return ERRNO_SUCCESS;
  }
  fd_read(size) {
    const slice = this.file.data.slice(Number(this.file_pos), Number(this.file_pos + BigInt(size)));
    this.file_pos += BigInt(slice.length);
    return { ret: 0, data: slice };
  }
  fd_pread(size, offset) {
    const slice = this.file.data.slice(Number(offset), Number(offset + BigInt(size)));
    return { ret: 0, data: slice };
  }
  fd_seek(offset, whence) {
    let calculated_offset;
    switch (whence) {
      case WHENCE_SET:
        calculated_offset = offset;
        break;
      case WHENCE_CUR:
        calculated_offset = this.file_pos + offset;
        break;
      case WHENCE_END:
        calculated_offset = BigInt(this.file.data.byteLength) + offset;
        break;
      default:
        return { ret: ERRNO_INVAL, offset: 0n };
    }
    if (calculated_offset < 0) {
      return { ret: ERRNO_INVAL, offset: 0n };
    }
    this.file_pos = calculated_offset;
    return { ret: 0, offset: this.file_pos };
  }
  fd_tell() {
    return { ret: 0, offset: this.file_pos };
  }
  fd_write(data) {
    if (this.file.readonly) return { ret: ERRNO_BADF, nwritten: 0 };
    if (this.file_pos + BigInt(data.byteLength) > this.file.size) {
      const old = this.file.data;
      this.file.data = new Uint8Array(Number(this.file_pos + BigInt(data.byteLength)));
      this.file.data.set(old);
    }
    this.file.data.set(data, Number(this.file_pos));
    this.file_pos += BigInt(data.byteLength);
    return { ret: 0, nwritten: data.byteLength };
  }
  fd_pwrite(data, offset) {
    if (this.file.readonly) return { ret: ERRNO_BADF, nwritten: 0 };
    if (offset + BigInt(data.byteLength) > this.file.size) {
      const old = this.file.data;
      this.file.data = new Uint8Array(Number(offset + BigInt(data.byteLength)));
      this.file.data.set(old);
    }
    this.file.data.set(data, Number(offset));
    return { ret: 0, nwritten: data.byteLength };
  }
  fd_filestat_get() {
    return { ret: 0, filestat: this.file.stat() };
  }
};
var OpenDirectory = class extends Fd {
  dir;
  constructor(dir) {
    super();
    this.dir = dir;
  }
  fd_seek(_offset, _whence) {
    return { ret: ERRNO_BADF, offset: 0n };
  }
  fd_tell() {
    return { ret: ERRNO_BADF, offset: 0n };
  }
  fd_allocate(_offset, _len) {
    return ERRNO_BADF;
  }
  fd_fdstat_get() {
    return { ret: 0, fdstat: new Fdstat(FILETYPE_DIRECTORY, 0) };
  }
  fd_readdir_single(cookie) {
    if (cookie == 0n) {
      return {
        ret: ERRNO_SUCCESS,
        dirent: new Dirent(1n, this.dir.ino, ".", FILETYPE_DIRECTORY)
      };
    } else if (cookie == 1n) {
      return {
        ret: ERRNO_SUCCESS,
        dirent: new Dirent(2n, this.dir.parent_ino(), "..", FILETYPE_DIRECTORY)
      };
    }
    if (cookie >= BigInt(this.dir.contents.size) + 2n) {
      return { ret: 0, dirent: null };
    }
    const [name, entry] = Array.from(this.dir.contents.entries())[Number(cookie - 2n)];
    return {
      ret: 0,
      dirent: new Dirent(cookie + 1n, entry.ino, name, entry.stat().filetype)
    };
  }
  path_filestat_get(_flags, path_str) {
    const { ret: path_err, path } = Path.from(path_str);
    if (path == null) {
      return { ret: path_err, filestat: null };
    }
    const { ret, entry } = this.dir.get_entry_for_path(path);
    if (entry == null) {
      return { ret, filestat: null };
    }
    return { ret: 0, filestat: entry.stat() };
  }
  path_lookup(path_str, _dirflags) {
    const { ret: path_ret, path } = Path.from(path_str);
    if (path == null) {
      return { ret: path_ret, inode_obj: null };
    }
    const { ret, entry } = this.dir.get_entry_for_path(path);
    if (entry == null) {
      return { ret, inode_obj: null };
    }
    return { ret: ERRNO_SUCCESS, inode_obj: entry };
  }
  path_open(_dirflags, path_str, oflags, fs_rights_base, _fs_rights_inheriting, fd_flags) {
    const { ret: path_ret, path } = Path.from(path_str);
    if (path == null) {
      return { ret: path_ret, fd_obj: null };
    }
    let { ret, entry } = this.dir.get_entry_for_path(path);
    if (entry == null) {
      if (ret != ERRNO_NOENT) {
        return { ret, fd_obj: null };
      }
      if ((oflags & OFLAGS_CREAT) == OFLAGS_CREAT) {
        const { ret: ret2, entry: new_entry } = this.dir.create_entry_for_path(
          path_str,
          (oflags & OFLAGS_DIRECTORY) == OFLAGS_DIRECTORY
        );
        if (new_entry == null) {
          return { ret: ret2, fd_obj: null };
        }
        entry = new_entry;
      } else {
        return { ret: ERRNO_NOENT, fd_obj: null };
      }
    } else if ((oflags & OFLAGS_EXCL) == OFLAGS_EXCL) {
      return { ret: ERRNO_EXIST, fd_obj: null };
    }
    if ((oflags & OFLAGS_DIRECTORY) == OFLAGS_DIRECTORY && entry.stat().filetype !== FILETYPE_DIRECTORY) {
      return { ret: ERRNO_NOTDIR, fd_obj: null };
    }
    return entry.path_open(oflags, fs_rights_base, fd_flags);
  }
  path_create_directory(path) {
    return this.path_open(0, path, OFLAGS_CREAT | OFLAGS_DIRECTORY, 0n, 0n, 0).ret;
  }
  path_link(path_str, inode, allow_dir) {
    const { ret: path_ret, path } = Path.from(path_str);
    if (path == null) {
      return path_ret;
    }
    if (path.is_dir) {
      return ERRNO_NOENT;
    }
    const { ret: parent_ret, parent_entry, filename, entry } = this.dir.get_parent_dir_and_entry_for_path(path, true);
    if (parent_entry == null || filename == null) {
      return parent_ret;
    }
    if (entry != null) {
      const source_is_dir = inode.stat().filetype == FILETYPE_DIRECTORY;
      const target_is_dir = entry.stat().filetype == FILETYPE_DIRECTORY;
      if (source_is_dir && target_is_dir) {
        if (allow_dir && entry instanceof Directory) {
          if (entry.contents.size == 0) {
          } else {
            return ERRNO_NOTEMPTY;
          }
        } else {
          return ERRNO_EXIST;
        }
      } else if (source_is_dir && !target_is_dir) {
        return ERRNO_NOTDIR;
      } else if (!source_is_dir && target_is_dir) {
        return ERRNO_ISDIR;
      } else if (inode.stat().filetype == FILETYPE_REGULAR_FILE && entry.stat().filetype == FILETYPE_REGULAR_FILE) {
      } else {
        return ERRNO_EXIST;
      }
    }
    if (!allow_dir && inode.stat().filetype == FILETYPE_DIRECTORY) {
      return ERRNO_PERM;
    }
    parent_entry.contents.set(filename, inode);
    return ERRNO_SUCCESS;
  }
  path_unlink(path_str) {
    const { ret: path_ret, path } = Path.from(path_str);
    if (path == null) {
      return { ret: path_ret, inode_obj: null };
    }
    const { ret: parent_ret, parent_entry, filename, entry } = this.dir.get_parent_dir_and_entry_for_path(path, true);
    if (parent_entry == null || filename == null) {
      return { ret: parent_ret, inode_obj: null };
    }
    if (entry == null) {
      return { ret: ERRNO_NOENT, inode_obj: null };
    }
    parent_entry.contents.delete(filename);
    return { ret: ERRNO_SUCCESS, inode_obj: entry };
  }
  path_unlink_file(path_str) {
    const { ret: path_ret, path } = Path.from(path_str);
    if (path == null) {
      return path_ret;
    }
    const { ret: parent_ret, parent_entry, filename, entry } = this.dir.get_parent_dir_and_entry_for_path(path, false);
    if (parent_entry == null || filename == null || entry == null) {
      return parent_ret;
    }
    if (entry.stat().filetype === FILETYPE_DIRECTORY) {
      return ERRNO_ISDIR;
    }
    parent_entry.contents.delete(filename);
    return ERRNO_SUCCESS;
  }
  path_remove_directory(path_str) {
    const { ret: path_ret, path } = Path.from(path_str);
    if (path == null) {
      return path_ret;
    }
    const { ret: parent_ret, parent_entry, filename, entry } = this.dir.get_parent_dir_and_entry_for_path(path, false);
    if (parent_entry == null || filename == null || entry == null) {
      return parent_ret;
    }
    if (!(entry instanceof Directory) || entry.stat().filetype !== FILETYPE_DIRECTORY) {
      return ERRNO_NOTDIR;
    }
    if (entry.contents.size !== 0) {
      return ERRNO_NOTEMPTY;
    }
    if (!parent_entry.contents.delete(filename)) {
      return ERRNO_NOENT;
    }
    return ERRNO_SUCCESS;
  }
  fd_filestat_get() {
    return { ret: 0, filestat: this.dir.stat() };
  }
  fd_filestat_set_size(_size) {
    return ERRNO_BADF;
  }
  fd_read(_size) {
    return { ret: ERRNO_BADF, data: new Uint8Array() };
  }
  fd_pread(_size, _offset) {
    return { ret: ERRNO_BADF, data: new Uint8Array() };
  }
  fd_write(_data) {
    return { ret: ERRNO_BADF, nwritten: 0 };
  }
  fd_pwrite(_data, _offset) {
    return { ret: ERRNO_BADF, nwritten: 0 };
  }
};
var PreopenDirectory = class extends OpenDirectory {
  prestat_name;
  constructor(name, contents) {
    super(new Directory(contents));
    this.prestat_name = name;
  }
  fd_prestat_get() {
    return {
      ret: 0,
      prestat: Prestat.dir(this.prestat_name)
    };
  }
};
var File = class extends Inode {
  data;
  readonly;
  constructor(data, options) {
    super();
    this.data = new Uint8Array(data);
    this.readonly = !!options?.readonly;
  }
  path_open(oflags, fs_rights_base, fd_flags) {
    if (this.readonly && (fs_rights_base & BigInt(RIGHTS_FD_WRITE)) == BigInt(RIGHTS_FD_WRITE)) {
      return { ret: ERRNO_PERM, fd_obj: null };
    }
    if ((oflags & OFLAGS_TRUNC) == OFLAGS_TRUNC) {
      if (this.readonly) return { ret: ERRNO_PERM, fd_obj: null };
      this.data = new Uint8Array([]);
    }
    const file = new OpenFile(this);
    if (fd_flags & FDFLAGS_APPEND) file.fd_seek(0n, WHENCE_END);
    return { ret: ERRNO_SUCCESS, fd_obj: file };
  }
  get size() {
    return BigInt(this.data.byteLength);
  }
  stat() {
    return new Filestat(this.ino, FILETYPE_REGULAR_FILE, this.size);
  }
};
var Path = class _Path {
  parts = [];
  is_dir = false;
  static from(path) {
    const self = new _Path();
    self.is_dir = path.endsWith("/");
    if (path.startsWith("/")) {
      return { ret: ERRNO_NOTCAPABLE, path: null };
    }
    if (path.includes("\0")) {
      return { ret: ERRNO_INVAL, path: null };
    }
    for (const component of path.split("/")) {
      if (component === "" || component === ".") {
        continue;
      }
      if (component === "..") {
        if (self.parts.pop() == void 0) {
          return { ret: ERRNO_NOTCAPABLE, path: null };
        }
        continue;
      }
      self.parts.push(component);
    }
    return { ret: ERRNO_SUCCESS, path: self };
  }
  to_path_string() {
    let s = this.parts.join("/");
    if (this.is_dir) {
      s += "/";
    }
    return s;
  }
};
var Directory = class _Directory extends Inode {
  contents;
  parent = null;
  constructor(contents) {
    super();
    if (contents instanceof Array) {
      this.contents = new Map(contents);
    } else {
      this.contents = contents;
    }
    for (const entry of this.contents.values()) {
      if (entry instanceof _Directory) {
        entry.parent = this;
      }
    }
  }
  parent_ino() {
    if (this.parent == null) {
      return Inode.root_ino();
    }
    return this.parent.ino;
  }
  path_open(_oflags, _fs_rights_base, _fd_flags) {
    return { ret: ERRNO_SUCCESS, fd_obj: new OpenDirectory(this) };
  }
  stat() {
    return new Filestat(this.ino, FILETYPE_DIRECTORY, 0n);
  }
  get_entry_for_path(path) {
    let entry = this;
    for (const component of path.parts) {
      if (!(entry instanceof _Directory)) {
        return { ret: ERRNO_NOTDIR, entry: null };
      }
      const child = entry.contents.get(component);
      if (child !== void 0) {
        entry = child;
      } else {
        return { ret: ERRNO_NOENT, entry: null };
      }
    }
    if (path.is_dir) {
      if (entry.stat().filetype != FILETYPE_DIRECTORY) {
        return { ret: ERRNO_NOTDIR, entry: null };
      }
    }
    return { ret: ERRNO_SUCCESS, entry };
  }
  get_parent_dir_and_entry_for_path(path, allow_undefined) {
    const filename = path.parts.pop();
    if (filename === void 0) {
      return {
        ret: ERRNO_INVAL,
        parent_entry: null,
        filename: null,
        entry: null
      };
    }
    const { ret: entry_ret, entry: parent_entry } = this.get_entry_for_path(path);
    if (parent_entry == null) {
      return {
        ret: entry_ret,
        parent_entry: null,
        filename: null,
        entry: null
      };
    }
    if (!(parent_entry instanceof _Directory)) {
      return {
        ret: ERRNO_NOTDIR,
        parent_entry: null,
        filename: null,
        entry: null
      };
    }
    const entry = parent_entry.contents.get(filename);
    if (entry === void 0) {
      if (!allow_undefined) {
        return {
          ret: ERRNO_NOENT,
          parent_entry: null,
          filename: null,
          entry: null
        };
      }
      return { ret: ERRNO_SUCCESS, parent_entry, filename, entry: null };
    }
    if (path.is_dir) {
      if (entry.stat().filetype != FILETYPE_DIRECTORY) {
        return {
          ret: ERRNO_NOTDIR,
          parent_entry: null,
          filename: null,
          entry: null
        };
      }
    }
    return { ret: ERRNO_SUCCESS, parent_entry, filename, entry };
  }
  create_entry_for_path(path_str, is_dir) {
    const { ret: path_ret, path } = Path.from(path_str);
    if (path == null) {
      return { ret: path_ret, entry: null };
    }
    let { ret: parent_ret, parent_entry, filename, entry } = this.get_parent_dir_and_entry_for_path(path, true);
    if (parent_entry == null || filename == null) {
      return { ret: parent_ret, entry: null };
    }
    if (entry != null) {
      return { ret: ERRNO_EXIST, entry: null };
    }
    let new_child;
    if (!is_dir) {
      new_child = new File(new ArrayBuffer(0));
    } else {
      new_child = new _Directory(/* @__PURE__ */ new Map());
    }
    parent_entry.contents.set(filename, new_child);
    entry = new_child;
    return { ret: ERRNO_SUCCESS, entry };
  }
};
var ConsoleStdout = class _ConsoleStdout extends Fd {
  ino;
  write;
  constructor(write) {
    super();
    this.ino = Inode.issue_ino();
    this.write = write;
  }
  fd_filestat_get() {
    const filestat = new Filestat(this.ino, FILETYPE_CHARACTER_DEVICE, BigInt(0));
    return { ret: 0, filestat };
  }
  fd_fdstat_get() {
    const fdstat = new Fdstat(FILETYPE_CHARACTER_DEVICE, 0);
    fdstat.fs_rights_base = BigInt(RIGHTS_FD_WRITE);
    return { ret: 0, fdstat };
  }
  fd_write(data) {
    this.write(data);
    return { ret: 0, nwritten: data.byteLength };
  }
  static lineBuffered(write) {
    const dec = new TextDecoder("utf-8", { fatal: false });
    let line_buf = "";
    return new _ConsoleStdout((buffer) => {
      line_buf += dec.decode(buffer, { stream: true });
      const lines = line_buf.split("\n");
      for (const [i, line] of lines.entries()) {
        if (i < lines.length - 1) {
          write(line);
        } else {
          line_buf = line;
        }
      }
    });
  }
};
var PollableStdin = class extends Fd {
  ino;
  buffer = [];
  bufferSize = 0;
  closed = false;
  constructor() {
    super();
    this.ino = Inode.issue_ino();
  }
  /** Push data to be read from stdin */
  push(data) {
    if (this.closed) return;
    this.buffer.push(data);
    this.bufferSize += data.byteLength;
  }
  /** Close stdin (future reads will return EOF) */
  close() {
    this.closed = true;
  }
  /** Check if data is available */
  hasData() {
    return this.bufferSize > 0;
  }
  /** Check if closed */
  isClosed() {
    return this.closed;
  }
  fd_fdstat_get() {
    const fdstat = new Fdstat(FILETYPE_CHARACTER_DEVICE, 0);
    fdstat.fs_rights_base = BigInt(RIGHTS_FD_READ);
    return { ret: 0, fdstat };
  }
  fd_filestat_get() {
    const filestat = new Filestat(this.ino, FILETYPE_CHARACTER_DEVICE, BigInt(0));
    return { ret: 0, filestat };
  }
  fd_read(size) {
    if (this.bufferSize === 0) {
      return { ret: 0, data: new Uint8Array(0) };
    }
    const result = [];
    let remaining = size;
    while (remaining > 0 && this.buffer.length > 0) {
      const chunk = this.buffer[0];
      if (chunk.byteLength <= remaining) {
        result.push(...chunk);
        remaining -= chunk.byteLength;
        this.bufferSize -= chunk.byteLength;
        this.buffer.shift();
      } else {
        result.push(...chunk.slice(0, remaining));
        this.buffer[0] = chunk.slice(remaining);
        this.bufferSize -= remaining;
        remaining = 0;
      }
    }
    return { ret: 0, data: new Uint8Array(result) };
  }
  /**
   * Poll for read readiness.
   * Returns ready=true if data is available or if the stream is closed.
   */
  fd_poll(eventtype) {
    if (eventtype === EVENTTYPE_FD_READ) {
      const hasData = this.bufferSize > 0;
      const isClosed = this.closed;
      return {
        ready: hasData || isClosed,
        nbytes: BigInt(this.bufferSize),
        flags: isClosed && !hasData ? EVENTRWFLAGS_FD_READWRITE_HANGUP : 0
      };
    }
    return { ready: false, nbytes: 0n, flags: 0 };
  }
};
var DevOut = class extends Fd {
  ino;
  onWrite;
  constructor(onWrite) {
    super();
    this.ino = Inode.issue_ino();
    this.onWrite = onWrite;
  }
  fd_fdstat_get() {
    const fdstat = new Fdstat(FILETYPE_CHARACTER_DEVICE, 0);
    fdstat.fs_rights_base = BigInt(RIGHTS_FD_WRITE);
    return { ret: 0, fdstat };
  }
  fd_filestat_get() {
    const filestat = new Filestat(this.ino, FILETYPE_CHARACTER_DEVICE, BigInt(0));
    return { ret: 0, filestat };
  }
  fd_write(data) {
    this.onWrite(data);
    return { ret: 0, nwritten: data.byteLength };
  }
  /**
   * Poll for write readiness.
   * Always ready for writing.
   */
  fd_poll(eventtype) {
    if (eventtype === EVENTTYPE_FD_WRITE) {
      return { ready: true, nbytes: 0n, flags: 0 };
    }
    return { ready: false, nbytes: 0n, flags: 0 };
  }
};
var DevDirectory = class extends Fd {
  devices;
  prestat_name;
  constructor(name, devices) {
    super();
    this.prestat_name = name;
    this.devices = devices;
  }
  fd_fdstat_get() {
    return { ret: 0, fdstat: new Fdstat(FILETYPE_DIRECTORY, 0) };
  }
  fd_prestat_get() {
    return {
      ret: 0,
      prestat: Prestat.dir(this.prestat_name)
    };
  }
  path_open(_dirflags, path, _oflags, _fs_rights_base, _fs_rights_inheriting, _fd_flags) {
    const device = this.devices.get(path);
    if (device) {
      return { ret: 0, fd_obj: device };
    }
    return { ret: ERRNO_NOENT, fd_obj: null };
  }
};
export {
  ADVICE_DONTNEED,
  ADVICE_NOREUSE,
  ADVICE_NORMAL,
  ADVICE_RANDOM,
  ADVICE_SEQUENTIAL,
  ADVICE_WILLNEED,
  CLOCKID_MONOTONIC,
  CLOCKID_PROCESS_CPUTIME_ID,
  CLOCKID_REALTIME,
  CLOCKID_THREAD_CPUTIME_ID,
  Ciovec,
  ConsoleStdout,
  DevDirectory,
  DevOut,
  Directory,
  Dirent,
  ERRNO_2BIG,
  ERRNO_ACCES,
  ERRNO_ADDRINUSE,
  ERRNO_ADDRNOTAVAIL,
  ERRNO_AFNOSUPPORT,
  ERRNO_AGAIN,
  ERRNO_ALREADY,
  ERRNO_BADF,
  ERRNO_BADMSG,
  ERRNO_BUSY,
  ERRNO_CANCELED,
  ERRNO_CHILD,
  ERRNO_CONNABORTED,
  ERRNO_CONNREFUSED,
  ERRNO_CONNRESET,
  ERRNO_DEADLK,
  ERRNO_DESTADDRREQ,
  ERRNO_DOM,
  ERRNO_DQUOT,
  ERRNO_EXIST,
  ERRNO_FAULT,
  ERRNO_FBIG,
  ERRNO_HOSTUNREACH,
  ERRNO_IDRM,
  ERRNO_ILSEQ,
  ERRNO_INPROGRESS,
  ERRNO_INTR,
  ERRNO_INVAL,
  ERRNO_IO,
  ERRNO_ISCONN,
  ERRNO_ISDIR,
  ERRNO_LOOP,
  ERRNO_MFILE,
  ERRNO_MLINK,
  ERRNO_MSGSIZE,
  ERRNO_MULTIHOP,
  ERRNO_NAMETOOLONG,
  ERRNO_NETDOWN,
  ERRNO_NETRESET,
  ERRNO_NETUNREACH,
  ERRNO_NFILE,
  ERRNO_NOBUFS,
  ERRNO_NODEV,
  ERRNO_NOENT,
  ERRNO_NOEXEC,
  ERRNO_NOLCK,
  ERRNO_NOLINK,
  ERRNO_NOMEM,
  ERRNO_NOMSG,
  ERRNO_NOPROTOOPT,
  ERRNO_NOSPC,
  ERRNO_NOSYS,
  ERRNO_NOTCAPABLE,
  ERRNO_NOTCONN,
  ERRNO_NOTDIR,
  ERRNO_NOTEMPTY,
  ERRNO_NOTRECOVERABLE,
  ERRNO_NOTSOCK,
  ERRNO_NOTSUP,
  ERRNO_NOTTY,
  ERRNO_NXIO,
  ERRNO_OVERFLOW,
  ERRNO_OWNERDEAD,
  ERRNO_PERM,
  ERRNO_PIPE,
  ERRNO_PROTO,
  ERRNO_PROTONOSUPPORT,
  ERRNO_PROTOTYPE,
  ERRNO_RANGE,
  ERRNO_ROFS,
  ERRNO_SPIPE,
  ERRNO_SRCH,
  ERRNO_STALE,
  ERRNO_SUCCESS,
  ERRNO_TIMEDOUT,
  ERRNO_TXTBSY,
  ERRNO_XDEV,
  EVENTRWFLAGS_FD_READWRITE_HANGUP,
  EVENTTYPE_CLOCK,
  EVENTTYPE_FD_READ,
  EVENTTYPE_FD_WRITE,
  Event,
  FDFLAGS_APPEND,
  FDFLAGS_DSYNC,
  FDFLAGS_NONBLOCK,
  FDFLAGS_RSYNC,
  FDFLAGS_SYNC,
  FD_STDERR,
  FD_STDIN,
  FD_STDOUT,
  FILETYPE_BLOCK_DEVICE,
  FILETYPE_CHARACTER_DEVICE,
  FILETYPE_DIRECTORY,
  FILETYPE_REGULAR_FILE,
  FILETYPE_SOCKET_DGRAM,
  FILETYPE_SOCKET_STREAM,
  FILETYPE_SYMBOLIC_LINK,
  FILETYPE_UNKNOWN,
  FSTFLAGS_ATIM,
  FSTFLAGS_ATIM_NOW,
  FSTFLAGS_MTIM,
  FSTFLAGS_MTIM_NOW,
  Fd,
  Fdstat,
  File,
  Filestat,
  Inode,
  Iovec,
  OFLAGS_CREAT,
  OFLAGS_DIRECTORY,
  OFLAGS_EXCL,
  OFLAGS_TRUNC,
  OpenDirectory,
  OpenFile,
  PREOPENTYPE_DIR,
  PollableStdin,
  PreopenDirectory,
  Prestat,
  PrestatDir,
  RIFLAGS_RECV_PEEK,
  RIFLAGS_RECV_WAITALL,
  RIGHTS_FD_ADVISE,
  RIGHTS_FD_ALLOCATE,
  RIGHTS_FD_DATASYNC,
  RIGHTS_FD_FDSTAT_SET_FLAGS,
  RIGHTS_FD_FILESTAT_GET,
  RIGHTS_FD_FILESTAT_SET_SIZE,
  RIGHTS_FD_FILESTAT_SET_TIMES,
  RIGHTS_FD_READ,
  RIGHTS_FD_READDIR,
  RIGHTS_FD_SEEK,
  RIGHTS_FD_SYNC,
  RIGHTS_FD_TELL,
  RIGHTS_FD_WRITE,
  RIGHTS_PATH_CREATE_DIRECTORY,
  RIGHTS_PATH_CREATE_FILE,
  RIGHTS_PATH_FILESTAT_GET,
  RIGHTS_PATH_FILESTAT_SET_SIZE,
  RIGHTS_PATH_FILESTAT_SET_TIMES,
  RIGHTS_PATH_LINK_SOURCE,
  RIGHTS_PATH_LINK_TARGET,
  RIGHTS_PATH_OPEN,
  RIGHTS_PATH_READLINK,
  RIGHTS_PATH_REMOVE_DIRECTORY,
  RIGHTS_PATH_RENAME_SOURCE,
  RIGHTS_PATH_RENAME_TARGET,
  RIGHTS_PATH_SYMLINK,
  RIGHTS_PATH_UNLINK_FILE,
  RIGHTS_POLL_FD_READWRITE,
  RIGHTS_SOCK_SHUTDOWN,
  ROFLAGS_RECV_DATA_TRUNCATED,
  SDFLAGS_RD,
  SDFLAGS_WR,
  SIGNAL_ABRT,
  SIGNAL_ALRM,
  SIGNAL_BUS,
  SIGNAL_CHLD,
  SIGNAL_CONT,
  SIGNAL_FPE,
  SIGNAL_HUP,
  SIGNAL_ILL,
  SIGNAL_INT,
  SIGNAL_KILL,
  SIGNAL_NONE,
  SIGNAL_PIPE,
  SIGNAL_POLL,
  SIGNAL_PROF,
  SIGNAL_PWR,
  SIGNAL_QUIT,
  SIGNAL_SEGV,
  SIGNAL_STOP,
  SIGNAL_SYS,
  SIGNAL_TERM,
  SIGNAL_TRAP,
  SIGNAL_TSTP,
  SIGNAL_TTIN,
  SIGNAL_TTOU,
  SIGNAL_URG,
  SIGNAL_USR1,
  SIGNAL_USR2,
  SIGNAL_VTALRM,
  SIGNAL_WINCH,
  SIGNAL_XCPU,
  SIGNAL_XFSZ,
  SUBCLOCKFLAGS_SUBSCRIPTION_CLOCK_ABSTIME,
  Subscription,
  WASI,
  WASIProcExit,
  WHENCE_CUR,
  WHENCE_END,
  WHENCE_SET
};
