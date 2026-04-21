// License for this file: MIT License
// Derived from: https://github.com/bjorn3/browser_wasi_shim
// Extended with fd polling support for QuickJS async I/O

import * as wasi from './wasi_defs.js'
import { Fd } from './fd.js'

export interface Options {
  debug?: boolean
}

/**
 * An exception that is thrown when the process exits.
 **/
export class WASIProcExit extends Error {
  constructor(public readonly code: number) {
    super('exit with exit code ' + code)
  }
}

export default class WASI {
  #freeFds: Array<number> = []

  args: Array<string> = []
  env: Array<string> = []
  fds: Array<Fd | undefined> = []
  inst!: { exports: { memory: WebAssembly.Memory } }
  debug: boolean = false
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  wasiImport: Record<string, (...args: any[]) => any>

  /// Start a WASI command
  start(instance: {
    exports: { memory: WebAssembly.Memory; _start: () => unknown }
  }) {
    this.inst = instance
    try {
      instance.exports._start()
      return 0
    } catch (e) {
      if (e instanceof WASIProcExit) {
        return e.code
      }
      throw e
    }
  }

  /// Initialize a WASI reactor
  initialize(instance: {
    exports: { memory: WebAssembly.Memory; _initialize?: () => unknown }
  }) {
    this.inst = instance
    if (instance.exports._initialize) {
      instance.exports._initialize()
    }
  }

  log(...args: unknown[]) {
    if (this.debug) {
      console.log('[WASI]', ...args)
    }
  }

  constructor(
    args: Array<string>,
    env: Array<string>,
    fds: Array<Fd>,
    options: Options = {},
  ) {
    this.debug = options.debug ?? false
    this.args = args
    this.env = env
    this.fds = fds

    this.wasiImport = {
      args_sizes_get: (argc: number, argv_buf_size: number): number => {
        const buffer = new DataView(this.inst.exports.memory.buffer)
        buffer.setUint32(argc, this.args.length, true)
        let buf_size = 0
        for (const arg of this.args) {
          buf_size += arg.length + 1
        }
        buffer.setUint32(argv_buf_size, buf_size, true)
        this.log('args_sizes_get', this.args.length, buf_size)
        return 0
      },

      args_get: (argv: number, argv_buf: number): number => {
        const buffer = new DataView(this.inst.exports.memory.buffer)
        const buffer8 = new Uint8Array(this.inst.exports.memory.buffer)
        for (let i = 0; i < this.args.length; i++) {
          buffer.setUint32(argv, argv_buf, true)
          argv += 4
          const arg = new TextEncoder().encode(this.args[i])
          buffer8.set(arg, argv_buf)
          buffer.setUint8(argv_buf + arg.length, 0)
          argv_buf += arg.length + 1
        }
        return 0
      },

      environ_sizes_get: (
        environ_count: number,
        environ_size: number,
      ): number => {
        const buffer = new DataView(this.inst.exports.memory.buffer)
        buffer.setUint32(environ_count, this.env.length, true)
        let buf_size = 0
        for (const environ of this.env) {
          buf_size += new TextEncoder().encode(environ).length + 1
        }
        buffer.setUint32(environ_size, buf_size, true)
        return 0
      },

      environ_get: (environ: number, environ_buf: number): number => {
        const buffer = new DataView(this.inst.exports.memory.buffer)
        const buffer8 = new Uint8Array(this.inst.exports.memory.buffer)
        for (let i = 0; i < this.env.length; i++) {
          buffer.setUint32(environ, environ_buf, true)
          environ += 4
          const e = new TextEncoder().encode(this.env[i])
          buffer8.set(e, environ_buf)
          buffer.setUint8(environ_buf + e.length, 0)
          environ_buf += e.length + 1
        }
        return 0
      },

      clock_res_get: (id: number, res_ptr: number): number => {
        let resolutionValue: bigint
        switch (id) {
          case wasi.CLOCKID_MONOTONIC: {
            resolutionValue = 5_000n // 5 microseconds
            break
          }
          case wasi.CLOCKID_REALTIME: {
            resolutionValue = 1_000_000n // 1 millisecond
            break
          }
          default:
            return wasi.ERRNO_NOSYS
        }
        const view = new DataView(this.inst.exports.memory.buffer)
        view.setBigUint64(res_ptr, resolutionValue, true)
        return wasi.ERRNO_SUCCESS
      },

      clock_time_get: (
        id: number,
        _precision: bigint,
        time: number,
      ): number => {
        const buffer = new DataView(this.inst.exports.memory.buffer)
        if (id === wasi.CLOCKID_REALTIME) {
          buffer.setBigUint64(
            time,
            BigInt(new Date().getTime()) * 1_000_000n,
            true,
          )
        } else if (id == wasi.CLOCKID_MONOTONIC) {
          let monotonic_time: bigint
          try {
            monotonic_time = BigInt(Math.round(performance.now() * 1000000))
          } catch {
            monotonic_time = 0n
          }
          buffer.setBigUint64(time, monotonic_time, true)
        } else {
          buffer.setBigUint64(time, 0n, true)
        }
        return 0
      },

      fd_advise: (
        fd: number,
        _offset: bigint,
        _len: bigint,
        _advice: number,
      ): number => {
        if (this.fds[fd] != undefined) {
          return wasi.ERRNO_SUCCESS
        }
        return wasi.ERRNO_BADF
      },

      fd_allocate: (fd: number, offset: bigint, len: bigint): number => {
        if (this.fds[fd] != undefined) {
          return this.fds[fd]!.fd_allocate(offset, len)
        }
        return wasi.ERRNO_BADF
      },

      fd_close: (fd: number): number => {
        if (this.fds[fd] != undefined) {
          const ret = this.fds[fd]!.fd_close()
          this.fds[fd] = undefined
          this.#freeFds.push(fd)
          return ret
        }
        return wasi.ERRNO_BADF
      },

      fd_datasync: (fd: number): number => {
        if (this.fds[fd] != undefined) {
          return this.fds[fd]!.fd_sync()
        }
        return wasi.ERRNO_BADF
      },

      fd_fdstat_get: (fd: number, fdstat_ptr: number): number => {
        if (this.fds[fd] != undefined) {
          const { ret, fdstat } = this.fds[fd]!.fd_fdstat_get()
          if (fdstat != null) {
            fdstat.write_bytes(
              new DataView(this.inst.exports.memory.buffer),
              fdstat_ptr,
            )
          }
          return ret
        }
        return wasi.ERRNO_BADF
      },

      fd_fdstat_set_flags: (fd: number, flags: number): number => {
        if (this.fds[fd] != undefined) {
          return this.fds[fd]!.fd_fdstat_set_flags(flags)
        }
        return wasi.ERRNO_BADF
      },

      fd_fdstat_set_rights: (
        fd: number,
        fs_rights_base: bigint,
        fs_rights_inheriting: bigint,
      ): number => {
        if (this.fds[fd] != undefined) {
          return this.fds[fd]!.fd_fdstat_set_rights(
            fs_rights_base,
            fs_rights_inheriting,
          )
        }
        return wasi.ERRNO_BADF
      },

      fd_filestat_get: (fd: number, filestat_ptr: number): number => {
        if (this.fds[fd] != undefined) {
          const { ret, filestat } = this.fds[fd]!.fd_filestat_get()
          if (filestat != null) {
            filestat.write_bytes(
              new DataView(this.inst.exports.memory.buffer),
              filestat_ptr,
            )
          }
          return ret
        }
        return wasi.ERRNO_BADF
      },

      fd_filestat_set_size: (fd: number, size: bigint): number => {
        if (this.fds[fd] != undefined) {
          return this.fds[fd]!.fd_filestat_set_size(size)
        }
        return wasi.ERRNO_BADF
      },

      fd_filestat_set_times: (
        fd: number,
        atim: bigint,
        mtim: bigint,
        fst_flags: number,
      ): number => {
        if (this.fds[fd] != undefined) {
          return this.fds[fd]!.fd_filestat_set_times(atim, mtim, fst_flags)
        }
        return wasi.ERRNO_BADF
      },

      fd_pread: (
        fd: number,
        iovs_ptr: number,
        iovs_len: number,
        offset: bigint,
        nread_ptr: number,
      ): number => {
        const buffer = new DataView(this.inst.exports.memory.buffer)
        const buffer8 = new Uint8Array(this.inst.exports.memory.buffer)
        if (this.fds[fd] != undefined) {
          const iovecs = wasi.Iovec.read_bytes_array(buffer, iovs_ptr, iovs_len)
          let nread = 0
          for (const iovec of iovecs) {
            const { ret, data } = this.fds[fd]!.fd_pread(iovec.buf_len, offset)
            if (ret != wasi.ERRNO_SUCCESS) {
              buffer.setUint32(nread_ptr, nread, true)
              return ret
            }
            buffer8.set(data, iovec.buf)
            nread += data.length
            offset += BigInt(data.length)
            if (data.length != iovec.buf_len) {
              break
            }
          }
          buffer.setUint32(nread_ptr, nread, true)
          return wasi.ERRNO_SUCCESS
        }
        return wasi.ERRNO_BADF
      },

      fd_prestat_get: (fd: number, buf_ptr: number): number => {
        const buffer = new DataView(this.inst.exports.memory.buffer)
        if (this.fds[fd] != undefined) {
          const { ret, prestat } = this.fds[fd]!.fd_prestat_get()
          if (prestat != null) {
            prestat.write_bytes(buffer, buf_ptr)
          }
          return ret
        }
        return wasi.ERRNO_BADF
      },

      fd_prestat_dir_name: (
        fd: number,
        path_ptr: number,
        path_len: number,
      ): number => {
        if (this.fds[fd] != undefined) {
          const { ret, prestat } = this.fds[fd]!.fd_prestat_get()
          if (prestat == null) {
            return ret
          }
          const prestat_dir_name = prestat.inner.pr_name
          const buffer8 = new Uint8Array(this.inst.exports.memory.buffer)
          buffer8.set(prestat_dir_name.slice(0, path_len), path_ptr)
          return prestat_dir_name.byteLength > path_len ?
              wasi.ERRNO_NAMETOOLONG
            : wasi.ERRNO_SUCCESS
        }
        return wasi.ERRNO_BADF
      },

      fd_pwrite: (
        fd: number,
        iovs_ptr: number,
        iovs_len: number,
        offset: bigint,
        nwritten_ptr: number,
      ): number => {
        const buffer = new DataView(this.inst.exports.memory.buffer)
        const buffer8 = new Uint8Array(this.inst.exports.memory.buffer)
        if (this.fds[fd] != undefined) {
          const iovecs = wasi.Ciovec.read_bytes_array(
            buffer,
            iovs_ptr,
            iovs_len,
          )
          let nwritten = 0
          for (const iovec of iovecs) {
            const data = buffer8.slice(iovec.buf, iovec.buf + iovec.buf_len)
            const { ret, nwritten: nwritten_part } = this.fds[fd]!.fd_pwrite(
              data,
              offset,
            )
            if (ret != wasi.ERRNO_SUCCESS) {
              buffer.setUint32(nwritten_ptr, nwritten, true)
              return ret
            }
            nwritten += nwritten_part
            offset += BigInt(nwritten_part)
            if (nwritten_part != data.byteLength) {
              break
            }
          }
          buffer.setUint32(nwritten_ptr, nwritten, true)
          return wasi.ERRNO_SUCCESS
        }
        return wasi.ERRNO_BADF
      },

      fd_read: (
        fd: number,
        iovs_ptr: number,
        iovs_len: number,
        nread_ptr: number,
      ): number => {
        const buffer = new DataView(this.inst.exports.memory.buffer)
        const buffer8 = new Uint8Array(this.inst.exports.memory.buffer)
        if (this.fds[fd] != undefined) {
          const iovecs = wasi.Iovec.read_bytes_array(buffer, iovs_ptr, iovs_len)
          let nread = 0
          for (const iovec of iovecs) {
            const { ret, data } = this.fds[fd]!.fd_read(iovec.buf_len)
            if (ret != wasi.ERRNO_SUCCESS) {
              buffer.setUint32(nread_ptr, nread, true)
              return ret
            }
            buffer8.set(data, iovec.buf)
            nread += data.length
            if (data.length != iovec.buf_len) {
              break
            }
          }
          buffer.setUint32(nread_ptr, nread, true)
          return wasi.ERRNO_SUCCESS
        }
        return wasi.ERRNO_BADF
      },

      fd_readdir: (
        fd: number,
        buf: number,
        buf_len: number,
        cookie: bigint,
        bufused_ptr: number,
      ): number => {
        const buffer = new DataView(this.inst.exports.memory.buffer)
        const buffer8 = new Uint8Array(this.inst.exports.memory.buffer)
        if (this.fds[fd] != undefined) {
          let bufused = 0

          for (;;) {
            const { ret, dirent } = this.fds[fd]!.fd_readdir_single(cookie)
            if (ret != 0) {
              buffer.setUint32(bufused_ptr, bufused, true)
              return ret
            }
            if (dirent == null) {
              break
            }

            if (buf_len - bufused < dirent.head_length()) {
              bufused = buf_len
              break
            }

            const head_bytes = new ArrayBuffer(dirent.head_length())
            dirent.write_head_bytes(new DataView(head_bytes), 0)
            buffer8.set(
              new Uint8Array(head_bytes).slice(
                0,
                Math.min(head_bytes.byteLength, buf_len - bufused),
              ),
              buf,
            )
            buf += dirent.head_length()
            bufused += dirent.head_length()

            if (buf_len - bufused < dirent.name_length()) {
              bufused = buf_len
              break
            }

            dirent.write_name_bytes(buffer8, buf, buf_len - bufused)
            buf += dirent.name_length()
            bufused += dirent.name_length()

            cookie = dirent.d_next
          }

          buffer.setUint32(bufused_ptr, bufused, true)
          return 0
        }
        return wasi.ERRNO_BADF
      },

      fd_renumber: (fd: number, to: number) => {
        if (this.fds[fd] != undefined && this.fds[to] != undefined) {
          const ret = this.fds[to]!.fd_close()
          if (ret != 0) {
            return ret
          }
          this.fds[to] = this.fds[fd]
          this.fds[fd] = undefined
          this.#freeFds.push(fd)
          return 0
        }
        return wasi.ERRNO_BADF
      },

      fd_seek: (
        fd: number,
        offset: bigint,
        whence: number,
        offset_out_ptr: number,
      ): number => {
        const buffer = new DataView(this.inst.exports.memory.buffer)
        if (this.fds[fd] != undefined) {
          const { ret, offset: offset_out } = this.fds[fd]!.fd_seek(
            offset,
            whence,
          )
          buffer.setBigInt64(offset_out_ptr, offset_out, true)
          return ret
        }
        return wasi.ERRNO_BADF
      },

      fd_sync: (fd: number): number => {
        if (this.fds[fd] != undefined) {
          return this.fds[fd]!.fd_sync()
        }
        return wasi.ERRNO_BADF
      },

      fd_tell: (fd: number, offset_ptr: number): number => {
        const buffer = new DataView(this.inst.exports.memory.buffer)
        if (this.fds[fd] != undefined) {
          const { ret, offset } = this.fds[fd]!.fd_tell()
          buffer.setBigUint64(offset_ptr, offset, true)
          return ret
        }
        return wasi.ERRNO_BADF
      },

      fd_write: (
        fd: number,
        iovs_ptr: number,
        iovs_len: number,
        nwritten_ptr: number,
      ): number => {
        const buffer = new DataView(this.inst.exports.memory.buffer)
        const buffer8 = new Uint8Array(this.inst.exports.memory.buffer)
        if (this.fds[fd] != undefined) {
          const iovecs = wasi.Ciovec.read_bytes_array(
            buffer,
            iovs_ptr,
            iovs_len,
          )
          let nwritten = 0
          for (const iovec of iovecs) {
            const data = buffer8.slice(iovec.buf, iovec.buf + iovec.buf_len)
            const { ret, nwritten: nwritten_part } =
              this.fds[fd]!.fd_write(data)
            if (ret != wasi.ERRNO_SUCCESS) {
              buffer.setUint32(nwritten_ptr, nwritten, true)
              return ret
            }
            nwritten += nwritten_part
            if (nwritten_part != data.byteLength) {
              break
            }
          }
          buffer.setUint32(nwritten_ptr, nwritten, true)
          return wasi.ERRNO_SUCCESS
        }
        return wasi.ERRNO_BADF
      },

      path_create_directory: (
        fd: number,
        path_ptr: number,
        path_len: number,
      ): number => {
        const buffer8 = new Uint8Array(this.inst.exports.memory.buffer)
        if (this.fds[fd] != undefined) {
          const path = new TextDecoder('utf-8').decode(
            buffer8.slice(path_ptr, path_ptr + path_len),
          )
          return this.fds[fd]!.path_create_directory(path)
        }
        return wasi.ERRNO_BADF
      },

      path_filestat_get: (
        fd: number,
        flags: number,
        path_ptr: number,
        path_len: number,
        filestat_ptr: number,
      ): number => {
        const buffer = new DataView(this.inst.exports.memory.buffer)
        const buffer8 = new Uint8Array(this.inst.exports.memory.buffer)
        if (this.fds[fd] != undefined) {
          const path = new TextDecoder('utf-8').decode(
            buffer8.slice(path_ptr, path_ptr + path_len),
          )
          const { ret, filestat } = this.fds[fd]!.path_filestat_get(flags, path)
          if (filestat != null) {
            filestat.write_bytes(buffer, filestat_ptr)
          }
          return ret
        }
        return wasi.ERRNO_BADF
      },

      path_filestat_set_times: (
        fd: number,
        flags: number,
        path_ptr: number,
        path_len: number,
        atim: bigint,
        mtim: bigint,
        fst_flags: number,
      ) => {
        const buffer8 = new Uint8Array(this.inst.exports.memory.buffer)
        if (this.fds[fd] != undefined) {
          const path = new TextDecoder('utf-8').decode(
            buffer8.slice(path_ptr, path_ptr + path_len),
          )
          return this.fds[fd]!.path_filestat_set_times(
            flags,
            path,
            atim,
            mtim,
            fst_flags,
          )
        }
        return wasi.ERRNO_BADF
      },

      path_link: (
        old_fd: number,
        old_flags: number,
        old_path_ptr: number,
        old_path_len: number,
        new_fd: number,
        new_path_ptr: number,
        new_path_len: number,
      ): number => {
        const buffer8 = new Uint8Array(this.inst.exports.memory.buffer)
        if (this.fds[old_fd] != undefined && this.fds[new_fd] != undefined) {
          const old_path = new TextDecoder('utf-8').decode(
            buffer8.slice(old_path_ptr, old_path_ptr + old_path_len),
          )
          const new_path = new TextDecoder('utf-8').decode(
            buffer8.slice(new_path_ptr, new_path_ptr + new_path_len),
          )
          const { ret, inode_obj } = this.fds[old_fd]!.path_lookup(
            old_path,
            old_flags,
          )
          if (inode_obj == null) {
            return ret
          }
          return this.fds[new_fd]!.path_link(new_path, inode_obj, false)
        }
        return wasi.ERRNO_BADF
      },

      path_open: (
        fd: number,
        dirflags: number,
        path_ptr: number,
        path_len: number,
        oflags: number,
        fs_rights_base: bigint,
        fs_rights_inheriting: bigint,
        fd_flags: number,
        opened_fd_ptr: number,
      ): number => {
        const buffer = new DataView(this.inst.exports.memory.buffer)
        const buffer8 = new Uint8Array(this.inst.exports.memory.buffer)
        if (this.fds[fd] != undefined) {
          const path = new TextDecoder('utf-8').decode(
            buffer8.slice(path_ptr, path_ptr + path_len),
          )
          this.log('path_open', path)
          const { ret, fd_obj } = this.fds[fd]!.path_open(
            dirflags,
            path,
            oflags,
            fs_rights_base,
            fs_rights_inheriting,
            fd_flags,
          )
          if (ret != 0) {
            return ret
          }
          const opened_fd = (() => {
            if (this.#freeFds.length > 0) {
              const fd = this.#freeFds.pop()!
              this.fds[fd] = fd_obj!
              return fd
            }
            this.fds.push(fd_obj!)
            return this.fds.length - 1
          })()
          buffer.setUint32(opened_fd_ptr, opened_fd, true)
          return 0
        }
        return wasi.ERRNO_BADF
      },

      path_readlink: (
        fd: number,
        path_ptr: number,
        path_len: number,
        buf_ptr: number,
        buf_len: number,
        nread_ptr: number,
      ): number => {
        const buffer = new DataView(this.inst.exports.memory.buffer)
        const buffer8 = new Uint8Array(this.inst.exports.memory.buffer)
        if (this.fds[fd] != undefined) {
          const path = new TextDecoder('utf-8').decode(
            buffer8.slice(path_ptr, path_ptr + path_len),
          )
          const { ret, data } = this.fds[fd]!.path_readlink(path)
          if (data != null) {
            const data_buf = new TextEncoder().encode(data)
            if (data_buf.length > buf_len) {
              buffer.setUint32(nread_ptr, 0, true)
              return wasi.ERRNO_BADF
            }
            buffer8.set(data_buf, buf_ptr)
            buffer.setUint32(nread_ptr, data_buf.length, true)
          }
          return ret
        }
        return wasi.ERRNO_BADF
      },

      path_remove_directory: (
        fd: number,
        path_ptr: number,
        path_len: number,
      ): number => {
        const buffer8 = new Uint8Array(this.inst.exports.memory.buffer)
        if (this.fds[fd] != undefined) {
          const path = new TextDecoder('utf-8').decode(
            buffer8.slice(path_ptr, path_ptr + path_len),
          )
          return this.fds[fd]!.path_remove_directory(path)
        }
        return wasi.ERRNO_BADF
      },

      path_rename: (
        fd: number,
        old_path_ptr: number,
        old_path_len: number,
        new_fd: number,
        new_path_ptr: number,
        new_path_len: number,
      ): number => {
        const buffer8 = new Uint8Array(this.inst.exports.memory.buffer)
        if (this.fds[fd] != undefined && this.fds[new_fd] != undefined) {
          const old_path = new TextDecoder('utf-8').decode(
            buffer8.slice(old_path_ptr, old_path_ptr + old_path_len),
          )
          const new_path = new TextDecoder('utf-8').decode(
            buffer8.slice(new_path_ptr, new_path_ptr + new_path_len),
          )
          const { ret: unlinkRet, inode_obj } =
            this.fds[fd]!.path_unlink(old_path)
          if (inode_obj == null) {
            return unlinkRet
          }
          const linkRet = this.fds[new_fd]!.path_link(new_path, inode_obj, true)
          if (linkRet != wasi.ERRNO_SUCCESS) {
            if (
              this.fds[fd]!.path_link(old_path, inode_obj, true) !=
              wasi.ERRNO_SUCCESS
            ) {
              throw 'path_link should always return success when relinking an inode back to the original place'
            }
          }
          return linkRet
        }
        return wasi.ERRNO_BADF
      },

      path_symlink: (
        old_path_ptr: number,
        old_path_len: number,
        fd: number,
        _new_path_ptr: number,
        _new_path_len: number,
      ): number => {
        const buffer8 = new Uint8Array(this.inst.exports.memory.buffer)
        if (this.fds[fd] != undefined) {
          new TextDecoder('utf-8').decode(
            buffer8.slice(old_path_ptr, old_path_ptr + old_path_len),
          )
          return wasi.ERRNO_NOTSUP
        }
        return wasi.ERRNO_BADF
      },

      path_unlink_file: (
        fd: number,
        path_ptr: number,
        path_len: number,
      ): number => {
        const buffer8 = new Uint8Array(this.inst.exports.memory.buffer)
        if (this.fds[fd] != undefined) {
          const path = new TextDecoder('utf-8').decode(
            buffer8.slice(path_ptr, path_ptr + path_len),
          )
          return this.fds[fd]!.path_unlink_file(path)
        }
        return wasi.ERRNO_BADF
      },

      /**
       * poll_oneoff - Poll for events on multiple subscriptions.
       *
       * This implementation supports:
       * - Clock subscriptions (sleep) - uses Atomics.wait for blocking
       * - FD_READ subscriptions (poll for readable data)
       * - FD_WRITE subscriptions (poll for writeable)
       *
       * For fd subscriptions, we call fd_poll() on the Fd object.
       *
       * When used with SAB-backed fds (SABPollableStdin, SABDevOut), this uses
       * Atomics.wait for true blocking instead of busy-waiting. This is critical
       * for the two-SharedWorker architecture where the QuickJS worker must block
       * while waiting for I/O, allowing the async I/O worker to process data.
       */
      poll_oneoff: (
        in_ptr: number,
        out_ptr: number,
        nsubscriptions: number,
        nevents_ptr: number,
      ): number => {
        if (nsubscriptions === 0) {
          return wasi.ERRNO_INVAL
        }

        const buffer = new DataView(this.inst.exports.memory.buffer)

        // Read all subscriptions
        const subscriptions: wasi.Subscription[] = []
        for (let i = 0; i < nsubscriptions; i++) {
          subscriptions.push(
            wasi.Subscription.read_bytes(
              buffer,
              in_ptr + i * wasi.Subscription.size(),
            ),
          )
        }

        this.log('poll_oneoff: subscriptions', subscriptions)

        // Process subscriptions and generate events
        const events: wasi.Event[] = []
        let clockTimeout: bigint | null = null
        let clockUserdata: bigint = 0n

        for (const s of subscriptions) {
          if (s.eventtype === wasi.EVENTTYPE_CLOCK) {
            // Calculate absolute end time
            let getNow: () => bigint
            if (s.clockid === wasi.CLOCKID_MONOTONIC) {
              getNow = () => BigInt(Math.round(performance.now() * 1_000_000))
            } else if (s.clockid === wasi.CLOCKID_REALTIME) {
              getNow = () => BigInt(new Date().getTime()) * 1_000_000n
            } else {
              events.push(
                new wasi.Event(s.userdata, wasi.ERRNO_INVAL, s.eventtype),
              )
              continue
            }

            const endTime =
              (s.flags & wasi.SUBCLOCKFLAGS_SUBSCRIPTION_CLOCK_ABSTIME) !== 0 ?
                s.timeout
              : getNow() + s.timeout

            // Track the earliest clock timeout
            if (clockTimeout === null || endTime < clockTimeout) {
              clockTimeout = endTime
              clockUserdata = s.userdata
            }
          } else if (
            s.eventtype === wasi.EVENTTYPE_FD_READ ||
            s.eventtype === wasi.EVENTTYPE_FD_WRITE
          ) {
            // Check if fd is valid
            if (this.fds[s.fd] == undefined) {
              events.push(
                new wasi.Event(s.userdata, wasi.ERRNO_BADF, s.eventtype),
              )
              continue
            }

            // Poll the fd
            const pollResult = this.fds[s.fd]!.fd_poll(s.eventtype)
            this.log('poll_oneoff: fd', s.fd, 'poll result', pollResult)

            if (pollResult.ready) {
              events.push(
                new wasi.Event(
                  s.userdata,
                  wasi.ERRNO_SUCCESS,
                  s.eventtype,
                  pollResult.nbytes,
                  pollResult.flags,
                ),
              )
            }
          } else {
            events.push(
              new wasi.Event(s.userdata, wasi.ERRNO_NOTSUP, s.eventtype),
            )
          }
        }

        // If no events are ready yet, we need to wait
        if (events.length === 0) {
          // Calculate timeout in milliseconds for Atomics.wait
          let timeoutMs: number | undefined
          if (clockTimeout !== null) {
            const getNow =
              (
                subscriptions.find((s) => s.eventtype === wasi.EVENTTYPE_CLOCK)
                  ?.clockid === wasi.CLOCKID_REALTIME
              ) ?
                () => BigInt(new Date().getTime()) * 1_000_000n
              : () => BigInt(Math.round(performance.now() * 1_000_000))

            const remaining = clockTimeout - getNow()
            if (remaining <= 0n) {
              // Clock already expired
              events.push(
                new wasi.Event(
                  clockUserdata,
                  wasi.ERRNO_SUCCESS,
                  wasi.EVENTTYPE_CLOCK,
                ),
              )
            } else {
              // Convert nanoseconds to milliseconds
              timeoutMs = Number(remaining / 1_000_000n)
            }
          }

          // If we still have no events, use Atomics.wait for true blocking
          // This is a dummy wait that will time out, but it releases the thread
          // to allow the async I/O worker to process data and wake us up
          if (events.length === 0) {
            // Create a dummy SharedArrayBuffer for Atomics.wait if needed
            // This allows proper blocking instead of busy-waiting
            const waitBuffer = new Int32Array(new SharedArrayBuffer(4))

            // Wait with timeout - this blocks the thread properly
            // During this time, the async I/O worker can write to SAB and notify
            const waitResult = Atomics.wait(waitBuffer, 0, 0, timeoutMs ?? 100)

            // After waking (timeout or notify), check fds again
            for (const s of subscriptions) {
              if (
                s.eventtype === wasi.EVENTTYPE_FD_READ ||
                s.eventtype === wasi.EVENTTYPE_FD_WRITE
              ) {
                if (this.fds[s.fd] != undefined) {
                  const pollResult = this.fds[s.fd]!.fd_poll(s.eventtype)
                  if (pollResult.ready) {
                    events.push(
                      new wasi.Event(
                        s.userdata,
                        wasi.ERRNO_SUCCESS,
                        s.eventtype,
                        pollResult.nbytes,
                        pollResult.flags,
                      ),
                    )
                  }
                }
              }
            }

            // If clock timed out and still no fd events, report clock event
            if (
              events.length === 0 &&
              clockTimeout !== null &&
              waitResult === 'timed-out'
            ) {
              events.push(
                new wasi.Event(
                  clockUserdata,
                  wasi.ERRNO_SUCCESS,
                  wasi.EVENTTYPE_CLOCK,
                ),
              )
            }

            // If still no events after first wait, loop with shorter intervals
            // This handles the case where SAB notification arrives between checks
            let loopCount = 0
            while (events.length === 0 && loopCount < 10) {
              loopCount++

              // Short wait to allow notifications to arrive
              Atomics.wait(waitBuffer, 0, 0, 10)

              // Check fds again
              for (const s of subscriptions) {
                if (
                  s.eventtype === wasi.EVENTTYPE_FD_READ ||
                  s.eventtype === wasi.EVENTTYPE_FD_WRITE
                ) {
                  if (this.fds[s.fd] != undefined) {
                    const pollResult = this.fds[s.fd]!.fd_poll(s.eventtype)
                    if (pollResult.ready) {
                      events.push(
                        new wasi.Event(
                          s.userdata,
                          wasi.ERRNO_SUCCESS,
                          s.eventtype,
                          pollResult.nbytes,
                          pollResult.flags,
                        ),
                      )
                    }
                  }
                }
              }

              // Check if clock expired
              if (clockTimeout !== null) {
                const getNow =
                  (
                    subscriptions.find(
                      (s) => s.eventtype === wasi.EVENTTYPE_CLOCK,
                    )?.clockid === wasi.CLOCKID_REALTIME
                  ) ?
                    () => BigInt(new Date().getTime()) * 1_000_000n
                  : () => BigInt(Math.round(performance.now() * 1_000_000))

                if (getNow() >= clockTimeout) {
                  events.push(
                    new wasi.Event(
                      clockUserdata,
                      wasi.ERRNO_SUCCESS,
                      wasi.EVENTTYPE_CLOCK,
                    ),
                  )
                  break
                }
              }
            }
          }
        }

        // Write events to output buffer
        for (let i = 0; i < events.length; i++) {
          events[i].write_bytes(buffer, out_ptr + i * wasi.Event.size())
        }

        // Write number of events
        buffer.setUint32(nevents_ptr, events.length, true)

        this.log('poll_oneoff: returning', events.length, 'events')
        return wasi.ERRNO_SUCCESS
      },

      proc_exit: (exit_code: number) => {
        throw new WASIProcExit(exit_code)
      },

      proc_raise: (sig: number) => {
        throw 'raised signal ' + sig
      },

      sched_yield: () => {},

      random_get: (buf: number, buf_len: number) => {
        const buffer8 = new Uint8Array(
          this.inst.exports.memory.buffer,
        ).subarray(buf, buf + buf_len)

        if (
          'crypto' in globalThis &&
          (typeof SharedArrayBuffer === 'undefined' ||
            !(this.inst.exports.memory.buffer instanceof SharedArrayBuffer))
        ) {
          for (let i = 0; i < buf_len; i += 65536) {
            crypto.getRandomValues(buffer8.subarray(i, i + 65536))
          }
        } else {
          for (let i = 0; i < buf_len; i++) {
            buffer8[i] = (Math.random() * 256) | 0
          }
        }
      },

      sock_recv: (_fd: number, _ri_data: unknown, _ri_flags: unknown) => {
        throw 'sockets not supported'
      },

      sock_send: (_fd: number, _si_data: unknown, _si_flags: unknown) => {
        throw 'sockets not supported'
      },

      sock_shutdown: (_fd: number, _how: unknown) => {
        throw 'sockets not supported'
      },

      sock_accept: (_fd: number, _flags: unknown) => {
        throw 'sockets not supported'
      },
    }
  }
}

export { WASI }
