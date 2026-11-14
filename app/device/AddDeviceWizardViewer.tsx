import {
  useCallback,
  useId,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react'
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
import { resourceContent } from '@aptre/bldr-sdk/hooks/index.js'
import {
  SpaceLinkCallback,
  SpaceLinkCompletionMode,
} from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { ViewerStatus } from '@s4wave/web/object/ViewerStatus.js'
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
import { DeviceTypeID } from '@s4wave/sdk/device/device.js'
import { Device } from '@s4wave/sdk/device/device.pb.js'
import { base58Encode } from '@s4wave/app/provider/spacewave/keypair-utils.js'
import {
  SecretKindSSHPassword,
  SecretKindSSHPrivateKey,
  SecretKindSSHPassphrase,
  SSHPrivateKeyContentType,
  SSHTextCredentialContentType,
} from '@s4wave/sdk/secret/secret.js'
import type { Session } from '@s4wave/sdk/session/session.js'
import type { Space } from '@s4wave/sdk/space/space.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import { CREATE_SSH_HOST_OP_ID } from '@s4wave/sdk/sshhost/create-ssh-host.js'
import {
  CreateSshHostOp,
  type SshHost,
  type SshHostCredentialRefs,
  type SshHostKeyPin,
} from '@s4wave/sdk/sshhost/sshhost.pb.js'
import { CREATE_TERMINAL_OP_ID } from '@s4wave/sdk/terminal/create-terminal.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'
import { useWorldQuery } from '@s4wave/web/hooks/useWorldQuery.js'
import { useNavigate } from '@s4wave/web/router/router.js'

import { buildObjectKey } from '../space/create-op-builders.js'
import { useWizardState } from '../wizard/useWizardState.js'
import {
  useWizardFinish,
  type WizardFinishState,
} from '../wizard/wizard-finish.js'
import type { UseWizardStateResult } from '../wizard/useWizardState.js'
import { WizardShell } from '../wizard/WizardShell.js'
import { WizardField } from '../wizard/WizardField.js'
import { WizardTextareaField } from '../wizard/WizardTextareaField.js'
import { ComputersDashboardTypeID } from './computers.js'
import { buildCreateSshHostTerminalOpData } from './terminal-action.js'
import { WizardLoading } from '@s4wave/app/wizard/WizardLoading.js'

const DEFAULT_SSH_PORT = 22
const DEVICE_APPROVAL_CLOUD_REQUIRED =
  'Device linking requires a Spacewave Cloud session. Sign in or create an account to continue.'
const INSTALL_AGENT_UNAVAILABLE =
  'Install Agent is unavailable until secure bootstrap is configured.'

const noKeys: string[] = []

type AddDeviceWizardMode = 'spacelink' | 'ssh'
type SshAuthMode = 'password' | 'private-key'
type SshSetupMode = 'host' | 'install-agent'

interface SshHostWizardConfig {
  host?: string
  port?: number
  username?: string
  authMode?: SshAuthMode
  setupMode?: SshSetupMode
  hostKeyAlgorithm?: string
  hostKeyFingerprint?: string
  hostKeyPublicKey?: string
}

// SshAddAttempt records the steps of an SSH Host Finish that have written,
// so Retry skips them: the host with its secrets, then the terminal, whose
// key is empty when the host has none. input is the person's input the
// steps were made from; an attempt made from other input is not reused.
interface SshAddAttempt {
  input: string
  host?: { host: SshHost; hostObjectKey: string }
  terminalObjectKey?: string
}

// sshAttemptInput is the input an SSH Host attempt is made from: the label,
// the host settings and the credentials. It stays in memory only.
function sshAttemptInput(
  label: string,
  config: SshHostWizardConfig,
  draft: SshCredentialDraft,
): string {
  return JSON.stringify([label, config, draft])
}

// useSshAddAttempt holds the SSH Host attempt for the person's current input.
// begin returns the attempt made from that input, or starts a new one; an
// edit after a failure starts a new one and leaves what the old one added,
// since those objects may already be named elsewhere, such as the host by its
// terminal. end clears a finished attempt, and finishState adds what a failed
// attempt left in the Space to the failed notice.
function useSshAddAttempt(state: WizardFinishState) {
  // The attempt changes inside a press; what it left shows on a failure.
  const ref = useRef<SshAddAttempt>({ input: '' })
  const [leftBehind, setLeftBehind] = useState('')

  // A press reuses the attempt only for the same input.
  const begin = useCallback((input: string) => {
    setLeftBehind('')
    if (ref.current.input !== input) ref.current = { input }
    return ref.current
  }, [])
  const end = useCallback(
    (attempt: SshAddAttempt, result: WizardFinishState | undefined) => {
      if (result?.status === 'done') ref.current = { input: '' }
      setLeftBehind(
        result?.status === 'failed' ? describeSshAttempt(attempt) : '',
      )
    },
    [],
  )

  // Only a failure names what was left behind.
  const finishState = useMemo<WizardFinishState>(
    () =>
      state.status === 'failed' && leftBehind
        ? { ...state, error: `${state.error}. ${leftBehind}` }
        : state,
    [leftBehind, state],
  )
  return useMemo(() => ({ begin, end, finishState }), [begin, end, finishState])
}

// describeSshAttempt names the objects an attempt added, for the failed
// notice, or returns '' when it added none.
function describeSshAttempt(attempt: SshAddAttempt): string {
  // An attempt that failed before the host added nothing.
  if (!attempt.host) return ''

  // List the host, its secrets, then the terminal.
  const { host, hostObjectKey } = attempt.host
  const keys = [
    `SSH Host ${hostObjectKey}`,
    ...Object.values(host.credentials ?? {})
      .filter((key): key is string => typeof key === 'string' && !!key)
      .map((key) => `secret ${key}`),
  ]
  if (attempt.terminalObjectKey) {
    keys.push(`terminal ${attempt.terminalObjectKey}`)
  }
  return `Added so far: ${keys.join(', ')}. Retry reuses them unless you change the form.`
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

// useAddDeviceWizardController owns wizard persistence, approval, and SSH
// object creation.
function useAddDeviceWizardController(props: ObjectViewerComponentProps) {
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
  const { spaceWorldResource } = SpaceContainerContext.useContext()
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
  const isSshInstallMode = mode === 'ssh' && sshSetupMode === 'install-agent'
  const ticket = config.ticket ?? ''
  const completion = config.completion ?? ''
  // pairedPeerId is the printable peer ID of the device the approved ticket
  // came from, the peer ID its Device object records.
  const pairedPeerId = useMemo(() => {
    const raw = config.preview?.agentPeerId
    return raw ? base58Encode(base64ToBytes(raw)) : ''
  }, [config.preview?.agentPeerId])
  const setupCommand = buildSetupCommand(
    ws.localName || 'Device',
    sharedObjectId,
  )
  const installSetupCommand = buildInstallSetupCommand(
    ws.localName || 'Device',
    sharedObjectId,
  )
  const completeCommand = completion
    ? `spacewave device complete --completion ${quoteShellArg(completion)}`
    : 'spacewave device complete --completion <completion>'
  const computersResource = useWorldQuery(
    spaceWorldResource,
    async (world, signal) => {
      const [devices, dashboards] = await Promise.all([
        world.listObjectsWithType(DeviceTypeID, signal),
        world.listObjectsWithType(ComputersDashboardTypeID, signal),
      ])
      return { devices, dashboards }
    },
    [],
  )
  const computers = resourceContent(computersResource)
  const deviceKeys = computers?.devices ?? noKeys
  const dashboardKey = computers?.dashboards[0] ?? ''

  // Finish moves the Space index off the wizard before deleting it, so it
  // waits until the World has said which Computers dashboard takes its place.
  const computersKnown = computers !== null

  // Save a config edit locally and on the wizard object.
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
      if (patch.setupMode && patch.setupMode !== 'host') {
        setOperationError(INSTALL_AGENT_UNAVAILABLE)
        toast.error(INSTALL_AGENT_UNAVAILABLE)
        return
      }
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

  // Finish for a linked device creates nothing: it moves a Space index that
  // names the wizard to the Computers dashboard and deletes the wizard in one
  // list, then opens the device.
  const finish = useWizardFinish(ws)
  const handleFinishSpaceLink = useCallback(async () => {
    if (!state || !computersKnown) return
    setOperationError('')
    await finish.finish(
      async () => {
        await ws.persistDraftState()
        // The device was paired before Finish, so it opens even when the
        // wizard is already gone. Until the paired device has enrolled, the
        // Computers dashboard opens in its place.
        const deviceKey = await findPairedDevice(
          ws.spaceWorld,
          deviceKeys,
          pairedPeerId,
        )
        return {
          creates: [],
          target: {
            openKey: deviceKey || dashboardKey,
            indexPath: dashboardKey,
          },
          targetMade: true,
        }
      },
      (target) => {
        if (target.openKey) ws.navigateToObjects([target.openKey])
      },
    )
  }, [
    computersKnown,
    dashboardKey,
    deviceKeys,
    finish,
    pairedPeerId,
    state,
    ws,
  ])

  // createSshHost writes the credential secrets and the SSH Host object.
  const createSshHost = useCallback(async () => {
    if (!state) throw new Error('Wizard state is not ready')
    if (!space) throw new Error('Space resource is not ready')
    const readerPublicKeyPem =
      sessionInfo?.cryptoInfo?.publicKeyPem?.trim() ?? ''
    if (!readerPublicKeyPem) {
      throw new Error('Session public key is not available')
    }

    const label = (ws.localName || state.name || 'SSH Host').trim()
    const timestamp = new Date()
    const [hostObjectKey, credentials] = await Promise.all([
      buildObjectKey(ws.spaceWorld, 'ssh-host/', label),
      createSshCredentialSecrets({
        space,
        label,
        config: sshConfig,
        draft: sshCredentialDraft,
        world: ws.spaceWorld,
        readerPublicKeyPem,
      }),
    ])
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
    return { host, hostObjectKey }
  }, [
    sessionInfo?.cryptoInfo?.publicKeyPem,
    space,
    sshConfig,
    sshCredentialDraft,
    state,
    ws,
  ])

  // createSshHostTerminal creates the terminal that opens the host, when the
  // host has one, and returns its key or an empty key.
  const createSshHostTerminal = useCallback(
    async (host: SshHost, hostObjectKey: string) => {
      const terminal = await buildCreateSshHostTerminalOpData({
        host,
        hostObjectKey,
        world: ws.spaceWorld,
      })
      if (!terminal) return ''
      await ws.spaceWorld.applyWorldOp(
        CREATE_TERMINAL_OP_ID,
        terminal.opData,
        ws.sessionPeerId,
      )
      return terminal.objectKey
    },
    [ws],
  )

  // Adding an SSH Host writes its credential secrets and host, then reads the
  // World to build the terminal, so it cannot be one list. The attempt keeps
  // each finished step in memory so Retry carries on from the step that
  // failed instead of adding a second host; the wizard's own removal is then
  // one list. A reload drops the attempt, so a host made before a reload
  // stays and a later Finish adds another.
  const sshAttempt = useSshAddAttempt(finish.state)
  const handleCreateSshHost = useCallback(async () => {
    if (!state || !computersKnown) return
    const error = getSshCreateError(sshConfig, sshCredentialDraft)
    if (error) {
      setOperationError(error)
      toast.error(error)
      return
    }

    setOperationError('')
    const label = (ws.localName || state.name || 'SSH Host').trim()
    const attempt = sshAttempt.begin(
      sshAttemptInput(label, sshConfig, sshCredentialDraft),
    )
    const result = await finish.finish(
      async () => {
        attempt.host ??= await createSshHost()
        const { host, hostObjectKey } = attempt.host
        attempt.terminalObjectKey ??= await createSshHostTerminal(
          host,
          hostObjectKey,
        )
        // The host and terminal exist before the list, so they open even
        // when the wizard is already gone.
        return {
          creates: [],
          target: {
            openKey: attempt.terminalObjectKey || hostObjectKey,
            indexPath: dashboardKey,
          },
          targetMade: true,
        }
      },
      (target) => {
        toast.success('SSH Host added')
        ws.navigateToObjects([target.openKey])
      },
    )
    sshAttempt.end(attempt, result)
  }, [
    computersKnown,
    createSshHost,
    createSshHostTerminal,
    dashboardKey,
    finish,
    sshAttempt,
    sshConfig,
    sshCredentialDraft,
    state,
    ws,
  ])

  const handleFinalize = useCallback(() => {
    if (isSshInstallMode) {
      setOperationError(INSTALL_AGENT_UNAVAILABLE)
      toast.error(INSTALL_AGENT_UNAVAILABLE)
      return
    }
    if (mode === 'ssh') {
      void handleCreateSshHost()
      return
    }
    void handleFinishSpaceLink()
  }, [handleCreateSshHost, handleFinishSpaceLink, isSshInstallMode, mode])

  // Cancel stays available while the computers read is unknown: it then clears
  // a Space index that names the wizard instead of waiting for the dashboard,
  // since an empty index opens the object list and never names a deleted object.
  const handleCancel = useCallback(() => {
    void ws.handleCancel(dashboardKey)
  }, [dashboardKey, ws])

  return {
    approvalError,
    busy,
    completeCommand,
    completion,
    computersKnown,
    computersResource,
    config,
    currentStep,
    deviceKeys,
    finish: sshAttempt.finishState,
    handleCancel,
    handleConfigUpdate,
    handleFinalize,
    handleModeChange,
    handleNameNext,
    handleSignIn,
    handleSshConfigChange,
    handleTicketChange,
    isOpeningStep,
    isSshInstallMode,
    installSetupCommand,
    mode,
    operationError,
    session,
    sessionInfoLoading,
    setApprovalError,
    setBusy,
    setOpeningStep,
    setOperationError,
    setSshCredentialDraft,
    setupCommand,
    sharedObjectId,
    sshAuthMode,
    sshConfig,
    sshCredentialDraft,
    sshSetupMode,
    state,
    supportsDeviceApproval,
    ticket,
    ws,
  }
}

type AddDeviceWizardController = ReturnType<typeof useAddDeviceWizardController>

// AddDeviceWizardSteps renders the connection-specific step bodies from the
// controller projection.
function AddDeviceWizardSteps({
  controller,
}: {
  controller: AddDeviceWizardController
}) {
  const {
    approvalError,
    busy,
    completeCommand,
    config,
    currentStep,
    deviceKeys,
    handleConfigUpdate,
    handleModeChange,
    handleSignIn,
    handleSshConfigChange,
    handleTicketChange,
    isOpeningStep,
    installSetupCommand,
    mode,
    operationError,
    session,
    sessionInfoLoading,
    setApprovalError,
    setBusy,
    setSshCredentialDraft,
    setupCommand,
    sharedObjectId,
    sshAuthMode,
    sshConfig,
    sshCredentialDraft,
    sshSetupMode,
    supportsDeviceApproval,
    ticket,
    ws,
  } = controller

  return (
    <>
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
                setupCommand={setupCommand}
                installSetupCommand={installSetupCommand}
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
          {deviceKeys.length > 0 && (
            <div className="border-foreground/6 bg-background-card/30 rounded-lg border p-3.5">
              <div className="text-foreground text-xs font-medium">
                Device Objects
              </div>
              <div className="mt-2 space-y-1">
                {deviceKeys.map((deviceKey) => (
                  <button
                    key={deviceKey}
                    type="button"
                    onClick={() => ws.navigateToObjects([deviceKey])}
                    className="hover:bg-foreground/5 text-foreground flex w-full min-w-0 rounded px-2 py-1 text-left text-xs"
                  >
                    <span className="truncate">{deviceKey}</span>
                  </button>
                ))}
              </div>
            </div>
          )}
        </section>
      )}
      <ComputersReadStatus controller={controller} />
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
    </>
  )
}

interface WizardCopy {
  totalSteps: number
  stepName: string
  nameLabel: string
  createLabel: string
  creatingLabel: string
  nextBusyLabel: string
}

/** wizardCopy returns the step count and labels for the connection mode and step. */
function wizardCopy(
  mode: AddDeviceWizardMode,
  currentStep: number,
  isSshInstallMode: boolean,
): WizardCopy {
  if (mode === 'ssh') {
    return {
      totalSteps: currentStep === 0 ? 3 : 2,
      stepName: currentStep === 0 ? 'Choose connection' : 'Configure SSH',
      nameLabel: 'Host Name',
      createLabel: isSshInstallMode
        ? 'Install Agent unavailable'
        : 'Add SSH Host and open terminal',
      creatingLabel: isSshInstallMode
        ? 'Install Agent unavailable'
        : 'Adding SSH Host and opening terminal…',
      nextBusyLabel: 'Opening SSH setup…',
    }
  }
  const stepNames = ['Choose connection', 'Set up Device', 'Finish']
  return {
    totalSteps: 3,
    stepName: stepNames[Math.min(currentStep, 2)] ?? 'Finish',
    nameLabel: 'Device Name',
    createLabel: 'Open Device',
    creatingLabel: 'Opening Device…',
    nextBusyLabel: 'Preparing Device setup…',
  }
}

// ComputersReadStatus tells the finishing step what Finish waits on: the read
// of the Space's Devices and Computers dashboard, or its failure with Retry.
// It shows only on the finishing step, and nothing once the read is known.
function ComputersReadStatus({
  controller,
}: {
  controller: AddDeviceWizardController
}) {
  const { computersResource: resource, currentStep, mode } = controller
  if (currentStep !== (mode === 'ssh' ? 1 : 2)) return null
  if (resource.error) {
    return (
      <ViewerStatus
        state="failed"
        message={`Could not read the Space's computers: ${resource.error.message}`}
        retry={resource.retry}
      />
    )
  }
  if (resource.value === null) {
    return (
      <ViewerStatus state="waiting" what="the Space's computers" rows={0} />
    )
  }
  return null
}

// AddDeviceWizardContent selects the loading state and configures the shared
// wizard shell.
function AddDeviceWizardContent({
  controller,
}: {
  controller: AddDeviceWizardController
}) {
  const {
    completion,
    computersKnown,
    currentStep,
    finish,
    handleCancel,
    handleFinalize,
    handleNameNext,
    isOpeningStep,
    isSshInstallMode,
    mode,
    setOpeningStep,
    setOperationError,
    sshConfig,
    sshCredentialDraft,
    state,
    ws,
  } = controller

  if (!state) {
    return (
      <WizardLoading
        wizardGone={ws.wizardGone}
        title="Loading Add Device"
        detail="Preparing the Device setup wizard."
      />
    )
  }

  const copy = wizardCopy(mode, currentStep, isSshInstallMode)
  return (
    <WizardShell
      icon={<LuHardDrive />}
      title="Add Device"
      step={currentStep}
      totalSteps={copy.totalSteps}
      stepName={copy.stepName}
      localName={ws.localName || 'Device'}
      onUpdateName={ws.handleUpdateName}
      onBack={() => {
        setOpeningStep(false)
        setOperationError('')
        void ws.handleBack()
      }}
      onCancel={handleCancel}
      cancel={ws.cancel}
      nameLabel={copy.nameLabel}
      namePlaceholder="Build server"
      nameStep={0}
      finish={finish}
      createLabel={copy.createLabel}
      creatingLabel={copy.creatingLabel}
      onFinalize={handleFinalize}
      canFinalize={
        computersKnown &&
        (mode === 'ssh'
          ? !isSshInstallMode &&
            !getSshCreateError(sshConfig, sshCredentialDraft)
          : !!completion)
      }
      onNext={currentStep === 0 ? () => void handleNameNext() : undefined}
      nextBusyLabel={copy.nextBusyLabel}
      nextBusy={isOpeningStep}
      canNext={!!ws.localName.trim() && !isOpeningStep}
      finalizeStep={mode === 'ssh' ? 1 : 2}
      width={mode === 'ssh' ? 'wide' : 'default'}
    >
      <AddDeviceWizardSteps controller={controller} />
    </WizardShell>
  )
}

export function AddDeviceWizardViewer(props: ObjectViewerComponentProps) {
  const controller = useAddDeviceWizardController(props)
  return <AddDeviceWizardContent controller={controller} />
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

const sshCardClassName =
  'border-foreground/6 bg-background-card/30 rounded-lg border p-3.5'

/** hostKeyTrustMode describes how the host key of the SSH host is verified. */
function hostKeyTrustMode(config: SshHostWizardConfig): string {
  const fingerprint = config.hostKeyFingerprint?.trim()
  return fingerprint ? `Pinned to ${fingerprint}` : 'Ask on first connection'
}

/** fieldError renders an inline validation message, or nothing when valid. */
function fieldError(message: string): ReactNode {
  if (!message) return undefined
  return <span className="text-destructive text-xs">{message}</span>
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
  return (
    <section className="space-y-3">
      <SshEndpointCard config={config} onConfigChange={onConfigChange} />
      <SshCredentialCard
        authMode={authMode}
        credentialDraft={credentialDraft}
        onConfigChange={onConfigChange}
        onCredentialChange={onCredentialChange}
      />
      <SshHostKeyTrustCard config={config} onConfigChange={onConfigChange} />
      <SshSetupModeCard setupMode={setupMode} onConfigChange={onConfigChange} />
      <SshReviewCard config={config} authMode={authMode} />
    </section>
  )
}

/** SshEndpointCard edits the host, port, and user of the SSH endpoint. */
function SshEndpointCard({
  config,
  onConfigChange,
}: {
  config: SshHostWizardConfig
  onConfigChange: (patch: Partial<SshHostWizardConfig>) => void
}) {
  const port = normalizeSshPort(config.port)
  const hostError = !config.host?.trim() ? 'Host is required.' : ''
  const userError = !config.username?.trim() ? 'User is required.' : ''
  const portError =
    port < 1 || port > 65535 ? 'Port must be between 1 and 65535.' : ''

  return (
    <div className={sshCardClassName}>
      <div className="text-foreground mb-2 flex items-center gap-1.5 text-xs font-medium">
        <LuServer className="size-3.5" />
        SSH Endpoint
      </div>
      <div className="sm:grid-cols-device grid gap-2">
        <WizardField
          label="Host"
          value={config.host ?? ''}
          onChange={(e) => onConfigChange({ host: e.target.value })}
          placeholder="host.example.com"
          help={fieldError(hostError)}
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
          help={fieldError(portError)}
        />
        <WizardField
          label="User"
          value={config.username ?? ''}
          onChange={(e) => onConfigChange({ username: e.target.value })}
          placeholder="user"
          help={fieldError(userError)}
        />
      </div>
    </div>
  )
}

/** SshCredentialCard picks the auth mode and edits its password or private key. */
function SshCredentialCard({
  authMode,
  credentialDraft,
  onConfigChange,
  onCredentialChange,
}: {
  authMode: SshAuthMode
  credentialDraft: SshCredentialDraft
  onConfigChange: (patch: Partial<SshHostWizardConfig>) => void
  onCredentialChange: (patch: Partial<SshCredentialDraft>) => void
}) {
  const credentialError =
    authMode === 'private-key' && !credentialDraft.privateKey.trim()
      ? 'Private key is required.'
      : ''

  return (
    <div className={sshCardClassName}>
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
            onChange={(e) => onCredentialChange({ privateKey: e.target.value })}
            placeholder="-----BEGIN OPENSSH PRIVATE KEY-----"
            className="min-h-28"
            help={fieldError(credentialError)}
          />
          <WizardField
            label="Passphrase"
            type="password"
            value={credentialDraft.passphrase}
            onChange={(e) => onCredentialChange({ passphrase: e.target.value })}
            placeholder="Private key passphrase (optional)"
            help="Optional for unencrypted private keys."
          />
        </div>
      )}
    </div>
  )
}

/** SshHostKeyTrustCard edits the optional pinned host key. */
function SshHostKeyTrustCard({
  config,
  onConfigChange,
}: {
  config: SshHostWizardConfig
  onConfigChange: (patch: Partial<SshHostWizardConfig>) => void
}) {
  return (
    <details className={sshCardClassName}>
      <summary className="text-foreground flex cursor-pointer items-center justify-between text-xs font-medium select-none [@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:py-3.5">
        <span>Host-key trust (advanced)</span>
        <span className="text-foreground-alt/60 text-xs">optional</span>
      </summary>
      <p className="text-foreground-alt/70 mt-2 text-xs leading-relaxed">
        Trust mode:{' '}
        <span className="font-medium">{hostKeyTrustMode(config)}</span>. Leave
        the fields empty to ask on the first connection.
      </p>
      <div className="sm:grid-cols-form mt-3 grid gap-2">
        <WizardField
          label="Algorithm"
          value={config.hostKeyAlgorithm ?? 'ssh-ed25519'}
          onChange={(e) => onConfigChange({ hostKeyAlgorithm: e.target.value })}
          placeholder="ssh-ed25519"
          variant="compactMono"
        />
        <WizardField
          label="Fingerprint"
          value={config.hostKeyFingerprint ?? ''}
          onChange={(e) =>
            onConfigChange({ hostKeyFingerprint: e.target.value })
          }
          placeholder="SHA256:..."
          variant="compactMono"
        />
      </div>
      <WizardTextareaField
        label="Public key"
        value={config.hostKeyPublicKey ?? ''}
        onChange={(e) => onConfigChange({ hostKeyPublicKey: e.target.value })}
        placeholder="[host]:22 ssh-ed25519 AAAA… or ssh-ed25519 AAAA…"
        className="min-h-16 font-mono text-xs"
        fieldClassName="mt-2"
      />
    </details>
  )
}

/** SshSetupModeCard shows the setup actions; installing the agent is unavailable. */
function SshSetupModeCard({
  setupMode,
  onConfigChange,
}: {
  setupMode: SshSetupMode
  onConfigChange: (patch: Partial<SshHostWizardConfig>) => void
}) {
  return (
    <div className={sshCardClassName}>
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
    </div>
  )
}

/** SshReviewCard summarizes the SSH host before it is created. */
function SshReviewCard({
  config,
  authMode,
}: {
  config: SshHostWizardConfig
  authMode: SshAuthMode
}) {
  const host = config.host?.trim()
  const endpoint = host
    ? `${host}:${normalizeSshPort(config.port)}`
    : 'Host not set'

  return (
    <div className={sshCardClassName}>
      <div className="text-foreground mb-2 text-xs font-medium">Review</div>
      <dl className="grid gap-2 text-xs sm:grid-cols-2">
        <ReviewRow label="Host label" value={host || 'Not set'} />
        <ReviewRow label="SSH endpoint" value={endpoint} mono />
        <ReviewRow
          label="Username"
          value={config.username?.trim() || 'Not set'}
        />
        <ReviewRow
          label="Credential"
          value={authMode === 'private-key' ? 'Private key' : 'Password'}
        />
        <ReviewRow label="Trust mode" value={hostKeyTrustMode(config)} />
      </dl>
    </div>
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
      <pre className="border-foreground/8 bg-background/40 text-foreground-alt micro-seven overflow-x-auto rounded-md border p-2 font-mono leading-relaxed">
        {command}
      </pre>
    </div>
  )
}

function SpaceLinkApprovalPanel({
  config,
  ticket,
  setupCommand,
  installSetupCommand,
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
  setupCommand: string
  installSetupCommand: string
  busy: boolean
  error?: string
  supported: boolean
  loading: boolean
  onTicketChange: (value: string) => void
  onApprove: () => void
  onSignIn: () => void
}) {
  const ticketId = useId()
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
          variant="brandOutline"
          size="toolbarWide"
          onClick={onSignIn}
          className="mt-3"
        >
          <LuLogIn className="size-3.5" />
          Sign in or create account
        </Button>
      </div>
    )
  }

  const ticketReady = ticket.trim().length > 0

  return (
    <div className="space-y-3">
      {!ticketReady && (
        <div>
          <h3 className="text-foreground text-xs font-medium">
            1. Run setup on the Device
          </h3>
          <p className="text-foreground-alt/70 mt-1 text-xs leading-relaxed">
            Choose the command that matches the Device. Both print a ticket.
          </p>
          <div className="mt-2 space-y-2">
            <CommandPanel
              title="Spacewave already installed"
              command={setupCommand}
              icon={<LuTerminal className="size-3.5" />}
            />
            <CommandPanel
              title="Install and run"
              command={installSetupCommand}
              icon={<LuTerminal className="size-3.5" />}
            />
          </div>
        </div>
      )}
      <div className="border-foreground/6 bg-background-card/30 rounded-lg border p-3.5">
        <label
          htmlFor={ticketId}
          className="text-foreground text-xs font-medium select-none"
        >
          {ticketReady ? 'Approve Device ticket' : '2. Paste the Device ticket'}
        </label>
        <p className="text-foreground-alt/70 mt-1 text-xs leading-relaxed">
          {ticketReady
            ? 'The ticket is ready. Approve it to create the completion command.'
            : 'Paste the base64 ticket printed by the Device setup command.'}
        </p>
        <textarea
          id={ticketId}
          aria-label="Device ticket"
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
        {ticketReady && (
          <Button
            onClick={onApprove}
            disabled={busy}
            variant="brandOutline"
            size="toolbarWide"
            className="mt-3"
          >
            <LuClipboardCheck className="size-3.5" />
            {busy ? 'Checking ticket…' : 'Approve'}
          </Button>
        )}
      </div>
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

async function createSshCredentialSecrets({
  space,
  label,
  config,
  draft,
  world,
  readerPublicKeyPem,
}: {
  space: Space
  label: string
  config: SshHostWizardConfig
  draft: SshCredentialDraft
  world: IWorldState
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
      world,
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
        world,
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
    world,
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
  world,
  readerPublicKeyPem,
}: {
  space: Space
  label: string
  suffix: string
  kind: string
  contentType: string
  value: string
  world: IWorldState
  readerPublicKeyPem: string
}): Promise<string> {
  const objectKey = await buildObjectKey(
    world,
    'secret/',
    `${label} SSH ${suffix}`,
  )
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
  return ['spacewave', buildDeviceSetupArgs(label, sharedObjectId)].join(' ')
}

function buildInstallSetupCommand(
  label: string,
  sharedObjectId: string,
): string {
  return [
    `tmp=$(mktemp) || exit; trap 'rm -f "$tmp"' 0 HUP INT TERM; curl -fsSL --output "$tmp" https://raw.githubusercontent.com/s4wave/spacewave/master/scripts/spacewave.sh || exit; sh "$tmp"`,
    buildDeviceSetupArgs(label, sharedObjectId),
  ].join(' ')
}

function buildDeviceSetupArgs(label: string, sharedObjectId: string): string {
  const parts = ['device', 'setup', '--label', quoteShellArg(label)]
  if (sharedObjectId) {
    parts.push('--target-hint', quoteShellArg(sharedObjectId))
  }
  return parts.join(' ')
}

function quoteShellArg(value: string): string {
  if (/^[a-zA-Z0-9_./:@%+=,-]+$/.test(value)) return value
  return `'${value.replaceAll("'", "'\"'\"'")}'`
}

// findPairedDevice returns the key of the Device among keys whose peer ID is
// peerId, or '' when none has enrolled with it.
async function findPairedDevice(
  world: IWorldState,
  keys: string[],
  peerId: string,
): Promise<string> {
  if (!peerId || keys.length === 0) return ''
  const bodies = await world.getObjectBodies(keys)
  const paired = bodies.find(
    (body) =>
      body.exists &&
      Device.fromBinary(body.body ?? new Uint8Array()).peerId === peerId,
  )
  return paired?.objectKey ?? ''
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
