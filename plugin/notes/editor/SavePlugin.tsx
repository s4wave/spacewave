import { useEffect, useEffectEvent, useRef } from 'react'
import { useLexicalComposerContext } from '@lexical/react/LexicalComposerContext'

interface SavePluginProps {
  exportString: () => string
  onSave: (content: string) => void
  debounceMs?: number
}

// SavePlugin exports text from Lexical state on debounce and blur.
function SavePlugin({
  exportString,
  onSave,
  debounceMs = 2000,
}: SavePluginProps) {
  const [editor] = useLexicalComposerContext()
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const lastExported = useRef<string>('')

  const doExport = useEffectEvent(() => {
    editor.getEditorState().read(() => {
      const content = exportString()
      if (content !== lastExported.current) {
        lastExported.current = content
        onSave(content)
      }
    })
  })

  useEffect(() => {
    return editor.registerUpdateListener(({ editorState, prevEditorState }) => {
      if (editorState === prevEditorState) return

      if (timer.current) {
        clearTimeout(timer.current)
      }
      timer.current = setTimeout(doExport, debounceMs)
    })
  }, [editor, debounceMs])

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
  }, [editor])

  return null
}

export default SavePlugin
