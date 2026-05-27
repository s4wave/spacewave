import { useCallback, type ReactNode } from 'react'
import { LuFolderOpen, LuHardDrive, LuUpload } from 'react-icons/lu'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import { parseObjectUri } from '@s4wave/sdk/space/object-uri.js'

import { applySpaceIndexPath } from '../space/space-settings.js'
import { useWizardState } from './useWizardState.js'
import { WizardShell } from './WizardShell.js'
import { DriveIntroTargetObjectKey } from './drive-intro.js'

export function DriveIntroWizardViewer(props: ObjectViewerComponentProps) {
  const ws = useWizardState(props, undefined)
  const { state } = ws

  const handleComplete = useCallback(async () => {
    if (!state || ws.creating) return
    ws.setCreating(true)
    try {
      await ws.persistDraftState()
      await replaceSpaceIndexIfWizardIsCurrent(ws)
      await ws.spaceWorld.deleteObject(ws.objectKey)
      ws.navigateToObjects([DriveIntroTargetObjectKey])
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to open files')
    } finally {
      ws.setCreating(false)
    }
  }, [state, ws])

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
              title: 'Loading Drive',
              detail: 'Preparing your Drive intro.',
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
          <LuHardDrive className="mr-2 size-4 shrink-0" />
          Drive
        </>
      }
      step={0}
      totalSteps={1}
      localName={ws.localName || state.name || 'My Drive'}
      onUpdateName={ws.handleUpdateName}
      onBack={() => void ws.handleBack()}
      onCancel={handleCancel}
      nameStep={1}
      creating={ws.creating}
      createLabel="Open files"
      creatingLabel="Opening..."
      onFinalize={() => void handleComplete()}
      finalizeStep={0}
    >
      <section className="border-foreground/6 bg-background-card/30 space-y-3 rounded-lg border p-3.5">
        <div>
          <h3 className="text-foreground text-sm font-semibold">
            Your Drive is ready
          </h3>
          <p className="text-foreground-alt/70 mt-1 text-xs leading-relaxed">
            Store files in this Space, organize folders, and open media or
            documents from the file list.
          </p>
        </div>
        <div className="grid gap-2 text-xs sm:grid-cols-2">
          <DriveIntroPoint
            icon={<LuUpload className="size-3.5" />}
            title="Add files"
            detail="Use upload or drag files into the browser."
          />
          <DriveIntroPoint
            icon={<LuFolderOpen className="size-3.5" />}
            title="Browse raw files"
            detail="The next screen is the generic file browser."
          />
        </div>
      </section>
    </WizardShell>
  )
}

async function replaceSpaceIndexIfWizardIsCurrent(
  ws: ReturnType<typeof useWizardState>,
) {
  if (
    parseObjectUri(ws.spaceSettings?.indexPath ?? '').objectKey !== ws.objectKey
  ) {
    return
  }
  await applySpaceIndexPath(
    ws.spaceWorld,
    ws.spaceSettings,
    DriveIntroTargetObjectKey,
    ws.sessionPeerId,
  )
}

function DriveIntroPoint({
  icon,
  title,
  detail,
}: {
  icon: ReactNode
  title: string
  detail: string
}) {
  return (
    <div className="border-foreground/6 bg-background/20 rounded-md border p-2.5">
      <div className="text-foreground flex items-center gap-1.5 font-medium">
        {icon}
        <span>{title}</span>
      </div>
      <p className="text-foreground-alt/60 mt-1 leading-relaxed">{detail}</p>
    </div>
  )
}
