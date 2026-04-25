import {
  useCallback,
  useMemo,
  useState,
  type ComponentType,
  type ReactNode,
} from 'react'
import Markdown from 'markdown-to-jsx'
import {
  LuArrowLeft,
  LuChevronRight,
  LuCode,
  LuFileText,
  LuHardDrive,
  LuPlay,
  LuServer,
} from 'react-icons/lu'

import { MessageInput } from '@s4wave/app/chat/MessageInput.js'
import { MessageList } from '@s4wave/app/chat/MessageList.js'
import { PreBlock } from '@s4wave/app/docs/CodeBlock.js'
import type { ChatMessageInfo } from '@s4wave/sdk/chat/rpc/rpc.pb.js'
import { FileList } from '@s4wave/web/editors/file-browser/FileList.js'
import type {
  FileEntry,
  FileEntryDetails,
} from '@s4wave/web/editors/file-browser/types.js'
import { cn } from '@s4wave/web/style/utils.js'
import { StatusList, type StatusListItem } from '@s4wave/web/ui/StatusList.js'
import LexicalEditor from '../../plugin/notes/LexicalEditor.js'
import '@s4wave/app/docs/docs-prose.css'

const markdownOverrides = {
  overrides: {
    pre: { component: PreBlock },
  },
}

const DRIVE_ENTRIES: Record<string, FileEntry[]> = {
  '/': [
    { id: 'docs', name: 'docs', isDir: true },
    { id: 'media', name: 'media', isDir: true },
    { id: 'README.md', name: 'README.md' },
    { id: 'deploy.sh', name: 'deploy.sh' },
  ],
  '/docs': [
    { id: 'roadmap.md', name: 'roadmap.md' },
    { id: 'launch-plan.md', name: 'launch-plan.md' },
    { id: 'team-notes.md', name: 'team-notes.md' },
  ],
  '/media': [
    { id: 'demo.mp4', name: 'demo.mp4' },
    { id: 'cover.png', name: 'cover.png' },
    { id: 'screenshot.jpg', name: 'screenshot.jpg' },
  ],
}

const DRIVE_DETAILS: Record<string, FileEntryDetails> = {
  docs: { modTime: new Date('2026-04-17T08:15:00Z') },
  media: { modTime: new Date('2026-04-17T08:12:00Z') },
  'README.md': {
    modTime: new Date('2026-04-17T08:10:00Z'),
    size: 1536,
  },
  'deploy.sh': {
    modTime: new Date('2026-04-17T08:09:00Z'),
    size: 384,
  },
  'roadmap.md': {
    modTime: new Date('2026-04-17T08:20:00Z'),
    size: 2048,
  },
  'launch-plan.md': {
    modTime: new Date('2026-04-17T08:17:00Z'),
    size: 1024,
  },
  'team-notes.md': {
    modTime: new Date('2026-04-17T08:16:00Z'),
    size: 768,
  },
  'demo.mp4': {
    modTime: new Date('2026-04-17T08:13:00Z'),
    size: 62_914_560,
  },
  'cover.png': {
    modTime: new Date('2026-04-17T08:11:00Z'),
    size: 438_272,
  },
  'screenshot.jpg': {
    modTime: new Date('2026-04-17T08:08:00Z'),
    size: 298_144,
  },
}

const DRIVE_PREVIEWS: Record<string, string> = {
  '/README.md': `# Team Space

The same space works on laptops, phones, and servers.

\`\`\`sh
spacewave drive sync ./docs
spacewave device list --json
\`\`\`
`,
  '/deploy.sh': `#!/usr/bin/env bash
set -euo pipefail
bun run typecheck
spacewave drive sync ./docs
`,
  '/docs/roadmap.md': `# Roadmap

- Ship the landing demos
- Keep cloud optional
- Let every device join the same swarm
`,
  '/docs/launch-plan.md': `# Launch plan

1. Publish the release
2. Send the changelog
3. Open the docs hub
`,
  '/docs/team-notes.md': `# Team notes

Everyone edits the same files without waiting for a server round-trip.
`,
  '/media/demo.mp4': `# demo.mp4

Video assets open inline, sync incrementally, and stay encrypted at rest.`,
  '/media/cover.png': `# cover.png

Images stay in the same space as the docs and release assets.`,
  '/media/screenshot.jpg': `# screenshot.jpg

Capture once, sync everywhere.`,
}

const INITIAL_CHAT_MESSAGES: ChatMessageInfo[] = [
  {
    objectKey: 'msg-1',
    senderPeerId: '12D3KooWteam',
    text: 'Planning doc is in sync on the studio mac and the backup node.',
    createdAt: new Date('2026-04-17T08:00:00Z'),
  },
  {
    objectKey: 'msg-2',
    senderPeerId: '12D3KooWphone',
    text: 'Perfect. I am adding the launch checklist from mobile.',
    createdAt: new Date('2026-04-17T08:01:00Z'),
  },
]

const INITIAL_DEVICE_ITEMS: StatusListItem[] = [
  {
    id: 'studio-mac',
    label: 'studio-mac',
    status: 'success',
    detail: '23ms',
  },
  {
    id: 'rack-node',
    label: 'rack-node',
    status: 'pending',
    detail: 'relay',
  },
  {
    id: 'field-phone',
    label: 'field-phone',
    status: 'success',
    detail: 'p2p',
  },
]

const DEVICE_LOGS: Record<string, string[]> = {
  'studio-mac': [
    '$ spacewave device shell studio-mac',
    'Connected directly over bifrost',
    'tailing release logs...',
  ],
  'rack-node': [
    '$ spacewave device wake rack-node',
    'relay handshake established',
    'warming object cache and sync queues...',
  ],
  'field-phone': [
    '$ spacewave device open field-phone',
    'camera upload stream ready',
    'background sync active on cellular',
  ],
}

const INITIAL_NOTES = {
  planning: {
    title: 'Launch checklist',
    description: 'Release coordination note with tasks and code blocks.',
    body: `# Launch checklist

- [x] Ship the latest desktop build
- [x] Publish the changelog
- [ ] Post the notes and docs update

\`\`\`ts
export const launchMode = 'local-first'
\`\`\`
`,
  },
  docs: {
    title: 'Team handbook',
    description: 'Shared reference note for the project space.',
    body: `# Team handbook

Spacewave notes are plain markdown. Edit locally, preview instantly, and sync
through the same encrypted stack as the rest of your space.
`,
  },
}

const INITIAL_PLUGIN_CODE = `export default {
  name: 'release-pulse',
  command: 'release:announce',
  description: 'Turn a changelog entry into a launch checklist and status card.',
}
`

const NOTE_IDS: Array<keyof typeof INITIAL_NOTES> = ['planning', 'docs']

function DemoFrame({
  icon: Icon,
  title,
  subtitle,
  children,
}: {
  icon: ComponentType<{ className?: string }>
  title: string
  subtitle: string
  children: ReactNode
}) {
  return (
    <div className="border-foreground/8 bg-background-card/40 overflow-hidden rounded-lg border backdrop-blur-sm">
      <div className="border-foreground/8 flex items-center gap-3 border-b px-4 py-3">
        <div className="bg-brand/8 flex h-9 w-9 items-center justify-center rounded-lg">
          <Icon className="text-brand h-4 w-4" />
        </div>
        <div className="min-w-0">
          <h3 className="text-foreground text-sm font-semibold">{title}</h3>
          <p className="text-foreground-alt text-xs">{subtitle}</p>
        </div>
      </div>
      {children}
    </div>
  )
}

function resolveDrivePath(currentPath: string, entry: FileEntry): string {
  return currentPath === '/' ? `/${entry.name}` : `${currentPath}/${entry.name}`
}

function getDriveParentPath(path: string): string {
  if (path === '/') return '/'
  const parts = path.split('/').filter(Boolean)
  if (parts.length <= 1) return '/'
  return `/${parts.slice(0, -1).join('/')}`
}

function suggestReply(text: string): string {
  const lower = text.toLowerCase()
  if (lower.includes('docs')) {
    return 'Shared docs updated. Every device picked up the new markdown blocks.'
  }
  if (lower.includes('deploy') || lower.includes('release')) {
    return 'Release checklist ready. The desktop build, changelog, and notes all match.'
  }
  return 'Message synced locally. The rest of the swarm will see it as soon as they connect.'
}

function derivePluginName(code: string): string {
  const match = code.match(/name:\s*'([^']+)'/)
  return match?.[1] ?? 'unnamed-plugin'
}

function derivePluginCommand(code: string): string {
  const match = code.match(/command:\s*'([^']+)'/)
  return match?.[1] ?? 'plugin:run'
}

// DriveLandingDemo renders an interactive file-browser style demo.
export function DriveLandingDemo() {
  const [currentPath, setCurrentPath] = useState('/')
  const [selectedPath, setSelectedPath] = useState('/README.md')
  const entries = DRIVE_ENTRIES[currentPath] ?? []
  const preview = DRIVE_PREVIEWS[selectedPath] ?? '# Preview\n\nSelect a file.'

  const handleOpen = useCallback(
    (openedEntries: FileEntry[]) => {
      const entry = openedEntries[0]
      if (!entry) return
      const nextPath = resolveDrivePath(currentPath, entry)
      if (entry.isDir) {
        setCurrentPath(nextPath)
        return
      }
      setSelectedPath(nextPath)
    },
    [currentPath],
  )

  const getEntryDetails = useCallback(
    (_index: number, entry: FileEntry, _signal: AbortSignal) =>
      Promise.resolve(DRIVE_DETAILS[entry.id] ?? null),
    [],
  )

  return (
    <DemoFrame
      icon={LuHardDrive}
      title="Live file browser"
      subtitle="Open folders, inspect files, and keep the preview in sync."
    >
      <div className="grid gap-0 @lg:grid-cols-[1.2fr_1fr]">
        <div className="border-foreground/8 border-b @lg:border-r @lg:border-b-0">
          <div className="border-foreground/8 flex items-center gap-2 border-b px-3 py-2 text-xs">
            <button
              type="button"
              onClick={() => setCurrentPath(getDriveParentPath(currentPath))}
              disabled={currentPath === '/'}
              aria-label="Go to parent folder"
              className="text-foreground-alt hover:text-foreground disabled:text-foreground-alt/40"
            >
              <LuArrowLeft className="h-3.5 w-3.5" />
            </button>
            <span className="text-foreground-alt">swarm://team-space</span>
            <LuChevronRight className="text-foreground-alt h-3 w-3" />
            <span className="text-foreground truncate">{currentPath}</span>
          </div>
          <div className="h-72 overflow-hidden">
            <FileList
              entries={entries}
              getEntryDetails={getEntryDetails}
              onOpen={handleOpen}
              autoHeight={true}
              currentPath={currentPath}
            />
          </div>
        </div>
        <div className="flex min-h-72 flex-col">
          <div className="border-foreground/8 border-b px-3 py-2 text-xs">
            <span className="text-foreground">{selectedPath}</span>
          </div>
          <div className="docs-prose min-h-0 flex-1 overflow-auto px-4 py-3 text-sm">
            <Markdown options={markdownOverrides}>{preview}</Markdown>
          </div>
        </div>
      </div>
    </DemoFrame>
  )
}

// DevicesLandingDemo renders an interactive device-status and command demo.
export function DevicesLandingDemo() {
  const [items, setItems] = useState(INITIAL_DEVICE_ITEMS)
  const [selectedId, setSelectedId] = useState('studio-mac')

  const selectedItem = useMemo(
    () => items.find((item) => item.id === selectedId) ?? items[0],
    [items, selectedId],
  )

  const runAction = useCallback(
    (status: StatusListItem['status'], detail: string) => {
      setItems((current) =>
        current.map((item) =>
          item.id === selectedId ? { ...item, status, detail } : item,
        ),
      )
    },
    [selectedId],
  )

  const log = DEVICE_LOGS[selectedId] ?? []

  return (
    <DemoFrame
      icon={LuServer}
      title="Live device surface"
      subtitle="Select a node, inspect its state, and run the same control path."
    >
      <div className="grid gap-0 @lg:grid-cols-[0.95fr_1.05fr]">
        <div className="border-foreground/8 border-b p-3 @lg:border-r @lg:border-b-0">
          <StatusList
            items={items}
            className="h-72"
            onItemClick={(item) => setSelectedId(item.id)}
          />
        </div>
        <div className="flex min-h-72 flex-col gap-3 p-3">
          <div className="border-foreground/8 bg-background/40 rounded-lg border p-3">
            <div className="mb-3 flex items-center justify-between">
              <div>
                <div className="text-foreground text-sm font-semibold">
                  {selectedItem?.label}
                </div>
                <div className="text-foreground-alt text-xs">
                  {selectedItem?.detail}
                </div>
              </div>
              <div
                className={cn(
                  'rounded-full px-2 py-0.5 text-[0.55rem] font-semibold tracking-widest uppercase',
                  selectedItem?.status === 'success' &&
                    'bg-success/15 text-success',
                  selectedItem?.status === 'pending' &&
                    'bg-warning/15 text-warning',
                )}
              >
                {selectedItem?.status}
              </div>
            </div>
            <div className="flex flex-wrap gap-2">
              <button
                type="button"
                onClick={() => runAction('success', 'shell')}
                className="border-brand/30 bg-brand/10 hover:border-brand/50 hover:bg-brand/15 text-foreground rounded-md border px-3 py-1.5 text-xs transition-colors"
              >
                Open shell
              </button>
              <button
                type="button"
                onClick={() => runAction('pending', 'syncing')}
                className="border-foreground/15 bg-background/50 hover:border-foreground/25 hover:bg-background/70 text-foreground rounded-md border px-3 py-1.5 text-xs transition-colors"
              >
                Sync state
              </button>
            </div>
          </div>
          <div className="border-foreground/8 bg-background/60 flex-1 rounded-lg border p-3 font-mono text-xs backdrop-blur-sm">
            {log.map((line) => (
              <div
                key={line}
                className="text-foreground-alt mb-1 whitespace-pre-wrap"
              >
                {line}
              </div>
            ))}
          </div>
        </div>
      </div>
    </DemoFrame>
  )
}

// ChatLandingDemo renders a local interactive chat using the existing chat widgets.
export function ChatLandingDemo() {
  const [messages, setMessages] = useState(INITIAL_CHAT_MESSAGES)

  const handleSend = useCallback((text: string) => {
    setMessages((current) => {
      const nextIndex = current.length + 1
      return [
        ...current,
        {
          objectKey: `msg-${nextIndex}`,
          senderPeerId: '12D3KooWyou',
          text,
          createdAt: new Date(),
        },
        {
          objectKey: `msg-${nextIndex + 1}`,
          senderPeerId: '12D3KooWsync',
          text: suggestReply(text),
          createdAt: new Date(),
        },
      ]
    })
    return Promise.resolve()
  }, [])

  return (
    <DemoFrame
      icon={LuFileText}
      title="Live channel"
      subtitle="Send a message through the same list/input mechanics the chat viewer uses."
    >
      <div className="flex h-80 flex-col">
        <MessageList messages={messages} />
        <MessageInput onSend={handleSend} />
      </div>
    </DemoFrame>
  )
}

// NotesLandingDemo renders a local markdown editor + preview demo.
export function NotesLandingDemo() {
  const [notes, setNotes] = useState(INITIAL_NOTES)
  const [selectedId, setSelectedId] =
    useState<keyof typeof INITIAL_NOTES>('planning')

  const selectedNote = notes[selectedId]

  const handleSave = useCallback(
    (body: string) => {
      setNotes((current) => {
        const next = current[selectedId]
        if (!next || next.body === body) return current
        return {
          ...current,
          [selectedId]: {
            ...next,
            body,
          },
        }
      })
    },
    [selectedId],
  )

  return (
    <DemoFrame
      icon={LuFileText}
      title="Real notebook editor"
      subtitle="Try out the markdown editor below."
    >
      <div className="grid gap-0 @lg:grid-cols-[0.65fr_1.35fr]">
        <div className="border-foreground/8 border-b p-3 @lg:border-r @lg:border-b-0">
          <div className="mb-3 space-y-1">
            <div className="text-foreground text-sm font-semibold">
              Demo notebook
            </div>
            <div className="text-foreground-alt text-xs leading-relaxed">
              Pick a note, type in the real editor, then try the toolbar or
              <span className="text-foreground"> / </span>
              commands.
            </div>
          </div>
          <div className="space-y-2">
            {NOTE_IDS.map((id) => (
              <button
                key={id}
                type="button"
                onClick={() => setSelectedId(id)}
                className={cn(
                  'w-full rounded-md border px-3 py-2 text-left transition-colors',
                  selectedId === id ?
                    'border-brand/40 bg-brand/10 text-foreground'
                  : 'border-foreground/10 bg-background/40 text-foreground-alt hover:border-foreground/20 hover:bg-background/60 hover:text-foreground',
                )}
              >
                <div className="text-sm font-medium">{notes[id].title}</div>
                <div className="mt-1 text-xs leading-relaxed">
                  {notes[id].description}
                </div>
              </button>
            ))}
          </div>
          <div className="border-foreground/8 bg-background/50 mt-3 rounded-lg border p-3 text-xs">
            <div className="text-foreground font-medium">Try this</div>
            <div className="text-foreground-alt mt-2 space-y-1">
              <div>
                Use the block menu to insert headings, lists, tables, or embeds.
              </div>
              <div>Select text to see the floating toolbar.</div>
              <div>
                Changes save back into markdown, not a proprietary format.
              </div>
            </div>
          </div>
        </div>
        <div className="flex min-h-80 flex-col">
          <div className="border-foreground/8 flex items-center justify-between border-b px-3 py-2">
            <div className="min-w-0">
              <div className="text-foreground truncate text-sm font-semibold">
                {selectedNote.title}
              </div>
              <div className="text-foreground-alt text-xs">
                Markdown-backed Lexical editing
              </div>
            </div>
            <div className="border-brand/20 bg-brand/8 text-brand rounded-full border px-2 py-0.5 text-[0.65rem] font-semibold tracking-wide uppercase">
              live stack
            </div>
          </div>
          <div className="bg-background/60 h-[28rem] min-h-0 overflow-hidden">
            <LexicalEditor
              composerKey={selectedId}
              markdown={selectedNote.body}
              onSave={handleSave}
            />
          </div>
        </div>
      </div>
    </DemoFrame>
  )
}

// PluginsLandingDemo renders a live code-and-preview SDK demo.
export function PluginsLandingDemo() {
  const [code, setCode] = useState(INITIAL_PLUGIN_CODE)
  const pluginName = derivePluginName(code)
  const commandName = derivePluginCommand(code)

  const previewDoc = useMemo(
    () => `### ${pluginName}

- command: \`${commandName}\`
- runtime: \`go + typescript\`
- reload: hot

\`\`\`ts
${code.trim()}
\`\`\`
`,
    [code, commandName, pluginName],
  )

  return (
    <DemoFrame
      icon={LuCode}
      title="SDK code preview"
      subtitle="Edit the plugin skeleton and watch the derived preview update."
    >
      <div className="grid gap-0 @lg:grid-cols-[1fr_1fr]">
        <div className="border-foreground/8 border-b p-3 @lg:border-r @lg:border-b-0">
          <textarea
            aria-label="Plugin source"
            value={code}
            onChange={(event) => setCode(event.target.value)}
            className="border-foreground/10 bg-background/60 text-foreground focus:border-brand/50 h-80 w-full rounded-md border p-3 font-mono text-xs backdrop-blur-sm transition-colors outline-none"
          />
        </div>
        <div className="flex min-h-80 flex-col gap-3 p-3">
          <div className="border-foreground/8 bg-background/40 rounded-lg border p-3">
            <div className="mb-2 flex items-center gap-2">
              <LuPlay className="text-brand h-4 w-4" />
              <span className="text-foreground text-sm font-semibold">
                Derived preview
              </span>
            </div>
            <div className="text-foreground-alt text-xs">
              Command palette entry:{' '}
              <span className="text-foreground">{commandName}</span>
            </div>
          </div>
          <div className="docs-prose min-h-0 flex-1 overflow-auto text-sm">
            <Markdown options={markdownOverrides}>{previewDoc}</Markdown>
          </div>
        </div>
      </div>
    </DemoFrame>
  )
}
