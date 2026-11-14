import { useMemo } from 'react'
import { useParams } from '@s4wave/web/router/router.js'
import { useListState, type ListState } from '@s4wave/web/ui/list'
import { FileList } from '@s4wave/web/editors/file-browser/FileList.js'
import type {
  FileEntry,
  FileEntryDetails,
  FileEntryDetailsMap,
} from '@s4wave/web/editors/file-browser/types.js'

// FileListFixture is one state of the file list the debug page can open.
interface FileListFixture {
  entries: FileEntry[]
  list?: Partial<ListState>
  loadingId?: string
}

// folder is a fixture folder of a dozen entries with short and long names.
const folder: FileEntry[] = [
  { id: 'docs', name: 'docs', isDir: true },
  { id: 'photos', name: 'Photos from the 2026 field trip', isDir: true },
  { id: 'src', name: 'src', isDir: true },
  { id: 'readme', name: 'README.md' },
  { id: 'license', name: 'LICENSE' },
  { id: 'notes', name: 'meeting-notes-quarterly-planning-final-v3.org' },
  { id: 'logo', name: 'logo.svg' },
  { id: 'video', name: 'demo-recording.mp4' },
  { id: 'data', name: 'measurements.csv' },
  { id: 'link', name: 'current', isSymlink: true },
  { id: 'archive', name: 'backup-2026-11-01.tar.zst' },
  { id: 'config', name: 'spacewave.yaml' },
]

// longNames is a folder whose names are longer than any column holds.
const longNames: FileEntry[] = [
  { id: 'a', name: 'a-very-long-directory-name-that-never-fits', isDir: true },
  { id: 'b', name: 'quarterly-report-with-appendices-and-tables-final.pdf' },
  { id: 'c', name: 'IMG_20261113_224512_HDR_portrait_edited_copy.jpeg' },
]

// fixtures are the list states the sweep opens, by the route's state segment.
const fixtures: Record<string, FileListFixture> = {
  populated: { entries: folder },
  selected: {
    entries: folder,
    list: {
      selectedIds: ['readme', 'license', 'notes'],
      lastSelectedIndex: 5,
      focusedIndex: 5,
    },
  },
  focused: { entries: folder, list: { focusedIndex: 4 } },
  'by-date': {
    entries: folder,
    list: { sortKey: 'date', sortDirection: 'desc' },
  },
  'by-size': {
    entries: folder,
    list: { sortKey: 'size', sortDirection: 'asc' },
  },
  'long-names': { entries: longNames, list: { selectedIds: ['b'] } },
  'loading-row': { entries: folder, loadingId: 'photos' },
  empty: { entries: [] },
}

// entryDetails gives each fixture entry a fixed date and, for files, a size.
function entryDetails(entries: FileEntry[]): FileEntryDetailsMap {
  return new Map(
    entries.map((entry, index) => {
      const details: FileEntryDetails = {
        modTime: new Date(2026, 10, 1 + ((index * 7) % 28)),
      }
      if (!entry.isDir) details.size = 37 * 1024 ** (index % 4) * (index + 1)
      return [entry.id, details]
    }),
  )
}

// FileListDebug renders the real FileList in one fixture state, with the
// viewport as its pane, for the viewer sweep. The route's state segment picks
// the fixture; an unknown or missing one shows the populated folder.
export function FileListDebug() {
  const { state: name } = useParams()
  const fixture = fixtures[name ?? ''] ?? fixtures.populated
  return <FileListFixtureView key={name} fixture={fixture} />
}

// FileListFixtureView holds the fixture's list state in memory.
function FileListFixtureView({ fixture }: { fixture: FileListFixture }) {
  const state = useListState(null, 'fileListDebug', {
    sortKey: 'name',
    sortDirection: 'asc',
    ...fixture.list,
  })

  // Give the fixture's entries their dates and sizes once.
  const details = useMemo(
    () => entryDetails(fixture.entries),
    [fixture.entries],
  )
  return (
    <div className="bg-background flex h-full w-full flex-col">
      <FileList
        entries={fixture.entries}
        entryDetails={details}
        state={state}
        loadingId={fixture.loadingId}
        placeholder="This folder is empty"
      />
    </div>
  )
}
