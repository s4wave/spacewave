import {
  createContext,
  use,
  useCallback,
  useEffect,
  useId,
  useMemo,
  useState,
  type ReactNode,
} from 'react'

const brandTitle = 'Spacewave'

const routeTitles: Record<string, string> = {
  auth: 'Account',
  checkout: 'Checkout',
  community: 'Community',
  debug: 'Debug',
  display: 'Display',
  dmca: 'DMCA',
  download: 'Download',
  join: 'Join Space',
  licenses: 'Licenses',
  login: 'Login',
  pair: 'Pair Device',
  pricing: 'Pricing',
  privacy: 'Privacy',
  quickstart: 'Quickstart',
  recover: 'Recover Account',
  sessions: 'Sessions',
  signup: 'Sign Up',
  tos: 'Terms of Service',
}

export interface DocumentTitleParts {
  view?: string
  space?: string
}

export interface UseDocumentTitleOptions {
  active?: boolean
  priority?: number
}

interface DocumentTitleCandidate {
  priority: number
  title: string
}

interface DocumentTitleContextValue {
  update: (id: string, candidate: DocumentTitleCandidate | null) => void
}

const DocumentTitleContext = createContext<DocumentTitleContextValue | null>(
  null,
)

function humanizeRouteSegment(segment: string): string {
  let decoded = segment
  try {
    decoded = decodeURIComponent(segment)
  } catch {
    // Keep malformed external URL text usable as a title fallback.
  }
  return decoded
    .replace(/[-_]+/g, ' ')
    .replace(/\b\w/g, (character) => character.toUpperCase())
}

// buildDocumentTitle formats the most-specific available context first.
export function buildDocumentTitle(parts: DocumentTitleParts): string {
  const seen = new Set<string>()
  const titleParts = [parts.view, parts.space, brandTitle].flatMap((part) => {
    const value = part?.trim() ?? ''
    const key = value.toLocaleLowerCase()
    if (!value || seen.has(key)) return []
    seen.add(key)
    return [value]
  })
  return titleParts.join(' - ')
}

// getSpaceDocumentTitleName returns a stable label for unnamed Spaces.
export function getSpaceDocumentTitleName(
  name: string,
  spaceId: string,
): string {
  const value = name.trim()
  return value && value !== spaceId ? value : 'Space'
}

// getRouteDocumentTitleParts derives the live fallback before resources load.
export function getRouteDocumentTitleParts(
  path: string,
  fallbackName: string,
): DocumentTitleParts {
  const segments = path.split('/').filter(Boolean)
  const fallback = fallbackName.trim()
  if (segments.length === 0) {
    return fallback && fallback !== 'Home' && fallback !== 'Tab'
      ? { view: fallback }
      : {}
  }
  if (path === '/landing') return {}

  if (fallback && fallback !== 'Home' && fallback !== 'Tab') {
    return { view: fallback }
  }

  const [section, detail] = segments
  if (section === 'landing') {
    return detail ? { view: humanizeRouteSegment(detail) } : {}
  }
  const sectionTitle = routeTitles[section]
  if (sectionTitle) return { view: sectionTitle }

  const lastSegment = segments.at(-1)
  return lastSegment ? { view: humanizeRouteSegment(lastSegment) } : {}
}

// DocumentTitleProvider is the only owner that writes document.title.
export function DocumentTitleProvider({ children }: { children: ReactNode }) {
  const [candidates, setCandidates] = useState(
    () => new Map<string, DocumentTitleCandidate>(),
  )
  const update = useCallback(
    (id: string, candidate: DocumentTitleCandidate | null) => {
      setCandidates((current) => {
        const existing = current.get(id)
        if (
          existing?.priority === candidate?.priority &&
          existing?.title === candidate?.title
        ) {
          return current
        }
        if (!existing && !candidate) return current

        const next = new Map(current)
        if (candidate) next.set(id, candidate)
        else next.delete(id)
        return next
      })
    },
    [],
  )
  const value = useMemo(() => ({ update }), [update])
  const title = useMemo(() => {
    let selected: DocumentTitleCandidate | undefined
    for (const candidate of candidates.values()) {
      if (!selected || candidate.priority > selected.priority) {
        selected = candidate
      }
    }
    return selected?.title ?? brandTitle
  }, [candidates])

  useEffect(() => {
    document.title = title
  }, [title])

  return (
    <DocumentTitleContext.Provider value={value}>
      {children}
    </DocumentTitleContext.Provider>
  )
}

// useDocumentTitle publishes context to the document-local title owner.
export function useDocumentTitle(
  parts: DocumentTitleParts,
  options: UseDocumentTitleOptions = {},
): void {
  const context = use(DocumentTitleContext)
  const id = useId()
  const active = options.active ?? true
  const priority = options.priority ?? 0
  const title = buildDocumentTitle(parts)

  useEffect(() => {
    if (!context || !active) return
    context.update(id, { priority, title })
    return () => context.update(id, null)
  }, [active, context, id, priority, title])
}
