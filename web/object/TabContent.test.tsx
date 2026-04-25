import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, cleanup } from '@testing-library/react'
import { TabContent } from './TabContent.js'
import { ObjectLayoutTab } from '@s4wave/sdk/layout/world/world.pb.js'
import { ObjectInfo } from './object.pb.js'

// Mock the SpaceContainerContext with a valid world state.
vi.mock('@s4wave/web/contexts/SpaceContainerContext.js', () => ({
  SpaceContainerContext: {
    useContextSafe: () => ({
      spaceWorldResource: { value: {} },
    }),
  },
}))

// Mock ObjectViewer to capture the props it receives.
vi.mock('./ObjectViewer.js', () => ({
  ObjectViewer: ({
    objectInfo,
    standalone,
    path,
    stateNamespace,
    bottomBarId,
  }: {
    objectInfo: ObjectInfo
    standalone?: boolean
    path?: string
    stateNamespace?: string[]
    bottomBarId?: string
  }) => (
    <div data-testid="object-viewer-mock">
      <span data-testid="info-case">{objectInfo?.info?.case ?? 'none'}</span>
      <span data-testid="standalone">{standalone ? 'true' : 'false'}</span>
      <span data-testid="path">{path ?? ''}</span>
      <span data-testid="bar-id">{bottomBarId ?? ''}</span>
      <span data-testid="namespace">{stateNamespace?.join('/') ?? ''}</span>
      {objectInfo?.info?.case === 'worldObjectInfo' && (
        <span data-testid="object-key">
          {(objectInfo.info.value as { objectKey?: string })?.objectKey ?? ''}
        </span>
      )}
      {objectInfo?.info?.case === 'unixfsObjectInfo' && (
        <span data-testid="unixfs-id">
          {(objectInfo.info.value as { unixfsId?: string })?.unixfsId ?? ''}
        </span>
      )}
    </div>
  ),
}))

describe('TabContent', () => {
  afterEach(() => {
    cleanup()
  })

  describe('delegates to ObjectViewer', () => {
    it('passes WorldObjectInfo to ObjectViewer in standalone mode', () => {
      const objectInfo: ObjectInfo = {
        info: {
          case: 'worldObjectInfo',
          value: {
            objectKey: 'files/getting-started.md',
            objectType: '',
          },
        },
      }

      const tabData = ObjectLayoutTab.toBinary({ objectInfo })
      const navigate = vi.fn()
      const addTab = vi.fn()

      const { getByTestId } = render(
        <TabContent
          tabID="test-tab"
          tabData={tabData}
          navigate={navigate}
          addTab={addTab}
        />,
      )

      expect(getByTestId('info-case').textContent).toBe('worldObjectInfo')
      expect(getByTestId('standalone').textContent).toBe('true')
      expect(getByTestId('object-key').textContent).toBe(
        'files/getting-started.md',
      )
      expect(getByTestId('bar-id').textContent).toBe('tab-test-tab')
      expect(getByTestId('namespace').textContent).toBe('tab/test-tab')
    })

    it('passes UnixfsObjectInfo to ObjectViewer', () => {
      const objectInfo: ObjectInfo = {
        info: {
          case: 'unixfsObjectInfo',
          value: {
            unixfsId: 'files',
            path: '/docs',
          },
        },
      }

      const tabData = ObjectLayoutTab.toBinary({ objectInfo })
      const navigate = vi.fn()
      const addTab = vi.fn()

      const { getByTestId } = render(
        <TabContent
          tabID="test-tab"
          tabData={tabData}
          navigate={navigate}
          addTab={addTab}
        />,
      )

      expect(getByTestId('info-case').textContent).toBe('unixfsObjectInfo')
      expect(getByTestId('standalone').textContent).toBe('true')
      expect(getByTestId('unixfs-id').textContent).toBe('files')
    })

    it('passes explicit path to ObjectViewer', () => {
      const objectInfo: ObjectInfo = {
        info: {
          case: 'worldObjectInfo',
          value: { objectKey: 'files', objectType: '' },
        },
      }

      const tabData = ObjectLayoutTab.toBinary({
        objectInfo,
        path: '/subdir',
      })
      const navigate = vi.fn()
      const addTab = vi.fn()

      const { getByTestId } = render(
        <TabContent
          tabID="test-tab"
          tabData={tabData}
          navigate={navigate}
          addTab={addTab}
        />,
      )

      expect(getByTestId('path').textContent).toBe('/subdir')
    })
  })

  describe('empty/invalid tab data', () => {
    it('renders empty tab message when tabData is undefined', () => {
      const navigate = vi.fn()
      const addTab = vi.fn()

      const { container } = render(
        <TabContent
          tabID="empty-tab"
          tabData={undefined}
          navigate={navigate}
          addTab={addTab}
        />,
      )

      expect(container.textContent?.includes('Empty tab: empty-tab')).toBe(true)
    })

    it('renders empty tab message when tabData is empty', () => {
      const navigate = vi.fn()
      const addTab = vi.fn()

      const { container } = render(
        <TabContent
          tabID="empty-tab"
          tabData={new Uint8Array(0)}
          navigate={navigate}
          addTab={addTab}
        />,
      )

      expect(container.textContent?.includes('Empty tab: empty-tab')).toBe(true)
    })
  })
})
