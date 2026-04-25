import { useDeferredValue, useState } from 'react'

import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { GitRepoHandle } from '@s4wave/sdk/git/repo.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { useSessionIndex } from '@s4wave/web/contexts/contexts.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { useHistory } from '@s4wave/web/router/HistoryRouter.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { PanelSizeGate } from '@s4wave/web/ui/PanelSizeGate.js'

import { buildUnixFSFileInlineURL } from '@s4wave/app/unixfs/download.js'

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

// GitRepoViewer renders a git repository object with branch/tag selector,
// file tree, last commit info, and README display.
export function GitRepoViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  const sessionIndex = useSessionIndex()
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
    route.mode === 'commit' || isDir !== false || !statResource.value ?
      undefined
    : !sessionIndex || !spaceCtx?.spaceId ? undefined
    : buildUnixFSFileInlineURL(
        sessionIndex,
        spaceCtx.spaceId,
        objectKey,
        displayPath,
      )
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
    onClickCommit:
      tipCommitHash ?
        () => navigate({ path: '/commit/' + tipCommitHash })
      : undefined,
    onClickTree:
      effectiveRef ?
        () => navigate({ path: '/tree/' + effectiveRef })
      : undefined,
    onClickLog:
      effectiveRef ?
        () => navigate({ path: '/commits/' + effectiveRef })
      : undefined,
  }

  function renderSizeFallback() {
    return (
      <GitViewerCenteredState
        title="Git Repository"
        subtitle={objectKey}
        detail="Make panel larger to view"
      />
    )
  }

  function renderContent() {
    const viewerFrameProps = {
      toolbarProps,
      refBarProps,
    }

    const repoInfo = repoInfoResource.value
    if (repoInfo?.isEmpty) {
      return (
        <GitViewerCenteredState
          title="Empty Repository"
          subtitle={objectKey}
          detail="This repository has no commits yet."
        />
      )
    }

    if (gitResource.loading) {
      return (
        <GitViewerCenteredState
          title={
            <span className="text-foreground-alt text-xs">
              Loading repository...
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
              className="text-brand mt-2 text-xs underline"
              onClick={gitResource.retry}
            >
              Retry
            </button>
          }
        />
      )
    }

    if (!gitResource.value) {
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

    if (route.mode !== 'commit' && isDir === false && statResource.value) {
      return (
        <GitViewerFrame
          {...viewerFrameProps}
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

    const isFileLoading =
      rootHandleResource.loading ||
      pathHandle.loading ||
      statResource.loading ||
      (isDir === true && entriesResource.loading)

    if (route.mode !== 'commit' && isFileLoading && staleEntries.length > 0) {
      return (
        <GitViewerFrame {...viewerFrameProps}>
          <div className="bg-file-back flex min-h-0 flex-1 flex-col overflow-hidden">
            <FileTree
              entries={staleEntries}
              onOpen={nav.handleOpen}
              loadingId={pendingName}
            />
          </div>
        </GitViewerFrame>
      )
    }

    if (route.mode !== 'commit' && isFileLoading) {
      return (
        <GitViewerFrame {...viewerFrameProps}>
          <div className="bg-file-back flex min-h-0 flex-1 flex-col items-center justify-center overflow-hidden">
            <div className="text-foreground-alt text-xs">Loading files...</div>
          </div>
        </GitViewerFrame>
      )
    }

    const fileError =
      route.mode === 'commit' ?
        null
      : (rootHandleResource.error ??
        pathHandle.error ??
        statResource.error ??
        entriesResource.error)
    if (fileError) {
      function handleRetry() {
        if (rootHandleResource.error) {
          rootHandleResource.retry()
          return
        }
        if (pathHandle.error) {
          pathHandle.retry()
          return
        }
        if (statResource.error) {
          statResource.retry()
          return
        }
        if (entriesResource.error) {
          entriesResource.retry()
        }
      }

      return (
        <GitViewerFrame {...viewerFrameProps}>
          <div className="bg-file-back flex min-h-0 flex-1 flex-col items-center justify-center overflow-hidden">
            <div className="text-destructive text-xs">Error loading files</div>
            <div className="text-foreground-alt/70 mt-1 text-xs">
              {fileError.message}
            </div>
            <button
              className="text-brand mt-2 text-xs underline"
              onClick={handleRetry}
            >
              Retry
            </button>
          </div>
        </GitViewerFrame>
      )
    }

    const isRoot = displayPath === '/'

    return (
      <GitViewerFrame
        {...viewerFrameProps}
        toolbarProps={{ ...toolbarProps, showPath: route.mode === 'files' }}
        mode={route.mode === 'commit' ? undefined : route.mode}
        onModeChange={
          route.mode === 'commit' ? undefined : nav.handleModeChange
        }
        hasReadme={!!readmePath}
      >
        <div className="bg-file-back min-h-0 flex-1 overflow-auto">
          {(!isRoot || route.mode === 'files') && (
            <FileTree
              entries={fileEntries}
              onOpen={nav.handleOpen}
              autoHeight
            />
          )}
          {isRoot && route.mode === 'readme' && (
            <ReadmeSection
              readmePath={readmePath ?? ''}
              content={readmeContent.value}
              loading={readmeContent.loading}
            />
          )}
          {isRoot &&
            route.mode === 'log' &&
            gitResource.value &&
            effectiveRef && (
              <CommitLog
                handle={gitResource.value}
                refName={effectiveRef}
                onCommitClick={(hash) => navigate({ path: '/commit/' + hash })}
              />
            )}
          {isRoot &&
            route.mode === 'commit' &&
            gitResource.value &&
            route.commitHash && (
              <CommitDetail
                handle={gitResource.value}
                commitHash={route.commitHash}
                onNavigateCommit={(hash) =>
                  navigate({ path: '/commit/' + hash })
                }
              />
            )}
        </div>
      </GitViewerFrame>
    )
  }

  return (
    <PanelSizeGate minWidth={400} fallback={renderSizeFallback()}>
      {renderContent()}
    </PanelSizeGate>
  )
}
