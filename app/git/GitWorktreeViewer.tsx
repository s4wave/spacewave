import {
  useCallback,
  useDeferredValue,
  useMemo,
  useState,
  type ReactNode,
} from 'react'

import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { GitWorktreeHandle } from '@s4wave/sdk/git/worktree.js'
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

import { buildUnixFSFileInlineURL } from '@s4wave/app/unixfs/download.js'

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

// GitWorktreeTypeID is the type identifier for git/worktree objects.
export const GitWorktreeTypeID = 'git/worktree'

// GitWorktreeViewer renders a git worktree object with repo context,
// working directory browser, and magit-style changes view.
export function GitWorktreeViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  const sessionIndex = useSessionIndex()
  const spaceCtx = SpaceContainerContext.useContextSafe()
  const navigate = useNavigate()
  const history = useHistory()
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
  const browsing = useGitBrowsingState(
    repoHandleResource,
    infoResource.value?.checkedOutRef,
    'git-worktree',
  )
  const { route, effectiveRef, tipCommitResource, rootHandleResource } =
    browsing
  const displayPath = route.subpath
  const readmePath = repoInfoResource.value?.readmePath
  const files = useGitFileEntries(
    rootHandleResource,
    displayPath,
    readmePath,
    route.mode === 'files',
  )
  const workdirPath = route.mode === 'workdir' ? route.subpath : '/'
  const workdirFiles = useGitFileEntries(
    workdirHandleResource,
    workdirPath,
    undefined,
    route.mode === 'workdir',
  )
  const statusMap = useMemo(() => {
    const map = new Map<string, StatusEntry>()
    for (const entry of statusEntries) {
      if (entry.filePath) {
        map.set(entry.filePath, entry)
      }
    }
    return map
  }, [statusEntries])
  const renderEntry = useCallback(
    (props: { entry: FileEntry; defaultNode: ReactNode; path: string }) => {
      const path =
        props.path === '/' ?
          props.entry.name
        : props.path.replace(/^\//, '') + '/' + props.entry.name
      const status = statusMap.get(path)
      if (!status) return props.defaultNode

      const code =
        (
          status.worktreeStatus !== undefined &&
          status.worktreeStatus !== FileStatusCode.UNMODIFIED
        ) ?
          status.worktreeStatus
        : (
          status.stagingStatus !== undefined &&
          status.stagingStatus !== FileStatusCode.UNMODIFIED
        ) ?
          status.stagingStatus
        : null
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
  const [pendingName, setPendingName] = useState<string | null>(null)
  const staleFileEntries = useDeferredValue(files.fileEntries)
  const staleWorkdirEntries = useDeferredValue(workdirFiles.fileEntries)
  const isWorkdirMode = route.mode === 'workdir'
  const staleEntries = isWorkdirMode ? staleWorkdirEntries : staleFileEntries
  const inlinePreviewObjectKey = useMemo(
    () =>
      getGitWorktreeInlinePreviewObjectKey({
        mode: route.mode === 'workdir' ? 'workdir' : 'files',
        repoObjectKey: infoResource.value?.repoObjectKey,
        workdirObjectKey: infoResource.value?.workdirObjectKey,
      }),
    [
      infoResource.value?.repoObjectKey,
      infoResource.value?.workdirObjectKey,
      route.mode,
    ],
  )
  const filesInlineFileURL =
    (
      route.mode !== 'files' ||
      files.isDir !== false ||
      !files.statResource.value ||
      !sessionIndex ||
      !spaceCtx?.spaceId ||
      !inlinePreviewObjectKey
    ) ?
      undefined
    : buildUnixFSFileInlineURL(
        sessionIndex,
        spaceCtx.spaceId,
        inlinePreviewObjectKey,
        displayPath,
      )
  const workdirInlineFileURL =
    (
      route.mode !== 'workdir' ||
      workdirFiles.isDir !== false ||
      !workdirFiles.statResource.value ||
      !sessionIndex ||
      !spaceCtx?.spaceId ||
      !inlinePreviewObjectKey
    ) ?
      undefined
    : buildUnixFSFileInlineURL(
        sessionIndex,
        spaceCtx.spaceId,
        inlinePreviewObjectKey,
        workdirPath,
      )
  const nav = useGitNavigation({
    route,
    effectiveRef,
    displayPath,
    navigate,
    history,
    workdirPath,
    onPendingName: setPendingName,
  })
  const availableModes = useMemo<ViewMode[]>(() => {
    const modes: ViewMode[] = ['files']
    if (hasWorkdir) {
      modes.push('workdir', 'changes')
    }
    modes.push('readme', 'log')
    return modes
  }, [hasWorkdir])
  const handleChangesFileClick = useCallback(
    (filePath: string) => {
      navigate({ path: '/workdir/' + filePath })
    },
    [navigate],
  )
  const currentPath = route.mode === 'workdir' ? workdirPath : displayPath
  const toolbarProps = {
    effectiveRef,
    refsResponse: refsResource.value,
    refsLoading: refsResource.loading,
    onRefSelect: nav.handleRefSelect,
    currentPath,
    onPathChange: nav.handlePathChange,
    onBack: nav.handleBack,
    onForward: nav.handleForward,
    onUp: nav.handleUp,
    canGoBack: history?.canGoBack ?? false,
    canGoForward: history?.canGoForward ?? false,
    showPath: route.mode === 'files' || route.mode === 'workdir',
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
        title="Git Worktree"
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

    if (worktreeResource.loading) {
      return (
        <GitViewerCenteredState
          title={
            <span className="text-foreground-alt text-xs">
              Loading worktree...
            </span>
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

    if (
      route.mode === 'files' &&
      files.isDir === false &&
      files.statResource.value
    ) {
      return (
        <GitViewerFrame
          {...viewerFrameProps}
          mode={route.mode}
          onModeChange={nav.handleModeChange}
          hasReadme={!!readmePath}
          availableModes={availableModes}
        >
          <FileViewer
            path={displayPath}
            stat={files.statResource.value}
            rootHandle={rootHandleResource}
            inlineFileURL={filesInlineFileURL}
          />
        </GitViewerFrame>
      )
    }

    if (
      route.mode === 'workdir' &&
      workdirFiles.isDir === false &&
      workdirFiles.statResource.value
    ) {
      return (
        <GitViewerFrame
          {...viewerFrameProps}
          mode={route.mode}
          onModeChange={nav.handleModeChange}
          hasReadme={!!readmePath}
          availableModes={availableModes}
        >
          <FileViewer
            path={workdirPath}
            stat={workdirFiles.statResource.value}
            rootHandle={workdirHandleResource}
            inlineFileURL={workdirInlineFileURL}
          />
        </GitViewerFrame>
      )
    }

    const isFileLoading =
      route.mode === 'files' &&
      (rootHandleResource.loading ||
        files.pathHandle.loading ||
        files.statResource.loading ||
        (files.isDir === true && files.entriesResource.loading))
    const isWorkdirLoading =
      route.mode === 'workdir' &&
      (workdirHandleResource.loading ||
        workdirFiles.pathHandle.loading ||
        workdirFiles.statResource.loading ||
        (workdirFiles.isDir === true && workdirFiles.entriesResource.loading))
    const isContentLoading = isFileLoading || isWorkdirLoading

    if (
      route.mode !== 'commit' &&
      route.mode !== 'changes' &&
      isContentLoading &&
      staleEntries.length > 0
    ) {
      return (
        <GitViewerFrame {...viewerFrameProps}>
          <div className="bg-file-back flex min-h-0 flex-1 flex-col overflow-hidden">
            <FileTree
              entries={staleEntries}
              onOpen={nav.handleOpen}
              loadingId={pendingName}
              renderEntry={route.mode === 'workdir' ? renderEntry : undefined}
              currentPath={route.mode === 'workdir' ? workdirPath : undefined}
            />
          </div>
        </GitViewerFrame>
      )
    }

    if (
      route.mode !== 'commit' &&
      route.mode !== 'changes' &&
      isContentLoading
    ) {
      return (
        <GitViewerFrame {...viewerFrameProps}>
          <div className="bg-file-back flex min-h-0 flex-1 flex-col items-center justify-center overflow-hidden">
            <div className="text-foreground-alt text-xs">Loading files...</div>
          </div>
        </GitViewerFrame>
      )
    }

    const fileError =
      route.mode === 'commit' || route.mode === 'changes' ? null
      : route.mode === 'workdir' ?
        (workdirHandleResource.error ??
        workdirFiles.pathHandle.error ??
        workdirFiles.statResource.error ??
        workdirFiles.entriesResource.error)
      : (rootHandleResource.error ??
        files.pathHandle.error ??
        files.statResource.error ??
        files.entriesResource.error)
    if (fileError) {
      function handleRetry() {
        if (route.mode === 'workdir') {
          if (workdirHandleResource.error) {
            workdirHandleResource.retry()
            return
          }
          if (workdirFiles.pathHandle.error) {
            workdirFiles.pathHandle.retry()
            return
          }
          if (workdirFiles.statResource.error) {
            workdirFiles.statResource.retry()
            return
          }
          if (workdirFiles.entriesResource.error) {
            workdirFiles.entriesResource.retry()
          }
          return
        }
        if (rootHandleResource.error) {
          rootHandleResource.retry()
          return
        }
        if (files.pathHandle.error) {
          files.pathHandle.retry()
          return
        }
        if (files.statResource.error) {
          files.statResource.retry()
          return
        }
        if (files.entriesResource.error) {
          files.entriesResource.retry()
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

    const isRoot =
      route.mode === 'workdir' ? workdirPath === '/' : displayPath === '/'

    return (
      <GitViewerFrame
        {...viewerFrameProps}
        mode={route.mode === 'commit' ? undefined : route.mode}
        onModeChange={
          route.mode === 'commit' ? undefined : nav.handleModeChange
        }
        hasReadme={!!readmePath}
        availableModes={availableModes}
      >
        <div className="bg-file-back min-h-0 flex-1 overflow-auto">
          {route.mode === 'files' && (
            <FileTree
              entries={files.fileEntries}
              onOpen={nav.handleOpen}
              autoHeight={isRoot}
            />
          )}
          {route.mode === 'workdir' && (
            <FileTree
              entries={workdirFiles.fileEntries}
              onOpen={nav.handleOpen}
              autoHeight={isRoot}
              renderEntry={renderEntry}
              currentPath={workdirPath}
            />
          )}
          {route.mode === 'changes' && worktreeResource.value && (
            <ChangesView
              entries={statusEntries}
              handle={worktreeResource.value}
              onFileClick={handleChangesFileClick}
            />
          )}
          {isRoot && route.mode === 'readme' && (
            <ReadmeSection
              readmePath={readmePath ?? ''}
              content={files.readmeContent.value}
              loading={files.readmeContent.loading}
            />
          )}
          {isRoot &&
            route.mode === 'log' &&
            repoHandleResource.value &&
            effectiveRef && (
              <CommitLog
                handle={repoHandleResource.value}
                refName={effectiveRef}
                onCommitClick={(hash) => navigate({ path: '/commit/' + hash })}
              />
            )}
          {isRoot &&
            route.mode === 'commit' &&
            repoHandleResource.value &&
            route.commitHash && (
              <CommitDetail
                handle={repoHandleResource.value}
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
