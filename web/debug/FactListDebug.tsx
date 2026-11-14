import { useParams } from '@s4wave/web/router/router.js'
import {
  FactList,
  FactRow,
  type FactRowProps,
} from '@s4wave/web/ui/FactList.js'

// peerId is a 46-character peer ID, the length the sweep reports.
const peerId = '12D3KooWQx7Hc4Lr8VbZ3TfPa9WkEe5Yd2Hs7GuLN2mXq9'

// url is a 60-character URL whose last segment is its tail.
const url = 'https://github.com/aperturerobotics/mirror-2026-11/spacewave'

// fixtures are the fact lists the sweep opens, by the route's state segment.
const fixtures: Record<string, FactRowProps[]> = {
  // ids holds identifiers long and short, each shortened in its middle.
  ids: [
    { label: 'Peer ID', value: peerId },
    { label: 'Remote', value: url },
    {
      label: 'Object key',
      value: 'spaces/home/objects/notes/2026/quarterly-planning',
    },
    {
      label: 'Root ref',
      value: 'bafkreihdwdcefgh4dqkjv67uzcmw7ojee6xedzdetojuzjevtenxquvyku',
    },
    { label: 'Type ID', value: 'git/repo' },
    // A head of four characters or fewer hands the ellipsis to the tail.
    { label: 'Path', value: 'src/some-very-long-file-name-for-the-sweep.tsx' },
    { label: 'Short', value: 'abc/defghijklmnopqrstuvwxyz-0123456789-tail' },
    // A value with no separator keeps its last six characters.
    { label: 'Token', value: 'abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGH' },
    // A URL draws without its scheme and keeps its last real segment.
    { label: 'Clone', value: 'https://github.com/aperturerobotics/spacewave/' },
    // Every scheme but https:// stays drawn.
    { label: 'Mirror', value: 'http://github.com/aperturerobotics/spacewave' },
    {
      label: 'Bucket',
      value: 's3://spacewave-releases/nightly/desktop.tar.gz',
    },
  ],
  // text holds plain facts, which end in an ellipsis when they do not fit.
  text: [
    { label: 'Name', value: 'production-cluster-west', kind: 'text' },
    { label: 'Created', value: '2026-11-13T23:41:07.512Z', kind: 'text' },
    {
      label: 'Description',
      value: 'Nightly build of the desktop app for every platform we ship',
      kind: 'text',
    },
    { label: 'Rev', value: '42', kind: 'text', control: null },
  ],
  // two-fact holds rows with a second fact under the value.
  'two-fact': [
    {
      label: 'HEAD',
      value: '4a4795a73c1f0e2b9d8a7c6e5f4b3a2918d7c6b5',
      sub: 'web/object: make the viewer slot a size container',
    },
    { label: 'Remote', value: url, sub: 'Fetched 12m ago' },
    { label: 'Peer ID', value: peerId },
  ],
  // narrow holds short heads in a 7rem list, whose value column holds the
  // least an id shows: four head characters, an ellipsis and six of its tail.
  narrow: [
    { label: 'Short', value: 'abcdefghij' },
    { label: 'Type ID', value: 'git/repo-with-a-long-name' },
    { label: 'Path', value: 'src/some-very-long-file-name.tsx' },
    { label: 'Peer ID', value: peerId },
  ],
  // tight holds the same rows in a 6rem list, below that least, where an id
  // is starved but still marks its cut with an ellipsis.
  tight: [
    { label: 'Short', value: 'abcdefghij' },
    { label: 'Type ID', value: 'git/repo-with-a-long-name' },
    { label: 'Path', value: 'src/some-very-long-file-name.tsx' },
    { label: 'Peer ID', value: peerId },
  ],
}

// FactListDebug renders FactRows in one fixture state inside a viewer pane,
// with the viewport as the pane, for the viewer sweep. The route's state
// segment picks the fixture; an unknown or missing one shows the ids.
export function FactListDebug() {
  const { state: name } = useParams()
  const rows = fixtures[name ?? ''] ?? fixtures.ids
  return (
    <div className="bg-background viewer-pane h-full w-full">
      <div className="viewer-inset">
        <FactList
          className={
            name === 'narrow' ? 'w-28' : name === 'tight' ? 'w-24' : undefined
          }
        >
          {rows.map((row) => (
            <FactRow key={row.label} {...row} />
          ))}
        </FactList>
      </div>
    </div>
  )
}
