import { parseAst } from 'rolldown/parseAst'

import { remapWebPkgSpecifier } from './plugin.js'

/** adaptDevelopmentClient replaces only Vite's transport initializer. */
export function adaptDevelopmentClient(
  code: string,
  sessionID: string,
): string {
  // Require the upstream seam before replacing any executable source.
  const program = parseAst(code)
  const bindings = program.body.flatMap((statement) =>
    statement.type === 'VariableDeclaration'
      ? statement.declarations.filter(
          (declaration) =>
            declaration.id.type === 'Identifier' &&
            declaration.id.name === 'transport',
        )
      : [],
  )
  const initializer = bindings[0]?.init
  if (
    bindings.length !== 1 ||
    initializer?.type !== 'CallExpression' ||
    initializer.callee.type !== 'Identifier' ||
    initializer.callee.name !== 'normalizeModuleRunnerTransport'
  ) {
    throw new Error(
      'Bldr frontend: unsupported Vite client transport initializer',
    )
  }

  // Each compiler uses its own attachment; Vite retains its update algorithm.
  const frontend = `globalThis.__bldrFrontends?.get(${JSON.stringify(sessionID)})`
  const replacement = `normalizeModuleRunnerTransport({
    connect(handlers) {
      const frontend = ${frontend};
      if (!frontend) throw new Error('Bldr frontend transport is not attached');
      return frontend.connect(handlers);
    },
    send(payload) { return ${frontend}?.send(payload); },
    disconnect() { ${frontend}?.disconnect(); }
  })`
  return (
    code.slice(0, initializer.start) + replacement + code.slice(initializer.end)
  )
}

/** bindDevelopmentImports preserves the document's canonical module identities. */
export function bindDevelopmentImports(
  code: string,
  prefix: string,
  external: string[],
  refreshPath: string,
  webPkgIDs: string[] = [],
): string {
  if (
    !code.includes(prefix + '@react-refresh') &&
    !external.some((pkg) => code.includes(prefix + '@id/' + pkg))
  )
    return code
  const replacements: { start: number; end: number; value: string }[] = []
  const rewrite = (node: {
    type: string
    value?: unknown
    start: number
    end: number
  }) => {
    if (node.type !== 'Literal' || typeof node.value !== 'string') return
    const source = node.value
    let target: string | undefined
    if (source === prefix + '@react-refresh') target = refreshPath
    else if (source.startsWith(prefix + '@id/')) {
      const id = source.slice((prefix + '@id/').length)
      target = remapWebPkgSpecifier(id, webPkgIDs, '/b/pkg')?.remapped
      if (
        !target &&
        external.some((pkg) => id === pkg || id.startsWith(pkg + '/'))
      )
        target = id
    }
    if (target)
      replacements.push({
        start: node.start,
        end: node.end,
        value: JSON.stringify(target),
      })
  }
  const visit = (value: unknown): void => {
    if (!value || typeof value !== 'object') return
    if (Array.isArray(value)) {
      value.forEach(visit)
      return
    }
    const node = value as Record<string, unknown>
    if (
      [
        'ImportDeclaration',
        'ExportNamedDeclaration',
        'ExportAllDeclaration',
        'ImportExpression',
      ].includes(node.type as string) &&
      node.source
    ) {
      rewrite(node.source as Parameters<typeof rewrite>[0])
    }
    Object.values(node).forEach(visit)
  }
  visit(parseAst(code))
  replacements.sort((a, b) => b.start - a.start)
  for (const replacement of replacements) {
    code =
      code.slice(0, replacement.start) +
      replacement.value +
      code.slice(replacement.end)
  }
  return code
}
