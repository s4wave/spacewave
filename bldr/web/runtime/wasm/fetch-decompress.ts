// fetchWithDecompress fetches data from the source and decompresses if the file ends in .gz.
export async function fetchWithDecompress(source: string): Promise<Response> {
  const response = await fetch(source, { method: 'GET', cache: 'force-cache' })
  if (!response.ok) {
    throw new Error(`fetching url ${source} returned status ${response.status}`)
  }

  if (!response.body) {
    return response
  }

  if (source.endsWith('.gz')) {
    const ds = new DecompressionStream('gzip')
    const decompressedStream = response.body.pipeThrough(ds)
    return new Response(decompressedStream, { headers: response.headers })
  }

  return response
}
