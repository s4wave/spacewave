import { afterEach, describe, expect, test } from 'bun:test'
import { mkdir, mkdtemp, readFile, rm, writeFile } from 'fs/promises'
import { resolve } from 'path'

import {
  generateResourceIDExtractors,
  generateResourceIDFile,
  generateResourceRPCMethodFile,
  parseAdoptingUnaryRPCMethods,
  parseResourceIDMessages,
} from './postprocess-generated.js'

const roots: string[] = []

afterEach(async () => {
  await Promise.all(
    roots.splice(0).map((root) => rm(root, { force: true, recursive: true })),
  )
})

async function newRepo(): Promise<string> {
  const base = resolve(import.meta.dirname, '..', '.tmp')
  await mkdir(base, { recursive: true })
  const root = await mkdtemp(resolve(base, 'resource-id-generator-'))
  roots.push(root)
  return root
}

describe('resource adoption ID generation', () => {
  test('binds markers to the immediately following supported field', () => {
    const messages = parseResourceIDMessages(`message Result {
  // @resource-adoption-id
  uint32 primary_id = 1;
  // @resource-adoption-id
  repeated uint32 child_ids = 2;
  // @resource-adoption-id
  map<uint32, uint32> refs = 3;
  // @resource-adoption-ids
  Child nested = 4;
}`)

    expect(messages).toEqual([
      {
        name: 'Result',
        fields: [
          { getter: 'GetPrimaryId', kind: 'singular' },
          { getter: 'GetChildIds', kind: 'repeated' },
          { getter: 'GetRefs', kind: 'map' },
          { getter: 'GetNested', kind: 'nested' },
        ],
      },
    ])
  })

  test('rejects unsupported or separated marker fields', () => {
    expect(() =>
      parseResourceIDMessages(`message Result {
  // @resource-adoption-id
  string resource_id = 1;
}`),
    ).toThrow('must immediately precede a supported field')

    expect(() =>
      parseResourceIDMessages(`message Result {
  // @resource-adoption-id
  // intervening comment
  uint32 resource_id = 1;
}`),
    ).toThrow('must immediately precede a supported field')
  })

  test('does not infer adoption from request or event resource IDs', () => {
    const messages = parseResourceIDMessages(`service Things {
  rpc Create(CreateRequest) returns (CreateResponse);
}
message CreateRequest {
  uint32 resource_id = 1;
}
message ResourceReleasedResponse {
  uint32 resource_id = 1;
}
message CreateResponse {
  uint32 resource_id = 1;
}`)

    expect(messages).toEqual([
      {
        name: 'CreateResponse',
        fields: [{ getter: 'GetResourceId', kind: 'singular' }],
      },
    ])
  })

  test('positive-lists only unary methods with generated response extraction', () => {
    const methods = parseAdoptingUnaryRPCMethods(`package fixture.rpc;
service Things {
  rpc Create(CreateRequest) returns (CreateResponse);
  rpc Watch(WatchRequest)
    returns (stream CreateResponse);
  rpc Inspect(InspectRequest) returns (InspectResponse);
}
message CreateResponse {
  uint32 resource_id = 1;
}
message InspectResponse {
  string name = 1;
}`)

    expect(methods).toEqual([
      {
        method: 'Create',
        service: 'fixture.rpc.Things',
      },
    ])
    const output = generateResourceRPCMethodFile(methods)
    expect(output).toContain('case "fixture.rpc.Things/Create":')
    expect(output).not.toContain('fixture.rpc.Things/Watch')
    expect(output).not.toContain('fixture.rpc.Things/Inspect')
    const sorted = generateResourceRPCMethodFile([
      ...methods,
      { method: 'Access', service: 'fixture.rpc.Things' },
    ])
    expect(sorted.indexOf('/Access')).toBeLessThan(sorted.indexOf('/Create'))
  })

  test('emits nil-safe multi-field and nested extraction', () => {
    const output = generateResourceIDFile('fixture', [
      {
        name: 'Child',
        fields: [{ getter: 'GetResourceId', kind: 'singular' }],
      },
      {
        name: 'Result',
        fields: [
          { getter: 'GetTransactionResourceId', kind: 'singular' },
          { getter: 'GetCursorResourceIds', kind: 'repeated' },
          { getter: 'GetChild', kind: 'nested' },
        ],
      },
    ])

    expect(output).toContain(
      'func (m *Result) GetAdoptedResourceIds() []uint32 {\n\tif m == nil {',
    )
    expect(output).toContain(
      'if resourceID := m.GetTransactionResourceId(); resourceID != 0 {',
    )
    expect(output).toContain(
      'for _, resourceID := range m.GetCursorResourceIds() {\n\t\tif resourceID != 0 {',
    )
    expect(output).toContain('m.GetChild().GetAdoptedResourceIds()...')
  })

  test('formats output, deletes stale files, and is deterministic', async () => {
    const root = await newRepo()
    const fixture = resolve(root, 'bldr', 'fixture')
    await mkdir(fixture, { recursive: true })
    await mkdir(resolve(root, 'bldr', 'resource'), {
      recursive: true,
    })
    await writeFile(
      resolve(fixture, 'fixture.proto'),
      `package fixture;
service FixtureService {
  rpc Create(CreateRequest) returns (Result);
  rpc Watch(WatchRequest) returns (stream Result);
}
message Result {
  // @resource-adoption-id
  repeated uint32 resource_ids = 1;
}\n`,
    )
    await writeFile(resolve(fixture, 'fixture.pb.go'), 'package fixture\n')
    const stale = resolve(fixture, 'stale_resource_ids.pb.go')
    await writeFile(stale, 'package fixture\n')
    const staleMethods = resolve(fixture, 'resource-rpc-methods.pb.go')
    await writeFile(staleMethods, 'package fixture\n')

    await generateResourceIDExtractors(root)
    const outputPath = resolve(fixture, 'fixture_resource_ids.pb.go')
    const first = await readFile(outputPath, 'utf-8')
    await expect(readFile(stale, 'utf-8')).rejects.toThrow()
    await expect(readFile(staleMethods, 'utf-8')).rejects.toThrow()
    expect(first).toContain('resourceIDs := make([]uint32, 0, 1)')
    const resourceRPCMethods = await readFile(
      resolve(root, 'bldr', 'resource', 'resource-rpc-methods.pb.go'),
      'utf-8',
    )
    expect(resourceRPCMethods).toContain(
      'case "fixture.FixtureService/Create":',
    )
    expect(resourceRPCMethods).not.toContain('fixture.FixtureService/Watch')
    expect(first.endsWith('\n')).toBe(true)

    await generateResourceIDExtractors(root)
    expect(await readFile(outputPath, 'utf-8')).toBe(first)
  })
})
