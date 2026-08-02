import { PreBlock } from './CodeBlock.js'
import { MarkdownLink } from './MarkdownLink.js'

// DocsTable wraps rendered markdown tables in a horizontal scroll container so a
// wide table scrolls inside the content column instead of forcing the whole
// page to scroll sideways on narrow displays.
function DocsTable(props: React.TableHTMLAttributes<HTMLTableElement>) {
  return (
    <div className="docs-table-scroll">
      <table {...props} />
    </div>
  )
}

// docsMarkdownOverrides is the shared markdown-to-jsx configuration for every
// docs surface: internal-link handling, syntax-highlighted code blocks, and
// overflow-safe tables. Both the docs site and the in-app documentation viewer
// render markdown through this one configuration so their prose behaves
// identically.
export const docsMarkdownOverrides = {
  overrides: {
    a: { component: MarkdownLink },
    pre: { component: PreBlock },
    table: { component: DocsTable },
  },
}
