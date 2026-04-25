import { useMemo } from 'react'

import {
  useResource,
  type Resource,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import type { FSHandle } from '@s4wave/sdk/unixfs/handle.js'
import type { FileEntry } from '@s4wave/web/editors/file-browser/types.js'

import { parseNote } from '../frontmatter.js'
import type { BlogPostData } from './types.js'

// useBlogPosts reads all .md file entries from a UnixFS root handle,
// parses frontmatter, and returns BlogPostData for files with a date field.
export function useBlogPosts(
  rootHandle: Resource<FSHandle>,
  entries: Resource<FileEntry[] | null>,
): Resource<BlogPostData[]> {
  const mdEntries = useMemo(() => {
    if (!entries.value) return []
    return entries.value.filter(
      (entry) => !entry.isDir && entry.name.endsWith('.md'),
    )
  }, [entries.value])

  return useResource(
    rootHandle,
    async (root, signal) => {
      if (!root || mdEntries.length === 0) return []

      const results: BlogPostData[] = []
      for (const entry of mdEntries) {
        if (signal.aborted) return results
        const child = await root.lookup(entry.name, signal)
        const result = await child.readAt(0n, 0n, signal)
        child.release()
        const text = new TextDecoder().decode(result.data)
        const parsed = parseNote(text)
        const fm = parsed.frontmatter as Record<string, unknown>

        // Only include files that have a date in frontmatter.
        const rawDate = fm.date
        const date =
          rawDate instanceof Date
            ? rawDate.toISOString().slice(0, 10)
            : typeof rawDate === 'string'
              ? rawDate
              : undefined
        if (!date) continue

        const title = (fm.title as string) || entry.name.replace(/\.md$/, '')
        const summary = (fm.summary as string) || ''
        const tags = fm.tags
        const author = fm.author
        const draft = !!fm.draft
        let authorName: string | undefined
        if (Array.isArray(author)) {
          const [firstAuthor] = author as unknown[]
          authorName =
            typeof firstAuthor === 'string' ? firstAuthor : undefined
        } else if (typeof author === 'string') {
          authorName = author
        }

        results.push({
          name: entry.name,
          title,
          date: String(date),
          summary,
          tags: Array.isArray(tags) ? tags.map(String) : [],
          body: parsed.body,
          author: authorName,
          draft,
        })
      }

      return results
    },
    [mdEntries],
  )
}
