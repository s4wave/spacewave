import { describe, expect, it } from 'vitest'

import {
  DeviceCapabilityGrantState,
  DeviceCapabilityLocalState,
  DeviceCapabilityState,
  DeviceCheckoutRootAccess,
  DeviceLiveness,
  DeviceSetupState,
  type Device,
} from './device.pb.js'
import {
  DeviceCapabilityKindFilesystem,
  DeviceCapabilityKindForgeWorker,
  deviceCheckoutRootCanRead,
  deviceCheckoutRootCanWrite,
  findDeviceCapabilityByKind,
  findReadableDeviceCheckoutRoot,
  findSelectableDeviceCheckoutRoot,
  findSelectableDeviceForgeWorker,
  findWritableDeviceCheckoutRoot,
  hasSelectableDeviceCapabilityKind,
  isDeviceCapabilitySelectable,
  isDeviceSelectable,
} from './device.js'

describe('Device selection helpers', () => {
  it('accepts lima as a workflow-selectable Device target', () => {
    const device: Device = {
      peerId: '12D3KooWLima',
      label: 'lima',
      setupState: DeviceSetupState.DEVICE_SESSION_READY,
      lastStatus: {
        liveness: DeviceLiveness.ONLINE,
        message: 'device session ready',
      },
      capabilities: [
        {
          id: 'checkout-root-skiffos',
          kind: DeviceCapabilityKindFilesystem,
          label: 'SkiffOS checkout',
          state: DeviceCapabilityState.AVAILABLE,
          link: {
            objectKey: 'unixfs/skiffos-checkout',
            typeId: 'unixfs/fs-node',
          },
          checkoutRoot: {
            name: 'skiffos',
            displayPath: '~/repos/skiffos/skiffos',
            selectionRef: 'device/lima/filesystem/skiffos',
            access: DeviceCheckoutRootAccess.READ_WRITE,
            readAvailable: true,
            writeAvailable: true,
          },
          policy: {
            localPolicyRef: 'device/lima/filesystem/skiffos',
            grantPolicyRef: 'space/grants/lima/skiffos',
            localState: DeviceCapabilityLocalState.ENABLED,
            grantState: DeviceCapabilityGrantState.ALLOWED,
          },
        },
        {
          id: 'forge-worker',
          kind: DeviceCapabilityKindForgeWorker,
          label: 'Forge Worker',
          state: DeviceCapabilityState.AVAILABLE,
          link: {
            objectKey: 'forge/workers/lima',
            typeId: 'forge/worker',
          },
          policy: {
            localPolicyRef: 'device/lima/forge-worker',
            grantPolicyRef: 'space/grants/lima/forge-worker',
          },
        },
      ],
    }

    expect(isDeviceSelectable(device)).toBe(true)
    expect(
      hasSelectableDeviceCapabilityKind(device, DeviceCapabilityKindFilesystem),
    ).toBe(true)
    expect(
      hasSelectableDeviceCapabilityKind(
        device,
        DeviceCapabilityKindForgeWorker,
      ),
    ).toBe(true)
    expect(
      findDeviceCapabilityByKind(device, DeviceCapabilityKindFilesystem)?.link
        ?.objectKey,
    ).toBe('unixfs/skiffos-checkout')
    const checkoutRoot = findSelectableDeviceCheckoutRoot(
      device,
      'skiffos',
    )?.checkoutRoot
    expect(checkoutRoot?.selectionRef).toBe('device/lima/filesystem/skiffos')
    expect(deviceCheckoutRootCanRead(checkoutRoot)).toBe(true)
    expect(deviceCheckoutRootCanWrite(checkoutRoot)).toBe(true)
    expect(
      findReadableDeviceCheckoutRoot(device, 'skiffos')?.link?.typeId,
    ).toBe('unixfs/fs-node')
    expect(
      findWritableDeviceCheckoutRoot(device, 'skiffos')?.link?.objectKey,
    ).toBe('unixfs/skiffos-checkout')
    expect(findSelectableDeviceForgeWorker(device)?.link?.objectKey).toBe(
      'forge/workers/lima',
    )
  })

  it('does not select blocked or disabled capability summaries', () => {
    expect(
      isDeviceCapabilitySelectable({
        id: 'checkout-root-skiffos',
        kind: DeviceCapabilityKindFilesystem,
        label: 'SkiffOS checkout',
        state: DeviceCapabilityState.GRANT_BLOCKED,
      }),
    ).toBe(false)
    expect(
      isDeviceCapabilitySelectable({
        id: 'forge-worker',
        kind: DeviceCapabilityKindForgeWorker,
        label: 'Forge Worker',
        state: DeviceCapabilityState.DISABLED,
      }),
    ).toBe(false)
  })

  it('requires a filesystem owner link before a checkout root is readable', () => {
    const device: Device = {
      peerId: '12D3KooWLima',
      label: 'lima',
      setupState: DeviceSetupState.DEVICE_SESSION_READY,
      capabilities: [
        {
          id: 'checkout-root-skiffos',
          kind: DeviceCapabilityKindFilesystem,
          label: 'SkiffOS checkout',
          state: DeviceCapabilityState.AVAILABLE,
          checkoutRoot: {
            name: 'skiffos',
            access: DeviceCheckoutRootAccess.READ_ONLY,
            readAvailable: true,
          },
        },
      ],
    }

    expect(findSelectableDeviceCheckoutRoot(device, 'skiffos')).toBeTruthy()
    expect(findReadableDeviceCheckoutRoot(device, 'skiffos')).toBeUndefined()
  })

  it('rejects unavailable filesystem and forge-worker capabilities', () => {
    const device: Device = {
      peerId: '12D3KooWLima',
      label: 'lima',
      setupState: DeviceSetupState.DEVICE_SESSION_READY,
      capabilities: [
        {
          id: 'checkout-root-skiffos',
          kind: DeviceCapabilityKindFilesystem,
          label: 'SkiffOS checkout',
          state: DeviceCapabilityState.DISABLED,
          link: {
            objectKey: 'unixfs/skiffos-checkout',
            typeId: 'unixfs/fs-node',
          },
          checkoutRoot: {
            name: 'skiffos',
            access: DeviceCheckoutRootAccess.READ_ONLY,
            readAvailable: true,
          },
        },
        {
          id: 'forge-worker',
          kind: DeviceCapabilityKindForgeWorker,
          label: 'Forge Worker',
          state: DeviceCapabilityState.DECLARED,
          link: {
            objectKey: 'forge/workers/lima',
            typeId: 'forge/worker',
          },
        },
      ],
    }

    expect(findReadableDeviceCheckoutRoot(device, 'skiffos')).toBeUndefined()
    expect(findSelectableDeviceForgeWorker(device)).toBeUndefined()
  })

  it('skips blocked same-kind capabilities when checking availability', () => {
    const device: Device = {
      peerId: '12D3KooWLima',
      label: 'lima',
      setupState: DeviceSetupState.DEVICE_SESSION_READY,
      capabilities: [
        {
          id: 'checkout-root-blocked',
          kind: DeviceCapabilityKindFilesystem,
          label: 'Blocked checkout',
          state: DeviceCapabilityState.DISABLED,
        },
        {
          id: 'checkout-root-skiffos',
          kind: DeviceCapabilityKindFilesystem,
          label: 'SkiffOS checkout',
          state: DeviceCapabilityState.AVAILABLE,
        },
      ],
    }

    expect(
      hasSelectableDeviceCapabilityKind(device, DeviceCapabilityKindFilesystem),
    ).toBe(true)
  })

  it('requires explicit checkout-root access before treating availability as readable', () => {
    const device: Device = {
      peerId: '12D3KooWLima',
      label: 'lima',
      setupState: DeviceSetupState.DEVICE_SESSION_READY,
      capabilities: [
        {
          id: 'checkout-root-skiffos',
          kind: DeviceCapabilityKindFilesystem,
          label: 'SkiffOS checkout',
          state: DeviceCapabilityState.AVAILABLE,
          link: {
            objectKey: 'unixfs/skiffos-checkout',
            typeId: 'unixfs/fs-node',
          },
          checkoutRoot: {
            name: 'skiffos',
            readAvailable: true,
          },
        },
      ],
    }

    const checkoutRoot = findSelectableDeviceCheckoutRoot(
      device,
      'skiffos',
    )?.checkoutRoot
    expect(deviceCheckoutRootCanRead(checkoutRoot)).toBe(false)
    expect(findReadableDeviceCheckoutRoot(device, 'skiffos')).toBeUndefined()
  })

  it('requires allowed policy before treating a checkout root as writable', () => {
    const device: Device = {
      peerId: '12D3KooWLima',
      label: 'lima',
      setupState: DeviceSetupState.DEVICE_SESSION_READY,
      capabilities: [
        {
          id: 'checkout-root-skiffos',
          kind: DeviceCapabilityKindFilesystem,
          label: 'SkiffOS checkout',
          state: DeviceCapabilityState.AVAILABLE,
          link: {
            objectKey: 'unixfs/skiffos-checkout',
            typeId: 'unixfs/fs-node',
          },
          checkoutRoot: {
            name: 'skiffos',
            access: DeviceCheckoutRootAccess.READ_WRITE,
            readAvailable: true,
            writeAvailable: true,
          },
          policy: {
            localState: DeviceCapabilityLocalState.ENABLED,
            grantState: DeviceCapabilityGrantState.BLOCKED,
          },
        },
      ],
    }

    expect(findReadableDeviceCheckoutRoot(device, 'skiffos')).toBeTruthy()
    expect(findWritableDeviceCheckoutRoot(device, 'skiffos')).toBeUndefined()
  })

  it('skips blocked same-kind forge workers when checking availability', () => {
    const device: Device = {
      peerId: '12D3KooWLima',
      label: 'lima',
      setupState: DeviceSetupState.DEVICE_SESSION_READY,
      capabilities: [
        {
          id: 'forge-worker-blocked',
          kind: DeviceCapabilityKindForgeWorker,
          label: 'Blocked Forge Worker',
          state: DeviceCapabilityState.DISABLED,
          link: {
            objectKey: 'forge/workers/blocked',
            typeId: 'forge/worker',
          },
        },
        {
          id: 'forge-worker',
          kind: DeviceCapabilityKindForgeWorker,
          label: 'Forge Worker',
          state: DeviceCapabilityState.AVAILABLE,
          link: {
            objectKey: 'forge/workers/lima',
            typeId: 'forge/worker',
          },
        },
      ],
    }

    expect(findSelectableDeviceForgeWorker(device)?.link?.objectKey).toBe(
      'forge/workers/lima',
    )
  })

  it('does not throw on partially populated legacy capability records', () => {
    const device: Device = {
      peerId: '12D3KooWLima',
      label: 'lima',
      setupState: DeviceSetupState.DEVICE_SESSION_READY,
      capabilities: [
        {
          id: 'missing-kind',
          label: 'Missing kind',
          state: DeviceCapabilityState.AVAILABLE,
          link: {
            objectKey: 'unixfs/missing-kind',
            typeId: 'unixfs/fs-node',
          },
          checkoutRoot: {
            readAvailable: true,
            access: DeviceCheckoutRootAccess.READ_ONLY,
          },
        },
        {
          id: 'missing-name',
          kind: DeviceCapabilityKindFilesystem,
          label: 'Missing name',
          state: DeviceCapabilityState.AVAILABLE,
          link: {
            objectKey: 'unixfs/missing-name',
            typeId: 'unixfs/fs-node',
          },
          checkoutRoot: {
            access: DeviceCheckoutRootAccess.READ_ONLY,
            readAvailable: true,
          },
        },
      ],
    }

    expect(
      hasSelectableDeviceCapabilityKind(device, DeviceCapabilityKindFilesystem),
    ).toBe(true)
    expect(findSelectableDeviceCheckoutRoot(device, 'skiffos')).toBeUndefined()
    expect(findReadableDeviceCheckoutRoot(device, 'skiffos')).toBeUndefined()
    expect(findSelectableDeviceForgeWorker(device)).toBeUndefined()
  })

  it('trims identity and owner links before treating Device state as selectable', () => {
    expect(
      isDeviceSelectable({
        peerId: '   ',
        label: 'lima',
        setupState: DeviceSetupState.DEVICE_SESSION_READY,
      }),
    ).toBe(false)

    const whitespaceOwner: Device = {
      peerId: '12D3KooWLima',
      label: 'lima',
      setupState: DeviceSetupState.DEVICE_SESSION_READY,
      capabilities: [
        {
          id: 'checkout-root-skiffos',
          kind: ` ${DeviceCapabilityKindFilesystem} `,
          label: 'SkiffOS checkout',
          state: DeviceCapabilityState.AVAILABLE,
          link: {
            objectKey: '   ',
            typeId: 'unixfs/fs-node',
          },
          checkoutRoot: {
            name: ' skiffos ',
            access: DeviceCheckoutRootAccess.READ_ONLY,
            readAvailable: true,
          },
        },
        {
          id: 'forge-worker',
          kind: DeviceCapabilityKindForgeWorker,
          label: 'Forge Worker',
          state: DeviceCapabilityState.AVAILABLE,
          link: {
            objectKey: 'forge/workers/lima',
            typeId: '   ',
          },
        },
      ],
    }

    expect(
      findReadableDeviceCheckoutRoot(whitespaceOwner, 'skiffos'),
    ).toBeUndefined()
    expect(findSelectableDeviceForgeWorker(whitespaceOwner)).toBeUndefined()

    const trimmed: Device = {
      peerId: '12D3KooWLima',
      label: 'lima',
      setupState: DeviceSetupState.DEVICE_SESSION_READY,
      capabilities: [
        {
          id: 'checkout-root-skiffos',
          kind: ` ${DeviceCapabilityKindFilesystem} `,
          label: 'SkiffOS checkout',
          state: DeviceCapabilityState.AVAILABLE,
          link: {
            objectKey: 'unixfs/skiffos-checkout',
            typeId: 'unixfs/fs-node',
          },
          checkoutRoot: {
            name: ' skiffos ',
            access: DeviceCheckoutRootAccess.READ_ONLY,
            readAvailable: true,
          },
        },
      ],
    }
    expect(
      findReadableDeviceCheckoutRoot(trimmed, ' skiffos ')?.link?.objectKey,
    ).toBe('unixfs/skiffos-checkout')
  })
})
