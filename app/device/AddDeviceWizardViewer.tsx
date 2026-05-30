import { useCallback, useMemo, useState, type ReactNode } from 'react'
import {
  LuCircleCheck,
  LuClipboardCheck,
  LuHardDrive,
  LuMonitor,
  LuTerminal,
} from 'react-icons/lu'

import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import {
  SpaceLinkCallback,
  SpaceLinkCompletionMode,
  type ApproveSpaceLinkRequest,
  type ApproveSpaceLinkResponse,
  type PreviewSpaceLinkRequest,
  type PreviewSpaceLinkResponse,
} from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import {
  SessionContext,
  SharedObjectContext,
} from '@s4wave/web/contexts/contexts.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import { CopyButton } from '@s4wave/web/ui/CopyButton.js'
import { Button } from '@s4wave/web/ui/button.js'
import { parseObjectUri } from '@s4wave/sdk/space/object-uri.js'
import { DeviceTypeID } from '@s4wave/sdk/device/device.js'

import { applySpaceIndexPath } from '../space/space-settings.js'
import { useWizardState } from '../wizard/useWizardState.js'
import { WizardShell } from '../wizard/WizardShell.js'
import { ComputersDashboardTypeID } from './computers.js'

interface AddDeviceWizardConfig {
  ticket?: string
  completion?: string
  preview?: {
    label?: string
    agentPeerId?: string
    targetHint?: string
    expiresAt?: string
  }
}

interface SpaceLinkSession {
  spacewave: {
    previewSpaceLink: (
      request: PreviewSpaceLinkRequest,
      abortSignal?: AbortSignal,
    ) => Promise<PreviewSpaceLinkResponse>
    approveSpaceLink: (
      request: ApproveSpaceLinkRequest,
      abortSignal?: AbortSignal,
    ) => Promise<ApproveSpaceLinkResponse>
  }
}

export { AddDeviceWizardTypeID } from './add-device-wizard.js'

export function AddDeviceWizardViewer(props: ObjectViewerComponentProps) {
  const ws = useWizardState(props, undefined)
  const { state } = ws
  const currentStep = state?.step ?? 0
  const session = useResourceValue(SessionContext.useContext()) as
    | SpaceLinkSession
    | undefined
  const sharedObject = useResourceValue(SharedObjectContext.useContext())
  const sharedObjectId = sharedObject?.meta.sharedObjectId ?? ''
  const { spaceState } = SpaceContainerContext.useContext()
  const [busy, setBusy] = useState(false)
  const config = useMemo(() => decodeConfig(ws.configData), [ws.configData])
  const ticket = config.ticket ?? ''
  const completion = config.completion ?? ''
  const setupCommand = buildSetupCommand(
    ws.localName || 'Device',
    sharedObjectId,
  )
  const completeCommand = completion
    ? `spacewave device complete --completion ${quoteShellArg(completion)}`
    : 'spacewave device complete --completion <completion>'
  const deviceObjects = useMemo(
    () =>
      (spaceState.worldContents?.objects ?? []).filter(
        (obj) => obj.objectType === DeviceTypeID,
      ),
    [spaceState.worldContents?.objects],
  )
  const dashboardKey = useMemo(
    () =>
      (spaceState.worldContents?.objects ?? []).find(
        (obj) => obj.objectType === ComputersDashboardTypeID,
      )?.objectKey ?? '',
    [spaceState.worldContents?.objects],
  )

  const handleConfigUpdate = useCallback(
    async (next: AddDeviceWizardConfig, step?: number) => {
      ws.handleConfigDataChange(encodeConfig(next))
      const handle = ws.wizardResource.value
      if (!handle) return
      const update: { configData: Uint8Array; step?: number } = {
        configData: encodeConfig(next),
      }
      if (step !== undefined) update.step = step
      await handle.updateState(update)
    },
    [ws],
  )

  const handleTicketChange = useCallback(
    (value: string) => {
      ws.handleConfigDataChange(encodeConfig({ ...config, ticket: value }))
    },
    [config, ws],
  )

  const handleNameNext = useCallback(async () => {
    const handle = ws.wizardResource.value
    if (!handle) return
    await ws.persistDraftState()
    await handle.updateState({ step: 1 })
  }, [ws])

  const handleFinalize = useCallback(async () => {
    if (!state || ws.creating) return
    ws.setCreating(true)
    try {
      await ws.persistDraftState()
      if (dashboardKey) {
        await replaceSpaceIndexIfWizardIsCurrent(ws, dashboardKey)
      }
      await ws.spaceWorld.deleteObject(ws.objectKey)
      const deviceKey = deviceObjects[0]?.objectKey ?? ''
      const openKey = deviceKey || dashboardKey
      if (openKey) ws.navigateToObjects([openKey])
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : 'Failed to finish Add Device',
      )
    } finally {
      ws.setCreating(false)
    }
  }, [dashboardKey, deviceObjects, state, ws])

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
              title: 'Loading Add Device',
              detail: 'Preparing the Device setup wizard.',
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
          Add Device
        </>
      }
      step={currentStep}
      totalSteps={3}
      localName={ws.localName || 'Device'}
      onUpdateName={ws.handleUpdateName}
      onBack={() => void ws.handleBack()}
      onCancel={handleCancel}
      nameLabel="Device Name"
      namePlaceholder="Build server"
      nameStep={0}
      creating={ws.creating}
      createLabel="Open device"
      creatingLabel="Opening..."
      onFinalize={() => void handleFinalize()}
      canFinalize={!!completion}
      onNext={currentStep === 0 ? () => void handleNameNext() : undefined}
      finalizeStep={2}
    >
      {currentStep === 1 && (
        <section className="space-y-3">
          <CommandPanel
            title="Local CLI"
            command={setupCommand}
            icon={<LuTerminal className="size-3.5" />}
          />
          <CommandPanel
            title="Container daemon"
            command={setupCommand}
            icon={<LuMonitor className="size-3.5" />}
          />
          <div className="border-foreground/6 bg-background-card/30 rounded-lg border p-3.5">
            <label className="text-foreground text-xs font-medium select-none">
              SpaceLink Ticket
            </label>
            <textarea
              value={ticket}
              onChange={(e) => handleTicketChange(e.target.value)}
              placeholder="Paste the base64 ticket from spacewave device setup"
              className="border-foreground/10 bg-background/20 text-foreground placeholder:text-foreground-alt/40 focus-visible:border-brand/50 focus-visible:ring-brand/15 mt-2 min-h-24 w-full rounded-md border p-2 font-mono text-xs outline-none"
            />
            {config.preview && (
              <div className="text-foreground-alt/60 mt-2 grid gap-1 text-xs">
                <span>{config.preview.label || 'Device'}</span>
                {config.preview.agentPeerId && (
                  <span className="truncate font-mono">
                    {config.preview.agentPeerId}
                  </span>
                )}
              </div>
            )}
            <Button
              size="sm"
              onClick={() =>
                void approveTicket({
                  ws,
                  sharedObjectId,
                  ticket,
                  config,
                  session,
                  setBusy,
                  persist: handleConfigUpdate,
                })
              }
              disabled={busy || !ticket.trim()}
              className="border-brand/30 bg-brand/10 hover:border-brand/50 hover:bg-brand/15 text-foreground mt-3 h-7 rounded-md border px-3 text-xs transition-all duration-150"
            >
              <LuClipboardCheck className="size-3.5" />
              {busy ? 'Approving...' : 'Approve'}
            </Button>
          </div>
        </section>
      )}
      {currentStep === 2 && (
        <section className="space-y-3">
          <div className="border-brand/20 bg-brand/5 rounded-lg border p-3.5">
            <div className="text-foreground flex items-center gap-2 text-sm font-medium">
              <LuCircleCheck className="size-4" />
              Approval ready
            </div>
            <p className="text-foreground-alt/70 mt-1 text-xs leading-relaxed">
              Import the completion on the Device daemon, then open the Device
              object after it appears.
            </p>
          </div>
          <CommandPanel
            title="Complete setup"
            command={completeCommand}
            icon={<LuTerminal className="size-3.5" />}
          />
          {deviceObjects.length > 0 && (
            <div className="border-foreground/6 bg-background-card/30 rounded-lg border p-3.5">
              <div className="text-foreground text-xs font-medium">
                Device Objects
              </div>
              <div className="mt-2 space-y-1">
                {deviceObjects.map((objectInfo) => (
                  <button
                    key={objectInfo.objectKey}
                    type="button"
                    onClick={() =>
                      objectInfo.objectKey &&
                      ws.navigateToObjects([objectInfo.objectKey])
                    }
                    className="hover:bg-foreground/5 text-foreground flex w-full min-w-0 rounded px-2 py-1 text-left text-xs"
                  >
                    <span className="truncate">{objectInfo.objectKey}</span>
                  </button>
                ))}
              </div>
            </div>
          )}
        </section>
      )}
    </WizardShell>
  )
}

function CommandPanel({
  title,
  command,
  icon,
}: {
  title: string
  command: string
  icon: ReactNode
}) {
  return (
    <div className="border-foreground/6 bg-background-card/30 rounded-lg border p-3.5">
      <div className="mb-2 flex items-center justify-between gap-2">
        <div className="text-foreground flex min-w-0 items-center gap-2 text-xs font-medium">
          {icon}
          <span>{title}</span>
        </div>
        <CopyButton text={command} label={`Copy ${title}`} />
      </div>
      <pre className="border-foreground/8 bg-background/40 text-foreground-alt overflow-x-auto rounded-md border p-2 font-mono text-[0.7rem] leading-relaxed">
        {command}
      </pre>
    </div>
  )
}

async function approveTicket({
  ws,
  sharedObjectId,
  ticket,
  config,
  session,
  setBusy,
  persist,
}: {
  ws: ReturnType<typeof useWizardState>
  sharedObjectId: string
  ticket: string
  config: AddDeviceWizardConfig
  session: SpaceLinkSession | undefined
  setBusy: (busy: boolean) => void
  persist: (config: AddDeviceWizardConfig, step?: number) => Promise<void>
}) {
  if (!sharedObjectId) {
    toast.error('Open this wizard from a mounted Space before approval')
    return
  }
  if (!session) {
    toast.error('Spacewave session is not ready')
    return
  }
  setBusy(true)
  try {
    await ws.persistDraftState()
    const ticketBytes = base64ToBytes(ticket)
    const preview = await session.spacewave.previewSpaceLink({
      ticket: ticketBytes,
    })
    if (
      preview.completionMode !==
      SpaceLinkCompletionMode.SpaceLinkCompletionMode_CLI
    ) {
      throw new Error('Device setup requires CLI SpaceLink completion')
    }
    const approval = await session.spacewave.approveSpaceLink({
      ticket: ticketBytes,
      resourceId: new TextEncoder().encode(sharedObjectId),
    })
    if (!approval.completion) {
      throw new Error('SpaceLink approval did not return completion data')
    }
    if (
      approval.completionMode !==
      SpaceLinkCompletionMode.SpaceLinkCompletionMode_CLI
    ) {
      throw new Error('Device setup requires CLI SpaceLink completion')
    }
    await persist(
      {
        ...config,
        ticket,
        completion: bytesToBase64(
          SpaceLinkCallback.toBinary(approval.completion),
        ),
        preview: {
          label: preview.label,
          agentPeerId: bytesToDisplay(preview.agentPeerId),
          targetHint: decodeUTF8(preview.targetHint),
          expiresAt: preview.expiresAt?.toString(),
        },
      },
      2,
    )
    toast.success('Device approved')
  } catch (err) {
    toast.error(err instanceof Error ? err.message : 'Failed to approve ticket')
  } finally {
    setBusy(false)
  }
}

async function replaceSpaceIndexIfWizardIsCurrent(
  ws: ReturnType<typeof useWizardState>,
  dashboardKey: string,
) {
  if (
    parseObjectUri(ws.spaceSettings?.indexPath ?? '').objectKey !== ws.objectKey
  ) {
    return
  }
  await applySpaceIndexPath(
    ws.spaceWorld,
    ws.spaceSettings,
    dashboardKey,
    ws.sessionPeerId,
  )
}

function decodeConfig(data: Uint8Array | undefined): AddDeviceWizardConfig {
  if (!data?.length) return {}
  try {
    return JSON.parse(new TextDecoder().decode(data)) as AddDeviceWizardConfig
  } catch {
    return {}
  }
}

function encodeConfig(config: AddDeviceWizardConfig): Uint8Array {
  return new TextEncoder().encode(JSON.stringify(config))
}

function buildSetupCommand(label: string, sharedObjectId: string): string {
  const parts = [
    'spacewave',
    'device',
    'setup',
    '--label',
    quoteShellArg(label),
  ]
  if (sharedObjectId) {
    parts.push('--target-hint', quoteShellArg(sharedObjectId))
  }
  return parts.join(' ')
}

function quoteShellArg(value: string): string {
  if (/^[a-zA-Z0-9_./:@%+=,-]+$/.test(value)) return value
  return `'${value.replaceAll("'", "'\"'\"'")}'`
}

function base64ToBytes(value: string): Uint8Array {
  const clean = value.trim()
  const binary = atob(clean)
  return Uint8Array.from(binary, (char) => char.charCodeAt(0))
}

function bytesToBase64(bytes: Uint8Array): string {
  let binary = ''
  for (const byte of bytes) binary += String.fromCharCode(byte)
  return btoa(binary)
}

function bytesToDisplay(bytes: Uint8Array | undefined): string {
  if (!bytes?.length) return ''
  return bytesToBase64(bytes)
}

function decodeUTF8(bytes: Uint8Array | undefined): string {
  if (!bytes?.length) return ''
  return new TextDecoder().decode(bytes)
}
