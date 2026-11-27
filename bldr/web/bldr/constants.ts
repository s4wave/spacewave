// BLDR_URI_PREFIXES lists /b/ and /p/ so URLs outside those prefixes are
// ignored.
export const BLDR_URI_PREFIXES = [
  // /b/ is short for bldr
  '/b/',
  // /p/ is short for plugin
  '/p/',
] as const
