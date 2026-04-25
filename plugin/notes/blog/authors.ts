import matter from 'gray-matter'

import {
  useResource,
  type Resource,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import type { FSHandle } from '@s4wave/sdk/unixfs/handle.js'

// AuthorInfo represents a single author's display data.
export interface AuthorInfo {
  // name is the display name of the author.
  name: string
  // avatar is a URL to the author's avatar image.
  avatar?: string
  // url is a link to the author's profile.
  url?: string
  // bio is a short biography.
  bio?: string
}

// AuthorRegistry maps author slugs to their display data.
export type AuthorRegistry = Record<string, AuthorInfo>

// parseAuthorRegistry parses YAML content into an AuthorRegistry.
// Uses gray-matter to parse YAML by wrapping content in frontmatter delimiters.
export function parseAuthorRegistry(yamlContent: string): AuthorRegistry {
  const wrapped = '---\n' + yamlContent + '\n---\n'
  const parsed = matter(wrapped).data
  if (!parsed || typeof parsed !== 'object') return {}

  const registry: AuthorRegistry = {}
  for (const [slug, value] of Object.entries(parsed as Record<string, unknown>)) {
    if (!value || typeof value !== 'object') continue
    const entry = value as Record<string, unknown>
    registry[slug] = {
      name: (entry.name as string) || slug,
      avatar: entry.avatar as string | undefined,
      url: entry.url as string | undefined,
      bio: entry.bio as string | undefined,
    }
  }
  return registry
}

// useAuthorRegistry reads and parses an authors.yaml file from UnixFS.
// Falls back to an empty registry if the file is missing or unparseable.
export function useAuthorRegistry(
  rootHandle: Resource<FSHandle>,
  registryPath: string,
): Resource<AuthorRegistry> {
  const path = registryPath || 'authors.yaml'

  return useResource(
    rootHandle,
    async (root, signal) => {
      if (!root) return {}
      try {
        const child = await root.lookup(path, signal)
        const result = await child.readAt(0n, 0n, signal)
        child.release()
        const text = new TextDecoder().decode(result.data)
        return parseAuthorRegistry(text)
      } catch {
        // File not found or unreadable, return empty registry.
        return {}
      }
    },
    [path],
  )
}

// resolveAuthor looks up an author slug in the registry.
// Returns the AuthorInfo if found, or a fallback with the raw slug as name.
export function resolveAuthor(
  registry: AuthorRegistry,
  slug: string | undefined,
): AuthorInfo | null {
  if (!slug) return null
  return registry[slug] ?? { name: slug }
}
