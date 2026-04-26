// Human-written metadata per dependency for the community page and
// /licenses page. Keyed by package name (Go module path or npm package name).
// Only annotated deps get purpose text and category grouping; unannotated
// deps still appear with a default "uncategorized" category.

export interface DependencyAnnotation {
  category: string
  purpose: string
  internal: boolean
  repo?: string
}

export interface CategoryDef {
  id: string
  label: string
  order: number
}

export const categories: CategoryDef[] = [
  { id: 'internal', label: 'Aperture Robotics', order: 0 },
  { id: 'ui-framework', label: 'UI Frameworks', order: 1 },
  { id: 'ui-components', label: 'UI Components', order: 2 },
  { id: 'rich-text', label: 'Rich Text Editing', order: 3 },
  { id: 'database', label: 'Database', order: 4 },
  { id: 'crypto', label: 'Cryptography', order: 5 },
  { id: 'networking', label: 'Networking & RPC', order: 6 },
  { id: 'build', label: 'Build Tools', order: 7 },
  { id: 'go-infra', label: 'Go Infrastructure', order: 8 },
  { id: 'go-crypto', label: 'Go Cryptography', order: 9 },
  { id: 'go-storage', label: 'Go Storage', order: 10 },
  { id: 'go-networking', label: 'Go Networking', order: 11 },
  { id: 'go-runtime', label: 'Go Runtime', order: 12 },
  { id: 'uncategorized', label: 'Other', order: 99 },
]

export const annotations: Record<string, DependencyAnnotation> = {
  // Internal: Aperture Robotics packages (JS)
  '@aptre/flex-layout': {
    category: 'internal',
    purpose: 'Flexible panel layout engine for resizable, dockable UI panels',
    internal: true,
    repo: 'https://github.com/aperturerobotics/flex-layout',
  },
  '@aptre/it-ws': {
    category: 'internal',
    purpose: 'WebSocket async iterator streams for browser and Node.js',
    internal: true,
    repo: 'https://github.com/aperturerobotics/it-ws',
  },
  '@aptre/protobuf-es-lite': {
    category: 'internal',
    purpose: 'Lightweight protobuf runtime for TypeScript with no reflection',
    internal: true,
    repo: 'https://github.com/aperturerobotics/protobuf-es-lite',
  },

  // Internal: Aperture Robotics packages (Go)
  'github.com/aperturerobotics/controllerbus': {
    category: 'internal',
    purpose:
      'Controller coordination kernel with directive-based dependency resolution',
    internal: true,
    repo: 'https://github.com/aperturerobotics/controllerbus',
  },
  'github.com/s4wave/spacewave/net': {
    category: 'internal',
    purpose: 'Peer-to-peer network routing engine with pluggable transports',
    internal: true,
    repo: 'https://github.com/s4wave/spacewave/net',
  },
  'github.com/s4wave/spacewave/db': {
    category: 'internal',
    purpose:
      'Block-DAG storage engine with SQL, graph, file, and KV interfaces',
    internal: true,
    repo: 'https://github.com/s4wave/spacewave/db',
  },
  'github.com/aperturerobotics/starpc': {
    category: 'internal',
    purpose: 'Streaming bidirectional RPC framework over any transport',
    internal: true,
    repo: 'https://github.com/aperturerobotics/starpc',
  },
  'github.com/aperturerobotics/protobuf-go-lite': {
    category: 'internal',
    purpose: 'Lightweight protobuf code generation without reflection',
    internal: true,
    repo: 'https://github.com/aperturerobotics/protobuf-go-lite',
  },
  'github.com/s4wave/spacewave/forge': {
    category: 'internal',
    purpose: 'Task orchestration engine with state machine workflows',
    internal: true,
    repo: 'https://github.com/s4wave/spacewave/forge',
  },
  'github.com/aperturerobotics/cayley': {
    category: 'internal',
    purpose: 'Graph database with quad-based predicate queries',
    internal: true,
    repo: 'https://github.com/aperturerobotics/cayley',
  },
  'github.com/s4wave/spacewave/bldr': {
    category: 'internal',
    purpose: 'Application build system with Go/TS/WASM compilation',
    internal: true,
    repo: 'https://github.com/s4wave/spacewave/bldr',
  },
  'github.com/s4wave/spacewave/auth': {
    category: 'internal',
    purpose: 'Authentication and authorization framework',
    internal: true,
    repo: 'https://github.com/s4wave/spacewave/auth',
  },
  'github.com/s4wave/spacewave/identity': {
    category: 'internal',
    purpose: 'Cryptographic identity management and key derivation',
    internal: true,
    repo: 'https://github.com/s4wave/spacewave/identity',
  },
  'github.com/aperturerobotics/entitygraph': {
    category: 'internal',
    purpose: 'Entity-relationship graph layer for structured data',
    internal: true,
    repo: 'https://github.com/aperturerobotics/entitygraph',
  },
  'github.com/aperturerobotics/util': {
    category: 'internal',
    purpose:
      'Shared utility packages for broadcast, keyed containers, and concurrency',
    internal: true,
    repo: 'https://github.com/aperturerobotics/util',
  },
  'github.com/aperturerobotics/cli': {
    category: 'internal',
    purpose: 'CLI framework for controllerbus applications',
    internal: true,
    repo: 'https://github.com/aperturerobotics/cli',
  },
  'github.com/aperturerobotics/fastjson': {
    category: 'internal',
    purpose: 'Fast JSON parser and serializer without reflection',
    internal: true,
    repo: 'https://github.com/aperturerobotics/fastjson',
  },

  // Internal: Aperture forks (Go)
  'github.com/aperturerobotics/bbolt': {
    category: 'go-storage',
    purpose: 'Embedded key/value database (fork with multi-process support)',
    internal: true,
    repo: 'https://github.com/aperturerobotics/bbolt',
  },
  'github.com/aperturerobotics/go-quickjs-wasi-reactor': {
    category: 'go-runtime',
    purpose: 'QuickJS JavaScript engine compiled to WASM reactor',
    internal: true,
    repo: 'https://github.com/aperturerobotics/go-quickjs-wasi-reactor',
  },
  'github.com/aperturerobotics/bldr-saucer': {
    category: 'internal',
    purpose: 'Chromium embedding for desktop application builds',
    internal: true,
    repo: 'https://github.com/aperturerobotics/bldr-saucer',
  },
  'github.com/aperturerobotics/esbuild': {
    category: 'build',
    purpose: 'JavaScript bundler (fork for WASM build support)',
    internal: true,
    repo: 'https://github.com/aperturerobotics/esbuild',
  },
  'github.com/aperturerobotics/go-kvfile': {
    category: 'go-storage',
    purpose: 'Key-value file storage with append-only log',
    internal: true,
    repo: 'https://github.com/aperturerobotics/go-kvfile',
  },

  // UI frameworks (JS)
  react: {
    category: 'ui-framework',
    purpose: 'Component-based UI rendering',
    internal: false,
  },
  'react-dom': {
    category: 'ui-framework',
    purpose: 'React DOM rendering and hydration',
    internal: false,
  },
  'react-router': {
    category: 'ui-framework',
    purpose: 'Client-side routing for React applications',
    internal: false,
  },

  // UI components (JS)
  sonner: {
    category: 'ui-components',
    purpose: 'Toast notification system',
    internal: false,
  },
  cmdk: {
    category: 'ui-components',
    purpose: 'Command palette component',
    internal: false,
  },
  'react-icons': {
    category: 'ui-components',
    purpose: 'Icon library with Lucide, Remix, and Phosphor icon sets',
    internal: false,
  },
  'class-variance-authority': {
    category: 'ui-components',
    purpose: 'Type-safe component variant management',
    internal: false,
  },
  clsx: {
    category: 'ui-components',
    purpose: 'Conditional CSS class name composition',
    internal: false,
  },
  'tailwind-merge': {
    category: 'ui-components',
    purpose: 'Tailwind CSS class deduplication and merging',
    internal: false,
  },

  // Rich text (JS)
  lexical: {
    category: 'rich-text',
    purpose: 'Extensible rich text editor framework',
    internal: false,
    repo: 'https://github.com/facebook/lexical',
  },

  // Networking (JS)
  'it-length-prefixed': {
    category: 'networking',
    purpose: 'Length-prefixed message framing for async iterators',
    internal: false,
  },
  'it-pipe': {
    category: 'networking',
    purpose: 'Async iterator pipeline composition',
    internal: false,
  },
  uint8arraylist: {
    category: 'networking',
    purpose: 'Efficient byte buffer list for streaming protocols',
    internal: false,
  },

  // Crypto (JS)
  multiformats: {
    category: 'crypto',
    purpose: 'Self-describing data format codecs (CID, multicodec, multihash)',
    internal: false,
  },

  // Go infrastructure
  'github.com/pkg/errors': {
    category: 'go-infra',
    purpose: 'Error wrapping with stack traces',
    internal: false,
  },
  'github.com/sirupsen/logrus': {
    category: 'go-infra',
    purpose: 'Structured logging',
    internal: false,
  },
  'github.com/Jeffail/gabs/v2': {
    category: 'go-infra',
    purpose: 'JSON path manipulation and parsing',
    internal: false,
  },

  // Go cryptography
  'filippo.io/age': {
    category: 'go-crypto',
    purpose: 'File encryption with modern algorithms',
    internal: false,
  },
  'filippo.io/edwards25519': {
    category: 'go-crypto',
    purpose: 'Edwards25519 elliptic curve operations',
    internal: false,
  },
  'filippo.io/hpke': {
    category: 'go-crypto',
    purpose: 'Hybrid Public Key Encryption (RFC 9180)',
    internal: false,
  },
  'golang.org/x/crypto': {
    category: 'go-crypto',
    purpose: 'Extended cryptographic algorithms (chacha20, curve25519, nacl)',
    internal: false,
  },
  'github.com/libp2p/go-libp2p-crypto': {
    category: 'go-crypto',
    purpose: 'Cryptographic key types for peer-to-peer identity',
    internal: false,
  },

  // Go storage
  'github.com/dgraph-io/badger': {
    category: 'go-storage',
    purpose: 'High-performance key-value store with LSM tree',
    internal: false,
  },
  'github.com/ipfs/go-datastore': {
    category: 'go-storage',
    purpose: 'Datastore interface for content-addressed storage',
    internal: false,
  },

  // Go networking
  'github.com/aperturerobotics/go-multiaddr': {
    category: 'go-networking',
    purpose: 'Self-describing network address format',
    internal: true,
  },
  'github.com/aperturerobotics/go-multistream': {
    category: 'go-networking',
    purpose: 'Protocol multiplexing over streams',
    internal: true,
  },
  'github.com/aperturerobotics/go-websocket': {
    category: 'go-networking',
    purpose: 'WebSocket client and server (fork for WASM compatibility)',
    internal: true,
  },
  'github.com/quic-go/quic-go': {
    category: 'go-networking',
    purpose: 'QUIC transport protocol implementation',
    internal: false,
  },

  // Go runtime
  'github.com/tetratelabs/wazero': {
    category: 'go-runtime',
    purpose: 'WebAssembly runtime with zero dependencies',
    internal: false,
  },
  'golang.org/x/sys': {
    category: 'go-runtime',
    purpose: 'Low-level OS interaction (syscalls, signals, memory mapping)',
    internal: false,
  },
  'golang.org/x/net': {
    category: 'go-networking',
    purpose: 'Extended networking (HTTP/2, WebSocket, DNS)',
    internal: false,
  },
  'golang.org/x/text': {
    category: 'go-infra',
    purpose: 'Unicode text processing and internationalization',
    internal: false,
  },

  // Go multiformat forks
  'github.com/aperturerobotics/go-multicodec': {
    category: 'go-networking',
    purpose: 'Self-describing codec identifiers',
    internal: true,
  },
  'github.com/aperturerobotics/go-multihash': {
    category: 'go-crypto',
    purpose: 'Self-describing hash function identifiers',
    internal: true,
  },
  'github.com/aperturerobotics/go-varint': {
    category: 'go-networking',
    purpose: 'Variable-length integer encoding for protocols',
    internal: true,
  },
  'github.com/aperturerobotics/fsnotify': {
    category: 'go-infra',
    purpose: 'Filesystem event notification (fork for WASM compatibility)',
    internal: true,
  },
  'github.com/aperturerobotics/json-iterator-lite': {
    category: 'go-infra',
    purpose: 'Fast JSON iterator without reflection',
    internal: true,
  },
}
