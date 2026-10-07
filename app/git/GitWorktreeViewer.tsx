import { useAppEnvironment } from '@s4wave/web/sdk/app/environment.js'
import {
  useCallback,
  useDeferredValue,
  useMemo,
  useState,
  type ComponentProps,
  type ReactNode,
} from 'react'

import {
  useResource,
  type Resource,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { GitWorktreeHandle } from '@s4wave/sdk/git/worktree.js'
import type { FSHandle } from '@s4wave/sdk/unixfs/handle.js'
import {
  FileStatusCode,
  type StatusEntry,
} from '@s4wave/sdk/git/worktree.pb.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { useSessionIndex } from '@s4wave/web/contexts/contexts.js'
import type { FileEntry } from '@s4wave/web/editors/file-browser/types.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { useHistory } from '@s4wave/web/router/HistoryRouter.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { cn } from '@s4wave/web/style/utils.js'
import { PanelSizeGate } from '@s4wave/web/ui/PanelSizeGate.js'

import { buildProjectedFileInlineURL } from '@s4wave/app/space/projected-url.js'

import { ChangesView } from './changes/ChangesView.js'
import {
  statusCodeToColor,
  statusCodeToLetter,
} from './changes/StatusSection.js'
import { CommitDetail } from './commits/CommitDetail.js'
import { CommitLog } from './commits/CommitLog.js'
import { FileTree } from './files/FileTree.js'
import { FileViewer } from './files/FileViewer.js'
import { GitViewerCenteredState, GitViewerFrame } from './GitViewerShell.js'
import { getGitWorktreeInlinePreviewObjectKey } from './inline-preview.js'
import { ReadmeSection } from './layout/ReadmeSection.js'
import type { ViewMode } from './layout/route.js'
import { useGitBrowsingState } from './useGitBrowsingState.js'
import { useGitFileEntries } from './useGitFileEntries.js'
import { useGitNavigation } from './useGitNavigation.js'

type GitFileEntries = ReturnType<typeof useGitFileEntries>

/**
 * BrowseSource is one browsable tree of the worktree: the committed files or
 * the working directory.
 */
interface BrowseSource {
  handle: Resource<FSHandle>
  files: GitFileEntries
  path: string
  inlineFileURL: string | undefined
}

interface InlineFileURLParams {
  enabled: boolean
  files: GitFileEntries
  path: string
  httpPathPrefix: string
  sessionIndex: number | null | undefined
  sharedObjectId: string | undefined
  objectKey: string | undefined
}

/** inlineFileURLFor builds the inline preview URL of the file at the path. */
function inlineFileURLFor({
  enabled,
  files,
  path,
  httpPathPrefix,
  sessionIndex,
  sharedObjectId,
  objectKey,
}: InlineFileURLParams): string | undefined {
  if (
    !enabled ||
    files.isDir !== false ||
    !files.statResource.value ||
    !sessionIndex ||
    !sharedObjectId ||
    !objectKey
  ) {
    return undefined
  }
  return buildProjectedFileInlineURL({
    httpPathPrefix,
    sessionIndex,
    sharedObjectId,
    objectKey,
    path,
  })
}

/** statusCodeFor picks the worktree status code, else the staging one. */
function statusCodeFor(status: StatusEntry): FileStatusCode | null {
  if (
    status.worktreeStatus !== undefined &&
    status.worktreeStatus !== FileStatusCode.UNMODIFIED
  ) {
    return status.worktreeStatus
  }
  if (
    status.stagingStatus !== undefined &&
    status.stagingStatus !== FileStatusCode.UNMODIFIED
  ) {
    return status.stagingStatus
  }
  return null
}

/** worktreeViewModes lists the view modes the worktree can show. */
function worktreeViewModes(
  hasWorkdir: boolean,
  effectiveRef: string | null,
): ViewMode[] {
  const modes: ViewMode[] = ['files']
  if (hasWorkdir) modes.push('workdir', 'changes')
  if (effectiveRef) modes.push('readme', 'log')
  return modes
}

/** useWorktreeResources mounts the worktree, its repo, and its workdir. */
function useWorktreeResources(
  worldState: ObjectViewerComponentProps['worldState'],
  objectKey: string,
) {
  const worktreeResource = useAccessTypedHandle(
    worldState,
    objectKey,
    GitWorktreeHandle,
  )
  const infoResource = useResource(
    worktreeResource,
    async (wt) => {
      if (!wt) return null
      return wt.getWorktreeInfo()
    },
    [],
  )
  const repoHandleResource = useResource(
    worktreeResource,
    async (wt, signal, cleanup) => {
      if (!wt) return null
      return cleanup(await wt.getRepoHandle(signal))
    },
    [],
  )
  const repoInfoResource = useResource(
    repoHandleResource,
    async (repo) => {
      if (!repo) return null
      return repo.getRepoInfo()
    },
    [],
  )
  const refsResource = useResource(
    repoHandleResource,
    async (repo) => {
      if (!repo) return null
      return repo.listRefs()
    },
    [],
  )
  const hasWorkdir = infoResource.value?.hasWorkdir ?? false
  const workdirHandleResource = useResource(
    worktreeResource,
    async (wt, signal, cleanup) => {
      if (!wt || !hasWorkdir) return null
      return cleanup(await wt.getWorkdirHandle(signal))
    },
    [hasWorkdir],
  )
  const statusState = useStreamingResource(
    worktreeResource,
    useCallback((wt, signal) => wt.watchStatus(signal), []),
    [],
  )
  const statusEntries = useMemo(
    () => statusState.value?.entries ?? [],
    [statusState.value?.entries],
  )

  return {
    worktreeResource,
    infoResource,
    repoHandleResource,
    repoInfoResource,
    refsResource,
    workdirHandleResource,
    hasWorkdir,
    statusEntries,
  }
}

/** useStatusEntryRenderer decorates file entries with their change letter. */
function useStatusEntryRenderer(statusEntries: StatusEntry[]) {
  const statusMap = useMemo(() => {
    const map = new Map<string, StatusEntry>()
    for (const entry of statusEntries) {
      if (entry.filePath) {
        map.set(entry.filePath, entry)
      }
    }
    return map
  }, [statusEntries])

  return useCallback(
    (props: { entry: FileEntry; defaultNode: ReactNode; path: string }) => {
      const path =
        props.path === '/'
          ? props.entry.name
          : props.path.replace(/^\//, '') + '/' + props.entry.name
      const status = statusMap.get(path)
      const code = status ? statusCodeFor(status) : null
      if (!code) return props.defaultNode

      return (
        <div className="flex w-full items-center">
          <div className="min-w-0 flex-1">{props.defaultNode}</div>
          <span
            className={cn(
              'mr-2 shrink-0 font-mono text-xs font-medium',
              statusCodeToColor(code),
            )}
          >
            {statusCodeToLetter(code)}
          </span>
        </div>
      )
    },
    [statusMap],
  )
}

/** refBarPropsFor builds the selected ref bar props from the tip commit. */
function refBarPropsFor(
  tipCommitResource: ReturnType<
    typeof useGitBrowsingState
  >['tipCommitResource'],
  effectiveRef: string | null,
  navigate: ReturnType<typeof useNavigate>,
) {
  const tipCommit = tipCommitResource.value
  const tipCommitHash = tipCommit?.hash ?? null
  return {
    lastCommit: tipCommit ?? undefined,
    loading: tipCommitResource.loading,
    error: tipCommitResource.error,
    onClickCommit: tipCommitHash
      ? () => navigate({ path: '/commit/' + tipCommitHash })
      : undefined,
    onClickTree: effectiveRef
      ? () => navigate({ path: '/tree/' + effectiveRef })
      : undefined,
    onClickLog: effectiveRef
      ? () => navigate({ path: '/commits/' + effectiveRef })
      : undefined,
  }
}

// useGitWorktreeController owns worktree resources, status projection, and
// navigation callbacks independently from presentation.
function useGitWorktreeController({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  const sessionIndex = useSessionIndex()
  const { httpPathPrefix } = useAppEnvironment()
  const spaceCtx = SpaceContainerContext.useContextSafe()
  const navigate = useNavigate()
  const history = useHistory()
  const {
    worktreeResource,
    infoResource,
    repoHandleResource,
    repoInfoResource,
    refsResource,
    workdirHandleResource,
    hasWorkdir,
    statusEntries,
  } = useWorktreeResources(worldState, objectKey)

  const browsing = useGitBrowsingState(
    repoHandleResource,
    infoResource.value?.checkedOutRef,
    'git-worktree',
  )
  const { route, effectiveRef, tipCommitResource, rootHandleResource } =
    browsing
  const isWorkdirMode = route.mode === 'workdir'
  const displayPath = route.subpath
  const readmePath = repoInfoResource.value?.readmePath
  const files = useGitFileEntries(
    rootHandleResource,
    displayPath,
    readmePath,
    route.mode === 'files',
  )
  const workdirPath = isWorkdirMode ? route.subpath : '/'
  const workdirFiles = useGitFileEntries(
    workdirHandleResource,
    workdirPath,
    undefined,
    isWorkdirMode,
  )
  const renderEntry = useStatusEntryRenderer(statusEntries)

  const [pendingName, setPendingName] = useState<string | null>(null)
  const staleFileEntries = useDeferredValue(files.fileEntries)
  const staleWorkdirEntries = useDeferredValue(workdirFiles.fileEntries)
  const staleEntries = isWorkdirMode ? staleWorkdirEntries : staleFileEntries

  const inlinePreviewObjectKey = useMemo(
    () =>
      getGitWorktreeInlinePreviewObjectKey({
        mode: isWorkdirMode ? 'workdir' : 'files',
        repoObjectKey: infoResource.value?.repoObjectKey,
        workdirObjectKey: infoResource.value?.workdirObjectKey,
      }),
    [
      infoResource.value?.repoObjectKey,
      infoResource.value?.workdirObjectKey,
      isWorkdirMode,
    ],
  )
  const inlineBase = {
    httpPathPrefix,
    sessionIndex,
    sharedObjectId: spaceCtx?.spaceId,
    objectKey: inlinePreviewObjectKey,
  }
  const filesSource: BrowseSource = {
    handle: rootHandleResource,
    files,
    path: displayPath,
    inlineFileURL: inlineFileURLFor({
      ...inlineBase,
      enabled: route.mode === 'files',
      files,
      path: displayPath,
    }),
  }
  const workdirSource: BrowseSource = {
    handle: workdirHandleResource,
    files: workdirFiles,
    path: workdirPath,
    inlineFileURL: inlineFileURLFor({
      ...inlineBase,
      enabled: isWorkdirMode,
      files: workdirFiles,
      path: workdirPath,
    }),
  }

  const nav = useGitNavigation({
    route,
    effectiveRef,
    displayPath,
    navigate,
    history,
    workdirPath,
    onPendingName: setPendingName,
  })
  const availableModes = useMemo(
    () => worktreeViewModes(hasWorkdir, effectiveRef),
    [effectiveRef, hasWorkdir],
  )
  const handleChangesFileClick = useCallback(
    (filePath: string) => {
      navigate({ path: '/workdir/' + filePath })
    },
    [navigate],
  )
  const toolbarProps = {
    effectiveRef,
    refsResponse: refsResource.value,
    refsLoading: refsResource.loading,
    onRefSelect: nav.handleRefSelect,
    currentPath: isWorkdirMode ? workdirPath : displayPath,
    onPathChange: nav.handlePathChange,
    onBack: nav.handleBack,
    onForward: nav.handleForward,
    onUp: nav.handleUp,
    canGoBack: history?.canGoBack ?? false,
    canGoForward: history?.canGoForward ?? false,
    showPath: route.mode === 'files' || isWorkdirMode,
  }
  const refBarProps = refBarPropsFor(tipCommitResource, effectiveRef, navigate)

  return {
    availableModes,
    displayPath,
    effectiveRef,
    files,
    filesSource,
    handleChangesFileClick,
    nav,
    navigate,
    objectKey,
    pendingName,
    readmePath,
    refBarProps,
    renderEntry,
    repoHandleResource,
    route,
    staleEntries,
    statusEntries,
    toolbarProps,
    workdirFiles,
    workdirPath,
    workdirSource,
    worktreeResource,
  }
}

type GitWorktreeController = ReturnType<typeof useGitWorktreeController>

/** browseSourceFor returns the tree the route browses, or null for other routes. */
function browseSourceFor(c: GitWorktreeController): BrowseSource | null {
  if (c.route.mode === 'files') return c.filesSource
  if (c.route.mode === 'workdir') return c.workdirSource
  return null
}

/** sourceResources lists the resources a source depends on, in error priority. */
function sourceResources(source: BrowseSource): Resource<unknown>[] {
  const { files } = source
  return [
    source.handle,
    files.pathHandle,
    files.statResource,
    files.entriesResource,
  ]
}

/** isSourceLoading reports whether the source tree is still loading. */
function isSourceLoading(source: BrowseSource): boolean {
  const { files } = source
  return (
    source.handle.loading ||
    files.pathHandle.loading ||
    files.statResource.loading ||
    (files.isDir === true && files.entriesResource.loading)
  )
}

type SourceView = 'file' | 'loading' | 'error' | 'ready'

/** sourceViewFor picks what the source shows in place of its directory tree. */
function sourceViewFor(source: BrowseSource): SourceView {
  if (source.files.isDir === false && source.files.statResource.value) {
    return 'file'
  }
  if (isSourceLoading(source)) return 'loading'
  if (sourceResources(source).some((resource) => resource.error)) return 'error'
  return 'ready'
}

type FrameProps = Omit<ComponentProps<typeof GitViewerFrame>, 'children'>

interface WorktreeSourceStatusProps {
  controller: GitWorktreeController
  source: BrowseSource
  view: SourceView
  frameProps: FrameProps
}

/** WorktreeSourceStatus renders the file, loading, or error view of a source. */
function WorktreeSourceStatus({
  controller,
  source,
  view,
  frameProps,
}: WorktreeSourceStatusProps) {
  const { route, nav, pendingName, renderEntry, staleEntries } = controller
  const { toolbarProps, refBarProps } = frameProps
  const bareFrameProps = { toolbarProps, refBarProps }
  const stat = source.files.statResource.value

  if (view === 'file' && stat) {
    return (
      <GitViewerFrame {...frameProps}>
        <FileViewer
          path={source.path}
          stat={stat}
          rootHandle={source.handle}
          inlineFileURL={source.inlineFileURL}
        />
      </GitViewerFrame>
    )
  }

  if (view === 'loading') {
    const isWorkdir = route.mode === 'workdir'
    return (
      <GitViewerFrame {...bareFrameProps}>
        {staleEntries.length > 0 ? (
          <div className="bg-file-back flex min-h-0 flex-1 flex-col overflow-hidden">
            <FileTree
              entries={staleEntries}
              onOpen={nav.handleOpen}
              loadingId={pendingName}
              renderEntry={isWorkdir ? renderEntry : undefined}
              currentPath={isWorkdir ? source.path : undefined}
            />
          </div>
        ) : (
          <div className="bg-file-back flex min-h-0 flex-1 flex-col items-center justify-center overflow-hidden">
            <div className="text-foreground-alt text-xs">Loading files…</div>
          </div>
        )}
      </GitViewerFrame>
    )
  }

  const failed = sourceResources(source).find((resource) => resource.error)
  return (
    <GitViewerFrame {...bareFrameProps}>
      <div className="bg-file-back flex min-h-0 flex-1 flex-col items-center justify-center overflow-hidden">
        <div className="text-destructive text-xs">Error loading files</div>
        <div className="text-foreground-alt/70 mt-1 text-xs">
          {failed?.error?.message}
        </div>
        <button
          type="button"
          className="text-brand mt-2 text-xs underline"
          onClick={failed?.retry}
        >
          Retry
        </button>
      </div>
    </GitViewerFrame>
  )
}

/** WorktreeBrowsePanel renders the file, workdir, or changes panel. */
function WorktreeBrowsePanel({
  controller,
}: {
  controller: GitWorktreeController
}) {
  const {
    displayPath,
    files,
    handleChangesFileClick,
    nav,
    renderEntry,
    route,
    statusEntries,
    workdirFiles,
    workdirPath,
    worktreeResource,
  } = controller
  const isRoot =
    route.mode === 'workdir' ? workdirPath === '/' : displayPath === '/'

  switch (route.mode) {
    case 'files':
      return (
        <FileTree
          entries={files.fileEntries}
          onOpen={nav.handleOpen}
          autoHeight={isRoot}
        />
      )
    case 'workdir':
      return (
        <FileTree
          entries={workdirFiles.fileEntries}
          onOpen={nav.handleOpen}
          autoHeight={isRoot}
          renderEntry={renderEntry}
          currentPath={workdirPath}
        />
      )
    default:
      return (
        worktreeResource.value && (
          <ChangesView
            entries={statusEntries}
            handle={worktreeResource.value}
            onFileClick={handleChangesFileClick}
          />
        )
      )
  }
}

/** WorktreeDetailPanel renders the README, log, or commit panel at the root. */
function WorktreeDetailPanel({
  controller,
}: {
  controller: GitWorktreeController
}) {
  const {
    displayPath,
    effectiveRef,
    files,
    navigate,
    readmePath,
    repoHandleResource,
    route,
  } = controller
  const repoHandle = repoHandleResource.value
  if (displayPath !== '/') return null

  switch (route.mode) {
    case 'readme':
      return (
        <ReadmeSection
          readmePath={readmePath ?? ''}
          content={files.readmeContent.value}
          loading={files.readmeContent.loading}
        />
      )
    case 'log':
      return (
        repoHandle &&
        effectiveRef && (
          <CommitLog
            handle={repoHandle}
            refName={effectiveRef}
            onCommitClick={(hash) => navigate({ path: '/commit/' + hash })}
          />
        )
      )
    case 'commit':
      return (
        repoHandle &&
        route.commitHash && (
          <CommitDetail
            handle={repoHandle}
            commitHash={route.commitHash}
            onNavigateCommit={(hash) => navigate({ path: '/commit/' + hash })}
          />
        )
      )
    default:
      return null
  }
}

/** WorktreeRouteBody renders the panel of the current route once loaded. */
function WorktreeRouteBody({
  controller,
}: {
  controller: GitWorktreeController
}) {
  switch (controller.route.mode) {
    case 'files':
    case 'workdir':
    case 'changes':
      return <WorktreeBrowsePanel controller={controller} />
    default:
      return <WorktreeDetailPanel controller={controller} />
  }
}

// GitWorktreeReadyContent renders file, workdir, change, README, log, and
// commit routes after the worktree handle is available.
function GitWorktreeReadyContent({
  controller,
}: {
  controller: GitWorktreeController
}) {
  const {
    availableModes,
    effectiveRef,
    nav,
    objectKey,
    readmePath,
    refBarProps,
    route,
    toolbarProps,
  } = controller
  const isCommit = route.mode === 'commit'
  const frameProps: FrameProps = {
    toolbarProps,
    refBarProps,
    mode: isCommit ? undefined : route.mode,
    onModeChange: isCommit ? undefined : nav.handleModeChange,
    hasReadme: !!readmePath,
    availableModes,
  }

  if (
    !effectiveRef &&
    (route.mode === 'files' || route.mode === 'readme' || route.mode === 'log')
  ) {
    return (
      <GitViewerFrame {...frameProps}>
        <GitViewerCenteredState
          title="Empty Repository"
          subtitle={objectKey}
          detail="This repository has no commits yet."
        />
      </GitViewerFrame>
    )
  }

  const source = browseSourceFor(controller)
  const view = source ? sourceViewFor(source) : 'ready'
  if (source && view !== 'ready') {
    return (
      <WorktreeSourceStatus
        controller={controller}
        source={source}
        view={view}
        frameProps={frameProps}
      />
    )
  }

  return (
    <GitViewerFrame {...frameProps}>
      <div className="bg-file-back min-h-0 flex-1 overflow-auto">
        <WorktreeRouteBody controller={controller} />
      </div>
    </GitViewerFrame>
  )
}

// GitWorktreeContent selects loading and failure states before rendering the
// ready worktree routes.
function GitWorktreeContent({
  controller,
}: {
  controller: GitWorktreeController
}) {
  const { objectKey, worktreeResource } = controller
  if (worktreeResource.loading) {
    return (
      <GitViewerCenteredState
        title={
          <span className="text-foreground-alt text-xs">Loading worktree…</span>
        }
      />
    )
  }

  if (worktreeResource.error) {
    return (
      <GitViewerCenteredState
        title={
          <span className="text-destructive text-xs">
            Error loading worktree
          </span>
        }
        detail={worktreeResource.error.message}
        action={
          <button
            type="button"
            className="text-brand mt-2 text-xs underline"
            onClick={worktreeResource.retry}
          >
            Retry
          </button>
        }
      />
    )
  }

  if (!worktreeResource.value) {
    return (
      <GitViewerCenteredState
        title={
          <span className="text-foreground-alt text-xs">
            Git worktree not found
          </span>
        }
        detail={`Object: ${objectKey || 'none'}`}
      />
    )
  }

  return <GitWorktreeReadyContent controller={controller} />
}

// GitWorktreeViewer renders a Git worktree object with repository metadata,
// working directory browser, and magit-style changes view.
export function GitWorktreeViewer(props: ObjectViewerComponentProps) {
  const controller = useGitWorktreeController(props)

  return (
    <PanelSizeGate
      minWidth={400}
      fallback={
        <GitViewerCenteredState
          title="Git Worktree"
          subtitle={controller.objectKey}
          detail="Make panel larger to view"
        />
      }
    >
      <GitWorktreeContent controller={controller} />
    </PanelSizeGate>
  )
}
