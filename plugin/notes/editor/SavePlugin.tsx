import { useEffect, useEffectEvent, useRef } from 'react'
import { useLexicalComposerContext } from '@lexical/react/LexicalComposerContext'

interface SavePluginProps {
  savedContent: string
  exportString: () => string
  onSave: (content: string) => void | Promise<void>
  onDraftChange?: (content: string) => void
  onDirty?: () => void
  debounceMs?: number
}

// SavePlugin exports text from Lexical state on debounce and blur. It writes
// only after an edit: the baseline is the export of the note as loaded, so an
// export that normalizes the source text, or an update that only moves the
// selection, never reaches onSave.
function SavePlugin({
  savedContent,
  exportString,
  onSave,
  onDraftChange,
  onDirty,
  debounceMs = 2000,
}: SavePluginProps) {
  const [editor] = useLexicalComposerContext()
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)
  // lastExported is the export that matches the file, not the file text itself.
  const lastExported = useRef(savedContent)
  const failedExport = useRef<string | null>(null)
  const pendingExports = useRef(new Set<string>())

  const doExport = useEffectEvent(async () => {
    let content = ''
    editor.getEditorState().read(() => {
      content = exportString()
    })
    if (
      content === lastExported.current ||
      content === failedExport.current ||
      pendingExports.current.has(content)
    ) {
      return
    }

    onDraftChange?.(content)
    pendingExports.current.add(content)
    try {
      await onSave(content)
      lastExported.current = content
      failedExport.current = null
    } catch {
      editor.getEditorState().read(() => {
        const current = exportString()
        failedExport.current = content
        if (current === content) onDraftChange?.(content)
      })
    } finally {
      pendingExports.current.delete(content)
    }
  })

  const markDirty = useEffectEvent(() => {
    onDirty?.()
    editor.getEditorState().read(() => {
      const current = exportString()
      // Publish the draft at edit time, not only at debounce fire, so the
      // save pipeline can supersede an in-flight write before it settles a
      // stale status over newer unsaved text.
      onDraftChange?.(current)
      if (failedExport.current !== null) failedExport.current = current
    })
  })

  const markLoaded = useEffectEvent(() => {
    // editor.read first commits the initial state if it is still pending.
    lastExported.current = editor.read(exportString)
  })

  useEffect(() => {
    lastExported.current = savedContent
    failedExport.current = null
  }, [savedContent])

  // Declared after the savedContent effect so the baseline on mount is the
  // export of the loaded note, not its source text.
  useEffect(() => {
    markLoaded()
  }, [editor])

  useEffect(() => {
    return editor.registerUpdateListener(
      ({ editorState, prevEditorState, dirtyElements, dirtyLeaves }) => {
        // A focus or caret move changes the editor state but not the note.
        if (
          editorState === prevEditorState ||
          (dirtyElements.size === 0 && dirtyLeaves.size === 0)
        ) {
          return
        }

        markDirty()

        if (timer.current) {
          clearTimeout(timer.current)
        }
        timer.current = setTimeout(doExport, debounceMs)
      },
    )
  }, [editor, debounceMs])

  useEffect(() => {
    const rootElement = editor.getRootElement()
    if (!rootElement) return

    const handleBlur = () => {
      if (timer.current) {
        clearTimeout(timer.current)
        timer.current = null
      }
      void doExport()
    }

    rootElement.addEventListener('blur', handleBlur, true)
    return () => {
      rootElement.removeEventListener('blur', handleBlur, true)
      if (timer.current) {
        clearTimeout(timer.current)
      }
    }
  }, [editor])

  return null
}

export default SavePlugin
