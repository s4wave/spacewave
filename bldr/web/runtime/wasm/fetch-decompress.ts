// fetchWithDecompress fetches source and manually decompresses .gz assets
// when the browser has not already decoded the response.
export async function fetchWithDecompress(source: string): Promise<Response> {
  const response = await fetch(source, { method: 'GET', cache: 'force-cache' })
  if (!response.ok) {
    throw new Error(`fetching url ${source} returned status ${response.status}`)
  }

  if (!source.endsWith('.gz') || !response.body) {
    return response
  }

  const { body, gzipEncoded } = await replayBodyWithGzipSniff(response.body)
  const headers = new Headers(response.headers)
  if (gzipEncoded) {
    headers.delete('content-encoding')
    const decompressedStream = body.pipeThrough(
      new DecompressionStream('gzip') as unknown as ReadableWritablePair<
        Uint8Array,
        Uint8Array
      >,
    )
    return new Response(decompressedStream, {
      headers,
      status: response.status,
      statusText: response.statusText,
    })
  }

  headers.delete('content-encoding')
  return new Response(body, {
    headers,
    status: response.status,
    statusText: response.statusText,
  })
}

async function replayBodyWithGzipSniff(
  body: ReadableStream<Uint8Array>,
): Promise<{
  body: ReadableStream<Uint8Array>
  gzipEncoded: boolean
}> {
  const reader = body.getReader()
  const first = await reader.read()
  const firstChunk = first.done ? undefined : first.value
  const gzipEncoded =
    !!firstChunk &&
    firstChunk.length >= 2 &&
    firstChunk[0] === 0x1f &&
    firstChunk[1] === 0x8b

  return {
    gzipEncoded,
    body: new ReadableStream<Uint8Array>({
      start(controller) {
        if (firstChunk) {
          controller.enqueue(firstChunk)
        }
        if (first.done) {
          controller.close()
          return
        }
        pumpReplayReader(reader, controller)
      },
      cancel(reason) {
        return reader.cancel(reason)
      },
    }),
  }
}

function pumpReplayReader(
  reader: ReadableStreamDefaultReader<Uint8Array>,
  controller: ReadableStreamDefaultController<Uint8Array>,
): void {
  reader
    .read()
    .then(({ done, value }) => {
      if (done) {
        controller.close()
        return
      }
      controller.enqueue(value)
      pumpReplayReader(reader, controller)
    })
    .catch((err: unknown) => {
      controller.error(err)
    })
}
