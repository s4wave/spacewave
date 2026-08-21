import { describe, expect, it, vi } from 'vitest'

import { DataChannelWrapper } from './webrtc-bridge.js'

function waitForIceGatheringComplete(peer: RTCPeerConnection): Promise<void> {
  if (peer.iceGatheringState === 'complete') return Promise.resolve()
  return new Promise((resolve) => {
    peer.onicegatheringstatechange = () => {
      if (peer.iceGatheringState === 'complete') resolve()
    }
  })
}

describe('DataChannelWrapper browser lifecycle', () => {
  it('does not send after a remote data channel closes', async () => {
    const local = new RTCPeerConnection()
    const remote = new RTCPeerConnection()
    let localChannel: RTCDataChannel | undefined
    let remoteChannel: RTCDataChannel | undefined

    try {
      const remoteChannelReady = new Promise<RTCDataChannel>((resolve) => {
        remote.ondatachannel = (event) => {
          resolve(event.channel)
        }
      })
      localChannel = local.createDataChannel('teardown')
      const localOpen = new Promise<void>((resolve) => {
        localChannel!.onopen = () => resolve()
      })

      const localCandidates: RTCIceCandidateInit[] = []
      const remoteCandidates: RTCIceCandidateInit[] = []
      local.onicecandidate = ({ candidate }) => {
        if (candidate) localCandidates.push(candidate.toJSON())
      }
      remote.onicecandidate = ({ candidate }) => {
        if (candidate) remoteCandidates.push(candidate.toJSON())
      }

      const offer = await local.createOffer()
      await local.setLocalDescription(offer)
      await waitForIceGatheringComplete(local)
      await remote.setRemoteDescription(offer)
      await Promise.all(
        localCandidates.map((candidate) => remote.addIceCandidate(candidate)),
      )
      const answer = await remote.createAnswer()
      await remote.setLocalDescription(answer)
      await waitForIceGatheringComplete(remote)
      await local.setRemoteDescription(answer)
      await Promise.all(
        remoteCandidates.map((candidate) => local.addIceCandidate(candidate)),
      )

      remoteChannel = await remoteChannelReady
      await localOpen

      const wrapper = new DataChannelWrapper(localChannel.label)
      wrapper.attach(localChannel)
      const nativeSend = vi.spyOn(localChannel, 'send')
      const closed = new Promise<void>((resolve) => {
        wrapper.onclose = () => resolve()
      })

      remoteChannel.close()
      await closed

      expect(() => wrapper.send('after-close')).toThrow(
        expect.objectContaining({ name: 'InvalidStateError' }),
      )
      expect(nativeSend).not.toHaveBeenCalled()
    } finally {
      localChannel?.close()
      remoteChannel?.close()
      local.close()
      remote.close()
    }
  }, 10_000)
})
