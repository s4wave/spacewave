/* eslint-disable @typescript-eslint/no-explicit-any */
import { isUint8ArrayList, Uint8ArrayList } from 'uint8arraylist'

/**
 * QuickjsStat is the file status object returned by stat() and lstat()
 * functions.
 */
export interface QuickjsStat {
  dev: number
  ino: number
  mode: number
  nlink: number
  uid: number
  gid: number
  rdev: number
  size: number
  blocks: number
  atime: number
  mtime: number
  ctime: number
}

export interface QuickjsGlobalScope {
  /** argv0 provides the executable path. */
  argv0: string

  /**
   * scriptArgs provides the command line arguments. The first argument is the
   * script name.
   */
  scriptArgs: string[]

  /**
   * print prints the arguments separated by spaces and a trailing newline.
   * @param args - Arguments to print
   */
  print(...args: any[]): void

  console: {
    /**
     * log prints the arguments separated by spaces and a trailing newline, the
     * same as print().
     * @param message - Message to log
     * @param optionalParams - Additional parameters to log
     */
    log(message?: any, ...optionalParams: any[]): void
  }

  navigator: {
    /** userAgent returns quickjs-ng/<version>. */
    userAgent: string
  }

  /**
   * gc is shorthand for std.gc().
   * Manually invoke the cycle removal algorithm.
   */
  gc(): void

  std: {
    /**
     * exit exits the process.
     * @param code - Exit code
     */
    exit(code: number): void

    /**
     * evalScript evaluates the string str as a script (global eval).
     * @param str - String to evaluate
     * @param options - Optional evaluation options
     * @param options.backtrace_barrier - If true, error backtraces do not list
     *   the stack frames below the evalScript
     * @param options.async - If true, await is accepted in the script and a
     *   promise is returned
     */
    evalScript(
      str: string,
      options?: { backtrace_barrier?: boolean; async?: boolean },
    ): any

    /**
     * loadScript evaluates the file filename as a script (global eval).
     * @param filename - File to evaluate
     */
    loadScript(filename: string): void

    /**
     * loadFile loads the file filename and returns it as a string assuming
     * UTF-8 encoding.
     * Return null in case of I/O error.
     * @param filename - File to load
     * @param options - Optional load options
     * @param options.binary - If true, return a Uint8Array instead of a string
     */
    loadFile(
      filename: string,
      options?: { binary?: boolean },
    ): string | Uint8Array | null

    /**
     * writeFile creates the file filename and writes data into it.
     * @param filename - File to create
     * @param data - Data to write (string, typed array, or ArrayBuffer)
     */
    writeFile(
      filename: string,
      data: string | ArrayBuffer | ArrayBufferView,
    ): void

    /**
     * open opens a file (wrapper to the libc fopen()).
     * @param filename - File to open
     * @param flags - File flags
     * @param errorObj - Optional error object to set errno property
     */
    open(
      filename: string,
      flags: string,
      errorObj?: { errno?: number },
    ): any | null

    /**
     * popen opens a process by creating a pipe (wrapper to the libc popen()).
     * @param command - Command to execute
     * @param flags - Pipe flags
     * @param errorObj - Optional error object to set errno property
     */
    popen(
      command: string,
      flags: string,
      errorObj?: { errno?: number },
    ): any | null

    /**
     * fdopen opens a file from a file handle (wrapper to the libc fdopen()).
     * @param fd - File descriptor
     * @param flags - File flags
     * @param errorObj - Optional error object to set errno property
     */
    fdopen(fd: number, flags: string, errorObj?: { errno?: number }): any | null

    /**
     * Open a temporary file.
     * @param errorObj - Optional error object to set errno property
     */
    tmpfile(errorObj?: { errno?: number }): any | null

    /**
     * puts is equivalent to std.out.puts(str).
     * @param str - String to output
     */
    puts(str: string): void

    /**
     * printf is equivalent to std.out.printf(fmt, ...args).
     * @param fmt - Format string
     * @param args - Arguments for formatting
     */
    printf(fmt: string, ...args: any[]): void

    /**
     * sprintf is equivalent to the libc sprintf().
     * @param fmt - Format string
     * @param args - Arguments for formatting
     */
    sprintf(fmt: string, ...args: any[]): string

    /** in is a wrapper to the libc file stdin. */
    in: any

    /** out is a wrapper to the libc file stdout. */
    out: any

    /** err is a wrapper to the libc file stderr. */
    err: any

    /**
     * Error is an enumeration object containing the integer value of common
     * errors.
     */
    Error: {
      EINVAL: number
      EIO: number
      EACCES: number
      EEXIST: number
      ENOSPC: number
      ENOSYS: number
      EBUSY: number
      ENOENT: number
      EPERM: number
      EPIPE: number
    }

    /**
     * strerror returns a string that describes the error errno.
     * @param errno - Error number
     */
    strerror(errno: number): string

    /**
     * gc manually invokes the cycle removal algorithm.
     */
    gc(): void

    /**
     * getenv returns the value of the environment variable name or undefined
     * if it is not defined.
     * @param name - Environment variable name
     */
    getenv(name: string): string | undefined

    /**
     * setenv sets the value of the environment variable name to the string
     * value.
     * @param name - Environment variable name
     * @param value - Environment variable value
     */
    setenv(name: string, value: string): void

    /**
     * unsetenv deletes the environment variable name.
     * @param name - Environment variable name
     */
    unsetenv(name: string): void

    /**
     * getenviron returns an object containing the environment variables as
     * key-value pairs.
     */
    getenviron(): Record<string, string>

    /** Constants for seek(). */
    SEEK_SET: number
    SEEK_CUR: number
    SEEK_END: number
  }

  os: {
    /**
     * open opens a file. Returns a handle or < 0 if error.
     * @param filename - File to open
     * @param flags - Open flags
     * @param mode - File mode (default: 0o666)
     */
    open(filename: string, flags: number, mode?: number): number

    /** POSIX open flags. */
    O_RDONLY: number
    O_WRONLY: number
    O_RDWR: number
    O_APPEND: number
    O_CREAT: number
    O_EXCL: number
    O_TRUNC: number

    /**
     * close closes the file handle fd.
     * @param fd - File descriptor
     */
    close(fd: number): number

    /**
     * seek seeks in the file. Use std.SEEK_* for whence.
     * @param fd - File descriptor
     * @param offset - Offset (number or bigint)
     * @param whence - Seek position reference
     */
    seek(fd: number, offset: number | bigint, whence: number): number | bigint

    /**
     * read reads length bytes from the file handle fd to the ArrayBuffer buffer
     * at byte position offset.
     * @param fd - File descriptor
     * @param buffer - ArrayBuffer to read into
     * @param offset - Byte position in buffer
     * @param length - Number of bytes to read
     */
    read(
      fd: number,
      buffer: ArrayBuffer,
      offset: number,
      length: number,
    ): number

    /**
     * write writes length bytes to the file handle fd from the ArrayBuffer
     * buffer at byte position offset.
     * @param fd - File descriptor
     * @param buffer - ArrayBuffer to write from
     * @param offset - Byte position in buffer
     * @param length - Number of bytes to write
     */
    write<TArrayBuffer extends ArrayBufferLike = ArrayBufferLike>(
      fd: number,
      buffer: TArrayBuffer,
      offset: number,
      length: number,
    ): number

    /**
     * isatty returns true if fd is a TTY (terminal) handle.
     * @param fd - File descriptor
     */
    isatty(fd: number): boolean

    /**
     * ttyGetWinSize returns the TTY size as [width, height] or null if not
     * available.
     * @param fd - File descriptor
     */
    ttyGetWinSize(fd: number): [number, number] | null

    /**
     * ttySetRaw sets the TTY in raw mode.
     * @param fd - File descriptor
     */
    ttySetRaw(fd: number): void

    /**
     * remove removes a file. Returns 0 if OK or -errno.
     * @param filename - File to remove
     */
    remove(filename: string): number

    /**
     * rename renames a file. Returns 0 if OK or -errno.
     * @param oldname - Old filename
     * @param newname - New filename
     */
    rename(oldname: string, newname: string): number

    /**
     * realpath returns [str, err] where str is the canonicalized absolute
     * pathname of path and err is the error code.
     * @param path - Path to resolve
     */
    realpath(path: string): [string, number]

    /**
     * getcwd returns [str, err] where str is the current working directory and
     * err is the error code.
     */
    getcwd(): [string, number]

    /**
     * chdir changes the current directory. Returns 0 if OK or -errno.
     * @param path - Directory path
     */
    chdir(path: string): number

    /**
     * mkdir creates a directory at path. Returns 0 if OK or -errno.
     * @param path - Directory path
     * @param mode - Directory mode (default: 0o777)
     */
    mkdir(path: string, mode?: number): number

    /**
     * stat returns [obj, err] where obj is an object containing the file status
     * of path.
     * @param path - File path
     */
    stat(path: string): [QuickjsStat, number]

    /**
     * lstat is the same as stat() except that it returns information about the
     * link itself.
     * @param path - File path
     */
    lstat(path: string): [QuickjsStat, number]

    /** Constants to interpret the mode property returned by stat(). */
    S_IFMT: number
    S_IFIFO: number
    S_IFCHR: number
    S_IFDIR: number
    S_IFBLK: number
    S_IFREG: number
    S_IFSOCK: number
    S_IFLNK: number
    S_ISGID: number
    S_ISUID: number

    /**
     * utimes changes the access and modification times of the file path.
     * @param path - File path
     * @param atime - Access time in milliseconds since 1970
     * @param mtime - Modification time in milliseconds since 1970
     */
    utimes(path: string, atime: number, mtime: number): number

    /**
     * symlink creates a link at linkpath containing the string target. Returns
     * 0 if OK or -errno.
     * @param target - Link target
     * @param linkpath - Link path
     */
    symlink(target: string, linkpath: string): number

    /**
     * readlink returns [str, err] where str is the link target and err is the
     * error code.
     * @param path - Link path
     */
    readlink(path: string): [string, number]

    /**
     * readdir returns [array, err] where array is an array of strings
     * containing the filenames of the directory path.
     * @param path - Directory path
     */
    readdir(path: string): [string[], number]

    /**
     * setReadHandler adds a read handler to the file handle fd. func is called
     * each time there is data pending for fd.
     * @param fd - File descriptor
     * @param func - Handler function (null to remove)
     */
    setReadHandler(fd: number, func: (() => void) | null): void

    /**
     * setWriteHandler adds a write handler to the file handle fd. func is
     * called each time data can be written to fd.
     * @param fd - File descriptor
     * @param func - Handler function (null to remove)
     */
    setWriteHandler(fd: number, func: (() => void) | null): void

    /**
     * signal calls the function func when the signal signal happens.
     * @param signal - Signal number
     * @param func - Handler function (null for default, undefined to ignore)
     */
    signal(signal: number, func: (() => void) | null | undefined): void

    /** POSIX signal numbers. */
    SIGINT: number
    SIGABRT: number
    SIGFPE: number
    SIGILL: number
    SIGSEGV: number
    SIGTERM: number
    SIGALRM: number
    SIGCHLD: number
    SIGCONT: number
    SIGPIPE: number
    SIGQUIT: number
    SIGSTOP: number
    SIGTSTP: number
    SIGTTIN: number
    SIGTTOU: number
    SIGUSR1: number
    SIGUSR2: number

    /**
     * kill sends the signal sig to the process pid.
     * @param pid - Process ID
     * @param sig - Signal number
     */
    kill(pid: number, sig: number): number

    /**
     * exec executes a process with the arguments args.
     * @param args - Process arguments
     * @param options - Optional execution options
     */
    exec(
      args: string[],
      options?: {
        block?: boolean
        usePath?: boolean
        file?: string
        cwd?: string
        stdin?: number
        stdout?: number
        stderr?: number
        env?: Record<string, string>
        uid?: number
        gid?: number
      },
    ): number

    /**
     * getpid returns the current process ID.
     */
    getpid(): number

    /**
     * waitpid calls the Unix system call. Returns the array [ret, status].
     * @param pid - Process ID
     * @param options - Wait options
     */
    waitpid(pid: number, options: number): [number, number]

    /** WNOHANG is a constant for the options argument of waitpid. */
    WNOHANG: number

    /**
     * dup calls the Unix system call.
     * @param fd - File descriptor
     */
    dup(fd: number): number

    /**
     * dup2 calls the Unix system call.
     * @param oldfd - Old file descriptor
     * @param newfd - New file descriptor
     */
    dup2(oldfd: number, newfd: number): number

    /**
     * pipe calls the Unix system call. Returns two handles as [read_fd,
     * write_fd] or null in case of error.
     */
    pipe(): [number, number] | null

    /**
     * sleep sleeps during delay_ms milliseconds.
     * @param delay_ms - Delay in milliseconds
     */
    sleep(delay_ms: number): void

    /**
     * sleepAsync sleeps asynchronously during delay_ms milliseconds. Returns a
     * promise.
     * @param delay_ms - Delay in milliseconds
     */
    sleepAsync(delay_ms: number): Promise<void>

    /**
     * now returns a timestamp in milliseconds with more precision than
     * Date.now().
     */
    now(): number

    /**
     * setTimeout calls the function func after delay ms. Returns a handle to
     * the timer.
     * @param func - Function to call
     * @param delay - Delay in milliseconds
     */
    setTimeout(func: () => void, delay: number): any

    /**
     * clearTimeout cancels a timer.
     * @param handle - Timer handle
     */
    clearTimeout(handle: any): void

    /**
     * setInterval calls the function func periodically with the given interval.
     * Returns a handle to the timer.
     * @param func - Function to call
     * @param delay - Interval in milliseconds
     */
    setInterval(func: () => void, delay: number): any

    /**
     * clearInterval cancels an interval timer.
     * @param handle - Timer handle
     */
    clearInterval(handle: any): void

    /**
     * cputime returns CPU time in milliseconds.
     */
    cputime(): number

    /**
     * exePath returns the path of the current executable.
     */
    exePath(): string

    /**
     * platform is a string representing the platform: "linux", "darwin",
     * "win32" or "js".
     */
    platform: string

    /**
     * Worker is a constructor to create a new thread (worker) with an API close
     * to the WebWorkers.
     * @param module_filename - Module filename to execute in the new thread
     */
    Worker: {
      new (module_filename: string): {
        /**
         * postMessage sends a message to the corresponding worker.
         * @param msg - Message to send
         */
        postMessage(msg: any): void

        /**
         * onmessage is a getter and setter for the message handler function.
         */
        onmessage: ((event: { data: any }) => void) | null
      }

      /**
       * parent represents the parent worker as Worker.parent in the created
       * worker.
       */
      parent: {
        /**
         * postMessage sends a message to the parent worker.
         * @param msg - Message to send
         */
        postMessage(msg: any): void

        /**
         * onmessage is a getter and setter for the message handler function.
         */
        onmessage: ((event: { data: any }) => void) | null
      } | null
    }
  }
}

/**
 * writeCompleteChunk writes a complete Uint8Array to a file descriptor,
 * handling partial writes.
 *
 * @param os - QuickJS os module instance
 * @param fd - File descriptor to write to
 * @param data - Data to write
 */
function writeCompleteChunk(
  os: QuickjsGlobalScope['os'],
  fd: number,
  data: Uint8Array,
): void {
  let offset = 0
  while (offset < data.length) {
    const bytesWritten = os.write(
      fd,
      data.buffer,
      data.byteOffset + offset,
      data.length - offset,
    )
    if (bytesWritten < 0) {
      throw new Error(`Write failed with error code: ${bytesWritten}`)
    }
    if (bytesWritten === 0) {
      throw new Error(
        'Write returned 0 bytes, possible full disk or broken pipe',
      )
    }
    offset += bytesWritten
  }
}

/**
 * writeSourceToFd consumes an AsyncIterable source containing Uint8Arrays or
 * Uint8ArrayLists and writes the data efficiently to a file path in append mode
 * using the QuickJS 'os' module.
 *
 * @param os - QuickJS os module instance
 * @param source - The input data stream
 * @param filePath - The path to the file (e.g., '/dev/out')
 */
export async function writeSourceToFd(
  os: QuickjsGlobalScope['os'],
  source: AsyncIterable<Uint8Array | Uint8ArrayList>,
  filePath: string,
): Promise<void> {
  // Define the POSIX flags for write-only, append, and create if not exists
  const flags = os.O_WRONLY | os.O_APPEND | os.O_CREAT
  // Standard permissions if the file is created (owner rw, group r, others r)
  const mode = 0o644

  let fd: number | undefined = undefined
  try {
    // Open the file descriptor using os.open
    fd = os.open(filePath, flags, mode)
    if (fd < 0) {
      // os.open returns negative errno on failure
      throw new Error(`Failed to open file ${filePath}. Error code: ${fd}`)
    }

    // Process the stream
    for await (const chunk of source) {
      if (isUint8ArrayList(chunk)) {
        // Uint8ArrayList is iterable over its internal buffers
        for (const internalBuf of chunk) {
          writeCompleteChunk(os, fd, internalBuf)
        }
      } else if (chunk instanceof Uint8Array) {
        writeCompleteChunk(os, fd, chunk)
      } else {
        throw new Error(
          `Received unsupported chunk type in stream: ${typeof chunk}`,
        )
      }
    }
  } finally {
    // Ensure the file descriptor is always closed, even if errors occur
    if (fd !== undefined && fd >= 0) {
      os.close(fd)
    }
  }
}
