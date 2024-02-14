import toIt from 'browser-readablestream-to-it'
import { Source } from 'it-stream-types'
import { pipe } from 'it-pipe'
import { pushable } from 'it-pushable'
import { castToError, buildPushableSink } from 'starpc'

import {
  FetchService,
  FetchRequestInfo,
  FetchResponse,
  ResponseInfo,
  FetchRequest,
} from './fetch.pb.js'

// buildFetchHeaders builds a Headers map from a Headers object.
export function buildFetchHeaders(headers: Headers): Record<string, string> {
  const result: Record<string, string> = {}
  headers.forEach((value: string, key: string) => {
    result[key] = value
  })
  return result
}

// buildHeaders builds the Headers object from a headers map.
export function buildHeaders(
  headersMap: { [key: string]: string } | null,
): Headers {
  return headersMap ? new Headers(headersMap) : new Headers()
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
      $case: 'requestData',
      requestData: { data: data || new Uint8Array(0), done },
    },
  }
}

// buildResponseInit builds the ResponseInit from the ResponseInfo.
export function buildResponseInit(info: ResponseInfo): ResponseInit {
  const headers = buildHeaders(info.headers || null)
  return {
    headers,
    status: info.status,
    statusText: info.statusText,
  }
}

// buildResponseStream builds the ReadableStream for a response body.
export function buildResponseStream(
  it: AsyncIterator<FetchResponse>,
): ReadableStream {
  async function readResponse(
    controller: ReadableStreamController<Uint8Array>,
  ) {
    // note: workaround type mismatch error here (types are fine)
    const enqueue: (data: Uint8Array) => void =
      controller.enqueue.bind(controller)
    while (it) {
      const next = await it.next()
      if (next.done) {
        controller.close()
        return
      }
      const value: FetchResponse = next.value
      if (value?.body?.$case !== 'responseData') {
        continue
      }
      const responseDataPkt = value.body.responseData
      const responseData = responseDataPkt?.data
      if (responseData && responseData.length) {
        enqueue(responseData as Uint8Array)
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
): Promise<Response> {
  let resultIt: AsyncIterator<FetchResponse> | null = null
  try {
    const requestBody = request.body
    const hasBody = !!requestBody
    // build the fetch request.
    const fetchRequestInfo = buildFetchRequestInfo(request, clientId, hasBody)
    // build the pushable
    const fetchRequestStream = pushable<FetchRequest>({ objectMode: true })
    // push the initial info packet
    fetchRequestStream.push({
      body: {
        $case: 'requestInfo',
        requestInfo: fetchRequestInfo,
      },
    })
    // start the rpc
    const resultIterable = svc.Fetch(fetchRequestStream)
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
    const firstPkt = await resultIt.next()
    const firstPktResp: FetchResponse = firstPkt?.value
    const firstPktBody = firstPktResp?.body
    if (!firstPktBody || !firstPkt || firstPkt.done) {
      throw new Error('empty fetch rpc response')
    }
    if (firstPktBody.$case !== 'responseInfo') {
      throw new Error('expected response info as first packet')
    }
    // responseInit is the headers and other immediate information.
    const responseInfo = firstPktBody.responseInfo
    const responseInit = buildResponseInit(responseInfo)
    const responseBody = buildResponseStream(resultIt)
    // return the streaming response
    return new Response(responseBody, responseInit)
  } catch (err) {
    const error = castToError(err, 'failed to start fetch request')
    console.error('fetch: proxyFetch catch error', error)
    if (resultIt && resultIt.throw) {
      resultIt.throw(error)
    }
    const responseBlob = new Blob([error.message + '\n'], {
      type: 'text/plain',
    })
    return new Response(responseBlob, {
      headers: { 'Content-Type': 'text/plain' },
      status: 500,
      statusText: error.message,
    })
  }
}
