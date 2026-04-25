import { loadDocs } from './load-docs.js'
import type { DocSection } from './types.js'

// DocSite defines a top-level documentation audience surface.
export interface DocSite {
  id: string
  label: string
  description: string
  order: number
}

// siteDefs defines the documentation audience surfaces in display order.
export const siteDefs: DocSite[] = [
  {
    id: 'users',
    label: 'Users',
    description: 'Understand and use.',
    order: 1,
  },
  {
    id: 'self-hosters',
    label: 'Self-Hosters',
    description: 'Deploy, operate, back up, and recover.',
    order: 2,
  },
  {
    id: 'developers',
    label: 'Developers',
    description: 'Use CLI and platform references.',
    order: 3,
  },
]

// SectionDef defines a documentation section without its pages.
export type SectionDef = Omit<DocSection, 'pages'>

// sectionDefs defines the documentation sections in display order.
export const sectionDefs: SectionDef[] = [
  // Users
  {
    id: 'overview',
    label: 'Overview',
    site: 'users',
    order: 1,
  },
  {
    id: 'getting-started',
    label: 'Getting Started',
    site: 'users',
    order: 2,
  },
  { id: 'spaces', label: 'Spaces', site: 'users', order: 3 },
  {
    id: 'files',
    label: 'Files & Data',
    site: 'users',
    order: 4,
  },
  {
    id: 'accounts',
    label: 'Accounts',
    site: 'users',
    order: 5,
  },
  {
    id: 'sessions',
    label: 'Sessions',
    site: 'users',
    order: 6,
  },
  {
    id: 'features',
    label: 'Features',
    site: 'users',
    order: 7,
  },
  {
    id: 'organizations',
    label: 'Organizations',
    site: 'users',
    order: 8,
  },
  {
    id: 'devices',
    label: 'Devices & Migration',
    site: 'users',
    order: 9,
  },
  {
    id: 'settings',
    label: 'Settings',
    site: 'users',
    order: 10,
  },
  {
    id: 'cli',
    label: 'Command Line',
    site: 'users',
    order: 11,
  },
  // Self-hosters
  {
    id: 'start-here',
    label: 'Start Here',
    site: 'self-hosters',
    order: 8,
  },
  {
    id: 'deployment-modes',
    label: 'Deployment Modes',
    site: 'self-hosters',
    order: 9,
  },
  {
    id: 'operations',
    label: 'Operations',
    site: 'self-hosters',
    order: 10,
  },
  {
    id: 'backups-and-recovery',
    label: 'Backups & Recovery',
    site: 'self-hosters',
    order: 11,
  },
  // Developers
  {
    id: 'dev-start-here',
    label: 'Start Here',
    site: 'developers',
    order: 20,
  },
  {
    id: 'plugins',
    label: 'Plugin Development',
    site: 'developers',
    order: 21,
  },
  {
    id: 'sdk',
    label: 'SDK Reference',
    site: 'developers',
    order: 22,
  },
  {
    id: 'cli',
    label: 'CLI & Tools',
    site: 'developers',
    order: 23,
  },
  {
    id: 'platform',
    label: 'Platform',
    site: 'developers',
    order: 24,
  },
  {
    id: 'internals',
    label: 'Internals',
    site: 'developers',
    order: 25,
  },
]

// cachedSections holds the parsed sections after first load.
let cachedSections: DocSection[] | null = null

// getSections returns all sections populated with their pages.
export function getSections(): DocSection[] {
  if (cachedSections) return cachedSections

  const docs = loadDocs()
  const sections: DocSection[] = sectionDefs
    .map((def) => ({
      ...def,
      pages: docs
        .filter((d) => d.section === def.id)
        .sort((a, b) => a.order - b.order),
    }))
    .filter((section) => section.pages.length > 0)

  cachedSections = sections
  return sections
}
