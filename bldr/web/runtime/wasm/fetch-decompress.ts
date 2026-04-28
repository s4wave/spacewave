// fetchWithDecompress fetches source and manually decompresses .gz assets
// when the browser has not already decoded the response.
export async function fetchWithDecompress(source: string): Promise<Response> {
  const response = await fetch(source, { method: 'GET', cache: 'force-cache' })
  if (!response.ok) {
    throw new Error(`fetching url ${source} returned status ${response.status}`)
  }

  if (!response.body) {
    return response
  }

  const contentEncoding = response.headers.get('content-encoding')?.toLowerCase()
  if (source.endsWith('.gz') && !contentEncoding?.includes('gzip')) {
    const ds = new DecompressionStream('gzip')
    const decompressedStream = response.body.pipeThrough(ds)
    return new Response(decompressedStream, { headers: response.headers })
  }

  return response
}
