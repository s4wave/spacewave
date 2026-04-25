import type { EditorThemeClasses } from 'lexical'

// editorTheme maps Lexical node types to Tailwind CSS classes.
const editorTheme: EditorThemeClasses = {
  heading: {
    h1: 'text-3xl font-bold mt-6 mb-3',
    h2: 'text-2xl font-bold mt-5 mb-2',
    h3: 'text-xl font-semibold mt-4 mb-2',
    h4: 'text-lg font-semibold mt-3 mb-1',
    h5: 'text-base font-semibold mt-2 mb-1',
    h6: 'text-sm font-semibold mt-2 mb-1',
  },
  text: {
    bold: 'font-bold',
    italic: 'italic',
    strikethrough: 'line-through',
    underline: 'underline',
    code: 'bg-muted rounded px-1.5 py-0.5 font-mono text-[0.9em] text-brand',
  },
  list: {
    ul: 'list-disc pl-6 my-2',
    ol: 'list-decimal pl-6 my-2',
    listitem: 'my-1',
    listitemChecked:
      'list-none relative pl-6 line-through opacity-60 before:absolute before:left-0 before:content-["\\2611"]',
    listitemUnchecked:
      'list-none relative pl-6 before:absolute before:left-0 before:content-["\\2610"]',
    nested: {
      listitem: 'list-none',
    },
  },
  link: 'text-brand underline cursor-pointer hover:opacity-80',
  quote:
    'border-l-4 border-border pl-4 italic text-muted-foreground my-3',
  code: 'bg-muted rounded-md p-4 font-mono text-[0.9em] my-3 block overflow-x-auto',
  table: 'border-collapse w-full my-3',
  tableCell:
    'border border-border px-3 py-2 text-left align-top',
  tableCellHeader:
    'border border-border bg-muted px-3 py-2 text-left font-semibold',
  tableRow: '',
  image: 'my-3',
  horizontalRule: 'border-t border-border my-6',
}

export default editorTheme
