import toIt from './readablestream-to-it.js'
import { Source } from 'it-stream-types'
import { pipe } from 'it-pipe'
import { pushable } from 'it-pushable'
import { castToError, buildPushableSink } from 'starpc'

import {
  FetchRequestInfo,
  FetchResponse,
  ResponseInfo,
  FetchRequest,
} from './fetch.pb.js'
import { FetchService } from './fetch_srpc.pb.js'
import { Message } from '@aptre/protobuf-es-lite'

export interface ProxyFetchOpts {
  // abortSignal is an optional extra signal owned by the caller.
  abortSignal?: AbortSignal
  // headerTimeoutMs aborts the proxied fetch if the first response packet does
  // not arrive in time. When unset, no timeout is applied.
  headerTimeoutMs?: number
}

// createLinkedAbortController links a new controller to any provided parent
// signals and returns a cleanup callback for the listeners.
function createLinkedAbortController(
  ...signals: Array<AbortSignal | undefined>
) {
  const abortController = new AbortController()
  const cleanupFns: Array<() => void> = []
  const abort = (reason?: unknown) => {
    if (!abortController.signal.aborted) {
      abortController.abort(reason)
    }
  }

  for (const signal of signals) {
    if (!signal) {
      continue
    }
    if (signal.aborted) {
      abort(signal.reason)
      break
    }
    const onAbort = () => abort(signal.reason)
    signal.addEventListener('abort', onAbort, { once: true })
    cleanupFns.push(() => signal.removeEventListener('abort', onAbort))
  }

  return {
    abortController,
    cleanup: () => {
      for (const cleanupFn of cleanupFns) {
        cleanupFn()
      }
    },
  }
}

// waitForFirstPacket waits for the first response packet, optionally with a
// timeout tied into the provided abort controller.
async function waitForFirstPacket(
  it: AsyncIterator<Message<FetchResponse>>,
  abortController: AbortController,
  headerTimeoutMs?: number,
) {
  if (headerTimeoutMs == null) {
    return it.next()
  }

  return new Promise<IteratorResult<Message<FetchResponse>>>(
    (resolve, reject) => {
      const timer = globalThis.setTimeout(() => {
        abortController.abort(
          new Error(
            `timed out waiting ${headerTimeoutMs}ms for proxied fetch response headers`,
          ),
        )
      }, headerTimeoutMs)

      const onAbort = () => {
        clearTimeout(timer)
        const reason = abortController.signal.reason
        reject(
          reason instanceof Error ? reason : (
            new Error('proxied fetch aborted before response headers')
          ),
        )
      }

      abortController.signal.addEventListener('abort', onAbort, { once: true })
      it.next()
        .then((value) => {
          clearTimeout(timer)
          abortController.signal.removeEventListener('abort', onAbort)
          resolve(value)
        })
        .catch((err) => {
          clearTimeout(timer)
          abortController.signal.removeEventListener('abort', onAbort)
          reject(err)
        })
    },
  )
}

// buildFetchHeaders builds a Headers map from a Headers object.
export function buildFetchHeaders(headers: Headers): Record<string, string> {
  const result: Record<string, string> = {}
  headers.forEach((value: string, key: string) => {
    result[key] = value
  })
  return result
}

// buildFetchRequestInfo builds a FetchRequestInfo message from a Request.
export function buildFetchRequestInfo(
  request: Request,
  clientId: string,
  hasBody: boolean,
): FetchRequestInfo {
  return {
    method: request.method,
    url: request.url,
    headers: buildFetchHeaders(request.headers),
    clientId,
    destination: request.destination,
    integrity: request.integrity,
    mode: request.mode,
    redirect: request.redirect,
    referrer: request.referrer,
    referrerPolicy: request.referrerPolicy,
    hasBody,
  }
}

// buildRequestData builds a RequestData packet.
export function buildRequestData(
  data: Uint8Array | null,
  done: boolean,
): FetchRequest {
  return {
    body: {
      case: 'requestData',
      value: { data: data || new Uint8Array(0), done },
    },
  }
}

// toByteString coerces a DOMString into a ByteString-compatible string.
function toByteString(value: string): string {
  for (let i = 0; i < value.length; i += 1) {
    if (value.charCodeAt(i) > 0xff) {
      const bytes = new TextEncoder().encode(value)
      let result = ''
      for (const byte of bytes) {
        result += String.fromCharCode(byte)
      }
      return result
    }
  }
  return value
}

// buildResponseHeaders builds response headers that satisfy the DOM ByteString
// requirement enforced by the Response constructor.
export function buildResponseHeaders(
  headers: Record<string, string> | undefined,
): Headers | undefined {
  if (!headers) {
    return undefined
  }
  const result = new Headers()
  Object.entries(headers).forEach(([key, value]) => {
    result.append(key, toByteString(value))
  })
  return result
}

// buildResponseInit builds the ResponseInit from the ResponseInfo.
export function buildResponseInit(info: ResponseInfo): ResponseInit {
  return {
    headers: buildResponseHeaders(info.headers ?? undefined),
    status: info.status,
    statusText: info.statusText,
  }
}

// buildResponseStream builds the ReadableStream for a response body.
export function buildResponseStream(
  it: AsyncIterator<Message<FetchResponse>>,
): ReadableStream {
  async function readResponse(
    controller: ReadableStreamController<Uint8Array>,
  ) {
    // note: workaround type mismatch error here (types are fine)
    const enqueue = (controller.enqueue as (data: Uint8Array) => void).bind(
      controller,
    )
    while (it) {
      const next = await it.next()
      if (next.done) {
        controller.close()
        return
      }
      const value = next.value
      if (value?.body?.case !== 'responseData') {
        continue
      }
      const responseDataPkt = value.body.value
      const responseData = responseDataPkt?.data
      if (responseData && responseData.length) {
        enqueue(responseData)
      }
      if (responseDataPkt?.done) {
        controller.close()
        return
      }
    }
  }
  // bodyInit is the streaming response body.
  return new ReadableStream({
    start(controller) {
      readResponse(controller).catch((err) => {
        const error = castToError(err, 'fetch response data')
        controller.error(error)
      })
    },
    cancel(reason) {
      if (it.return) {
        it.return(reason)
      }
    },
  })
}

// transformRequestData wraps a Uint8Array source in RequestData packets.
// Transform<Uint8Array, FetchRequest>
export async function* transformRequestData(
  source: Source<Uint8Array>,
): AsyncIterable<FetchRequest> {
  for await (const pkt of source) {
    if (Array.isArray(pkt)) {
      for (const p of pkt) {
        yield* [buildRequestData(p, false)]
      }
    } else {
      yield* [buildRequestData(pkt, false)]
    }
  }
}

// proxyFetch proxies a Fetch request to a remote Fetch service.
export async function proxyFetch(
  svc: FetchService,
  request: Request,
  clientId: string,
  opts?: ProxyFetchOpts,
): Promise<Response> {
  let resultIt: AsyncIterator<Message<FetchResponse>> | null = null
  const { abortController, cleanup } = createLinkedAbortController(
    request.signal,
    opts?.abortSignal,
  )
  try {
    // get the request body
    const requestBody = request.body
    const hasBody = !!requestBody
    // build the fetch request.
    const fetchRequestInfo = buildFetchRequestInfo(request, clientId, hasBody)
    // build the pushable
    const fetchRequestStream = pushable<FetchRequest>({
      objectMode: true,
    })
    // push the initial info packet
    fetchRequestStream.push({
      body: {
        case: 'requestInfo',
        value: fetchRequestInfo,
      },
    })

    // TODO: Browsers do not cancel request.signal when the request is canceled.
    // This is a long-standing browser bug and is not yet fixed.
    // See: https://github.com/w3c/ServiceWorker/issues/1544
    const resultIterable = svc.Fetch(fetchRequestStream, abortController.signal)

    // stream the body
    if (hasBody) {
      const bodyIt = toIt(requestBody!)
      const fetchRequestSink =
        buildPushableSink<FetchRequest>(fetchRequestStream)
      pipe(bodyIt, transformRequestData, fetchRequestSink)
        .catch((err) => fetchRequestStream.end(err))
        .then(() => fetchRequestStream.end())
    }

    // wait for the first packet w/ the response headers
    resultIt = resultIterable[Symbol.asyncIterator]()

    // firstPkt contains the result headers.
    const firstPkt = await waitForFirstPacket(
      resultIt,
      abortController,
      opts?.headerTimeoutMs,
    )
    const firstPktResp: FetchResponse = firstPkt?.value
    const firstPktBody = firstPktResp?.body
    if (!firstPktBody || !firstPkt || firstPkt.done) {
      throw new Error('empty fetch rpc response')
    }
    if (firstPktBody.case !== 'responseInfo') {
      throw new Error('expected response info as first packet')
    }

    // responseInit is the headers and other immediate information.
    const responseInfo = firstPktBody.value
    const responseInit = buildResponseInit(responseInfo)
    const responseBody = buildResponseStream(resultIt)

    // return the streaming response
    return new Response(responseBody, responseInit)
  } catch (err) {
    if (resultIt?.return) {
      void resultIt.return(err)
    }
    const error = castToError(err, 'failed to start fetch request')
    logProxyFetchError(error)

    let responseMessage = error.message
    let responseStatus = 500
    if (error.message === 'Failed to fetch') {
      // return a more descriptive error
      responseStatus = 503
      responseMessage = 'Error making the request.'
    }

    const responseBlob = new Blob([responseMessage + '\n'], {
      type: 'text/plain',
    })
    return new Response(responseBlob, {
      headers: { 'Content-Type': 'text/plain' },
      status: responseStatus,
      // statusText: 'Error: ' + error.message,
    })
  } finally {
    cleanup()
  }
}

function logProxyFetchError(error: Error) {
  try {
    console.error('fetch: proxyFetch catch error', error)
  } catch (err) {
    if (!isBrokenPipeError(err)) {
      throw err
    }
  }
}

function isBrokenPipeError(err: unknown): boolean {
  return (
    typeof err === 'object' &&
    err !== null &&
    'code' in err &&
    err.code === 'EPIPE'
  )
}
