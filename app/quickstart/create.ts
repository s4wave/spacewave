/* eslint-disable react-doctor/async-await-in-loop */
import type { RegisterCleanup } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { Root } from '@s4wave/sdk/root'
import type { Session } from '@s4wave/sdk/session'
import type { CreateSpaceResponse } from '@s4wave/sdk/session/session.pb.js'
import type { CreateAccountResponse } from '@s4wave/sdk/provider/local/local.pb.js'
import type { WatchQuickstartsResponse } from '@s4wave/sdk/quickstart/registry/registry.pb.js'
import { QuickstartRegistryResourceServiceClient } from '@s4wave/sdk/quickstart/registry/registry_srpc.pb.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import { LocalProvider } from '@s4wave/sdk/provider/local/local.js'
import { Space } from '@s4wave/sdk/space/space.js'
import { SpaceContents } from '@s4wave/sdk/space/contents.js'
import { Engine } from '@s4wave/sdk/world/engine.js'
import { EngineWorldState } from '@s4wave/sdk/world/engine-state.js'
import { BucketLookupCursor } from '@s4wave/sdk/bucket/lookup/lookup.js'
import {
  isValidSpacePluginId,
  SPACE_SETTINGS_BLOCK_TYPE,
  SPACE_SETTINGS_OBJECT_KEY,
} from '@s4wave/core/space/world/world.js'
import { SpaceSettings } from '@s4wave/core/space/world/world.pb.js'
import {
  InitUnixFSOp,
  InitObjectLayoutOp,
  SetSpaceSettingsOp,
} from '@s4wave/core/space/world/ops/ops.pb.js'
import { SET_SPACE_SETTINGS_OP_ID } from '@s4wave/core/space/world/ops/set-space-settings.js'
import {
  INIT_UNIXFS_OP_ID,
  UNIXFS_OBJECT_KEY,
} from '@s4wave/core/space/world/ops/init-unixfs.js'
import {
  INIT_OBJECT_LAYOUT_OP_ID,
  OBJECT_LAYOUT_OBJECT_KEY,
} from '@s4wave/core/space/world/ops/init-object-layout.js'
import {
  INIT_CANVAS_DEMO_OP_ID,
  CANVAS_DEMO_OBJECT_KEY,
} from '@s4wave/core/space/world/ops/init-canvas-demo.js'
import { InitCanvasDemoOp } from '@s4wave/core/space/world/ops/ops.pb.js'

import { InitChatDemoOp } from '@s4wave/sdk/chat/chat.pb.js'
import {
  INIT_CHAT_DEMO_OP_ID,
  CHAT_DEMO_CHANNEL_KEY,
} from '@s4wave/sdk/chat/init-chat-demo.js'
import { V86WizardConfig } from '@s4wave/sdk/vm/v86-wizard.pb.js'
import { CreateWizardObjectOp } from '@s4wave/sdk/world/wizard/wizard.pb.js'
import { CREATE_WIZARD_OBJECT_OP_ID } from '@s4wave/sdk/world/wizard/create-wizard.js'
import { InitForgeQuickstartOp } from '@s4wave/core/forge/dashboard/dashboard.pb.js'
import { INIT_FORGE_QUICKSTART_OP_ID } from '@s4wave/sdk/forge/dashboard/init-forge-quickstart.js'
import { markInteracted } from '@s4wave/web/state/interaction.js'
import { mountSpace } from '@s4wave/app/space/space.js'
import { buildWizardObjectKey } from '@s4wave/app/space/create-op-builders.js'
import {
  buildV86QuickstartWizardConfig,
  buildV86QuickstartWizardKey,
  V86_WIZARD_TARGET_KEY_PREFIX,
  V86_WIZARD_TARGET_TYPE_ID,
  V86_WIZARD_TYPE_ID,
} from '@s4wave/app/vm/v86-wizard-config.js'

import { type QuickstartSpaceCreateId } from './options.js'

const NOTES_PLUGIN_ID = 'spacewave-notes'
const QUICKSTART_REGISTRATION_TIMEOUT_MS = import.meta.env?.DEV ? 240000 : 30000
const QUICKSTART_LOCAL_PROVIDER_READY_TIMEOUT_MS = 120000
const QUICKSTART_CREATE_LOCAL_ACCOUNT_TIMEOUT_MS = import.meta.env?.DEV
  ? 240000
  : 90000
const QUICKSTART_RECOVER_LOCAL_SESSION_TIMEOUT_MS = import.meta.env?.DEV
  ? 60000
  : 15000

type NotesQuickstartId = Extract<
  QuickstartSpaceCreateId,
  'notebook' | 'docs' | 'blog'
>

export interface QuickstartPhaseTiming {
  name: string
  startedMs: number
  finishedMs?: number
  elapsedMs?: number
  error?: string
}

export interface QuickstartSetupTiming {
  quickstartId: QuickstartSpaceCreateId
  state: 'loading' | 'progress-ready' | 'content-ready' | 'error' | 'cancelled'
  startedMs: number
  progressReadyMs?: number
  contentReadyMs?: number
  finishedMs?: number
  elapsedMs?: number
  error?: string
  phases: QuickstartPhaseTiming[]
}

export type QuickstartProgressStep = 'session' | 'space' | 'frame' | 'content'

export interface QuickstartProgressState {
  step: QuickstartProgressStep
  stepIndex: number
  stepCount: number
  detail: string
}

export type QuickstartProgressReporter = (
  state: QuickstartProgressState,
) => void

declare global {
  var __s4waveQuickstartTiming: QuickstartSetupTiming | undefined
  var __s4waveLogQuickstartTiming: boolean | undefined
  var __s4wave_debug: { quickstartTiming?: QuickstartSetupTiming } | undefined
}

const quickstartProgressOrder: QuickstartProgressStep[] = [
  'session',
  'space',
  'frame',
  'content',
]

function reportQuickstartProgress(
  progress: QuickstartProgressReporter | undefined,
  step: QuickstartProgressStep,
  detail: string,
): void {
  if (!progress) return
  progress({
    step,
    stepIndex: quickstartProgressOrder.indexOf(step) + 1,
    stepCount: quickstartProgressOrder.length,
    detail,
  })
}

function nowMs(): number {
  return Math.round(performance.now())
}

function getErrorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}

function isQuickstartRpcAbort(err: unknown): boolean {
  return getErrorMessage(err).includes('ERR_RPC_ABORT')
}

function startQuickstartTiming(
  quickstartId: QuickstartSpaceCreateId,
): QuickstartSetupTiming {
  const timing: QuickstartSetupTiming = {
    quickstartId,
    state: 'loading',
    startedMs: nowMs(),
    phases: [],
  }
  publishQuickstartTiming(timing)
  return timing
}

function publishQuickstartTiming(timing: QuickstartSetupTiming): void {
  globalThis.__s4waveQuickstartTiming = timing
  if (globalThis.__s4wave_debug) {
    globalThis.__s4wave_debug.quickstartTiming = timing
  }
}

function finishQuickstartTiming(
  timing: QuickstartSetupTiming,
  err?: unknown,
  abortSignal?: AbortSignal,
): void {
  const finishedMs = nowMs()
  timing.finishedMs = finishedMs
  timing.elapsedMs = finishedMs - timing.startedMs
  if (err) {
    timing.error = getErrorMessage(err)
    timing.state = abortSignal?.aborted ? 'cancelled' : 'error'
  } else {
    timing.state = 'content-ready'
    timing.contentReadyMs = finishedMs
  }
  publishQuickstartTiming(timing)
  if (globalThis.__s4waveLogQuickstartTiming) {
    console.log('quickstart timing: ' + JSON.stringify(timing))
  }
}

function markQuickstartProgressReady(timing: QuickstartSetupTiming): void {
  if (typeof timing.progressReadyMs === 'number') return
  timing.progressReadyMs = nowMs()
  timing.state = 'progress-ready'
  publishQuickstartTiming(timing)
  if (globalThis.__s4waveLogQuickstartTiming) {
    console.log('quickstart progress ready: ' + JSON.stringify(timing))
  }
}

async function timeQuickstartPhase<T>(
  timing: QuickstartSetupTiming | undefined,
  name: string,
  cb: () => Promise<T>,
): Promise<T> {
  if (!timing) {
    return cb()
  }

  const startedMs = nowMs()
  const phase: QuickstartPhaseTiming = { name, startedMs }
  timing.phases.push(phase)
  publishQuickstartTiming(timing)
  if (globalThis.__s4waveLogQuickstartTiming) {
    console.log('quickstart phase started: ' + name)
  }

  try {
    const result = await cb()
    const finishedMs = nowMs()
    phase.finishedMs = finishedMs
    phase.elapsedMs = finishedMs - startedMs
    publishQuickstartTiming(timing)
    if (globalThis.__s4waveLogQuickstartTiming) {
      console.log('quickstart phase finished: ' + JSON.stringify(phase))
    }
    return result
  } catch (err) {
    const finishedMs = nowMs()
    phase.finishedMs = finishedMs
    phase.elapsedMs = finishedMs - startedMs
    phase.error = getErrorMessage(err)
    publishQuickstartTiming(timing)
    if (globalThis.__s4waveLogQuickstartTiming) {
      console.log('quickstart phase failed: ' + JSON.stringify(phase))
    }
    throw err
  }
}

function makeQuickstartAttemptSignal(
  abortSignal: AbortSignal | undefined,
  timeoutMs: number,
): AbortSignal {
  const timeoutSignal = AbortSignal.timeout(timeoutMs)
  if (!abortSignal) return timeoutSignal
  return AbortSignal.any([abortSignal, timeoutSignal])
}

async function retryQuickstartRpc<T>(
  abortSignal: AbortSignal | undefined,
  timeoutMs: number,
  cb: (signal: AbortSignal) => Promise<T>,
  maxAttempts = 3,
): Promise<T> {
  let lastErr: unknown
  for (let attempt = 0; attempt < maxAttempts; attempt++) {
    if (abortSignal?.aborted) {
      throw new DOMException('Aborted', 'AbortError')
    }
    const signal = makeQuickstartAttemptSignal(abortSignal, timeoutMs)
    try {
      return await cb(signal)
    } catch (err) {
      lastErr = err
      if (abortSignal?.aborted) throw err
    }
  }
  throw lastErr
}

// findMostRecentLocalSession returns the most recent local session from the
// current session list, or undefined if none exist.
async function findMostRecentLocalSession(
  root: Root,
  abortSignal: AbortSignal,
): Promise<
  | {
      sessionRef: import('@s4wave/core/session/session.pb.js').SessionRef
      sessionIndex: number
    }
  | undefined
> {
  const resp = await retryQuickstartRpc(abortSignal, 15000, (signal) =>
    root.listSessions(signal),
  )
  const sessions = resp.sessions ?? []
  let best: (typeof sessions)[number] | undefined
  for (const s of sessions) {
    if (s.sessionRef?.providerResourceRef?.providerId !== 'local') continue
    if (!best || (s.sessionIndex ?? 0) > (best.sessionIndex ?? 0)) {
      best = s
    }
  }
  if (best?.sessionRef) {
    return {
      sessionRef: best.sessionRef,
      sessionIndex: best.sessionIndex ?? 0,
    }
  }
  return undefined
}

function hasStoredLocalSessionHint(): boolean {
  if (typeof localStorage === 'undefined') return true
  return localStorage.getItem('spacewave-has-session') === '1'
}

export function getQuickstartSpaceName(
  quickstartId: QuickstartSpaceCreateId,
): string {
  switch (quickstartId) {
    case 'space':
      return 'My Space'
    case 'drive':
      return 'My Drive'
    case 'git':
      return 'My Git Repository'
    case 'notebook':
      return 'My Notebook'
    case 'canvas':
      return 'My Canvas'
    case 'chat':
      return 'My Chat'
    case 'docs':
      return 'My Docs'
    case 'blog':
      return 'My Blog'
    case 'v86':
      return 'My V86 VM'
    case 'forge':
      return 'My Forge Dashboard'
  }
}

// createLocalSession creates a local provider account and mounts a session without creating a space.
// If forceNew is false and a local session already exists, it reuses the most recent one.
export async function createLocalSession(
  root: Root,
  abortSignal: AbortSignal,
  cleanup: RegisterCleanup,
  forceNew?: boolean,
  timing?: QuickstartSetupTiming,
  progress?: QuickstartProgressReporter,
): Promise<LocalSessionSetup> {
  // Check for an existing local session to reuse.
  const hasLocalSessionHint = hasStoredLocalSessionHint()
  if (!forceNew && hasLocalSessionHint) {
    reportQuickstartProgress(
      progress,
      'session',
      'Checking for an existing local session',
    )
    const existing = await timeQuickstartPhase(
      timing,
      'find-existing-local-session',
      () => findMostRecentLocalSession(root, abortSignal),
    )
    if (existing) {
      reportQuickstartProgress(progress, 'session', 'Mounting local session')
      const session = cleanup(
        await timeQuickstartPhase(timing, 'mount-existing-local-session', () =>
          root.mountSession({ sessionRef: existing.sessionRef }, abortSignal),
        ),
      )
      markInteracted()
      return { sessionIndex: existing.sessionIndex, session }
    }
  }

  // No existing local session (or forceNew): create a new account.
  reportQuickstartProgress(progress, 'session', 'Opening the local provider')
  using provider = await timeQuickstartPhase(
    timing,
    'lookup-local-provider',
    () =>
      retryQuickstartRpc(
        abortSignal,
        QUICKSTART_LOCAL_PROVIDER_READY_TIMEOUT_MS,
        (signal) => root.lookupProvider('local', signal),
        2,
      ),
  )
  const lp = new LocalProvider(provider.resourceRef)
  let accountResp: CreateAccountResponse
  try {
    reportQuickstartProgress(progress, 'session', 'Creating a local session')
    accountResp = await timeQuickstartPhase(
      timing,
      'create-local-account',
      () =>
        lp.createAccount(
          makeQuickstartAttemptSignal(
            abortSignal,
            QUICKSTART_CREATE_LOCAL_ACCOUNT_TIMEOUT_MS,
          ),
        ),
    )
  } catch (err) {
    if (!isQuickstartRpcAbort(err)) throw err
    if (!forceNew && !hasLocalSessionHint) {
      reportQuickstartProgress(
        progress,
        'session',
        'Recovering the created local session',
      )
      const mounted = await timeQuickstartPhase(
        timing,
        'mount-created-local-session-by-index',
        () =>
          retryQuickstartRpc(
            abortSignal,
            QUICKSTART_RECOVER_LOCAL_SESSION_TIMEOUT_MS,
            (signal) => root.mountSessionByIdx({ sessionIdx: 1 }, signal),
          ),
      )
      if (mounted) {
        const session = cleanup(mounted.session)
        markInteracted()
        return { sessionIndex: 1, session }
      }
    }
    reportQuickstartProgress(
      progress,
      'session',
      'Finding the created local session',
    )
    const existing = await timeQuickstartPhase(
      timing,
      'recover-created-local-session',
      () => findMostRecentLocalSession(root, abortSignal),
    )
    if (existing) {
      reportQuickstartProgress(
        progress,
        'session',
        'Mounting recovered local session',
      )
      const session = cleanup(
        await timeQuickstartPhase(timing, 'mount-recovered-local-session', () =>
          root.mountSession({ sessionRef: existing.sessionRef }, abortSignal),
        ),
      )
      markInteracted()
      return { sessionIndex: existing.sessionIndex, session }
    }
    throw err
  }
  const sessionIndex = accountResp.sessionListEntry?.sessionIndex ?? 1

  // Mount the session using the account's session reference.
  reportQuickstartProgress(progress, 'session', 'Mounting new local session')
  const session = cleanup(
    await timeQuickstartPhase(timing, 'mount-new-local-session', () =>
      root.mountSession(
        { sessionRef: accountResp.sessionListEntry?.sessionRef },
        abortSignal,
      ),
    ),
  )

  markInteracted()

  return { accountResp, sessionIndex, session }
}

// LocalSessionSetup is the result of creating or reusing a local session.
export interface LocalSessionSetup {
  accountResp?: CreateAccountResponse
  sessionIndex: number
  session: Session
}

export interface QuickstartSetup {
  root?: Root
  accountResp?: CreateAccountResponse
  sessionIndex?: number
  spaceResp: CreateSpaceResponse
  session: Session
  space: Space
  spaceContents?: SpaceContents
  spaceWorld: EngineWorldState
  spaceWorldState: BucketLookupCursor
}

// QuickstartSetupParams contains the parameters for creating a quickstart setup.
export interface QuickstartSetupParams {
  session: Session
  spaceResp: CreateSpaceResponse
  abortSignal: AbortSignal
  cleanup: RegisterCleanup
  timing?: QuickstartSetupTiming
  progress?: QuickstartProgressReporter
  mountContents?: boolean
}

// createQuickstartSetupFromSession creates a quickstart setup from an existing session and space response.
export async function createQuickstartSetupFromSession(
  params: QuickstartSetupParams,
): Promise<
  Omit<
    QuickstartSetup,
    'accountResp' | 'sessionIndex' | 'session' | 'spaceResp'
  >
> {
  const {
    session,
    spaceResp,
    abortSignal,
    cleanup,
    timing,
    progress,
    mountContents = true,
  } = params

  // Mount the space from the response.
  reportQuickstartProgress(progress, 'frame', 'Mounting the new space')
  const space = await timeQuickstartPhase(timing, 'mount-space', () =>
    mountSpace({
      session,
      spaceResp,
      abortSignal,
      cleanup,
      phase: (name, cb) => timeQuickstartPhase(timing, name, cb),
    }),
  )

  // Access the World associated with the space as a WorldState.
  reportQuickstartProgress(progress, 'frame', 'Loading the space frame')
  const spaceWorld = cleanup(
    await timeQuickstartPhase(timing, 'access-space-world', async () => {
      const spaceWorldResourceId = spaceResp.spaceWorldResourceId ?? 0
      if (spaceWorldResourceId !== 0) {
        return new EngineWorldState(
          session.resourceRef.createResource(spaceWorldResourceId, Engine),
          true,
          true,
        )
      }
      return await space.accessWorldState(true, abortSignal)
    }),
  )
  let spaceContents: SpaceContents | undefined
  if (mountContents) {
    reportQuickstartProgress(progress, 'frame', 'Mounting space contents')
    spaceContents = cleanup(
      await timeQuickstartPhase(timing, 'mount-space-contents', () =>
        space.mountSpaceContents(abortSignal),
      ),
    )
  }

  // Access the world state bucket storage.
  reportQuickstartProgress(progress, 'frame', 'Preparing world state')
  const spaceWorldState = cleanup(
    await timeQuickstartPhase(timing, 'access-space-world-state', () =>
      spaceWorld.accessWorldState(undefined, abortSignal),
    ),
  )

  return {
    space,
    ...(spaceContents ? { spaceContents } : {}),
    spaceWorld,
    spaceWorldState,
  }
}

export async function createQuickstartSetup(
  root: Root,
  quickstartId: QuickstartSpaceCreateId,
  abortSignal: AbortSignal,
  cleanup: RegisterCleanup,
  progress?: QuickstartProgressReporter,
): Promise<QuickstartSetup> {
  const timing = startQuickstartTiming(quickstartId)
  try {
    reportQuickstartProgress(progress, 'session', 'Preparing a local session')
    // Reuse existing local session or create a new one.
    const { accountResp, sessionIndex, session } = await createLocalSession(
      root,
      abortSignal,
      cleanup,
      undefined,
      timing,
      progress,
    )

    // Create a new space with the quickstart ID as the name.
    reportQuickstartProgress(
      progress,
      'space',
      'Creating ' + getQuickstartSpaceName(quickstartId),
    )
    const spaceResp = await timeQuickstartPhase(timing, 'create-space', () =>
      session.createSpace(
        { spaceName: getQuickstartSpaceName(quickstartId) },
        abortSignal,
      ),
    )

    // Create the setup from the session and space response.
    const setup = await createQuickstartSetupFromSession({
      session,
      spaceResp,
      abortSignal,
      cleanup,
      timing,
      progress,
      mountContents: quickstartId !== 'drive',
    })

    // Construct the result
    const result = {
      root,
      accountResp,
      sessionIndex,
      session,
      spaceResp,
      ...setup,
    }

    markQuickstartProgressReady(timing)

    // Populate the space with quickstart-specific content.
    reportQuickstartProgress(
      progress,
      'content',
      'Seeding ' + getQuickstartSpaceName(quickstartId) + ' content',
    )
    await timeQuickstartPhase(timing, 'populate-space', () =>
      populateSpace(quickstartId, result, abortSignal, timing),
    )

    finishQuickstartTiming(timing)
    return result
  } catch (err) {
    finishQuickstartTiming(timing, err, abortSignal)
    throw err
  }
}

export function getQuickstartInitialObjectKey(
  quickstartId: QuickstartSpaceCreateId,
): string {
  switch (quickstartId) {
    case 'drive':
      return UNIXFS_OBJECT_KEY
    case 'canvas':
      return CANVAS_DEMO_OBJECT_KEY
    case 'chat':
      return CHAT_DEMO_CHANNEL_KEY
    case 'forge':
      return 'forge'
    case 'space':
    case 'git':
    case 'notebook':
    case 'docs':
    case 'blog':
    case 'v86':
      return ''
    default: {
      const _exhaustive: never = quickstartId
      return _exhaustive
    }
  }
}

export function getQuickstartInitialObjectType(
  quickstartId: QuickstartSpaceCreateId,
): string {
  switch (quickstartId) {
    case 'drive':
      return 'unixfs/fs-node'
    case 'space':
    case 'git':
    case 'notebook':
    case 'docs':
    case 'blog':
    case 'canvas':
    case 'chat':
    case 'v86':
    case 'forge':
      return ''
    default: {
      const _exhaustive: never = quickstartId
      return _exhaustive
    }
  }
}

export function getQuickstartInitialObjectRouteHandoff(
  quickstartId: QuickstartSpaceCreateId,
): { objectKey: string; objectType: string } | undefined {
  const objectKey = getQuickstartInitialObjectKey(quickstartId)
  const objectType = getQuickstartInitialObjectType(quickstartId)
  if (!objectKey || !objectType) {
    return undefined
  }
  return { objectKey, objectType }
}

export function buildQuickstartSpaceRoutePath(
  basePath: string,
  quickstartId: QuickstartSpaceCreateId,
): string {
  const objectKey = getQuickstartInitialObjectKey(quickstartId)
  if (!objectKey) {
    return basePath
  }
  return basePath.replace(/\/+$/, '') + '/' + objectKey
}

// createSpaceSettingsObject creates the SpaceSettings object in the world.
export async function createSpaceSettingsObject(
  spaceWorld: IWorldState,
  abortSignal?: AbortSignal,
  indexPath?: string,
  pluginIds?: string[],
  timing?: QuickstartSetupTiming,
  phasePrefix = 'create-space-settings',
): Promise<void> {
  let existingSettings: SpaceSettings | undefined
  const existing = await timeQuickstartPhase(
    timing,
    phasePrefix + '-get-object',
    () => spaceWorld.getObject(SPACE_SETTINGS_OBJECT_KEY, abortSignal),
  )
  try {
    if (existing) {
      try {
        existingSettings = await timeQuickstartPhase(
          timing,
          phasePrefix + '-read-existing',
          async () => {
            using cursor = await existing.accessWorldState(
              undefined,
              abortSignal,
            )
            const blockResp = await cursor.unmarshal(
              { blockType: SPACE_SETTINGS_BLOCK_TYPE },
              abortSignal,
            )
            if (blockResp.found && blockResp.data?.length) {
              const settings = SpaceSettings.fromBinary(blockResp.data)
              return {
                ...settings,
                // A corrupted settings block can decode arbitrary bytes into
                // pluginIds; never re-persist invalid manifest IDs.
                pluginIds: (settings.pluginIds ?? []).filter(
                  isValidSpacePluginId,
                ),
              }
            }
            return undefined
          },
        )
      } catch {
        existingSettings = undefined
      }
    }

    const requestedPluginIds = pluginIds ?? []
    const invalidPluginId = requestedPluginIds.find(
      (pluginId) => !isValidSpacePluginId(pluginId),
    )
    if (invalidPluginId !== undefined) {
      throw new Error(
        `quickstart returned invalid plugin id with length ${invalidPluginId.length}`,
      )
    }
    const mergedPluginIds = Array.from(
      new Set([...(existingSettings?.pluginIds ?? []), ...requestedPluginIds]),
    )
    const settings: SpaceSettings = {
      indexPath: indexPath ?? existingSettings?.indexPath ?? '',
      pluginIds: mergedPluginIds,
    }
    await applyQuickstartWorldOp(
      spaceWorld,
      SET_SPACE_SETTINGS_OP_ID,
      SetSpaceSettingsOp.toBinary({
        objectKey: SPACE_SETTINGS_OBJECT_KEY,
        settings,
        overwrite: true,
        timestamp: new Date(),
      }),
      '',
      abortSignal,
      timing,
      phasePrefix,
    )
  } finally {
    existing?.release()
  }
}

export async function ensureSpacePlugins(
  spaceWorld: IWorldState,
  pluginIds: string[],
  indexPath?: string,
  abortSignal?: AbortSignal,
): Promise<void> {
  await createSpaceSettingsObject(spaceWorld, abortSignal, indexPath, pluginIds)
}

export async function executeDynamicQuickstart(
  root: Root,
  quickstartId: string,
  setup: QuickstartSetup,
  abortSignal?: AbortSignal,
): Promise<void> {
  const registry = new QuickstartRegistryResourceServiceClient(root.client)
  const resp = await registry.ExecuteQuickstart(
    {
      quickstartId,
      spaceResourceId: setup.space.id,
    },
    abortSignal,
  )
  const pluginIds = resp.pluginIds ?? []
  if (resp.indexPath || pluginIds.length) {
    await ensureSpacePlugins(
      setup.spaceWorld,
      pluginIds,
      resp.indexPath || undefined,
      abortSignal,
    )
  }
}

function hasQuickstartRegistration(
  resp: { registrations?: WatchQuickstartsResponse['registrations'] },
  quickstartId: string,
  pluginId: string,
): boolean {
  return (resp.registrations ?? []).some(
    (reg) =>
      reg.quickstartId === quickstartId && (reg.pluginId ?? '') === pluginId,
  )
}

async function waitForQuickstartRegistration(
  root: Root,
  quickstartId: string,
  pluginId: string,
  abortSignal?: AbortSignal,
): Promise<void> {
  const registry = new QuickstartRegistryResourceServiceClient(root.client)
  const list = await registry.ListQuickstarts({}, abortSignal)
  if (hasQuickstartRegistration(list, quickstartId, pluginId)) return

  const timeoutSignal = AbortSignal.timeout(QUICKSTART_REGISTRATION_TIMEOUT_MS)
  const signal = abortSignal
    ? AbortSignal.any([abortSignal, timeoutSignal])
    : timeoutSignal
  try {
    const stream = registry.WatchQuickstarts({}, signal)
    for await (const resp of stream) {
      if (hasQuickstartRegistration(resp, quickstartId, pluginId)) return
    }
  } catch (err) {
    if (timeoutSignal.aborted && !abortSignal?.aborted) {
      throw new Error(
        'Timed out waiting for quickstart registration: ' + quickstartId,
        { cause: err },
      )
    }
    throw err
  }
  throw new Error(
    'Quickstart registration stream ended before registration: ' + quickstartId,
  )
}

// initUnixFS initializes a UnixFS filesystem with starter content.
export async function initUnixFS(
  spaceWorld: EngineWorldState,
  abortSignal?: AbortSignal,
  timing?: QuickstartSetupTiming,
): Promise<void> {
  // Create the InitUnixFSOp operation
  const op: InitUnixFSOp = {
    objectKey: UNIXFS_OBJECT_KEY,
    timestamp: new Date(),
  }

  // Apply the operation using ApplyWorldOp
  const opData = InitUnixFSOp.toBinary(op)
  await applyQuickstartWorldOp(
    spaceWorld,
    INIT_UNIXFS_OP_ID,
    opData,
    '',
    abortSignal,
    timing,
    'init-drive-unixfs',
  )
}

// initObjectLayout initializes an ObjectLayout with starter content.
export async function initObjectLayout(
  spaceWorld: EngineWorldState,
  abortSignal?: AbortSignal,
): Promise<void> {
  // Create the InitObjectLayoutOp operation
  const op: InitObjectLayoutOp = {
    objectKey: OBJECT_LAYOUT_OBJECT_KEY,
    timestamp: new Date(),
  }

  // Apply the operation using ApplyWorldOp
  const opData = InitObjectLayoutOp.toBinary(op)
  await spaceWorld.applyWorldOp(
    INIT_OBJECT_LAYOUT_OP_ID,
    opData,
    '',
    abortSignal,
  )
}

// initCanvasDemo initializes a Canvas with demo content.
export async function initCanvasDemo(
  spaceWorld: EngineWorldState,
  abortSignal?: AbortSignal,
): Promise<void> {
  const op: InitCanvasDemoOp = {
    objectKey: CANVAS_DEMO_OBJECT_KEY,
    timestamp: new Date(),
  }
  const opData = InitCanvasDemoOp.toBinary(op)
  await spaceWorld.applyWorldOp(INIT_CANVAS_DEMO_OP_ID, opData, '', abortSignal)
}

// populateSpace populates the space based on the quickstart type.
export async function populateSpace(
  quickstartId: QuickstartSpaceCreateId,
  setup: QuickstartSetup,
  abortSignal?: AbortSignal,
  timing?: QuickstartSetupTiming,
): Promise<void> {
  switch (quickstartId) {
    case 'space':
      await createSpaceSettingsObject(setup.spaceWorld, abortSignal)
      break
    case 'drive':
      await createDrive(setup.spaceWorld, abortSignal, timing)
      break
    case 'git':
      await initGitQuickstart(setup, abortSignal)
      break
    case 'notebook':
      await initNotesQuickstart(setup, quickstartId, abortSignal)
      break
    case 'canvas':
      await initUnixFS(setup.spaceWorld, abortSignal)
      await initCanvasDemo(setup.spaceWorld, abortSignal)
      await createSpaceSettingsObject(
        setup.spaceWorld,
        abortSignal,
        CANVAS_DEMO_OBJECT_KEY,
      )
      break
    case 'chat':
      await initChatQuickstart(setup.spaceWorld, abortSignal)
      break
    case 'docs':
      await initNotesQuickstart(setup, quickstartId, abortSignal)
      break
    case 'blog':
      await initNotesQuickstart(setup, quickstartId, abortSignal)
      break
    case 'v86':
      await initV86Quickstart(setup, abortSignal)
      break
    case 'forge':
      await initForgeQuickstart(setup, abortSignal)
      break
    default: {
      const _exhaustive: never = quickstartId
      throw new Error('Unknown quickstart ID: ' + String(_exhaustive))
    }
  }
}

function isNotesQuickstartId(
  quickstartId: QuickstartSpaceCreateId,
): quickstartId is NotesQuickstartId {
  return (
    quickstartId === 'notebook' ||
    quickstartId === 'docs' ||
    quickstartId === 'blog'
  )
}

async function initNotesQuickstart(
  setup: QuickstartSetup,
  quickstartId: QuickstartSpaceCreateId,
  abortSignal?: AbortSignal,
): Promise<void> {
  if (!isNotesQuickstartId(quickstartId)) {
    throw new Error('Unknown notes quickstart ID: ' + quickstartId)
  }
  if (!setup.root) {
    throw new Error('Root resource is required for Notes quickstart seeding')
  }
  await prepareNotesQuickstart(setup.root, setup, quickstartId, abortSignal)
  await executeDynamicQuickstart(setup.root, quickstartId, setup, abortSignal)
}

async function prepareNotesQuickstart(
  root: Root,
  setup: QuickstartSetup,
  quickstartId: NotesQuickstartId,
  abortSignal?: AbortSignal,
): Promise<void> {
  await ensureSpacePlugins(
    setup.spaceWorld,
    [NOTES_PLUGIN_ID],
    undefined,
    abortSignal,
  )
  await waitForQuickstartRegistration(
    root,
    quickstartId,
    NOTES_PLUGIN_ID,
    abortSignal,
  )
}

// createDrive sets up a drive with UnixFS content.
export async function createDrive(
  spaceWorld: EngineWorldState,
  abortSignal?: AbortSignal,
  timing?: QuickstartSetupTiming,
): Promise<void> {
  await timeQuickstartPhase(timing, 'init-drive-unixfs', () =>
    initUnixFS(spaceWorld, abortSignal, timing),
  )
  await timeQuickstartPhase(timing, 'create-drive-settings', () =>
    createSpaceSettingsObject(
      spaceWorld,
      abortSignal,
      UNIXFS_OBJECT_KEY,
      undefined,
      timing,
      'create-drive-settings',
    ),
  )
}

async function applyQuickstartWorldOp(
  spaceWorld: IWorldState,
  opTypeId: string,
  opData: Uint8Array,
  sender: string,
  abortSignal: AbortSignal | undefined,
  timing: QuickstartSetupTiming | undefined,
  phasePrefix: string,
): Promise<void> {
  type TimedWorldTx = {
    applyWorldOp(
      opTypeId: string,
      opData: Uint8Array,
      sender: string,
      abortSignal?: AbortSignal,
    ): Promise<unknown>
    commit(abortSignal?: AbortSignal): Promise<void>
    discard(abortSignal?: AbortSignal): Promise<void>
  }
  type TimedWorldEngine = {
    newTransaction(
      write: boolean,
      abortSignal?: AbortSignal,
    ): Promise<TimedWorldTx>
  }

  const getEngine = (spaceWorld as { getEngine?: () => TimedWorldEngine })
    .getEngine
  if (!timing || typeof getEngine !== 'function') {
    await spaceWorld.applyWorldOp(opTypeId, opData, sender, abortSignal)
    return
  }
  const engine = getEngine.call(spaceWorld)

  const tx = await timeQuickstartPhase(
    timing,
    phasePrefix + '-new-transaction',
    () => engine.newTransaction(true, abortSignal),
  )
  try {
    await timeQuickstartPhase(timing, phasePrefix + '-apply-op', () =>
      tx.applyWorldOp(opTypeId, opData, sender, abortSignal),
    )
    await timeQuickstartPhase(timing, phasePrefix + '-commit', () =>
      tx.commit(abortSignal),
    )
  } finally {
    await timeQuickstartPhase(timing, phasePrefix + '-discard', () =>
      tx.discard(abortSignal).catch(() => undefined),
    )
  }
}

// initGitQuickstart seeds a persistent git/repo wizard and indexes the Space to it.
async function initGitQuickstart(
  setup: QuickstartSetup,
  abortSignal?: AbortSignal,
): Promise<void> {
  const now = new Date()
  const wizardKey = buildWizardObjectKey(
    'Repository ' + now.getTime().toString(36),
  )
  const op: CreateWizardObjectOp = {
    objectKey: wizardKey,
    wizardTypeId: 'wizard/git/repo',
    targetTypeId: 'git/repo',
    targetKeyPrefix: 'git/repo/',
    name: 'Repository',
    timestamp: now,
  }
  const opData = CreateWizardObjectOp.toBinary(op)
  await setup.spaceWorld.applyWorldOp(
    CREATE_WIZARD_OBJECT_OP_ID,
    opData,
    '',
    abortSignal,
  )
  await createSpaceSettingsObject(setup.spaceWorld, abortSignal, wizardKey)
}

// initChatQuickstart creates a chat channel in the space.
async function initChatQuickstart(
  spaceWorld: EngineWorldState,
  abortSignal?: AbortSignal,
): Promise<void> {
  const op: InitChatDemoOp = {
    channelObjectKey: CHAT_DEMO_CHANNEL_KEY,
    timestamp: new Date(),
  }
  const opData = InitChatDemoOp.toBinary(op)
  await spaceWorld.applyWorldOp(INIT_CHAT_DEMO_OP_ID, opData, '', abortSignal)
  await createSpaceSettingsObject(
    spaceWorld,
    abortSignal,
    CHAT_DEMO_CHANNEL_KEY,
  )
}

// initV86Quickstart seeds a persistent v86 wizard and indexes the Space to it.
async function initV86Quickstart(
  setup: QuickstartSetup,
  abortSignal?: AbortSignal,
): Promise<void> {
  const now = new Date()
  const wizardKey = buildV86QuickstartWizardKey(now)
  const cfg = buildV86QuickstartWizardConfig()
  const op: CreateWizardObjectOp = {
    objectKey: wizardKey,
    wizardTypeId: V86_WIZARD_TYPE_ID,
    targetTypeId: V86_WIZARD_TARGET_TYPE_ID,
    targetKeyPrefix: V86_WIZARD_TARGET_KEY_PREFIX,
    name: '',
    timestamp: now,
    initialStep: 1,
    initialConfigData: V86WizardConfig.toBinary(cfg),
  }
  const opData = CreateWizardObjectOp.toBinary(op)
  await setup.spaceWorld.applyWorldOp(
    CREATE_WIZARD_OBJECT_OP_ID,
    opData,
    '',
    abortSignal,
  )
  await createSpaceSettingsObject(setup.spaceWorld, abortSignal, wizardKey)
}

// initForgeQuickstart creates a complete Forge environment in the space:
// ObjectLayout with dashboard tab, cluster, sample job with tasks, and a
// worker registered to the creating session.
async function initForgeQuickstart(
  setup: QuickstartSetup,
  abortSignal?: AbortSignal,
): Promise<void> {
  const layoutKey = 'forge'

  // Get the session peer ID for worker registration.
  const sessionInfo = await setup.session.getSessionInfo(abortSignal)
  const sessionPeerId = sessionInfo.peerId ?? ''

  const op: InitForgeQuickstartOp = {
    layoutKey,
    dashboardKey: 'dashboard',
    clusterKey: 'cluster',
    clusterName: 'default',
    workerKey: 'session-worker',
    sessionPeerId,
    timestamp: new Date(),
  }
  const opData = InitForgeQuickstartOp.toBinary(op)
  await setup.spaceWorld.applyWorldOp(
    INIT_FORGE_QUICKSTART_OP_ID,
    opData,
    sessionPeerId,
    abortSignal,
  )
  await createSpaceSettingsObject(setup.spaceWorld, abortSignal, layoutKey)
}
