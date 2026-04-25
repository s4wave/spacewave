import { useCallback, useEffect, useRef } from 'react'
import { useLexicalComposerContext } from '@lexical/react/LexicalComposerContext'
import { $convertToMarkdownString } from '@lexical/markdown'
import type { Transformer } from '@lexical/markdown'

interface SavePluginProps {
  transformers: Transformer[]
  onSave: (markdown: string) => void
  debounceMs?: number
}

// SavePlugin exports markdown from Lexical state on debounce and blur.
function SavePlugin({
  transformers,
  onSave,
  debounceMs = 2000,
}: SavePluginProps) {
  const [editor] = useLexicalComposerContext()
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const lastExported = useRef<string>('')

  const doExport = useCallback(() => {
    editor.getEditorState().read(() => {
      const md = $convertToMarkdownString(transformers, undefined, true)
      if (md !== lastExported.current) {
        lastExported.current = md
        onSave(md)
      }
    })
  }, [editor, transformers, onSave])

  useEffect(() => {
    return editor.registerUpdateListener(({ editorState, prevEditorState }) => {
      if (editorState === prevEditorState) return

      if (timer.current) {
        clearTimeout(timer.current)
      }
      timer.current = setTimeout(doExport, debounceMs)
    })
  }, [editor, doExport, debounceMs])

  useEffect(() => {
    const rootElement = editor.getRootElement()
    if (!rootElement) return

    const handleBlur = () => {
      if (timer.current) {
        clearTimeout(timer.current)
        timer.current = null
      }
      doExport()
    }

    rootElement.addEventListener('blur', handleBlur, true)
    return () => {
      rootElement.removeEventListener('blur', handleBlur, true)
      if (timer.current) {
        clearTimeout(timer.current)
      }
    }
  }, [editor, doExport])

  return null
}

export default SavePlugin
