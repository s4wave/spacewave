import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import {
  ChatLandingDemo,
  DevicesLandingDemo,
  DriveLandingDemo,
  NotesLandingDemo,
  PluginsLandingDemo,
} from './LandingDemos.js'

describe('landing demos', () => {
  afterEach(() => {
    cleanup()
  })

  it('lets the drive demo navigate into folders and update the preview', () => {
    render(<DriveLandingDemo />)

    expect(screen.getByText('Studio laptop')).toBeTruthy()
    expect(screen.getByText('Offline edits saved locally')).toBeTruthy()
    expect(screen.getByText('Sync pending')).toBeTruthy()
    expect(screen.getByText('Optional reach enabled')).toBeTruthy()
    expect(screen.getByText('offline-ready')).toBeTruthy()

    fireEvent.doubleClick(screen.getByText('docs'))

    expect(screen.getByText('/docs')).toBeTruthy()
    expect(screen.getByText('roadmap.md')).toBeTruthy()
  })

  it('lets the devices demo switch the selected device surface', () => {
    render(<DevicesLandingDemo />)

    fireEvent.click(screen.getByText('rack-node'))

    expect(screen.getByText('relay handshake established')).toBeTruthy()
  })

  it('lets the chat demo send a local message through the shared chat widgets', async () => {
    render(<ChatLandingDemo />)

    const input = screen.getByPlaceholderText('Type a message...')
    fireEvent.change(input, { target: { value: 'Ship the docs update next.' } })
    fireEvent.keyDown(input, { key: 'Enter' })

    expect(await screen.findByText('Ship the docs update next.')).toBeTruthy()
    expect(
      await screen.findByText(
        'Shared docs updated. Every device picked up the new markdown blocks.',
      ),
    ).toBeTruthy()
  })

  it('switches the notes markdown preview to the selected note', () => {
    render(<NotesLandingDemo />)

    fireEvent.click(screen.getByText('Team handbook'))

    const editor = screen.getByRole('textbox', { name: 'Notebook markdown' })
    expect(screen.getAllByText('Team handbook').length).toBeGreaterThan(0)
    expect((editor as HTMLTextAreaElement).value).toContain(
      'Spacewave notes are plain markdown.',
    )
  })

  it('derives the plugin preview from the current code', () => {
    render(<PluginsLandingDemo />)

    const textarea = screen.getByRole('textbox', { name: 'Plugin source' })
    fireEvent.change(textarea, {
      target: {
        value: `export default {
  name: 'sync-lens',
  command: 'sync:inspect',
  description: 'Inspect replication state.',
}
`,
      },
    })

    expect(screen.getByText('sync-lens')).toBeTruthy()
    expect(screen.getAllByText(/sync:inspect/).length).toBeGreaterThan(0)
  })
})
