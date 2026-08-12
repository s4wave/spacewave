import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import { MessageList } from './MessageList.js'

afterEach(cleanup)

describe('MessageList', () => {
  it('settles an empty channel with an actionable first-message state', () => {
    render(<MessageList messages={[]} />)
    expect(screen.getByText('Start the conversation')).toBeDefined()
    expect(screen.getByLabelText('Channel messages')).toBeDefined()
  })

  it('renders persisted sender identity, text, and time', () => {
    render(
      <MessageList
        messages={[
          {
            objectKey: 'message/1',
            senderPeerId: 'peer-123456789012345',
            text: 'hello',
            createdAt: new Date('2026-08-11T12:30:00Z'),
            replyToKey: '',
          },
        ]}
      />,
    )
    expect(screen.getByText('peer-12…12345')).toBeDefined()
    expect(screen.getByText('hello')).toBeDefined()
    expect(document.querySelector('time')?.textContent).toBeTruthy()
  })
})
