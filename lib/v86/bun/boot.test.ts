import { describe, it, expect } from 'vitest'
import path from 'node:path'
import url from 'node:url'
import fs from 'node:fs'

const __dirname = url.fileURLToPath(new URL('.', import.meta.url))

// V86_DIR: directory containing build/v86.wasm, bios/, src/main.js.
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

async function createEmulator(): Promise<any> {
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

  // Log serial output to stderr so we can see the guest boot.
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

  return emulator
}

describe.runIf(HAS_ASSETS)(
  'forge v86 bun',
  { timeout: 180_000 },
  () => {
    it('boots VM and runs echo', async () => {
      const emulator = await createEmulator()
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
      const emulator = await createEmulator()
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
      const emulator = await createEmulator()
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
