import { useCallback, useMemo } from 'react'

import {
  SharedObjectContext,
  useSessionIndex,
  useSessionNavigate,
} from '@s4wave/web/contexts/contexts.js'
import { useCommand } from '@s4wave/web/command/useCommand.js'
import { CommandSurface } from '@s4wave/sdk/command/command.pb.js'
import { useOpenCommand } from '@s4wave/web/command/CommandContext.js'
import { useIsTabActive } from '@s4wave/web/contexts/TabActiveContext.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { pluginPathPrefix } from '@s4wave/app/urls.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { downloadURL } from '@s4wave/web/download.js'
import { CreateWizardObjectOp } from '@s4wave/sdk/world/wizard/wizard.pb.js'
import { CREATE_WIZARD_OBJECT_OP_ID } from '@s4wave/sdk/world/wizard/create-wizard.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import { WorldOperationRejection } from '@s4wave/sdk/world/errors.js'
import type { SubItemsCallback } from '@s4wave/web/command/CommandContext.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import type { ObjectWizard } from '@s4wave/sdk/world/wizard/wizard.pb.js'
import { useExperimentalCreatorsEnabled } from '../creator-visibility.js'
import { normalizeObjectWizards } from './object-wizards.js'
import { useListObjectWizards, useObjectWizards } from './useObjectWizards.js'
import {
  lookupCreateOpBuilder,
  buildObjectKey,
  buildWizardObjectKey,
} from './create-op-builders.js'

interface SpaceCommandsProps {
  canRename: boolean
  canShare: boolean
  onRenameSpace: () => void
  onShareSpace: () => void
}

// CreatePlan is one create command's object key and op, built once.
interface CreatePlan {
  name: string
  objectKey: string
  // objectType is the type the op gives the object at objectKey.
  objectType: string
  opId: string
  opData: Uint8Array
  // persistent creates a wizard object rather than the target object.
  persistent: boolean
}

// createName returns the name a wizard's create gives its object.
function createName(wizard: ObjectWizard): string {
  if (wizard.persistent && wizard.wizardTypeId) {
    return wizard.defaultNamePattern || wizard.displayName || 'Wizard'
  }
  return wizard.defaultNamePattern || wizard.displayName || 'Untitled'
}

// planCreate picks a free key and builds the op that creates the wizard's
// object there, or returns null when the wizard has no create op.
async function planCreate(
  world: IWorldState,
  wizard: ObjectWizard,
  name: string,
): Promise<CreatePlan | null> {
  if (wizard.persistent && wizard.wizardTypeId) {
    const objectKey = await buildWizardObjectKey(world, name)
    const opData = CreateWizardObjectOp.toBinary({
      objectKey,
      wizardTypeId: wizard.wizardTypeId,
      targetTypeId: wizard.typeId ?? '',
      targetKeyPrefix: wizard.keyPrefix ?? '',
      name,
      timestamp: new Date(),
    })
    return {
      name,
      objectKey,
      objectType: wizard.wizardTypeId,
      opId: CREATE_WIZARD_OBJECT_OP_ID,
      opData,
      persistent: true,
    }
  }

  if (!wizard.createOpId || !wizard.keyPrefix || !wizard.typeId) return null
  const builder = lookupCreateOpBuilder(wizard.createOpId)
  if (!builder) return null
  const objectKey = await buildObjectKey(world, wizard.keyPrefix, name)
  return {
    name,
    objectKey,
    objectType: wizard.typeId,
    opId: wizard.createOpId,
    opData: builder(objectKey, name),
    persistent: false,
  }
}

// readObjectType returns the type of the object at key, '' for an object
// with no type, or null when no object exists there.
async function readObjectType(
  world: IWorldState,
  key: string,
): Promise<string | null> {
  const page = await world.listObjects({ prefix: key, limit: 1 })
  const object = page.objects?.[0]
  return object?.objectKey === key ? (object.typeId ?? '') : null
}

// SpaceCommands registers space-scoped commands. Returns null (no UI).
export function SpaceCommands({
  canRename,
  canShare,
  onRenameSpace,
  onShareSpace,
}: SpaceCommandsProps) {
  const sharedObjectResource = SharedObjectContext.useContext()
  const sharedObject = useResourceValue(sharedObjectResource)
  const sharedObjectId = sharedObject?.meta.sharedObjectId ?? ''
  const sessionIndex = useSessionIndex()
  const navigateSession = useSessionNavigate()
  const isTabActive = useIsTabActive()
  const openCommand = useOpenCommand()
  const { spaceWorld, navigateToObjects } = SpaceContainerContext.useContext()
  const experimentalCreatorsEnabled = useExperimentalCreatorsEnabled()
  const wizardState = useObjectWizards()
  const listWizards = useListObjectWizards()
  const wizards = useMemo(
    () =>
      normalizeObjectWizards(
        wizardState.value?.wizards ?? [],
        experimentalCreatorsEnabled,
      ),
    [experimentalCreatorsEnabled, wizardState.value?.wizards],
  )
  // The watch may lag a route transition, so the palette reads a fresh list.
  const loadWizards = useCallback(
    async (signal?: AbortSignal) =>
      normalizeObjectWizards(
        await listWizards(signal),
        experimentalCreatorsEnabled,
      ),
    [experimentalCreatorsEnabled, listWizards],
  )

  const handleCloseSpace = useCallback(() => {
    navigateSession({ path: '' })
  }, [navigateSession])

  const handleExportSpace = useCallback(() => {
    if (sessionIndex == null || !sharedObjectId) return
    void downloadURL(
      `${pluginPathPrefix}/export/u/${sessionIndex}/so/${encodeURIComponent(sharedObjectId)}`,
    ).catch((err: unknown) => {
      console.error('failed to export space', err)
      toast.error('Export failed', {
        description: 'Space export could not be prepared.',
      })
    })
  }, [sessionIndex, sharedObjectId])

  const handleShareSpace = useCallback(() => {
    if (!canShare || !sharedObjectId) return
    onShareSpace()
  }, [canShare, onShareSpace, sharedObjectId])

  useCommand({
    commandId: 'spacewave.file.close-space',
    label: 'Close Space',
    description: 'Return to the session dashboard',
    menuPath: 'File/Close Space',
    menuGroup: 50,
    menuOrder: 1,
    active: isTabActive,
    enabled: sessionIndex != null,
    handler: handleCloseSpace,
  })

  useCommand({
    commandId: 'spacewave.file.rename-space',
    label: 'Rename Space',
    description: 'Change the display name of this space',
    menuPath: 'File/Rename Space',
    menuGroup: 40,
    menuOrder: 1,
    active: isTabActive,
    enabled: canRename && !!sharedObjectId,
    handler: onRenameSpace,
  })

  useCommand({
    commandId: 'spacewave.share-space',
    label: 'Share Space',
    description: 'Invite people to this space',
    menuPath: 'File/Share Space',
    menuGroup: 40,
    menuOrder: 2,
    active: isTabActive,
    enabled: canShare && !!sharedObjectId,
    handler: handleShareSpace,
  })

  useCommand({
    commandId: 'spacewave.file.export',
    label: 'Export Space',
    description: 'Download all space contents as a zip archive',
    menuPath: 'File/Export Space',
    menuGroup: 40,
    menuOrder: 3,
    active: isTabActive,
    enabled: !!sharedObjectId && sessionIndex != null,
    handler: handleExportSpace,
  })

  const createObjectSubItems: SubItemsCallback = useCallback(
    async (query, signal) => {
      if (signal.aborted) return []
      const visibleWizards = await loadWizards(signal)
      if (signal.aborted) return []
      const q = query.toLowerCase()
      return visibleWizards.flatMap((w) => {
        if (
          q &&
          !w.displayName?.toLowerCase().includes(q) &&
          !w.category?.toLowerCase().includes(q) &&
          !w.description?.toLowerCase().includes(q)
        ) {
          return []
        }
        return [
          {
            id: w.typeId ?? '',
            label: w.displayName ?? '',
            description: w.description || w.category,
            iconName: w.iconName,
            experimental: w.experimental || undefined,
          },
        ]
      })
    },
    [loadWizards],
  )

  // createObject runs one create command. The key and op are built once, so
  // Try again repeats the same create instead of choosing a new key. A failed
  // create shows a toast that stays until dismissed or retried; a refusal
  // shows the op's own explanation.
  const createObject = useCallback(
    (typeId: string) => {
      let name = ''
      let plan: CreatePlan | null = null
      let sent = false

      const done = (created: CreatePlan) => {
        if (!created.persistent) toast.success(`Created ${created.name}`)
        navigateToObjects([created.objectKey])
      }
      const fail = (err: unknown, retry: boolean) => {
        toast.error(`Could not create ${name || 'the object'}`, {
          description:
            err instanceof WorldOperationRejection
              ? err.message
              : 'The Space could not finish the change.',
          ...(retry && {
            duration: Infinity,
            action: { label: 'Try again', onClick: attempt },
          }),
        })
      }
      const run = async () => {
        if (!plan) {
          let wizard = wizards.find((w) => w.typeId === typeId)
          if (!wizard) {
            wizard = (await loadWizards()).find((w) => w.typeId === typeId)
          }
          if (!wizard) return
          name = createName(wizard)
          plan = await planCreate(spaceWorld, wizard, name)
          if (!plan) return
        }
        // Contract: a create whose reply was lost may still have committed,
        // and the op does not yet treat an identical repeat as a no-op, so a
        // repeat first reads its key. The object there is either this create
        // landing, or another writer's object that this create must not
        // touch.
        if (sent) {
          const found = await readObjectType(spaceWorld, plan.objectKey)
          if (found === plan.objectType) {
            done(plan)
            return
          }
          if (found !== null) {
            fail(null, false)
            return
          }
        }
        sent = true
        await spaceWorld.applyWorldOp(plan.opId, plan.opData, '')
        done(plan)
      }
      const attempt = () => {
        run().catch((err: unknown) => {
          console.error('failed to create object', err)
          fail(err, true)
        })
      }
      attempt()
    },
    [spaceWorld, navigateToObjects, wizards, loadWizards],
  )

  const handleCreateObject = useCallback(
    (args: Record<string, string>) => {
      if (args.subItemId) {
        createObject(args.subItemId)
        return
      }
      openCommand('spacewave.create-object')
    },
    [openCommand, createObject],
  )

  useCommand({
    commandId: 'spacewave.create-object',
    label: 'Create Object',
    description: 'Create a new object in this space',
    menuPath: 'File/Create Object',
    menuGroup: 10,
    menuOrder: 0,
    defaultBindings: [
      {
        id: 'default',
        binding: { case: 'combo', value: { combo: 'CmdOrCtrl+N' } },
        surface: CommandSurface.WEB,
      },
    ],
    active: isTabActive,
    hasSubItems: true,
    subItems: createObjectSubItems,
    handler: handleCreateObject,
  })

  return null
}
