import { useAppEnvironment } from '@s4wave/web/sdk/app/environment.js'
import { useDeferredValue, useState, type ComponentProps } from 'react'

import {
  useResource,
  type Resource,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import { GitRepoHandle } from '@s4wave/sdk/git/repo.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { useSessionIndex } from '@s4wave/web/contexts/contexts.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { useHistory } from '@s4wave/web/router/HistoryRouter.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { PanelSizeGate } from '@s4wave/web/ui/PanelSizeGate.js'

import { buildProjectedFileInlineURL } from '@s4wave/app/space/projected-url.js'

import { CommitDetail } from './commits/CommitDetail.js'
import { CommitLog } from './commits/CommitLog.js'
import { FileTree } from './files/FileTree.js'
import { FileViewer } from './files/FileViewer.js'
import { GitViewerCenteredState, GitViewerFrame } from './GitViewerShell.js'
import { ReadmeSection } from './layout/ReadmeSection.js'
import { useGitBrowsingState } from './useGitBrowsingState.js'
import { useGitFileEntries } from './useGitFileEntries.js'
import { useGitNavigation } from './useGitNavigation.js'

// GitRepoTypeID is the type identifier for git/repo objects.
export const GitRepoTypeID = 'git/repo'

// useGitRepoController owns repository resources, route projection, and
// navigation callbacks independently from the repository presentation.
function useGitRepoController({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  const sessionIndex = useSessionIndex()
  const { httpPathPrefix } = useAppEnvironment()
  const spaceCtx = SpaceContainerContext.useContextSafe()
  const navigate = useNavigate()
  const history = useHistory()
  const gitResource = useAccessTypedHandle(worldState, objectKey, GitRepoHandle)
  const repoInfoResource = useResource(
    gitResource,
    async (git) => {
      if (!git) return null
      return git.getRepoInfo()
    },
    [],
  )
  const refsResource = useResource(
    gitResource,
    async (git) => {
      if (!git) return null
      return git.listRefs()
    },
    [],
  )
  const browsing = useGitBrowsingState(
    gitResource,
    repoInfoResource.value?.headRef,
    'git',
  )
  const { route, effectiveRef, tipCommitResource, rootHandleResource } =
    browsing
  const displayPath = route.subpath
  const readmePath = repoInfoResource.value?.readmePath
  const files = useGitFileEntries(rootHandleResource, displayPath, readmePath)
  const {
    pathHandle,
    statResource,
    isDir,
    entriesResource,
    fileEntries,
    readmeContent,
  } = files
  const [pendingName, setPendingName] = useState<string | null>(null)
  const staleEntries = useDeferredValue(fileEntries)
  const inlineFileURL =
    route.mode === 'commit' || isDir !== false || !statResource.value
      ? undefined
      : !sessionIndex || !spaceCtx?.spaceId
        ? undefined
        : buildProjectedFileInlineURL({
            httpPathPrefix,
            sessionIndex,
            sharedObjectId: spaceCtx.spaceId,
            objectKey,
            path: displayPath,
          })
  const nav = useGitNavigation({
    route,
    effectiveRef,
    displayPath,
    navigate,
    history,
    onPendingName: setPendingName,
  })
  const toolbarProps = {
    effectiveRef,
    refsResponse: refsResource.value,
    refsLoading: refsResource.loading,
    onRefSelect: nav.handleRefSelect,
    currentPath: displayPath,
    onPathChange: nav.handlePathChange,
    onBack: nav.handleBack,
    onForward: nav.handleForward,
    onUp: nav.handleUp,
    canGoBack: history?.canGoBack ?? false,
    canGoForward: history?.canGoForward ?? false,
  }
  const tipCommit = tipCommitResource.value
  const tipCommitHash = tipCommit?.hash ?? null
  const refBarProps = {
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

  return {
    displayPath,
    effectiveRef,
    entriesResource,
    fileEntries,
    gitResource,
    inlineFileURL,
    isDir,
    nav,
    navigate,
    objectKey,
    pathHandle,
    pendingName,
    readmeContent,
    readmePath,
    refBarProps,
    repoInfoResource,
    rootHandleResource,
    route,
    staleEntries,
    statResource,
    toolbarProps,
  }
}

type GitRepoController = ReturnType<typeof useGitRepoController>

/** GitRepoLoadState renders the loading, error, or not-found repository state. */
function GitRepoLoadState({ controller }: { controller: GitRepoController }) {
  const { gitResource, objectKey } = controller

  if (gitResource.loading) {
    return (
      <GitViewerCenteredState
        title={
          <span className="text-foreground-alt text-xs">
            Loading repository…
          </span>
        }
      />
    )
  }

  if (gitResource.error) {
    return (
      <GitViewerCenteredState
        title={
          <span className="text-destructive text-xs">
            Error loading repository
          </span>
        }
        detail={gitResource.error.message}
        action={
          <button
            type="button"
            className="text-brand mt-2 text-xs underline"
            onClick={gitResource.retry}
          >
            Retry
          </button>
        }
      />
    )
  }

  return (
    <GitViewerCenteredState
      title={
        <span className="text-foreground-alt text-xs">
          Git repository not found
        </span>
      }
      detail={`Object: ${objectKey || 'none'}`}
    />
  )
}

/** fileResources lists the file resources of a repo path, in error priority. */
function fileResources(c: GitRepoController): Resource<unknown>[] {
  return [c.rootHandleResource, c.pathHandle, c.statResource, c.entriesResource]
}

type FileView = 'file' | 'loading' | 'error' | 'ready'

/** fileViewFor picks what the path shows in place of its directory tree. */
function fileViewFor(c: GitRepoController): FileView {
  if (c.route.mode === 'commit') return 'ready'
  if (c.isDir === false && c.statResource.value) return 'file'
  const isLoading =
    c.rootHandleResource.loading ||
    c.pathHandle.loading ||
    c.statResource.loading ||
    (c.isDir === true && c.entriesResource.loading)
  if (isLoading) return 'loading'
  if (fileResources(c).some((resource) => resource.error)) return 'error'
  return 'ready'
}

type FrameProps = Omit<ComponentProps<typeof GitViewerFrame>, 'children'>

interface GitRepoFileStatusProps {
  controller: GitRepoController
  view: FileView
}

/** GitRepoFileStatus renders the file, loading, or error view of the path. */
function GitRepoFileStatus({ controller, view }: GitRepoFileStatusProps) {
  const {
    displayPath,
    inlineFileURL,
    nav,
    pendingName,
    readmePath,
    refBarProps,
    rootHandleResource,
    route,
    staleEntries,
    statResource,
    toolbarProps,
  } = controller
  const bareFrameProps: FrameProps = { toolbarProps, refBarProps }

  if (view === 'file' && statResource.value) {
    return (
      <GitViewerFrame
        {...bareFrameProps}
        mode={route.mode}
        onModeChange={nav.handleModeChange}
        hasReadme={!!readmePath}
      >
        <FileViewer
          path={displayPath}
          stat={statResource.value}
          rootHandle={rootHandleResource}
          inlineFileURL={inlineFileURL}
        />
      </GitViewerFrame>
    )
  }

  if (view === 'loading') {
    return (
      <GitViewerFrame {...bareFrameProps}>
        {staleEntries.length > 0 ? (
          <div className="bg-file-back flex min-h-0 flex-1 flex-col overflow-hidden">
            <FileTree
              entries={staleEntries}
              onOpen={nav.handleOpen}
              loadingId={pendingName}
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

  const failed = fileResources(controller).find((resource) => resource.error)
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

interface GitRepoRouteBodyProps {
  controller: GitRepoController
  handle: GitRepoHandle
}

/** GitRepoRouteBody renders the file tree, README, log, or commit panel. */
function GitRepoRouteBody({ controller, handle }: GitRepoRouteBodyProps) {
  const {
    displayPath,
    effectiveRef,
    fileEntries,
    nav,
    navigate,
    readmeContent,
    readmePath,
    route,
  } = controller
  const isRoot = displayPath === '/'

  if (!isRoot || route.mode === 'files') {
    return <FileTree entries={fileEntries} onOpen={nav.handleOpen} autoHeight />
  }

  switch (route.mode) {
    case 'readme':
      return (
        <ReadmeSection
          readmePath={readmePath ?? ''}
          content={readmeContent.value}
          loading={readmeContent.loading}
        />
      )
    case 'log':
      return (
        effectiveRef && (
          <CommitLog
            handle={handle}
            refName={effectiveRef}
            onCommitClick={(hash) => navigate({ path: '/commit/' + hash })}
          />
        )
      )
    case 'commit':
      return (
        route.commitHash && (
          <CommitDetail
            handle={handle}
            commitHash={route.commitHash}
            onNavigateCommit={(hash) => navigate({ path: '/commit/' + hash })}
          />
        )
      )
    default:
      return null
  }
}

// GitRepoContent selects the repository state and renders the corresponding
// file, commit, loading, or failure surface.
function GitRepoContent({ controller }: { controller: GitRepoController }) {
  const {
    gitResource,
    nav,
    objectKey,
    readmePath,
    refBarProps,
    repoInfoResource,
    route,
    toolbarProps,
  } = controller

  if (repoInfoResource.value?.isEmpty) {
    return (
      <GitViewerCenteredState
        title="Empty Repository"
        subtitle={objectKey}
        detail="This repository has no commits yet."
      />
    )
  }

  const handle = gitResource.value
  if (gitResource.loading || gitResource.error || !handle) {
    return <GitRepoLoadState controller={controller} />
  }

  const view = fileViewFor(controller)
  if (view !== 'ready') {
    return <GitRepoFileStatus controller={controller} view={view} />
  }

  const isCommit = route.mode === 'commit'
  return (
    <GitViewerFrame
      toolbarProps={{ ...toolbarProps, showPath: route.mode === 'files' }}
      refBarProps={refBarProps}
      mode={isCommit ? undefined : route.mode}
      onModeChange={isCommit ? undefined : nav.handleModeChange}
      hasReadme={!!readmePath}
    >
      <div className="bg-file-back min-h-0 flex-1 overflow-auto">
        <GitRepoRouteBody controller={controller} handle={handle} />
      </div>
    </GitViewerFrame>
  )
}

// GitRepoViewer renders a Git repository object with branch/tag selector,
// file tree, last commit info, and README display.
export function GitRepoViewer(props: ObjectViewerComponentProps) {
  const controller = useGitRepoController(props)

  return (
    <PanelSizeGate
      minWidth={400}
      fallback={
        <GitViewerCenteredState
          title="Git Repository"
          subtitle={controller.objectKey}
          detail="Make panel larger to view"
        />
      }
    >
      <GitRepoContent controller={controller} />
    </PanelSizeGate>
  )
}
