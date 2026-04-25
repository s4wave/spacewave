import { useEffect } from 'react'
import { useLexicalComposerContext } from '@lexical/react/LexicalComposerContext'
import {
  registerCodeHighlighting,
  ShikiTokenizer,
} from '@lexical/code-shiki'

// CodeHighlightPlugin registers shiki-based syntax highlighting for code blocks.
function CodeHighlightPlugin() {
  const [editor] = useLexicalComposerContext()
  useEffect(() => {
    const tokenizer = { ...ShikiTokenizer, defaultTheme: 'vesper' }
    return registerCodeHighlighting(editor, tokenizer)
  }, [editor])
  return null
}

export default CodeHighlightPlugin
