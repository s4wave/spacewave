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
    description:
      'Start a Space, move files into it, keep your account safe, and work from a terminal when you want to.',
    order: 1,
  },
  {
    id: 'self-hosters',
    label: 'Self-Hosters',
    description:
      'Decide where the data lives, run the service, and know which recovery path applies when something breaks.',
    order: 2,
  },
  {
    id: 'developers',
    label: 'Developers',
    description:
      'Build ObjectTypes, plugins, Quickstarts, SDK resources, and public surfaces on top of Spaces.',
    order: 3,
  },
]

// SectionDef defines a documentation section without its pages.
export type SectionDef = Omit<DocSection, 'pages'>

// sectionDefs defines the documentation sections in display order.
export const sectionDefs: SectionDef[] = [
  // Users
  { id: 'start', label: 'Start', site: 'users', order: 1 },
  { id: 'files', label: 'Files and Drive', site: 'users', order: 2 },
  { id: 'spaces', label: 'Spaces', site: 'users', order: 3 },
  {
    id: 'accounts',
    label: 'Accounts and Storage',
    site: 'users',
    order: 4,
  },
  { id: 'devices', label: 'Devices', site: 'users', order: 5 },
  { id: 'cli', label: 'Command Line', site: 'users', order: 6 },
  { id: 'features', label: 'More Features', site: 'users', order: 7 },

  // Self-hosters
  { id: 'start', label: 'Start', site: 'self-hosters', order: 1 },
  {
    id: 'storage',
    label: 'Storage and Recovery',
    site: 'self-hosters',
    order: 2,
  },
  { id: 'operations', label: 'Operations', site: 'self-hosters', order: 3 },
  { id: 'ownership', label: 'Ownership', site: 'self-hosters', order: 4 },

  // Developers
  { id: 'start', label: 'Start', site: 'developers', order: 1 },
  { id: 'plugins', label: 'Plugins', site: 'developers', order: 2 },
  { id: 'objects', label: 'Objects', site: 'developers', order: 3 },
  { id: 'sdk', label: 'SDK and RPC', site: 'developers', order: 4 },
  { id: 'cli', label: 'CLI Reference', site: 'developers', order: 5 },
  { id: 'platform', label: 'Platform', site: 'developers', order: 6 },
]

// getSectionKey is the stable identity for a section inside an audience site.
export function getSectionKey(site: string, section: string): string {
  return `${site}/${section}`
}

// getSectionDef returns the definition for a site-owned section.
export function getSectionDef(
  site: string,
  section: string,
): SectionDef | undefined {
  return sectionDefs.find((def) => def.site === site && def.id === section)
}

// getSectionLabel returns the label for a site-owned section.
export function getSectionLabel(site: string, section: string): string {
  return getSectionDef(site, section)?.label ?? section
}

// cachedSections holds the parsed sections after first load.
let cachedSections: DocSection[] | null = null

// getSections returns all sections populated with their pages.
export function getSections(): DocSection[] {
  if (cachedSections) return cachedSections

  const docs = loadDocs()
  const sections: DocSection[] = sectionDefs.flatMap((def) => {
    const pages = docs
      .filter((d) => d.site === def.site && d.section === def.id)
      .sort((a, b) => a.order - b.order)
    return pages.length > 0 ? [{ ...def, pages }] : []
  })

  cachedSections = sections
  return sections
}
