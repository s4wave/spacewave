// Ignore any URLs that are outside of /b/ or /p/.
export const BLDR_URI_PREFIXES = [
  // /b/ is short for bldr
  '/b/',
  // /p/ is short for plugin
  '/p/',
] as const
