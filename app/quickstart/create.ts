import type { RegisterCleanup } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { Root } from '@s4wave/sdk/root'
import type { Session } from '@s4wave/sdk/session'
import type { CreateSpaceResponse } from '@s4wave/sdk/session/session.pb.js'
import type { CreateAccountResponse } from '@s4wave/sdk/provider/local/local.pb.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import { LocalProvider } from '@s4wave/sdk/provider/local/local.js'
import { Space } from '@s4wave/sdk/space/space.js'
import { SpaceContents } from '@s4wave/sdk/space/contents.js'
import { EngineWorldState } from '@s4wave/sdk/world/engine-state.js'
import { BucketLookupCursor } from '@s4wave/sdk/bucket/lookup/lookup.js'
import { SPACE_SETTINGS_OBJECT_KEY } from '@s4wave/core/space/world/world.js'
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

import { NOTEBOOK_OBJECT_KEY } from '../../plugin/notes/proto/init-notebook.js'
import { InitChatDemoOp } from '@s4wave/sdk/chat/chat.pb.js'
import {
  INIT_CHAT_DEMO_OP_ID,
  CHAT_DEMO_CHANNEL_KEY,
} from '@s4wave/sdk/chat/init-chat-demo.js'
import { createBlogClientSide } from '../../plugin/notes/blog-seed.js'
import {
  createDocsClientSide,
  createNotebookClientSide,
} from '../../plugin/notes/content-seed.js'
import { V86WizardConfig } from '@s4wave/sdk/vm/v86-wizard.pb.js'
import { CreateWizardObjectOp } from '@s4wave/sdk/world/wizard/wizard.pb.js'
import { CREATE_WIZARD_OBJECT_OP_ID } from '@s4wave/sdk/world/wizard/create-wizard.js'
import { InitForgeQuickstartOp } from '@s4wave/core/forge/dashboard/dashboard.pb.js'
import { INIT_FORGE_QUICKSTART_OP_ID } from '@s4wave/sdk/forge/dashboard/init-forge-quickstart.js'
import { markInteracted } from '@s4wave/web/state/interaction.js'
import { mountSpace } from '@s4wave/app/space/space.js'
import {
  buildV86QuickstartWizardConfig,
  buildV86QuickstartWizardKey,
  V86_WIZARD_TARGET_KEY_PREFIX,
  V86_WIZARD_TARGET_TYPE_ID,
  V86_WIZARD_TYPE_ID,
} from '@s4wave/app/vm/v86-wizard-config.js'

import { type QuickstartSpaceCreateId } from './options.js'

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
  const resp = await root.listSessions(abortSignal)
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
): Promise<LocalSessionSetup> {
  // Check for an existing local session to reuse.
  if (!forceNew) {
    const existing = await findMostRecentLocalSession(root, abortSignal)
    if (existing) {
      const session = cleanup(
        await root.mountSession(
          { sessionRef: existing.sessionRef },
          abortSignal,
        ),
      )
      markInteracted()
      return { sessionIndex: existing.sessionIndex, session }
    }
  }

  // No existing local session (or forceNew): create a new account.
  using provider = await root.lookupProvider('local')
  const lp = new LocalProvider(provider.resourceRef)
  const accountResp = await lp.createAccount(abortSignal)
  const sessionIndex = accountResp.sessionListEntry?.sessionIndex ?? 1

  // Mount the session using the account's session reference.
  const session = cleanup(
    await root.mountSession(
      { sessionRef: accountResp.sessionListEntry?.sessionRef },
      abortSignal,
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
  accountResp?: CreateAccountResponse
  sessionIndex: number
  spaceResp: CreateSpaceResponse
  session: Session
  space: Space
  spaceContents: SpaceContents
  spaceWorld: EngineWorldState
  spaceWorldState: BucketLookupCursor
}

// QuickstartSetupParams contains the parameters for creating a quickstart setup.
export interface QuickstartSetupParams {
  session: Session
  spaceResp: CreateSpaceResponse
  abortSignal: AbortSignal
  cleanup: RegisterCleanup
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
  const { session, spaceResp, abortSignal, cleanup } = params

  // Mount the space from the response.
  const space = await mountSpace({
    session,
    spaceResp,
    abortSignal,
    cleanup,
  })

  // Access the World associated with the space as a WorldState.
  const spaceWorld = await space.accessWorldState(true, abortSignal)
  const spaceContents = cleanup(await space.mountSpaceContents(abortSignal))

  // Access the world state bucket storage.
  const spaceWorldState = cleanup(
    await spaceWorld.accessWorldState(undefined, abortSignal),
  )

  return {
    space,
    spaceContents,
    spaceWorld,
    spaceWorldState,
  }
}

export async function createQuickstartSetup(
  root: Root,
  quickstartId: QuickstartSpaceCreateId,
  abortSignal: AbortSignal,
  cleanup: RegisterCleanup,
): Promise<QuickstartSetup> {
  // Reuse existing local session or create a new one.
  const { accountResp, sessionIndex, session } = await createLocalSession(
    root,
    abortSignal,
    cleanup,
  )

  // Create a new space with the quickstart ID as the name.
  const spaceResp = await session.createSpace(
    { spaceName: getQuickstartSpaceName(quickstartId) },
    abortSignal,
  )

  // Create the setup from the session and space response.
  const setup = await createQuickstartSetupFromSession({
    session,
    spaceResp,
    abortSignal,
    cleanup,
  })

  // Construct the result
  const result = {
    accountResp,
    sessionIndex,
    session,
    spaceResp,
    ...setup,
  }

  // Populate the space with quickstart-specific content.
  await populateSpace(quickstartId, result, abortSignal)

  return result
}

// createSpaceSettingsObject creates the SpaceSettings object in the world.
export async function createSpaceSettingsObject(
  spaceWorld: IWorldState,
  abortSignal?: AbortSignal,
  indexPath?: string,
  pluginIds?: string[],
): Promise<void> {
  let existingSettings: SpaceSettings | undefined
  const existing = await spaceWorld.getObject(
    SPACE_SETTINGS_OBJECT_KEY,
    abortSignal,
  )
  try {
    if (existing) {
      try {
        using cursor = await existing.accessWorldState(undefined, abortSignal)
        const blockResp = await cursor.getBlock({}, abortSignal)
        if (blockResp.found && blockResp.data) {
          existingSettings = SpaceSettings.fromBinary(blockResp.data)
        }
      } catch {
        existingSettings = undefined
      }
    }

    const mergedPluginIds = Array.from(
      new Set(
        [...(existingSettings?.pluginIds ?? []), ...(pluginIds ?? [])].filter(
          Boolean,
        ),
      ),
    )
    const settings: SpaceSettings = {
      indexPath: indexPath ?? existingSettings?.indexPath ?? '',
      pluginIds: mergedPluginIds,
    }
    await spaceWorld.applyWorldOp(
      SET_SPACE_SETTINGS_OP_ID,
      SetSpaceSettingsOp.toBinary({
        objectKey: SPACE_SETTINGS_OBJECT_KEY,
        settings,
        overwrite: true,
        timestamp: new Date(),
      }),
      '',
      abortSignal,
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

export async function approveSpacePlugins(
  spaceContents: SpaceContents,
  pluginIds: string[],
  abortSignal?: AbortSignal,
): Promise<void> {
  const ids = Array.from(new Set(pluginIds.filter(Boolean)))
  for (const pluginId of ids) {
    await spaceContents.setPluginApproval(pluginId, true, abortSignal)
  }
}

async function withWritableWorldState<T>(
  worldState: IWorldState,
  abortSignal: AbortSignal | undefined,
  cb: (writeState: IWorldState) => Promise<T>,
): Promise<T> {
  if (!(worldState instanceof EngineWorldState)) {
    return cb(worldState)
  }

  const tx = await worldState.getEngine().newTransaction(true, abortSignal)
  let committed = false
  try {
    const result = await cb(tx)
    await tx.commit(abortSignal)
    committed = true
    return result
  } finally {
    if (!committed) {
      await tx.discard(abortSignal).catch(() => {})
    }
    tx.release()
  }
}

async function runQuickstartStep<T>(
  label: string,
  cb: () => Promise<T>,
): Promise<T> {
  try {
    return await cb()
  } catch (err) {
    throw new Error(
      label + ': ' + (err instanceof Error ? err.message : String(err)),
    )
  }
}

// initUnixFS initializes a UnixFS filesystem with starter content.
export async function initUnixFS(
  spaceWorld: EngineWorldState,
  abortSignal?: AbortSignal,
): Promise<void> {
  // Create the InitUnixFSOp operation
  const op: InitUnixFSOp = {
    objectKey: UNIXFS_OBJECT_KEY,
    timestamp: new Date(),
  }

  // Apply the operation using ApplyWorldOp
  const opData = InitUnixFSOp.toBinary(op)
  await spaceWorld.applyWorldOp(INIT_UNIXFS_OP_ID, opData, '', abortSignal)
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
): Promise<void> {
  switch (quickstartId) {
    case 'space':
      await createSpaceSettingsObject(setup.spaceWorld, abortSignal)
      break
    case 'drive':
      await createDrive(setup.spaceWorld, abortSignal)
      break
    case 'git':
      await initGitQuickstart(setup, abortSignal)
      break
    case 'notebook':
      await initNotebookQuickstart(setup.spaceWorld, abortSignal)
      break
    case 'canvas':
      await createSpaceSettingsObject(
        setup.spaceWorld,
        abortSignal,
        CANVAS_DEMO_OBJECT_KEY,
      )
      await initUnixFS(setup.spaceWorld, abortSignal)
      await initCanvasDemo(setup.spaceWorld, abortSignal)
      break
    case 'chat':
      await initChatQuickstart(setup.spaceWorld, abortSignal)
      break
    case 'docs':
      await initDocsQuickstart(setup.spaceWorld, abortSignal)
      break
    case 'blog':
      await initBlogQuickstart(setup.spaceWorld, abortSignal)
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

// initNotebookQuickstart creates a Notebook space with a UnixFS object
// for note storage and a Notebook world object referencing it.
async function initNotebookQuickstart(
  spaceWorld: EngineWorldState,
  abortSignal?: AbortSignal,
): Promise<void> {
  await createSpaceSettingsObject(spaceWorld, abortSignal, NOTEBOOK_OBJECT_KEY)
  await createNotebookClientSide(
    spaceWorld,
    NOTEBOOK_OBJECT_KEY,
    UNIXFS_OBJECT_KEY,
    'Notes',
    new Date(),
    abortSignal,
  )
}

// createDrive sets up a drive with UnixFS content.
export async function createDrive(
  spaceWorld: EngineWorldState,
  abortSignal?: AbortSignal,
): Promise<void> {
  await createSpaceSettingsObject(spaceWorld, abortSignal, UNIXFS_OBJECT_KEY)
  await initUnixFS(spaceWorld, abortSignal)
}

// initGitQuickstart seeds a persistent git/repo wizard and indexes the Space to it.
async function initGitQuickstart(
  setup: QuickstartSetup,
  abortSignal?: AbortSignal,
): Promise<void> {
  const now = new Date()
  const wizardKey = `wizard/git/repo/${now.getTime().toString(36)}`
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

// initDocsQuickstart creates a documentation site in the space.
async function initDocsQuickstart(
  spaceWorld: EngineWorldState,
  abortSignal?: AbortSignal,
): Promise<void> {
  const objectKey = 'docs/documentation'
  await createSpaceSettingsObject(spaceWorld, abortSignal, objectKey)
  await createDocsClientSide(
    spaceWorld,
    objectKey,
    'Documentation',
    '',
    new Date(),
    abortSignal,
  )
}

// initBlogQuickstart creates a blog in the space.
async function initBlogQuickstart(
  spaceWorld: EngineWorldState,
  abortSignal?: AbortSignal,
): Promise<void> {
  const objectKey = 'blog/site'
  const timestamp = new Date()
  await withWritableWorldState(spaceWorld, abortSignal, async (writeState) => {
    await runQuickstartStep('init blog content', async () => {
      await createBlogClientSide(
        writeState,
        objectKey,
        'Blog',
        '',
        '',
        timestamp,
        abortSignal,
      )
    })
    await runQuickstartStep('configure blog space settings', async () => {
      await createSpaceSettingsObject(writeState, abortSignal, objectKey)
    })
  })
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
  const layoutKey = 'object-layout/forge'

  // Get the session peer ID for worker registration.
  const sessionInfo = await setup.session.getSessionInfo(abortSignal)
  const sessionPeerId = sessionInfo.peerId ?? ''

  const op: InitForgeQuickstartOp = {
    layoutKey,
    dashboardKey: 'forge/dashboard',
    clusterKey: 'forge/cluster',
    clusterName: 'default',
    workerKey: 'forge/worker/session',
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
