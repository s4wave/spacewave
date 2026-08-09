import { describe, expect, it, vi } from 'vitest'
import type { DragEvent as ReactDragEvent } from 'react'
import { APP_DRAG_MIME, APP_DRAG_VERSION } from '@s4wave/web/dnd/app-drag.js'
import { buildShellExternalDrag } from './shell-app-drag.js'

function createDragEvent(items: unknown[]) {
  const appDrag = JSON.stringify({
    version: APP_DRAG_VERSION,
    items,
  })
  return {
    dataTransfer: {
      types: [APP_DRAG_MIME],
      getData: (format: string) => (format === APP_DRAG_MIME ? appDrag : ''),
    },
  } as unknown as ReactDragEvent<HTMLElement>
}

function openableItem(id: string, label: string, path: string) {
  return {
    id,
    label,
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
                  path,
                },
              },
            },
            path: '',
            routePath: `/u/7/so/space-1/-/files/-${path}`,
          },
        },
      },
    ],
  }
}

describe('buildShellExternalDrag', () => {
  it('builds a shell tab drop adapter from an openable app drag', () => {
    const onAddTabs = vi.fn<Parameters<typeof buildShellExternalDrag>[1]>()
    const externalDrag = buildShellExternalDrag(
      createDragEvent([openableItem('report', 'report.md', '/docs/report.md')]),
      onAddTabs,
    )

    expect(externalDrag?.json).toMatchObject({
      type: 'tab',
      name: 'report.md',
      component: 'shell-content',
    })

    const droppedNode = { getId: () => 'shell-tab-1' }
    externalDrag?.onDrop(droppedNode as never)

    expect(onAddTabs).toHaveBeenCalledWith(
      [
        {
          id: 'shell-tab-1',
          name: 'report.md',
          path: '/u/7/so/space-1/-/files/-/docs/report.md',
        },
      ],
      droppedNode,
    )
  })

  it('preserves every openable item in selection order', () => {
    const onAddTabs = vi.fn<Parameters<typeof buildShellExternalDrag>[1]>()
    const externalDrag = buildShellExternalDrag(
      createDragEvent([
        openableItem('docs', 'docs', '/docs'),
        openableItem('report', 'report.md', '/docs/report.md'),
        openableItem('image', 'image.png', '/docs/image.png'),
      ]),
      onAddTabs,
    )

    externalDrag?.onDrop({ getId: () => 'dropped-tab' } as never)

    const addedTabs = onAddTabs.mock.calls[0]?.[0]
    expect(addedTabs?.map(({ id: _id, ...tab }) => tab)).toEqual([
      {
        name: 'docs',
        path: '/u/7/so/space-1/-/files/-/docs',
      },
      {
        name: 'report.md',
        path: '/u/7/so/space-1/-/files/-/docs/report.md',
      },
      {
        name: 'image.png',
        path: '/u/7/so/space-1/-/files/-/docs/image.png',
      },
    ])
    expect(addedTabs?.[0]?.id).toBe('dropped-tab')
    expect(addedTabs?.every((tab) => typeof tab.id === 'string')).toBe(true)
  })

  it('rejects app drags without a shell route hint', () => {
    const event = createDragEvent([
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
    ])

    expect(buildShellExternalDrag(event, vi.fn())).toBeUndefined()
  })
})
