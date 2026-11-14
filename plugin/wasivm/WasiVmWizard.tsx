import { useCallback, useState } from 'react'
import { LuTerminal } from 'react-icons/lu'

import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { Root } from '@s4wave/sdk/root/root.js'
import { UnixFSTypeID } from '@s4wave/sdk/unixfs/type.js'
import {
  CREATE_WASIVM_OP_ID,
  DEFAULT_WASIVM_INIT_PATH,
} from '@s4wave/sdk/vm/wasivm.js'
import { CreateWasiVmOp } from '@s4wave/sdk/vm/wasivm.pb.js'
import { listObjectsWithType } from '@s4wave/sdk/world/types/types.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { toast } from '@s4wave/web/ui/toaster.js'

import { buildObjectKey } from '@s4wave/app/space/create-op-builders.js'
import { useWizardState } from '@s4wave/app/wizard/useWizardState.js'
import { useWizardFinish } from '@s4wave/app/wizard/wizard-finish.js'
import { WizardField } from '@s4wave/app/wizard/WizardField.js'
import { WizardFieldFrame } from '@s4wave/app/wizard/WizardFieldFrame.js'
import { wizardInputClassName } from '@s4wave/app/wizard/wizard-field-styles.js'
import { WizardShell } from '@s4wave/app/wizard/WizardShell.js'

import {
  formatWasiVmImageLabel,
  linkCdnWasiVmImage,
  listCdnWasiVmImages,
} from './wasivm-images.js'

// imageChoicePrefix and fsChoicePrefix mark a root filesystem choice as a
// published image or a filesystem in this Space.
const imageChoicePrefix = 'image:'
const fsChoicePrefix = 'fs:'

// WasiVmWizard creates a WasiVM: its name, the root filesystem it boots,
// either a published image or a UnixFS object in the Space, and the init
// program it runs.
export default function WasiVmWizard(props: ObjectViewerComponentProps) {
  const ws = useWizardState(props, undefined)
  const { state } = ws
  const [rootfsChoice, setRootfsChoice] = useState('')
  const [initPath, setInitPath] = useState('')

  // List the published images, newest first, and the filesystems in this
  // Space; the newest published image is the default choice. When the CDN is unreachable only
  // the Space filesystems are listed.
  const rootResource = useRootResource()
  const cdnImages =
    useResource(
      rootResource,
      (root: Root, signal: AbortSignal) => listCdnWasiVmImages(root, signal),
      [],
    ).value ?? []
  const unixfsKeys =
    useResource(
      props.worldState,
      (world: IWorldState, signal: AbortSignal) =>
        listObjectsWithType(world, UnixFSTypeID, signal),
      [],
    ).value ?? []
  const choice =
    rootfsChoice ||
    (cdnImages[0]
      ? imageChoicePrefix + cdnImages[0].objectKey
      : unixfsKeys[0]
        ? fsChoicePrefix + unixfsKeys[0]
        : '')
  const image = cdnImages.find(
    (entry) => imageChoicePrefix + entry.objectKey === choice,
  )
  const rootfsKey = choice.startsWith(fsChoicePrefix)
    ? choice.slice(fsChoicePrefix.length)
    : ''
  const name = ws.localName.trim()
  const root = rootResource.value

  // Link the published image when one is chosen, before the write
  // transaction opens since the link reads the CDN and skips an image already
  // linked; then create the VM and replace the wizard with it in one list.
  const finish = useWizardFinish(ws)
  const handleFinalize = useCallback(async () => {
    if (!state) return
    await finish.finish(
      async () => {
        if (!name || (!image && !rootfsKey)) {
          throw new Error('Name the VM and choose an image')
        }
        await ws.persistDraftState()
        if (image) {
          if (!root) throw new Error('root resource not ready')
          await linkCdnWasiVmImage(
            ws.spaceWorld,
            root,
            image,
            ws.sessionPeerId,
            new AbortController().signal,
          )
        }
        const objectKey = await buildObjectKey(
          ws.spaceWorld,
          'vm/wasivm/',
          name,
        )
        const opData = CreateWasiVmOp.toBinary({
          objectKey,
          name,
          rootfsObjectKey: rootfsKey,
          imageObjectKey: image?.objectKey ?? '',
          config: { initPath: initPath.trim() },
          timestamp: new Date(),
        })
        return {
          creates: [{ kind: 'op', opId: CREATE_WASIVM_OP_ID, opData }],
          target: { openKey: objectKey, indexPath: objectKey, name },
        }
      },
      (target) => {
        toast.success(`Created ${target.name}`)
        ws.navigateToObjects([target.openKey])
      },
    )
  }, [finish, state, ws, name, image, rootfsKey, root, initPath])

  if (!state) {
    return (
      <div className="flex flex-1 items-center justify-center p-4">
        <LoadingCard
          view={{
            state: 'active',
            title: 'Loading wizard',
            detail: 'Preparing the virtual machine settings.',
          }}
          className="w-full max-w-sm"
        />
      </div>
    )
  }

  return (
    <WizardShell
      icon={<LuTerminal />}
      title="New WebAssembly VM"
      step={state.step ?? 0}
      localName={ws.localName}
      onUpdateName={ws.handleUpdateName}
      onBack={() => void ws.handleBack()}
      onCancel={() => void ws.handleCancel()}
      nameLabel="VM name"
      finish={finish.state}
      onFinalize={() => void handleFinalize()}
      canFinalize={!!name && (!!image || !!rootfsKey)}
    >
      <WizardFieldFrame
        label="Root filesystem"
        help={
          !choice
            ? 'Add a filesystem to this Space first; the VM runs its programs.'
            : 'The VM runs on a copy of this filesystem; its writes are discarded when it stops.'
        }
      >
        <select
          value={choice}
          disabled={!choice}
          onChange={(e) => setRootfsChoice(e.target.value)}
          className={`${wizardInputClassName} w-full rounded-md border px-2`}
        >
          {cdnImages.length !== 0 && (
            <optgroup label="Published images">
              {cdnImages.map((entry) => (
                <option
                  key={entry.objectKey}
                  value={imageChoicePrefix + entry.objectKey}
                >
                  {formatWasiVmImageLabel(entry.image)}
                </option>
              ))}
            </optgroup>
          )}
          {unixfsKeys.length !== 0 && (
            <optgroup label="Filesystems in this Space">
              {unixfsKeys.map((key) => (
                <option key={key} value={fsChoicePrefix + key}>
                  {key}
                </option>
              ))}
            </optgroup>
          )}
        </select>
      </WizardFieldFrame>
      <WizardField
        label="Init program"
        help="The WebAssembly program the VM starts first, as a path in its filesystem."
        variant="compactMono"
        placeholder={DEFAULT_WASIVM_INIT_PATH}
        value={initPath}
        onChange={(e) => setInitPath(e.target.value)}
      />
    </WizardShell>
  )
}
