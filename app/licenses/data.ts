// Loads and merges license data with human annotations.
// Imported at build time by both /licenses and /community pages.

import licensesJson from './licenses.json'
import {
  annotations,
  categories,
  type DependencyAnnotation,
  type CategoryDef,
} from './annotations.js'

export type { DependencyAnnotation, CategoryDef }

export interface LicenseEntry {
  name: string
  version: string
  spdx: string
  source: 'go' | 'js' | 'both'
  repo?: string
  isDev?: boolean
  copyrightNotice?: string
  fullText?: string
}

export interface LicensesData {
  bases: Record<string, string>
  entries: LicenseEntry[]
}

export interface AnnotatedLicenseEntry extends LicenseEntry {
  category: string
  purpose?: string
  internal: boolean
  isDev: boolean
}

const data = licensesJson as LicensesData

function derivePackageUrl(entry: LicenseEntry): string | undefined {
  if (entry.source === 'js') {
    return `https://www.npmjs.com/package/${entry.name}`
  }
  if (entry.source === 'go') {
    return `https://pkg.go.dev/${entry.name}`
  }
}

// Reconstruct full license text from base + copyright notice.
export function reconstructText(entry: LicenseEntry): string {
  if (entry.fullText) return entry.fullText
  const base = data.bases[entry.spdx]
  if (!base) return ''
  const parts: string[] = []
  if (entry.copyrightNotice) parts.push(entry.copyrightNotice)
  parts.push('')
  parts.push(base)
  return parts.join('\n')
}

// Merge license entries with annotations.
function buildAnnotatedEntries(): AnnotatedLicenseEntry[] {
  return data.entries.map((entry) => {
    const annotation = annotations[entry.name]
    const repo = annotation?.repo ?? entry.repo ?? derivePackageUrl(entry)
    return {
      ...entry,
      category: annotation?.category ?? 'uncategorized',
      purpose: annotation?.purpose,
      internal: annotation?.internal ?? false,
      isDev: entry.isDev ?? false,
      repo,
    }
  })
}

export const licenseBases = data.bases
export const licenseEntries: AnnotatedLicenseEntry[] = buildAnnotatedEntries()
export const licenseCategories: CategoryDef[] = categories

// Group entries by category, sorted by category order.
export function groupByCategory(
  entries: AnnotatedLicenseEntry[],
): Array<{ category: CategoryDef; entries: AnnotatedLicenseEntry[] }> {
  const catMap = new Map<string, CategoryDef>()
  for (const cat of categories) catMap.set(cat.id, cat)

  const groups = new Map<string, AnnotatedLicenseEntry[]>()
  for (const entry of entries) {
    const cat = entry.category
    if (!groups.has(cat)) groups.set(cat, [])
    groups.get(cat)!.push(entry)
  }

  return [...groups.entries()]
    .map(([id, entries]) => ({
      category: catMap.get(id) ?? { id, label: id, order: 98 },
      entries: entries.sort((a, b) => a.name.localeCompare(b.name)),
    }))
    .sort((a, b) => a.category.order - b.category.order)
}

// Summary stats for the /licenses page header.
export function licenseStats() {
  const goCount = licenseEntries.filter(
    (e) => e.source === 'go' || e.source === 'both',
  ).length
  const jsCount = licenseEntries.filter(
    (e) => e.source === 'js' || e.source === 'both',
  ).length
  const spdxSet = new Set(licenseEntries.map((e) => e.spdx))
  return {
    total: licenseEntries.length,
    goCount,
    jsCount,
    uniqueLicenses: spdxSet.size,
  }
}
