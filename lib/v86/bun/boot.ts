/**
 * boot.ts - v86 boot script for forge bun subprocess controller.
 *
 * Boots a v86 VM headless in bun, runs commands via serial console,
 * and reports the exit code.
 *
 * Usage: bun run boot.ts --socket <path> --memory <mb> --output-dir <dir>
 *
 * The script connects to a unix socket serving v86fs SRPC for filesystem
 * access. For the initial prototype, rootfs is loaded via 9p handle.
 * v86fs SRPC integration is a follow-on iteration.
 */

import path from 'node:path'
import url from 'node:url'
import fs from 'node:fs'

const __dirname = url.fileURLToPath(new URL('.', import.meta.url))

// Patch fetch to support file:// URLs and local paths in bun/node
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

// Parse CLI arguments
function parseArgs(): {
  socket: string
  memory: number
  outputDir: string
  commands: string[]
  bzimage: string
  v86Dir: string
} {
  const args = process.argv.slice(2)
  let socket = ''
  let memory = 256
  let outputDir = '/output'
  let bzimage = ''
  let v86Dir = ''
  const commands: string[] = []

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
      case '--cmd':
        commands.push(args[++i])
        break
    }
  }

  return { socket, memory, outputDir, commands, bzimage, v86Dir }
}

// Strip ANSI escape codes from serial output
const ANSI_RE = /\x1b\[[0-9;?]*[a-zA-Z]/g
function stripAnsi(s: string): string {
  return s.replace(ANSI_RE, '').replace(/\r/g, '')
}

// Wait for a marker string in serial output
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

async function main() {
  const opts = parseArgs()

  // Resolve v86 directory (contains build/, bios/)
  const v86Dir =
    opts.v86Dir || process.env.V86_DIR || path.resolve(__dirname, '../../../..')

  // Resolve paths
  const wasmPath = path.join(v86Dir, 'build/v86.wasm')
  const biosPath = path.join(v86Dir, 'bios/seabios.bin')
  const vgaBiosPath = path.join(v86Dir, 'bios/vgabios.bin')
  const bzimagePath = opts.bzimage || path.join(v86Dir, 'bzImage')

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

  // Import v86
  const { V86 } = await import('@aptre/v86')

  console.error(
    `[forge-v86] booting VM: memory=${opts.memory}MB commands=${opts.commands.length}`,
  )

  // Boot the emulator headless
  const emulator = new V86({
    wasm_path: wasmPath,
    memory_size: opts.memory * 1024 * 1024,
    vga_memory_size: 2 * 1024 * 1024,
    bios: { url: biosPath },
    vga_bios: { url: vgaBiosPath },
    bzimage: { url: bzimagePath },
    cmdline:
      'rw init=/usr/bin/bash root=host9p rootfstype=9p rootflags=trans=virtio,cache=loose console=ttyS0',
    filesystem: {},
    autostart: true,
  })

  // Wait for shell prompt
  const prompt = ':/#'
  await waitForSerial(emulator, prompt)
  console.error('[forge-v86] shell ready')

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

  console.error(`[forge-v86] done, exit code: ${exitCode}`)
  process.exit(exitCode)
}

main().catch((err) => {
  console.error('[forge-v86] fatal:', err)
  process.exit(1)
})
