import { useCallback, useMemo, useRef } from 'react'

import {
  SharedObjectContext,
  SpaceContext,
  useSessionIndex,
  useSessionNavigate,
} from '@s4wave/web/contexts/contexts.js'
import { useCommand } from '@s4wave/web/command/useCommand.js'
import { useOpenCommand } from '@s4wave/web/command/CommandContext.js'
import { useIsTabActive } from '@s4wave/web/contexts/TabActiveContext.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { pluginPathPrefix } from '@s4wave/app/urls.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { downloadURL } from '@s4wave/web/download.js'
import { CreateWizardObjectOp } from '@s4wave/sdk/world/wizard/wizard.pb.js'
import { CREATE_WIZARD_OBJECT_OP_ID } from '@s4wave/sdk/world/wizard/create-wizard.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import type { SubItemsCallback } from '@s4wave/web/command/CommandContext.js'
import type { ObjectWizard } from '@s4wave/sdk/world/wizard/wizard.pb.js'
import { CREATE_BLOG_OP_ID } from '../../plugin/notes/proto/create-blog.js'
import { createBlogClientSide } from '../../plugin/notes/blog-seed.js'
import {
  buildNotebookUnixfsObjectKey,
  createDocsClientSide,
  createNotebookClientSide,
} from '../../plugin/notes/content-seed.js'
import { CREATE_DOCS_OP_ID } from '../../plugin/notes/proto/create-docs.js'
import { INIT_NOTEBOOK_OP_ID } from '../../plugin/notes/proto/init-notebook.js'
import { normalizeObjectWizards } from './object-wizards.js'
import { lookupCreateOpBuilder, buildObjectKey } from './create-op-builders.js'

interface SpaceCommandsProps {
  canRename: boolean
  onRenameSpace: () => void
}

// SpaceCommands registers space-scoped commands. Returns null (no UI).
export function SpaceCommands({
  canRename,
  onRenameSpace,
}: SpaceCommandsProps) {
  const sharedObjectResource = SharedObjectContext.useContext()
  const sharedObject = useResourceValue(sharedObjectResource)
  const sharedObjectId = sharedObject?.meta.sharedObjectId ?? ''
  const sessionIndex = useSessionIndex()
  const navigateSession = useSessionNavigate()
  const isTabActive = useIsTabActive()
  const openCommand = useOpenCommand()
  const { spaceState, spaceWorld, navigateToObjects } =
    SpaceContainerContext.useContext()
  const spaceResource = SpaceContext.useContext()
  const space = useResourceValue(spaceResource)
  const existingObjectKeys = useMemo(
    () =>
      spaceState.worldContents?.objects?.map((obj) => obj.objectKey ?? '') ??
      [],
    [spaceState.worldContents?.objects],
  )

  const handleCloseSpace = useCallback(() => {
    navigateSession({ path: '' })
  }, [navigateSession])

  const handleExportSpace = useCallback(() => {
    if (sessionIndex == null || !sharedObjectId) return
    downloadURL(
      `${pluginPathPrefix}/export/u/${sessionIndex}/so/${encodeURIComponent(sharedObjectId)}`,
    )
  }, [sessionIndex, sharedObjectId])

  useCommand({
    commandId: 'spacewave.file.close-space',
    label: 'Close Space',
    description: 'Return to the session dashboard',
    menuPath: 'File/Close Space',
    menuGroup: 1,
    menuOrder: 2,
    active: isTabActive,
    enabled: sessionIndex != null,
    handler: handleCloseSpace,
  })

  useCommand({
    commandId: 'spacewave.file.rename-space',
    label: 'Rename Space',
    description: 'Change the display name of this space',
    menuPath: 'File/Rename Space',
    menuGroup: 3,
    menuOrder: 0,
    active: isTabActive,
    enabled: canRename && !!sharedObjectId,
    handler: onRenameSpace,
  })

  useCommand({
    commandId: 'spacewave.file.export',
    label: 'Export Space',
    description: 'Download all space contents as a zip archive',
    menuPath: 'File/Export Space',
    menuGroup: 4,
    menuOrder: 1,
    active: isTabActive,
    enabled: !!sharedObjectId && sessionIndex != null,
    handler: handleExportSpace,
  })

  // Cache the wizard list so sub-item queries filter locally instead of re-fetching.
  const wizardCache = useRef<ObjectWizard[] | null>(null)
  const createObjectSubItems: SubItemsCallback = useCallback(
    async (query, signal) => {
      if (!space) return []
      if (!wizardCache.current) {
        wizardCache.current = normalizeObjectWizards(
          await space.listWizards(signal),
        )
      }
      const wizards = wizardCache.current
      const q = query.toLowerCase()
      return wizards
        .filter(
          (w) =>
            !q ||
            w.displayName?.toLowerCase().includes(q) ||
            w.category?.toLowerCase().includes(q),
        )
        .map((w) => ({
          id: w.typeId ?? '',
          label: w.displayName ?? '',
          description: w.category,
        }))
    },
    [space],
  )

  const createFromWizard = useCallback(
    async (typeId: string) => {
      const wizards = wizardCache.current
      if (!wizards) return
      const wizard = wizards.find((w) => w.typeId === typeId)
      if (!wizard) return

      if (wizard.persistent && wizard.wizardTypeId) {
        const suffix = Date.now().toString(36)
        const wizardKey = `${wizard.wizardTypeId}/${suffix}`
        const opData = CreateWizardObjectOp.toBinary({
          objectKey: wizardKey,
          wizardTypeId: wizard.wizardTypeId,
          targetTypeId: wizard.typeId ?? '',
          targetKeyPrefix: wizard.keyPrefix ?? '',
          name: wizard.defaultNamePattern ?? '',
          timestamp: new Date(),
        })
        await spaceWorld.applyWorldOp(CREATE_WIZARD_OBJECT_OP_ID, opData, '')
        navigateToObjects([wizardKey])
        return
      }

      if (!wizard.createOpId || !wizard.keyPrefix) return
      const builder = lookupCreateOpBuilder(wizard.createOpId)
      if (!builder) return
      const name = wizard.defaultNamePattern || wizard.displayName || 'Untitled'
      const objectKey = buildObjectKey(
        wizard.keyPrefix,
        name,
        existingObjectKeys,
      )
      if (wizard.createOpId === INIT_NOTEBOOK_OP_ID) {
        await createNotebookClientSide(
          spaceWorld,
          objectKey,
          buildNotebookUnixfsObjectKey(objectKey),
          name,
          new Date(),
        )
        toast.success(`Created ${name}`)
        navigateToObjects([objectKey])
        return
      }

      if (wizard.createOpId === CREATE_DOCS_OP_ID) {
        await createDocsClientSide(spaceWorld, objectKey, name, '', new Date())
        toast.success(`Created ${name}`)
        navigateToObjects([objectKey])
        return
      }

      if (wizard.createOpId === CREATE_BLOG_OP_ID) {
        await createBlogClientSide(
          spaceWorld,
          objectKey,
          name,
          '',
          '',
          new Date(),
        )
        toast.success(`Created ${name}`)
        navigateToObjects([objectKey])
        return
      }

      const opData = builder(objectKey, name)
      await spaceWorld.applyWorldOp(wizard.createOpId, opData, '')
      toast.success(`Created ${name}`)
      navigateToObjects([objectKey])
    },
    [existingObjectKeys, spaceWorld, navigateToObjects],
  )

  const handleCreateObject = useCallback(
    (args: Record<string, string>) => {
      if (args.subItemId) {
        void createFromWizard(args.subItemId)
        return
      }
      openCommand('spacewave.create-object')
    },
    [openCommand, createFromWizard],
  )

  useCommand({
    commandId: 'spacewave.create-object',
    label: 'Create Object',
    description: 'Create a new object in this space',
    menuPath: 'File/Create Object',
    menuGroup: 2,
    menuOrder: 0,
    keybinding: 'CmdOrCtrl+N',
    active: isTabActive,
    hasSubItems: true,
    subItems: createObjectSubItems,
    handler: handleCreateObject,
  })

  return null
}
