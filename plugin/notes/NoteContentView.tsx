import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import { parseObjectUri } from '@s4wave/sdk/space/object-uri.js'
import {
  useUnixFSRootHandle,
  useUnixFSHandle,
  useUnixFSHandleTextContent,
} from '@s4wave/web/hooks/useUnixFSHandle.js'
import { cn } from '@s4wave/web/style/utils.js'
import { LuCode, LuPenLine } from 'react-icons/lu'

import { parseNote, reassembleNote } from './frontmatter.js'
import FrontmatterDisplay from './FrontmatterDisplay.js'
import LexicalEditor from './LexicalEditor.js'

interface NoteContentViewProps {
  worldState: Resource<IWorldState>
  sourceRef: string
  noteName: string
  editing: boolean
  onToggleEdit: () => void
  onFilterTag?: (tag: string | undefined) => void
  onFilterStatus?: (status: string | undefined) => void
}

// NoteContentView displays a note with WYSIWYG (Lexical) or source (textarea) mode.
function NoteContentView({
  worldState,
  sourceRef,
  noteName,
  editing,
  onToggleEdit,
  onFilterTag,
  onFilterStatus,
}: NoteContentViewProps) {
  const parsed = useMemo(() => parseObjectUri(sourceRef), [sourceRef])
  const filePath = useMemo(() => {
    const base = parsed.path
    return base ? `${base}/${noteName}` : noteName
  }, [parsed.path, noteName])

  const rootHandle = useUnixFSRootHandle(worldState, parsed.objectKey)
  const fileHandle = useUnixFSHandle(rootHandle, filePath)
  const textResource = useUnixFSHandleTextContent(fileHandle)

  // Parse frontmatter from file content.
  const parsedNote = useMemo(() => {
    if (!textResource.value) return null
    return parseNote(textResource.value)
  }, [textResource.value])

  // Track the raw frontmatter for round-trip preservation.
  const rawFrontmatter = useRef('')
  useEffect(() => {
    if (parsedNote) {
      rawFrontmatter.current = parsedNote.rawFrontmatter
    }
  }, [parsedNote])

  // Source mode edit state.
  const [sourceContent, setSourceContent] = useState<string | null>(null)

  const writeFile = useCallback(
    async (content: string) => {
      const handle = fileHandle.value
      if (!handle) return
      const encoded = new TextEncoder().encode(content)
      await handle.writeAt(0n, encoded)
      await handle.truncate(BigInt(encoded.byteLength))
    },
    [fileHandle.value],
  )

  // WYSIWYG save: re-assemble frontmatter + exported body, then write.
  const handleWysiwygSave = useCallback(
    (body: string) => {
      const full = reassembleNote(rawFrontmatter.current, body)
      void writeFile(full)
    },
    [writeFile],
  )

  // Source mode blur: write the raw content.
  const handleSourceBlur = useCallback(() => {
    if (sourceContent !== null) {
      void writeFile(sourceContent)
    }
  }, [sourceContent, writeFile])

  const handleToggle = useCallback(() => {
    if (editing) {
      // Switching from source to WYSIWYG.
      if (sourceContent !== null) {
        void writeFile(sourceContent)
        setSourceContent(null)
      }
    } else {
      // Switching from WYSIWYG to source.
      setSourceContent(textResource.value ?? '')
    }
    onToggleEdit()
  }, [editing, sourceContent, textResource.value, onToggleEdit, writeFile])

  if (!noteName) {
    return (
      <div className="text-muted-foreground flex h-full items-center justify-center text-xs">
        Select a note to view
      </div>
    )
  }

  if (textResource.loading) {
    return (
      <div className="text-muted-foreground flex h-full items-center justify-center text-xs">
        Loading...
      </div>
    )
  }

  if (textResource.error) {
    return (
      <div className="text-destructive flex h-full flex-col items-center justify-center gap-2 p-4 text-xs">
        <span>Failed to load note</span>
        <span className="text-foreground-alt/50 text-xs">{textResource.error.message}</span>
      </div>
    )
  }

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center justify-between border-b border-border px-3 py-1.5">
        <span className="text-xs font-medium">
          {noteName.split('/').pop()?.replace(/\.md$/, '') ?? noteName}
        </span>
        <button
          type="button"
          className={cn(
            'flex items-center gap-1 rounded px-2 py-0.5 text-xs',
            'hover:bg-list-hover-background',
            editing ? 'text-brand' : 'text-foreground-alt',
          )}
          onClick={handleToggle}
          title={editing ? 'Switch to WYSIWYG' : 'Switch to source'}
        >
          {editing ?
            <>
              <LuPenLine className="h-3 w-3" />
              WYSIWYG
            </>
          : <>
              <LuCode className="h-3 w-3" />
              Source
            </>
          }
        </button>
      </div>
      {editing ?
        <div className="flex-1 overflow-auto">
          <textarea
            className="bg-background-primary text-editor-foreground h-full w-full resize-none border-none p-4 font-mono text-xs outline-none"
            value={sourceContent ?? textResource.value ?? ''}
            onChange={(e) => setSourceContent(e.target.value)}
            onBlur={handleSourceBlur}
          />
        </div>
      : <>
          {parsedNote && (
            <FrontmatterDisplay
              frontmatter={parsedNote.frontmatter}
              onTagClick={onFilterTag}
              onStatusClick={onFilterStatus}
            />
          )}
          <div className="flex flex-1 flex-col overflow-hidden">
            <LexicalEditor
              markdown={parsedNote?.body ?? ''}
              onSave={handleWysiwygSave}
            />
          </div>
        </>
      }
    </div>
  )
}

export default NoteContentView
