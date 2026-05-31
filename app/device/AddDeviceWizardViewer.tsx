import { useCallback, useMemo, useState, type ReactNode } from 'react'
import {
  LuCircleCheck,
  LuClipboardCheck,
  LuHardDrive,
  LuKeyRound,
  LuMonitor,
  LuServer,
  LuTerminal,
} from 'react-icons/lu'

import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import {
  SpaceLinkCallback,
  SpaceLinkCompletionMode,
} from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import {
  SessionContext,
  SharedObjectContext,
  SpaceContext,
} from '@s4wave/web/contexts/contexts.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import { CopyButton } from '@s4wave/web/ui/CopyButton.js'
import { Button } from '@s4wave/web/ui/button.js'
import { Input } from '@s4wave/web/ui/input.js'
import { parseObjectUri } from '@s4wave/sdk/space/object-uri.js'
import { DeviceTypeID } from '@s4wave/sdk/device/device.js'
import {
  SecretKindSSHPassword,
  SecretKindSSHPrivateKey,
  SecretKindSSHPassphrase,
  SSHPrivateKeyContentType,
  SSHTextCredentialContentType,
} from '@s4wave/sdk/secret/secret.js'
import type { Session } from '@s4wave/sdk/session/session.js'
import type { Space } from '@s4wave/sdk/space/space.js'
import { CREATE_SSH_HOST_OP_ID } from '@s4wave/sdk/sshhost/create-ssh-host.js'
import {
  CreateSshHostOp,
  type SshHost,
  type SshHostCredentialRefs,
  type SshHostKeyPin,
} from '@s4wave/sdk/sshhost/sshhost.pb.js'
import { CREATE_TERMINAL_OP_ID } from '@s4wave/sdk/terminal/create-terminal.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'

import { applySpaceIndexPath } from '../space/space-settings.js'
import { buildObjectKey } from '../space/create-op-builders.js'
import { useWizardState } from '../wizard/useWizardState.js'
import { WizardShell } from '../wizard/WizardShell.js'
import { ComputersDashboardTypeID } from './computers.js'
import { buildCreateSshHostTerminalOpData } from './terminal-action.js'

const DEFAULT_SSH_PORT = 22

type AddDeviceWizardMode = 'spacelink' | 'ssh'
type SshAuthMode = 'password' | 'private-key'
type SshSetupMode = 'host' | 'install-agent'

interface SshSetupStatus {
  state?: 'terminal_created' | 'failed'
  hostObjectKey?: string
  terminalObjectKey?: string
  message?: string
}

interface SshHostWizardConfig {
  host?: string
  port?: number
  username?: string
  authMode?: SshAuthMode
  setupMode?: SshSetupMode
  hostKeyAlgorithm?: string
  hostKeyFingerprint?: string
  hostKeyPublicKey?: string
  hostObjectKey?: string
  terminalObjectKey?: string
  setupStatus?: SshSetupStatus
}

interface SshCredentialDraft {
  password: string
  privateKey: string
  passphrase: string
}

interface AddDeviceWizardConfig {
  mode?: AddDeviceWizardMode
  ticket?: string
  completion?: string
  preview?: {
    label?: string
    agentPeerId?: string
    targetHint?: string
    expiresAt?: string
  }
  ssh?: SshHostWizardConfig
}

export { AddDeviceWizardTypeID } from './add-device-wizard.js'

export function AddDeviceWizardViewer(props: ObjectViewerComponentProps) {
  const ws = useWizardState(props, undefined)
  const { state } = ws
  const currentStep = state?.step ?? 0
  const space = useResourceValue(SpaceContext.useContext())
  const session = useResourceValue(SessionContext.useContext()) as
    | Session
    | undefined
  const { sessionInfo } = useSessionInfo(session)
  const sharedObject = useResourceValue(SharedObjectContext.useContext())
  const sharedObjectId = sharedObject?.meta.sharedObjectId ?? ''
  const { spaceState } = SpaceContainerContext.useContext()
  const [busy, setBusy] = useState(false)
  const [sshCredentialDraft, setSshCredentialDraft] =
    useState<SshCredentialDraft>({
      password: '',
      privateKey: '',
      passphrase: '',
    })
  const config = useMemo(() => decodeConfig(ws.configData), [ws.configData])
  const mode = config.mode ?? 'spacelink'
  const sshConfig = useMemo(() => config.ssh ?? {}, [config.ssh])
  const sshAuthMode = sshConfig.authMode ?? 'password'
  const sshSetupMode = sshConfig.setupMode ?? 'host'
  const sshInstallCommand = buildSshInstallCommand(
    ws.localName || 'Device',
    sharedObjectId,
  )
  const isSshInstallMode = mode === 'ssh' && sshSetupMode === 'install-agent'
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

  const handleModeChange = useCallback(
    (mode: AddDeviceWizardMode) => {
      ws.handleConfigDataChange(encodeConfig({ ...config, mode }))
    },
    [config, ws],
  )

  const handleTicketChange = useCallback(
    (value: string) => {
      ws.handleConfigDataChange(
        encodeConfig({ ...config, mode, ticket: value }),
      )
    },
    [config, mode, ws],
  )

  const handleSshConfigChange = useCallback(
    (patch: Partial<SshHostWizardConfig>) => {
      ws.handleConfigDataChange(
        encodeConfig({
          ...config,
          mode: 'ssh',
          ssh: { ...sshConfig, ...patch },
        }),
      )
    },
    [config, sshConfig, ws],
  )

  const handleNameNext = useCallback(async () => {
    const handle = ws.wizardResource.value
    if (!handle) return
    await ws.persistDraftState()
    await handle.updateState({ step: 1 })
  }, [ws])

  const handleFinishSpaceLink = useCallback(async () => {
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

  const createSshHostTerminalObjects = useCallback(
    async (terminalCommand?: string) => {
      if (!state) throw new Error('Wizard state is not ready')
      if (!space) throw new Error('Space resource is not ready')
      const readerPublicKeyPem =
        sessionInfo?.cryptoInfo?.publicKeyPem?.trim() ?? ''
      if (!readerPublicKeyPem) {
        throw new Error('Session public key is not available')
      }

      const label = (ws.localName || state.name || 'SSH Host').trim()
      const timestamp = new Date()
      const existingObjectKeys = new Set(ws.existingObjectKeys)
      const hostObjectKey = buildObjectKey(
        'ssh-host/',
        label,
        existingObjectKeys,
      )
      existingObjectKeys.add(hostObjectKey)

      const credentials = await createSshCredentialSecrets({
        space,
        label,
        config: sshConfig,
        draft: sshCredentialDraft,
        existingObjectKeys,
        readerPublicKeyPem,
      })
      const hostKeyPins = buildSshHostKeyPins(
        sshConfig,
        ws.sessionPeerId,
        timestamp,
      )
      const endpoint = {
        host: sshConfig.host?.trim(),
        port: normalizeSshPort(sshConfig.port),
        username: sshConfig.username?.trim(),
      }
      const host: SshHost = {
        label,
        endpoint,
        credentials,
        hostKeyPins,
        createdAt: timestamp,
        updatedAt: timestamp,
      }
      await ws.spaceWorld.applyWorldOp(
        CREATE_SSH_HOST_OP_ID,
        CreateSshHostOp.toBinary({
          objectKey: hostObjectKey,
          label,
          endpoint,
          credentials,
          hostKeyPins,
          timestamp,
        }),
        ws.sessionPeerId,
      )

      const terminal = buildCreateSshHostTerminalOpData({
        host,
        hostObjectKey,
        existingObjectKeys,
        command: terminalCommand,
      })
      if (terminal) {
        existingObjectKeys.add(terminal.objectKey)
        await ws.spaceWorld.applyWorldOp(
          CREATE_TERMINAL_OP_ID,
          terminal.opData,
          ws.sessionPeerId,
        )
      }
      return {
        hostObjectKey,
        terminalObjectKey: terminal?.objectKey ?? '',
      }
    },
    [
      sessionInfo?.cryptoInfo?.publicKeyPem,
      space,
      sshConfig,
      sshCredentialDraft,
      state,
      ws,
    ],
  )

  const handleCreateSshHost = useCallback(async () => {
    if (!state || ws.creating) return
    const error = getSshCreateError(sshConfig, sshCredentialDraft)
    if (error) {
      toast.error(error)
      return
    }

    ws.setCreating(true)
    try {
      const { hostObjectKey, terminalObjectKey } =
        await createSshHostTerminalObjects()
      if (dashboardKey) {
        await replaceSpaceIndexIfWizardIsCurrent(ws, dashboardKey)
      }
      await ws.spaceWorld.deleteObject(ws.objectKey)
      toast.success('SSH Host added')
      ws.navigateToObjects([terminalObjectKey || hostObjectKey])
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to add SSH Host')
    } finally {
      ws.setCreating(false)
    }
  }, [
    createSshHostTerminalObjects,
    dashboardKey,
    sshConfig,
    sshCredentialDraft,
    state,
    ws,
  ])

  const handleOpenSshInstaller = useCallback(async () => {
    if (!state || ws.creating) return
    const error = getSshCreateError(sshConfig, sshCredentialDraft)
    if (error) {
      toast.error(error)
      return
    }

    ws.setCreating(true)
    try {
      const { hostObjectKey, terminalObjectKey } =
        await createSshHostTerminalObjects(sshInstallCommand)
      await handleConfigUpdate({
        ...config,
        mode: 'ssh',
        ssh: {
          ...sshConfig,
          setupMode: 'install-agent',
          setupStatus: {
            state: 'terminal_created',
            hostObjectKey,
            terminalObjectKey,
            message: 'installer terminal opened',
          },
        },
      })
      toast.success('SSH installer opened')
      ws.navigateToObjects([terminalObjectKey || hostObjectKey])
    } catch (err) {
      const message =
        err instanceof Error ? err.message : 'Failed to open SSH installer'
      try {
        await handleConfigUpdate({
          ...config,
          mode: 'ssh',
          ssh: {
            ...sshConfig,
            setupMode: 'install-agent',
            setupStatus: {
              state: 'failed',
              message,
            },
          },
        })
      } catch {
        // Preserve the primary setup failure as the visible error.
      }
      toast.error(message)
    } finally {
      ws.setCreating(false)
    }
  }, [
    config,
    createSshHostTerminalObjects,
    handleConfigUpdate,
    sshConfig,
    sshCredentialDraft,
    sshInstallCommand,
    state,
    ws,
  ])

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
      totalSteps={mode === 'ssh' ? 2 : 3}
      localName={ws.localName || 'Device'}
      onUpdateName={ws.handleUpdateName}
      onBack={() => void ws.handleBack()}
      onCancel={handleCancel}
      nameLabel={mode === 'ssh' ? 'Host Name' : 'Device Name'}
      namePlaceholder="Build server"
      nameStep={0}
      creating={ws.creating}
      createLabel={
        isSshInstallMode
          ? 'Open installer'
          : mode === 'ssh'
            ? 'Open terminal'
            : 'Open device'
      }
      creatingLabel={mode === 'ssh' ? 'Adding...' : 'Opening...'}
      onFinalize={() =>
        void (isSshInstallMode
          ? handleOpenSshInstaller()
          : mode === 'ssh'
            ? handleCreateSshHost()
            : handleFinishSpaceLink())
      }
      canFinalize={
        mode === 'ssh'
          ? !getSshCreateError(sshConfig, sshCredentialDraft)
          : !!completion
      }
      onNext={currentStep === 0 ? () => void handleNameNext() : undefined}
      canNext={!!ws.localName.trim()}
      finalizeStep={mode === 'ssh' ? 1 : 2}
    >
      {currentStep === 0 && (
        <section className="grid grid-cols-2 gap-2">
          <ModeButton
            active={mode === 'spacelink'}
            icon={<LuHardDrive className="size-3.5" />}
            label="SpaceLink"
            detail="Device agent"
            onClick={() => handleModeChange('spacelink')}
          />
          <ModeButton
            active={mode === 'ssh'}
            icon={<LuServer className="size-3.5" />}
            label="SSH Host"
            detail="No agent"
            onClick={() => handleModeChange('ssh')}
          />
        </section>
      )}
      {currentStep === 1 && (
        <>
          {mode === 'spacelink' && (
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
              <SpaceLinkApprovalPanel
                config={config}
                ticket={ticket}
                busy={busy}
                onTicketChange={handleTicketChange}
                onApprove={() =>
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
              />
            </section>
          )}
          {mode === 'ssh' && (
            <section className="space-y-3">
              <SshHostSetupForm
                config={sshConfig}
                credentialDraft={sshCredentialDraft}
                authMode={sshAuthMode}
                setupMode={sshSetupMode}
                onConfigChange={handleSshConfigChange}
                onCredentialChange={(patch) =>
                  setSshCredentialDraft((prev) => ({ ...prev, ...patch }))
                }
              />
              {isSshInstallMode && (
                <>
                  <CommandPanel
                    title="Remote setup"
                    command={sshInstallCommand}
                    icon={<LuTerminal className="size-3.5" />}
                  />
                  <SpaceLinkApprovalPanel
                    config={config}
                    ticket={ticket}
                    busy={busy}
                    onTicketChange={handleTicketChange}
                    onApprove={() =>
                      void approveTicket({
                        ws,
                        sharedObjectId,
                        ticket,
                        config,
                        session,
                        setBusy,
                        persist: handleConfigUpdate,
                        nextStep: 1,
                      })
                    }
                  />
                  {completion && (
                    <CommandPanel
                      title="Complete setup"
                      command={completeCommand}
                      icon={<LuTerminal className="size-3.5" />}
                    />
                  )}
                </>
              )}
            </section>
          )}
        </>
      )}
      {mode === 'spacelink' && currentStep === 2 && (
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

function ModeButton({
  active,
  icon,
  label,
  detail,
  onClick,
}: {
  active: boolean
  icon: ReactNode
  label: string
  detail: string
  onClick: () => void
}) {
  return (
    <button
      type="button"
      aria-pressed={active}
      onClick={onClick}
      className={[
        'flex min-w-0 items-center gap-2 rounded-lg border p-3 text-left transition-all duration-150',
        active
          ? 'border-brand/40 bg-brand/10 text-foreground'
          : 'border-foreground/8 bg-background-card/20 text-foreground-alt hover:border-foreground/16 hover:bg-foreground/5',
      ].join(' ')}
    >
      <span className="bg-foreground/5 flex size-7 shrink-0 items-center justify-center rounded-md">
        {icon}
      </span>
      <span className="min-w-0">
        <span className="block truncate text-xs font-medium">{label}</span>
        <span className="text-foreground-alt/60 block truncate text-[0.65rem]">
          {detail}
        </span>
      </span>
    </button>
  )
}

function SshHostSetupForm({
  config,
  credentialDraft,
  authMode,
  setupMode,
  onConfigChange,
  onCredentialChange,
}: {
  config: SshHostWizardConfig
  credentialDraft: SshCredentialDraft
  authMode: SshAuthMode
  setupMode: SshSetupMode
  onConfigChange: (patch: Partial<SshHostWizardConfig>) => void
  onCredentialChange: (patch: Partial<SshCredentialDraft>) => void
}) {
  const port = normalizeSshPort(config.port)
  return (
    <section className="space-y-3">
      <div className="border-foreground/6 bg-background-card/30 rounded-lg border p-3.5">
        <div className="text-foreground mb-2 flex items-center gap-2 text-xs font-medium">
          <LuServer className="size-3.5" />
          SSH Endpoint
        </div>
        <div className="grid gap-2 sm:grid-cols-[1fr_5rem_8rem]">
          <Input
            value={config.host ?? ''}
            onChange={(e) => onConfigChange({ host: e.target.value })}
            placeholder="host.example.com"
            className="border-foreground/10 bg-background/20 text-foreground placeholder:text-foreground-alt/40 focus-visible:border-brand/50 focus-visible:ring-brand/15 h-9"
          />
          <Input
            type="number"
            min={1}
            max={65535}
            value={String(port)}
            onChange={(e) =>
              onConfigChange({ port: parsePortInput(e.target.value) })
            }
            aria-label="SSH port"
            className="border-foreground/10 bg-background/20 text-foreground placeholder:text-foreground-alt/40 focus-visible:border-brand/50 focus-visible:ring-brand/15 h-9"
          />
          <Input
            value={config.username ?? ''}
            onChange={(e) => onConfigChange({ username: e.target.value })}
            placeholder="user"
            className="border-foreground/10 bg-background/20 text-foreground placeholder:text-foreground-alt/40 focus-visible:border-brand/50 focus-visible:ring-brand/15 h-9"
          />
        </div>
      </div>

      <div className="border-foreground/6 bg-background-card/30 rounded-lg border p-3.5">
        <div className="text-foreground mb-2 text-xs font-medium">
          Setup Target
        </div>
        <div className="grid grid-cols-2 gap-2">
          <AuthModeButton
            active={setupMode === 'host'}
            label="SSH Host"
            onClick={() => onConfigChange({ setupMode: 'host' })}
          />
          <AuthModeButton
            active={setupMode === 'install-agent'}
            label="Install Agent"
            onClick={() => onConfigChange({ setupMode: 'install-agent' })}
          />
        </div>
        {config.setupStatus && (
          <div className="text-foreground-alt/60 mt-2 truncate text-xs">
            {formatSshSetupStatus(config.setupStatus)}
          </div>
        )}
      </div>

      <div className="border-foreground/6 bg-background-card/30 rounded-lg border p-3.5">
        <div className="text-foreground mb-2 flex items-center gap-2 text-xs font-medium">
          <LuKeyRound className="size-3.5" />
          SSH Credential
        </div>
        <div className="mb-3 grid grid-cols-2 gap-2">
          <AuthModeButton
            active={authMode === 'password'}
            label="Password"
            onClick={() => onConfigChange({ authMode: 'password' })}
          />
          <AuthModeButton
            active={authMode === 'private-key'}
            label="Private Key"
            onClick={() => onConfigChange({ authMode: 'private-key' })}
          />
        </div>
        {authMode === 'password' ? (
          <Input
            type="password"
            value={credentialDraft.password}
            onChange={(e) => onCredentialChange({ password: e.target.value })}
            placeholder="SSH password"
            className="border-foreground/10 bg-background/20 text-foreground placeholder:text-foreground-alt/40 focus-visible:border-brand/50 focus-visible:ring-brand/15 h-9"
          />
        ) : (
          <div className="space-y-2">
            <textarea
              value={credentialDraft.privateKey}
              onChange={(e) =>
                onCredentialChange({ privateKey: e.target.value })
              }
              placeholder="-----BEGIN OPENSSH PRIVATE KEY-----"
              className="border-foreground/10 bg-background/20 text-foreground placeholder:text-foreground-alt/40 focus-visible:border-brand/50 focus-visible:ring-brand/15 min-h-28 w-full rounded-md border p-2 font-mono text-xs outline-none"
            />
            <Input
              type="password"
              value={credentialDraft.passphrase}
              onChange={(e) =>
                onCredentialChange({ passphrase: e.target.value })
              }
              placeholder="Private key passphrase"
              className="border-foreground/10 bg-background/20 text-foreground placeholder:text-foreground-alt/40 focus-visible:border-brand/50 focus-visible:ring-brand/15 h-9"
            />
          </div>
        )}
      </div>

      <div className="border-foreground/6 bg-background-card/30 rounded-lg border p-3.5">
        <div className="text-foreground mb-2 text-xs font-medium">
          Host Key Trust
        </div>
        <div className="grid gap-2 sm:grid-cols-[8rem_1fr]">
          <Input
            value={config.hostKeyAlgorithm ?? 'ssh-ed25519'}
            onChange={(e) =>
              onConfigChange({ hostKeyAlgorithm: e.target.value })
            }
            placeholder="ssh-ed25519"
            className="border-foreground/10 bg-background/20 text-foreground placeholder:text-foreground-alt/40 focus-visible:border-brand/50 focus-visible:ring-brand/15 h-9"
          />
          <Input
            value={config.hostKeyFingerprint ?? ''}
            onChange={(e) =>
              onConfigChange({ hostKeyFingerprint: e.target.value })
            }
            placeholder="SHA256:..."
            className="border-foreground/10 bg-background/20 text-foreground placeholder:text-foreground-alt/40 focus-visible:border-brand/50 focus-visible:ring-brand/15 h-9"
          />
        </div>
        <textarea
          value={config.hostKeyPublicKey ?? ''}
          onChange={(e) => onConfigChange({ hostKeyPublicKey: e.target.value })}
          placeholder="ssh-ed25519 AAAA..."
          className="border-foreground/10 bg-background/20 text-foreground placeholder:text-foreground-alt/40 focus-visible:border-brand/50 focus-visible:ring-brand/15 mt-2 min-h-16 w-full rounded-md border p-2 font-mono text-xs outline-none"
        />
      </div>
    </section>
  )
}

function AuthModeButton({
  active,
  label,
  onClick,
}: {
  active: boolean
  label: string
  onClick: () => void
}) {
  return (
    <button
      type="button"
      aria-pressed={active}
      onClick={onClick}
      className={[
        'h-8 rounded-md border px-3 text-xs transition-all duration-150',
        active
          ? 'border-brand/40 bg-brand/10 text-foreground'
          : 'border-foreground/8 bg-background/20 text-foreground-alt hover:border-foreground/16 hover:bg-foreground/5',
      ].join(' ')}
    >
      {label}
    </button>
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

function SpaceLinkApprovalPanel({
  config,
  ticket,
  busy,
  onTicketChange,
  onApprove,
}: {
  config: AddDeviceWizardConfig
  ticket: string
  busy: boolean
  onTicketChange: (value: string) => void
  onApprove: () => void
}) {
  return (
    <div className="border-foreground/6 bg-background-card/30 rounded-lg border p-3.5">
      <label className="text-foreground text-xs font-medium select-none">
        SpaceLink Ticket
      </label>
      <textarea
        value={ticket}
        onChange={(e) => onTicketChange(e.target.value)}
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
        onClick={onApprove}
        disabled={busy || !ticket.trim()}
        className="border-brand/30 bg-brand/10 hover:border-brand/50 hover:bg-brand/15 text-foreground mt-3 h-7 rounded-md border px-3 text-xs transition-all duration-150"
      >
        <LuClipboardCheck className="size-3.5" />
        {busy ? 'Approving...' : 'Approve'}
      </Button>
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
  nextStep = 2,
}: {
  ws: ReturnType<typeof useWizardState>
  sharedObjectId: string
  ticket: string
  config: AddDeviceWizardConfig
  session: Session | undefined
  setBusy: (busy: boolean) => void
  persist: (config: AddDeviceWizardConfig, step?: number) => Promise<void>
  nextStep?: number
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
      nextStep,
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

async function createSshCredentialSecrets({
  space,
  label,
  config,
  draft,
  existingObjectKeys,
  readerPublicKeyPem,
}: {
  space: Space
  label: string
  config: SshHostWizardConfig
  draft: SshCredentialDraft
  existingObjectKeys: Set<string>
  readerPublicKeyPem: string
}): Promise<SshHostCredentialRefs> {
  const authMode = config.authMode ?? 'password'
  const credentials: SshHostCredentialRefs = {}
  if (authMode === 'private-key') {
    credentials.privateKeySecretObjectKey = await createSshCredentialSecret({
      space,
      label,
      suffix: 'private key',
      kind: SecretKindSSHPrivateKey,
      contentType: SSHPrivateKeyContentType,
      value: draft.privateKey,
      existingObjectKeys,
      readerPublicKeyPem,
    })
    if (draft.passphrase.trim()) {
      credentials.passphraseSecretObjectKey = await createSshCredentialSecret({
        space,
        label,
        suffix: 'passphrase',
        kind: SecretKindSSHPassphrase,
        contentType: SSHTextCredentialContentType,
        value: draft.passphrase,
        existingObjectKeys,
        readerPublicKeyPem,
      })
    }
    return credentials
  }

  credentials.passwordSecretObjectKey = await createSshCredentialSecret({
    space,
    label,
    suffix: 'password',
    kind: SecretKindSSHPassword,
    contentType: SSHTextCredentialContentType,
    value: draft.password,
    existingObjectKeys,
    readerPublicKeyPem,
  })
  return credentials
}

async function createSshCredentialSecret({
  space,
  label,
  suffix,
  kind,
  contentType,
  value,
  existingObjectKeys,
  readerPublicKeyPem,
}: {
  space: Space
  label: string
  suffix: string
  kind: string
  contentType: string
  value: string
  existingObjectKeys: Set<string>
  readerPublicKeyPem: string
}): Promise<string> {
  const objectKey = buildObjectKey(
    'secret/',
    `${label} SSH ${suffix}`,
    existingObjectKeys,
  )
  existingObjectKeys.add(objectKey)
  await space.createSecret({
    objectKey,
    displayName: `${label} SSH ${suffix}`,
    kind,
    contentType,
    value: new TextEncoder().encode(value),
    readerPublicKeyPem: new TextEncoder().encode(readerPublicKeyPem),
  })
  return objectKey
}

function buildSshHostKeyPins(
  config: SshHostWizardConfig,
  acceptedByPeerId: string,
  acceptedAt: Date,
): SshHostKeyPin[] {
  return [
    {
      algorithm: (config.hostKeyAlgorithm ?? 'ssh-ed25519').trim(),
      publicKey: config.hostKeyPublicKey?.trim(),
      sha256Fingerprint: config.hostKeyFingerprint?.trim(),
      acceptedByPeerId,
      acceptedAt,
    },
  ]
}

function formatSshSetupStatus(status: SshSetupStatus): string {
  if (status.state === 'failed') return status.message || 'setup failed'
  if (status.terminalObjectKey) {
    return `installer terminal ${status.terminalObjectKey}`
  }
  if (status.hostObjectKey) {
    return `installer host ${status.hostObjectKey}`
  }
  return status.message || 'setup started'
}

function getSshCreateError(
  config: SshHostWizardConfig,
  draft: SshCredentialDraft,
): string {
  if (!config.host?.trim()) return 'SSH host is required'
  if (!config.username?.trim()) return 'SSH username is required'
  const port = normalizeSshPort(config.port)
  if (port < 1 || port > 65535) return 'SSH port is invalid'
  const authMode = config.authMode ?? 'password'
  if (authMode === 'private-key' && !draft.privateKey.trim()) {
    return 'SSH private key is required'
  }
  if (authMode === 'password' && !draft.password) {
    return 'SSH password is required'
  }
  if (!config.hostKeyFingerprint?.trim() && !config.hostKeyPublicKey?.trim()) {
    return 'SSH host key fingerprint or public key is required'
  }
  if (!(config.hostKeyAlgorithm ?? 'ssh-ed25519').trim()) {
    return 'SSH host key algorithm is required'
  }
  return ''
}

function normalizeSshPort(port: number | undefined): number {
  if (!Number.isFinite(port) || !port) return DEFAULT_SSH_PORT
  return Math.trunc(port)
}

function parsePortInput(value: string): number | undefined {
  const parsed = Number.parseInt(value, 10)
  if (!Number.isFinite(parsed)) return undefined
  return parsed
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

function buildSshInstallCommand(label: string, sharedObjectId: string): string {
  const setupCommand = buildSetupCommand(label, sharedObjectId)
  return [
    'if command -v spacewave >/dev/null 2>&1; then',
    `  ${setupCommand}`,
    'else',
    "  echo 'spacewave CLI not found on remote host' >&2",
    '  exit 127',
    'fi',
  ].join('\n')
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
  return btoa(Array.from(bytes, (byte) => String.fromCharCode(byte)).join(''))
}

function bytesToDisplay(bytes: Uint8Array | undefined): string {
  if (!bytes?.length) return ''
  return bytesToBase64(bytes)
}

function decodeUTF8(bytes: Uint8Array | undefined): string {
  if (!bytes?.length) return ''
  return new TextDecoder().decode(bytes)
}
