import { describe, it, expect } from 'vitest'
import { StreamConn, HandleStreamFunc } from 'starpc'
import { pushable } from 'it-pushable'
import { pipe } from 'it-pipe'

describe('Sync Yamux Processing', () => {
  it('should process yamux data synchronously with pushable', async () => {
    // This test verifies whether pushable + pipe can process data
    // when data is pushed synchronously in a tight loop

    const received: Uint8Array[] = []
    let streamHandlerCalled = false

    // Set up yamux connection (outbound direction like shw-quickjs.ts)
    const handleStreamFromPlugin: HandleStreamFunc = async (stream) => {
      streamHandlerCalled = true
      console.log('Stream handler called!')
      // Read from stream
      for await (const chunk of stream.source) {
        console.log('Received from stream:', chunk)
      }
    }

    const hostConn = new StreamConn(
      { handlePacketStream: handleStreamFromPlugin },
      {
        direction: 'outbound',
        yamuxParams: {
          enableKeepAlive: false,
          maxMessageSize: 32 * 1024,
        },
      },
    )

    // Set up the data flow like shw-quickjs.ts
    const devOutStream = pushable<Uint8Array>({ objectMode: true })
    const stdinData: Uint8Array[] = []

    // Start the pipe (this returns a promise that we'll await later)
    const pipePromise = pipe(devOutStream, hostConn, async (source) => {
      console.log('Pipe sink started')
      for await (const chunk of source) {
        const data = chunk instanceof Uint8Array ? chunk : new Uint8Array(chunk.subarray())
        console.log('Pipe sink received:', data.length, 'bytes')
        stdinData.push(data)
      }
      console.log('Pipe sink ended')
    }).catch((err) => {
      console.error('Pipe error:', err)
    })

    // Now simulate what happens in QuickJS:
    // Push some yamux data synchronously (like DevOut.fd_write does)
    
    // This is a valid yamux header: Version=0, Type=Ping, Flags=SYN, StreamID=0, Length=0
    const yamuxPing = new Uint8Array([0, 2, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0])
    console.log('Pushing yamux ping...')
    devOutStream.push(yamuxPing)

    // Push window update
    const yamuxWindowUpdate = new Uint8Array([0, 1, 0, 1, 0, 0, 0, 2, 0, 0, 0, 0])
    console.log('Pushing yamux window update...')
    devOutStream.push(yamuxWindowUpdate)

    // End the stream
    console.log('Ending devOutStream...')
    devOutStream.end()

    // Wait for the pipe to complete
    console.log('Waiting for pipe...')
    await pipePromise

    console.log('stdinData received:', stdinData.length, 'chunks')
    console.log('streamHandlerCalled:', streamHandlerCalled)

    // The ping should have been processed by yamux
    expect(stdinData.length).toBeGreaterThan(0)
  })

  it('should process yamux stream open', async () => {
    // Test that when the plugin opens a yamux stream (to make an RPC call),
    // the handleStreamFromPlugin callback is invoked

    let streamHandlerCalled = false
    let receivedStreamData: Uint8Array[] = []

    const handleStreamFromPlugin: HandleStreamFunc = async (stream) => {
      console.log('handleStreamFromPlugin called!')
      streamHandlerCalled = true
      
      for await (const chunk of stream.source) {
        const data = chunk instanceof Uint8Array ? chunk : new Uint8Array(chunk.subarray())
        receivedStreamData.push(data)
        console.log('Received stream data:', data.length, 'bytes')
      }
    }

    const hostConn = new StreamConn(
      { handlePacketStream: handleStreamFromPlugin },
      {
        direction: 'outbound',
        yamuxParams: {
          enableKeepAlive: false,
          maxMessageSize: 32 * 1024,
        },
      },
    )

    const devOutStream = pushable<Uint8Array>({ objectMode: true })
    const stdinData: Uint8Array[] = []

    const pipePromise = pipe(devOutStream, hostConn, async (source) => {
      for await (const chunk of source) {
        const data = chunk instanceof Uint8Array ? chunk : new Uint8Array(chunk.subarray())
        stdinData.push(data)
        console.log('stdin received:', data.length, 'bytes')
      }
    }).catch((err) => {
      console.error('Pipe error:', err)
    })

    // Simulate what the boot harness sends when opening a stream
    // The boot harness creates an 'inbound' StreamConn and opens a stream
    // When it opens stream, it sends yamux SYN + WindowUpdate
    
    // First, yamux header for SYN on stream 2 (outbound conn uses odd stream IDs but receives even from inbound)
    // Actually let's check what the real output looks like from the test above:
    // From rpc-debug test output:
    //   WindowUpdate: flags=1, streamId=2, length=0  -> 00 01 00 01 00 00 00 02 00 00 00 00
    //   Data: flags=0, streamId=2, length=47 -> 00 00 00 00 00 00 00 02 00 00 00 2f
    
    // The plugin (inbound conn) opens stream 2 with SYN (this is what we'd receive on outbound side)
    // Stream ID 2 is even, which is what an inbound conn would create
    
    // Let's push the exact sequence from the rpc-debug test:
    // Ping: 00 02 00 01 00 00 00 00 00 00 00 00
    devOutStream.push(new Uint8Array([0, 2, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0]))
    
    // WindowUpdate with SYN for stream 2: 00 01 00 01 00 00 00 02 00 00 00 00
    devOutStream.push(new Uint8Array([0, 1, 0, 1, 0, 0, 0, 2, 0, 0, 0, 0]))
    
    // Data header for stream 2, 47 bytes: 00 00 00 00 00 00 00 02 00 00 00 2f
    devOutStream.push(new Uint8Array([0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 47]))
    
    // Fake 47 bytes of data (the actual RPC payload)
    devOutStream.push(new Uint8Array(47).fill(0x41))
    
    // Give the async pipe time to process
    await new Promise(resolve => setTimeout(resolve, 100))
    
    devOutStream.end()
    await pipePromise

    console.log('streamHandlerCalled:', streamHandlerCalled)
    console.log('receivedStreamData:', receivedStreamData.length, 'chunks')
    console.log('stdinData:', stdinData.length, 'chunks')

    // The stream handler should have been called when yamux received the stream open
    expect(streamHandlerCalled).toBe(true)
    expect(receivedStreamData.length).toBeGreaterThan(0)
  })
})
