import { describe, expect, it, vi } from 'vitest'
import type { DragEvent as ReactDragEvent } from 'react'
import { APP_DRAG_MIME, APP_DRAG_VERSION } from '@s4wave/web/dnd/app-drag.js'
import { buildShellExternalDrag } from './shell-app-drag.js'

describe('buildShellExternalDrag', () => {
  it('builds a shell tab drop adapter from an openable app drag', () => {
    const onAddTab = vi.fn()
    const appDrag = JSON.stringify({
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
                  path: '',
                  routePath: '/u/7/so/space-1/-/files/-/docs/report.md',
                },
              },
            },
          ],
        },
      ],
    })
    const event = {
      dataTransfer: {
        types: [APP_DRAG_MIME],
        getData: (format: string) => (format === APP_DRAG_MIME ? appDrag : ''),
      },
    } as unknown as ReactDragEvent<HTMLElement>

    const externalDrag = buildShellExternalDrag(event, onAddTab)

    expect(externalDrag?.json).toMatchObject({
      type: 'tab',
      name: 'report.md',
      component: 'shell-content',
    })

    externalDrag?.onDrop({
      getId: () => 'shell-tab-1',
    } as never)

    expect(onAddTab).toHaveBeenCalledWith({
      id: 'shell-tab-1',
      name: 'report.md',
      path: '/u/7/so/space-1/-/files/-/docs/report.md',
    })
  })

  it('rejects app drags without a shell route hint', () => {
    const event = {
      dataTransfer: {
        types: [APP_DRAG_MIME],
        getData: () =>
          JSON.stringify({
            version: APP_DRAG_VERSION,
            items: [
              {
                id: 'report',
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
                        path: '',
                      },
                    },
                  },
                ],
              },
            ],
          }),
      },
    } as unknown as ReactDragEvent<HTMLElement>

    expect(buildShellExternalDrag(event, vi.fn())).toBeUndefined()
  })
})
