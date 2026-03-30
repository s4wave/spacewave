import { describe, it, expect } from 'vitest'
import path from 'node:path'
import url from 'node:url'
import fs from 'node:fs'

const __dirname = url.fileURLToPath(new URL('.', import.meta.url))

// V86_DIR: directory containing build/v86-debug.wasm, bios/, src/main.js.
// Set via env var or defaults to the v86 repo checkout for local dev.
const V86_DIR = path.resolve(
  process.env.V86_DIR ?? path.resolve(__dirname, '../../../../repos/v86'),
)

// V86FS_DIR: directory containing bzImage, fs.json, flat/.
// Set via env var or defaults to the wasivm prototype.
const V86FS_DIR = path.resolve(
  process.env.V86FS_DIR ??
    path.resolve(__dirname, '../../../../repos/wasivm/prototypes/debian-v86'),
)

const HAS_WASM =
  fs.existsSync(path.join(V86_DIR, 'build/v86-debug.wasm')) &&
  fs.existsSync(path.join(V86_DIR, 'bios/seabios.bin'))
const HAS_ROOTFS =
  fs.existsSync(path.join(V86FS_DIR, 'bzImage')) &&
  fs.existsSync(path.join(V86FS_DIR, 'fs.json'))
const HAS_ASSETS = HAS_WASM && HAS_ROOTFS

// Import V86 from the v86 source (same as v86 repo's own tests).
const { V86 } = HAS_ASSETS
  ? await import(path.join(V86_DIR, 'src/main.js'))
  : ({ V86: undefined } as any)

// Import V86FSAdapter types for v86fs tests.
const v86fsModule = HAS_ASSETS
  ? await import(path.join(V86_DIR, 'src/virtio_v86fs.js'))
  : ({} as any)

// Patch fetch to support file:// URLs and local paths for bun/node.
const _origFetch = globalThis.fetch
globalThis.fetch = async (input: any, init?: any) => {
  const u = typeof input === 'string' ? input : input.url
  if (u.startsWith('file://')) {
    const filePath = url.fileURLToPath(u)
    const data = fs.readFileSync(filePath)
    return new Response(data)
  }
  if (u.startsWith('/')) {
    const data = fs.readFileSync(u)
    return new Response(data)
  }
  return _origFetch(input, init)
}

// Strip ANSI escape codes from serial output.
const ANSI_RE = /\x1b\[[0-9;?]*[a-zA-Z]/g
function stripAnsi(s: string): string {
  return s.replace(ANSI_RE, '').replace(/\r/g, '')
}

// Wait for a marker string in serial output.
function waitForSerial(
  emulator: any,
  marker: string,
  timeoutMs = 120_000,
): Promise<string> {
  return new Promise((resolve, reject) => {
    let buf = ''
    const timer = setTimeout(() => {
      reject(
        new Error(
          `Timed out waiting for "${marker}". Got:\n${buf.slice(-500)}`,
        ),
      )
    }, timeoutMs)

    function onByte(byte: number): void {
      buf += String.fromCharCode(byte)
      if (buf.includes(marker)) {
        clearTimeout(timer)
        emulator.remove_listener('serial0-output-byte', onByte)
        resolve(buf)
      }
    }
    emulator.add_listener('serial0-output-byte', onByte)
  })
}

// Send a command via serial and wait for shell prompt.
async function runCommand(
  emulator: any,
  cmd: string,
  prompt = ':/#',
  timeoutMs = 30_000,
): Promise<string> {
  const p = waitForSerial(emulator, prompt, timeoutMs)
  emulator.serial0_send(cmd + '\n')
  const buf = await p
  const clean = stripAnsi(buf)
  const lines = clean.split('\n')
  const cmdIdx = lines.findIndex((l: string) => l.includes(cmd))
  const promptIdx = lines.findLastIndex((l: string) => l.includes(prompt))
  if (cmdIdx >= 0 && promptIdx > cmdIdx) {
    return lines
      .slice(cmdIdx + 1, promptIdx)
      .join('\n')
      .trim()
  }
  return clean
}

// Load handle9p from the V86FS_DIR (fs.json + flat/).
async function loadHandle9p(): Promise<any> {
  const serverPath = path.join(V86FS_DIR, 'handle9p-server.mjs')
  if (!fs.existsSync(serverPath)) {
    // Fall back to v86 repo's test helper.
    const fallback = path.join(V86_DIR, 'tests/v86fs/handle9p-server.mjs')
    const mod = await import(fallback)
    const fsJsonUrl = url.pathToFileURL(path.join(V86FS_DIR, 'fs.json')).href
    const flatUrl =
      url.pathToFileURL(path.join(V86FS_DIR, 'flat')).href + '/'
    return mod.createHandle9p(fsJsonUrl, flatUrl)
  }
  const mod = await import(serverPath)
  const fsJsonUrl = url.pathToFileURL(path.join(V86FS_DIR, 'fs.json')).href
  const flatUrl = url.pathToFileURL(path.join(V86FS_DIR, 'flat')).href + '/'
  return mod.createHandle9p(fsJsonUrl, flatUrl)
}

// Log serial output to stderr.
function addSerialLogger(emulator: any): void {
  let lineBuf = ''
  emulator.add_listener('serial0-output-byte', (byte: number) => {
    const ch = String.fromCharCode(byte)
    if (ch === '\n') {
      process.stderr.write('[serial] ' + lineBuf + '\n')
      lineBuf = ''
    } else if (ch !== '\r') {
      lineBuf += ch
    }
  })
}

// Create emulator with 9p rootfs only.
async function createEmulator9p(): Promise<any> {
  const handle9p = await loadHandle9p()
  const emulator = new V86({
    wasm_path: path.join(V86_DIR, 'build/v86-debug.wasm'),
    memory_size: 512 * 1024 * 1024,
    vga_memory_size: 2 * 1024 * 1024,
    bios: { url: path.join(V86_DIR, 'bios/seabios.bin') },
    vga_bios: { url: path.join(V86_DIR, 'bios/vgabios.bin') },
    bzimage: { url: path.join(V86FS_DIR, 'bzImage') },
    cmdline:
      'rw init=/usr/bin/bash root=host9p rootfstype=9p rootflags=trans=virtio,cache=loose console=ttyS0',
    filesystem: { handle9p },
    autostart: true,
  })
  addSerialLogger(emulator)
  return emulator
}

// S_IF* mode bits for adapter.
const S_IFDIR = 0o040000
const S_IFREG = 0o100000
const DT_DIR = 4
const DT_REG = 8

// Create a simple in-memory V86FSAdapter for testing.
function createMapAdapter(): any {
  const inodeMap = new Map<
    number,
    {
      name: string
      mode: number
      size: number
      dt_type: number
      mtime_sec: number
      mtime_nsec: number
      content?: Uint8Array
    }
  >()
  const dirEntries = new Map<number, number[]>()
  let nextInode = 100
  let nextHandle = 1
  const openHandles = new Map<number, number>() // handle -> inode

  // Seed root directory.
  inodeMap.set(1, {
    name: '',
    mode: S_IFDIR | 0o755,
    size: 0,
    dt_type: DT_DIR,
    mtime_sec: 1711500000,
    mtime_nsec: 0,
  })
  dirEntries.set(1, [])

  // Seed a test file.
  const testContent = new TextEncoder().encode('v86fs-test-content\n')
  const fileInode = nextInode++
  inodeMap.set(fileInode, {
    name: 'hello.txt',
    mode: S_IFREG | 0o644,
    size: testContent.length,
    dt_type: DT_REG,
    mtime_sec: 1711500000,
    mtime_nsec: 0,
    content: testContent,
  })
  dirEntries.get(1)!.push(fileInode)

  return {
    adapter: {
      onMount(
        _name: string,
        reply: (status: number, root_inode_id: number, mode: number) => void,
      ) {
        reply(0, 1, S_IFDIR | 0o755)
      },

      onLookup(
        parent_id: number,
        name: string,
        reply: (
          status: number,
          inode_id: number,
          mode: number,
          size: number,
        ) => void,
      ) {
        const children = dirEntries.get(parent_id)
        if (!children) {
          reply(2, 0, 0, 0) // ENOENT
          return
        }
        for (const childId of children) {
          const entry = inodeMap.get(childId)
          if (entry && entry.name === name) {
            reply(0, childId, entry.mode, entry.size)
            return
          }
        }
        reply(2, 0, 0, 0) // ENOENT
      },

      onGetattr(
        inode_id: number,
        reply: (
          status: number,
          mode: number,
          size: number,
          mtime_sec: number,
          mtime_nsec: number,
        ) => void,
      ) {
        const entry = inodeMap.get(inode_id)
        if (!entry) {
          reply(2, 0, 0, 0, 0)
          return
        }
        reply(0, entry.mode, entry.size, entry.mtime_sec, entry.mtime_nsec)
      },

      onReaddir(
        dir_id: number,
        reply: (
          status: number,
          entries: Array<{
            inode_id: number
            dt_type: number
            name: string
          }>,
        ) => void,
      ) {
        const children = dirEntries.get(dir_id)
        if (!children) {
          reply(2, [])
          return
        }
        const entries = children
          .map((id) => {
            const e = inodeMap.get(id)
            return e
              ? { inode_id: id, dt_type: e.dt_type, name: e.name }
              : null
          })
          .filter(Boolean) as Array<{
          inode_id: number
          dt_type: number
          name: string
        }>
        reply(0, entries)
      },

      onOpen(
        inode_id: number,
        _flags: number,
        reply: (status: number, handle_id: number) => void,
      ) {
        if (!inodeMap.has(inode_id)) {
          reply(2, 0)
          return
        }
        const h = nextHandle++
        openHandles.set(h, inode_id)
        reply(0, h)
      },

      onClose(handle_id: number, reply: (status: number) => void) {
        openHandles.delete(handle_id)
        reply(0)
      },

      onRead(
        handle_id: number,
        offset: number,
        size: number,
        reply: (status: number, data: Uint8Array) => void,
      ) {
        const inodeId = openHandles.get(handle_id)
        if (inodeId === undefined) {
          reply(9, new Uint8Array(0)) // EBADF
          return
        }
        const entry = inodeMap.get(inodeId)
        if (!entry?.content) {
          reply(0, new Uint8Array(0))
          return
        }
        const slice = entry.content.slice(offset, offset + size)
        reply(0, slice)
      },

      onCreate(
        parent_id: number,
        name: string,
        mode: number,
        reply: (status: number, inode_id: number, mode: number) => void,
      ) {
        const id = nextInode++
        const m = S_IFREG | (mode & 0o7777)
        inodeMap.set(id, {
          name,
          mode: m,
          size: 0,
          dt_type: DT_REG,
          mtime_sec: Math.floor(Date.now() / 1000),
          mtime_nsec: 0,
          content: new Uint8Array(0),
        })
        const children = dirEntries.get(parent_id) ?? []
        children.push(id)
        dirEntries.set(parent_id, children)
        reply(0, id, m)
      },

      onWrite(
        inode_id: number,
        offset: number,
        data: Uint8Array,
        reply: (status: number, bytes_written: number) => void,
      ) {
        const entry = inodeMap.get(inode_id)
        if (!entry) {
          reply(2, 0)
          return
        }
        const existing = entry.content ?? new Uint8Array(0)
        const needed = offset + data.length
        if (needed > existing.length) {
          const grown = new Uint8Array(needed)
          grown.set(existing)
          entry.content = grown
        }
        entry.content!.set(data, offset)
        entry.size = entry.content!.length
        reply(0, data.length)
      },

      onMkdir(
        parent_id: number,
        name: string,
        mode: number,
        reply: (status: number, inode_id: number, mode: number) => void,
      ) {
        const id = nextInode++
        const m = S_IFDIR | (mode & 0o7777)
        inodeMap.set(id, {
          name,
          mode: m,
          size: 0,
          dt_type: DT_DIR,
          mtime_sec: Math.floor(Date.now() / 1000),
          mtime_nsec: 0,
        })
        dirEntries.set(id, [])
        const children = dirEntries.get(parent_id) ?? []
        children.push(id)
        dirEntries.set(parent_id, children)
        reply(0, id, m)
      },

      onSetattr(
        _inode_id: number,
        _valid: number,
        _mode: number,
        _size: number,
        reply: (status: number) => void,
      ) {
        reply(0)
      },

      onFsync(_inode_id: number, reply: (status: number) => void) {
        reply(0)
      },

      onUnlink(
        parent_id: number,
        name: string,
        reply: (status: number) => void,
      ) {
        const children = dirEntries.get(parent_id)
        if (!children) {
          reply(2)
          return
        }
        const idx = children.findIndex(
          (id) => inodeMap.get(id)?.name === name,
        )
        if (idx < 0) {
          reply(2)
          return
        }
        const removed = children.splice(idx, 1)[0]
        inodeMap.delete(removed)
        reply(0)
      },

      onStatfs(
        reply: (
          status: number,
          blocks: number,
          bfree: number,
          bavail: number,
          files: number,
          ffree: number,
          bsize: number,
        ) => void,
      ) {
        reply(0, 1000000, 500000, 500000, 100000, 50000, 4096)
      },
    },
    inodeMap,
    dirEntries,
  }
}

// Create emulator with 9p rootfs + v86fs device.
async function createEmulatorV86fs(v86fsAdapter: any): Promise<any> {
  const handle9p = await loadHandle9p()
  const emulator = new V86({
    wasm_path: path.join(V86_DIR, 'build/v86-debug.wasm'),
    memory_size: 512 * 1024 * 1024,
    vga_memory_size: 2 * 1024 * 1024,
    bios: { url: path.join(V86_DIR, 'bios/seabios.bin') },
    vga_bios: { url: path.join(V86_DIR, 'bios/vgabios.bin') },
    bzimage: { url: path.join(V86FS_DIR, 'bzImage') },
    cmdline:
      'rw init=/usr/bin/bash root=host9p rootfstype=9p rootflags=trans=virtio,cache=loose console=ttyS0',
    filesystem: { handle9p },
    virtio_v86fs: true,
    virtio_v86fs_adapter: v86fsAdapter,
    autostart: true,
  })
  addSerialLogger(emulator)
  return emulator
}

// --- 9p tests (legacy path) ---

describe.runIf(HAS_ASSETS)(
  'forge v86 bun - 9p',
  { timeout: 180_000 },
  () => {
    it('boots VM and runs echo', async () => {
      const emulator = await createEmulator9p()
      try {
        await waitForSerial(emulator, ':/#', 120_000)

        const output = await runCommand(emulator, 'echo hello')
        expect(output).toContain('hello')

        const exitCode = await runCommand(emulator, 'echo $?')
        expect(exitCode.trim()).toBe('0')
      } finally {
        emulator.stop()
        emulator.destroy()
      }
    })

    it('captures non-zero exit code', async () => {
      const emulator = await createEmulator9p()
      try {
        await waitForSerial(emulator, ':/#', 120_000)

        await runCommand(emulator, 'false')
        const exitCode = await runCommand(emulator, 'echo $?')
        expect(exitCode.trim()).toBe('1')
      } finally {
        emulator.stop()
        emulator.destroy()
      }
    })

    it('runs multiple commands sequentially', async () => {
      const emulator = await createEmulator9p()
      try {
        await waitForSerial(emulator, ':/#', 120_000)

        // Use tmpfs since the 9p rootfs is read-only.
        await runCommand(emulator, 'mount -t tmpfs tmpfs /tmp')
        await runCommand(emulator, 'echo hello > /tmp/test.txt')

        const content = await runCommand(emulator, 'cat /tmp/test.txt')
        expect(content.trim()).toBe('hello')

        const exitCode = await runCommand(emulator, 'echo $?')
        expect(exitCode.trim()).toBe('0')
      } finally {
        emulator.stop()
        emulator.destroy()
      }
    })
  },
)

// --- v86fs tests (primary path) ---

describe.runIf(HAS_ASSETS)(
  'forge v86 bun - v86fs',
  { timeout: 180_000 },
  () => {
    it('mounts v86fs and reads a file', async () => {
      const { adapter } = createMapAdapter()
      const emulator = await createEmulatorV86fs(adapter)
      try {
        await waitForSerial(emulator, ':/#', 120_000)

        // Mount v86fs at /mnt.
        await runCommand(emulator, 'mkdir -p /mnt')
        await runCommand(emulator, 'mount -t v86fs none /mnt')

        // Read the seeded test file.
        const content = await runCommand(emulator, 'cat /mnt/hello.txt')
        expect(content.trim()).toBe('v86fs-test-content')
      } finally {
        emulator.stop()
        emulator.destroy()
      }
    })

    it('writes and reads back via v86fs', async () => {
      const { adapter } = createMapAdapter()
      const emulator = await createEmulatorV86fs(adapter)
      try {
        await waitForSerial(emulator, ':/#', 120_000)

        await runCommand(emulator, 'mkdir -p /mnt')
        await runCommand(emulator, 'mount -t v86fs none /mnt')

        // Write a new file via v86fs.
        await runCommand(
          emulator,
          'echo "written-via-v86fs" > /mnt/output.txt',
        )

        // Read it back.
        const content = await runCommand(emulator, 'cat /mnt/output.txt')
        expect(content.trim()).toBe('written-via-v86fs')
      } finally {
        emulator.stop()
        emulator.destroy()
      }
    })
  },
)
