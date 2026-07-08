import type { RegisterCleanup } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
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
import { SUBPATH_DELIMITER } from '@s4wave/sdk/space/object-uri.js'
import { Engine } from '@s4wave/sdk/world/engine.js'
import { EngineWorldState } from '@s4wave/sdk/world/engine-state.js'
import { createWorldObject } from '@s4wave/sdk/world/utils.js'
import { setObjectType } from '@s4wave/sdk/world/types/types.js'
import { KvStore, KvStoreTypeID } from '@s4wave/sdk/kv/index.js'
import {
  SqlDatabase,
  SqlDbTypeID,
  SqlQuery,
  SqlQueryBlockTypeID,
  SqlQueryTypeID,
} from '@s4wave/sdk/sql/index.js'
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
import { FSHandle } from '@s4wave/sdk/unixfs/index.js'
import { UnixFSTypeID } from '@s4wave/sdk/unixfs/type.js'
import {
  INIT_OBJECT_LAYOUT_OP_ID,
  OBJECT_LAYOUT_OBJECT_KEY,
} from '@s4wave/core/space/world/ops/init-object-layout.js'
import { ObjectLayoutTypeID } from '@s4wave/sdk/layout/world/object-layout.js'
import {
  INIT_CANVAS_DEMO_OP_ID,
  CANVAS_DEMO_OBJECT_KEY,
} from '@s4wave/core/space/world/ops/init-canvas-demo.js'
import { InitCanvasDemoOp } from '@s4wave/core/space/world/ops/ops.pb.js'
import { CanvasTypeID } from '@s4wave/app/canvas/type.js'

import { InitChatDemoOp } from '@s4wave/sdk/chat/chat.pb.js'
import {
  INIT_CHAT_DEMO_OP_ID,
  CHAT_DEMO_CHANNEL_KEY,
} from '@s4wave/sdk/chat/init-chat-demo.js'
import { DeviceTypeID } from '@s4wave/sdk/device/device.js'
import { CreateComputersDashboardOp } from '@s4wave/sdk/device/device.pb.js'
import { CREATE_COMPUTERS_DASHBOARD_OP_ID } from '@s4wave/sdk/device/computers/create-computers-dashboard.js'
import { Query } from '@s4wave/sdk/sql/query/query.pb.js'
import { CreateVmV86Op, SetV86StateOp, VmState } from '@s4wave/sdk/vm/v86.pb.js'
import { CREATE_VM_V86_OP_ID } from '@s4wave/sdk/vm/create-vm-v86.js'
import {
  CreateWizardObjectOp,
  IntroWizardConfig,
} from '@s4wave/sdk/world/wizard/wizard.pb.js'
import { CREATE_WIZARD_OBJECT_OP_ID } from '@s4wave/sdk/world/wizard/create-wizard.js'
import {
  IntroWizardTypeID,
  driveIntroConfig,
} from '@s4wave/app/wizard/intro.js'
import { InitForgeQuickstartOp } from '@s4wave/core/forge/dashboard/dashboard.pb.js'
import { INIT_FORGE_QUICKSTART_OP_ID } from '@s4wave/sdk/forge/dashboard/init-forge-quickstart.js'
import { markInteracted } from '@s4wave/web/state/interaction.js'
import { mountSpace } from '@s4wave/app/space/space.js'
import {
  buildObjectKey,
  buildWizardObjectKey,
} from '@s4wave/app/space/create-op-builders.js'
import {
  DEFAULT_V86_MEMORY_MB,
  DEFAULT_V86_VGA_MEMORY_MB,
  V86_DEFAULT_CDN_IMAGE_OBJECT_KEY,
  V86_USER_IMAGE_OBJECT_KEY,
} from '@s4wave/app/vm/v86-wizard-config.js'
import {
  AddDeviceDefaultName,
  AddDeviceWizardTargetKeyPrefix,
  AddDeviceWizardTypeID,
} from '@s4wave/app/device/add-device-wizard.js'

import { type QuickstartSpaceCreateId } from './options.js'
import { markQuickstartStartupBoundary } from './startup-boundary.js'

const NOTES_PLUGIN_ID = 'spacewave-notes'
const V86_PLUGIN_ID = 'spacewave-v86'
const SET_V86_STATE_OP_ID = 'vm/v86/set-state'
const QUICKSTART_REGISTRATION_TIMEOUT_MS = 120000
const QUICKSTART_LOCAL_PROVIDER_READY_TIMEOUT_MS = 120000
const QUICKSTART_CREATE_LOCAL_ACCOUNT_TIMEOUT_MS = import.meta.env?.DEV
  ? 240000
  : 90000
const QUICKSTART_RECOVER_LOCAL_SESSION_TIMEOUT_MS = import.meta.env?.DEV
  ? 60000
  : 15000
const KV_QUICKSTART_STORE_KEY = 'kv/store'
const SQL_QUICKSTART_DB_KEY = 'sql/db'
const SQL_QUICKSTART_QUERY_KEY = 'sql/query/example'
const DRIVE_STARTER_GUIDE_NAME = 'getting-started.md'
const DEVICE_QUICKSTART_DASHBOARD_KEY = 'computers'
const DRIVE_STARTER_GUIDE_CONTENT = `# Getting Started

Welcome to your new drive! This starter guide is written by the Drive
Quickstart after the generic UnixFS filesystem is initialized.

## Next steps

Try uploading a few files and opening them here. Video files are the best ones
to try first.
`

type NotesQuickstartId = Extract<
  QuickstartSpaceCreateId,
  'notebook' | 'docs' | 'blog'
>

type QuickstartResourceHandle = {
  release(): void
}

type QuickstartResourceConstructor<T extends QuickstartResourceHandle> = new (
  resourceRef: ClientResourceRef,
) => T

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
  markQuickstartStartupBoundary('quickstart.started', {
    quickstartId,
  })
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
    markQuickstartStartupBoundary('quickstart.content-ready', {
      quickstartId: timing.quickstartId,
      state: timing.state,
      elapsedMs: timing.elapsedMs,
    })
  }
  markQuickstartStartupBoundary('quickstart.finished', {
    quickstartId: timing.quickstartId,
    state: timing.state,
    elapsedMs: timing.elapsedMs,
    error: timing.error ?? null,
  })
  publishQuickstartTiming(timing)
  if (globalThis.__s4waveLogQuickstartTiming) {
    console.log('quickstart timing: ' + JSON.stringify(timing))
  }
}

function markQuickstartProgressReady(timing: QuickstartSetupTiming): void {
  if (typeof timing.progressReadyMs === 'number') return
  timing.progressReadyMs = nowMs()
  timing.state = 'progress-ready'
  markQuickstartStartupBoundary('quickstart.progress-ready', {
    quickstartId: timing.quickstartId,
  })
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
      // eslint-disable-next-line react-doctor/async-await-in-loop -- retry attempts are sequential by contract.
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
    case 'device':
      return 'My Computers'
    case 'forge':
      return 'My Forge Dashboard'
    case 'kv':
      return 'My Key-Value Store'
    case 'sql':
      return 'My SQL Database'
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

  return {
    space,
    ...(spaceContents ? { spaceContents } : {}),
    spaceWorld,
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

    // Populate the space with quickstart-specific content.
    reportQuickstartProgress(
      progress,
      'content',
      'Seeding ' + getQuickstartSpaceName(quickstartId) + ' content',
    )
    await timeQuickstartPhase(timing, 'populate-space', () =>
      populateSpace(quickstartId, result, abortSignal, timing),
    )

    markQuickstartProgressReady(timing)

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
      return ''
    case 'canvas':
      return CANVAS_DEMO_OBJECT_KEY
    case 'chat':
      return CHAT_DEMO_CHANNEL_KEY
    case 'forge':
      return 'forge'
    case 'kv':
      return KV_QUICKSTART_STORE_KEY
    case 'sql':
      return SQL_QUICKSTART_DB_KEY
    case 'device':
      return ''
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
      return ''
    case 'canvas':
      return CanvasTypeID
    case 'kv':
      return KvStoreTypeID
    case 'sql':
      return SqlDbTypeID
    case 'forge':
      return ObjectLayoutTypeID
    case 'space':
    case 'git':
    case 'notebook':
    case 'docs':
    case 'blog':
    case 'chat':
    case 'v86':
    case 'device':
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
  return basePath.replace(/\/+$/, '') + SUBPATH_DELIMITER + objectKey
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

// initUnixFS initializes an empty UnixFS filesystem.
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

async function writeDriveStarterGuide(
  spaceWorld: EngineWorldState,
  abortSignal?: AbortSignal,
  timing?: QuickstartSetupTiming,
): Promise<void> {
  const access = await timeQuickstartPhase(
    timing,
    'write-drive-starter-guide-access',
    () => spaceWorld.accessTypedObject(UNIXFS_OBJECT_KEY, abortSignal),
  )
  if (!access.resourceId || access.typeId !== UnixFSTypeID) {
    throw new Error(
      `Drive starter guide expected ${UnixFSTypeID}, got ${access.typeId || 'unknown'}`,
    )
  }

  const ref = spaceWorld.getResourceRef().createRef(access.resourceId)
  const root = new FSHandle(ref)
  try {
    const data = new TextEncoder().encode(DRIVE_STARTER_GUIDE_CONTENT)
    await timeQuickstartPhase(timing, 'write-drive-starter-guide-upload', () =>
      root.uploadFile(
        DRIVE_STARTER_GUIDE_NAME,
        BigInt(data.byteLength),
        new ReadableStream<Uint8Array>({
          start(controller) {
            controller.enqueue(data)
            controller.close()
          },
        }),
        0o644,
        undefined,
        abortSignal,
      ),
    )
  } finally {
    root.release()
  }
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
    case 'kv':
      await initKvQuickstart(setup.spaceWorld, abortSignal)
      break
    case 'sql':
      await initSqlQuickstart(setup.spaceWorld, abortSignal)
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
    case 'device':
      await initDeviceQuickstart(setup, abortSignal, timing)
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

async function createEmptyTypedObject(
  spaceWorld: IWorldState,
  objectKey: string,
  typeId: string,
  abortSignal?: AbortSignal,
): Promise<void> {
  const objectState = await spaceWorld.createObject(objectKey, {}, abortSignal)
  try {
    await setObjectType(spaceWorld, objectKey, typeId, abortSignal)
  } finally {
    objectState.release()
  }
}

async function createSeededTypedObject(
  spaceWorld: EngineWorldState,
  objectKey: string,
  typeId: string,
  blockTypeId: string,
  data: Uint8Array,
  abortSignal?: AbortSignal,
): Promise<void> {
  using worldCursor = await spaceWorld.buildStorageCursor(abortSignal)
  const created = await createWorldObject(
    spaceWorld,
    worldCursor,
    objectKey,
    (cursor) =>
      cursor.setBlock(
        { data, markDirty: true, blockType: blockTypeId },
        abortSignal,
      ),
    abortSignal,
  )
  try {
    await setObjectType(spaceWorld, objectKey, typeId, abortSignal)
  } finally {
    created.objectState.release()
  }
}

async function openQuickstartHandle<T extends QuickstartResourceHandle>(
  spaceWorld: IWorldState,
  objectKey: string,
  typeId: string,
  ResourceCtor: QuickstartResourceConstructor<T>,
  abortSignal?: AbortSignal,
): Promise<T> {
  const access = await spaceWorld.accessTypedObject(objectKey, abortSignal)
  if (!access.resourceId || access.typeId !== typeId) {
    throw new Error(
      `quickstart expected ${typeId} at ${objectKey}, got ${access.typeId || 'unknown'}`,
    )
  }
  return new ResourceCtor(
    spaceWorld.getResourceRef().createRef(access.resourceId),
  )
}

async function initKvQuickstart(
  spaceWorld: EngineWorldState,
  abortSignal?: AbortSignal,
): Promise<void> {
  await createEmptyTypedObject(
    spaceWorld,
    KV_QUICKSTART_STORE_KEY,
    KvStoreTypeID,
    abortSignal,
  )
  const store = await openQuickstartHandle(
    spaceWorld,
    KV_QUICKSTART_STORE_KEY,
    KvStoreTypeID,
    KvStore,
    abortSignal,
  )
  try {
    const encoder = new TextEncoder()
    await store.withTransaction(
      true,
      async (tx) => {
        // eslint-disable-next-line react-doctor/async-parallel -- transaction writes stay ordered on one transaction owner.
        await tx.set(encoder.encode('hello'), encoder.encode('world'))
        await tx.set(
          encoder.encode('profile.json'),
          encoder.encode(
            JSON.stringify({
              name: 'Ada Lovelace',
              role: 'analyst',
              active: true,
            }),
          ),
        )
        await tx.set(
          encoder.encode('binary/blob'),
          new Uint8Array([0, 1, 2, 3, 5, 8, 13]),
        )
      },
      abortSignal,
    )
  } finally {
    store.release()
  }
  await createSpaceSettingsObject(
    spaceWorld,
    abortSignal,
    KV_QUICKSTART_STORE_KEY,
  )
}

async function initSqlQuickstart(
  spaceWorld: EngineWorldState,
  abortSignal?: AbortSignal,
): Promise<void> {
  await createEmptyTypedObject(
    spaceWorld,
    SQL_QUICKSTART_DB_KEY,
    SqlDbTypeID,
    abortSignal,
  )
  const db = await openQuickstartHandle(
    spaceWorld,
    SQL_QUICKSTART_DB_KEY,
    SqlDbTypeID,
    SqlDatabase,
    abortSignal,
  )
  try {
    await db.withTransaction(
      true,
      '',
      async (tx) => {
        await tx.exec('CREATE DATABASE quickstart')
      },
      abortSignal,
    )
    await db.withTransaction(
      true,
      '/quickstart',
      async (tx) => {
        await tx.exec(
          'CREATE TABLE people (id BIGINT NOT NULL PRIMARY KEY, name TEXT NOT NULL, role TEXT NOT NULL)',
        )
        await tx.exec(
          "INSERT INTO people (id, name, role) VALUES (1, 'ada', 'analyst')",
        )
        await tx.exec(
          "INSERT INTO people (id, name, role) VALUES (2, 'grace', 'engineer')",
        )
        await tx.exec(
          'CREATE TABLE projects (id BIGINT NOT NULL PRIMARY KEY, owner_id BIGINT NOT NULL, title TEXT NOT NULL)',
        )
        await tx.exec(
          "INSERT INTO projects (id, owner_id, title) VALUES (10, 1, 'difference engine notes')",
        )
        await tx.exec(
          "INSERT INTO projects (id, owner_id, title) VALUES (11, 2, 'compiler logbook')",
        )
      },
      abortSignal,
    )
  } finally {
    db.release()
  }

  await createSeededTypedObject(
    spaceWorld,
    SQL_QUICKSTART_QUERY_KEY,
    SqlQueryTypeID,
    SqlQueryBlockTypeID,
    Query.toBinary({}),
    abortSignal,
  )
  const query = await openQuickstartHandle(
    spaceWorld,
    SQL_QUICKSTART_QUERY_KEY,
    SqlQueryTypeID,
    SqlQuery,
    abortSignal,
  )
  try {
    await query.setQueryText(
      'SELECT name, role FROM quickstart.people WHERE id = ?',
      'mysql',
      SQL_QUICKSTART_DB_KEY,
      abortSignal,
    )
    await query.setParameters(
      [{ value: { case: 'intValue', value: 1n } }],
      abortSignal,
    )
  } finally {
    query.release()
  }
  await createSpaceSettingsObject(
    spaceWorld,
    abortSignal,
    SQL_QUICKSTART_DB_KEY,
  )
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
  await writeDriveStarterGuide(spaceWorld, abortSignal, timing)
  // Wrap the raw files object in the new-user intro: index the Space at a
  // wizard/intro object carrying the introduced key and the Drive intro
  // content. Finishing the intro sets the index to UNIXFS_OBJECT_KEY.
  const introKey = buildWizardObjectKey('Welcome')
  await applyQuickstartWorldOp(
    spaceWorld,
    CREATE_WIZARD_OBJECT_OP_ID,
    CreateWizardObjectOp.toBinary({
      objectKey: introKey,
      wizardTypeId: IntroWizardTypeID,
      targetTypeId: UnixFSTypeID,
      targetKeyPrefix: UNIXFS_OBJECT_KEY,
      name: 'Welcome',
      timestamp: new Date(),
      initialConfigData: IntroWizardConfig.toBinary(driveIntroConfig()),
    }),
    '',
    abortSignal,
    timing,
    'create-drive-intro-wizard',
  )
  await timeQuickstartPhase(timing, 'create-drive-settings', () =>
    createSpaceSettingsObject(
      spaceWorld,
      abortSignal,
      introKey,
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

// initV86Quickstart creates a VM from the default CDN image and requests boot.
async function initV86Quickstart(
  setup: QuickstartSetup,
  abortSignal?: AbortSignal,
): Promise<void> {
  if (!setup.root) {
    throw new Error('Root resource is required for V86 quickstart seeding')
  }
  const spaceId = setup.spaceResp.sharedObjectRef?.providerResourceRef?.id ?? ''
  if (!spaceId) {
    throw new Error('Space id is required for V86 CDN image copy')
  }
  const sessionIndex = setup.sessionIndex ?? 0
  if (sessionIndex === 0) {
    throw new Error('Session index is required for V86 CDN image copy')
  }
  const now = new Date()
  const vmKey = buildObjectKey(
    'vm/v86/',
    'V86 VM ' + now.getTime().toString(36),
  )
  const { cdn } = await setup.root.getCdn('', abortSignal)
  using cdnHandle = cdn
  await cdnHandle.copyV86ImageToSpace(
    sessionIndex,
    spaceId,
    V86_DEFAULT_CDN_IMAGE_OBJECT_KEY,
    V86_USER_IMAGE_OBJECT_KEY,
    abortSignal,
  )

  const createOp: CreateVmV86Op = {
    objectKey: vmKey,
    name: 'V86 VM',
    timestamp: now,
    imageObjectKey: V86_USER_IMAGE_OBJECT_KEY,
    config: {
      memoryMb: DEFAULT_V86_MEMORY_MB,
      vgaMemoryMb: DEFAULT_V86_VGA_MEMORY_MB,
      networking: false,
      serialEnabled: true,
      bootArgs: '',
      mounts: [],
    },
  }
  await setup.spaceWorld.applyWorldOp(
    CREATE_VM_V86_OP_ID,
    CreateVmV86Op.toBinary(createOp),
    '',
    abortSignal,
  )
  await createSpaceSettingsObject(setup.spaceWorld, abortSignal, vmKey, [
    V86_PLUGIN_ID,
  ])
  if (setup.spaceContents) {
    await setup.spaceContents.setProcessBinding(
      vmKey,
      'vm/v86',
      true,
      abortSignal,
    )
  }
  await setup.spaceWorld.applyWorldOp(
    SET_V86_STATE_OP_ID,
    SetV86StateOp.toBinary({
      objectKey: vmKey,
      state: VmState.VmState_STARTING,
      errorMessage: '',
    }),
    '',
    abortSignal,
  )
}

async function initDeviceQuickstart(
  setup: QuickstartSetup,
  abortSignal?: AbortSignal,
  timing?: QuickstartSetupTiming,
): Promise<void> {
  const now = new Date()
  const wizardKey = buildWizardObjectKey(
    'Add Device ' + now.getTime().toString(36),
  )
  // eslint-disable-next-line react-doctor/async-parallel -- quickstart world ops stay ordered so routing observes the dashboard before the wizard.
  await applyQuickstartWorldOp(
    setup.spaceWorld,
    CREATE_COMPUTERS_DASHBOARD_OP_ID,
    CreateComputersDashboardOp.toBinary({
      objectKey: DEVICE_QUICKSTART_DASHBOARD_KEY,
      name: 'Computers',
      timestamp: now,
    }),
    '',
    abortSignal,
    timing,
    'create-device-dashboard',
  )
  await applyQuickstartWorldOp(
    setup.spaceWorld,
    CREATE_WIZARD_OBJECT_OP_ID,
    CreateWizardObjectOp.toBinary({
      objectKey: wizardKey,
      wizardTypeId: AddDeviceWizardTypeID,
      targetTypeId: DeviceTypeID,
      targetKeyPrefix: AddDeviceWizardTargetKeyPrefix,
      name: AddDeviceDefaultName,
      timestamp: now,
    }),
    '',
    abortSignal,
    timing,
    'create-device-wizard',
  )
  await createSpaceSettingsObject(
    setup.spaceWorld,
    abortSignal,
    DEVICE_QUICKSTART_DASHBOARD_KEY,
    undefined,
    timing,
    'create-device-settings',
  )
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
