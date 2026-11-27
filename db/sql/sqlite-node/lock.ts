import { mkdirSync, realpathSync } from 'node:fs'
import { join } from 'node:path'
import { DatabaseSync } from 'node:sqlite'

// lockFileName identifies the lock database file inside a locked directory.
// The file is persistent ownership metadata: it carries the OS lock only while
// a writer holds its connection open, so it is created once and never deleted.
const lockFileName = '.spacewave-writer.db'

// DirectoryWriterLock is one directory's writer slot, held for the lifetime of
// the engine Worker that acquired it.
export interface DirectoryWriterLock {
  // directory is the canonical physical directory this lock admits one writer to.
  directory: string

  // close releases the lock by closing the held connection. SQLite rolls back
  // the open transaction and releases OS locks on close. The method is
  // idempotent.
  close(): void
}

// acquireDirectoryLock admits exactly one writer to a physical directory. It
// creates directory when missing, canonicalizes it with realpathSync, and
// holds BEGIN EXCLUSIVE on a dedicated .spacewave-writer.db with
// busy_timeout=0 for the calling process or Worker lifetime. A second
// acquisition for the same physical directory, including through a symlink
// alias, fails promptly. SQLite releases the OS lock when the connection
// closes or the process or Worker dies, so ownership needs no stale-file
// detection or timers.
export function acquireDirectoryLock(directory: string): DirectoryWriterLock {
  mkdirSync(directory, { recursive: true })
  const canonical = realpathSync(directory)
  const db = new DatabaseSync(join(canonical, lockFileName))
  try {
    db.exec('PRAGMA busy_timeout = 0')
    db.exec('BEGIN EXCLUSIVE')
  } catch (err) {
    db.close()
    const message = err instanceof Error ? err.message : String(err)
    throw new Error(
      `acquire directory writer lock for ${canonical}: ${message}`,
      { cause: err },
    )
  }
  let closed = false
  return {
    directory: canonical,
    close(): void {
      if (closed) return
      db.close()
      closed = true
    },
  }
}
