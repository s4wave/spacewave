import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { ChatChannelViewer } from './ChatChannelViewer.js'

const retry = vi.fn()
let streamState: {
  value: never[] | null
  loading: boolean
  error: Error | null
  retry: () => void
}

vi.mock('@s4wave/web/hooks/useAccessTypedHandle.js', () => ({
  useAccessTypedHandle: () => ({
    value: { watchMessages: vi.fn(), sendMessage: vi.fn() },
    loading: false,
    error: null,
    retry: vi.fn(),
  }),
}))

vi.mock('@aptre/bldr-sdk/hooks/useStreamingResource.js', () => ({
  useStreamingResource: () => streamState,
}))

const objectInfo = {
  info: {
    case: 'worldObjectInfo' as const,
    value: { objectKey: 'chat/general', objectType: 'spacewave-chat/channel' },
  },
}

const worldState = { value: null, loading: false, error: null, retry: vi.fn() }

afterEach(() => {
  cleanup()
  retry.mockClear()
})

describe('ChatChannelViewer', () => {
  it('settles an empty initial snapshot', () => {
    streamState = { value: [], loading: false, error: null, retry }
    render(
      <ChatChannelViewer
        objectInfo={objectInfo}
        worldState={worldState as never}
      />,
    )
    expect(screen.getByText('Start the conversation')).toBeDefined()
    expect(screen.queryByText('Loading messages')).toBeNull()
  })

  it('does not carry a populated channel into a newly selected empty channel', async () => {
    streamState = {
      value: [
        {
          objectKey: 'chat/a/message/0',
          senderPeerId: 'peer-a',
          text: 'channel A message',
          createdAt: new Date(),
          replyToKey: '',
        },
      ] as never[],
      loading: false,
      error: null,
      retry,
    }
    const { rerender } = render(
      <ChatChannelViewer
        objectInfo={objectInfo}
        worldState={worldState as never}
      />,
    )
    expect(await screen.findByText('channel A message')).toBeDefined()

    streamState = { value: [] as never[], loading: false, error: null, retry }
    rerender(
      <ChatChannelViewer
        objectInfo={{
          info: {
            case: 'worldObjectInfo',
            value: {
              objectKey: 'chat/empty',
              objectType: 'spacewave-chat/channel',
            },
          },
        }}
        worldState={worldState as never}
      />,
    )

    await waitFor(() =>
      expect(screen.queryByText('channel A message')).toBeNull(),
    )
    expect(screen.getByText('Start the conversation')).toBeDefined()
  })

  it('offers retry when channel history fails', () => {
    streamState = {
      value: null,
      loading: false,
      error: new Error('offline'),
      retry,
    }
    render(
      <ChatChannelViewer
        objectInfo={objectInfo}
        worldState={worldState as never}
      />,
    )
    fireEvent.click(screen.getByRole('button', { name: 'Try again' }))
    expect(retry).toHaveBeenCalledOnce()
  })
})
