import { readFile, unlink, writeFile } from 'fs/promises'
import { basename, dirname, relative, resolve } from 'path'

const root = resolve(import.meta.dirname, '..')

async function ensureBuildTag(path: string, tag: string) {
  const fullPath = resolve(root, path)
  const content = await readFile(fullPath, 'utf-8')
  const header = `//go:build ${tag}\n\n`
  if (content.startsWith(header)) {
    return
  }
  if (content.startsWith('//go:build ')) {
    throw new Error(`${path} already has a different build tag`)
  }
  await writeFile(fullPath, header + content)
}

// ResourceIDField describes one generated resource ID accessor.
export type ResourceIDField = {
  getter: string
  kind: 'map' | 'nested' | 'repeated' | 'singular'
}

// ResourceIDMessage describes the resource ID accessors generated for a message.
export type ResourceIDMessage = {
  fields: ResourceIDField[]
  name: string
}

// ResourceRPCMethod identifies one unary method that supports resource adoption.
export type ResourceRPCMethod = {
  method: string
  service: string
}

const resourceIDMarker = '// @resource-adoption-id'
const resourceIDsMarker = '// @resource-adoption-ids'
const generatedSuffix = '_resource_ids.pb.go'
const resourceRPCMethodsOutput = 'bldr/resource/resource-rpc-methods.pb.go'

function goName(name: string): string {
  return name
    .split('_')
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join('')
}

// parseAdoptingUnaryRPCMethods returns unary methods with generated response metadata.
export function parseAdoptingUnaryRPCMethods(
  content: string,
  messages = parseResourceIDMessages(content),
): ResourceRPCMethod[] {
  const packageName = content.match(/^package\s+([\w.]+);$/m)?.[1]
  if (!packageName) {
    return []
  }

  const adoptingResponses = new Set(messages.map((message) => message.name))
  const methods: ResourceRPCMethod[] = []
  let service: string | undefined
  let serviceDepth = 0
  let serviceLines: string[] = []
  for (const line of content.split('\n')) {
    if (!service) {
      const serviceMatch = line.match(/^\s*service\s+(\w+)\s+\{/)
      if (!serviceMatch) {
        continue
      }
      service = serviceMatch[1]
      serviceLines = []
    }

    serviceLines.push(line)
    serviceDepth += (line.match(/\{/g) || []).length
    serviceDepth -= (line.match(/\}/g) || []).length
    if (serviceDepth !== 0) {
      continue
    }

    for (const match of serviceLines
      .join('\n')
      .matchAll(
        /\brpc\s+(\w+)\s*\([^)]*\)\s*returns\s*\(\s*(stream\s+)?(?:\.\w+\.)*(\w+)\s*\)/g,
      )) {
      if (!match[2] && adoptingResponses.has(match[3])) {
        methods.push({
          method: match[1],
          service: `${packageName}.${service}`,
        })
      }
    }
    service = undefined
    serviceLines = []
  }
  return methods
}

// parseResourceIDMessages returns messages with declared resource ID fields.
export function parseResourceIDMessages(content: string): ResourceIDMessage[] {
  const canonicalResponses = new Set(
    Array.from(
      content.matchAll(
        /\brpc\s+\w+\s*\([^)]*\)\s*returns\s*\(\s*(?:stream\s+)?(?:\.\w+\.)*(\w+)\s*\)/g,
      ),
      (match) => match[1],
    ),
  )
  const messages: ResourceIDMessage[] = []
  let message: ResourceIDMessage | undefined
  let marker: 'ids' | 'id' | undefined
  let depth = 0

  for (const line of content.split('\n')) {
    const messageMatch = line.match(/^message\s+(\w+)\s+\{$/)
    if (messageMatch && depth === 0) {
      message = { fields: [], name: messageMatch[1] }
    }

    const trimmed = line.trim()
    if (message && trimmed === resourceIDMarker) {
      if (marker) {
        throw new Error(`${resourceIDMarker} must immediately precede a field`)
      }
      marker = 'id'
    } else if (message && trimmed === resourceIDsMarker) {
      if (marker) {
        throw new Error(`${resourceIDsMarker} must immediately precede a field`)
      }
      marker = 'ids'
    } else if (message) {
      const fieldMatch = line.match(
        /^\s*(?:(repeated)\s+)?uint32\s+(\w+)\s*=\s*\d+/,
      )
      const mapMatch = line.match(
        /^\s*map\s*<\s*uint32\s*,\s*uint32\s*>\s+(\w+)\s*=\s*\d+/,
      )
      const nestedMatch = line.match(/^\s*(?:\.\w+\.)*(\w+)\s+(\w+)\s*=\s*\d+/)
      const canonicalResourceID =
        fieldMatch?.[2] === 'resource_id' &&
        canonicalResponses.has(message.name)
      if (fieldMatch && (marker === 'id' || canonicalResourceID)) {
        message.fields.push({
          getter: `Get${goName(fieldMatch[2])}`,
          kind: fieldMatch[1] ? 'repeated' : 'singular',
        })
        marker = undefined
      } else if (mapMatch && marker === 'id') {
        message.fields.push({
          getter: `Get${goName(mapMatch[1])}`,
          kind: 'map',
        })
        marker = undefined
      } else if (nestedMatch && marker === 'ids') {
        message.fields.push({
          getter: `Get${goName(nestedMatch[2])}`,
          kind: 'nested',
        })
        marker = undefined
      } else if (marker) {
        throw new Error(
          `${marker === 'id' ? resourceIDMarker : resourceIDsMarker} must immediately precede a supported field`,
        )
      }
    }

    depth += (line.match(/\{/g) || []).length
    depth -= (line.match(/\}/g) || []).length
    if (message && depth === 0) {
      if (marker) {
        throw new Error(
          `${marker === 'id' ? resourceIDMarker : resourceIDsMarker} has no field`,
        )
      }
      if (message.fields.length) {
        messages.push(message)
      }
      message = undefined
    }
  }
  return messages
}

// generateResourceIDFile renders Go extraction methods for one proto package.
export function generateResourceIDFile(
  packageName: string,
  messages: ResourceIDMessage[],
): string {
  const lines = [
    '// Code generated by scripts/postprocess-generated.ts. DO NOT EDIT.',
    '',
    `package ${packageName}`,
    '',
  ]
  for (const message of messages) {
    lines.push(
      `// GetAdoptedResourceIds returns Resource IDs whose invocation ownership transfers to the caller.`,
      `func (m *${message.name}) GetAdoptedResourceIds() []uint32 {`,
      '\tif m == nil {',
      '\t\treturn nil',
      '\t}',
      '',
      `\tresourceIDs := make([]uint32, 0, ${message.fields.length})`,
    )
    for (const field of message.fields) {
      if (field.kind === 'singular') {
        lines.push(
          `\tif resourceID := m.${field.getter}(); resourceID != 0 {`,
          '\t\tresourceIDs = append(resourceIDs, resourceID)',
          '\t}',
        )
      } else if (field.kind === 'nested') {
        lines.push(
          `\tresourceIDs = append(resourceIDs, m.${field.getter}().GetAdoptedResourceIds()...)`,
        )
      } else {
        lines.push(
          `\tfor _, resourceID := range m.${field.getter}() {`,
          '\t\tif resourceID != 0 {',
          '\t\t\tresourceIDs = append(resourceIDs, resourceID)',
          '\t\t}',
          '\t}',
        )
      }
    }
    lines.push('\treturn resourceIDs', '}', '')
  }
  return lines.join('\n')
}

// generateResourceRPCMethodFile renders the shared adopting-method predicate.
export function generateResourceRPCMethodFile(
  methods: ResourceRPCMethod[],
): string {
  const lines = [
    '// Code generated by scripts/postprocess-generated.ts. DO NOT EDIT.',
    '',
    'package resource',
    '',
    '// IsResourceRPCAdoptingUnaryMethod reports whether a unary method supports held-receipt adoption.',
    'func IsResourceRPCAdoptingUnaryMethod(serviceID, methodID string) bool {',
    '\tswitch serviceID + "/" + methodID {',
  ]
  for (const method of methods.toSorted((a, b) => {
    const aID = `${a.service}/${a.method}`
    const bID = `${b.service}/${b.method}`
    if (aID < bID) return -1
    if (aID > bID) return 1
    return 0
  })) {
    lines.push(
      `\tcase "${method.service}/${method.method}":`,
      '\t\treturn true',
    )
  }
  lines.push('\t}', '\treturn false', '}', '')
  return lines.join('\n')
}

// generateResourceIDExtractors writes canonical extraction and method metadata.
export async function generateResourceIDExtractors(repoRoot = root) {
  const expected = new Set<string>()
  const adoptingUnaryMethods: ResourceRPCMethod[] = []
  const protoGlob = new Bun.Glob('{bldr,sdk}/**/*.proto')
  const protos: string[] = []
  for await (const path of protoGlob.scan({ cwd: repoRoot })) {
    protos.push(path)
  }
  protos.sort()

  for (const path of protos) {
    const content = await readFile(resolve(repoRoot, path), 'utf-8')
    const messages = parseResourceIDMessages(content)
    adoptingUnaryMethods.push(
      ...parseAdoptingUnaryRPCMethods(content, messages),
    )
    if (!messages.length) {
      continue
    }

    const base = basename(path, '.proto')
    const pbPath = resolve(repoRoot, dirname(path), `${base}.pb.go`)
    const pbContent = await readFile(pbPath, 'utf-8')
    const packageMatch = pbContent.match(/^package\s+(\w+)$/m)
    if (!packageMatch) {
      throw new Error(`missing Go package in ${pbPath}`)
    }

    const outputPath = resolve(
      repoRoot,
      dirname(path),
      `${base}${generatedSuffix}`,
    )
    expected.add(outputPath)
    await writeFile(
      outputPath,
      generateResourceIDFile(packageMatch[1], messages),
    )
    const gofmt = Bun.spawn(['gofmt', '-w', outputPath], {
      stderr: 'pipe',
      stdout: 'pipe',
    })
    const exitCode = await gofmt.exited
    if (exitCode !== 0) {
      throw new Error(
        `gofmt ${relative(repoRoot, outputPath)}: ${await new Response(gofmt.stderr).text()}`,
      )
    }
  }

  const resourceRPCMethodsPath = resolve(repoRoot, resourceRPCMethodsOutput)
  await writeFile(
    resourceRPCMethodsPath,
    generateResourceRPCMethodFile(adoptingUnaryMethods),
  )
  const gofmtResourceRPCMethods = Bun.spawn(
    ['gofmt', '-w', resourceRPCMethodsPath],
    {
      stderr: 'pipe',
      stdout: 'pipe',
    },
  )
  const resourceRPCMethodsExitCode = await gofmtResourceRPCMethods.exited
  if (resourceRPCMethodsExitCode !== 0) {
    throw new Error(
      `gofmt ${resourceRPCMethodsOutput}: ${await new Response(gofmtResourceRPCMethods.stderr).text()}`,
    )
  }

  const resourceRPCMethodsGlob = new Bun.Glob(
    '{bldr,sdk}/**/resource-rpc-methods.pb.go',
  )
  for await (const path of resourceRPCMethodsGlob.scan({ cwd: repoRoot })) {
    const fullPath = resolve(repoRoot, path)
    if (fullPath !== resourceRPCMethodsPath) {
      await unlink(fullPath)
    }
  }

  const generatedGlob = new Bun.Glob(`{bldr,sdk}/**/*${generatedSuffix}`)
  const generated: string[] = []
  for await (const path of generatedGlob.scan({ cwd: repoRoot })) {
    generated.push(path)
  }
  generated.sort()
  for (const path of generated) {
    const fullPath = resolve(repoRoot, path)
    if (!expected.has(fullPath)) {
      await unlink(fullPath)
    }
  }
}

if (import.meta.main) {
  await ensureBuildTag('sdk/layout/layout_srpc.pb.go', '!goscript')
  await generateResourceIDExtractors()
}
