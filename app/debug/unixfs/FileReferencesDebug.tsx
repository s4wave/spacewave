import { useParams } from '@s4wave/web/router/router.js'

import { FileReferencesStrip } from '@s4wave/app/unixfs/FileReferences.js'
import { TextFileFrame } from '@s4wave/app/unixfs/UnixFSTextFileViewer.js'
import {
  fileReferencesView,
  type FileReference,
} from '@s4wave/app/unixfs/file-references.js'
import {
  deriveTextFileView,
  initialTextFileState,
  reduceTextFile,
  type TextContent,
} from '@s4wave/app/unixfs/text-file.js'
import type { KeyedSet } from '@s4wave/sdk/world/entity-set.js'

// noop stands in for every press; a fixture holds one state.
const noop = () => {}

const text = Array.from(
  { length: 30 },
  (_, i) => `${i + 1}. Design note: the docking arm reaches bay ${i + 1}.`,
).join('\n')
const content: TextContent = {
  preview: null,
  crlf: false,
  version: new Uint8Array([1]),
  text,
}
const file = deriveTextFileView(
  { value: content, loading: false, error: null },
  reduceTextFile(initialTextFileState, { type: 'read', content }),
  true,
)

// want is the strip's first page.
const want = 8

const observation = 'glados/observation'
const session = 'glados/llm-session'

// referrers builds a read of the given referrers, in key order.
function referrers(
  rows: [key: string, typeId: string][],
): KeyedSet<FileReference> {
  return {
    seqno: 1n,
    keys: rows.map(([key]) => key),
    rows: new Map(rows.map(([key, typeId]) => [key, { typeId }])),
    complete: true,
  }
}

const found: [string, string][] = [
  ['glados/sessions/7xht2k/llm-session', session],
  ['glados/sessions/7xht2k/observations/1', observation],
  ['glados/sessions/7xht2k/observations/2', observation],
]

// fixtures are the strip's states in FileReferences.tla, each read as the
// watch yields it: loading, failed, none, found, more (a page with more
// past it) and untyped (a referrer with no type, such as a placeholder).
const fixtures: Record<string, Parameters<typeof fileReferencesView>[0]> = {
  loading: { value: null, retry: noop },
  failed: { error: new Error('stream closed'), retry: noop },
  none: { value: referrers([]), retry: noop },
  found: { value: referrers(found), retry: noop },
  more: {
    value: referrers(
      Array.from({ length: want + 1 }, (_, i) => [
        `glados/sessions/7xht2k/observations/${i + 1}`,
        observation,
      ]),
    ),
    retry: noop,
  },
  untyped: {
    value: referrers([...found, ['glados/placeholders/design-review', '']]),
    retry: noop,
  },
}

/**
 * FileReferencesDebug renders a text file with its references strip in one
 * state of FileReferences.tla, with the viewport as the pane, for the viewer
 * sweep: loading, failed, none, found, more and untyped. An unknown or
 * missing state shows found.
 */
export function FileReferencesDebug() {
  const { state = 'found' } = useParams()
  const fixture = fixtures[state] ?? fixtures.found
  return (
    <div className="viewer-pane bg-file-back flex h-full w-full flex-col overflow-hidden">
      <TextFileFrame
        view={file}
        path="/design.md"
        description="Markdown, 1.6 KB"
        dispatch={noop}
        onSave={noop}
        retryRead={noop}
        readBack={noop}
      />
      <FileReferencesStrip
        view={fileReferencesView(fixture, want)}
        onMore={noop}
      />
    </div>
  )
}
