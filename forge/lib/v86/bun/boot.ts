/**
 * boot.ts - v86 boot script for forge bun subprocess controller.
 *
 * Boots a v86 VM headless in bun, runs commands via serial console,
 * and reports the exit code. Supports two filesystem modes:
 *
 * 1. v86fs over SRPC (primary): --socket <path> connects to a unix
 *    socket serving v86fs SRPC. The VM boots with 9p rootfs and mounts
 *    v86fs at configured paths for workspace/toolchain access.
 *
 * 2. 9p only (fallback): --v86fs-dir <path> loads rootfs from local
 *    fs.json + flat/ files. No v86fs device.
 *
 * Usage: bun run boot.ts [--socket <path>] [--v86fs-dir <path>]
 *        [--v86-dir <path>] [--memory <mb>] [--output-dir <dir>]
 *        [--bzimage <path>] [--cmd <command>]...
 */

import path from 'node:path'
import url from 'node:url'
import fs from 'node:fs'
import { StreamConn } from 'starpc'
import {
  connectToPipe,
} from '@go/github.com/aperturerobotics/util/pipesock/pipesock.js'
import { createV86fsSrpcAdapter } from './v86fs-bridge.js'

const __dirname = url.fileURLToPath(new URL('.', import.meta.url))

interface V86Emulator {
  add_listener(
    event: 'serial0-output-byte',
    handler: (byte: number) => void,
  ): void
  remove_listener(
    event: 'serial0-output-byte',
    handler: (byte: number) => void,
  ): void
  serial0_send(data: string): void
  stop(): void
  destroy(): void
}

interface V86Ctor {
  new (config: Record<string, unknown>): V86Emulator
}

interface Handle9pModule {
  createHandle9p(fsJsonUrl: string, flatUrl: string): unknown
}

interface V86fsBridge {
  adapter: unknown
  close: () => void
}

function getFetchUrl(input: string | URL | Request): string {
  if (typeof input === 'string') return input
  if (input instanceof URL) return input.href
  return input.url
}

// Patch fetch to support file:// URLs and local paths in bun/node
const _origFetch = globalThis.fetch
globalThis.fetch = async (
  input: string | URL | Request,
  init?: RequestInit,
) => {
  const u = getFetchUrl(input)
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

// Parse CLI arguments
function parseArgs(): {
  socket: string
  memory: number
  outputDir: string
  commands: string[]
  bzimage: string
  v86Dir: string
  v86fsDir: string
  mounts: Array<{ name: string; path: string }>
} {
  const args = process.argv.slice(2)
  let socket = ''
  let memory = 256
  let outputDir = '/output'
  let bzimage = ''
  let v86Dir = ''
  let v86fsDir = ''
  const commands: string[] = []
  const mounts: Array<{ name: string; path: string }> = []

  for (let i = 0; i < args.length; i++) {
    switch (args[i]) {
      case '--socket':
        socket = args[++i]
        break
      case '--memory':
        memory = parseInt(args[++i], 10)
        break
      case '--output-dir':
        outputDir = args[++i]
        break
      case '--bzimage':
        bzimage = args[++i]
        break
      case '--v86-dir':
        v86Dir = args[++i]
        break
      case '--v86fs-dir':
        v86fsDir = args[++i]
        break
      case '--mount': {
        // --mount name=/guest/path
        const val = args[++i]
        const eq = val.indexOf('=')
        if (eq > 0) {
          mounts.push({ name: val.slice(0, eq), path: val.slice(eq + 1) })
        }
        break
      }
      case '--cmd':
        commands.push(args[++i])
        break
    }
  }

  return { socket, memory, outputDir, commands, bzimage, v86Dir, v86fsDir, mounts }
}

// Strip ANSI escape codes from serial output
const ESC = String.fromCharCode(27)
const ANSI_RE = new RegExp(`${ESC}\\[[0-9;?]*[a-zA-Z]`, 'g')
function stripAnsi(s: string): string {
  return s.replace(ANSI_RE, '').replace(/\r/g, '')
}

// Wait for a marker string in serial output
function waitForSerial(
  emulator: V86Emulator,
  marker: string,
  timeoutMs = 120_000,
): Promise<string> {
  return new Promise((resolve, reject) => {
    let buf = ''
    const timer = setTimeout(() => {
      reject(
        new Error(
          `Timed out waiting for "${marker}" in serial. Got:\n${buf.slice(-500)}`,
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

// Send a command via serial and wait for shell prompt
async function runCommand(
  emulator: V86Emulator,
  cmd: string,
  prompt = '# ',
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

// Load handle9p from a v86fs directory (fs.json + flat/).
async function loadHandle9p(v86Dir: string, v86fsDir: string): Promise<unknown> {
  const serverPath = path.join(v86fsDir, 'handle9p-server.mjs')
  const fallback = path.join(v86Dir, 'tests/v86fs/handle9p-server.mjs')
  const modPath = fs.existsSync(serverPath) ? serverPath : fallback
  const mod = (await import(modPath)) as Handle9pModule
  const fsJsonUrl = url.pathToFileURL(path.join(v86fsDir, 'fs.json')).href
  const flatUrl = url.pathToFileURL(path.join(v86fsDir, 'flat')).href + '/'
  return mod.createHandle9p(fsJsonUrl, flatUrl)
}

// Connect to SRPC unix socket and create v86fs adapter.
async function connectV86fsSrpc(
  socketPath: string,
): Promise<V86fsBridge> {
  const streamConn = new StreamConn(undefined, { direction: 'outbound' })
  const client = streamConn.buildClient()

  // Connect socket and wait for connection.
  await new Promise<void>((resolve, reject) => {
    const socket = connectToPipe(socketPath, streamConn, () => resolve())
    socket.on('error', reject)
  })

  return createV86fsSrpcAdapter(client)
}

async function main() {
  const opts = parseArgs()

  // Resolve v86 directory (contains build/, bios/, src/)
  const v86Dir =
    opts.v86Dir || process.env.V86_DIR || path.resolve(__dirname, '../../../..')
  // Resolve v86fs directory (contains bzImage, fs.json, flat/)
  const v86fsDir = opts.v86fsDir || process.env.V86FS_DIR || ''
  const useV86fs = !!opts.socket

  // Resolve paths
  const wasmPath = path.join(v86Dir, 'build/v86-debug.wasm')
  const biosPath = path.join(v86Dir, 'bios/seabios.bin')
  const vgaBiosPath = path.join(v86Dir, 'bios/vgabios.bin')
  const bzimagePath =
    opts.bzimage ||
    (v86fsDir ? path.join(v86fsDir, 'bzImage') : path.join(v86Dir, 'bzImage'))

  // Verify required files exist
  for (const [name, p] of Object.entries({
    wasm: wasmPath,
    bios: biosPath,
    vga_bios: vgaBiosPath,
    bzimage: bzimagePath,
  })) {
    if (!fs.existsSync(p)) {
      console.error(`Required file not found: ${name} at ${p}`)
      process.exit(1)
    }
  }

  // Import V86 from source (same as v86 repo's own tests)
  const { V86 } = (await import(path.join(v86Dir, 'src/main.js'))) as {
    V86: V86Ctor
  }

  // Connect to v86fs SRPC server if socket provided.
  let v86fsBridge: V86fsBridge | undefined
  if (useV86fs) {
    console.error(`[forge-v86] connecting to v86fs SRPC: ${opts.socket}`)
    v86fsBridge = await connectV86fsSrpc(opts.socket)
    console.error('[forge-v86] v86fs SRPC connected')
  }

  // Determine boot mode:
  // - v86fs root: kernel mounts v86fs as root (requires v86fs-capable kernel)
  // - 9p root: load rootfs from local fs.json + flat/ (fallback)
  const useV86fsRoot = useV86fs && !v86fsDir
  let handle9p: unknown
  let cmdline: string

  if (useV86fsRoot) {
    // v86fs root: rootfs.tar served through the v86fs SRPC server.
    cmdline =
      'rw init=/usr/bin/bash root=v86fs rootfstype=v86fs rootflags= console=ttyS0'
    console.error('[forge-v86] booting with v86fs root')
  } else {
    // 9p root: load rootfs from local fs.json + flat/ files.
    handle9p = v86fsDir ? await loadHandle9p(v86Dir, v86fsDir) : undefined
    cmdline =
      'rw init=/usr/bin/bash root=host9p rootfstype=9p rootflags=trans=virtio,cache=loose console=ttyS0'
    console.error('[forge-v86] booting with 9p root')
  }

  console.error(
    `[forge-v86] booting VM: memory=${opts.memory}MB commands=${opts.commands.length} v86fs=${useV86fs}`,
  )

  // Boot the emulator headless.
  const emulator = new V86({
    wasm_path: wasmPath,
    memory_size: opts.memory * 1024 * 1024,
    vga_memory_size: 2 * 1024 * 1024,
    bios: { url: biosPath },
    vga_bios: { url: vgaBiosPath },
    bzimage: { url: bzimagePath },
    cmdline,
    filesystem: handle9p ? { handle9p } : {},
    virtio_v86fs: useV86fs,
    virtio_v86fs_adapter: v86fsBridge?.adapter,
    net_device: {
      type: 'virtio',
      relay_url: 'fetch',
    },
    autostart: true,
  })

  // Log serial output to stderr.
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

  // Wait for shell prompt
  const prompt = '# '
  await waitForSerial(emulator, prompt)
  console.error('[forge-v86] shell ready')

  // Mount named v86fs filesystems at their guest paths.
  // On 9p root, the rootfs is read-only for structural changes (mkdir fails).
  // Mount tmpfs at /tmp first, then use paths under writable areas.
  // With v86fs root (from tar), the rootfs is fully writable.
  if (useV86fs && opts.mounts.length > 0) {
    if (!useV86fsRoot) {
      await runCommand(emulator, 'mount -t tmpfs tmpfs /tmp 2>/dev/null; true')
    }
    for (const m of opts.mounts) {
      console.error(`[forge-v86] mounting v86fs: ${m.name} at ${m.path}`)
      await runCommand(emulator, `mkdir -p ${m.path}`)
      await runCommand(
        emulator,
        `mount -t v86fs none ${m.path} -o name=${m.name}`,
      )
    }
  }

  // Run commands sequentially
  let exitCode = 0
  for (const cmd of opts.commands) {
    console.error(`[forge-v86] running: ${cmd}`)
    const output = await runCommand(emulator, cmd, prompt)
    if (output) {
      process.stdout.write(output + '\n')
    }

    // Check exit code of last command
    const ecOutput = await runCommand(emulator, 'echo $?', prompt)
    const ec = parseInt(ecOutput.trim(), 10)
    if (isNaN(ec)) {
      console.error(`[forge-v86] could not parse exit code: ${ecOutput}`)
      exitCode = 1
      break
    }
    if (ec !== 0) {
      console.error(`[forge-v86] command failed with exit code ${ec}: ${cmd}`)
      exitCode = ec
      break
    }
  }

  // Stop emulator
  emulator.stop()
  emulator.destroy()

  // Close v86fs SRPC bridge.
  v86fsBridge?.close()

  console.error(`[forge-v86] done, exit code: ${exitCode}`)
  process.exit(exitCode)
}

main().catch((err) => {
  console.error('[forge-v86] fatal:', err)
  process.exit(1)
})
