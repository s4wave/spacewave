import { useCallback, useMemo, useState, type ReactNode } from 'react'
import {
  LuCircleCheck,
  LuClipboardCheck,
  LuHardDrive,
  LuKeyRound,
  LuLogIn,
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
import { cn } from '@s4wave/web/style/utils.js'
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
import { useNavigate } from '@s4wave/web/router/router.js'

import { applySpaceIndexPath } from '../space/space-settings.js'
import { buildObjectKey } from '../space/create-op-builders.js'
import { useWizardState } from '../wizard/useWizardState.js'
import type { UseWizardStateResult } from '../wizard/useWizardState.js'
import { WizardShell } from '../wizard/WizardShell.js'
import { WizardField } from '../wizard/WizardField.js'
import { WizardTextareaField } from '../wizard/WizardTextareaField.js'
import { ComputersDashboardTypeID } from './computers.js'
import { buildCreateSshHostTerminalOpData } from './terminal-action.js'

const DEFAULT_SSH_PORT = 22
const DEVICE_APPROVAL_CLOUD_REQUIRED =
  'Device linking requires a Spacewave Cloud session. Sign in or create an account to continue.'

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
  const {
    sessionInfo,
    loading: sessionInfoLoading,
    supportsDeviceApproval,
  } = useSessionInfo(session)
  const sharedObject = useResourceValue(SharedObjectContext.useContext())
  const sharedObjectId = sharedObject?.meta.sharedObjectId ?? ''
  const { spaceState } = SpaceContainerContext.useContext()
  const navigate = useNavigate()
  const handleSignIn = useCallback(() => {
    navigate({ path: '/login' })
  }, [navigate])
  const [busy, setBusy] = useState(false)
  const [approvalError, setApprovalError] = useState('')
  const [operationError, setOperationError] = useState('')
  const [openingStep, setOpeningStep] = useState(false)
  const isOpeningStep = openingStep && currentStep === 0
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
    if (!handle) {
      setOperationError('Device setup is still loading. Try again.')
      return
    }
    setOperationError('')
    setOpeningStep(true)
    try {
      await ws.persistDraftState()
      await handle.updateState({ step: 1 })
    } catch {
      setOpeningStep(false)
      setOperationError(
        mode === 'ssh'
          ? 'SSH setup could not be opened. Try again.'
          : 'Device setup could not be opened. Try again.',
      )
    }
  }, [mode, ws])

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
    } catch {
      const message = 'Device setup could not be completed. Try again.'
      setOperationError(message)
      toast.error(message)
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
      setOperationError(error)
      toast.error(error)
      return
    }

    setOperationError('')
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
    } catch {
      const message = 'SSH Host could not be added. Try again.'
      setOperationError(message)
      toast.error(message)
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
      setOperationError(error)
      toast.error(error)
      return
    }

    setOperationError('')
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
    } catch {
      const message =
        'SSH installer could not be opened. Check the connection and try again.'
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
      setOperationError(message)
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

  const totalSteps = currentStep === 0 ? 3 : mode === 'ssh' ? 2 : 3
  const stepName =
    currentStep === 0
      ? 'Choose connection'
      : mode === 'ssh'
        ? 'Configure SSH'
        : currentStep === 1
          ? 'Set up Device'
          : 'Finish'
  return (
    <WizardShell
      title={
        <>
          <LuHardDrive className="mr-2 size-4 shrink-0" />
          Add Device
        </>
      }
      step={currentStep}
      totalSteps={totalSteps}
      stepName={stepName}
      localName={ws.localName || 'Device'}
      onUpdateName={ws.handleUpdateName}
      onBack={() => {
        setOpeningStep(false)
        setOperationError('')
        void ws.handleBack()
      }}
      onCancel={handleCancel}
      nameLabel={mode === 'ssh' ? 'Host Name' : 'Device Name'}
      namePlaceholder="Build server"
      nameStep={0}
      creating={ws.creating}
      createLabel={
        isSshInstallMode
          ? 'Open installer'
          : mode === 'ssh'
            ? 'Add SSH Host and open terminal'
            : 'Open Device'
      }
      creatingLabel={
        isSshInstallMode
          ? 'Opening installer…'
          : mode === 'ssh'
            ? 'Adding SSH Host and opening terminal…'
            : 'Opening Device…'
      }
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
      nextBusyLabel={
        mode === 'ssh' ? 'Opening SSH setup…' : 'Preparing Device setup…'
      }
      nextBusy={isOpeningStep}
      canNext={!!ws.localName.trim() && !isOpeningStep}
      finalizeStep={mode === 'ssh' ? 1 : 2}
      width={mode === 'ssh' ? 'wide' : 'default'}
    >
      {currentStep === 0 && (
        <section className="grid grid-cols-1 gap-2 sm:grid-cols-2">
          <ModeButton
            active={mode === 'spacelink'}
            icon={<LuHardDrive className="size-3.5" />}
            label="Managed Device"
            detail="Install Spacewave for persistent status, policy, and capabilities. 3 steps"
            onClick={() => handleModeChange('spacelink')}
          />
          <ModeButton
            active={mode === 'ssh'}
            icon={<LuServer className="size-3.5" />}
            label="SSH Host"
            detail="Connect on demand over SSH without installing an agent. 2 steps"
            onClick={() => handleModeChange('ssh')}
          />
        </section>
      )}
      {currentStep === 1 && (
        <>
          {mode === 'spacelink' && (
            <section className="space-y-3">
              <SpaceLinkApprovalPanel
                supported={supportsDeviceApproval}
                loading={sessionInfoLoading}
                onSignIn={handleSignIn}
                config={config}
                ticket={ticket}
                busy={busy}
                error={approvalError}
                onTicketChange={(value) => {
                  setApprovalError('')
                  handleTicketChange(value)
                }}
                onApprove={() => {
                  setApprovalError('')
                  void approveTicket({
                    ws,
                    sharedObjectId,
                    ticket,
                    config,
                    session,
                    setBusy,
                    setError: setApprovalError,
                    persist: handleConfigUpdate,
                  })
                }}
              />
              <div className="space-y-2">
                <div>
                  <h3 className="text-foreground text-xs font-medium">
                    Get a ticket on the Device
                  </h3>
                  <p className="text-foreground-alt/70 mt-1 text-xs leading-relaxed">
                    Run this command with the installed Local CLI or Container
                    daemon, then paste the ticket above.
                  </p>
                </div>
                <CommandPanel
                  title="Device setup command"
                  command={setupCommand}
                  icon={<LuTerminal className="size-3.5" />}
                />
              </div>
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
                    supported={supportsDeviceApproval}
                    loading={sessionInfoLoading}
                    onSignIn={handleSignIn}
                    config={config}
                    ticket={ticket}
                    busy={busy}
                    error={approvalError}
                    onTicketChange={(value) => {
                      setApprovalError('')
                      handleTicketChange(value)
                    }}
                    onApprove={() => {
                      setApprovalError('')
                      void approveTicket({
                        ws,
                        sharedObjectId,
                        ticket,
                        config,
                        session,
                        setBusy,
                        setError: setApprovalError,
                        persist: handleConfigUpdate,
                        nextStep: 1,
                      })
                    }}
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
      {(operationError || isOpeningStep) && (
        <div
          className={cn(
            'rounded-lg border p-3 text-xs leading-relaxed',
            operationError
              ? 'border-destructive/15 bg-destructive/5 text-destructive'
              : 'border-brand/20 bg-brand/5 text-foreground-alt/80',
          )}
          role={operationError ? 'alert' : 'status'}
        >
          {operationError ||
            (mode === 'ssh' ? 'Opening SSH setup…' : 'Preparing Device setup…')}
        </div>
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
      className={cn(
        'flex min-w-0 items-center gap-3 rounded-lg border p-3 text-left transition-all duration-150',
        active
          ? 'border-brand/30 bg-brand/10 text-foreground'
          : 'border-foreground/10 bg-background-card/30 text-foreground-alt hover:border-foreground/20 hover:bg-background-card/50',
      )}
    >
      <span className="bg-foreground/5 flex size-7 shrink-0 items-center justify-center rounded-md">
        {icon}
      </span>
      <span className="min-w-0">
        <span className="block truncate text-sm font-medium">{label}</span>
        <span className="text-foreground-alt/70 block text-xs leading-relaxed">
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
  const hostError = !config.host?.trim() ? 'Host is required.' : ''
  const userError = !config.username?.trim() ? 'User is required.' : ''
  const portError =
    port < 1 || port > 65535 ? 'Port must be between 1 and 65535.' : ''
  const credentialError =
    authMode === 'private-key' && !credentialDraft.privateKey.trim()
      ? 'Private key is required.'
      : ''
  const trustMode = config.hostKeyFingerprint?.trim()
    ? `Pinned to ${config.hostKeyFingerprint.trim()}`
    : 'Ask on first connection'
  const credentialKind = authMode === 'private-key' ? 'Private key' : 'Password'
  const endpoint = config.host?.trim()
    ? `${config.host.trim()}:${port}`
    : 'Host not set'

  return (
    <section className="space-y-3">
      <div className="border-foreground/6 bg-background-card/30 rounded-lg border p-3.5">
        <div className="text-foreground mb-2 flex items-center gap-1.5 text-xs font-medium">
          <LuServer className="size-3.5" />
          SSH Endpoint
        </div>
        <div className="grid gap-2 sm:grid-cols-[1fr_5rem_8rem]">
          <WizardField
            label="Host"
            value={config.host ?? ''}
            onChange={(e) => onConfigChange({ host: e.target.value })}
            placeholder="host.example.com"
            help={
              hostError ? (
                <span className="text-destructive text-xs">{hostError}</span>
              ) : undefined
            }
          />
          <WizardField
            label="Port"
            type="number"
            min={1}
            max={65535}
            value={String(port)}
            onChange={(e) =>
              onConfigChange({ port: parsePortInput(e.target.value) })
            }
            aria-label="SSH port"
            help={
              portError ? (
                <span className="text-destructive text-xs">{portError}</span>
              ) : undefined
            }
          />
          <WizardField
            label="User"
            value={config.username ?? ''}
            onChange={(e) => onConfigChange({ username: e.target.value })}
            placeholder="user"
            help={
              userError ? (
                <span className="text-destructive text-xs">{userError}</span>
              ) : undefined
            }
          />
        </div>
      </div>

      <div className="border-foreground/6 bg-background-card/30 rounded-lg border p-3.5">
        <div className="text-foreground mb-2 flex items-center gap-1.5 text-xs font-medium">
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
          <WizardField
            label="Password"
            type="password"
            value={credentialDraft.password}
            onChange={(e) => onCredentialChange({ password: e.target.value })}
            placeholder="SSH password"
            help="Leave blank if the host accepts an empty password or requires no authentication."
          />
        ) : (
          <div className="space-y-2">
            <WizardTextareaField
              label="Private key"
              value={credentialDraft.privateKey}
              onChange={(e) =>
                onCredentialChange({ privateKey: e.target.value })
              }
              placeholder="-----BEGIN OPENSSH PRIVATE KEY-----"
              className="min-h-28"
              help={
                credentialError ? (
                  <span className="text-destructive text-xs">
                    {credentialError}
                  </span>
                ) : undefined
              }
            />
            <WizardField
              label="Passphrase"
              type="password"
              value={credentialDraft.passphrase}
              onChange={(e) =>
                onCredentialChange({ passphrase: e.target.value })
              }
              placeholder="Private key passphrase (optional)"
              help="Optional for unencrypted private keys."
            />
          </div>
        )}
      </div>

      <details className="border-foreground/6 bg-background-card/30 rounded-lg border p-3.5">
        <summary className="text-foreground flex cursor-pointer items-center justify-between text-xs font-medium select-none">
          <span>Host-key trust (advanced)</span>
          <span className="text-foreground-alt/60 text-xs">optional</span>
        </summary>
        <p className="text-foreground-alt/70 mt-2 text-xs leading-relaxed">
          Trust mode: <span className="font-medium">{trustMode}</span>. Leave
          the fields empty to ask on the first connection.
        </p>
        <div className="mt-3 grid gap-2 sm:grid-cols-[8rem_1fr]">
          <WizardField
            label="Algorithm"
            value={config.hostKeyAlgorithm ?? 'ssh-ed25519'}
            onChange={(e) =>
              onConfigChange({ hostKeyAlgorithm: e.target.value })
            }
            placeholder="ssh-ed25519"
            className="font-mono text-xs"
          />
          <WizardField
            label="Fingerprint"
            value={config.hostKeyFingerprint ?? ''}
            onChange={(e) =>
              onConfigChange({ hostKeyFingerprint: e.target.value })
            }
            placeholder="SHA256:..."
            className="font-mono text-xs"
          />
        </div>
        <WizardTextareaField
          label="Public key"
          value={config.hostKeyPublicKey ?? ''}
          onChange={(e) => onConfigChange({ hostKeyPublicKey: e.target.value })}
          placeholder="[host]:22 ssh-ed25519 AAAA... or ssh-ed25519 AAAA..."
          className="min-h-16 font-mono text-xs"
          fieldClassName="mt-2"
        />
      </details>

      <div className="border-foreground/6 bg-background-card/30 rounded-lg border p-3.5">
        <div className="text-foreground mb-2 text-xs font-medium">
          What do you want to do?
        </div>
        <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
          <AuthModeButton
            active={setupMode === 'host'}
            label="Add SSH Host"
            onClick={() => onConfigChange({ setupMode: 'host' })}
          />
          <AuthModeButton
            active={setupMode === 'install-agent'}
            label="Install Agent"
            onClick={() => undefined}
            disabled
          />
        </div>
        <p className="text-foreground-alt/70 mt-2 text-xs leading-relaxed">
          Install Agent is not available until secure bootstrap is configured.
          Add the SSH Host to keep on-demand terminal access.
        </p>
      </div>

      <div className="border-foreground/6 bg-background-card/30 rounded-lg border p-3.5">
        <div className="text-foreground mb-2 text-xs font-medium">Review</div>
        <dl className="grid gap-2 text-xs sm:grid-cols-2">
          <ReviewRow
            label="Host label"
            value={config.host?.trim() || 'Not set'}
          />
          <ReviewRow label="SSH endpoint" value={endpoint} mono />
          <ReviewRow
            label="Username"
            value={config.username?.trim() || 'Not set'}
          />
          <ReviewRow label="Credential" value={credentialKind} />
          <ReviewRow label="Trust mode" value={trustMode} />
          <ReviewRow label="Runtime" value="Native SSH connector required" />
        </dl>
      </div>
    </section>
  )
}
function ReviewRow({
  label,
  value,
  mono = false,
}: {
  label: string
  value: string
  mono?: boolean
}) {
  return (
    <div className="min-w-0">
      <dt className="text-foreground-alt/60">{label}</dt>
      <dd
        className={cn(
          'text-foreground mt-0.5 truncate',
          mono && 'font-mono text-xs',
        )}
      >
        {value}
      </dd>
    </div>
  )
}

function AuthModeButton({
  active,
  label,
  onClick,
  disabled = false,
}: {
  active: boolean
  label: string
  onClick: () => void
  disabled?: boolean
}) {
  return (
    <button
      type="button"
      aria-pressed={active}
      disabled={disabled}
      onClick={onClick}
      className={cn(
        'h-8 rounded-md border px-3 text-xs font-medium transition-all duration-150',
        disabled && 'cursor-not-allowed opacity-50',
        active
          ? 'border-brand/30 bg-brand/10 text-foreground'
          : 'border-foreground/10 bg-background/20 text-foreground-alt hover:border-foreground/20 hover:bg-foreground/5',
      )}
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
  error,
  supported,
  loading,
  onTicketChange,
  onApprove,
  onSignIn,
}: {
  config: AddDeviceWizardConfig
  ticket: string
  busy: boolean
  error?: string
  supported: boolean
  loading: boolean
  onTicketChange: (value: string) => void
  onApprove: () => void
  onSignIn: () => void
}) {
  if (loading) {
    return (
      <LoadingCard
        view={{
          state: 'active',
          title: 'Checking device linking',
          detail: 'Checking whether this session can approve device links.',
        }}
      />
    )
  }

  if (!supported) {
    return (
      <div className="border-foreground/6 bg-background-card/30 rounded-lg border p-3.5">
        <div className="text-foreground text-xs font-medium">
          Spacewave Cloud required
        </div>
        <p className="text-foreground-alt/60 mt-1 text-xs">
          {DEVICE_APPROVAL_CLOUD_REQUIRED}
        </p>
        <Button
          size="sm"
          onClick={onSignIn}
          className="border-brand/30 bg-brand/10 hover:border-brand/50 hover:bg-brand/15 text-foreground mt-3 h-7 rounded-md border px-3 text-xs transition-all duration-150"
        >
          <LuLogIn className="size-3.5" />
          Sign in or create account
        </Button>
      </div>
    )
  }

  return (
    <div className="border-foreground/6 bg-background-card/30 rounded-lg border p-3.5">
      <label className="text-foreground text-xs font-medium select-none">
        SpaceLink Ticket
      </label>
      <p className="text-foreground-alt/70 mt-1 text-xs leading-relaxed">
        Paste the ticket created by the Device setup command to prepare
        enrollment.
      </p>
      <textarea
        value={ticket}
        onChange={(e) => onTicketChange(e.target.value)}
        placeholder="Paste the base64 ticket from spacewave device setup"
        disabled={busy}
        className="border-foreground/10 bg-background/20 text-foreground placeholder:text-foreground-alt/40 focus-visible:border-brand/50 focus-visible:ring-brand/15 mt-2 min-h-24 w-full rounded-md border p-2 font-mono text-xs outline-none disabled:opacity-60"
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
      {busy && (
        <p className="text-foreground-alt/80 mt-2 text-xs leading-relaxed">
          Checking ticket and preparing enrollment…
        </p>
      )}
      {error && (
        <div
          className="border-destructive/15 bg-destructive/5 text-destructive mt-2 rounded-md border p-2 text-xs leading-relaxed"
          role="alert"
        >
          <div className="font-medium">Ticket could not be approved</div>
          <div className="mt-0.5">{error}</div>
        </div>
      )}
      <Button
        size="sm"
        onClick={onApprove}
        disabled={busy || !ticket.trim()}
        className="border-brand/30 bg-brand/10 hover:border-brand/50 hover:bg-brand/15 text-foreground mt-3 h-7 rounded-md border px-3 text-xs transition-all duration-150"
      >
        <LuClipboardCheck className="size-3.5" />
        {busy ? 'Checking ticket…' : 'Approve'}
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
  setError,
  persist,
  nextStep = 2,
}: {
  ws: UseWizardStateResult
  sharedObjectId: string
  ticket: string
  config: AddDeviceWizardConfig
  session: Session | undefined
  setBusy: (busy: boolean) => void
  setError?: (message: string) => void
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
    const message = getApprovalErrorMessage(err)
    setError?.(message)
    toast.error(
      message === DEVICE_APPROVAL_CLOUD_REQUIRED
        ? message
        : `Ticket could not be approved. ${message}`,
    )
  } finally {
    setBusy(false)
  }
}

export function getApprovalErrorMessage(error: unknown): string {
  if (isUnimplementedRpcError(error)) {
    return DEVICE_APPROVAL_CLOUD_REQUIRED
  }
  return 'Check the ticket and try again.'
}

function isUnimplementedRpcError(error: unknown): boolean {
  const message =
    error instanceof Error
      ? error.message
      : typeof error === 'string'
        ? error
        : ''
  if (/\bunimplemented\b|\bmethod not implemented\b/i.test(message)) {
    return true
  }
  if (typeof error !== 'object' || error === null || !('rpcError' in error)) {
    return false
  }
  return (
    typeof error.rpcError === 'string' &&
    /\bunimplemented\b|\bmethod not implemented\b/i.test(error.rpcError)
  )
}

async function replaceSpaceIndexIfWizardIsCurrent(
  ws: UseWizardStateResult,
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
  const publicKey = normalizeSshHostPublicKey(config.hostKeyPublicKey)
  const fingerprint = config.hostKeyFingerprint?.trim()
  if (!publicKey && !fingerprint) return []
  const publicKeyAlgorithm = getSshPublicKeyAlgorithm(publicKey)
  return [
    {
      algorithm:
        publicKeyAlgorithm || (config.hostKeyAlgorithm ?? 'ssh-ed25519').trim(),
      publicKey,
      sha256Fingerprint: fingerprint,
      acceptedByPeerId,
      acceptedAt,
    },
  ]
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
  const publicKey = normalizeSshHostPublicKey(config.hostKeyPublicKey)
  const hasManualTrust = !!publicKey || !!config.hostKeyFingerprint?.trim()
  const publicKeyAlgorithm = getSshPublicKeyAlgorithm(publicKey)
  if (
    hasManualTrust &&
    !publicKeyAlgorithm &&
    !(config.hostKeyAlgorithm ?? 'ssh-ed25519').trim()
  ) {
    return 'SSH host key algorithm is required'
  }
  return ''
}

function normalizeSshHostPublicKey(value: string | undefined): string {
  const raw = value?.trim()
  if (!raw) return ''
  const fields = raw.split(/\s+/)
  const keyIndex = fields.findIndex(isSshPublicKeyAlgorithm)
  if (keyIndex >= 0 && fields[keyIndex + 1]) {
    return `${fields[keyIndex]} ${fields[keyIndex + 1]}`
  }
  return raw
}

function getSshPublicKeyAlgorithm(publicKey: string): string {
  const fields = publicKey.trim().split(/\s+/)
  return isSshPublicKeyAlgorithm(fields[0] ?? '') ? fields[0] : ''
}

function isSshPublicKeyAlgorithm(value: string): boolean {
  return (
    value.startsWith('ssh-') ||
    value.startsWith('ecdsa-') ||
    value.startsWith('sk-ssh-') ||
    value.startsWith('sk-ecdsa-')
  )
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
