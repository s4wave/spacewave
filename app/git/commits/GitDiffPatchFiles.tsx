import { useCallback, useMemo, useState } from 'react'
import { LuChevronDown, LuChevronRight } from 'react-icons/lu'

import type { DiffFileStat } from '@s4wave/sdk/git/repo.pb.js'

import { GitDiffPatch } from './GitDiffPatch.js'

export interface GitDiffPatchFilesProps {
  files: DiffFileStat[] | undefined
  patch: string | undefined
  loading: boolean
  truncated?: boolean
  totalBytes?: bigint | number
  limitBytes?: number
  error?: Error | null
}

export function GitDiffPatchFiles({
  files,
  patch,
  loading,
  truncated,
  totalBytes,
  limitBytes,
  error,
}: GitDiffPatchFilesProps) {
  const sections = useMemo(
    () => splitPatchFiles(patch, files, truncated ?? false),
    [patch, files, truncated],
  )
  const [collapsed, setCollapsed] = useState<Set<string>>(() => new Set())

  const toggle = useCallback((path: string) => {
    setCollapsed((prev) => {
      const next = new Set(prev)
      if (next.has(path)) {
        next.delete(path)
        return next
      }
      next.add(path)
      return next
    })
  }, [])

  if (loading) {
    return (
      <div className="text-foreground-alt px-3 py-2 text-xs">Loading diff…</div>
    )
  }

  if (error) {
    return (
      <div className="text-destructive px-3 py-2 text-xs">
        Failed to load diff: {error.message}
      </div>
    )
  }

  if (sections.length === 0) {
    return (
      <div className="text-foreground-alt/70 px-3 py-2 text-xs">
        No diff to display
      </div>
    )
  }

  return (
    <div className="space-y-2">
      {truncated && (
        <div className="border-warning/30 bg-warning/10 text-warning rounded-lg border px-3 py-2 text-xs">
          Showing the first {formatBytes(limitBytes ?? 0)} of{' '}
          {formatBytes(totalBytes ?? 0)}.
        </div>
      )}
      {sections.map((section) => {
        const isCollapsed = collapsed.has(section.path)
        return (
          <div
            key={section.path}
            className="border-foreground/6 bg-background-card/30 overflow-hidden rounded-lg border"
          >
            <button
              className="hover:bg-background-card/50 flex h-10 w-full items-center gap-2 px-3 text-left transition-colors"
              onClick={() => toggle(section.path)}
            >
              {isCollapsed ? (
                <LuChevronRight className="text-foreground-alt/50 size-3.5 shrink-0" />
              ) : (
                <LuChevronDown className="text-foreground-alt/50 size-3.5 shrink-0" />
              )}
              <span className="text-foreground min-w-0 flex-1 truncate font-mono text-xs">
                {section.path}
              </span>
              <span className="flex shrink-0 items-center gap-2 font-mono text-xs">
                {section.deletions > 0 && (
                  <span className="text-error">-{section.deletions}</span>
                )}
                {section.additions > 0 && (
                  <span className="text-success">+{section.additions}</span>
                )}
              </span>
            </button>
            {!isCollapsed && (
              <GitDiffPatch
                patch={section.patch}
                className="border-foreground/6 border-t"
              />
            )}
          </div>
        )
      })}
    </div>
  )
}

interface PatchSection {
  path: string
  additions: number
  deletions: number
  patch?: string
}

function splitPatchFiles(
  patch: string | undefined,
  files: DiffFileStat[] | undefined,
  truncated: boolean,
): PatchSection[] {
  if (!patch) return []

  const chunks = splitPatchChunks(patch)
  if (files?.length) {
    const statPaths = new Set(
      files.flatMap((file) => (file.path ? [file.path] : [])),
    )
    const claims = new Map<string, string[]>()
    for (const chunk of chunks) {
      const parsed = parseNativePatchChunk(chunk)
      if (!parsed) continue
      const matchingPairs = parsed.pairs.filter((pair) =>
        statPaths.has(statPathForPair(parsed, pair)),
      )
      if (matchingPairs.length !== 1) continue
      const statPath = statPathForPair(parsed, matchingPairs[0]!)
      const claimedChunks = claims.get(statPath) ?? []
      claimedChunks.push(chunk)
      claims.set(statPath, claimedChunks)
    }

    const sections = files.map((file) => {
      const path = file.path ?? ''
      const claimedChunks = claims.get(path)
      return {
        path,
        additions: file.additions ?? 0,
        deletions: file.deletions ?? 0,
        patch: claimedChunks?.length === 1 ? claimedChunks[0] : undefined,
      }
    })
    return truncated ? sections.filter((section) => section.patch) : sections
  }

  return chunks.flatMap((chunk) => {
    const parsed = parseNativePatchChunk(chunk)
    if (!parsed) {
      return chunk.startsWith('diff --git ')
        ? []
        : [
            {
              path: 'diff',
              additions: 0,
              deletions: 0,
              patch: chunk,
            },
          ]
    }
    if (parsed.pairs.length !== 1) return []
    return [
      {
        path: statPathForPair(parsed, parsed.pairs[0]!),
        additions: 0,
        deletions: 0,
        patch: chunk,
      },
    ]
  })
}

function splitPatchChunks(patch: string): string[] {
  if (patch.includes('\ndiff --git ') || patch.startsWith('diff --git ')) {
    return splitPatchChunksByHeader(patch, (line) =>
      line.startsWith('diff --git '),
    )
  }
  return splitPatchChunksByHeader(patch, (line) => line.startsWith('--- '))
}

function splitPatchChunksByHeader(
  patch: string,
  isHeader: (line: string) => boolean,
): string[] {
  const chunks: string[] = []
  const lines = patch.split('\n')
  const current: string[] = []
  for (const line of lines) {
    if (isHeader(line) && current.length > 0) {
      chunks.push(current.join('\n'))
      current.length = 0
    }
    current.push(line)
  }
  if (current.length > 0) chunks.push(current.join('\n'))
  return chunks.filter((chunk) => chunk.trim().length > 0)
}

interface NativeDiffPathPair {
  sourcePath: string
  destinationPath: string
}

interface NativePatchChunk {
  pairs: NativeDiffPathPair[]
  sourceEvidence?: string | null
  destinationEvidence?: string | null
}

function parseNativePatchChunk(chunk: string): NativePatchChunk | undefined {
  const lines = chunk.split('\n')
  const pairs = parseNativeDiffHeader(lines[0] ?? '')
  if (pairs.length === 0) return undefined

  let sourceEvidence: string | null | undefined
  let destinationEvidence: string | null | undefined
  let extendedKind: 'rename' | 'copy' | undefined
  let invalid = false
  const recordSource = (path: string | null | undefined) => {
    if (path === undefined || sourceEvidence === path) return
    if (sourceEvidence !== undefined) {
      invalid = true
      return
    }
    sourceEvidence = path
  }
  const recordDestination = (path: string | null | undefined) => {
    if (path === undefined || destinationEvidence === path) return
    if (destinationEvidence !== undefined) {
      invalid = true
      return
    }
    destinationEvidence = path
  }

  let extendedSource: string | undefined
  let extendedDestination: string | undefined
  for (const line of lines.slice(1)) {
    if (line.startsWith('rename from ')) {
      if (extendedKind && extendedKind !== 'rename') invalid = true
      extendedKind = 'rename'
      extendedSource = parseGitPathToEnd(line, 'rename from '.length)
      if (!extendedSource) invalid = true
    } else if (line.startsWith('rename to ')) {
      if (extendedKind && extendedKind !== 'rename') invalid = true
      extendedKind = 'rename'
      extendedDestination = parseGitPathToEnd(line, 'rename to '.length)
      if (!extendedDestination) invalid = true
    } else if (line.startsWith('copy from ')) {
      if (extendedKind && extendedKind !== 'copy') invalid = true
      extendedKind = 'copy'
      extendedSource = parseGitPathToEnd(line, 'copy from '.length)
      if (!extendedSource) invalid = true
    } else if (line.startsWith('copy to ')) {
      if (extendedKind && extendedKind !== 'copy') invalid = true
      extendedKind = 'copy'
      extendedDestination = parseGitPathToEnd(line, 'copy to '.length)
      if (!extendedDestination) invalid = true
    } else if (line.startsWith('--- ')) {
      const path = parsePatchMarkerPath(line.slice(4), 'i/')
      if (path === undefined) invalid = true
      else recordSource(path)
    } else if (line.startsWith('+++ ')) {
      const path = parsePatchMarkerPath(line.slice(4), 'w/')
      if (path === undefined) invalid = true
      else recordDestination(path)
    }
  }

  if (extendedKind) {
    if (!extendedSource || !extendedDestination) return undefined
    recordSource(extendedSource)
    recordDestination(extendedDestination)
  }
  if (invalid) return undefined

  const narrowedPairs = pairs.filter((pair) => {
    if (sourceEvidence !== undefined && sourceEvidence !== null) {
      if (pair.sourcePath !== sourceEvidence) return false
    }
    if (destinationEvidence !== undefined && destinationEvidence !== null) {
      if (pair.destinationPath !== destinationEvidence) return false
    }
    if (!extendedKind && pair.sourcePath !== pair.destinationPath) return false
    return true
  })
  const uniquePairs = new Map(
    narrowedPairs.map((pair) => [
      `${pair.sourcePath}\0${pair.destinationPath}`,
      pair,
    ]),
  )
  return {
    pairs: [...uniquePairs.values()],
    sourceEvidence,
    destinationEvidence,
  }
}

function statPathForPair(
  patch: NativePatchChunk,
  pair: NativeDiffPathPair,
): string {
  return patch.destinationEvidence === null
    ? pair.sourcePath
    : pair.destinationPath
}

function parsePatchMarkerPath(
  token: string,
  prefix: 'i/' | 'w/',
): string | null | undefined {
  const pathToken = token.startsWith('"') ? token : token.split('\t', 1)[0]!
  if (pathToken === '/dev/null') return null
  const path = parseGitPathToEnd(pathToken, 0)
  if (!path?.startsWith(prefix) || path.length === prefix.length) {
    return undefined
  }
  return path.slice(prefix.length)
}

function parseNativeDiffHeader(line: string): NativeDiffPathPair[] {
  const marker = 'diff --git '
  if (!line.startsWith(marker)) return []
  const input = line.slice(marker.length)
  const candidates: NativeDiffPathPair[] = []

  if (input.startsWith('"')) {
    const first = parseQuotedGitPath(input, 0)
    if (!first || input[first.offset] !== ' ') return []
    const second = parseGitPathToEnd(input, first.offset + 1)
    const candidate = makeNativeDiffPathPair(first.path, second)
    return candidate ? [candidate] : []
  }

  for (let offset = 1; offset < input.length; offset++) {
    if (input[offset] !== ' ') continue
    const second = parseGitPathToEnd(input, offset + 1)
    const candidate = makeNativeDiffPathPair(input.slice(0, offset), second)
    if (candidate) candidates.push(candidate)
  }
  return candidates
}

function parseGitPathToEnd(input: string, start: number): string | undefined {
  if (input[start] === '"') {
    const parsed = parseQuotedGitPath(input, start)
    return parsed?.offset === input.length ? parsed.path : undefined
  }
  const path = input.slice(start)
  if (!path || hasUnsafeUnquotedPathChar(path)) return undefined
  return path
}

function hasUnsafeUnquotedPathChar(path: string): boolean {
  for (const char of path) {
    const code = char.charCodeAt(0)
    if (char === '"' || char === '\\' || code <= 0x1f || code === 0x7f) {
      return true
    }
  }
  return false
}

function makeNativeDiffPathPair(
  sourceToken: string,
  destinationToken: string | undefined,
): NativeDiffPathPair | undefined {
  if (!destinationToken) return undefined
  if (
    !sourceToken.startsWith('i/') ||
    !destinationToken.startsWith('w/') ||
    sourceToken.length === 2 ||
    destinationToken.length === 2 ||
    sourceToken.includes('\0') ||
    destinationToken.includes('\0')
  ) {
    return undefined
  }
  return {
    sourcePath: sourceToken.slice(2),
    destinationPath: destinationToken.slice(2),
  }
}

function parseQuotedGitPath(
  input: string,
  start: number,
): { path: string; offset: number } | undefined {
  if (input[start] !== '"') return undefined
  const bytes: number[] = []
  let offset = start + 1

  while (offset < input.length) {
    const char = input[offset++]
    if (char === '"') {
      try {
        return {
          path: new TextDecoder('utf-8', { fatal: true }).decode(
            Uint8Array.from(bytes),
          ),
          offset,
        }
      } catch {
        return undefined
      }
    }

    if (char !== '\\') {
      const encoded = new TextEncoder().encode(char)
      bytes.push(...encoded)
      continue
    }

    const escape = input[offset++]
    if (escape === undefined) return undefined
    const standardEscapes: Record<string, number> = {
      a: 0x07,
      b: 0x08,
      f: 0x0c,
      n: 0x0a,
      r: 0x0d,
      t: 0x09,
      v: 0x0b,
      '\\': 0x5c,
      '"': 0x22,
    }
    const standard = standardEscapes[escape]
    if (standard !== undefined) {
      bytes.push(standard)
      continue
    }

    if (!/[0-7]/.test(escape)) return undefined
    const remaining = input.slice(offset, offset + 2)
    if (!/^[0-7]{2}$/.test(remaining)) return undefined
    const byte = Number.parseInt(`${escape}${remaining}`, 8)
    if (byte > 0xff) return undefined
    bytes.push(byte)
    offset += 2
  }

  return undefined
}

function formatBytes(bytes: bigint | number): string {
  const value = Number(bytes)
  if (!Number.isFinite(value) || value <= 0) return '0 B'
  const units = ['B', 'KiB', 'MiB', 'GiB']
  let scaled = value
  let unit = 0
  while (scaled >= 1024 && unit < units.length - 1) {
    scaled /= 1024
    unit++
  }
  return `${scaled.toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`
}
