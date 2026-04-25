import { describe, expect, it } from 'vitest'

import {
  getMimeType,
  isAudioMimeType,
  isVideoMimeType,
} from './useUnixFSHandle.js'

describe('UnixFS MIME helpers', () => {
  it.each([
    ['song.mp3', 'audio/mpeg'],
    ['song.m4a', 'audio/mp4'],
    ['song.wav', 'audio/wav'],
    ['song.ogg', 'audio/ogg'],
    ['song.opus', 'audio/ogg'],
    ['song.oga', 'audio/ogg'],
    ['song.flac', 'audio/flac'],
    ['audiobook.m4b', 'audio/mp4'],
    ['ringtone.m4r', 'audio/mp4'],
  ])('classifies %s as audio', (filename, expected) => {
    const mimeType = getMimeType(filename)

    expect(mimeType).toBe(expected)
    expect(isAudioMimeType(mimeType)).toBe(true)
  })

  it('keeps webm extension routing as video media', () => {
    const mimeType = getMimeType('demo.webm')

    expect(mimeType).toBe('video/webm')
    expect(isVideoMimeType(mimeType)).toBe(true)
  })
})
