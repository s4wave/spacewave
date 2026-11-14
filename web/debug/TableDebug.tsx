import { useParams } from '@s4wave/web/router/router.js'
import {
  type ListItem,
  type ListState,
  useListState,
} from '@s4wave/web/ui/list'
import { Table, type TableColumn } from '@s4wave/web/ui/table/Table.js'

// Peer is one fixture record: a peer a Device can reach.
interface Peer {
  id: string
  state: 'up' | 'down'
  latencyMs: number
  address: string
  note: string
}

// columns covers every step: State from 16rem, Latency from 22rem, Address
// from 40rem, and Note, too wide for any step, only as a fact.
const columns: TableColumn<Peer>[] = [
  { key: 'peer', label: 'Peer', kind: 'id', text: (p) => p.id },
  {
    key: 'state',
    label: 'State',
    minWidth: 4.5,
    text: (p) => (p.state === 'up' ? 'Up' : 'Down'),
  },
  {
    key: 'address',
    label: 'Address',
    kind: 'id',
    minWidth: 9,
    priority: 3,
    text: (p) => p.address,
  },
  {
    key: 'latency',
    label: 'Latency',
    minWidth: 3.5,
    priority: 2,
    end: true,
    text: (p) => `${p.latencyMs} ms`,
    compare: (a, b) => a.latencyMs - b.latencyMs,
  },
  { key: 'note', label: 'Note', minWidth: 14, text: (p) => p.note },
]

// peer builds one fixture row.
function peer(
  id: string,
  state: Peer['state'],
  latencyMs: number,
  address: string,
  note: string,
): ListItem<Peer> {
  return { id, data: { id, state, latencyMs, address, note } }
}

// peers are a dozen fixture peers with short and long ids.
const peers: ListItem<Peer>[] = [
  peer(
    '12D3KooWQx7Hc4ZpE1vN2mXq9',
    'up',
    12,
    '/ip4/10.0.0.4/udp/4242/quic',
    'Laptop',
  ),
  peer(
    '12D3KooWBr8sL0aT5yK3dPq2',
    'up',
    48,
    '/ip4/192.168.1.20/tcp/443',
    'Office desktop',
  ),
  peer(
    '12D3KooWJm2Vf9nC6xH1sRt7',
    'down',
    0,
    '/dns4/relay.example.net/tcp/443/wss',
    'Relay, last seen yesterday',
  ),
  peer(
    '12D3KooWLp4Gd7kX2wQ8zNc5',
    'up',
    230,
    '/ip6/2001:db8::7/udp/4242/quic',
    'Phone on cellular',
  ),
  peer(
    '12D3KooWTs9Ha3mB6vF0yLe1',
    'up',
    7,
    '/ip4/10.0.0.9/udp/4242/quic',
    'Build host',
  ),
  peer(
    '12D3KooWNc6Jy1qP8rD4uKa3',
    'down',
    0,
    '/ip4/172.16.4.2/tcp/4001',
    'Old NAS',
  ),
  peer(
    '12D3KooWRf3Mb5tZ9eX7wGh2',
    'up',
    95,
    '/ip4/203.0.113.8/udp/4242/quic',
    'Cloud worker',
  ),
  peer(
    '12D3KooWAz7Kc2nV4pS1xJq8',
    'up',
    31,
    '/ip4/10.0.0.12/tcp/4001',
    'Tablet',
  ),
]

// longIds are peers whose ids and addresses are longer than any cell holds.
const longIds: ListItem<Peer>[] = [
  peer(
    '12D3KooWQx7Hc4ZpE1vN2mXq9AbCdEfGhIjKlMnOpQrStUvWx',
    'up',
    12,
    '/dns4/a-very-long-relay-hostname.internal.example.net/tcp/443/wss/p2p-circuit',
    'A note long enough that it never fits on one line of the facts',
  ),
  peer(
    '12D3KooWBr8sL0aT5yK3dPq2ZyXwVuTsRqPoNmLkJiHgFeDcBa',
    'down',
    0,
    '/ip6/2001:db8:85a3::8a2e:370:7334/udp/4242/quic-v1',
    'Second',
  ),
]

// fixtures are the table states the sweep opens, by the route's state segment.
const fixtures: Record<
  string,
  { rows: ListItem<Peer>[]; list?: Partial<ListState> }
> = {
  populated: { rows: peers },
  selected: {
    rows: peers,
    list: { selectedIds: [peers[2].id], lastSelectedIndex: 2, focusedIndex: 2 },
  },
  range: {
    rows: peers,
    list: {
      selectedIds: [peers[1].id, peers[2].id, peers[3].id],
      lastSelectedIndex: 3,
      focusedIndex: 3,
    },
  },
  focused: { rows: peers, list: { focusedIndex: 4 } },
  'by-latency': {
    rows: peers,
    list: { sortKey: 'latency', sortDirection: 'desc' },
  },
  'long-ids': { rows: longIds, list: { selectedIds: [longIds[0].id] } },
  empty: { rows: [] },
}

// TableDebug renders the real Table in one fixture state, with the viewport
// as its pane, for the viewer sweep. The route's state segment picks the
// fixture; an unknown or missing one shows the populated table.
export function TableDebug() {
  const { state: name } = useParams()
  const fixture = fixtures[name ?? ''] ?? fixtures.populated
  return <TableFixtureView key={name} {...fixture} />
}

// TableFixtureView holds the fixture's list state in memory.
function TableFixtureView({
  rows,
  list,
}: {
  rows: ListItem<Peer>[]
  list?: Partial<ListState>
}) {
  const state = useListState(null, 'tableDebug', {
    sortKey: 'peer',
    sortDirection: 'asc',
    ...list,
  })
  return (
    <div className="viewer-pane bg-background flex h-full w-full flex-col">
      <Table
        rows={rows}
        columns={columns}
        state={state}
        placeholder="No peers"
        className="max-w-viewer-table min-h-0 flex-1"
      />
    </div>
  )
}
