import { describe, expect, it, vi, afterEach } from 'vitest'
import type { DragEvent as ReactDragEvent } from 'react'
import { ObjectLayoutTab } from '@s4wave/sdk/layout/world/world.pb.js'
import { APP_DRAG_MIME, APP_DRAG_VERSION } from '@s4wave/web/dnd/app-drag.js'
import { buildObjectLayoutExternalDrag } from './layout-object-app-drag.js'

describe('buildObjectLayoutExternalDrag', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('builds config-backed object-layout tab JSON from an openable drag', () => {
    vi.spyOn(Date, 'now').mockReturnValue(12345)
    vi.spyOn(Math, 'random').mockReturnValue(0.123456789)

    const event = {
      dataTransfer: {
        types: [APP_DRAG_MIME],
        getData: () =>
          JSON.stringify({
            version: APP_DRAG_VERSION,
            items: [
              {
                id: 'report',
                label: 'report.md',
                capabilities: [
                  {
                    kind: 'openable',
                    value: {
                      case: 'object',
                      value: {
                        objectInfo: {
                          info: {
                            case: 'unixfsObjectInfo',
                            value: {
                              unixfsId: 'files',
                              path: '/docs/report.md',
                            },
                          },
                        },
                        path: '/preview',
                        componentId: 'details',
                      },
                    },
                  },
                ],
              },
            ],
          }),
      },
    } as unknown as ReactDragEvent<HTMLElement>

    const externalDrag = buildObjectLayoutExternalDrag(event)
    const json = externalDrag?.json

    expect(json).toMatchObject({
      type: 'tab',
      id: 'tab-12345-4fzzzxj',
      name: 'report.md',
      component: 'tab-content',
    })

    const layoutTab = ObjectLayoutTab.fromBinary(json?.config as Uint8Array)
    expect(layoutTab).toMatchObject({
      componentId: 'details',
      path: '/preview',
      objectInfo: {
        info: {
          case: 'unixfsObjectInfo',
          value: {
            unixfsId: 'files',
            path: '/docs/report.md',
          },
        },
      },
    })
  })

  it('rejects drags without an openable object payload', () => {
    const event = {
      dataTransfer: {
        types: [APP_DRAG_MIME],
        getData: () =>
          JSON.stringify({
            version: APP_DRAG_VERSION,
            items: [
              {
                id: 'folder',
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
            ],
          }),
      },
    } as unknown as ReactDragEvent<HTMLElement>

    expect(buildObjectLayoutExternalDrag(event)).toBeUndefined()
  })
})
