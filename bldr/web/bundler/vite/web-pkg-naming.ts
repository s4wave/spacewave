import path from 'path'

// Extensions stripped from compiled JavaScript entries, including ".pb" for
// proto modules addressed by export subpath.
const KNOWN_EXTENSIONS = new Set([
  '.js',
  '.cjs',
  '.mjs',
  '.ts',
  '.tsx',
  '.jsx',
  '.pb',
  '.css',
])

// Extensions of TypeScript sources, which consumers import by their emitted
// ".js" path.
const TYPESCRIPT_EXTENSIONS = new Set(['.ts', '.tsx'])

// Extensions a consumer may write on an entry specifier.
const SPECIFIER_EXTENSIONS = new Set(['.js', '.mjs', '.jsx', '.ts', '.tsx'])

/** isTypeScriptEntry reports whether a package import path is a TypeScript source. */
export function isTypeScriptEntry(importPath: string): boolean {
  return TYPESCRIPT_EXTENSIONS.has(path.extname(importPath))
}

// stripKnownExts removes all trailing known extensions, e.g.
// "google/protobuf/timestamp.pb.js" -> "google/protobuf/timestamp".
function stripKnownExts(name: string): string {
  while (true) {
    const ext = path.extname(name)
    if (!ext || !KNOWN_EXTENSIONS.has(ext)) break
    name = name.substring(0, name.length - ext.length)
  }
  return name
}

function trimDotSlash(importPath: string): string {
  return importPath.startsWith('./') ? importPath.substring(2) : importPath
}

/**
 * servedEntryName returns the served "[name].mjs" base for a package import
 * path. A TypeScript source drops only its own extension, so "resource.pb.ts"
 * and "resource.ts" stay distinct. A compiled JavaScript entry drops every
 * known extension to match its export subpath, e.g.
 * "google/protobuf/timestamp.pb.js" -> "google/protobuf/timestamp".
 */
export function servedEntryName(importPath: string): string {
  const name = trimDotSlash(importPath)
  const ext = path.extname(name)
  if (TYPESCRIPT_EXTENSIONS.has(ext)) {
    return name.substring(0, name.length - ext.length)
  }
  return stripKnownExts(name)
}

/**
 * specifierEntryNames returns the served-name candidates for a consumer
 * subpath, most specific first: "object/object.pb.js" names a TypeScript
 * "object/object.pb" entry or a compiled "object/object" entry.
 */
export function specifierEntryNames(subPath: string): string[] {
  const name = trimDotSlash(subPath)
  const ext = path.extname(name)
  const exact = SPECIFIER_EXTENSIONS.has(ext)
    ? name.substring(0, name.length - ext.length)
    : name
  const stripped = stripKnownExts(name)
  return exact === stripped ? [exact] : [exact, stripped]
}

/**
 * buildWebPkgImportSpecifier returns the import map specifier for a served
 * entry. The package root entry maps the bare package id. TypeScript entries
 * map their emitted ".js" path, which is what consumers import.
 */
export function buildWebPkgImportSpecifier(
  pkgId: string,
  baseName: string,
  rootServedName: string | null,
  typescript: boolean,
): string {
  if (
    baseName === rootServedName ||
    (!rootServedName && baseName === 'index')
  ) {
    return pkgId
  }
  return typescript ? `${pkgId}/${baseName}.js` : `${pkgId}/${baseName}`
}
