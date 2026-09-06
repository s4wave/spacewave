import type { Message, MessageType } from '@aptre/protobuf-es-lite/message'

import {
  EnrollManagedAccountRequest,
  EnrollManagedAccountResponse,
  GetApplicationRequest,
  GetApplicationResponse,
} from '@s4wave/core/provider/spacewave/api/application.pb.js'
import {
  ErrorResponse,
  SigningPayload,
} from '@s4wave/core/provider/spacewave/api/api.pb.js'

// maxResponseBodySize is the maximum HTTP response body size (10 MiB).
const maxResponseBodySize = 10 * 1024 * 1024

// CloudError is a structured error returned by the Spacewave cloud API.
// Message text comes from the server's ErrorResponse and never contains
// local credential material.
export class CloudError extends Error {
  public readonly statusCode: number
  public readonly code?: string
  public readonly retryable?: boolean
  public readonly retryAfterSeconds?: number

  public constructor(
    statusCode: number,
    code?: string,
    message?: string,
    retryable?: boolean,
    retryAfterSeconds?: number,
  ) {
    super(`${statusCode} ${code ?? 'unknown'}: ${message ?? 'request failed'}`)
    this.name = 'CloudError'
    this.statusCode = statusCode
    this.code = code
    this.retryable = retryable
    this.retryAfterSeconds = retryAfterSeconds
  }
}

// SigningFn signs the exact binary payload; key custody remains with the caller.
export type SigningFn = (
  payload: Uint8Array,
  signal?: AbortSignal,
) => Promise<Uint8Array>

// ApplicationOperatorClientOptions configures the ApplicationOperatorClient.
export interface ApplicationOperatorClientOptions {
  // apiOrigin is the cloud API origin, e.g. "https://api.spacewave.dev".
  apiOrigin: string
  // signingEnvPrefix is the environment prefix carried in signed payloads.
  signingEnvPrefix: string
  // sessionPeerId is the base58 peer ID of the operator session, sent as X-Peer-ID.
  sessionPeerId: string
  // sign signs the serialized SigningPayload; keys are never handed to this client.
  sign: SigningFn
  // fetch overrides the global fetch implementation (for tests).
  fetch?: typeof globalThis.fetch
}

// bytesToHex renders bytes as lowercase hex.
function bytesToHex(bytes: Uint8Array): string {
  return Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('')
}

// sha256Hex hashes the request body and hex-encodes the digest.
async function sha256Hex(body: Uint8Array): Promise<string> {
  const digest = await crypto.subtle.digest('SHA-256', body.slice().buffer)
  return bytesToHex(new Uint8Array(digest))
}

// uint8ArrayToBase64 encodes bytes using standard base64.
function uint8ArrayToBase64(bytes: Uint8Array): string {
  let binary = ''
  for (const b of bytes) binary += String.fromCharCode(b)
  return btoa(binary)
}

// parseCloudError decodes a JSON ErrorResponse body into a CloudError.
// Malformed bodies degrade to a status-only error rather than failing the call.
function parseCloudError(statusCode: number, body: Uint8Array): CloudError {
  try {
    const resp = ErrorResponse.fromJsonString(new TextDecoder().decode(body))
    return new CloudError(
      statusCode,
      resp.code,
      resp.message,
      resp.retryable,
      resp.retryAfterSeconds,
    )
  } catch {
    return new CloudError(statusCode)
  }
}

// readBoundedBody reads the response body with a size limit.
// Returns an error if the body exceeds maxResponseBodySize.
async function readBoundedBody(response: Response): Promise<Uint8Array> {
  const reader = response.body?.getReader()
  if (!reader) return new Uint8Array(0)
  const chunks: Uint8Array[] = []
  let total = 0
  try {
    for (;;) {
      const { done, value } = await reader.read()
      if (done) break
      total += value.length
      if (total > maxResponseBodySize) {
        await reader.cancel()
        throw new Error('response body exceeds maximum size')
      }
      chunks.push(value)
    }
  } finally {
    reader.releaseLock()
  }
  const out = new Uint8Array(total)
  let offset = 0
  for (const chunk of chunks) {
    out.set(chunk, offset)
    offset += chunk.length
  }
  return out
}

// validateOrigin enforces an absolute origin: https, or http on loopback for
// local development. No credentials, path, query, or fragment.
function validateOrigin(value: string): string {
  let url: URL
  try {
    url = new URL(value)
  } catch {
    throw new Error('apiOrigin is not an absolute URL')
  }
  const isLoopback =
    url.hostname === 'localhost' ||
    url.hostname === '[::1]' ||
    /^127(?:\.\d{1,3}){3}$/.test(url.hostname)
  if (url.protocol !== 'https:' && !(url.protocol === 'http:' && isLoopback)) {
    throw new Error('apiOrigin must use https or http on loopback')
  }
  if (
    url.username !== '' ||
    url.password !== '' ||
    (url.pathname !== '/' && url.pathname !== '') ||
    url.search !== '' ||
    url.hash !== ''
  ) {
    throw new Error(
      'apiOrigin must be a bare origin without path, query, or credentials',
    )
  }
  return url.origin
}

// ApplicationOperatorClient performs Ed25519-signed cloud API calls for
// application operators over the shared proto-binary HTTP contract.
//
// The signed payload is the deterministic proto binary of
// provider.spacewave.api.SigningPayload; keys are held by the caller-supplied
// sign callback, never by this client.
export class ApplicationOperatorClient {
  private readonly apiOrigin: string
  private readonly signingEnvPrefix: string
  private readonly sessionPeerId: string
  private readonly sign: SigningFn
  private readonly fetchImpl: typeof globalThis.fetch

  public constructor(options: ApplicationOperatorClientOptions) {
    this.apiOrigin = validateOrigin(options.apiOrigin)
    this.signingEnvPrefix = options.signingEnvPrefix
    this.sessionPeerId = options.sessionPeerId
    this.sign = options.sign

    // fetchImpl binds the default fetch to globalThis; the Worker fetch
    // requires globalThis as its receiver.
    this.fetchImpl = options.fetch ?? globalThis.fetch.bind(globalThis)
  }

  // getApplication reads a product registration visible to this session's account.
  public async getApplication(
    applicationId: string,
    signal?: AbortSignal,
  ): Promise<GetApplicationResponse> {
    const request: GetApplicationRequest = { applicationId }
    return this.postBinary(
      '/api/applications/get',
      GetApplicationRequest,
      GetApplicationResponse,
      request,
      signal,
    )
  }

  // enrollManagedAccount submits the independently signed credential proof
  // after the application's verified login.
  public async enrollManagedAccount(
    request: EnrollManagedAccountRequest,
    signal?: AbortSignal,
  ): Promise<EnrollManagedAccountResponse> {
    return this.postBinary(
      '/api/applications/accounts/enroll',
      EnrollManagedAccountRequest,
      EnrollManagedAccountResponse,
      request,
      signal,
    )
  }

  // postBinary signs and executes a POST with proto-binary content, returning
  // the decoded response message. Signed headers bind content-type, path,
  // timestamp, content length, and body hash exactly as the Go client does.
  private async postBinary<Req extends Message<Req>, Res extends Message<Res>>(
    path: string,
    requestType: MessageType<Req>,
    responseType: MessageType<Res>,
    request: Req,
    signal?: AbortSignal,
  ): Promise<Res> {
    const body = requestType.toBinary(request)
    const bodyHashHex = await sha256Hex(body)
    const timestampMs = BigInt(Date.now())
    const payload = SigningPayload.toBinary({
      envPrefix: this.signingEnvPrefix,
      method: 'POST',
      path,
      timestampMs,
      contentLength: BigInt(body.length),
      bodyHashHex,
      signedHeaders: 'content-type=application/octet-stream',
    })
    const signature = await this.sign(payload, signal)
    const headers: Record<string, string> = {
      'Content-Type': 'application/octet-stream',
      Accept: 'application/octet-stream',
      'X-Peer-ID': this.sessionPeerId,
      'X-Timestamp': timestampMs.toString(),
      'X-Sw-Hash': bodyHashHex,
      'X-Signature': uint8ArrayToBase64(signature),
      'X-Signed-Headers': 'content-type',
    }

    const response = await this.fetchImpl(
      new URL(path, this.apiOrigin).toString(),
      {
        method: 'POST',
        headers,
        body: body.slice() as BodyInit,
        // Never forward signed requests across redirects. Manual mode also
        // works in workerd, which does not implement redirect: 'error'.
        redirect: 'manual',
        signal,
      },
    )

    const bytes = await readBoundedBody(response)

    if (response.status < 200 || response.status >= 300) {
      throw parseCloudError(response.status, bytes)
    }

    return responseType.fromBinary(bytes)
  }
}
