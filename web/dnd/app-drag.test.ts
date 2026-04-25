import { afterEach, describe, expect, it } from 'vitest'
import type {
  ObjectInfo,
  UnixfsObjectInfo,
} from '@s4wave/web/object/object.pb.js'
import {
  APP_DRAG_MIME,
  APP_DRAG_VERSION,
  clearActiveAppDragEnvelope,
  hasAppDragEnvelope,
  hasNativeFileDrag,
  isAppDragEnvelope,
  readAppDragEnvelope,
  readAppDragEnvelopeWithActiveFallback,
  type AppDragEnvelope,
  writeAppDragEnvelope,
} from './app-drag.js'

function buildUnixFSObjectInfo(path: string): ObjectInfo {
  return {
    info: {
      case: 'unixfsObjectInfo',
      value: {
        unixfsId: 'files',
        path,
      } satisfies UnixfsObjectInfo,
    },
  }
}

describe('app drag envelope', () => {
  afterEach(() => {
    clearActiveAppDragEnvelope()
  })

  it('supports openable and movable capabilities on one item', () => {
    const envelope = {
      version: APP_DRAG_VERSION,
      items: [
        {
          id: 'docs/report.md',
          label: 'report.md',
          capabilities: [
            {
              kind: 'openable',
              value: {
                case: 'object',
                value: {
                  objectInfo: buildUnixFSObjectInfo('/docs/report.md'),
                  path: '',
                },
              },
            },
            {
              kind: 'movable',
              value: {
                case: 'unixfs-entry',
                value: {
                  unixfsId: 'files',
                  path: '/docs/report.md',
                  isDir: false,
                },
              },
            },
          ],
        },
      ],
    } satisfies AppDragEnvelope

    expect(envelope.version).toBe(APP_DRAG_VERSION)
    expect(envelope.items).toHaveLength(1)
    expect(envelope.items[0].capabilities.map((cap) => cap.kind)).toEqual([
      'openable',
      'movable',
    ])
  })

  it('supports multi-item drags with per-item capability sets', () => {
    const envelope = {
      version: APP_DRAG_VERSION,
      items: [
        {
          id: 'docs',
          label: 'docs',
          capabilities: [
            {
              kind: 'movable',
              value: {
                case: 'unixfs-entry',
                value: {
                  unixfsId: 'files',
                  path: '/docs',
                  isDir: true,
                },
              },
            },
          ],
        },
        {
          id: 'notes/today.md',
          label: 'today.md',
          capabilities: [
            {
              kind: 'openable',
              value: {
                case: 'object',
                value: {
                  objectInfo: buildUnixFSObjectInfo('/notes/today.md'),
                  path: '',
                  componentId: 'markdown',
                },
              },
            },
          ],
        },
      ],
    } satisfies AppDragEnvelope

    expect(envelope.items).toHaveLength(2)
    expect(envelope.items[0].capabilities[0].kind).toBe('movable')
    expect(envelope.items[1].capabilities[0].kind).toBe('openable')
    expect(envelope.items[1].capabilities[0].value.case).toBe('object')
  })

  it('publishes and reads internal app drags through DataTransfer', () => {
    const writes = new Map<string, string>()
    const dataTransfer = {
      types: [APP_DRAG_MIME],
      getData: (format: string) => writes.get(format) ?? '',
      setData: (format: string, data: string) => {
        writes.set(format, data)
      },
    }

    const envelope = {
      version: APP_DRAG_VERSION,
      items: [
        {
          id: 'docs/report.md',
          capabilities: [
            {
              kind: 'openable',
              value: {
                case: 'object',
                value: {
                  objectInfo: buildUnixFSObjectInfo('/docs/report.md'),
                  path: '',
                },
              },
            },
          ],
        },
      ],
    } satisfies AppDragEnvelope

    writeAppDragEnvelope(dataTransfer, envelope)

    expect(hasAppDragEnvelope(dataTransfer)).toBe(true)
    expect(readAppDragEnvelope(dataTransfer)).toEqual(envelope)
  })

  it('falls back to the active in-memory envelope during dragover when getData is empty', () => {
    const writes = new Map<string, string>()
    const dragStartTransfer = {
      types: [APP_DRAG_MIME],
      getData: (format: string) => writes.get(format) ?? '',
      setData: (format: string, data: string) => {
        writes.set(format, data)
      },
    }
    const dragOverTransfer = {
      types: [APP_DRAG_MIME],
      getData: () => '',
    }
    const envelope = {
      version: APP_DRAG_VERSION,
      items: [
        {
          id: 'docs/report.md',
          capabilities: [
            {
              kind: 'openable',
              value: {
                case: 'object',
                value: {
                  objectInfo: buildUnixFSObjectInfo('/docs/report.md'),
                  path: '',
                },
              },
            },
          ],
        },
      ],
    } satisfies AppDragEnvelope

    writeAppDragEnvelope(dragStartTransfer, envelope)

    expect(readAppDragEnvelope(dragOverTransfer)).toBeNull()
    expect(readAppDragEnvelopeWithActiveFallback(dragOverTransfer)).toEqual(
      envelope,
    )
  })

  it('validates envelopes before accepting them', () => {
    expect(
      isAppDragEnvelope({
        version: APP_DRAG_VERSION,
        items: [{ id: 'ok', capabilities: [] }],
      }),
    ).toBe(true)

    expect(
      isAppDragEnvelope({
        version: APP_DRAG_VERSION,
        items: [{ id: 'bad', capabilities: [{ kind: 'openable' }] }],
      }),
    ).toBe(false)
    expect(
      readAppDragEnvelope({
        getData: () =>
          '{"version":1,"items":[{"id":"bad","capabilities":[{"kind":"openable"}]}]}',
      }),
    ).toBeNull()
  })

  it('distinguishes native file drags from internal app drags', () => {
    expect(
      hasNativeFileDrag({
        items: [{ kind: 'file' }],
        types: ['Files'],
      }),
    ).toBe(true)
    expect(
      hasNativeFileDrag({
        items: [{ kind: 'string' }],
        types: [APP_DRAG_MIME],
      }),
    ).toBe(false)
    expect(
      hasNativeFileDrag({
        types: ['Files'],
      }),
    ).toBe(true)
  })
})
