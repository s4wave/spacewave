import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { useObjectName } from '@s4wave/web/object/ObjectViewerContext.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import { ViewerStatusShell } from '@s4wave/web/object/ViewerStatusShell.js'

import {
  LuArrowLeft,
  LuChevronDown,
  LuMenu,
  LuX,
} from '@s4wave/web/images/icons.js'

import { Notebook } from './proto/notebook.pb.js'
import { NotebookHandle, NotebookTypeID } from './sdk/notebook.js'
import { useWorldObjectMessageState } from './useWorldObjectMessageState.js'
import { ConfirmActionDialog, SourceInputDialog } from './NoteDialogs.js'

import NotebookSidebar from './NotebookSidebar.js'
import NoteList from './NoteList.js'
import NoteContentView from './NoteContentView.js'
import { useNotebookViewerState } from './useNotebookViewerState.js'
import NotebookSavedViews from './NotebookSavedViews.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '@s4wave/web/style/tokens.stylex.js'

import {
  colors as themeColors,
  utilityState as themeUtilityState,
  fonts as themeFonts,
} from '@s4wave/web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    overflow: 'hidden',
    backgroundColor: themeColors['--color-background-primary'],
    display: 'flex',
    flexDirection: {
      default: 'column',
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 'row',
      },
    },
    position: 'relative',
    height: '100%',
    minHeight: 0,
    minWidth: 0,
    width: '100%',
  },
  rule2: {
    borderColor: themeColors['--color-border'],
    alignItems: 'center',
    display: {
      default: 'flex',
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 'none',
      },
    },
    flexShrink: 0,
    zIndex: 40,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    height: `calc(${themeLayout['--spacing']} * 11)`,
  },
  rule3: {
    alignItems: 'center',
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 11)`,
    width: `calc(${themeLayout['--spacing']} * 11)`,
  },
  rule4: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    overflow: 'hidden',
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    minWidth: 0,
  },
  rule5: {
    borderColor: themeColors['--color-border'],
    insetBlock: {
      default: null,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 0,
      },
    },
    backgroundColor: themeColors['--color-background-primary'],
    display: {
      default: 'block',
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 'block',
      },
    },
    position: {
      default: 'absolute',
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 'relative',
      },
    },
    zIndex: 30,
    borderRightStyle: themeUtilityState['--tw-border-style'],
    borderRightWidth: '1px',
    bottom: 0,
    left: 0,
    maxWidth: '100%',
    minWidth: {
      default: null,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': `calc(${themeLayout['--spacing']} * 50)`,
      },
    },
    overflowY: 'auto',
    top: `calc(${themeLayout['--spacing']} * 11)`,
    width: {
      default: `calc(${themeLayout['--spacing']} * 72)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': `calc(${themeLayout['--spacing']} * 50)`,
      },
    },
  },
  rule6: {
    borderColor: themeColors['--color-border'],
    display: {
      default: 'none',
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 'block',
      },
    },
    position: {
      default: null,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 'relative',
      },
    },
    borderRightStyle: themeUtilityState['--tw-border-style'],
    borderRightWidth: '1px',
    maxWidth: '100%',
    minWidth: {
      default: null,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': `calc(${themeLayout['--spacing']} * 50)`,
      },
    },
    overflowY: 'auto',
    width: {
      default: `calc(${themeLayout['--spacing']} * 72)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': `calc(${themeLayout['--spacing']} * 50)`,
      },
    },
  },
  rule7: {
    borderColor: themeColors['--color-border'],
    flex: {
      default: 1,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 'none',
      },
    },
    flexDirection: 'column',
    borderRightStyle: themeUtilityState['--tw-border-style'],
    borderRightWidth: '1px',
    minHeight: 0,
    minWidth: {
      default: 0,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': `calc(${themeLayout['--spacing']} * 62.5)`,
      },
    },
    width: {
      default: null,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': `calc(${themeLayout['--spacing']} * 62.5)`,
      },
    },
  },
  rule8: {
    display: {
      default: 'none',
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 'flex',
      },
    },
  },
  rule9: { display: 'flex' },
  rule10: {
    borderColor: themeColors['--color-border'],
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    justifyContent: 'space-between',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    minHeight: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 0,
      },
    },
  },
  rule11: { position: 'relative' },
  rule12: {
    borderRadius: '.25rem',
    paddingBlock: themeLayout['--spacing'],
    appearance: {
      default: 'none',
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 'auto',
      },
    },
    backgroundColor: themeColors['--color-background-primary'],
    height: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 'auto',
      },
    },
    paddingLeft: themeLayout['--spacing'],
    paddingRight: {
      default: `calc(${themeLayout['--spacing']} * 6)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': themeLayout['--spacing'],
      },
    },
  },
  rule13: { flex: 1, minHeight: 0 },
  rule14: { flex: 1, minHeight: 0, minWidth: 0 },
  rule15: { display: 'block' },
  rule16: {
    display: {
      default: 'none',
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 'block',
      },
    },
  },
  rule17: {
    alignItems: 'center',
    color: themeColors['--color-muted-foreground'],
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    height: '100%',
  },
  rule18: {
    inset: 0,
    backgroundColor: 'color-mix(in oklab, var(--color-black) 40%, transparent)',
    display: {
      default: null,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 'none',
      },
    },
    position: 'absolute',
    zIndex: 20,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule2: {
    '--tw-translate-y': 'calc(calc(1 / 2 * 100%) * -1)',
    display: {
      default: null,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 'none',
      },
    },
    pointerEvents: 'none',
    position: 'absolute',
    translate: 'var(--tw-translate-x) var(--tw-translate-y)',
    height: `calc(${themeLayout['--spacing']} * 3)`,
    right: themeLayout['--spacing'],
    top: '50%',
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

/** NotebookViewer presents the notebook's source, note list, and active note. */
function NotebookViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)

  const resource = useAccessTypedHandle(
    worldState,
    objectKey,
    NotebookHandle,
    NotebookTypeID,
  )
  const { state, sources } = useWorldObjectMessageState(
    worldState,
    objectKey,
    Notebook.fromBinary,
  )
  useObjectName(state.value?.name)

  const notebookHandle = resource.value
  const viewerState = useNotebookViewerState({ sources, notebookHandle })
  const {
    ns,
    selectedSource,
    selectedNote,
    currentPath,
    editing,
    filterTag,
    filterStatus,
    sort,
    setSort,
    handleLoadView,
    sidebarOpen,
    addSourceOpen,
    removeSourceIndex,
    currentSource,
    setFilterTag,
    setFilterStatus,
    setSidebarOpen,
    setAddSourceOpen,
    setRemoveSourceIndex,
    handleSelectSource,
    handleAddSource,
    handleConfirmAddSource,
    handleRemoveSource,
    handleConfirmRemoveSource,
    handleMoveSource,
    handleSelectNote,
    handleChangePath,
    handleNoteRenamed,
    handleNoteDeleted,
    handleToggleEdit,
  } = viewerState

  return (
    <ViewerStatusShell
      resource={state}
      state={state}
      loadingText="Loading notebook..."
    >
      <div sx={stylexStyles2.rule1}>
        <div sx={stylexStyles2.rule2}>
          <button
            type="button"
            aria-label={
              sidebarOpen
                ? 'Close notebook navigation'
                : 'Open notebook navigation'
            }
            aria-expanded={sidebarOpen}
            sx={stylexStyles2.rule3}
            onClick={() => setSidebarOpen(!sidebarOpen)}
          >
            {sidebarOpen ? (
              <LuX sx={stylexStyles.rule1} />
            ) : (
              <LuMenu sx={stylexStyles.rule1} />
            )}
          </button>
          {selectedNote && (
            <button
              type="button"
              aria-label="Back to notes"
              sx={stylexStyles2.rule3}
              onClick={() => handleChangePath(currentPath)}
            >
              <LuArrowLeft sx={stylexStyles.rule1} />
            </button>
          )}
          <span sx={stylexStyles2.rule4}>
            {currentSource?.name || 'Notebook'}
          </span>
        </div>

        {/* Touch navigation overlays the list so landscape retains room for the note. */}
        <div sx={sidebarOpen ? stylexStyles2.rule5 : stylexStyles2.rule6}>
          <NotebookSavedViews
            world={worldState}
            notebook={objectKey}
            handle={notebookHandle}
            draft={{
              sourceRef: currentSource?.ref ?? '',
              path: currentPath,
              filterTag: filterTag ?? null,
              filterStatus: filterStatus ?? null,
              sort,
            }}
            onLoad={handleLoadView}
          />
          <NotebookSidebar
            sources={sources}
            selectedSource={selectedSource}
            onSelectSource={handleSelectSource}
            onAddSource={handleAddSource}
            onRemoveSource={handleRemoveSource}
            onMoveSource={(index, delta) => void handleMoveSource(index, delta)}
            namespace={ns}
          />
        </div>

        {/* Touch navigation shows either the list or the selected note. */}
        <div
          sx={[
            stylexStyles2.rule7,
            selectedNote ? stylexStyles2.rule8 : stylexStyles2.rule9,
          ]}
        >
          <label sx={stylexStyles2.rule10}>
            Sort notes
            <span sx={stylexStyles2.rule11}>
              <select
                aria-label="Sort notes"
                value={sort}
                onChange={(event) => setSort(event.target.value as typeof sort)}
                sx={stylexStyles2.rule12}
              >
                <option value="name">File name</option>
                <option value="title">Title</option>
              </select>
              <LuChevronDown aria-hidden="true" sx={stylexStyles.rule2} />
            </span>
          </label>
          <div sx={stylexStyles2.rule13}>
            <NoteList
              source={currentSource}
              worldState={worldState}
              selectedNote={selectedNote}
              currentPath={currentPath}
              onSelectNote={handleSelectNote}
              onChangePath={handleChangePath}
              onNoteRenamed={handleNoteRenamed}
              onNoteDeleted={handleNoteDeleted}
              filterTag={filterTag}
              filterStatus={filterStatus}
              sort={sort}
              onFilterTagChange={setFilterTag}
              onFilterStatusChange={setFilterStatus}
            />
          </div>
        </div>

        {/* Content area */}
        <div
          sx={[
            stylexStyles2.rule14,
            selectedNote ? stylexStyles2.rule15 : stylexStyles2.rule16,
          ]}
        >
          {currentSource?.ref && selectedNote ? (
            <NoteContentView
              worldState={worldState}
              sourceRef={currentSource.ref}
              noteName={selectedNote}
              editing={editing}
              onToggleEdit={handleToggleEdit}
              onFilterTag={setFilterTag}
              onFilterStatus={setFilterStatus}
            />
          ) : (
            <div sx={stylexStyles2.rule17}>
              {sources.length === 0
                ? 'No sources configured for this notebook'
                : 'Select a note to view'}
            </div>
          )}
        </div>

        {/* Backdrop for mobile sidebar */}
        {sidebarOpen && (
          <button
            type="button"
            aria-label="Close sidebar"
            sx={stylexStyles2.rule18}
            onClick={() => setSidebarOpen(false)}
          />
        )}
        <SourceInputDialog
          open={addSourceOpen}
          onOpenChange={setAddSourceOpen}
          onConfirm={(source) => void handleConfirmAddSource(source)}
        />
        <ConfirmActionDialog
          open={removeSourceIndex !== null}
          title="Remove source"
          description="Remove this source from the notebook?"
          confirmLabel="Remove"
          onOpenChange={(open) => {
            if (!open) setRemoveSourceIndex(null)
          }}
          onConfirm={() => void handleConfirmRemoveSource()}
        />
      </div>
    </ViewerStatusShell>
  )
}

export { NotebookViewer }
export default NotebookViewer
