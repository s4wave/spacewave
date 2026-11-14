import type { RegisterCleanup } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import type { Root } from '@s4wave/sdk/root'
import type { Session } from '@s4wave/sdk/session'
import type { CreateSpaceResponse } from '@s4wave/sdk/session/session.pb.js'
import type { CreateAccountResponse } from '@s4wave/sdk/provider/local/local.pb.js'
import type { WatchQuickstartsResponse } from '@s4wave/sdk/quickstart/registry/registry.pb.js'
import { QuickstartRegistryResourceServiceClient } from '@s4wave/sdk/quickstart/registry/registry_srpc.pb.js'
import { ObjectTypeRegistryResourceServiceClient } from '@s4wave/sdk/objecttype/registry/registry_srpc.pb.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import { LocalProvider } from '@s4wave/sdk/provider/local/local.js'
import { Space } from '@s4wave/sdk/space/space.js'
import type { SpaceSoListEntry } from '@s4wave/core/space/space.pb.js'
import { SpaceContents } from '@s4wave/sdk/space/contents.js'
import { SUBPATH_DELIMITER } from '@s4wave/sdk/space/object-uri.js'
import { Engine } from '@s4wave/sdk/world/engine.js'
import { EngineWorldState } from '@s4wave/sdk/world/engine-state.js'
import { setObjectType } from '@s4wave/sdk/world/types/types.js'
import { KvStore, KvStoreTypeID } from '@s4wave/sdk/kv/index.js'
import {
  isValidSpacePluginId,
  SPACE_SETTINGS_BLOCK_TYPE,
  SPACE_SETTINGS_OBJECT_KEY,
} from '@s4wave/core/space/world/world.js'
import { SpaceSettings } from '@s4wave/core/space/world/world.pb.js'
import {
  InitUnixFSOp,
  InitObjectLayoutOp,
  SetSpaceIndexPathOp,
  SetSpaceSettingsOp,
} from '@s4wave/core/space/world/ops/ops.pb.js'
import {
  SET_SPACE_INDEX_PATH_OP_ID,
  SET_SPACE_SETTINGS_OP_ID,
} from '@s4wave/core/space/world/ops/set-space-settings.js'
import { throwBatchError } from '@s4wave/sdk/world/errors.js'
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
import { CanvasTypeID } from '@s4wave/app/canvas/types.js'

import { InitChatDemoOp } from '@s4wave/sdk/chat/chat.pb.js'
import {
  INIT_CHAT_DEMO_OP_ID,
  CHAT_DEMO_CHANNEL_KEY,
} from '@s4wave/sdk/chat/init-chat-demo.js'
import { DeviceTypeID } from '@s4wave/sdk/device/device.js'
import { CreateComputersDashboardOp } from '@s4wave/sdk/device/device.pb.js'
import { CREATE_COMPUTERS_DASHBOARD_OP_ID } from '@s4wave/sdk/device/computers/create-computers-dashboard.js'
import { V86WizardConfig } from '@s4wave/sdk/vm/v86-wizard.pb.js'
import { VmV86TypeID } from '@s4wave/sdk/vm/vmv86.js'
import { CreateWizardObjectOp } from '@s4wave/sdk/world/wizard/wizard.pb.js'
import { CREATE_WIZARD_OBJECT_OP_ID } from '@s4wave/sdk/world/wizard/create-wizard.js'
import { InitForgeQuickstartOp } from '@s4wave/core/forge/dashboard/dashboard.pb.js'
import { INIT_FORGE_QUICKSTART_OP_ID } from '@s4wave/sdk/forge/dashboard/init-forge-quickstart.js'
import { getRootEnvironment } from '@s4wave/web/sdk/app/environment.js'
import { getDebugContext } from '@s4wave/sdk/debug/context.js'
import { markInteracted } from '@s4wave/web/state/interaction.js'
import { mountSpace } from '@s4wave/app/space/space.js'
import { wizardObjectKey } from '@s4wave/app/space/create-op-builders.js'
import {
  buildV86QuickstartWizardConfig,
  buildV86QuickstartWizardKey,
  V86_WIZARD_TARGET_KEY_PREFIX,
  V86_WIZARD_TYPE_ID,
} from '@s4wave/app/vm/v86-wizard-config.js'
import {
  AddDeviceDefaultName,
  AddDeviceWizardTargetKeyPrefix,
  AddDeviceWizardTypeID,
} from '@s4wave/app/device/add-device-wizard.js'

import { type QuickstartSpaceCreateId } from './options.js'
import { markQuickstartStartupBoundary } from './startup-boundary.js'
import { DRIVE_STARTER_GUIDE_NAME } from './drive-starter-guide.js'

const NOTES_PLUGIN_ID = 'spacewave-notes'
const V86_PLUGIN_ID = 'spacewave-v86'
const SQL_PLUGIN_ID = 'spacewave-sql'
const SQL_DB_TYPE_ID = 'sql/db'
const SQL_QUERY_TYPE_ID = 'sql/query'
const SQL_QUERY_RESULT_TYPE_ID = 'sql/query-result'
const SQL_SCHEMA_TYPE_ID = 'sql/schema'
const SQL_TABLE_VIEW_TYPE_ID = 'sql/table-view'
const SQL_WORKBENCH_TYPE_ID = 'sql/workbench'
const QUICKSTART_REGISTRATION_TIMEOUT_MS = import.meta.env?.DEV ? 240000 : 30000
const QUICKSTART_LOCAL_PROVIDER_READY_TIMEOUT_MS = 120000
const QUICKSTART_CREATE_LOCAL_ACCOUNT_TIMEOUT_MS = import.meta.env?.DEV
  ? 240000
  : 90000
const QUICKSTART_RECOVER_LOCAL_SESSION_TIMEOUT_MS = import.meta.env?.DEV
  ? 60000
  : 15000
const KV_QUICKSTART_STORE_KEY = 'kv/store'
const SQL_QUICKSTART_DB_KEY = 'sql/db'
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
  appId?: string
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
  appId: string,
): QuickstartSetupTiming {
  const timing: QuickstartSetupTiming = {
    quickstartId,
    appId,
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
  if (timing.appId) {
    try {
      getDebugContext(timing.appId).quickstartTiming = timing
    } catch {
      /* The frontend may still be mounting. */
    }
    return
  }
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

function hasStoredLocalSessionHint(root: Root): boolean {
  const localStorage = getRootEnvironment(root).storage
  if (typeof localStorage === 'undefined') return true
  return localStorage.getItem('spacewave-has-session') === '1'
}

// QUICKSTART_SEEDS is the per-quickstart descriptor table: the seeded space
// name plus the optional initial object key and type. Typing the Record
// against QuickstartSpaceCreateId keeps new quickstarts exhaustive.
const QUICKSTART_SEEDS: Record<
  QuickstartSpaceCreateId,
  { spaceName: string; initialObjectKey?: string; initialObjectType?: string }
> = {
  space: { spaceName: 'My Space' },
  drive: { spaceName: 'My Drive' },
  git: { spaceName: 'My Git Repository' },
  notebook: { spaceName: 'My Notebook' },
  canvas: {
    spaceName: 'My Canvas',
    initialObjectKey: CANVAS_DEMO_OBJECT_KEY,
    initialObjectType: CanvasTypeID,
  },
  chat: { spaceName: 'My Chat', initialObjectKey: CHAT_DEMO_CHANNEL_KEY },
  docs: { spaceName: 'My Docs' },
  blog: { spaceName: 'My Blog' },
  v86: { spaceName: 'My VM' },
  device: { spaceName: 'My Computers' },
  forge: {
    spaceName: 'My Forge Dashboard',
    initialObjectKey: 'forge',
    initialObjectType: ObjectLayoutTypeID,
  },
  kv: {
    spaceName: 'My Key-Value Store',
    initialObjectKey: KV_QUICKSTART_STORE_KEY,
    initialObjectType: KvStoreTypeID,
  },
  sql: {
    spaceName: 'My SQL Database',
    initialObjectKey: SQL_QUICKSTART_DB_KEY,
    initialObjectType: SQL_DB_TYPE_ID,
  },
}

export function getQuickstartSpaceName(
  quickstartId: QuickstartSpaceCreateId,
): string {
  return QUICKSTART_SEEDS[quickstartId].spaceName
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
  const hasLocalSessionHint = hasStoredLocalSessionHint(root)
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
      markInteracted(getRootEnvironment(root).storage)
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
        markInteracted(getRootEnvironment(root).storage)
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
      markInteracted(getRootEnvironment(root).storage)
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

  markInteracted(getRootEnvironment(root).storage)

  return { accountResp, sessionIndex, session }
}

// LocalSessionSetup is the result of creating or reusing a local session.
export interface LocalSessionSetup {
  accountResp?: CreateAccountResponse
  sessionIndex: number
  session: Session
}

// QuickstartSetup retains mounted resources and a newly seeded initial route.
export interface QuickstartSetup {
  root?: Root
  accountResp?: CreateAccountResponse
  sessionIndex?: number
  spaceResp: CreateSpaceResponse
  session: Session
  space: Space
  spaceContents?: SpaceContents
  spaceWorld: EngineWorldState
  initialObjectRoute?: { objectKey: string; objectType: string }
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

// findExistingQuickstartSpace returns the first space-list entry whose space
// name matches the quickstart's seeded space name, or undefined when no
// snapshot entry matches.
async function findExistingQuickstartSpace(
  session: Session,
  spaceName: string,
  abortSignal: AbortSignal,
): Promise<SpaceSoListEntry | undefined> {
  for await (const resp of session.watchResourcesList(
    { includeIndexObjectTypes: true },
    abortSignal,
  )) {
    return (resp.spacesList ?? []).find(
      (entry) =>
        entry.spaceMeta?.name === spaceName &&
        !!entry.entry?.ref?.providerResourceRef?.id,
    )
  }
  return undefined
}

export async function createQuickstartSetup(
  root: Root,
  quickstartId: QuickstartSpaceCreateId,
  abortSignal: AbortSignal,
  cleanup: RegisterCleanup,
  progress?: QuickstartProgressReporter,
): Promise<QuickstartSetup> {
  const timing = startQuickstartTiming(
    quickstartId,
    getRootEnvironment(root).id,
  )
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

    // Reuse the account's existing space for this quickstart; creating again
    // would duplicate it. A reused Space is ready only if its seed applied.
    const spaceName = getQuickstartSpaceName(quickstartId)
    reportQuickstartProgress(
      progress,
      'space',
      'Checking for an existing ' + spaceName,
    )
    const existingEntry = await timeQuickstartPhase(
      timing,
      'find-existing-space',
      () => findExistingQuickstartSpace(session, spaceName, abortSignal),
    )
    const sharedObjectRef = existingEntry?.entry?.ref

    let spaceResp: CreateSpaceResponse
    if (sharedObjectRef) {
      reportQuickstartProgress(progress, 'space', 'Opening ' + spaceName)
      spaceResp = { sharedObjectRef }
    } else {
      reportQuickstartProgress(progress, 'space', 'Creating ' + spaceName)
      spaceResp = await timeQuickstartPhase(timing, 'create-space', () =>
        session.createSpace({ spaceName }, abortSignal),
      )
    }

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

    const result: QuickstartSetup = {
      root,
      accountResp,
      sessionIndex,
      session,
      spaceResp,
      ...setup,
    }

    // A quickstart Space is absent, created but not seeded, or seeded. A
    // created Space whose seed failed part way is seeded again: each seed
    // skips the steps that landed, so repeating it is safe.
    const seeded =
      !!sharedObjectRef &&
      (await timeQuickstartPhase(timing, 'check-space-seeded', () =>
        isQuickstartSpaceSeeded(quickstartId, result.spaceWorld, abortSignal),
      ))
    if (!seeded) {
      reportQuickstartProgress(
        progress,
        'content',
        'Seeding ' + getQuickstartSpaceName(quickstartId) + ' content',
      )
      await timeQuickstartPhase(timing, 'populate-space', () =>
        populateSpace(quickstartId, result, abortSignal, timing),
      )
      if (quickstartId === 'drive') {
        // Only a just seeded Drive has a known index; seeded Spaces keep theirs.
        result.initialObjectRoute = {
          objectKey: UNIXFS_OBJECT_KEY,
          objectType: UnixFSTypeID,
        }
      }
    }

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
  return QUICKSTART_SEEDS[quickstartId].initialObjectKey ?? ''
}

export function getQuickstartInitialObjectType(
  quickstartId: QuickstartSpaceCreateId,
): string {
  return QUICKSTART_SEEDS[quickstartId].initialObjectType ?? ''
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
  objectKey = getQuickstartInitialObjectKey(quickstartId),
): string {
  if (!objectKey) {
    return basePath
  }
  return basePath.replace(/\/+$/, '') + SUBPATH_DELIMITER + objectKey
}

// readSpaceSettings reads the Space's SpaceSettings, or undefined when the
// object is missing or its block does not decode. Invalid plugin IDs are
// dropped, since a corrupted block can decode arbitrary bytes into them.
async function readSpaceSettings(
  spaceWorld: IWorldState,
  abortSignal?: AbortSignal,
  timing?: QuickstartSetupTiming,
  phasePrefix = 'read-space-settings',
): Promise<SpaceSettings | undefined> {
  const existing = await timeQuickstartPhase(
    timing,
    phasePrefix + '-get-object',
    () => spaceWorld.getObject(SPACE_SETTINGS_OBJECT_KEY, abortSignal),
  )
  if (!existing) {
    return undefined
  }
  try {
    return await timeQuickstartPhase(
      timing,
      phasePrefix + '-read-existing',
      async () => {
        using cursor = await existing.accessWorldState(undefined, abortSignal)
        const blockResp = await cursor.unmarshal(
          { blockType: SPACE_SETTINGS_BLOCK_TYPE },
          abortSignal,
        )
        if (!blockResp.found || !blockResp.data?.length) {
          return undefined
        }
        const settings = SpaceSettings.fromBinary(blockResp.data)
        return {
          ...settings,
          pluginIds: (settings.pluginIds ?? []).filter(isValidSpacePluginId),
        }
      },
    )
  } catch {
    return undefined
  } finally {
    existing.release()
  }
}

// isQuickstartSpaceSeeded reports whether a quickstart Space holds its applied
// seed. Every seed but the bare Space's sets the Space index as its last World
// write, so a Space without an index was created but not seeded; the bare
// Space's seed is the settings object alone.
async function isQuickstartSpaceSeeded(
  quickstartId: QuickstartSpaceCreateId,
  spaceWorld: IWorldState,
  abortSignal?: AbortSignal,
): Promise<boolean> {
  const settings = await readSpaceSettings(spaceWorld, abortSignal)
  if (!settings) {
    return false
  }
  return quickstartId === 'space' || !!settings.indexPath
}

// hasSeedObject reports whether a seed step's object already exists, so a
// seed repeated after a failure skips the steps that landed.
async function hasSeedObject(
  spaceWorld: IWorldState,
  objectKey: string,
  abortSignal?: AbortSignal,
): Promise<boolean> {
  const existing = await spaceWorld.getObject(objectKey, abortSignal)
  existing?.release()
  return !!existing
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
  const op = await spaceSettingsOp(
    spaceWorld,
    abortSignal,
    indexPath,
    pluginIds,
    timing,
    phasePrefix,
  )
  await applyQuickstartOps(spaceWorld, [op], abortSignal, timing, phasePrefix)
}

// spaceSettingsOp builds the op that writes the Space settings with indexPath
// and pluginIds merged over the existing settings, so a seed can apply it in
// one batch with the ops that create what the index names.
async function spaceSettingsOp(
  spaceWorld: IWorldState,
  abortSignal?: AbortSignal,
  indexPath?: string,
  pluginIds?: string[],
  timing?: QuickstartSetupTiming,
  phasePrefix = 'create-space-settings',
): Promise<QuickstartOp> {
  const existingSettings = await readSpaceSettings(
    spaceWorld,
    abortSignal,
    timing,
    phasePrefix,
  )
  const requestedPluginIds = pluginIds ?? []
  const invalidPluginId = requestedPluginIds.find(
    (pluginId) => !isValidSpacePluginId(pluginId),
  )
  if (invalidPluginId !== undefined) {
    throw new Error(
      `quickstart returned invalid plugin id with length ${invalidPluginId.length}`,
    )
  }
  const settings: SpaceSettings = {
    indexPath: indexPath ?? existingSettings?.indexPath ?? '',
    pluginIds: Array.from(
      new Set([...(existingSettings?.pluginIds ?? []), ...requestedPluginIds]),
    ),
  }
  return {
    opTypeId: SET_SPACE_SETTINGS_OP_ID,
    opData: SetSpaceSettingsOp.toBinary({
      objectKey: SPACE_SETTINGS_OBJECT_KEY,
      settings,
      overwrite: true,
      timestamp: new Date(),
    }),
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

// getSpaceInstanceKey returns the Space world engine id, which is the instance
// key that selects the registrations of plugins installed in that Space.
async function getSpaceInstanceKey(
  setup: QuickstartSetup,
  abortSignal?: AbortSignal,
): Promise<string> {
  const info = await setup.spaceWorld.getEngine().getEngineInfo(abortSignal)
  return info.engineInfo?.engineId ?? ''
}

async function waitForObjectTypesRegistered(
  root: Root,
  instanceKey: string,
  typeIds: string[],
  abortSignal?: AbortSignal,
): Promise<void> {
  const pending = new Set(typeIds)
  const registry = new ObjectTypeRegistryResourceServiceClient(root.client)
  const timeoutSignal = AbortSignal.timeout(QUICKSTART_REGISTRATION_TIMEOUT_MS)
  const signal = abortSignal
    ? AbortSignal.any([abortSignal, timeoutSignal])
    : timeoutSignal
  try {
    for await (const state of registry.WatchObjectTypes(
      { instanceKey },
      signal,
    )) {
      for (const registration of state.registrations ?? []) {
        pending.delete(registration.typeId ?? '')
      }
      if (pending.size === 0) {
        return
      }
    }
  } catch (err) {
    if (timeoutSignal.aborted && !abortSignal?.aborted) {
      throw new Error(
        'Timed out waiting for quickstart object types: ' +
          Array.from(pending).join(', '),
        { cause: err },
      )
    }
    throw err
  }
  throw new Error(
    `quickstart object type stream ended before ${Array.from(pending).join(', ')} registered`,
  )
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
  instanceKey: string,
  quickstartId: string,
  pluginId: string,
  abortSignal?: AbortSignal,
): Promise<void> {
  const registry = new QuickstartRegistryResourceServiceClient(root.client)
  const list = await registry.ListQuickstarts({ instanceKey }, abortSignal)
  if (hasQuickstartRegistration(list, quickstartId, pluginId)) return

  const timeoutSignal = AbortSignal.timeout(QUICKSTART_REGISTRATION_TIMEOUT_MS)
  const signal = abortSignal
    ? AbortSignal.any([abortSignal, timeoutSignal])
    : timeoutSignal
  try {
    const stream = registry.WatchQuickstarts({ instanceKey }, signal)
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
  spaceWorld: IWorldState,
  abortSignal?: AbortSignal,
): Promise<void> {
  const { opTypeId, opData } = initUnixFSMutation()
  await spaceWorld.applyWorldOp(opTypeId, opData, '', abortSignal)
}

// hasDriveFile reports whether the Drive root holds name. A failed lookup
// counts as missing; the upload that follows reports any real fault.
async function hasDriveFile(
  root: FSHandle,
  name: string,
  abortSignal?: AbortSignal,
): Promise<boolean> {
  try {
    const child = await root.lookup(name, abortSignal)
    child.release()
    return true
  } catch {
    return false
  }
}

// writeDriveStarterGuide uploads the starter guide to the Drive root. On a
// repeated seed it leaves a guide that is already there, so a retry never
// overwrites a person's edits.
async function writeDriveStarterGuide(
  spaceWorld: EngineWorldState,
  repeat: boolean,
  abortSignal?: AbortSignal,
  timing?: QuickstartSetupTiming,
): Promise<void> {
  const access = await timeQuickstartPhase(
    timing,
    'write-drive-starter-guide-access',
    () => spaceWorld.accessTypedObject(UNIXFS_OBJECT_KEY, abortSignal),
  )
  if (!access.resourceId || access.typeId !== UnixFSTypeID) {
    if (access.resourceId) {
      spaceWorld.getResourceRef().createRef(access.resourceId).release()
    }
    throw new Error(
      `Drive starter guide expected ${UnixFSTypeID}, got ${access.typeId || 'unknown'}`,
    )
  }

  const ref = spaceWorld.getResourceRef().createRef(access.resourceId)
  const root = new FSHandle(ref)
  try {
    if (
      repeat &&
      (await hasDriveFile(root, DRIVE_STARTER_GUIDE_NAME, abortSignal))
    ) {
      return
    }
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
  const op: InitObjectLayoutOp = {
    objectKey: OBJECT_LAYOUT_OBJECT_KEY,
    timestamp: new Date(),
  }

  const opData = InitObjectLayoutOp.toBinary(op)
  await spaceWorld.applyWorldOp(
    INIT_OBJECT_LAYOUT_OP_ID,
    opData,
    '',
    abortSignal,
  )
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
      // Publish the Canvas, reference filesystem, and initial route together.
      await timeQuickstartPhase(timing, 'init-canvas-batch', () =>
        applyQuickstartBatch(
          setup.spaceWorld.getEngine(),
          [
            initUnixFSMutation(),
            {
              opTypeId: INIT_CANVAS_DEMO_OP_ID,
              opData: InitCanvasDemoOp.toBinary({
                objectKey: CANVAS_DEMO_OBJECT_KEY,
                timestamp: new Date(),
              }),
            },
            indexPathMutation(CANVAS_DEMO_OBJECT_KEY),
          ],
          abortSignal,
        ),
      )
      break
    case 'chat':
      await initChatQuickstart(setup.spaceWorld, abortSignal)
      break
    case 'kv':
      await initKvQuickstart(setup.spaceWorld, abortSignal)
      break
    case 'sql':
      if (!setup.root) {
        throw new Error('Root resource is required for SQL quickstart seeding')
      }
      await prepareSqlQuickstart(setup.root, setup, abortSignal)
      await executeDynamicQuickstart(
        setup.root,
        quickstartId,
        setup,
        abortSignal,
      )
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

async function openQuickstartHandle<T extends QuickstartResourceHandle>(
  spaceWorld: IWorldState,
  objectKey: string,
  typeId: string,
  ResourceCtor: QuickstartResourceConstructor<T>,
  abortSignal?: AbortSignal,
): Promise<T> {
  const access = await spaceWorld.accessTypedObject(objectKey, abortSignal)
  if (!access.resourceId || access.typeId !== typeId) {
    if (access.resourceId) {
      spaceWorld.getResourceRef().createRef(access.resourceId).release()
    }
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
  if (
    !(await hasSeedObject(spaceWorld, KV_QUICKSTART_STORE_KEY, abortSignal))
  ) {
    await createEmptyTypedObject(
      spaceWorld,
      KV_QUICKSTART_STORE_KEY,
      KvStoreTypeID,
      abortSignal,
    )
  }
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
        // eslint-disable-next-line react-doctor/async-parallel -- transaction writes stay ordered on one transaction.
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

async function prepareSqlQuickstart(
  root: Root,
  setup: QuickstartSetup,
  abortSignal?: AbortSignal,
): Promise<void> {
  await ensureSpacePlugins(
    setup.spaceWorld,
    [SQL_PLUGIN_ID],
    undefined,
    abortSignal,
  )
  const instanceKey = await getSpaceInstanceKey(setup, abortSignal)

  // Once the plugin is installed, its quickstart and object-type registrations
  // land independently, so wait for both concurrently. The type registrations
  // must be present before the quickstart opens because the initial object
  // navigation resolves the sql viewer by object type.
  await Promise.all([
    waitForQuickstartRegistration(
      root,
      instanceKey,
      'sql',
      SQL_PLUGIN_ID,
      abortSignal,
    ),
    waitForObjectTypesRegistered(
      root,
      instanceKey,
      [
        SQL_DB_TYPE_ID,
        SQL_QUERY_TYPE_ID,
        SQL_QUERY_RESULT_TYPE_ID,
        SQL_SCHEMA_TYPE_ID,
        SQL_TABLE_VIEW_TYPE_ID,
        SQL_WORKBENCH_TYPE_ID,
      ],
      abortSignal,
    ),
  ])
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
    await getSpaceInstanceKey(setup, abortSignal),
    quickstartId,
    NOTES_PLUGIN_ID,
    abortSignal,
  )
}

// createDrive sets up a drive with UnixFS content. The index goes in last, so
// a Space whose guide upload failed stays unseeded and a retry finishes it; a
// retry keeps the filesystem and any guide already in it.
export async function createDrive(
  spaceWorld: EngineWorldState,
  abortSignal?: AbortSignal,
  timing?: QuickstartSetupTiming,
): Promise<void> {
  const repeat = await hasSeedObject(spaceWorld, UNIXFS_OBJECT_KEY, abortSignal)
  if (!repeat) {
    await timeQuickstartPhase(timing, 'init-drive-batch', () =>
      applyQuickstartBatch(
        spaceWorld.getEngine(),
        [initUnixFSMutation()],
        abortSignal,
      ),
    )
  }
  await writeDriveStarterGuide(spaceWorld, repeat, abortSignal, timing)
  await timeQuickstartPhase(timing, 'index-drive-batch', () =>
    applyQuickstartBatch(
      spaceWorld.getEngine(),
      [indexPathMutation(UNIXFS_OBJECT_KEY)],
      abortSignal,
    ),
  )
}

// QuickstartOp is one World op a quickstart applies.
interface QuickstartOp {
  opTypeId: string
  opData: Uint8Array
}

// applyQuickstartBatch applies a quickstart's ops as one World batch, so they
// land together or not at all and hold the World's writer only for the call.
async function applyQuickstartBatch(
  engine: Pick<Engine, 'executeWorldBatch'>,
  ops: QuickstartOp[],
  abortSignal?: AbortSignal,
): Promise<void> {
  const response = await engine.executeWorldBatch(
    ops.map((value) => ({ mutation: { case: 'op', value } })),
    abortSignal,
  )
  throwBatchError(response)
}

// initUnixFSMutation creates the quickstart's root filesystem.
function initUnixFSMutation(): QuickstartOp {
  return {
    opTypeId: INIT_UNIXFS_OP_ID,
    opData: InitUnixFSOp.toBinary({
      objectKey: UNIXFS_OBJECT_KEY,
      timestamp: new Date(),
    }),
  }
}

// indexPathMutation points the Space index at indexPath, creating the Space
// settings when they are missing and keeping their plugins.
function indexPathMutation(indexPath: string): QuickstartOp {
  return {
    opTypeId: SET_SPACE_INDEX_PATH_OP_ID,
    opData: SetSpaceIndexPathOp.toBinary({ indexPath, timestamp: new Date() }),
  }
}

// applyQuickstartOps applies a seed's World ops. A World with a live Engine
// takes them as one batch, so they land together or not at all and the writer
// is held only inside the call.
async function applyQuickstartOps(
  spaceWorld: IWorldState,
  ops: QuickstartOp[],
  abortSignal: AbortSignal | undefined,
  timing: QuickstartSetupTiming | undefined,
  phasePrefix: string,
): Promise<void> {
  const engine = spaceWorld.getEngine?.()
  await timeQuickstartPhase(timing, phasePrefix + '-apply-op', async () => {
    if (engine) {
      await applyQuickstartBatch(engine, ops, abortSignal)
      return
    }
    for (const op of ops) {
      // eslint-disable-next-line react-doctor/async-await-in-loop -- a World without an Engine applies the ops in order.
      await spaceWorld.applyWorldOp(op.opTypeId, op.opData, '', abortSignal)
    }
  })
}

// initGitQuickstart seeds a persistent git/repo wizard and indexes the Space to it.
async function initGitQuickstart(
  setup: QuickstartSetup,
  abortSignal?: AbortSignal,
): Promise<void> {
  const now = new Date()
  const wizardKey = wizardObjectKey('Repository ' + now.getTime().toString(36))
  await applyIndexedSeed(
    setup.spaceWorld,
    [
      {
        opTypeId: CREATE_WIZARD_OBJECT_OP_ID,
        opData: CreateWizardObjectOp.toBinary({
          objectKey: wizardKey,
          wizardTypeId: 'wizard/git/repo',
          targetTypeId: 'git/repo',
          targetKeyPrefix: 'git/repo/',
          name: 'Repository',
          timestamp: now,
        }),
      },
    ],
    wizardKey,
    undefined,
    abortSignal,
  )
}

// initChatQuickstart creates a chat channel in the space.
async function initChatQuickstart(
  spaceWorld: EngineWorldState,
  abortSignal?: AbortSignal,
): Promise<void> {
  await applyIndexedSeed(
    spaceWorld,
    [
      {
        opTypeId: INIT_CHAT_DEMO_OP_ID,
        opData: InitChatDemoOp.toBinary({
          channelObjectKey: CHAT_DEMO_CHANNEL_KEY,
          timestamp: new Date(),
        }),
      },
    ],
    CHAT_DEMO_CHANNEL_KEY,
    undefined,
    abortSignal,
  )
}

// initV86Quickstart creates a V86 wizard with the default CDN image selected.
async function initV86Quickstart(
  setup: QuickstartSetup,
  abortSignal?: AbortSignal,
): Promise<void> {
  const now = new Date()
  const wizardKey = buildV86QuickstartWizardKey(now)
  const config = buildV86QuickstartWizardConfig()
  await applyIndexedSeed(
    setup.spaceWorld,
    [
      {
        opTypeId: CREATE_WIZARD_OBJECT_OP_ID,
        opData: CreateWizardObjectOp.toBinary({
          objectKey: wizardKey,
          wizardTypeId: V86_WIZARD_TYPE_ID,
          targetTypeId: VmV86TypeID,
          targetKeyPrefix: V86_WIZARD_TARGET_KEY_PREFIX,
          name: 'VM',
          timestamp: now,
          initialStep: 1,
          initialConfigData: V86WizardConfig.toBinary(config),
        }),
      },
    ],
    wizardKey,
    [V86_PLUGIN_ID],
    abortSignal,
  )
}

// initDeviceQuickstart creates the Computers dashboard and an Add Device
// wizard, and indexes the Space to the dashboard.
async function initDeviceQuickstart(
  setup: QuickstartSetup,
  abortSignal?: AbortSignal,
  timing?: QuickstartSetupTiming,
): Promise<void> {
  const now = new Date()
  const wizardKey = wizardObjectKey('Add Device ' + now.getTime().toString(36))
  // The dashboard precedes the wizard so routing observes it first.
  await applyIndexedSeed(
    setup.spaceWorld,
    [
      {
        opTypeId: CREATE_COMPUTERS_DASHBOARD_OP_ID,
        opData: CreateComputersDashboardOp.toBinary({
          objectKey: DEVICE_QUICKSTART_DASHBOARD_KEY,
          name: 'Computers',
          timestamp: now,
        }),
      },
      {
        opTypeId: CREATE_WIZARD_OBJECT_OP_ID,
        opData: CreateWizardObjectOp.toBinary({
          objectKey: wizardKey,
          wizardTypeId: AddDeviceWizardTypeID,
          targetTypeId: DeviceTypeID,
          targetKeyPrefix: AddDeviceWizardTargetKeyPrefix,
          name: AddDeviceDefaultName,
          timestamp: now,
        }),
      },
    ],
    DEVICE_QUICKSTART_DASHBOARD_KEY,
    undefined,
    abortSignal,
    timing,
    'create-device',
  )
}

// applyIndexedSeed applies a seed's ops and the settings write that indexes
// the Space at indexPath as one batch. The index marks the seed applied, so
// the seed lands whole or not at all and repeating it after a failure is safe.
async function applyIndexedSeed(
  spaceWorld: IWorldState,
  ops: QuickstartOp[],
  indexPath: string,
  pluginIds: string[] | undefined,
  abortSignal?: AbortSignal,
  timing?: QuickstartSetupTiming,
  phasePrefix = 'seed',
): Promise<void> {
  const settingsOp = await spaceSettingsOp(
    spaceWorld,
    abortSignal,
    indexPath,
    pluginIds,
    timing,
    phasePrefix + '-settings',
  )
  await applyQuickstartOps(
    spaceWorld,
    [...ops, settingsOp],
    abortSignal,
    timing,
    phasePrefix,
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
  if (!(await hasSeedObject(setup.spaceWorld, layoutKey, abortSignal))) {
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
  }
  await createSpaceSettingsObject(setup.spaceWorld, abortSignal, layoutKey)
}
