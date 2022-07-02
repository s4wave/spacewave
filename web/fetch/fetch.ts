import {
  FetchService,
  FetchRequest,
  FetchResponse,
  ResponseInfo,
} from './fetch.pb.js'
import { castToError } from '../bldr/error.js'

// buildFetchHeaders builds a Headers map from a Headers object.
export function buildFetchHeaders(headers: Headers): { [key: string]: string } {
  const result: any = {}
  headers.forEach((value: string, key: string) => {
    result[key] = value
  })
  return result
}

// buildHeaders builds the Headers object from a headers map.
export function buildHeaders(
  headersMap: { [key: string]: string } | null
): Headers {
  return headersMap ? new Headers(headersMap) : new Headers()
}

// buildFetchRequest builds a FetchRequest message from a Request.
export function buildFetchRequest(
  request: Request,
  clientId: string
): FetchRequest {
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
  it: AsyncIterator<FetchResponse>
): ReadableStream {
  async function readResponse(
    controller: ReadableStreamController<Uint8Array>
  ) {
    try {
      while (true) {
        const next = await it.next()
        if (next.done) {
          controller.close()
          return
        }
        const value: FetchResponse = next.value
        if (value?.body?.$case !== 'responseData') {
          continue
        }
        const responseData = value.body.responseData?.data
        if (responseData && responseData.length) {
          controller.enqueue(responseData)
        }
      }
    } catch (err) {
      const error = castToError(err, 'fetch response data')
      controller.error(error)
      if (it && it.throw) {
        it.throw(error)
      }
    }
  }
  // bodyInit is the streaming response body.
  return new ReadableStream({
    start(controller) {
      readResponse(controller)
    },
    cancel(reason) {
      const error = castToError(reason, 'fetch canceled')
      if (it && it.throw) {
        it.throw(error)
      }
    },
  })
}

// proxyFetch proxies a Fetch request to a remote Fetch service.
export async function proxyFetch(
  svc: FetchService,
  request: Request,
  clientId: string
): Promise<Response> {
  let resultIt: AsyncIterator<FetchResponse> | null = null
  try {
    // build the fetch request.
    const fetchRequest = buildFetchRequest(request, clientId)
    // start the rpc
    const resultIterable = svc.Fetch(fetchRequest)
    // wait for the first packet w/ the response headers
    resultIt = resultIterable[Symbol.asyncIterator]()
    // firstPkt contains the result headers.
    const firstPkt = await resultIt.next()
    const firstPktResp: FetchResponse = firstPkt.value
    const firstPktBody = firstPktResp.body
    if (!firstPktBody || firstPkt.done) {
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
    if (resultIt && resultIt.throw) {
      resultIt.throw(error)
    }
    let responseBlob = new Blob([error.message + '\n'], {type: 'text/plain'})
    return new Response(responseBlob, {
      headers: { 'Content-Type': 'text/plain' },
      status: 500,
      statusText: error.message,
    })
  }
}
