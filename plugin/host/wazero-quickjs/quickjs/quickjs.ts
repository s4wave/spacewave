import { isUint8ArrayList, Uint8ArrayList } from 'uint8arraylist'

/**
 * File status object returned by stat() and lstat() functions.
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
  /** Provides the executable path. */
  argv0: string

  /** Provides the command line arguments. The first argument is the script name. */
  scriptArgs: string[]

  /**
   * Print the arguments separated by spaces and a trailing newline.
   * @param args - Arguments to print
   */
  print(...args: any[]): void

  console: {
    /**
     * Same as print(). Print the arguments separated by spaces and a trailing newline.
     * @param message - Message to log
     * @param optionalParams - Additional parameters to log
     */
    log(message?: any, ...optionalParams: any[]): void
  }

  navigator: {
    /** Returns quickjs-ng/<version>. */
    userAgent: string
  }

  /**
   * Shorthand for std.gc().
   * Manually invoke the cycle removal algorithm.
   */
  gc(): void

  std: {
    /**
     * Exit the process.
     * @param code - Exit code
     */
    exit(code: number): void

    /**
     * Evaluate the string str as a script (global eval).
     * @param str - String to evaluate
     * @param options - Optional evaluation options
     * @param options.backtrace_barrier - If true, error backtraces do not list the stack frames below the evalScript
     * @param options.async - If true, await is accepted in the script and a promise is returned
     */
    evalScript(
      str: string,
      options?: { backtrace_barrier?: boolean; async?: boolean },
    ): any

    /**
     * Evaluate the file filename as a script (global eval).
     * @param filename - File to evaluate
     */
    loadScript(filename: string): void

    /**
     * Load the file filename and return it as a string assuming UTF-8 encoding.
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
     * Create the file filename and write data into it.
     * @param filename - File to create
     * @param data - Data to write (string, typed array, or ArrayBuffer)
     */
    writeFile(
      filename: string,
      data: string | ArrayBuffer | ArrayBufferView,
    ): void

    /**
     * Open a file (wrapper to the libc fopen()).
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
     * Open a process by creating a pipe (wrapper to the libc popen()).
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
     * Open a file from a file handle (wrapper to the libc fdopen()).
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
     * Equivalent to std.out.puts(str).
     * @param str - String to output
     */
    puts(str: string): void

    /**
     * Equivalent to std.out.printf(fmt, ...args).
     * @param fmt - Format string
     * @param args - Arguments for formatting
     */
    printf(fmt: string, ...args: any[]): void

    /**
     * Equivalent to the libc sprintf().
     * @param fmt - Format string
     * @param args - Arguments for formatting
     */
    sprintf(fmt: string, ...args: any[]): string

    /** Wrapper to the libc file stdin. */
    in: any

    /** Wrapper to the libc file stdout. */
    out: any

    /** Wrapper to the libc file stderr. */
    err: any

    /** Enumeration object containing the integer value of common errors. */
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
     * Return a string that describes the error errno.
     * @param errno - Error number
     */
    strerror(errno: number): string

    /**
     * Manually invoke the cycle removal algorithm.
     */
    gc(): void

    /**
     * Return the value of the environment variable name or undefined if it is not defined.
     * @param name - Environment variable name
     */
    getenv(name: string): string | undefined

    /**
     * Set the value of the environment variable name to the string value.
     * @param name - Environment variable name
     * @param value - Environment variable value
     */
    setenv(name: string, value: string): void

    /**
     * Delete the environment variable name.
     * @param name - Environment variable name
     */
    unsetenv(name: string): void

    /**
     * Return an object containing the environment variables as key-value pairs.
     */
    getenviron(): Record<string, string>

    /** Constants for seek(). */
    SEEK_SET: number
    SEEK_CUR: number
    SEEK_END: number
  }

  os: {
    /**
     * Open a file. Return a handle or < 0 if error.
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
     * Close the file handle fd.
     * @param fd - File descriptor
     */
    close(fd: number): number

    /**
     * Seek in the file. Use std.SEEK_* for whence.
     * @param fd - File descriptor
     * @param offset - Offset (number or bigint)
     * @param whence - Seek position reference
     */
    seek(fd: number, offset: number | bigint, whence: number): number | bigint

    /**
     * Read length bytes from the file handle fd to the ArrayBuffer buffer at byte position offset.
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
     * Write length bytes to the file handle fd from the ArrayBuffer buffer at byte position offset.
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
     * Return true if fd is a TTY (terminal) handle.
     * @param fd - File descriptor
     */
    isatty(fd: number): boolean

    /**
     * Return the TTY size as [width, height] or null if not available.
     * @param fd - File descriptor
     */
    ttyGetWinSize(fd: number): [number, number] | null

    /**
     * Set the TTY in raw mode.
     * @param fd - File descriptor
     */
    ttySetRaw(fd: number): void

    /**
     * Remove a file. Return 0 if OK or -errno.
     * @param filename - File to remove
     */
    remove(filename: string): number

    /**
     * Rename a file. Return 0 if OK or -errno.
     * @param oldname - Old filename
     * @param newname - New filename
     */
    rename(oldname: string, newname: string): number

    /**
     * Return [str, err] where str is the canonicalized absolute pathname of path and err the error code.
     * @param path - Path to resolve
     */
    realpath(path: string): [string, number]

    /**
     * Return [str, err] where str is the current working directory and err the error code.
     */
    getcwd(): [string, number]

    /**
     * Change the current directory. Return 0 if OK or -errno.
     * @param path - Directory path
     */
    chdir(path: string): number

    /**
     * Create a directory at path. Return 0 if OK or -errno.
     * @param path - Directory path
     * @param mode - Directory mode (default: 0o777)
     */
    mkdir(path: string, mode?: number): number

    /**
     * Return [obj, err] where obj is an object containing the file status of path.
     * @param path - File path
     */
    stat(path: string): [QuickjsStat, number]

    /**
     * Same as stat() except that it returns information about the link itself.
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
     * Change the access and modification times of the file path.
     * @param path - File path
     * @param atime - Access time in milliseconds since 1970
     * @param mtime - Modification time in milliseconds since 1970
     */
    utimes(path: string, atime: number, mtime: number): number

    /**
     * Create a link at linkpath containing the string target. Return 0 if OK or -errno.
     * @param target - Link target
     * @param linkpath - Link path
     */
    symlink(target: string, linkpath: string): number

    /**
     * Return [str, err] where str is the link target and err the error code.
     * @param path - Link path
     */
    readlink(path: string): [string, number]

    /**
     * Return [array, err] where array is an array of strings containing the filenames of the directory path.
     * @param path - Directory path
     */
    readdir(path: string): [string[], number]

    /**
     * Add a read handler to the file handle fd. func is called each time there is data pending for fd.
     * @param fd - File descriptor
     * @param func - Handler function (null to remove)
     */
    setReadHandler(fd: number, func: (() => void) | null): void

    /**
     * Add a write handler to the file handle fd. func is called each time data can be written to fd.
     * @param fd - File descriptor
     * @param func - Handler function (null to remove)
     */
    setWriteHandler(fd: number, func: (() => void) | null): void

    /**
     * Call the function func when the signal signal happens.
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
     * Send the signal sig to the process pid.
     * @param pid - Process ID
     * @param sig - Signal number
     */
    kill(pid: number, sig: number): number

    /**
     * Execute a process with the arguments args.
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
     * Return the current process ID.
     */
    getpid(): number

    /**
     * waitpid Unix system call. Return the array [ret, status].
     * @param pid - Process ID
     * @param options - Wait options
     */
    waitpid(pid: number, options: number): [number, number]

    /** Constant for the options argument of waitpid. */
    WNOHANG: number

    /**
     * dup Unix system call.
     * @param fd - File descriptor
     */
    dup(fd: number): number

    /**
     * dup2 Unix system call.
     * @param oldfd - Old file descriptor
     * @param newfd - New file descriptor
     */
    dup2(oldfd: number, newfd: number): number

    /**
     * pipe Unix system call. Return two handles as [read_fd, write_fd] or null in case of error.
     */
    pipe(): [number, number] | null

    /**
     * Sleep during delay_ms milliseconds.
     * @param delay_ms - Delay in milliseconds
     */
    sleep(delay_ms: number): void

    /**
     * Asynchronous sleep during delay_ms milliseconds. Returns a promise.
     * @param delay_ms - Delay in milliseconds
     */
    sleepAsync(delay_ms: number): Promise<void>

    /**
     * Return a timestamp in milliseconds with more precision than Date.now().
     */
    now(): number

    /**
     * Call the function func after delay ms. Return a handle to the timer.
     * @param func - Function to call
     * @param delay - Delay in milliseconds
     */
    setTimeout(func: () => void, delay: number): any

    /**
     * Cancel a timer.
     * @param handle - Timer handle
     */
    clearTimeout(handle: any): void

    /**
     * Call the function func periodically with the given interval. Return a handle to the timer.
     * @param func - Function to call
     * @param delay - Interval in milliseconds
     */
    setInterval(func: () => void, delay: number): any

    /**
     * Cancel an interval timer.
     * @param handle - Timer handle
     */
    clearInterval(handle: any): void

    /**
     * Return CPU time in milliseconds.
     */
    cputime(): number

    /**
     * Return the path of the current executable.
     */
    exePath(): string

    /** Return a string representing the platform: "linux", "darwin", "win32" or "js". */
    platform: string

    /**
     * Constructor to create a new thread (worker) with an API close to the WebWorkers.
     * @param module_filename - Module filename to execute in the new thread
     */
    Worker: {
      new (module_filename: string): {
        /**
         * Send a message to the corresponding worker.
         * @param msg - Message to send
         */
        postMessage(msg: any): void

        /** Getter and setter for message handler function. */
        onmessage: ((event: { data: any }) => void) | null
      }

      /** In the created worker, Worker.parent represents the parent worker. */
      parent: {
        /**
         * Send a message to the parent worker.
         * @param msg - Message to send
         */
        postMessage(msg: any): void

        /** Getter and setter for message handler function. */
        onmessage: ((event: { data: any }) => void) | null
      } | null
    }
  }
}

/**
 * Writes a complete Uint8Array to a file descriptor, handling partial writes.
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
 * Consumes an AsyncIterable source containing Uint8Arrays or Uint8ArrayLists
 * and writes the data efficiently to a file path in append mode using QuickJS 'os' module.
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
  } catch (error) {
    // Re-throw the error so the caller knows the pipe failed
    throw error
  } finally {
    // Ensure the file descriptor is always closed, even if errors occur
    if (fd !== undefined && fd >= 0) {
      os.close(fd)
    }
  }
}
