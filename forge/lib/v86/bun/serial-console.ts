// Shared v86 runtime helpers: local fetch patching, serial console driving,
// and handle9p loading. Used by the bun boot script and its tests.
import path from 'node:path'
import url from 'node:url'
import fs from 'node:fs'

export interface V86Emulator {
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

export interface V86Ctor {
  new (config: Record<string, unknown>): V86Emulator
}

export interface Handle9pModule {
  createHandle9p(fsJsonUrl: string, flatUrl: string): unknown
}

export function getFetchUrl(input: string | URL | Request): string {
  if (typeof input === 'string') return input
  if (input instanceof URL) return input.href
  return input.url
}

let fetchPatched = false

// Patch fetch to support file:// URLs and local paths in bun/node.
export function installLocalFetchPatch(): void {
  if (fetchPatched) return
  fetchPatched = true
  const origFetch = globalThis.fetch
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
    return origFetch(input, init)
  }
}

// Strip ANSI escape codes and carriage returns from serial output.
const ESC = String.fromCharCode(27)
const ANSI_RE = new RegExp(`${ESC}\\[[0-9;?]*[a-zA-Z]`, 'g')
export function stripAnsi(s: string): string {
  return s.replace(ANSI_RE, '').replace(/\r/g, '')
}

// Wait for a marker string in serial output.
export function waitForSerial(
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

// Send a command via serial and wait for the shell prompt.
export async function runCommand(
  emulator: V86Emulator,
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

// Load handle9p from a v86fs directory (fs.json + flat/).
export async function loadHandle9p(
  v86Dir: string,
  v86fsDir: string,
): Promise<unknown> {
  const serverPath = path.join(v86fsDir, 'handle9p-server.mjs')
  const fallback = path.join(v86Dir, 'tests/v86fs/handle9p-server.mjs')
  const modPath = fs.existsSync(serverPath) ? serverPath : fallback

  const mod = (await import(modPath)) as Handle9pModule
  const fsJsonUrl = url.pathToFileURL(path.join(v86fsDir, 'fs.json')).href
  const flatUrl = url.pathToFileURL(path.join(v86fsDir, 'flat')).href + '/'
  return mod.createHandle9p(fsJsonUrl, flatUrl)
}
