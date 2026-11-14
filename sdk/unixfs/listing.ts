import type { DirEntry, HandleWatchReaddirResponse } from './handle.pb.js'

// applyReaddirChange returns the listing after one WatchReaddir response: the
// entries of listing without the names the response removes, with each entry
// it changes replaced and each entry it adds placed in name order. An entry
// the response leaves alone keeps its object.
export function applyReaddirChange(
  listing: DirEntry[],
  resp: HandleWatchReaddirResponse,
): DirEntry[] {
  // Index the response's changes by name.
  const removed = new Set(resp.removed ?? [])
  const upserts = new Map<string, DirEntry>()
  for (const entry of resp.entries ?? []) {
    upserts.set(entry.name ?? '', entry)
  }

  // Carry each kept entry across, replacing the changed ones.
  const kept: DirEntry[] = []
  for (const entry of listing) {
    const name = entry.name ?? ''
    if (removed.has(name)) continue
    const update = upserts.get(name)
    if (update) upserts.delete(name)
    kept.push(update ?? entry)
  }
  if (upserts.size === 0) return kept

  // Merge the added entries, which arrive in directory order.
  const added = [...upserts.values()]
  const next: DirEntry[] = []
  let i = 0
  let j = 0
  while (i < kept.length || j < added.length) {
    const takeKept =
      j >= added.length ||
      (i < kept.length &&
        compareEntryNames(kept[i].name ?? '', added[j].name ?? '') < 0)
    next.push(takeKept ? kept[i++] : added[j++])
  }
  return next
}

// compareEntryNames orders names as a directory does, by their UTF-8 bytes,
// which is the order of their code points.
function compareEntryNames(a: string, b: string): number {
  const n = Math.min(a.length, b.length)
  for (let i = 0; i < n; i++) {
    const ca = a.codePointAt(i) ?? 0
    const cb = b.codePointAt(i) ?? 0
    if (ca !== cb) return ca - cb
    if (ca > 0xffff) i++
  }
  return a.length - b.length
}
