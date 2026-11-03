import { useCallback, useEffect, useMemo, useRef } from 'react'
import { isDesktop } from '@aptre/bldr'
import { LuGitBranch, LuTriangleAlert } from '@s4wave/web/images/icons.js'

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

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  colors as themeColors,
  radii as themeRadii,
  utilityState as themeUtilityState,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
    padding: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule2: { width: '100%', maxWidth: 'var(--container-sm)' },
  rule3: { marginBottom: `calc(${themeLayout['--spacing']} * 2)` },
  rule4: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-wider)',
    letterSpacing: 'var(--tracking-wider)',
    color: themeColors['--color-foreground'],
    textTransform: 'uppercase',
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule5: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-warning']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-warning']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-warning'],
  },
})

const stylexStyles = stylex.create({
  rule1: {
    marginRight: `calc(${themeLayout['--spacing']} * 2)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    flexShrink: 0,
  },
  rule2: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
  },
})

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

/**
 * useGitRepoFinalize returns the click handler that creates the repository,
 * or starts the clone, from the wizard's draft.
 */
function useGitRepoFinalize(
  ws: WizardState,
  isClone: boolean,
  cloneOpts: GitCloneOpts,
) {
  const { state } = ws

  const handleFinalize = useCallback(async () => {
    if (!state || ws.creating || !ws.localName.trim()) return
    if (isClone && !cloneOpts?.url?.trim()) return

    ws.setCreating(true)
    try {
      await ws.persistDraftState()
      const handle = ws.wizardResource.value
      const repoKey = await buildObjectKey(
        ws.spaceWorld,
        'git/repo/',
        ws.localName,
      )
      const opData = CreateGitRepoWizardOp.toBinary({
        objectKey: repoKey,
        name: ws.localName,
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

  return useCallback(() => {
    void handleFinalize()
  }, [handleFinalize])
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

/** cloneUrlErrorFor returns the browser-only error for a clone URL, or an empty string. */
function cloneUrlErrorFor(isClone: boolean, url: string): string {
  return isClone && !isDesktop && isGithubCloneUrl(url)
    ? githubBrowserCloneError
    : ''
}

/** GitRepoWizardLoading is shown until the wizard state arrives. */
function GitRepoWizardLoading() {
  return (
    <div sx={stylexStyles2.rule1}>
      <div sx={stylexStyles2.rule2}>
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
  const cloneUrlError = cloneUrlErrorFor(isClone, cloneOpts?.url ?? '')
  const { progress, cloning, failed } = useGitCloneProgress(ws)
  const currentStep = state?.step ?? 0

  useCloneAutoName(ws, isClone, inferredCloneName)
  useCloneCompletion(ws, progress)
  const handleFinalizeClick = useGitRepoFinalize(ws, isClone, cloneOpts)

  const handleNext = useCallback(async () => {
    const handle = ws.wizardResource.value
    if (!handle) return
    await ws.persistDraftState()
    await handle.updateState({ step: 1 })
  }, [ws])

  const handleCancel = useCallback(() => {
    void ws.handleCancel()
  }, [ws])

  if (!state) return <GitRepoWizardLoading />

  return (
    <WizardShell
      title={
        <>
          <LuGitBranch sx={stylexStyles.rule1} />
          New Git Repository
        </>
      }
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
        <GitCloneProgressStep progress={progress} failed={failed} />
      )}
    </WizardShell>
  )
}

async function replaceSpaceIndexIfWizardIsCurrent(
  ws: WizardState,
  objectKey: string,
) {
  if (
    parseObjectUri(ws.spaceSettings?.indexPath ?? '').objectKey !== ws.objectKey
  ) {
    return
  }
  await applySpaceIndexPath(ws.spaceWorld, objectKey, {
    sender: ws.sessionPeerId,
    expectedIndexPath: ws.spaceSettings?.indexPath ?? '',
  })
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
      <div sx={stylexStyles2.rule3}>
        <h3 sx={stylexStyles2.rule4}>Clone Progress</h3>
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
    <div sx={stylexStyles2.rule5}>
      <LuTriangleAlert sx={stylexStyles.rule2} />
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
