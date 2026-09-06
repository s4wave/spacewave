import { afterEach, describe, expect, it, vi } from 'vitest'

import { EnrollManagedAccountRequest } from '@s4wave/core/provider/spacewave/api/application.pb.js'
import { SigningPayload } from '@s4wave/core/provider/spacewave/api/api.pb.js'

import {
  ApplicationOperatorClient,
  type ApplicationOperatorClientOptions,
} from './application-client.js'

interface CapturedRequest {
  url: string
  init: RequestInit
  body: Uint8Array
}

// makeKeypair generates a real Ed25519 keypair for signature verification.
async function makeKeypair(): Promise<{
  privateKey: CryptoKey
  publicKey: CryptoKey
  rawPublic: Uint8Array
}> {
  const pair = await crypto.subtle.generateKey('Ed25519', true, [
    'sign',
    'verify',
  ])
  const rawPublic = new Uint8Array(
    await crypto.subtle.exportKey('raw', pair.publicKey),
  )
  return { privateKey: pair.privateKey, publicKey: pair.publicKey, rawPublic }
}

// makeClient builds a client whose fetch records the request and replays the
// given response.
function makeClient(
  keypair: Awaited<ReturnType<typeof makeKeypair>>,
  respond: (body: Uint8Array) => Response | Promise<Response>,
): { client: ApplicationOperatorClient; captured: CapturedRequest[] } {
  const captured: CapturedRequest[] = []
  const fetchImpl = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const signal = init?.signal
      if (signal?.aborted) {
        throw new DOMException('The operation was aborted.', 'AbortError')
      }
      const body = init!.body as Uint8Array
      captured.push({
        url: String(input),
        init: init!,
        body,
      })
      return respond(body)
    },
  )
  const options: ApplicationOperatorClientOptions = {
    apiOrigin: 'https://api.example.com',
    signingEnvPrefix: 'spacewave',
    sessionPeerId: '12D3KooWTest',
    sign: async (payload) =>
      new Uint8Array(
        await crypto.subtle.sign(
          'Ed25519',
          keypair.privateKey,
          payload.slice(),
        ),
      ),
    fetch: fetchImpl as unknown as typeof globalThis.fetch,
  }
  return { client: new ApplicationOperatorClient(options), captured }
}

describe('ApplicationOperatorClient', () => {
  const originalFetch = globalThis.fetch

  afterEach(() => {
    globalThis.fetch = originalFetch
    vi.restoreAllMocks()
  })

  it('signs and sends GetApplication with a verifiable Ed25519 signature', async () => {
    const keypair = await makeKeypair()
    // GetApplicationResponse{application: Application{id:"123"}}
    const responseBody = new Uint8Array([
      0x0a, 0x05, 0x0a, 0x03, 0x31, 0x32, 0x33,
    ])
    const { client, captured } = makeClient(
      keypair,
      () => new Response(responseBody, { status: 200 }),
    )

    const response = await client.getApplication('app-123')

    expect(captured).toHaveLength(1)
    const req = captured[0]
    expect(req.url).toBe('https://api.example.com/api/applications/get')
    expect(req.init.method).toBe('POST')
    expect(req.init.redirect).toBe('manual')

    const headers = new Headers(req.init.headers as HeadersInit)
    expect(headers.get('Content-Type')).toBe('application/octet-stream')
    expect(headers.get('Accept')).toBe('application/octet-stream')
    expect(headers.get('X-Peer-ID')).toBe('12D3KooWTest')
    // Go sets the header to the bare sorted key list (strings.Join(keys, ",")),
    // while the payload carries "key=value" pairs.
    expect(headers.get('X-Signed-Headers')).toBe('content-type')

    // Reconstruct the signed payload from the request itself, exactly like the
    // Go server does, and verify the Ed25519 signature with the real public key.
    const payload = {
      envPrefix: 'spacewave',
      method: 'POST',
      path: '/api/applications/get',
      timestampMs: BigInt(headers.get('X-Timestamp') ?? '0'),
      contentLength: BigInt(req.body.length),
      bodyHashHex: headers.get('X-Sw-Hash') ?? '',
      signedHeaders: `content-type=${headers.get('Content-Type')}`,
    }
    // The payload's signed_headers must be "key=value" pairs, independent of
    // the X-Signed-Headers header format.
    const payloadBytes = SigningPayload.toBinary(payload)
    const signature = Uint8Array.from(
      atob(headers.get('X-Signature') ?? ''),
      (c) => c.charCodeAt(0),
    )
    const valid = await crypto.subtle.verify(
      'Ed25519',
      keypair.publicKey,
      signature.slice(),
      payloadBytes.slice(),
    )
    expect(valid).toBe(true)

    // The decoded response must round-trip through the generated codec.
    expect(response.application?.id).toBe('123')
  })

  it('encodes EnrollManagedAccountRequest identically to the Go MarshalVT contract', async () => {
    const keypair = await makeKeypair()
    const { client, captured } = makeClient(
      keypair,
      () => new Response(new Uint8Array(0), { status: 200 }),
    )

    const request = EnrollManagedAccountRequest.create({
      enrollment: {
        applicationId: 'app-123',
        issuer: 'https://id.example.com',
        subject: 'user-42',
        keypairPeerId: '12D3KooWGOLDEN',
        expectedApplicationRevision: BigInt(7),
      },
      signature: Uint8Array.from([1, 2, 3, 4, 5]),
    })
    await client.enrollManagedAccount(request)

    const goHex =
      '0a3c0a076170702d313233121668747470733a2f2f69642e6578616d706c652e636f6d1a07757365722d3432220e313244334b6f6f57474f4c44454e280712050102030405'
    const expected = new Uint8Array(
      goHex.match(/.{2}/g)!.map((h) => parseInt(h, 16)),
    )
    expect(captured[0].body).toEqual(expected)
  })

  it('propagates cancellation from the AbortSignal', async () => {
    const keypair = await makeKeypair()
    const controller = new AbortController()
    const { client } = makeClient(keypair, () => {
      throw new Error('should not reach response')
    })

    const promise = client.getApplication('app-123', controller.signal)
    controller.abort()
    await expect(promise).rejects.toThrow()
  })

  it('accepts http loopback origins for local development', () => {
    const client = new ApplicationOperatorClient({
      apiOrigin: 'http://127.0.0.1:8787',
      signingEnvPrefix: 'spacewave',
      sessionPeerId: '12D3KooWTest',
      sign: async () => new Uint8Array(64),
    })
    expect(client).toBeInstanceOf(ApplicationOperatorClient)
  })

  it('rejects apiOrigin with path, query, credentials, or plain http', () => {
    const base: ApplicationOperatorClientOptions = {
      apiOrigin: 'https://api.example.com',
      signingEnvPrefix: 'spacewave',
      sessionPeerId: '12D3KooWTest',
      sign: async () => new Uint8Array(64),
    }
    for (const bad of [
      'https://api.example.com/base/path',
      'https://api.example.com/?x=1',
      'https://user:pass@api.example.com',
      'ftp://api.example.com',
      'http://api.example.com',
      'not a url',
    ]) {
      expect(
        () => new ApplicationOperatorClient({ ...base, apiOrigin: bad }),
      ).toThrow()
    }
  })

  it('rejects a redirect response instead of following it', async () => {
    const keypair = await makeKeypair()
    const { client, captured } = makeClient(
      keypair,
      () =>
        new Response(null, {
          status: 302,
          headers: {
            Location: 'https://evil.example.com/api/applications/get',
          },
        }),
    )

    await expect(client.getApplication('app-123')).rejects.toMatchObject({
      statusCode: 302,
    })
    // Only the original request must have been sent; the redirect was not followed.
    expect(captured).toHaveLength(1)
  })

  it('rejects non-2xx responses with the parsed cloud error', async () => {
    const keypair = await makeKeypair()
    const errorBody = new TextEncoder().encode(
      JSON.stringify({
        code: 'not_authorized',
        message: 'nope',
        retryable: false,
      }),
    )
    const { client } = makeClient(
      keypair,
      () => new Response(errorBody, { status: 403 }),
    )

    await expect(client.getApplication('app-123')).rejects.toMatchObject({
      statusCode: 403,
      code: 'not_authorized',
    })
  })
})
