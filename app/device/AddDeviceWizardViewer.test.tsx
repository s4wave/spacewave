import { spawnSync } from 'child_process'
import {
  chmodSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from 'fs'
import { tmpdir } from 'os'
import { join } from 'path'

import React, { type DependencyList } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'

import { SOParticipantRole } from '@s4wave/core/sobject/sobject.pb.js'
import { SetSpaceIndexPathOp } from '@s4wave/core/space/world/ops/ops.pb.js'
import { SET_SPACE_INDEX_PATH_OP_ID } from '@s4wave/core/space/world/ops/set-space-settings.js'
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
import { CreateSshHostOp, SshHost } from '@s4wave/sdk/sshhost/sshhost.pb.js'
import { CREATE_TERMINAL_OP_ID } from '@s4wave/sdk/terminal/create-terminal.js'
import type { WorldQuery } from '@s4wave/sdk/world/world-query.js'
import { WorldErrorCode } from '@s4wave/sdk/world/world.pb.js'
import type { WorldMutation } from '@s4wave/sdk/world/world.pb.js'
import {
  CreateTerminalOp,
  Terminal,
  TerminalTargetKind,
} from '@s4wave/sdk/terminal/terminal.pb.js'
import type { Quad } from '@go/github.com/s4wave/spacewave/db/block/quad/quad.pb.js'
import { keyToIRI } from '@s4wave/sdk/world/graph-utils.js'
import { keysWorld } from '@s4wave/web/test/world-query.js'

import { ComputersDashboardTypeID } from './computers.js'
import { deriveSshHostKeys } from './ssh-host-keys.js'
import { PRED_WIZARD_SSH_HOST } from './ssh-host-made.js'
import type {
  UseWizardStateOptions,
  WizardCancelContext,
} from '../wizard/useWizardState.js'

const h = vi.hoisted(() => ({
  computersError: null as Error | null,
  computersRetry: vi.fn(),
  applyWorldOp: vi.fn().mockResolvedValue({ seqno: 1n, sysErr: false }),
  deleteObject: vi.fn().mockResolvedValue({ deleted: true }),
  commit: vi.fn().mockResolvedValue(undefined),
  handleCancel: vi.fn().mockResolvedValue(undefined),
  navigateToObjects: vi.fn(),
  navigate: vi.fn(),
  updateState: vi.fn().mockResolvedValue(undefined),
  persistDraftState: vi.fn().mockResolvedValue(undefined),
  handleConfigDataChange: vi.fn(),
  setCreating: vi.fn(),
  previewSpaceLink: vi.fn(),
  approveSpaceLink: vi.fn(),
  createSecret: vi.fn(),
  writeSecretPayload: vi.fn().mockResolvedValue({}),
  deleteSecret: vi.fn(),
  // secrets holds the keys of the Secrets created so far.
  secrets: new Set<string>(),
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
  // quads and bodies are what committed batches linked and created.
  quads: [] as Quad[],
  bodies: {} as Record<string, Uint8Array>,
  // wizardOptions and spaceWorld are what the viewer last gave and got from
  // the wizard hook.
  wizardOptions: undefined as UseWizardStateOptions | undefined,
  spaceWorld: undefined as WizardCancelContext['spaceWorld'] | undefined,
}))

// createSecret refuses a key that holds a Secret, as the Space does.
h.createSecret.mockImplementation((req: { objectKey?: string }) => {
  const key = req.objectKey ?? ''
  if (h.secrets.has(key)) {
    return Promise.reject(new Error('object already exists'))
  }
  h.secrets.add(key)
  return Promise.resolve({ secret: {} })
})

// deleteSecret removes a Secret, and does nothing for a key that holds none,
// as the Space does.
h.deleteSecret.mockImplementation((req: { objectKey?: string }) => {
  h.secrets.delete(req.objectKey ?? '')
  return Promise.resolve({})
})

// batchThroughSpies applies a Finish batch through the hoisted spies, in
// order, and lands it through commit. An SSH Host or terminal it creates, and
// a quad it sets, become readable only once the commit lands.
async function batchThroughSpies(mutations: WorldMutation[]) {
  const quads: Quad[] = []
  const bodies: Record<string, Uint8Array> = {}
  for (const [failedIndex, m] of mutations.entries()) {
    if (m.mutation?.case === 'op') {
      const op = m.mutation.value
      await h.applyWorldOp(op.opTypeId, op.opData, op.opSender)
      // A create under a key that exists is refused, as its op refuses it.
      const created =
        op.opTypeId === CREATE_SSH_HOST_OP_ID && op.opData
          ? CreateSshHostOp.fromBinary(op.opData).objectKey
          : op.opTypeId === CREATE_TERMINAL_OP_ID && op.opData
            ? CreateTerminalOp.fromBinary(op.opData).objectKey
            : undefined
      if (created && h.bodies[created]) {
        throw new Error('object already exists')
      }
      if (op.opTypeId === CREATE_SSH_HOST_OP_ID && op.opData) {
        const host = CreateSshHostOp.fromBinary(op.opData)
        bodies[host.objectKey ?? ''] = SshHost.toBinary({
          label: host.label,
          endpoint: host.endpoint,
          credentials: host.credentials,
        })
      }
      if (op.opTypeId === CREATE_TERMINAL_OP_ID && op.opData) {
        const terminal = CreateTerminalOp.fromBinary(op.opData)
        bodies[terminal.objectKey ?? ''] = Terminal.toBinary({
          sshHostObjectKey: terminal.sshHostObjectKey,
        })
      }
    } else if (m.mutation?.case === 'setGraphQuad') {
      quads.push(m.mutation.value.quad ?? {})
    } else if (m.mutation?.case === 'deleteObject') {
      const { deleted } = await h.deleteObject(m.mutation.value.objectKey)
      if (!deleted) {
        return { failedIndex, errorCode: WorldErrorCode.OBJECT_NOT_FOUND }
      }
    }
  }
  await h.commit()
  h.quads.push(...quads)
  Object.assign(h.bodies, bodies)
  return { seqno: 1n }
}

// linkedWorld answers quad lookups and body reads from what batches landed,
// and object lookups from those bodies, the Secrets and the seeded objects.
const linkedWorld = {
  lookupGraphQuads: (subject?: string, predicate?: string) =>
    Promise.resolve({
      quads: h.quads.filter(
        (q) =>
          (!subject || q.subject === subject) &&
          (!predicate || q.predicate === predicate),
      ),
    }),
  getObjectBodies: (keys: string[]) =>
    Promise.resolve(
      keys.map((objectKey) => {
        const body = h.bodies[objectKey]
        return body
          ? { objectKey, exists: true, body }
          : { objectKey, exists: false }
      }),
    ),
  getObject: (key: string) =>
    Promise.resolve(
      h.bodies[key] ||
        h.secrets.has(key) ||
        h.worldObjects.some((obj) => obj.objectKey === key)
        ? { [Symbol.dispose]: () => {} }
        : null,
    ),
}

vi.mock('../wizard/useWizardState.js', () => ({
  useWizardState: (
    _props: unknown,
    _configTypeId: unknown,
    options?: UseWizardStateOptions,
  ) => {
    h.wizardOptions = options
    const ws = wizardStateResult()
    h.spaceWorld = ws.spaceWorld as unknown as WizardCancelContext['spaceWorld']
    return ws
  },
}))

// wizardStateResult is the wizard hook's result for the hoisted state.
function wizardStateResult() {
  return {
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
    // Finish writes through a batch that passes each write to the same
    // spies, then lands through commit, so a test reads one call order
    // across both.
    spaceWorld: {
      ...keysWorld(() => h.worldObjects.map((obj) => obj.objectKey)),
      applyWorldOp: h.applyWorldOp,
      deleteObject: h.deleteObject,
      ...linkedWorld,
      getEngine: () => ({ executeWorldBatch: batchThroughSpies }),
    },
    spaceSettings: {
      indexPath: h.spaceSettingsIndexPath,
      pluginIds: ['spacewave-web'],
    },
    navigateToObjects: h.navigateToObjects,
    wizardResource: { value: { updateState: h.updateState } },
    configEditor: { element: null, value: undefined },
    configData: h.configData,
    persistDraftState: h.persistDraftState,
    handleConfigDataChange: h.handleConfigDataChange,
    handleUpdateName: vi.fn(),
    handleBack: vi.fn(),
    cancel: { status: 'idle' },
    handleCancel: h.handleCancel,
  }
}

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
        writeSecretPayload: h.writeSecretPayload,
        deleteSecret: h.deleteSecret,
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
      spaceWorldResource: {},
    }),
  },
}))

vi.mock('@s4wave/web/hooks/useWorldQuery.js', async () => {
  const { typedObjectsWorld, useFakeWorldQuery } =
    await import('@s4wave/web/test/world-query.js')
  // The World is built on first use: this factory runs before the module's
  // own constants are initialized.
  let world: ReturnType<typeof typedObjectsWorld> | undefined
  const getWorld = () =>
    (world ??= {
      ...typedObjectsWorld(() => h.worldObjects),
      ...linkedWorld,
    } as Partial<ReturnType<typeof typedObjectsWorld>> as ReturnType<
      typeof typedObjectsWorld
    >)
  return {
    useWorldQuery: <T,>(
      _world: unknown,
      query: WorldQuery<T>,
      deps: DependencyList,
    ) => {
      const read = useFakeWorldQuery(getWorld(), query, deps)
      if (!h.computersError) return read
      return {
        value: null,
        loading: false,
        error: h.computersError,
        last: null,
        retry: h.computersRetry,
      }
    },
  }
})

vi.mock('@s4wave/web/ui/toaster.js', () => ({
  toast: {
    success: h.toastSuccess,
    error: h.toastError,
  },
}))

import { AddDeviceWizardViewer } from './AddDeviceWizardViewer.js'

// pairedConfig is the wizard config after approving Build Host's ticket,
// whose Device object takes devices/build-host once it joins.
function pairedConfig(): Uint8Array {
  return new TextEncoder().encode(
    JSON.stringify({
      completion: 'completion',
      deviceKey: 'devices/build-host',
      preview: { label: 'Build Host' },
    }),
  )
}

// expectIndexMovedBeforeDelete asserts that Finish moved the Space index off
// the wizard to indexPath, as the session, before Finish deleted the wizard.
function expectIndexMovedBeforeDelete(indexPath: string) {
  const at = h.applyWorldOp.mock.calls.findIndex(
    ([opId]) => opId === SET_SPACE_INDEX_PATH_OP_ID,
  )
  if (at < 0) throw new Error('expected a Space index op')
  const [, opData, sender] = h.applyWorldOp.mock.calls[at] as [
    string,
    unknown,
    string,
  ]
  if (!(opData instanceof Uint8Array)) throw new Error('expected op bytes')
  expect(SetSpaceIndexPathOp.fromBinary(opData).indexPath).toBe(indexPath)
  expect(sender).toBe('12D3KooWSession')

  // The index write lands before the wizard is deleted, and both commit
  // together in the last batch.
  expect(h.deleteObject.mock.calls[0]?.[0]).toBe('wizard/device-setup')
  expect(h.applyWorldOp.mock.invocationCallOrder[at]).toBeLessThan(
    h.deleteObject.mock.invocationCallOrder[0] ?? 0,
  )
  expect(h.deleteObject.mock.invocationCallOrder[0]).toBeLessThan(
    h.commit.mock.invocationCallOrder.at(-1) ?? 0,
  )
}

function installSetupCommand(): string {
  return `tmp=$(mktemp) || exit; trap 'rm -f "$tmp"' 0 HUP INT TERM; curl -fsSL --output "$tmp" https://raw.githubusercontent.com/s4wave/spacewave/master/scripts/spacewave.sh || exit; sh "$tmp" device setup --label 'Build Host' --target-hint space-1`
}

function renderedInstallSetupCommand(): string {
  const command = screen.getByText(installSetupCommand()).textContent
  if (!command) throw new Error('expected the install setup command')
  return command
}

describe('AddDeviceWizardViewer', () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
    h.computersError = null
    h.currentStep = 1
    h.configData = undefined
    h.providerId = 'spacewave'
    h.sessionInfoLoading = false
    h.spaceSettingsIndexPath = 'wizard/device-setup'
    h.quads = []
    h.bodies = {}
    h.secrets.clear()
    h.worldObjects = [
      { objectKey: 'computers', objectType: ComputersDashboardTypeID },
      { objectKey: 'devices/build-host', objectType: 'spacewave/device' },
    ]
  })

  it('shows direct and bootstrap setup commands before the Device ticket is available', () => {
    renderViewer()

    expect(screen.getByText('Add Device')).toBeTruthy()
    expect(screen.queryByText('Link My Device')).toBeNull()
    expect(screen.getByText('Spacewave already installed')).toBeTruthy()
    expect(screen.getByText('Install and run')).toBeTruthy()
    expect(
      screen.getByText(
        /^spacewave device setup --label 'Build Host' --target-hint space-1$/,
      ),
    ).toBeTruthy()
    expect(screen.getByText(installSetupCommand())).toBeTruthy()
    expect(screen.getByText('2. Paste the Device ticket')).toBeTruthy()
    expect(screen.queryByRole('button', { name: /^approve$/i })).toBeNull()
  })

  it('does not execute a truncated bootstrap when curl fails', () => {
    renderViewer()
    const dir = mkdtempSync(join(tmpdir(), 'spacewave-device-bootstrap-'))
    const bin = join(dir, 'bin')
    const marker = join(dir, 'script-ran')
    const curl = join(bin, 'curl')
    try {
      mkdirSync(bin)
      writeFileSync(
        curl,
        `#!/bin/sh
set -eu
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--output" ]; then
    output="$2"
    break
  fi
  shift
done
printf '%s\\n' 'printf "%s\\n" script-ran > "$SPACEWAVE_TEST_MARKER"' > "\${output:?}"
printf '%s' 'truncated' >> "\${output:?}"
exit 22
`,
      )
      chmodSync(curl, 0o755)

      const result = spawnSync('sh', ['-c', renderedInstallSetupCommand()], {
        encoding: 'utf8',
        env: {
          ...process.env,
          PATH: `${bin}:${process.env.PATH}`,
          SPACEWAVE_TEST_MARKER: marker,
        },
      })

      expect(result.error).toBeUndefined()
      expect(result.status).not.toBe(0)
      expect(existsSync(marker)).toBe(false)
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  })

  it('runs the downloaded bootstrap with the displayed setup arguments', () => {
    renderViewer()
    const dir = mkdtempSync(join(tmpdir(), 'spacewave-device-bootstrap-'))
    const bin = join(dir, 'bin')
    const args = join(dir, 'args')
    const curl = join(bin, 'curl')
    try {
      mkdirSync(bin)
      writeFileSync(
        curl,
        `#!/bin/sh
set -eu
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--output" ]; then
    output="$2"
    break
  fi
  shift
done
printf '%s\\n' '#!/bin/sh' 'printf "%s\\n" "$@" > "$SPACEWAVE_TEST_ARGS"' > "\${output:?}"
`,
      )
      chmodSync(curl, 0o755)

      const result = spawnSync('sh', ['-c', renderedInstallSetupCommand()], {
        encoding: 'utf8',
        env: {
          ...process.env,
          PATH: `${bin}:${process.env.PATH}`,
          SPACEWAVE_TEST_ARGS: args,
        },
      })

      expect(result.error).toBeUndefined()
      expect(result.status).toBe(0)
      expect(readFileSync(args, 'utf8')).toBe(
        'device\nsetup\n--label\nBuild Host\n--target-hint\nspace-1\n',
      )
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  })

  it('uses the shared compact input class for the Device name', () => {
    h.currentStep = 0
    renderViewer()

    const name = screen.getByLabelText('Device Name')
    expect(name.className).toContain('text-xs')
    expect(name.className).not.toMatch(/(^|\s)text-base($|\s)/)
    expect(name.className).not.toMatch(/(^|\s)text-sm($|\s)/)
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
      deviceObjectKey: 'devices/build-host',
    })
    h.configData = new TextEncoder().encode(JSON.stringify({ ticket }))
    renderViewer()

    expect(
      screen.getByPlaceholderText(/paste the base64 ticket/i),
    ).toHaveProperty('value', ticket)
    expect(screen.getByText('Approve Device ticket')).toBeTruthy()
    expect(screen.queryByText('Spacewave already installed')).toBeNull()
    expect(screen.queryByText('Install and run')).toBeNull()
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
      deviceKey?: string
    }
    expect(config.ticket).toBe(ticket)
    expect(config.deviceKey).toBe('devices/build-host')
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

  it('refuses an approval that does not report the Device key', async () => {
    const ticket = bytesToBase64(new Uint8Array([1, 2, 3]))
    h.previewSpaceLink.mockResolvedValue({
      label: 'Build Host',
      completionMode: SpaceLinkCompletionMode.SpaceLinkCompletionMode_CLI,
    })
    // An older responder sends no device_object_key.
    h.approveSpaceLink.mockResolvedValue({
      completionMode: SpaceLinkCompletionMode.SpaceLinkCompletionMode_CLI,
      completion: {
        status: SpaceLinkCallbackStatus.SpaceLinkCallbackStatus_OK,
        nonce: new Uint8Array([9]),
      },
    })
    h.configData = new TextEncoder().encode(JSON.stringify({ ticket }))
    renderViewer()

    fireEvent.click(screen.getByRole('button', { name: /approve/i }))

    await waitFor(() =>
      expect(h.toastError).toHaveBeenCalledWith(
        'Ticket could not be approved. This Spacewave version did not report the Device key. Update Spacewave and approve the ticket again.',
      ),
    )
    expect(h.updateState).not.toHaveBeenCalled()
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
    expect(screen.queryByText('Spacewave already installed')).toBeNull()
    expect(screen.queryByText('Install and run')).toBeNull()
    expect(screen.queryByText(/^spacewave device setup/)).toBeNull()
    expect(
      screen.queryByText(/raw\.githubusercontent\.com\/s4wave\/spacewave/),
    ).toBeNull()
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
    h.configData = pairedConfig()
    renderViewer()

    expect(await screen.findByText('Build Host joined')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: /open device/i }))

    await waitFor(() => expect(h.deleteObject).toHaveBeenCalled())
    expectIndexMovedBeforeDelete('computers')
    expect(h.navigateToObjects).toHaveBeenCalledWith(['devices/build-host'])
  })

  it('opens the paired Device when its Finish is refused because the wizard is gone', async () => {
    // Another tab cancelled the wizard after this tab paired the device.
    h.deleteObject.mockResolvedValueOnce({ deleted: false })
    h.currentStep = 2
    h.configData = pairedConfig()
    renderViewer()

    expect(await screen.findByText('Build Host joined')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: /open device/i }))

    // The list wrote nothing, yet the device is real, so it opens.
    await waitFor(() =>
      expect(h.navigateToObjects).toHaveBeenCalledWith(['devices/build-host']),
    )
    expect(h.commit).not.toHaveBeenCalled()
    expect(screen.queryByText(/Could not finish/)).toBeNull()
  })

  it('waits for the approved Device to join and opens Computers until it does', async () => {
    // Another Device has joined, but not the one the ticket approved.
    h.worldObjects = [
      { objectKey: 'computers', objectType: ComputersDashboardTypeID },
      { objectKey: 'devices/aaa-laptop', objectType: 'spacewave/device' },
    ]
    h.currentStep = 2
    h.configData = pairedConfig()
    renderViewer()

    expect(
      await screen.findByText('Waiting for Build Host to join'),
    ).toBeTruthy()
    expect(screen.getByText('Complete setup')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: /open computers/i }))

    await waitFor(() =>
      expect(h.navigateToObjects).toHaveBeenCalledWith(['computers']),
    )
    expect(h.navigateToObjects).toHaveBeenCalledTimes(1)
  })

  it('holds Finish while the computers read failed, so the index never names a deleted wizard', () => {
    h.currentStep = 2
    h.configData = new TextEncoder().encode(
      JSON.stringify({ completion: 'completion' }),
    )
    h.computersError = new Error('world closed')
    renderViewer()

    expect(screen.getByRole('alert').textContent).toContain(
      "Could not read the Space's computers: world closed",
    )
    const finish = screen.getByRole('button', { name: /open computers/i })
    expect((finish as HTMLButtonElement).disabled).toBe(true)
    fireEvent.click(finish)
    expect(h.deleteObject).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(h.computersRetry).toHaveBeenCalledTimes(1)
  })

  it('cancels with a cleared index while the computers read failed', async () => {
    // The computers read failed, so no dashboard is known.
    h.computersError = new Error('world closed')
    renderViewer()

    // Cancel stays available and asks the hook to clear the index.
    fireEvent.click(screen.getByRole('button', { name: 'Delete wizard' }))
    await waitFor(() => expect(h.handleCancel).toHaveBeenCalledWith(''))
  })

  it('creates an SSH-only Host without unavailable-install or runtime review copy', async () => {
    h.currentStep = 1
    h.configData = new TextEncoder().encode(
      JSON.stringify({
        mode: 'ssh',
        ssh: {
          host: 'build.example.com',
          port: 2222,
          username: 'ubuntu',
          authMode: 'password',
          setupMode: 'host',
          hostKeyAlgorithm: 'ssh-ed25519',
          hostKeyFingerprint: 'SHA256:trusted-host',
        },
      }),
    )
    renderViewer()

    expect(screen.getByPlaceholderText('host.example.com')).toHaveProperty(
      'value',
      'build.example.com',
    )
    expect(screen.getByPlaceholderText('SSH password')).toBeTruthy()
    await enabledButton(/add ssh host and open terminal/i)
    expect(screen.queryByText('Runtime')).toBeNull()
    expect(screen.queryByText('Native SSH connector required')).toBeNull()
    expect(
      screen.queryByText(
        'Install Agent is not available until secure bootstrap is configured.',
      ),
    ).toBeNull()

    fireEvent.change(screen.getByPlaceholderText('SSH password'), {
      target: { value: 'raw-secret-value' },
    })
    fireEvent.click(await enabledButton(/open terminal/i))

    const keys = await sshKeys()
    await waitFor(() => expect(h.createSecret).toHaveBeenCalled())
    expect(h.createSecret).toHaveBeenCalledWith({
      objectKey: keys.password,
      displayName: 'Build Host SSH password',
      kind: SecretKindSSHPassword,
      contentType: SSHTextCredentialContentType,
      value: new TextEncoder().encode('raw-secret-value'),
      readerPublicKeyPem: new TextEncoder().encode(
        '-----BEGIN PUBLIC KEY-----\nmock\n-----END PUBLIC KEY-----',
      ),
      participantRole: SOParticipantRole.SOParticipantRole_WRITER,
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
    expect(hostOp.objectKey).toBe(keys.host)
    expect(hostOp.endpoint).toEqual({
      host: 'build.example.com',
      port: 2222,
      username: 'ubuntu',
    })
    expect(hostOp.credentials).toEqual({
      passwordSecretObjectKey: keys.password,
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
    expect(terminalOp.sshHostObjectKey).toBe(keys.host)
    expect(terminalOp.deviceObjectKey ?? '').toBe('')
    expect(terminalOp.devicePeerId ?? '').toBe('')
    expect(JSON.stringify(terminalOp)).not.toContain('raw-secret-value')

    expect(
      h.applyWorldOp.mock.calls.some(
        ([opId]) => typeof opId === 'string' && opId.includes('/device'),
      ),
    ).toBe(false)
    expectIndexMovedBeforeDelete('computers')
    expect(h.navigateToObjects).toHaveBeenCalledWith([`${keys.host}-terminal`])
  })

  it('keeps the SSH Host and terminal it made when Finish fails, and Retry creates neither again', async () => {
    // The person entered a host and its password.
    h.currentStep = 1
    h.configData = sshConfigData('build.example.com')
    renderViewer()
    fireEvent.change(screen.getByPlaceholderText('SSH password'), {
      target: { value: 'raw-secret-value' },
    })

    // The host and terminal land, then the wizard removal fails.
    failWizardRemoval()
    fireEvent.click(await enabledButton(/open terminal/i))
    expect(
      await screen.findByText('Could not finish: commit failed'),
    ).toBeTruthy()
    expect(h.navigateToObjects).not.toHaveBeenCalled()

    // Retry resumes after the terminal: one secret, host and terminal.
    fireEvent.click(await enabledButton(/retry/i))
    const terminalKey = `${(await sshKeys()).host}-terminal`
    await waitFor(() =>
      expect(h.navigateToObjects).toHaveBeenCalledWith([terminalKey]),
    )
    const count = (id: string) =>
      h.applyWorldOp.mock.calls.filter(([opId]) => opId === id).length
    expect(h.createSecret).toHaveBeenCalledTimes(1)
    expect(sshHostEndpoints()).toEqual(['build.example.com'])
    expect(count(CREATE_TERMINAL_OP_ID)).toBe(1)
  })

  it('adds the host the person entered when they edit it after a failed Finish', async () => {
    // The person entered a host and its password.
    h.currentStep = 1
    h.configData = sshConfigData('build.example.com')
    const view = renderViewer()
    fireEvent.change(screen.getByPlaceholderText('SSH password'), {
      target: { value: 'raw-secret-value' },
    })

    // The first host lands, then the wizard removal fails.
    failWizardRemoval()
    fireEvent.click(await enabledButton(/open terminal/i))
    expect(
      await screen.findByText(/^Could not finish: commit failed/),
    ).toBeTruthy()

    // The person enters another host, and Retry adds that one.
    h.configData = sshConfigData('other.example.com')
    view.rerender(viewer())
    fireEvent.click(await enabledButton(/retry/i))
    await waitFor(() => expect(h.navigateToObjects).toHaveBeenCalled())
    expect(sshHostEndpoints()).toEqual([
      'build.example.com',
      'other.example.com',
    ])
    expect(h.createSecret).toHaveBeenCalledTimes(2)
  })

  it('reuses the SSH Host and terminal it made before a reload, without asking for credentials', async () => {
    // The host and terminal land, then the wizard removal fails.
    h.currentStep = 1
    h.configData = sshConfigData('build.example.com')
    renderViewer()
    fireEvent.change(screen.getByPlaceholderText('SSH password'), {
      target: { value: 'raw-secret-value' },
    })
    failWizardRemoval()
    fireEvent.click(await enabledButton(/open terminal/i))
    expect(
      await screen.findByText('Could not finish: commit failed'),
    ).toBeTruthy()

    // A reload drops everything in memory, but the wizard's links reference
    // the host, so the step identifies the host in place of the credential
    // fields.
    cleanup()
    renderViewer()
    expect(await screen.findByText('SSH Host already added')).toBeTruthy()
    expect(screen.queryByPlaceholderText('SSH password')).toBeNull()

    // Finish opens the terminal made before and creates nothing again.
    fireEvent.click(await enabledButton(/open terminal/i))
    const terminalKey = `${(await sshKeys()).host}-terminal`
    await waitFor(() =>
      expect(h.navigateToObjects).toHaveBeenCalledWith([terminalKey]),
    )
    const count = (id: string) =>
      h.applyWorldOp.mock.calls.filter(([opId]) => opId === id).length
    expect(h.createSecret).toHaveBeenCalledTimes(1)
    expect(sshHostEndpoints()).toEqual(['build.example.com'])
    expect(count(CREATE_TERMINAL_OP_ID)).toBe(1)
  })

  it('replaces the Secret with the corrected password when the host create failed', async () => {
    // The person entered a host and a wrong password.
    h.currentStep = 1
    h.configData = sshConfigData('build.example.com')
    renderViewer()
    fireEvent.change(screen.getByPlaceholderText('SSH password'), {
      target: { value: 'wrong-fake-password' },
    })

    // The Secret lands, then the host create fails.
    h.commit.mockRejectedValueOnce(new Error('host create failed'))
    fireEvent.click(await enabledButton(/open terminal/i))
    expect(
      await screen.findByText(/^Could not finish: host create failed/),
    ).toBeTruthy()

    // The person corrects the password and presses again: the Secret under
    // the same key takes the corrected payload, and the host lands once.
    fireEvent.change(screen.getByPlaceholderText('SSH password'), {
      target: { value: 'corrected-fake-password' },
    })
    fireEvent.click(await enabledButton(/retry/i))
    const keys = await sshKeys()
    const terminalKey = `${keys.host}-terminal`
    await waitFor(() =>
      expect(h.navigateToObjects).toHaveBeenCalledWith([terminalKey]),
    )
    expect(h.createSecret).toHaveBeenCalledTimes(1)
    expect(h.writeSecretPayload).toHaveBeenCalledTimes(1)
    expect(h.writeSecretPayload).toHaveBeenCalledWith({
      objectKey: keys.password,
      expectedKind: SecretKindSSHPassword,
      contentType: SSHTextCredentialContentType,
      value: new TextEncoder().encode('corrected-fake-password'),
    })
    expect(Object.keys(h.bodies).sort()).toEqual([keys.host, terminalKey])
  })

  it('keeps one Secret, host and terminal when another tab creates them first', async () => {
    h.currentStep = 1
    h.configData = sshConfigData('build.example.com')
    renderViewer()
    fireEvent.change(screen.getByPlaceholderText('SSH password'), {
      target: { value: 'fake-password' },
    })

    // Another tab with the same entries lands the Secret, then the host
    // with the wizard's link, while this press runs.
    const keys = await sshKeys()
    h.createSecret.mockImplementationOnce((req: { objectKey?: string }) => {
      h.secrets.add(req.objectKey ?? '')
      h.bodies[keys.host] = SshHost.toBinary({
        label: 'Build Host',
        endpoint: { host: 'build.example.com', port: 2222, username: 'ubuntu' },
        credentials: { passwordSecretObjectKey: keys.password },
      })
      h.quads.push({
        subject: keyToIRI('wizard/device-setup'),
        predicate: PRED_WIZARD_SSH_HOST,
        obj: keyToIRI(keys.host),
      })
      return Promise.reject(new Error('object already exists'))
    })
    fireEvent.click(await enabledButton(/open terminal/i))

    // This press writes its payload to the Secret and opens the host the
    // other tab made.
    const terminalKey = `${keys.host}-terminal`
    await waitFor(() =>
      expect(h.navigateToObjects).toHaveBeenCalledWith([terminalKey]),
    )
    expect([...h.secrets]).toEqual([keys.password])
    expect(h.writeSecretPayload).toHaveBeenCalledTimes(1)
    expect(Object.keys(h.bodies).sort()).toEqual([keys.host, terminalKey])
  })

  it('deletes the Secret a failed Finish created when the wizard is cancelled', async () => {
    h.currentStep = 1
    h.configData = sshConfigData('build.example.com')
    renderViewer()
    fireEvent.change(screen.getByPlaceholderText('SSH password'), {
      target: { value: 'fake-password' },
    })

    // The Secret lands, then the host create fails.
    h.commit.mockRejectedValueOnce(new Error('host create failed'))
    fireEvent.click(await enabledButton(/open terminal/i))
    expect(
      await screen.findByText(/^Could not finish: host create failed/),
    ).toBeTruthy()
    const keys = await sshKeys()
    expect([...h.secrets]).toEqual([keys.password])

    // Cancel deletes the Secret, since no host references the Secret.
    await cancelWithRecordedConfig()
    expect(h.deleteSecret).toHaveBeenCalledWith({ objectKey: keys.password })
    expect(h.secrets.size).toBe(0)
  })

  it('keeps the Secret an SSH Host the wizard made names when the wizard is cancelled', async () => {
    h.currentStep = 1
    h.configData = sshConfigData('build.example.com')
    renderViewer()
    fireEvent.change(screen.getByPlaceholderText('SSH password'), {
      target: { value: 'fake-password' },
    })

    // The host and terminal land, then the wizard removal fails.
    failWizardRemoval()
    fireEvent.click(await enabledButton(/open terminal/i))
    expect(
      await screen.findByText('Could not finish: commit failed'),
    ).toBeTruthy()

    // Cancel leaves the host's Secret in place.
    await cancelWithRecordedConfig()
    const keys = await sshKeys()
    expect(h.deleteSecret).not.toHaveBeenCalledWith({
      objectKey: keys.password,
    })
    expect([...h.secrets]).toEqual([keys.password])
  })

  it('ends clean when a Finish fails after recording its Secret keys and before creating them', async () => {
    h.currentStep = 1
    h.configData = sshConfigData('build.example.com')
    renderViewer()
    fireEvent.change(screen.getByPlaceholderText('SSH password'), {
      target: { value: 'fake-password' },
    })

    // The keys are recorded, then the Secret create fails.
    h.createSecret.mockRejectedValueOnce(new Error('secret create failed'))
    fireEvent.click(await enabledButton(/open terminal/i))
    expect(
      await screen.findByText(/^Could not finish: secret create failed/),
    ).toBeTruthy()
    expect(h.updateState.mock.invocationCallOrder[0]).toBeLessThan(
      h.createSecret.mock.invocationCallOrder[0] ?? 0,
    )

    // Cancel deletes every recorded key, though none holds a Secret.
    await cancelWithRecordedConfig()
    const keys = await sshKeys()
    expect(h.deleteSecret.mock.calls.map(([req]) => req)).toEqual([
      { objectKey: keys.password },
      { objectKey: keys.privateKey },
      { objectKey: keys.passphrase },
    ])
    expect(h.secrets.size).toBe(0)
    expect(Object.keys(h.bodies)).toEqual([])
  })

  it('allows a blank SSH password for empty-password or no-auth hosts', async () => {
    h.currentStep = 1
    h.configData = new TextEncoder().encode(
      JSON.stringify({
        mode: 'ssh',
        ssh: {
          host: 'build.example.com',
          port: 22,
          username: 'ubuntu',
          authMode: 'password',
          hostKeyAlgorithm: 'ssh-ed25519',
          hostKeyFingerprint: 'SHA256:trusted-host',
        },
      }),
    )
    renderViewer()

    expect(
      screen.getByText(
        'Leave blank if the host accepts an empty password or requires no authentication.',
      ),
    ).toBeTruthy()
    fireEvent.click(await enabledButton(/open terminal/i))

    const keys = await sshKeys('build.example.com', 22)
    await waitFor(() => expect(h.createSecret).toHaveBeenCalled())
    expect(h.createSecret).toHaveBeenCalledWith(
      expect.objectContaining({
        kind: SecretKindSSHPassword,
        value: new Uint8Array(),
      }),
    )
    expect(
      h.applyWorldOp.mock.calls.some(
        ([opId]) => opId === CREATE_SSH_HOST_OP_ID,
      ),
    ).toBe(true)
    expect(h.navigateToObjects).toHaveBeenCalledWith([`${keys.host}-terminal`])
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
    fireEvent.click(await enabledButton(/open terminal/i))

    const keys = await sshKeys('192.168.1.15', 1940, 'root')
    await waitFor(() => expect(h.createSecret).toHaveBeenCalledTimes(1))
    expect(h.createSecret).toHaveBeenCalledWith({
      objectKey: keys.privateKey,
      displayName: 'Build Host SSH private key',
      kind: SecretKindSSHPrivateKey,
      contentType: SSHPrivateKeyContentType,
      value: new TextEncoder().encode(privateKey),
      readerPublicKeyPem: new TextEncoder().encode(
        '-----BEGIN PUBLIC KEY-----\nmock\n-----END PUBLIC KEY-----',
      ),
      participantRole: SOParticipantRole.SOParticipantRole_WRITER,
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
      privateKeySecretObjectKey: keys.privateKey,
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
    fireEvent.click(await enabledButton(/open terminal/i))

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
    fireEvent.click(await enabledButton(/open terminal/i))

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

  it('rejects direct Install Agent selection without side effects', () => {
    h.currentStep = 1
    h.configData = new TextEncoder().encode(
      JSON.stringify({
        mode: 'ssh',
        ssh: {
          host: 'install.example.com',
          port: 22,
          username: 'root',
          authMode: 'password',
          setupMode: 'host',
        },
      }),
    )
    renderViewer()

    const install = screen.getByRole('button', { name: 'Install Agent' })
    expect(install).toHaveProperty('disabled', true)
    fireEvent.click(install)

    expect(
      screen
        .getByRole('button', { name: 'Add SSH Host' })
        .getAttribute('aria-pressed'),
    ).toBe('true')
    expect(h.toastError).not.toHaveBeenCalled()
    expectNoInstallAgentSideEffects()
  })

  it.each(['spacewave', 'local'])(
    'rejects a restored Install Agent mode without %s side effects',
    (providerId) => {
      h.currentStep = 1
      h.providerId = providerId
      h.configData = new TextEncoder().encode(
        JSON.stringify({
          mode: 'ssh',
          ticket: 'persisted-ticket',
          completion: 'persisted-completion',
          ssh: {
            host: 'install.example.com',
            port: 22,
            username: 'root',
            authMode: 'password',
            setupMode: 'install-agent',
            setupStatus: {
              state: 'terminal_created',
              hostObjectKey: 'persisted-host',
              terminalObjectKey: 'persisted-terminal',
            },
          },
        }),
      )
      renderViewer()

      expect(screen.queryByText('Remote setup')).toBeNull()
      expect(screen.queryByText('Complete setup')).toBeNull()
      expect(
        screen.queryByPlaceholderText(/paste the base64 ticket/i),
      ).toBeNull()
      expect(screen.queryByRole('button', { name: /^approve$/i })).toBeNull()
      expect(
        screen.queryByRole('button', { name: 'Sign in or create account' }),
      ).toBeNull()
      const finalize = screen.getByRole('button', {
        name: 'Install Agent unavailable',
      })
      expect(finalize).toHaveProperty('disabled', true)
      fireEvent.click(finalize)

      expect(
        screen.queryByText(
          'Install Agent is not available until secure bootstrap is configured.',
        ),
      ).toBeNull()
      expect(h.toastError).not.toHaveBeenCalled()
      expectNoInstallAgentSideEffects()
    },
  )
})

// enabledButton waits for Finish to enable, which follows the computers read.
async function enabledButton(name: RegExp): Promise<HTMLElement> {
  const button = screen.getByRole('button', { name })
  await waitFor(() => expect(button).toHaveProperty('disabled', false))
  return button
}

// cancelWithRecordedConfig runs the step the viewer adds to the wizard's
// Cancel, with the config the last Finish press recorded, as the wizard hook
// runs it before deleting the wizard.
async function cancelWithRecordedConfig() {
  const configData = h.updateState.mock.calls
    .map(([update]) => (update as { configData?: Uint8Array }).configData)
    .filter((data) => data !== undefined)
    .at(-1)
  const spaceWorld = h.spaceWorld
  if (!configData || !spaceWorld) throw new Error('expected recorded config')
  await h.wizardOptions?.beforeCancel?.({
    objectKey: 'wizard/device-setup',
    spaceWorld,
    configData,
  })
}

function renderViewer() {
  return render(viewer())
}

function viewer() {
  return (
    <AddDeviceWizardViewer
      objectInfo={{}}
      worldState={{
        value: null,
        loading: false,
        error: null,
        retry: vi.fn(),
      }}
    />
  )
}

// failWizardRemoval lets the SSH Host and terminal batches commit and fails
// the next commit, the wizard removal.
function failWizardRemoval() {
  h.commit
    .mockResolvedValueOnce(undefined)
    .mockResolvedValueOnce(undefined)
    .mockRejectedValueOnce(new Error('commit failed'))
}

// sshKeys derives the keys the wizard writes for the Build Host label and
// the endpoint given.
function sshKeys(host = 'build.example.com', port = 2222, username = 'ubuntu') {
  return deriveSshHostKeys('wizard/device-setup', 'Build Host', {
    host,
    port,
    username,
  })
}

// sshConfigData is an SSH Host wizard's config for the host given.
function sshConfigData(host: string) {
  return new TextEncoder().encode(
    JSON.stringify({
      mode: 'ssh',
      ssh: {
        host,
        port: 2222,
        username: 'ubuntu',
        authMode: 'password',
        setupMode: 'host',
        hostKeyAlgorithm: 'ssh-ed25519',
        hostKeyFingerprint: 'SHA256:trusted-host',
      },
    }),
  )
}

// sshHostEndpoints lists the host of each SSH Host create op, in order.
function sshHostEndpoints() {
  return h.applyWorldOp.mock.calls
    .filter(([opId]) => opId === CREATE_SSH_HOST_OP_ID)
    .map(([, opData]) => {
      if (!(opData instanceof Uint8Array)) throw new Error('expected op bytes')
      return CreateSshHostOp.fromBinary(opData).endpoint?.host
    })
}

function expectNoInstallAgentSideEffects() {
  expect(h.applyWorldOp).not.toHaveBeenCalled()
  expect(h.createSecret).not.toHaveBeenCalled()
  expect(h.deleteObject).not.toHaveBeenCalled()
  expect(h.navigateToObjects).not.toHaveBeenCalled()
  expect(h.navigate).not.toHaveBeenCalled()
  expect(h.previewSpaceLink).not.toHaveBeenCalled()
  expect(h.approveSpaceLink).not.toHaveBeenCalled()
  expect(h.persistDraftState).not.toHaveBeenCalled()
  expect(h.handleConfigDataChange).not.toHaveBeenCalled()
  expect(h.updateState).not.toHaveBeenCalled()
  expect(h.setCreating).not.toHaveBeenCalled()
  expect(h.toastSuccess).not.toHaveBeenCalled()
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
