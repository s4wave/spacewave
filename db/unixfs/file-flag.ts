// File open flag constants matching POSIX/Go os package values.
export const O_RDONLY = 0
export const O_WRONLY = 1
export const O_RDWR = 2
export const O_APPEND = 0x400
export const O_CREATE = 0x40
export const O_EXCL = 0x80
export const O_TRUNC = 0x200

// flagIsCreate returns true if the create flag is set.
export function flagIsCreate(flag: number): boolean {
  return (flag & O_CREATE) !== 0
}

// flagIsExclusive returns true if the exclusive flag is set.
export function flagIsExclusive(flag: number): boolean {
  return (flag & O_EXCL) !== 0
}

// flagIsReadOnly returns true if the flag is read-only.
export function flagIsReadOnly(flag: number): boolean {
  return flag === O_RDONLY
}

// flagIsAppend returns true if the append flag is set.
export function flagIsAppend(flag: number): boolean {
  return (flag & O_APPEND) !== 0
}

// flagIsTruncate returns true if the truncate flag is set.
export function flagIsTruncate(flag: number): boolean {
  return (flag & O_TRUNC) !== 0
}

// flagIsReadAndWrite returns true if the read-write flag is set.
export function flagIsReadAndWrite(flag: number): boolean {
  return (flag & O_RDWR) !== 0
}

// flagIsWriteOnly returns true if the write-only flag is set.
export function flagIsWriteOnly(flag: number): boolean {
  return (flag & O_WRONLY) !== 0
}
