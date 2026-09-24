import type { BundledLanguage, Highlighter } from 'shiki'

// codeTheme is the Shiki theme every code surface renders with.
export const codeTheme = 'vesper'

let highlighter: Promise<Highlighter> | undefined

/**
 * getHighlighter returns the shared Shiki highlighter. It starts with the code
 * theme and no languages; highlightCode loads each language on first use.
 */
function getHighlighter(): Promise<Highlighter> {
  highlighter ??= import('shiki').then((shiki) =>
    shiki.createHighlighter({ themes: [codeTheme], langs: [] }),
  )
  return highlighter
}

/**
 * highlightCode renders code as Shiki HTML in the code theme. It resolves to
 * null when Shiki has no grammar for lang, so callers render plain text.
 */
export async function highlightCode(
  code: string,
  lang: string,
): Promise<string | null> {
  const shiki = await getHighlighter()

  // Load the grammar on first use; an unknown language renders as plain text.
  if (!shiki.getLoadedLanguages().includes(lang)) {
    try {
      await shiki.loadLanguage(lang as BundledLanguage)
    } catch {
      return null
    }
  }

  return shiki.codeToHtml(code, { lang, theme: codeTheme })
}
