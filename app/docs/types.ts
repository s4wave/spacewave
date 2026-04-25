// DocFrontmatter represents the YAML frontmatter of a documentation page.
export interface DocFrontmatter {
  title: string
  section: string
  order: number
  summary: string
  draft?: boolean
}

// DocPage represents a parsed documentation page.
export interface DocPage {
  slug: string
  url: string
  title: string
  site: string
  section: string
  order: number
  summary: string
  body: string
  // filename is the original filename (e.g., "01-create-a-space.md").
  filename: string
}

// DocSection represents a group of documentation pages.
export interface DocSection {
  id: string
  label: string
  site: string
  order: number
  pages: DocPage[]
}
