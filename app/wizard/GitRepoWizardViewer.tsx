import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { isDesktop } from '@aptre/bldr'
import { LuGitBranch, LuTriangleAlert } from 'react-icons/lu'

import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { CreateGitRepoWizardOp } from '@s4wave/core/git/git.pb.js'
import {
  GitCloneFailure,
  GitCloneProgressState,
  type GitCloneProgress,
} from '@s4wave/sdk/world/wizard/wizard.pb.js'

import { buildObjectKey } from '../space/create-op-builders.js'
import { useWizardState } from './useWizardState.js'
import {
  useWizardFinish,
  WizardInputError,
  type WizardFinishState,
} from './wizard-finish.js'
import { WizardLoading } from './WizardLoading.js'
import { WizardShell } from './WizardShell.js'

export const GitRepoWizardTypeID = 'wizard/git/repo'
const githubBrowserCloneError =
  'The browser cannot clone directly from GitHub because GitHub does not set the required cross-origin headers. Use the desktop app to clone this repository.'

type WizardState = ReturnType<typeof useWizardState>
type GitCloneOpts = CreateGitRepoWizardOp['cloneOpts']

/** useCloneAutoName names the repository after the clone URL until the user renames it. */
function useCloneAutoName(
  ws: WizardState,
  isClone: boolean,
  inferredCloneName: string,
) {
  const lastAutoNameRef = useRef<string | undefined>(undefined)

  useEffect(() => {
    if (!isClone || !inferredCloneName) return
    const current = ws.localName.trim()
    const previous = lastAutoNameRef.current
    if (current && current !== 'Repository' && current !== previous) return
    lastAutoNameRef.current = inferredCloneName
    ws.handleUpdateName(inferredCloneName)
  }, [inferredCloneName, isClone, ws])
}

/** useCloneCompletion opens the cloned repository once, when the clone finishes. */
function useCloneCompletion(
  ws: WizardState,
  cloneProgress: GitCloneProgress | undefined,
) {
  const completedObjectKeyRef = useRef<string | undefined>(undefined)
  const cloneState = cloneProgress?.state
  const objectKey = cloneProgress?.objectKey

  useEffect(() => {
    if (cloneState !== GitCloneProgressState.DONE || !objectKey) return
    if (completedObjectKeyRef.current === objectKey) return
    completedObjectKeyRef.current = objectKey
    toast.success(`Cloned ${ws.localName}`)
    ws.navigateToObjects([objectKey])
  }, [objectKey, cloneState, ws])
}

// gitRepoCreateOp builds the create op for a new or cloned repository.
function gitRepoCreateOp(
  repoKey: string,
  name: string,
  isClone: boolean,
  cloneOpts: GitCloneOpts,
) {
  return CreateGitRepoWizardOp.toBinary({
    objectKey: repoKey,
    name,
    clone: isClone,
    cloneOpts: isClone
      ? {
          url: cloneOpts?.url,
          ref: cloneOpts?.ref || undefined,
          depth: cloneOpts?.depth || undefined,
          recursive: cloneOpts?.recursive || undefined,
        }
      : undefined,
    timestamp: new Date(),
  })
}

/**
 * useGitRepoFinalize returns the Finish state and the click handler that
 * creates the repository through the shared finish, or starts the clone,
 * which the clone job finishes on its own.
 */
function useGitRepoFinalize(
  ws: WizardState,
  isClone: boolean,
  cloneOpts: GitCloneOpts,
) {
  const { state } = ws
  const finish = useWizardFinish(ws)
  const [starting, setStarting] = useState(false)

  const startClone = useCallback(async () => {
    const handle = ws.wizardResource.value
    if (starting || !handle || !ws.localName.trim()) return
    if (!cloneOpts?.url?.trim()) return
    setStarting(true)
    try {
      await ws.persistDraftState()
      const repoKey = await buildObjectKey(
        ws.spaceWorld,
        'git/repo/',
        ws.localName,
      )
      await handle.updateState({ step: 2 })
      await handle.startGitClone({
        objectKey: repoKey,
        name: ws.localName,
        configData: gitRepoCreateOp(repoKey, ws.localName, true, cloneOpts),
        opSender: ws.sessionPeerId,
      })
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to clone')
    } finally {
      setStarting(false)
    }
  }, [cloneOpts, starting, ws])

  const create = useCallback(async () => {
    await finish.finish(
      async () => {
        // Check and save the draft and choose the key before the write
        // transaction opens, so it holds the writer only for the writes.
        if (!ws.localName.trim())
          throw new WizardInputError('Name the repository')
        await ws.persistDraftState()
        const repoKey = await buildObjectKey(
          ws.spaceWorld,
          'git/repo/',
          ws.localName,
        )
        const opData = gitRepoCreateOp(repoKey, ws.localName, false, cloneOpts)
        return {
          creates: [{ kind: 'op', opId: 'spacewave/git/repo/create', opData }],
          target: { openKey: repoKey, indexPath: repoKey, name: ws.localName },
        }
      },
      (target) => {
        toast.success(`Created ${target.name}`)
        ws.navigateToObjects([target.openKey])
      },
    )
  }, [cloneOpts, finish, ws])

  const click = useCallback(() => {
    if (!state) return
    void (isClone ? startClone() : create())
  }, [create, isClone, startClone, state])

  return { finish: finish.state, starting, click }
}

/** useGitCloneProgress watches the wizard's clone progress stream. */
function useGitCloneProgress(ws: WizardState) {
  const cloneProgressResource = useStreamingResource(
    ws.wizardResource,
    (handle, signal) => handle.watchGitCloneProgress(signal),
    [],
  )
  const progress = cloneProgressResource.value ?? undefined
  const cloneState = progress?.state ?? GitCloneProgressState.IDLE

  return {
    progress,
    cloning: cloneState === GitCloneProgressState.RUNNING,
    failed: cloneState === GitCloneProgressState.FAILED,
  }
}

/**
 * gitRepoFinalizeStep returns the step that shows Create or Clone: the name
 * step, or the progress step while it shows a failed clone, so Clone sits
 * beside the failure.
 */
function gitRepoFinalizeStep(failed: boolean, step: number): number {
  return failed && step === 2 ? 2 : 1
}

/**
 * gitRepoFinishState returns the Finish control's state: finishing while a
 * clone starts or runs, gone once a clone was refused because the wizard was
 * removed elsewhere, and the shared Finish state otherwise. A clone refused
 * for a taken key stays failed, so Clone again sits beside its message.
 */
function gitRepoFinishState(
  busy: boolean,
  progress: GitCloneProgress | undefined,
  finish: WizardFinishState,
): WizardFinishState {
  if (busy) return { status: 'finishing' }
  if (
    progress?.state === GitCloneProgressState.FAILED &&
    progress.failure === GitCloneFailure.WIZARD_GONE
  ) {
    return { status: 'gone' }
  }
  return finish
}

/** cloneUrlErrorFor returns the browser-only error for a clone URL, or an empty string. */
function cloneUrlErrorFor(isClone: boolean, url: string): string {
  return isClone && !isDesktop && isGithubCloneUrl(url)
    ? githubBrowserCloneError
    : ''
}

// GitRepoWizardViewer is a custom wizard viewer for creating Git repositories.
// Step 0: config editor (mode toggle + clone options). Step 1: repository name.
// Step 2: clone progress; a failed or refused clone shows Clone beside its
// message, and Clone again picks a free key and starts over.
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
  const cloneUrlError = cloneUrlErrorFor(isClone, cloneOpts?.url ?? '')
  const { progress, cloning, failed } = useGitCloneProgress(ws)
  const currentStep = state?.step ?? 0

  useCloneAutoName(ws, isClone, inferredCloneName)
  useCloneCompletion(ws, progress)
  const finalize = useGitRepoFinalize(ws, isClone, cloneOpts)

  const handleNext = useCallback(async () => {
    const handle = ws.wizardResource.value
    if (!handle) return
    await ws.persistDraftState()
    await handle.updateState({ step: 1 })
  }, [ws])

  const handleCancel = useCallback(() => {
    void ws.handleCancel()
  }, [ws])

  if (!state) {
    return (
      <WizardLoading
        wizardGone={ws.wizardGone}
        detail="Preparing the Git repository workflow."
      />
    )
  }

  return (
    <WizardShell
      icon={<LuGitBranch />}
      title="New Git Repository"
      step={currentStep}
      totalSteps={isClone ? 3 : 2}
      localName={ws.localName}
      onUpdateName={ws.handleUpdateName}
      onBack={() => void ws.handleBack()}
      canBack={!cloning}
      onCancel={handleCancel}
      nameLabel="Repository Name"
      namePlaceholder="Enter repository name..."
      nameStep={1}
      selectNameOnFocus
      finish={gitRepoFinishState(
        cloning || finalize.starting,
        progress,
        finalize.finish,
      )}
      createLabel={isClone ? 'Clone' : 'Create'}
      creatingLabel={isClone ? 'Cloning...' : 'Creating...'}
      onFinalize={finalize.click}
      canFinalize={(!isClone || !!cloneOpts?.url?.trim()) && !cloning}
      onNext={() => void handleNext()}
      canNext={!cloneUrlError}
      finalizeStep={gitRepoFinalizeStep(failed, currentStep)}
    >
      {currentStep === 0 && (
        <>
          {configEditor.element}
          {cloneUrlError && <GitCloneUrlError message={cloneUrlError} />}
        </>
      )}
      {currentStep === 2 && (
        <GitCloneProgressStep progress={progress} failed={failed} />
      )}
    </WizardShell>
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
      {failed ? (
        <LoadingCard
          view={{
            state: 'error',
            title: 'Clone failed',
            error: error || message,
          }}
        />
      ) : (
        <LoadingCard
          view={{
            state: 'active',
            title: 'Cloning repository',
            detail: message,
          }}
        />
      )}
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
