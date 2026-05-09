import { useCallback, useEffect, useMemo, useRef } from 'react'
import { isDesktop } from '@aptre/bldr'
import { LuGitBranch, LuTriangleAlert } from 'react-icons/lu'

import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { CreateGitRepoWizardOp } from '@s4wave/core/git/git.pb.js'
import { parseObjectUri } from '@s4wave/sdk/space/object-uri.js'
import {
  GitCloneProgressState,
  type GitCloneProgress,
} from '@s4wave/sdk/world/wizard/wizard.pb.js'

import { buildObjectKey } from '../space/create-op-builders.js'
import { applySpaceIndexPath } from '../space/space-settings.js'
import { useWizardState } from './useWizardState.js'
import { WizardShell } from './WizardShell.js'

export const GitRepoWizardTypeID = 'wizard/git/repo'
const githubBrowserCloneError =
  'The browser cannot clone directly from GitHub because GitHub does not set the required cross-origin headers. Use the desktop app to clone this repository.'

// GitRepoWizardViewer is a custom wizard viewer for creating Git repositories.
// Step 0: config editor (mode toggle + clone options). Step 1: repository name.
export function GitRepoWizardViewer(props: ObjectViewerComponentProps) {
  const ws = useWizardState(props, 'git/repo')
  const { state, configEditor } = ws

  const config = configEditor.value as CreateGitRepoWizardOp | undefined
  const isClone = config?.clone ?? false
  const cloneOpts = config?.cloneOpts
  const inferredCloneName = useMemo(
    () => inferGitRepoName(cloneOpts?.url ?? ''),
    [cloneOpts?.url],
  )
  const cloneUrlError =
    isClone && !isDesktop && isGithubCloneUrl(cloneOpts?.url ?? '') ?
      githubBrowserCloneError
    : ''
  const lastAutoNameRef = useRef<string | undefined>(undefined)
  const completedObjectKeyRef = useRef<string | undefined>(undefined)

  const cloneProgressResource = useStreamingResource(
    ws.wizardResource,
    (handle, signal) => handle.watchGitCloneProgress(signal),
    [],
  )
  const cloneProgress = cloneProgressResource.value ?? undefined
  const cloneState = cloneProgress?.state ?? GitCloneProgressState.IDLE
  const cloning = cloneState === GitCloneProgressState.RUNNING
  const cloneFailed = cloneState === GitCloneProgressState.FAILED
  const currentStep = state?.step ?? 0
  const totalSteps = isClone ? 3 : 2

  useEffect(() => {
    if (!isClone || !inferredCloneName) return
    const current = ws.localName.trim()
    const previous = lastAutoNameRef.current
    if (current && current !== 'Repository' && current !== previous) return
    lastAutoNameRef.current = inferredCloneName
    ws.handleUpdateName(inferredCloneName)
  }, [inferredCloneName, isClone, ws])

  useEffect(() => {
    const objectKey = cloneProgress?.objectKey
    if (cloneState !== GitCloneProgressState.DONE || !objectKey) return
    if (completedObjectKeyRef.current === objectKey) return
    completedObjectKeyRef.current = objectKey
    toast.success(`Cloned ${ws.localName}`)
    ws.navigateToObjects([objectKey])
  }, [cloneProgress?.objectKey, cloneState, ws])

  const handleNext = useCallback(async () => {
    const handle = ws.wizardResource.value
    if (!handle) return
    await ws.persistDraftState()
    await handle.updateState({ step: 1 })
  }, [ws])

  const handleFinalize = useCallback(async () => {
    if (!state || ws.creating || !ws.localName.trim()) return
    if (isClone && !cloneOpts?.url?.trim()) return

    ws.setCreating(true)
    try {
      await ws.persistDraftState()
      const handle = ws.wizardResource.value
      const repoKey = buildObjectKey(
        'git/repo/',
        ws.localName,
        ws.existingObjectKeys,
      )
      const opData = CreateGitRepoWizardOp.toBinary({
        objectKey: repoKey,
        name: ws.localName,
        clone: isClone,
        cloneOpts:
          isClone ?
            {
              url: cloneOpts?.url,
              ref: cloneOpts?.ref || undefined,
              depth: cloneOpts?.depth || undefined,
              recursive: cloneOpts?.recursive || undefined,
            }
          : undefined,
        timestamp: new Date(),
      })
      if (isClone) {
        if (!handle) return
        await handle.updateState({ step: 2 })
        await handle.startGitClone({
          objectKey: repoKey,
          name: ws.localName,
          configData: opData,
          opSender: ws.sessionPeerId,
        })
        return
      }
      // eslint-disable-next-line react-doctor/async-parallel
      await ws.spaceWorld.applyWorldOp(
        'spacewave/git/repo/create',
        opData,
        ws.sessionPeerId,
      )
      await replaceSpaceIndexIfWizardIsCurrent(ws, repoKey)
      await ws.spaceWorld.deleteObject(ws.objectKey)
      toast.success(`Created ${ws.localName}`)
      ws.navigateToObjects([repoKey])
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : 'Failed to create repository',
      )
    } finally {
      ws.setCreating(false)
    }
  }, [state, ws, isClone, cloneOpts])

  const handleFinalizeClick = useCallback(() => {
    void handleFinalize()
  }, [handleFinalize])

  const handleCancel = useCallback(() => {
    void ws.handleCancel()
  }, [ws])

  if (!state) {
    return (
      <div className="flex flex-1 items-center justify-center p-6">
        <div className="w-full max-w-sm">
          <LoadingCard
            view={{
              state: 'active',
              title: 'Loading wizard',
              detail: 'Preparing the Git repository workflow.',
            }}
          />
        </div>
      </div>
    )
  }

  return (
    <WizardShell
      title={
        <>
          <LuGitBranch className="mr-2 size-4 shrink-0" />
          New Git Repository
        </>
      }
      step={currentStep}
      totalSteps={totalSteps}
      localName={ws.localName}
      onUpdateName={ws.handleUpdateName}
      onBack={() => void ws.handleBack()}
      canBack={!cloning}
      onCancel={handleCancel}
      nameLabel="Repository Name"
      namePlaceholder="Enter repository name..."
      nameStep={1}
      creating={ws.creating || cloning}
      createLabel={isClone ? 'Clone' : 'Create'}
      creatingLabel={isClone ? 'Cloning...' : 'Creating...'}
      onFinalize={handleFinalizeClick}
      canFinalize={(!isClone || !!cloneOpts?.url?.trim()) && !cloning}
      onNext={() => void handleNext()}
      canNext={!cloneUrlError}
      finalizeStep={1}
    >
      {currentStep === 0 && (
        <>
          {configEditor.element}
          {cloneUrlError && <GitCloneUrlError message={cloneUrlError} />}
        </>
      )}
      {currentStep === 2 && (
        <GitCloneProgressStep progress={cloneProgress} failed={cloneFailed} />
      )}
    </WizardShell>
  )
}

async function replaceSpaceIndexIfWizardIsCurrent(
  ws: ReturnType<typeof useWizardState>,
  objectKey: string,
) {
  if (
    parseObjectUri(ws.spaceSettings?.indexPath ?? '').objectKey !== ws.objectKey
  ) {
    return
  }
  await applySpaceIndexPath(
    ws.spaceWorld,
    ws.spaceSettings,
    objectKey,
    ws.sessionPeerId,
  )
}

function GitCloneProgressStep({
  progress,
  failed,
}: {
  progress: GitCloneProgress | undefined
  failed: boolean
}) {
  const message =
    progress?.message ||
    (failed ? 'Clone failed.' : 'Cloning objects into the local block store.')
  const error = getGitCloneProgressError(progress)

  return (
    <section>
      <div className="mb-2">
        <h3 className="text-foreground text-xs font-medium tracking-wider uppercase select-none">
          Clone Progress
        </h3>
      </div>
      {failed ?
        <LoadingCard
          view={{
            state: 'error',
            title: 'Clone failed',
            error: error || message,
          }}
        />
      : <LoadingCard
          view={{
            state: 'active',
            title: 'Cloning repository',
            detail: message,
          }}
        />
      }
    </section>
  )
}

function GitCloneUrlError({ message }: { message: string }) {
  return (
    <div className="border-warning/20 bg-warning/5 text-warning flex items-start gap-2 rounded-lg border px-3 py-2 text-xs">
      <LuTriangleAlert className="mt-0.5 size-3.5 shrink-0" />
      <p>{message}</p>
    </div>
  )
}

function getGitCloneProgressError(
  progress: GitCloneProgress | undefined,
): string {
  const error = progress?.error ?? ''
  if (isGithubCorsCloneError(error)) return githubBrowserCloneError
  return error
}

function isGithubCorsCloneError(error: string): boolean {
  return (
    error.includes('clone: http transport: unexpected requesting') &&
    error.includes('github.com/') &&
    error.includes('status code: 500') &&
    error.includes('Failed to fetch')
  )
}

function isGithubCloneUrl(url: string): boolean {
  const trimmed = url.trim()
  if (!trimmed) return false
  const host = getGitCloneUrlHost(trimmed).toLowerCase()
  return host === 'github.com'
}

function getGitCloneUrlHost(url: string): string {
  try {
    return new URL(url).hostname
  } catch {
    const scpMatch = url.match(/^[^@]+@([^:]+):.+$/)
    if (scpMatch) return scpMatch[1] ?? ''
    const slashMatch = url.match(/^([^/]+)\/[^/]+\/[^/]+/)
    return slashMatch?.[1] ?? ''
  }
}

function inferGitRepoName(url: string): string {
  const trimmed = url.trim()
  if (!trimmed) return ''
  const withoutQuery = trimmed.split(/[?#]/, 1)[0] ?? ''
  const normalized = withoutQuery.replace(/\/+$/, '').replace(/\.git$/i, '')
  const scpMatch = normalized.match(/^[^@]+@[^:]+:(.+)$/)
  const path = scpMatch?.[1] ?? normalized
  return path.split('/').filter(Boolean).at(-1) ?? ''
}
