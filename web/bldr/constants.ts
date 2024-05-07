// Ignore any URLs that are outside of /b/ or /p/.
export const BLDR_URI_PREFIXES = [
  // /b/ is short for bldr
  '/b/',
  // /p/ is short for plugin
  '/p/',
] as const

// BLDR_CACHE_PATHS are files to cache on sw startup.
export const BLDR_CACHE_PATHS = ['/', '/index.html'] as const
