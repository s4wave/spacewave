import { describe, expect, it } from 'vitest'
import {
  buildUnixFSEntryAppDragEnvelope,
  buildUnixFSSelectionAppDragEnvelope,
  readUnixFSMovableAppDragItem,
  readUnixFSMovableAppDragItems,
} from './unixfs-app-drag.js'

describe('buildUnixFSEntryAppDragEnvelope', () => {
  it('builds an openable UnixFS app-drag with a shell route hint', () => {
    const envelope = buildUnixFSEntryAppDragEnvelope({
      entry: { id: 'report', name: 'report.md', isDir: false },
      currentPath: '/docs',
      sessionIndex: 7,
      spaceId: 'space-1',
      unixfsId: 'files',
    })

    expect(envelope).not.toBeNull()
    expect(envelope?.items[0].label).toBe('report.md')
    expect(envelope?.items[0].capabilities[0]).toMatchObject({
      kind: 'openable',
      value: {
        case: 'object',
        value: {
          path: '',
          routePath: '/u/7/so/space-1/-/files/-/docs/report.md',
          objectInfo: {
            info: {
              case: 'unixfsObjectInfo',
              value: {
                unixfsId: 'files',
                path: '/docs/report.md',
              },
            },
          },
        },
      },
    })
    expect(envelope?.items[0].capabilities[1]).toMatchObject({
      kind: 'movable',
      value: {
        case: 'unixfs-entry',
        value: {
          unixfsId: 'files',
          path: '/docs/report.md',
          isDir: false,
        },
      },
    })
  })

  it('builds a movable-only drag without shell routing context', () => {
    const envelope = buildUnixFSEntryAppDragEnvelope({
      entry: { id: 'report', name: 'report.md', isDir: false },
      currentPath: '/docs',
      sessionIndex: null,
      spaceId: 'space-1',
      unixfsId: 'files',
    })

    expect(envelope).not.toBeNull()
    expect(envelope?.items[0].capabilities).toHaveLength(1)
    expect(envelope?.items[0].capabilities[0]).toMatchObject({
      kind: 'movable',
      value: {
        case: 'unixfs-entry',
        value: {
          unixfsId: 'files',
          path: '/docs/report.md',
          isDir: false,
        },
      },
    })
  })

  it('reads the movable UnixFS drag item back from dataTransfer', () => {
    const envelope = buildUnixFSEntryAppDragEnvelope({
      entry: { id: 'docs', name: 'docs', isDir: true },
      currentPath: '/',
      sessionIndex: null,
      spaceId: null,
      unixfsId: 'files',
    })
    const json = JSON.stringify(envelope)
    const dataTransfer = {
      types: ['application/x-s4wave-app-drag+json'],
      getData: (format: string) =>
        format === 'application/x-s4wave-app-drag+json' ? json : '',
    }

    expect(readUnixFSMovableAppDragItem(dataTransfer)).toEqual({
      id: 'docs',
      label: 'docs',
      value: {
        unixfsId: 'files',
        path: '/docs',
        isDir: true,
      },
    })
  })

  it('builds a multi-item selection drag with openable items and one movable dragged row', () => {
    const envelope = buildUnixFSSelectionAppDragEnvelope({
      entries: [
        { id: 'docs', name: 'docs', isDir: true },
        { id: 'report', name: 'report.md', isDir: false },
      ],
      currentPath: '/',
      sessionIndex: 7,
      spaceId: 'space-1',
      unixfsId: 'files',
      movableEntryIds: ['docs', 'report'],
    })

    expect(envelope?.items).toHaveLength(2)
    expect(
      envelope?.items.map((item) => ({
        id: item.id,
        capabilities: item.capabilities.map((cap) => cap.kind),
      })),
    ).toEqual([
      { id: 'docs', capabilities: ['openable', 'movable'] },
      { id: 'report', capabilities: ['openable', 'movable'] },
    ])
  })

  it('reads every movable UnixFS drag item back from dataTransfer', () => {
    const envelope = buildUnixFSSelectionAppDragEnvelope({
      entries: [
        { id: 'docs', name: 'docs', isDir: true },
        { id: 'report', name: 'report.md', isDir: false },
      ],
      currentPath: '/',
      sessionIndex: null,
      spaceId: null,
      unixfsId: 'files',
      movableEntryIds: ['docs', 'report'],
    })
    const json = JSON.stringify(envelope)
    const dataTransfer = {
      types: ['application/x-s4wave-app-drag+json'],
      getData: (format: string) =>
        format === 'application/x-s4wave-app-drag+json' ? json : '',
    }

    expect(readUnixFSMovableAppDragItems(dataTransfer)).toEqual([
      {
        id: 'docs',
        label: 'docs',
        value: {
          unixfsId: 'files',
          path: '/docs',
          isDir: true,
        },
      },
      {
        id: 'report',
        label: 'report.md',
        value: {
          unixfsId: 'files',
          path: '/report.md',
          isDir: false,
        },
      },
    ])
    expect(readUnixFSMovableAppDragItem(dataTransfer)).toEqual({
      id: 'docs',
      label: 'docs',
      value: {
        unixfsId: 'files',
        path: '/docs',
        isDir: true,
      },
    })
  })
})
