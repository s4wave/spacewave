export type NoteFileFormat = 'markdown' | 'org'

const NOTE_FILE_EXTENSIONS: Record<NoteFileFormat, string> = {
  markdown: '.md',
  org: '.org',
}

const orgTitlePattern = /^#\+TITLE:\s*(.+)$/im

export function getNoteFileFormat(name: string): NoteFileFormat | null {
  const lower = name.toLowerCase()
  if (lower.endsWith(NOTE_FILE_EXTENSIONS.org)) return 'org'
  if (lower.endsWith(NOTE_FILE_EXTENSIONS.markdown)) return 'markdown'
  return null
}

export function isNoteFileName(name: string): boolean {
  return getNoteFileFormat(name) !== null
}

export function noteFileExtension(format: NoteFileFormat): string {
  return NOTE_FILE_EXTENSIONS[format]
}

export function stripNoteFileExtension(name: string): string {
  return name.replace(/\.(md|org)$/i, '')
}

export function noteTitleFromContent(name: string, content: string): string {
  const format = getNoteFileFormat(name)
  if (format === 'org') {
    const title = content.match(orgTitlePattern)?.[1]?.trim()
    if (title) return title
  }
  return stripNoteFileExtension(name)
}

export function nextUntitledNoteName(
  existing: Set<string>,
  format: NoteFileFormat,
): string {
  const ext = noteFileExtension(format)
  let name = `untitled${ext}`
  let counter = 1
  while (existing.has(name)) {
    name = `untitled-${counter}${ext}`
    counter++
  }
  return name
}

export function normalizeNoteRename(
  currentName: string,
  nextTitle: string,
): string | null {
  const format = getNoteFileFormat(currentName)
  if (!format) return null
  const title = stripNoteFileExtension(nextTitle.trim())
  if (!title) return null
  return `${title}${noteFileExtension(format)}`
}

export function createNoteTemplate(
  name: string,
  format: NoteFileFormat,
  date = new Date(),
): string {
  const title = stripNoteFileExtension(name)
  if (format === 'org') return `#+TITLE: ${title}\n\n* ${title}\n\n`
  return `---\ncreated: ${date.toISOString().slice(0, 10)}\ntags: []\n---\n\n# ${title}\n\n`
}
