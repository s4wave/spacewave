/* eslint-disable react-doctor/no-giant-component */
import { useCallback, useMemo, useState, type ReactNode } from 'react'
import Markdown from 'markdown-to-jsx'
import {
  LuBookOpen,
  LuFile,
  LuMenu,
  LuPenLine,
  LuPlus,
  LuSearch,
  LuX,
} from '@s4wave/web/images/icons.js'

import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import '@s4wave/app/docs/docs-prose.css'
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetTitle,
  SheetTrigger,
} from '@s4wave/web/ui/sheet.js'
import { Documentation } from '@s4wave/sdk/docs/docs.pb.js'
import { MknodType } from '@s4wave/sdk/unixfs/index.js'
import { keyToIRI, iriToKey } from '@s4wave/sdk/world/graph-utils.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { useForgeBlockData } from '@s4wave/web/forge/useForgeBlockData.js'
import {
  useUnixFSRootHandle,
  useUnixFSHandle,
  useUnixFSHandleEntries,
  useUnixFSHandleTextContent,
} from '@s4wave/web/hooks/useUnixFSHandle.js'
import { useStateAtom, useStateNamespace } from '@s4wave/web/state/index.js'

import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'

import { docsMarkdownOverrides } from './markdown-overrides.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import { colors as themeColors } from '../../web/style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  fonts as themeFonts,
  radii as themeRadii,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    backgroundColor: themeColors['--color-background-primary'],
    display: 'flex',
    flexDirection: 'column',
    height: '100%',
    width: '100%',
  },
  rule2: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    height: `calc(${themeLayout['--spacing']} * 9)`,
  },
  rule3: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    WebkitUserSelect: 'none',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    color: themeColors['--color-foreground'],
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    userSelect: 'none',
  },
  rule4: {
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
  },
  rule5: {
    borderColor: themeColors['--color-border'],
    gap: themeLayout['--spacing'],
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
  rule6: {
    borderRadius: '.25rem',
    flex: 1,
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    backgroundColor: themeColors['--color-muted'],
    display: 'flex',
  },
  rule7: {
    '--tw-border-style': 'none',
    '--tw-outline-style': 'none',
    borderStyle: 'none',
    backgroundColor: '#0000',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    outlineStyle: 'none',
    width: '100%',
    '::placeholder': { color: themeColors['--color-muted-foreground'] },
  },
  rule8: {
    padding: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: '.25rem',
    alignItems: 'center',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-list-hover-background'],
      },
    },
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    display: 'flex',
    justifyContent: 'center',
  },
  rule9: { flex: 1, overflowY: 'auto' },
  rule10: {
    padding: `calc(${themeLayout['--spacing']} * 6)`,
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    color: themeColors['--color-muted-foreground'],
    display: 'flex',
    flexDirection: 'column',
    justifyContent: 'center',
    textAlign: 'center',
  },
  rule11: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule12: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderRadius: themeRadii['--radius-md'],
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    backgroundColor: themeColors['--color-brand'],
    color: themeColors['--color-brand-foreground'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    opacity: {
      default: null,
      '@media (hover: hover)': { default: null, ':hover': 0.9 },
    },
  },
  rule13: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: {
      default: themeColors['--color-list-active-selection-background'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-list-hover-background'],
      },
    },
    color: themeColors['--color-list-active-selection-foreground'],
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'left',
    width: '100%',
  },
  rule14: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-list-hover-background'],
      },
    },
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'left',
    width: '100%',
  },
  rule15: {
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  rule16: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: { default: 'flex', '@container (min-width: 32rem)': 'none' },
    flexShrink: 0,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    height: `calc(${themeLayout['--spacing']} * 9)`,
  },
  rule17: {
    padding: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: '.25rem',
    alignItems: 'center',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-list-hover-background'],
      },
    },
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    display: 'flex',
    justifyContent: 'center',
    marginLeft: `calc(${themeLayout['--spacing']} * -1)`,
  },
  rule18: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-tight)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  rule19: { flex: 1, display: 'flex', flexDirection: 'column', minHeight: 0 },
  rule20: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'space-between',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
  rule21: {
    padding: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: themeRadii['--radius-md'],
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule22: {
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    flex: 1,
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'center',
  },
  rule23: {
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    flex: 1,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    color: themeColors['--color-destructive'],
    display: 'flex',
    flexDirection: 'column',
    fontSize: themeFonts['--text-xs'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule24: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule25: { flex: 1, overflow: 'auto' },
  rule26: {
    '--tw-border-style': 'none',
    '--tw-outline-style': 'none',
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    borderStyle: 'none',
    backgroundColor: themeColors['--color-background-primary'],
    color: themeColors['--color-editor-foreground'],
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    outlineStyle: 'none',
    resize: 'none',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    height: '100%',
    width: '100%',
  },
  rule27: {
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    flex: 1,
    overflow: 'auto',
  },
  rule28: {
    flex: 1,
    alignItems: 'center',
    color: themeColors['--color-muted-foreground'],
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule29: {
    borderColor: themeColors['--color-border'],
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'space-between',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
  rule30: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule31: {
    gap: themeLayout['--spacing'],
    alignItems: 'center',
    display: 'flex',
  },
  rule32: {
    borderRadius: '.25rem',
    gap: themeLayout['--spacing'],
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-list-hover-background'],
      },
    },
    color: themeColors['--color-foreground-alt'],
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule33: {
    borderRadius: '.25rem',
    gap: themeLayout['--spacing'],
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-list-hover-background'],
      },
    },
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule34: { color: themeColors['--color-brand'] },
  rule35: { color: themeColors['--color-foreground-alt'] },
  rule36: {
    padding: `calc(${themeLayout['--spacing']} * 6)`,
    flex: 1,
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'center',
  },
  rule37: { maxWidth: 'var(--container-sm)', width: '100%' },
  rule38: {
    overflow: 'hidden',
    backgroundColor: themeColors['--color-background-primary'],
    containerType: 'inline-size',
    display: 'flex',
    flexDirection: 'column',
    position: 'relative',
    height: '100%',
    width: '100%',
  },
  rule39: { flex: 1, overflow: 'hidden', display: 'flex', minHeight: 0 },
  rule40: {
    borderColor: themeColors['--color-border'],
    display: { default: 'none', '@container (min-width: 32rem)': 'flex' },
    flexDirection: 'column',
    flexShrink: 0,
    borderRightStyle: themeUtilityState['--tw-border-style'],
    borderRightWidth: '1px',
    width: `calc(${themeLayout['--spacing']} * 55)`,
  },
  rule41: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    height: `calc(${themeLayout['--spacing']} * 9)`,
  },
  rule42: { flex: 1, display: 'flex', flexDirection: 'column', minWidth: 0 },
})

const stylexStyles2 = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    color: themeColors['--color-muted-foreground'],
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule3: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule4: {
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule5: {
    color: themeColors['--color-foreground'],
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule6: {
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule7: {
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    maxWidth: {
      default: themeLayout['--max-width-content'],
      '@media (min-width: 40rem)': 'var(--container-sm)',
    },
    width: `calc(${themeLayout['--spacing']} * 60)`,
  },
})

export const DocumentationTypeID = 'spacewave-docs/documentation'

// DOC_SOURCE_PREDICATE is the graph predicate linking documentation to its UnixFS source.
const DOC_SOURCE_PREDICATE = '<doc/source>'

type DocHandle = NonNullable<ReturnType<typeof useUnixFSRootHandle>['value']>
type DocEntry = NonNullable<
  ReturnType<typeof useUnixFSHandleEntries>['value']
>[number]
type DocTextResource = ReturnType<typeof useUnixFSHandleTextContent>

/**
 * useDocSource resolves the doc/source graph edge to the linked UnixFS object
 * key. sourceKey is empty while resolving or when nothing is linked.
 */
function useDocSource(
  worldState: ObjectViewerComponentProps['worldState'],
  objectKey: string,
) {
  const linkedSource = useResource(
    worldState,
    async (world: IWorldState, signal: AbortSignal) => {
      // Resolve the documentation source edge from the current World.
      if (!world) return null
      const iri = keyToIRI(objectKey)
      const result = await world.lookupGraphQuads(
        iri,
        DOC_SOURCE_PREDICATE,
        undefined,
        undefined,
        1,
        signal,
      )
      const quads = result.quads ?? []
      if (quads.length === 0 || !quads[0].obj) return null
      return iriToKey(quads[0].obj)
    },
    [objectKey],
  )

  return { sourceKey: linkedSource.value ?? '', loading: linkedSource.loading }
}

/** createUntitledPage creates the next free untitled page and returns its name. */
async function createUntitledPage(
  handle: DocHandle,
  mdEntries: DocEntry[],
): Promise<string> {
  // Select the first untitled file name absent from the current directory.
  const existing = new Set(mdEntries.map((e) => e.name))
  let name = 'untitled.md'
  let counter = 1
  while (existing.has(name)) {
    name = `untitled-${counter}.md`
    counter++
  }

  // Create the file and prepare its initial Markdown title.
  await handle.mknod([name], MknodType.FILE)
  const child = await handle.lookup(name)
  const title = name.replace(/\.md$/, '')
  const template = `# ${title}\n`
  const encoded = new TextEncoder().encode(template)

  // Write and sync the new page before releasing its handle.
  await child.writeAt(0n, encoded)
  await child.sync()
  child.release()
  return name
}

/**
 * useDocPage tracks the selected page and its edit session. The selected page
 * and editing mode persist in the viewer's state namespace.
 */
function useDocPage(
  ns: ReturnType<typeof useStateNamespace>,
  rootHandle: ReturnType<typeof useUnixFSRootHandle>,
) {
  // Read the saved page and edit mode with the current text resource.
  const [selectedPage, setSelectedPage] = useStateAtom<string>(
    ns,
    'selectedPage',
    '',
  )
  const [editing, setEditing] = useStateAtom<boolean>(ns, 'editing', false)
  const [editContent, setEditContent] = useState<string | null>(null)
  const fileHandle = useUnixFSHandle(rootHandle, selectedPage)
  const textResource = useUnixFSHandleTextContent(fileHandle)

  // Selecting another page discards the local edit draft.
  const selectPage = useCallback(
    (name: string) => {
      setSelectedPage(name)
      setEditing(false)
      setEditContent(null)
    },
    [setSelectedPage, setEditing],
  )

  // Enter editing from the resource text, or save the draft when leaving it.
  const toggleEdit = useCallback(() => {
    if (editing) {
      // Save on exit from edit mode.
      if (editContent !== null) {
        const handle = fileHandle.value
        if (handle) {
          const encoded = new TextEncoder().encode(editContent)
          void handle
            .writeAt(0n, encoded)
            .then(() => handle.truncate(BigInt(encoded.byteLength)))
        }
        setEditContent(null)
      }
    } else {
      setEditContent(textResource.value ?? '')
    }
    setEditing((prev) => !prev)
  }, [editing, editContent, fileHandle.value, textResource.value, setEditing])

  // Cancelling discards the draft and returns to the reader.
  const cancelEdit = useCallback(() => {
    setEditContent(null)
    setEditing(false)
  }, [setEditing])

  return {
    selectedPage,
    editing,
    editContent,
    setEditContent,
    textResource,
    selectPage,
    toggleEdit,
    cancelEdit,
  }
}

/** DocPlaceholder shows the documentation title bar over a single centered body. */
function DocPlaceholder({
  title,
  children,
}: {
  title: string
  children: ReactNode
}) {
  return (
    <div sx={stylexStyles3.rule1}>
      <div sx={stylexStyles3.rule2}>
        <div sx={stylexStyles3.rule3}>
          <LuBookOpen sx={stylexStyles2.rule1} />
          <span sx={stylexStyles3.rule4}>{title}</span>
        </div>
      </div>
      {children}
    </div>
  )
}

interface DocSidebarBodyProps {
  searchQuery: string
  onSearchChange: (query: string) => void
  mdEntries: DocEntry[]
  selectedPage: string
  onSelectPage: (name: string) => void
  onCreatePage: () => void
}

/** DocSidebarBody is the search, create, and page listing shared by the rail and the drawer. */
function DocSidebarBody({
  searchQuery,
  onSearchChange,
  mdEntries,
  selectedPage,
  onSelectPage,
  onCreatePage,
}: DocSidebarBodyProps) {
  const lower = searchQuery.toLowerCase()
  const filteredEntries = searchQuery
    ? mdEntries.filter((entry) => entry.name.toLowerCase().includes(lower))
    : mdEntries

  return (
    <>
      {/* Search and create */}
      <div sx={stylexStyles3.rule5}>
        <div sx={stylexStyles3.rule6}>
          <LuSearch sx={stylexStyles2.rule2} />
          <input
            aria-label="Search pages"
            type="text"
            placeholder="Search pages…"
            value={searchQuery}
            onChange={(e) => onSearchChange(e.target.value)}
            sx={stylexStyles3.rule7}
          />
        </div>
        <button
          type="button"
          sx={stylexStyles3.rule8}
          onClick={onCreatePage}
          title="New page"
        >
          <LuPlus sx={stylexStyles2.rule3} />
        </button>
      </div>

      {/* File listing */}
      <div sx={stylexStyles3.rule9}>
        {filteredEntries.length === 0 ? (
          <div sx={stylexStyles3.rule10}>
            {mdEntries.length === 0 ? (
              <>
                <span sx={stylexStyles3.rule11}>No pages yet</span>
                <button
                  type="button"
                  sx={stylexStyles3.rule12}
                  onClick={onCreatePage}
                >
                  Create first page
                </button>
              </>
            ) : (
              <span sx={stylexStyles3.rule11}>No matching pages</span>
            )}
          </div>
        ) : (
          filteredEntries.map((entry) => (
            <button
              key={entry.name}
              type="button"
              sx={
                selectedPage === entry.name
                  ? stylexStyles3.rule13
                  : stylexStyles3.rule14
              }
              onClick={() => onSelectPage(entry.name)}
            >
              <LuFile sx={stylexStyles2.rule4} />
              <span sx={stylexStyles3.rule15}>
                {entry.name.replace(/\.md$/, '')}
              </span>
            </button>
          ))
        )}
      </div>
    </>
  )
}

interface DocNavDrawerProps {
  title: string
  open: boolean
  onOpenChange: (open: boolean) => void
  portalContainer: HTMLDivElement | null
  children: ReactNode
}

/** DocNavDrawer is the narrow-width page drawer, scoped to the viewer's pane. */
function DocNavDrawer({
  title,
  open,
  onOpenChange,
  portalContainer,
  children,
}: DocNavDrawerProps) {
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <div sx={stylexStyles3.rule16}>
        <SheetTrigger asChild>
          <button
            type="button"
            aria-label="Open documentation pages"
            sx={stylexStyles3.rule17}
          >
            <LuMenu sx={stylexStyles2.rule1} />
          </button>
        </SheetTrigger>
        <LuBookOpen sx={stylexStyles2.rule5} />
        <span sx={stylexStyles3.rule18}>{title}</span>
      </div>
      <SheetContent
        side="left"
        position="absolute"
        portalContainer={portalContainer}
        showCloseButton={false}
        variant="docs"
        sx={stylexStyles.rule1}
      >
        <div sx={stylexStyles3.rule19}>
          <div sx={stylexStyles3.rule20}>
            <SheetTitle variant="docs">
              <LuBookOpen sx={stylexStyles2.rule6} />
              {title}
            </SheetTitle>
            <SheetClose asChild>
              <button
                type="button"
                aria-label="Close documentation pages"
                sx={stylexStyles3.rule21}
              >
                <LuX sx={stylexStyles2.rule1} />
              </button>
            </SheetClose>
          </div>
          {children}
        </div>
      </SheetContent>
    </Sheet>
  )
}

interface DocPageBodyProps {
  textResource: DocTextResource
  editing: boolean
  editContent: string | null
  onEditContentChange: (content: string) => void
}

/** DocPageBody renders the page as loading, an error, the editor, or markdown. */
function DocPageBody({
  textResource,
  editing,
  editContent,
  onEditContentChange,
}: DocPageBodyProps) {
  if (textResource.loading) {
    return (
      <div sx={stylexStyles3.rule22}>
        <LoadingInline label="Loading page" tone="muted" size="sm" />
      </div>
    )
  }
  if (textResource.error) {
    return (
      <div sx={stylexStyles3.rule23}>
        <span>Failed to load page</span>
        <span sx={stylexStyles3.rule24}>{textResource.error.message}</span>
      </div>
    )
  }
  if (editing) {
    return (
      <div sx={stylexStyles3.rule25}>
        <textarea
          aria-label="Document content"
          sx={stylexStyles3.rule26}
          value={editContent ?? textResource.value ?? ''}
          onChange={(e) => onEditContentChange(e.target.value)}
        />
      </div>
    )
  }
  return (
    <div sx={stylexStyles3.rule27}>
      <div className={'docs-prose'}>
        <Markdown options={docsMarkdownOverrides}>
          {textResource.value ?? ''}
        </Markdown>
      </div>
    </div>
  )
}

interface DocContentProps extends DocPageBodyProps {
  selectedPage: string
  onToggleEdit: () => void
  onCancelEdit: () => void
}

/** DocContent shows the selected page with its edit controls, or a prompt to pick one. */
function DocContent({
  selectedPage,
  onToggleEdit,
  onCancelEdit,
  ...body
}: DocContentProps) {
  const { editing } = body
  if (!selectedPage) {
    return <div sx={stylexStyles3.rule28}>Select a page to view</div>
  }

  return (
    <>
      {/* Content header */}
      <div sx={stylexStyles3.rule29}>
        <span sx={stylexStyles3.rule30}>
          {selectedPage.replace(/\.md$/, '')}
        </span>
        <div sx={stylexStyles3.rule31}>
          {editing && (
            <button
              type="button"
              sx={stylexStyles3.rule32}
              onClick={onCancelEdit}
              title="Cancel editing"
            >
              <LuX sx={stylexStyles2.rule7} />
              Cancel
            </button>
          )}
          <button
            type="button"
            sx={[
              stylexStyles3.rule33,
              editing ? stylexStyles3.rule34 : stylexStyles3.rule35,
            ]}
            onClick={onToggleEdit}
            title={editing ? 'Save and preview' : 'Edit page'}
          >
            <LuPenLine sx={stylexStyles2.rule7} />
            {editing ? 'Save' : 'Edit'}
          </button>
        </div>
      </div>

      <DocPageBody {...body} />
    </>
  )
}

// DocumentationViewer displays a Documentation world object with a file sidebar
// and markdown content viewer.
export function DocumentationViewer({
  objectInfo,
  worldState,
  objectState,
}: ObjectViewerComponentProps) {
  // Resolve this documentation object and its linked source.
  const objectKey = getObjectKey(objectInfo)
  const ns = useStateNamespace(['docs', objectKey])
  const doc = useForgeBlockData(objectState, DocumentationTypeID, Documentation)
  const { sourceKey, loading: sourceLoading } = useDocSource(
    worldState,
    objectKey,
  )

  // Access the UnixFS root and list directory entries.
  const rootHandle = useUnixFSRootHandle(worldState, sourceKey)
  const entries = useUnixFSHandleEntries(rootHandle, {
    enabled: !!sourceKey,
  })

  // Filter to .md files only.
  const mdEntries = useMemo(() => {
    if (!entries.value) return []
    return entries.value.filter(
      (entry) => !entry.isDir && entry.name.endsWith('.md'),
    )
  }, [entries.value])

  // Bind page selection and its local search draft to the current root.
  const page = useDocPage(ns, rootHandle)
  const { selectPage } = page
  const [searchQuery, setSearchQuery] = useState('')

  // Narrow-width navigation drawer state. portalContainer scopes the drawer
  // overlay to this viewer so it stays inside a resizable pane.
  const [navOpen, setNavOpen] = useState(false)
  const [portalContainer, setPortalContainer] = useState<HTMLDivElement | null>(
    null,
  )

  // Selecting or creating a page closes the drawer; on desktop the drawer is
  // already closed so the call is a no-op.
  const handleSelectPage = useCallback(
    (name: string) => {
      setNavOpen(false)
      selectPage(name)
    },
    [selectPage],
  )

  // Create the next untitled page from the current directory, then select it.
  const handleCreatePage = useCallback(async () => {
    setNavOpen(false)
    const handle = rootHandle.value
    if (!handle) return
    selectPage(await createUntitledPage(handle, mdEntries))
  }, [rootHandle.value, mdEntries, selectPage])

  // Use the document's stored name throughout the viewer chrome.
  const title = doc?.name || 'Documentation'

  // Gate the viewer on the doc source resource: while the linked UnixFS source
  // is resolving (or the initial directory listing has not arrived yet), show a
  // single LoadingCard in the content chrome instead of flashing partial UI.
  if (sourceLoading || (!!sourceKey && entries.loading && !entries.value)) {
    return (
      <DocPlaceholder title={title}>
        <div sx={stylexStyles3.rule36}>
          <div sx={stylexStyles3.rule37}>
            <LoadingCard
              view={{
                state: 'active',
                title: 'Loading documentation',
                detail: 'Resolving the source and reading pages.',
              }}
            />
          </div>
        </div>
      </DocPlaceholder>
    )
  }

  // An unlinked document keeps its chrome and explains the missing source.
  if (!sourceKey) {
    return (
      <DocPlaceholder title={title}>
        <div sx={stylexStyles3.rule28}>No documentation source linked</div>
      </DocPlaceholder>
    )
  }

  // Share the same navigation body between the desktop sidebar and narrow drawer.
  const sidebarBody = (
    <DocSidebarBody
      searchQuery={searchQuery}
      onSearchChange={setSearchQuery}
      mdEntries={mdEntries}
      selectedPage={page.selectedPage}
      onSelectPage={handleSelectPage}
      onCreatePage={() => void handleCreatePage()}
    />
  )

  return (
    <div ref={setPortalContainer} sx={stylexStyles3.rule38}>
      <DocNavDrawer
        title={title}
        open={navOpen}
        onOpenChange={setNavOpen}
        portalContainer={portalContainer}
      >
        {sidebarBody}
      </DocNavDrawer>

      <div sx={stylexStyles3.rule39}>
        {/* Sidebar - wide widths only */}
        <div sx={stylexStyles3.rule40}>
          {/* Header */}
          <div sx={stylexStyles3.rule41}>
            <LuBookOpen sx={stylexStyles2.rule5} />
            <span sx={stylexStyles3.rule18}>{title}</span>
          </div>
          {sidebarBody}
        </div>

        {/* Content area */}
        <div sx={stylexStyles3.rule42}>
          <DocContent
            selectedPage={page.selectedPage}
            editing={page.editing}
            editContent={page.editContent}
            onEditContentChange={page.setEditContent}
            textResource={page.textResource}
            onToggleEdit={page.toggleEdit}
            onCancelEdit={page.cancelEdit}
          />
        </div>
      </div>
    </div>
  )
}
