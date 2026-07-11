import React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'

import { SetSpaceSettingsOp } from '@s4wave/core/space/world/ops/ops.pb.js'
import { SET_SPACE_SETTINGS_OP_ID } from '@s4wave/core/space/world/ops/set-space-settings.js'
import {
  SpaceLinkCallback,
  SpaceLinkCallbackStatus,
  SpaceLinkCompletionMode,
} from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import {
  SecretKindSSHPassword,
  SecretKindSSHPrivateKey,
  SSHPrivateKeyContentType,
  SSHTextCredentialContentType,
} from '@s4wave/sdk/secret/secret.js'
import { CREATE_SSH_HOST_OP_ID } from '@s4wave/sdk/sshhost/create-ssh-host.js'
import { CreateSshHostOp } from '@s4wave/sdk/sshhost/sshhost.pb.js'
import { CREATE_TERMINAL_OP_ID } from '@s4wave/sdk/terminal/create-terminal.js'
import {
  CreateTerminalOp,
  TerminalTargetKind,
} from '@s4wave/sdk/terminal/terminal.pb.js'

import { ComputersDashboardTypeID } from './computers.js'

const h = vi.hoisted(() => ({
  applyWorldOp: vi.fn().mockResolvedValue({ seqno: 1n, sysErr: false }),
  deleteObject: vi.fn().mockResolvedValue({ deleted: true }),
  navigateToObjects: vi.fn(),
  navigate: vi.fn(),
  updateState: vi.fn().mockResolvedValue(undefined),
  persistDraftState: vi.fn().mockResolvedValue(undefined),
  handleConfigDataChange: vi.fn(),
  setCreating: vi.fn(),
  previewSpaceLink: vi.fn(),
  approveSpaceLink: vi.fn(),
  createSecret: vi.fn().mockResolvedValue({ secret: {} }),
  toastSuccess: vi.fn(),
  toastError: vi.fn(),
  currentStep: 1,
  configData: undefined as Uint8Array | undefined,
  providerId: 'spacewave',
  sessionInfoLoading: false,
  spaceSettingsIndexPath: 'wizard/device-setup',
  worldObjects: [
    { objectKey: 'computers', objectType: 'spacewave/computers' },
    { objectKey: 'devices/build-host', objectType: 'spacewave/device' },
  ],
}))

vi.mock('../wizard/useWizardState.js', () => ({
  useWizardState: () => ({
    objectKey: 'wizard/device-setup',
    state: {
      step: h.currentStep,
      targetTypeId: 'spacewave/device',
      targetKeyPrefix: 'devices/',
      name: 'Build Host',
    },
    localName: 'Build Host',
    creating: false,
    setCreating: h.setCreating,
    sessionPeerId: '12D3KooWSession',
    spaceWorld: {
      applyWorldOp: h.applyWorldOp,
      deleteObject: h.deleteObject,
    },
    spaceSettings: {
      indexPath: h.spaceSettingsIndexPath,
      pluginIds: ['spacewave-web'],
    },
    existingObjectKeys: h.worldObjects.map((obj) => obj.objectKey),
    navigateToObjects: h.navigateToObjects,
    wizardResource: { value: { updateState: h.updateState } },
    configEditor: { element: null, value: undefined },
    configData: h.configData,
    persistDraftState: h.persistDraftState,
    handleConfigDataChange: h.handleConfigDataChange,
    handleUpdateName: vi.fn(),
    handleBack: vi.fn(),
    handleCancel: vi.fn(),
  }),
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResourceValue: (resource: { value?: unknown }) => resource.value,
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  SessionContext: {
    useContext: () => ({
      value: {
        spacewave: {
          previewSpaceLink: h.previewSpaceLink,
          approveSpaceLink: h.approveSpaceLink,
        },
      },
    }),
  },
  SpaceContext: {
    useContext: () => ({
      value: {
        createSecret: h.createSecret,
      },
    }),
  },
  SharedObjectContext: {
    useContext: () => ({ value: { meta: { sharedObjectId: 'space-1' } } }),
  },
}))

vi.mock('@s4wave/web/hooks/useSessionInfo.js', () => ({
  useSessionInfo: () => ({
    sessionInfo: {
      cryptoInfo: {
        publicKeyPem:
          '-----BEGIN PUBLIC KEY-----\nmock\n-----END PUBLIC KEY-----',
      },
    },
    loading: h.sessionInfoLoading,
    providerId: h.providerId,
    supportsDeviceApproval: h.providerId === 'spacewave',
  }),
}))

vi.mock('@s4wave/web/router/router.js', async (importOriginal) => ({
  ...(await importOriginal()),
  useNavigate: () => h.navigate,
}))

vi.mock('@s4wave/web/contexts/SpaceContainerContext.js', () => ({
  SpaceContainerContext: {
    useContext: () => ({
      spaceState: { worldContents: { objects: h.worldObjects } },
    }),
  },
}))

vi.mock('@s4wave/web/ui/toaster.js', () => ({
  toast: {
    success: h.toastSuccess,
    error: h.toastError,
  },
}))

import { AddDeviceWizardViewer } from './AddDeviceWizardViewer.js'

describe('AddDeviceWizardViewer', () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
    h.currentStep = 1
    h.configData = undefined
    h.providerId = 'spacewave'
    h.sessionInfoLoading = false
    h.spaceSettingsIndexPath = 'wizard/device-setup'
    h.worldObjects = [
      { objectKey: 'computers', objectType: ComputersDashboardTypeID },
      { objectKey: 'devices/build-host', objectType: 'spacewave/device' },
    ]
  })

  it('renders CLI and container setup copy distinct from account device pairing', () => {
    renderViewer()

    expect(screen.getByText('Add Device')).toBeTruthy()
    expect(screen.queryByText('Link My Device')).toBeNull()
    expect(screen.getAllByText(/spacewave device setup/)).toHaveLength(2)
    expect(screen.getAllByText(/--target-hint space-1/)).toHaveLength(2)
  })

  it('approves a CLI SpaceLink ticket and persists the completion command', async () => {
    const ticket = bytesToBase64(new Uint8Array([1, 2, 3]))
    h.previewSpaceLink.mockResolvedValue({
      label: 'Build Host',
      agentPeerId: new Uint8Array([4, 5]),
      targetHint: new TextEncoder().encode('space-1'),
      completionMode: SpaceLinkCompletionMode.SpaceLinkCompletionMode_CLI,
    })
    h.approveSpaceLink.mockResolvedValue({
      completionMode: SpaceLinkCompletionMode.SpaceLinkCompletionMode_CLI,
      completion: {
        status: SpaceLinkCallbackStatus.SpaceLinkCallbackStatus_OK,
        nonce: new Uint8Array([9]),
      },
    })
    h.configData = new TextEncoder().encode(JSON.stringify({ ticket }))
    renderViewer()

    expect(
      screen.getByPlaceholderText(/paste the base64 ticket/i),
    ).toHaveProperty('value', ticket)
    fireEvent.click(screen.getByRole('button', { name: /approve/i }))

    await waitFor(() => expect(h.updateState).toHaveBeenCalled())
    expect(h.previewSpaceLink).toHaveBeenCalledWith({
      ticket: new Uint8Array([1, 2, 3]),
    })
    expect(h.approveSpaceLink).toHaveBeenCalledWith({
      ticket: new Uint8Array([1, 2, 3]),
      resourceId: new TextEncoder().encode('space-1'),
    })
    const update = h.updateState.mock.calls.at(-1)?.[0] as
      | { step?: number; configData?: Uint8Array }
      | undefined
    if (!update?.configData) {
      throw new Error('expected wizard state update')
    }
    expect(update.step).toBe(2)
    const config = JSON.parse(new TextDecoder().decode(update.configData)) as {
      ticket?: string
      completion?: string
    }
    expect(config.ticket).toBe(ticket)
    if (!config.completion) {
      throw new Error('expected completion')
    }
    const completion = SpaceLinkCallback.fromBinary(
      base64ToBytes(config.completion),
    )
    expect(completion.status).toBe(
      SpaceLinkCallbackStatus.SpaceLinkCallbackStatus_OK,
    )
  })

  it('replaces SpaceLink approval with the Cloud sign-in affordance for local sessions', () => {
    h.providerId = 'local'
    renderViewer()

    expect(screen.getByText('Spacewave Cloud required')).toBeTruthy()
    expect(
      screen.getByText(
        'Device linking requires a Spacewave Cloud session. Sign in or create an account to continue.',
      ),
    ).toBeTruthy()
    expect(screen.queryByPlaceholderText(/paste the base64 ticket/i)).toBeNull()
    expect(screen.queryByRole('button', { name: /^approve$/i })).toBeNull()

    fireEvent.click(
      screen.getByRole('button', { name: 'Sign in or create account' }),
    )
    expect(h.navigate).toHaveBeenCalledWith({ path: '/login' })
    expect(h.previewSpaceLink).not.toHaveBeenCalled()
    expect(h.approveSpaceLink).not.toHaveBeenCalled()
  })

  it('maps an unimplemented approval RPC to the Cloud-session explanation', async () => {
    const ticket = bytesToBase64(new Uint8Array([1, 2, 3]))
    h.configData = new TextEncoder().encode(JSON.stringify({ ticket }))
    h.previewSpaceLink.mockRejectedValue(
      new Error('Server error: unimplemented'),
    )
    renderViewer()

    fireEvent.click(screen.getByRole('button', { name: /^approve$/i }))

    await waitFor(() =>
      expect(h.toastError).toHaveBeenCalledWith(
        'Device linking requires a Spacewave Cloud session. Sign in or create an account to continue.',
      ),
    )
    expect(h.toastError).not.toHaveBeenCalledWith(
      expect.stringMatching(/unimplemented/i),
    )
  })

  it('promotes Computers after completion and opens the created Device object', async () => {
    h.currentStep = 2
    h.configData = new TextEncoder().encode(
      JSON.stringify({ completion: 'completion' }),
    )
    renderViewer()

    fireEvent.click(screen.getByRole('button', { name: /open device/i }))

    await waitFor(() => expect(h.deleteObject).toHaveBeenCalled())
    expect(h.applyWorldOp).toHaveBeenCalledWith(
      SET_SPACE_SETTINGS_OP_ID,
      expect.any(Uint8Array),
      '12D3KooWSession',
    )
    const settingsOpData: unknown = h.applyWorldOp.mock.calls[0]?.[1]
    if (!(settingsOpData instanceof Uint8Array)) {
      throw new Error('expected settings op bytes')
    }
    const settingsOp = SetSpaceSettingsOp.fromBinary(settingsOpData)
    expect(settingsOp.settings?.indexPath).toBe('computers')
    expect(settingsOp.settings?.pluginIds).toEqual(['spacewave-web'])
    expect(h.deleteObject).toHaveBeenCalledWith('wizard/device-setup')
    expect(h.navigateToObjects).toHaveBeenCalledWith(['devices/build-host'])
  })

  it('creates an SSH-only Host with Secret refs and an SSH Host terminal', async () => {
    h.currentStep = 1
    h.configData = new TextEncoder().encode(
      JSON.stringify({
        mode: 'ssh',
        ssh: {
          host: 'build.example.com',
          port: 2222,
          username: 'ubuntu',
          authMode: 'password',
          hostKeyAlgorithm: 'ssh-ed25519',
          hostKeyFingerprint: 'SHA256:trusted-host',
        },
      }),
    )
    renderViewer()

    fireEvent.change(screen.getByPlaceholderText('SSH password'), {
      target: { value: 'raw-secret-value' },
    })
    fireEvent.click(screen.getByRole('button', { name: /open terminal/i }))

    await waitFor(() => expect(h.createSecret).toHaveBeenCalled())
    expect(h.createSecret).toHaveBeenCalledWith({
      objectKey: 'build-host-ssh-password-1',
      displayName: 'Build Host SSH password',
      kind: SecretKindSSHPassword,
      contentType: SSHTextCredentialContentType,
      value: new TextEncoder().encode('raw-secret-value'),
      readerPublicKeyPem: new TextEncoder().encode(
        '-----BEGIN PUBLIC KEY-----\nmock\n-----END PUBLIC KEY-----',
      ),
    })

    const hostCall = h.applyWorldOp.mock.calls.find(
      ([opId]) => opId === CREATE_SSH_HOST_OP_ID,
    ) as [string, unknown, string?] | undefined
    if (!hostCall) throw new Error('expected SSH Host create op')
    const hostOpData: unknown = hostCall[1]
    if (!(hostOpData instanceof Uint8Array)) {
      throw new Error('expected SSH Host op bytes')
    }
    const hostOp = CreateSshHostOp.fromBinary(hostOpData)
    expect(hostOp.objectKey).toBe('build-host-1')
    expect(hostOp.endpoint).toEqual({
      host: 'build.example.com',
      port: 2222,
      username: 'ubuntu',
    })
    expect(hostOp.credentials).toEqual({
      passwordSecretObjectKey: 'build-host-ssh-password-1',
    })
    expect(JSON.stringify(hostOp)).not.toContain('raw-secret-value')

    const terminalCall = h.applyWorldOp.mock.calls.find(
      ([opId]) => opId === CREATE_TERMINAL_OP_ID,
    ) as [string, unknown, string?] | undefined
    if (!terminalCall) throw new Error('expected SSH Host terminal create op')
    const terminalOpData: unknown = terminalCall[1]
    if (!(terminalOpData instanceof Uint8Array)) {
      throw new Error('expected terminal op bytes')
    }
    const terminalOp = CreateTerminalOp.fromBinary(terminalOpData)
    expect(terminalOp.targetKind).toBe(TerminalTargetKind.SSH_HOST)
    expect(terminalOp.sshHostObjectKey).toBe('build-host-1')
    expect(terminalOp.deviceObjectKey ?? '').toBe('')
    expect(terminalOp.devicePeerId ?? '').toBe('')
    expect(JSON.stringify(terminalOp)).not.toContain('raw-secret-value')

    expect(
      h.applyWorldOp.mock.calls.some(
        ([opId]) => typeof opId === 'string' && opId.includes('/device'),
      ),
    ).toBe(false)
    expect(h.deleteObject).toHaveBeenCalledWith('wizard/device-setup')
    expect(h.navigateToObjects).toHaveBeenCalledWith(['build-host-terminal-1'])
  })

  it('allows private-key auth without passphrase or prefilled host key trust', async () => {
    h.currentStep = 1
    h.configData = new TextEncoder().encode(
      JSON.stringify({
        mode: 'ssh',
        ssh: {
          host: '192.168.1.15',
          port: 1940,
          username: 'root',
          authMode: 'private-key',
          hostKeyAlgorithm: '',
        },
      }),
    )
    renderViewer()

    const privateKey =
      '-----BEGIN OPENSSH PRIVATE KEY-----\nmock-key\n-----END OPENSSH PRIVATE KEY-----'
    fireEvent.change(
      screen.getByPlaceholderText('-----BEGIN OPENSSH PRIVATE KEY-----'),
      {
        target: { value: privateKey },
      },
    )
    const openButton = screen.getByRole('button', { name: /open terminal/i })
    expect(openButton).toHaveProperty('disabled', false)
    fireEvent.click(openButton)

    await waitFor(() => expect(h.createSecret).toHaveBeenCalledTimes(1))
    expect(h.createSecret).toHaveBeenCalledWith({
      objectKey: 'build-host-ssh-private-key-1',
      displayName: 'Build Host SSH private key',
      kind: SecretKindSSHPrivateKey,
      contentType: SSHPrivateKeyContentType,
      value: new TextEncoder().encode(privateKey),
      readerPublicKeyPem: new TextEncoder().encode(
        '-----BEGIN PUBLIC KEY-----\nmock\n-----END PUBLIC KEY-----',
      ),
    })

    const hostCall = h.applyWorldOp.mock.calls.find(
      ([opId]) => opId === CREATE_SSH_HOST_OP_ID,
    )
    if (!hostCall) throw new Error('expected SSH Host create op')
    const hostOpData: unknown = hostCall[1]
    if (!(hostOpData instanceof Uint8Array)) {
      throw new Error('expected SSH Host op bytes')
    }
    const hostOp = CreateSshHostOp.fromBinary(hostOpData)
    expect(hostOp.endpoint).toEqual({
      host: '192.168.1.15',
      port: 1940,
      username: 'root',
    })
    expect(hostOp.credentials).toEqual({
      privateKeySecretObjectKey: 'build-host-ssh-private-key-1',
    })
    expect(hostOp.hostKeyPins ?? []).toHaveLength(0)
  })

  it('normalizes ssh-keyscan host key lines when creating an SSH Host', async () => {
    h.currentStep = 1
    h.configData = new TextEncoder().encode(
      JSON.stringify({
        mode: 'ssh',
        ssh: {
          host: '192.168.1.15',
          port: 1940,
          username: 'root',
          authMode: 'password',
          hostKeyPublicKey:
            '[192.168.1.15]:1940 ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDtrusted',
        },
      }),
    )
    renderViewer()

    fireEvent.change(screen.getByPlaceholderText('SSH password'), {
      target: { value: 'raw-secret-value' },
    })
    fireEvent.click(screen.getByRole('button', { name: /open terminal/i }))

    await waitFor(() =>
      expect(h.applyWorldOp).toHaveBeenCalledWith(
        CREATE_SSH_HOST_OP_ID,
        expect.any(Uint8Array),
        '12D3KooWSession',
      ),
    )

    const hostCall = h.applyWorldOp.mock.calls.find(
      ([opId]) => opId === CREATE_SSH_HOST_OP_ID,
    )
    if (!hostCall) throw new Error('expected SSH Host create op')
    const hostOpData: unknown = hostCall[1]
    if (!(hostOpData instanceof Uint8Array)) {
      throw new Error('expected SSH Host op bytes')
    }
    const hostOp = CreateSshHostOp.fromBinary(hostOpData)
    expect(hostOp.endpoint).toEqual({
      host: '192.168.1.15',
      port: 1940,
      username: 'root',
    })
    const pins = hostOp.hostKeyPins ?? []
    expect(pins).toHaveLength(1)
    expect(pins[0]?.algorithm).toBe('ssh-rsa')
    expect(pins[0]?.publicKey).toBe(
      'ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDtrusted',
    )
  })

  it('stores bare authorized-key trust input when creating an SSH Host', async () => {
    h.currentStep = 1
    h.configData = new TextEncoder().encode(
      JSON.stringify({
        mode: 'ssh',
        ssh: {
          host: '192.168.1.15',
          port: 1940,
          username: 'root',
          authMode: 'password',
          hostKeyPublicKey:
            'ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAItrustedbarekey',
        },
      }),
    )
    renderViewer()

    fireEvent.change(screen.getByPlaceholderText('SSH password'), {
      target: { value: 'raw-secret-value' },
    })
    fireEvent.click(screen.getByRole('button', { name: /open terminal/i }))

    await waitFor(() =>
      expect(h.applyWorldOp).toHaveBeenCalledWith(
        CREATE_SSH_HOST_OP_ID,
        expect.any(Uint8Array),
        '12D3KooWSession',
      ),
    )

    const hostCall = h.applyWorldOp.mock.calls.find(
      ([opId]) => opId === CREATE_SSH_HOST_OP_ID,
    )
    if (!hostCall) throw new Error('expected SSH Host create op')
    const hostOpData: unknown = hostCall[1]
    if (!(hostOpData instanceof Uint8Array)) {
      throw new Error('expected SSH Host op bytes')
    }
    const hostOp = CreateSshHostOp.fromBinary(hostOpData)
    const pins = hostOp.hostKeyPins ?? []
    expect(pins).toHaveLength(1)
    expect(pins[0]?.algorithm).toBe('ssh-ed25519')
    expect(pins[0]?.publicKey).toBe(
      'ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAItrustedbarekey',
    )
  })

  it('opens an SSH installer terminal without creating a Device object', async () => {
    h.currentStep = 1
    h.configData = new TextEncoder().encode(
      JSON.stringify({
        mode: 'ssh',
        ssh: {
          host: 'install.example.com',
          port: 22,
          username: 'root',
          authMode: 'password',
          setupMode: 'install-agent',
          hostKeyAlgorithm: 'ssh-ed25519',
          hostKeyFingerprint: 'SHA256:installer-host',
        },
      }),
    )
    renderViewer()

    expect(screen.getByText('Remote setup')).toBeTruthy()
    expect(screen.getByText(/spacewave device setup/)).toBeTruthy()
    fireEvent.change(screen.getByPlaceholderText('SSH password'), {
      target: { value: 'installer-secret' },
    })
    fireEvent.click(screen.getByRole('button', { name: /open installer/i }))

    await waitFor(() =>
      expect(h.applyWorldOp).toHaveBeenCalledWith(
        CREATE_TERMINAL_OP_ID,
        expect.any(Uint8Array),
        '12D3KooWSession',
      ),
    )
    const terminalCall = h.applyWorldOp.mock.calls.find(
      ([opId]) => opId === CREATE_TERMINAL_OP_ID,
    ) as [string, unknown, string?] | undefined
    if (!terminalCall) throw new Error('expected installer terminal create op')
    const terminalOpData: unknown = terminalCall[1]
    if (!(terminalOpData instanceof Uint8Array)) {
      throw new Error('expected terminal op bytes')
    }
    const terminalOp = CreateTerminalOp.fromBinary(terminalOpData)
    expect(terminalOp.targetKind).toBe(TerminalTargetKind.SSH_HOST)
    expect(terminalOp.command).toContain('spacewave device setup')
    expect(terminalOp.command).toContain('--target-hint space-1')
    expect(terminalOp.deviceObjectKey ?? '').toBe('')
    expect(terminalOp.devicePeerId ?? '').toBe('')

    expect(h.deleteObject).not.toHaveBeenCalled()
    expect(h.navigateToObjects).toHaveBeenCalledWith(['build-host-terminal-1'])
    const update = h.updateState.mock.calls.at(-1)?.[0] as
      | { configData?: Uint8Array; step?: number }
      | undefined
    if (!update?.configData) {
      throw new Error('expected installer status update')
    }
    expect(update.step).toBeUndefined()
    const config = JSON.parse(new TextDecoder().decode(update.configData)) as {
      ssh?: { setupStatus?: { state?: string; terminalObjectKey?: string } }
    }
    expect(config.ssh?.setupStatus).toEqual({
      state: 'terminal_created',
      hostObjectKey: 'build-host-1',
      terminalObjectKey: 'build-host-terminal-1',
      message: 'installer terminal opened',
    })
    expect(
      h.applyWorldOp.mock.calls.some(
        ([opId]) => typeof opId === 'string' && opId.includes('/device'),
      ),
    ).toBe(false)
  })
})

function renderViewer() {
  render(
    <AddDeviceWizardViewer
      objectInfo={{}}
      worldState={{
        value: null,
        loading: false,
        error: null,
        retry: vi.fn(),
      }}
    />,
  )
}

function bytesToBase64(bytes: Uint8Array): string {
  let binary = ''
  for (const byte of bytes) binary += String.fromCharCode(byte)
  return btoa(binary)
}

function base64ToBytes(value: string): Uint8Array {
  const binary = atob(value)
  return Uint8Array.from(binary, (char) => char.charCodeAt(0))
}
