// License for this file: MIT License
// Derived from: https://github.com/bjorn3/browser_wasi_shim

import * as wasi from './wasi_defs.js'

/**
 * PollResult is returned by fd_poll to indicate readiness for I/O.
 */
export interface PollResult {
  /** Whether the fd is ready for the requested operation */
  ready: boolean
  /** Number of bytes available to read (for FD_READ) */
  nbytes: bigint
  /** Any flags to set on the event (e.g., HANGUP) */
  flags: number
}

export abstract class Fd {
  fd_allocate(_offset: bigint, _len: bigint): number {
    return wasi.ERRNO_NOTSUP
  }
  fd_close(): number {
    return 0
  }
  fd_fdstat_get(): { ret: number; fdstat: wasi.Fdstat | null } {
    return { ret: wasi.ERRNO_NOTSUP, fdstat: null }
  }
  fd_fdstat_set_flags(_flags: number): number {
    return wasi.ERRNO_NOTSUP
  }
  fd_fdstat_set_rights(_fs_rights_base: bigint, _fs_rights_inheriting: bigint): number {
    return wasi.ERRNO_NOTSUP
  }
  fd_filestat_get(): { ret: number; filestat: wasi.Filestat | null } {
    return { ret: wasi.ERRNO_NOTSUP, filestat: null }
  }
  fd_filestat_set_size(_size: bigint): number {
    return wasi.ERRNO_NOTSUP
  }
  fd_filestat_set_times(_atim: bigint, _mtim: bigint, _fst_flags: number): number {
    return wasi.ERRNO_NOTSUP
  }
  fd_pread(_size: number, _offset: bigint): { ret: number; data: Uint8Array } {
    return { ret: wasi.ERRNO_NOTSUP, data: new Uint8Array() }
  }
  fd_prestat_get(): { ret: number; prestat: wasi.Prestat | null } {
    return { ret: wasi.ERRNO_NOTSUP, prestat: null }
  }
  fd_pwrite(_data: Uint8Array, _offset: bigint): { ret: number; nwritten: number } {
    return { ret: wasi.ERRNO_NOTSUP, nwritten: 0 }
  }
  fd_read(_size: number): { ret: number; data: Uint8Array } {
    return { ret: wasi.ERRNO_NOTSUP, data: new Uint8Array() }
  }
  fd_readdir_single(_cookie: bigint): {
    ret: number
    dirent: wasi.Dirent | null
  } {
    return { ret: wasi.ERRNO_NOTSUP, dirent: null }
  }
  fd_seek(_offset: bigint, _whence: number): { ret: number; offset: bigint } {
    return { ret: wasi.ERRNO_NOTSUP, offset: 0n }
  }
  fd_sync(): number {
    return 0
  }
  fd_tell(): { ret: number; offset: bigint } {
    return { ret: wasi.ERRNO_NOTSUP, offset: 0n }
  }
  fd_write(_data: Uint8Array): { ret: number; nwritten: number } {
    return { ret: wasi.ERRNO_NOTSUP, nwritten: 0 }
  }
  path_create_directory(_path: string): number {
    return wasi.ERRNO_NOTSUP
  }
  path_filestat_get(_flags: number, _path: string): { ret: number; filestat: wasi.Filestat | null } {
    return { ret: wasi.ERRNO_NOTSUP, filestat: null }
  }
  path_filestat_set_times(
    _flags: number,
    _path: string,
    _atim: bigint,
    _mtim: bigint,
    _fst_flags: number,
  ): number {
    return wasi.ERRNO_NOTSUP
  }
  path_link(_path: string, _inode: Inode, _allow_dir: boolean): number {
    return wasi.ERRNO_NOTSUP
  }
  path_unlink(_path: string): { ret: number; inode_obj: Inode | null } {
    return { ret: wasi.ERRNO_NOTSUP, inode_obj: null }
  }
  path_lookup(_path: string, _dirflags: number): { ret: number; inode_obj: Inode | null } {
    return { ret: wasi.ERRNO_NOTSUP, inode_obj: null }
  }
  path_open(
    _dirflags: number,
    _path: string,
    _oflags: number,
    _fs_rights_base: bigint,
    _fs_rights_inheriting: bigint,
    _fd_flags: number,
  ): { ret: number; fd_obj: Fd | null } {
    return { ret: wasi.ERRNO_NOTDIR, fd_obj: null }
  }
  path_readlink(_path: string): { ret: number; data: string | null } {
    return { ret: wasi.ERRNO_NOTSUP, data: null }
  }
  path_remove_directory(_path: string): number {
    return wasi.ERRNO_NOTSUP
  }
  path_rename(_old_path: string, _new_fd: number, _new_path: string): number {
    return wasi.ERRNO_NOTSUP
  }
  path_unlink_file(_path: string): number {
    return wasi.ERRNO_NOTSUP
  }

  /**
   * Poll for I/O readiness. Override this in subclasses that support polling.
   * @param eventtype EVENTTYPE_FD_READ or EVENTTYPE_FD_WRITE
   * @returns PollResult indicating readiness
   */
  fd_poll(_eventtype: number): PollResult {
    // Default: not ready (override in pollable Fd implementations)
    return { ready: false, nbytes: 0n, flags: 0 }
  }
}

export abstract class Inode {
  ino: bigint

  constructor() {
    this.ino = Inode.issue_ino()
  }

  // NOTE: ino 0 is reserved for the root directory
  private static next_ino: bigint = 1n
  static issue_ino(): bigint {
    return Inode.next_ino++
  }
  static root_ino(): bigint {
    return 0n
  }

  abstract path_open(
    oflags: number,
    fs_rights_base: bigint,
    fd_flags: number,
  ): { ret: number; fd_obj: Fd | null }

  abstract stat(): wasi.Filestat
}
