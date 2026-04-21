// ModeDir is the directory bit in a file mode (matching Go os.ModeDir).
const ModeDir = 0x80000000

// FileInfo contains information about a file.
export class FileInfo {
  private readonly name: string
  private readonly size: bigint
  private readonly mode: number
  private readonly modTime: Date

  constructor(name: string, size: bigint, mode: number, modTime: Date) {
    this.name = name
    this.size = size
    this.mode = mode
    this.modTime = modTime
  }

  // getName returns the name of the file.
  getName(): string {
    return this.name
  }

  // getSize returns the length in bytes for regular files.
  getSize(): bigint {
    return this.size
  }

  // getMode returns the unixfs file mode bitset.
  getMode(): number {
    return this.mode
  }

  // getModTime returns the modification time.
  getModTime(): Date {
    return this.modTime
  }

  // isDir returns true if the mode indicates a directory.
  isDir(): boolean {
    return (this.mode & ModeDir) !== 0
  }
}
