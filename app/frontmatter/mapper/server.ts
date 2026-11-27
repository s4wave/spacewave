// The frontmatter content mapper lets `tsrs --runExternalCode` type check the
// frontmatter of Markdown files. tsconfig.json selects the mapper for `.md`
// files and lists schemas, each a directory and the TypeScript type its pages
// satisfy.
// A file under a schema directory becomes a `satisfies` expression over its
// fields, so a missing, unknown or mistyped field fails the type check on its
// own line. Any other `.md` file becomes an empty module.
//
// TypeScript caches transforms by this package's version, so bump the version
// in package.json, and relock with bun install, when the output changes.

import { dirname, relative, resolve } from 'node:path'

import { readFrontmatter } from '../frontmatter.js'

// Schema assigns a frontmatter type to the Markdown files under a directory.
interface Schema {
  // dir is the directory, relative to the tsconfig.json directory.
  dir: string
  // module is the module that exports the type, relative to the same directory.
  module: string
  // type is the exported type name.
  type: string
}

// Project holds one tsconfig.json project's schemas with absolute paths.
interface Project {
  schemas: Schema[]
}

// SpanKind is the content mapper protocol's span kind.
const enum SpanKind {
  Atom = 1,
}

// Request is a JSON-RPC request from TypeScript.
interface Request {
  id?: number | string
  method: string
  params: Record<string, unknown>
}

// projects maps each open project handle to its schemas.
const projects = new Map<string, Project>()

// openProject reads a project's schema options and reports invalid entries.
function openProject(params: Record<string, unknown>) {
  const configDir = dirname(params['configFileName'] as string)
  const options = (params['options'] ?? {}) as Record<string, unknown>
  const entries = Array.isArray(options['schemas']) ? options['schemas'] : []

  // Keep each complete schema and report the rest against their options path.
  const schemas: Schema[] = []
  const optionDiagnostics = []
  if (!Array.isArray(options['schemas'])) {
    optionDiagnostics.push({
      path: ['schemas'],
      messageText: 'schemas must list { dir, module, type } entries.',
      code: 1,
    })
  }
  for (const [index, entry] of entries.entries()) {
    const { dir, module, type } = (entry ?? {}) as Record<string, unknown>
    if (
      typeof dir !== 'string' ||
      typeof module !== 'string' ||
      typeof type !== 'string'
    ) {
      optionDiagnostics.push({
        path: ['schemas', index],
        messageText: 'A schema needs string dir, module and type fields.',
        code: 2,
      })
      continue
    }
    schemas.push({
      dir: resolve(configDir, dir),
      module: resolve(configDir, module),
      type,
    })
  }

  projects.set(params['projectHandle'] as string, { schemas })
  return { optionDiagnostics }
}

// transform renders one Markdown file as TypeScript that checks its
// frontmatter against the schema for its directory.
function transform(params: Record<string, unknown>) {
  const fileName = params['fileName'] as string
  const project = projects.get(params['projectHandle'] as string)
  const schema = project?.schemas.find((s) => fileName.startsWith(s.dir + '/'))
  if (!schema) return { text: 'export {}\n', extension: '.ts' }

  // Import the schema type by a path relative to the Markdown file.
  let specifier = relative(dirname(fileName), schema.module)
  if (!specifier.startsWith('.')) specifier = './' + specifier
  let text = `import type { ${schema.type} } from ${JSON.stringify(specifier)}\n;(`

  // Map the object and its type to the first line, so a missing field or
  // missing block reports there.
  const content = params['content'] as string
  const newline = content.indexOf('\n')
  const firstLine = newline === -1 ? content.length : newline
  const mappings: number[][] = [[text.length, 1, 0, firstLine, SpanKind.Atom]]
  text += '{\n'

  // Write each field, mapping its quoted key and its value to the raw text.
  for (const field of readFrontmatter(content).fields) {
    text += '  '
    const key = JSON.stringify(field.key)
    mappings.push([
      text.length,
      key.length,
      field.keyStart,
      field.key.length,
      SpanKind.Atom,
    ])
    text += `${key}: `
    const literal = JSON.stringify(field.value)
    mappings.push([
      text.length,
      literal.length,
      field.valueStart,
      field.valueEnd - field.valueStart,
      SpanKind.Atom,
    ])
    text += `${literal},\n`
  }

  // Check the object against the schema type, reporting on the first line.
  text += '}) '
  const check = `satisfies ${schema.type}`
  mappings.push([text.length, check.length, 0, firstLine, SpanKind.Atom])
  text += `${check}\nexport {}\n`
  return { text, extension: '.ts', mappings }
}

// handle answers one request from TypeScript.
function handle(request: Request): unknown {
  switch (request.method) {
    case 'initialize':
      return { positionEncoding: 'utf-16', diagnosticSource: 'frontmatter' }
    case 'openProject':
      return openProject(request.params)
    case 'transform':
      return transform(request.params)
    case 'closeProject':
      projects.delete(request.params['projectHandle'] as string)
      return null
  }
  throw new Error(`unknown method ${request.method}`)
}

// send writes one JSON-RPC message with LSP Content-Length framing.
function send(message: unknown) {
  const body = Buffer.from(JSON.stringify(message))
  process.stdout.write(`Content-Length: ${body.length}\r\n\r\n`)
  process.stdout.write(body)
}

// Read framed requests from stdin and answer each one in order.
let pending = Buffer.alloc(0)
process.stdin.on('data', (chunk: Buffer) => {
  pending = Buffer.concat([pending, chunk])
  for (;;) {
    const headerEnd = pending.indexOf('\r\n\r\n')
    if (headerEnd === -1) return
    const header = pending.subarray(0, headerEnd).toString()
    const length = Number(/Content-Length: (\d+)/i.exec(header)?.[1])
    const bodyStart = headerEnd + 4
    if (pending.length < bodyStart + length) return
    const request = JSON.parse(
      pending.subarray(bodyStart, bodyStart + length).toString(),
    ) as Request
    pending = pending.subarray(bodyStart + length)
    if (request.id == null) continue
    try {
      send({ jsonrpc: '2.0', id: request.id, result: handle(request) })
    } catch (err) {
      send({
        jsonrpc: '2.0',
        id: request.id,
        error: { code: -32601, message: String(err) },
      })
    }
  }
})
process.stdin.on('end', () => process.exit(0))
