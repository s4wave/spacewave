import { describe, expect, it } from 'vitest'

import { V86WizardConfig_Source } from '@s4wave/sdk/vm/v86-wizard.pb.js'

import {
  buildV86QuickstartWizardConfig,
  buildV86QuickstartWizardKey,
  compareV86ImageNewestFirst,
  isDefaultV86Image,
  seedV86WizardConfig,
} from './v86-wizard-config.js'

describe('v86 wizard config', () => {
  it('builds the quickstart wizard config for CDN copy', () => {
    const cfg = buildV86QuickstartWizardConfig()

    expect(cfg.name).toBe('')
    expect(cfg.memoryMb).toBe(256)
    expect(cfg.vgaMemoryMb).toBe(8)
    expect(cfg.imageObjectKey).toBe('vm-image/default')
    expect(cfg.source).toBe(V86WizardConfig_Source.COPY_FROM_CDN)
    expect(cfg.cdnSourceObjectKey).toBe('')
    expect(cfg.cdnId).toBe('')
  })

  it('builds a deterministic quickstart wizard key from the timestamp', () => {
    const now = new Date('2026-04-20T04:54:00Z')
    expect(buildV86QuickstartWizardKey(now)).toBe(
      `wizard/v86-vm-${now.getTime().toString(36)}-1`,
    )
  })

  it('prefers the newest existing VM image when seeding an unspecified wizard', () => {
    const cfg = seedV86WizardConfig(
      {},
      { imageKey: 'vm-image/from-existing-vm' },
      [{ objectKey: 'vm-image/direct' }],
    )

    expect(cfg.source).toBe(V86WizardConfig_Source.EXISTING_IN_SPACE)
    expect(cfg.imageObjectKey).toBe('vm-image/from-existing-vm')
    expect(cfg.memoryMb).toBe(256)
    expect(cfg.vgaMemoryMb).toBe(8)
  })

  it('falls back to the newest in-space image before CDN copy', () => {
    const cfg = seedV86WizardConfig({}, undefined, [
      { objectKey: 'vm-image/direct' },
    ])

    expect(cfg.source).toBe(V86WizardConfig_Source.EXISTING_IN_SPACE)
    expect(cfg.imageObjectKey).toBe('vm-image/direct')
  })

  it('falls back to CDN copy when the space has no VM images', () => {
    const cfg = seedV86WizardConfig({}, undefined, [])

    expect(cfg.source).toBe(V86WizardConfig_Source.COPY_FROM_CDN)
    expect(cfg.imageObjectKey).toBe('vm-image/default')
  })

  it('keeps an already-selected source unchanged', () => {
    const cfg = seedV86WizardConfig(
      {
        source: V86WizardConfig_Source.COPY_FROM_CDN,
        imageObjectKey: 'vm-image/custom',
        memoryMb: 512,
      },
      { imageKey: 'vm-image/from-existing-vm' },
      [{ objectKey: 'vm-image/direct' }],
    )

    expect(cfg.source).toBe(V86WizardConfig_Source.COPY_FROM_CDN)
    expect(cfg.imageObjectKey).toBe('vm-image/custom')
    expect(cfg.memoryMb).toBe(512)
  })

  it('sorts V86Images newest-first with version and key tie-breakers', () => {
    const items = [
      {
        objectKey: 'vm-image/b',
        image: { createdAt: new Date('2026-04-19T00:00:00Z'), version: '1.0' },
      },
      {
        objectKey: 'vm-image/a',
        image: { createdAt: new Date('2026-04-20T00:00:00Z'), version: '1.0' },
      },
      {
        objectKey: 'vm-image/c',
        image: { createdAt: new Date('2026-04-20T00:00:00Z'), version: '2.0' },
      },
    ]

    items.sort(compareV86ImageNewestFirst)

    expect(items.map((item) => item.objectKey)).toEqual([
      'vm-image/c',
      'vm-image/a',
      'vm-image/b',
    ])
  })

  it('matches only default v86 V86Images', () => {
    expect(
      isDefaultV86Image({
        platform: 'v86',
        tags: ['default', 'stable'],
      } as never),
    ).toBe(true)
    expect(
      isDefaultV86Image({
        platform: 'wasi',
        tags: ['default'],
      } as never),
    ).toBe(false)
    expect(
      isDefaultV86Image({
        platform: 'v86',
        tags: ['stable'],
      } as never),
    ).toBe(false)
  })
})
