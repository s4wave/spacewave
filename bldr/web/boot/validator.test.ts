import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { describe, expect, it, vi } from 'vitest'

import {
  BootReport,
  BootValidation,
  BootValidationViolationKind,
} from './report.pb.js'
import { marshalBootReportJson } from './codec.js'
import { validateBootReport } from './validator.js'

function loadBootReportFixture(name: string) {
  return BootReport.fromJsonString(
    readFileSync(
      resolve(process.cwd(), `bldr/web/boot/testdata/${name}.json`),
      'utf8',
    ),
  )
}

function cloneBootReportFixture(name: string) {
  const report = BootReport.clone(loadBootReportFixture(name))
  if (!report) throw new Error(`failed to clone ${name} fixture`)
  return report
}

describe('validateBootReport', () => {
  const tests = [
    { name: 'successful', pass: true, kinds: [] },
    { name: 'failed', pass: true, kinds: [] },
    { name: 'aborted', pass: true, kinds: [] },
    { name: 'concurrent', pass: true, kinds: [] },
    {
      name: 'reordered',
      pass: false,
      kinds: [BootValidationViolationKind.MARK_ORDER],
    },
    {
      name: 'stalled',
      pass: false,
      kinds: [
        BootValidationViolationKind.GAP_COVERAGE,
        BootValidationViolationKind.GENERIC_LEAF,
        BootValidationViolationKind.UNKNOWN_SHARE,
      ],
    },
    {
      name: 'actionable-ancestor',
      pass: false,
      kinds: [
        BootValidationViolationKind.GENERIC_LEAF,
        BootValidationViolationKind.GENERIC_LEAF,
        BootValidationViolationKind.UNKNOWN_SHARE,
      ],
    },
    {
      name: 'duplicate-span',
      pass: false,
      kinds: [BootValidationViolationKind.SPAN_CONTRACT],
    },
    {
      name: 'deep-span',
      pass: false,
      kinds: [BootValidationViolationKind.SPAN_CONTRACT],
    },
    {
      name: 'post-terminal',
      pass: false,
      kinds: [BootValidationViolationKind.TERMINAL_CONTRACT],
    },
    {
      name: 'invalid-privacy',
      pass: false,
      kinds: [BootValidationViolationKind.TERMINAL_CONTRACT],
    },
    {
      name: 'invalid-accounting',
      pass: false,
      kinds: [BootValidationViolationKind.REPORT_CONTRACT],
    },
    {
      name: 'nonfinite-scalar',
      pass: false,
      kinds: [BootValidationViolationKind.REPORT_CONTRACT],
    },
    {
      name: 'oversized-record',
      pass: false,
      kinds: [BootValidationViolationKind.REPORT_CONTRACT],
    },
  ] as const

  for (const test of tests) {
    it(`validates the ${test.name} golden report`, () => {
      const report = loadBootReportFixture(test.name)
      const validation = validateBootReport(report)
      expect(BootValidation.toJsonString(validation)).toBe(
        BootValidation.toJsonString(report.validation ?? {}),
      )
      expect(validation.pass).toBe(test.pass)
      expect(validation.violations?.map((entry) => entry.kind)).toEqual(
        test.kinds,
      )
    })
  }

  it('rejects multiple usable marks', () => {
    const report = cloneBootReportFixture('successful')
    const marks = report.marks ?? []
    marks[1].label = report.usableMark
    const validation = validateBootReport(report)
    expect(validation.violations?.map((entry) => entry.kind)).toContain(
      BootValidationViolationKind.TERMINAL_CONTRACT,
    )
  })

  it('rejects the shared forbidden privacy vocabulary', () => {
    const values = readFileSync(
      resolve(process.cwd(), 'bldr/web/boot/testdata/privacy-vectors.txt'),
      'utf8',
    )
      .trim()
      .split('\n')
    for (const value of values) {
      const report = cloneBootReportFixture('failed')
      report.terminalErrorCode = value
      const validation = validateBootReport(report)
      expect(validation.violations?.map((entry) => entry.kind)).toContain(
        BootValidationViolationKind.TERMINAL_CONTRACT,
      )
    }
  })

  it('rejects collection and ordered-detail bounds', () => {
    const oversized = cloneBootReportFixture('successful')
    oversized.marks = Array.from({ length: 4097 }, () => ({}))
    expect(
      validateBootReport(oversized).violations?.map((entry) => entry.kind),
    ).toContain(BootValidationViolationKind.REPORT_CONTRACT)

    const unordered = cloneBootReportFixture('successful')
    const detail = unordered.marks?.[0]?.detail
    if (detail) detail.reverse()
    expect(
      validateBootReport(unordered).violations?.map((entry) => entry.kind),
    ).toContain(BootValidationViolationKind.REPORT_CONTRACT)
  })

  it('round trips generated binary and proto JSON', () => {
    const report = loadBootReportFixture('successful')
    const binary = BootReport.toBinary(report)
    const binaryDecoded = BootReport.fromBinary(binary)
    expect(BootReport.toBinary(binaryDecoded)).toEqual(binary)
    const json = BootReport.toJsonString(binaryDecoded)
    const jsonDecoded = BootReport.fromJsonString(json)
    expect(BootReport.toJsonString(jsonDecoded)).toBe(json)
  })

  it('matches the repeated Go and TypeScript canonical proto JSON golden', () => {
    const report = loadBootReportFixture('successful')
    const golden = readFileSync(
      resolve(
        process.cwd(),
        'bldr/web/boot/testdata/successful.canonical.json',
      ),
      'utf8',
    )
    expect(BootReport.toBinary(BootReport.fromJsonString(golden))).toEqual(
      BootReport.toBinary(report),
    )
    const compact = JSON.stringify(JSON.parse(golden))
    for (let attempt = 0; attempt < 20; attempt++) {
      expect(marshalBootReportJson(report)).toBe(compact)
    }
  })

  it('applies the shared field-specific vocabulary vectors', () => {
    const vectors = readFileSync(
      resolve(process.cwd(), 'bldr/web/boot/testdata/vocabulary-vectors.txt'),
      'utf8',
    )
      .trim()
      .split('\n')
    const kinds = {
      REPORT_CONTRACT: BootValidationViolationKind.REPORT_CONTRACT,
      SPAN_CONTRACT: BootValidationViolationKind.SPAN_CONTRACT,
    } as const
    for (const vector of vectors) {
      const [disposition, target, value, expected] = vector.split('|')
      const report = cloneBootReportFixture('successful')
      if (target === 'report-id') report.reportId = value
      else if (target === 'mark-label') report.marks![1].label = value
      else if (target === 'detail-string') {
        report.marks![0].detail![1].value = {
          value: { case: 'stringValue', value },
        }
      } else if (target === 'operation') report.spans![1].operation = value
      else if (target === 'browser-engine') report.environment!.browserEngine = value
      else if (target === 'os-family') report.environment!.osFamily = value
      else if (target === 'worker-mode') report.build!.workerMode = Number(value)
      else throw new Error(`unknown target ${target}`)
      const validation = validateBootReport(report)
      if (disposition === 'accept') expect(validation.pass).toBe(true)
      else {
        expect(validation.violations?.map((entry) => entry.kind)).toContain(
          kinds[expected as keyof typeof kinds],
        )
      }
    }
  })

  it('rejects every shared unsupported enum vector after binary decoding', () => {
    const vectors = readFileSync(
      resolve(process.cwd(), 'bldr/web/boot/testdata/unsupported-enums.txt'),
      'utf8',
    )
      .trim()
      .split('\n')
    const kinds = {
      REPORT_CONTRACT: BootValidationViolationKind.REPORT_CONTRACT,
      TERMINAL_CONTRACT: BootValidationViolationKind.TERMINAL_CONTRACT,
      MARK_ORDER: BootValidationViolationKind.MARK_ORDER,
      SPAN_CONTRACT: BootValidationViolationKind.SPAN_CONTRACT,
    } as const
    for (const vector of vectors) {
      const [target, rawValue, expected] = vector.split('|')
      const value = Number(rawValue)
      const report = cloneBootReportFixture('successful')
      switch (target) {
        case 'report-state':
          report.state = value
          break
        case 'build-type':
          report.build!.buildType = value
          break
        case 'runtime-kind':
          report.build!.runtimeKind = value
          break
        case 'worker-mode':
          report.build!.workerMode = value
          break
        case 'environment-class':
          report.environment!.class = value
          break
        case 'service-worker-state':
          report.environment!.serviceWorkerState = value
          break
        case 'cache-state':
          report.environment!.cacheState = value
          break
        case 'recovery-decision':
          report.environment!.recoveryDecision = value
          break
        case 'mark-phase':
          report.marks![0].phase = value
          break
        case 'counter-unit':
          report.accounting!.samples![0].unit = value
          break
        case 'attachment-kind':
          report.attachments = [
            {
              artifactId: 'boot-artifact-fixture',
              kind: value,
              contentHash: 'a'.repeat(64),
              sizeBytes: 1n,
              releaseGeneration: 'fixture',
            },
          ]
          break
        case 'share-destination':
          report.privacy!.sharedUnixMicros = 1n
          report.privacy!.shareDestination = value
          break
        case 'span-result':
          report.spans![0].result = value
          break
        case 'span-work-class':
          report.spans![0].workClass = value
          break
        default:
          throw new Error(`unknown enum target ${target}`)
      }
      const decoded = BootReport.fromBinary(BootReport.toBinary(report))
      expect(
        validateBootReport(decoded).violations?.map((entry) => entry.kind),
      ).toContain(kinds[expected as keyof typeof kinds])
    }
  })

  it('returns before traversing an oversized graph', () => {
    const report = cloneBootReportFixture('successful')
    report.spans = new Proxy(
      Array.from({ length: 4097 }, () => ({})),
      {
        get(target, property, receiver) {
          if (property === Symbol.iterator) {
            throw new Error('oversized spans were traversed')
          }
          return Reflect.get(target, property, receiver)
        },
      },
    )
    expect(
      validateBootReport(report).violations?.map((entry) => entry.kind),
    ).toEqual([BootValidationViolationKind.REPORT_CONTRACT])
  })

  it('validates without materializing the complete protobuf', () => {
    const toBinary = vi.spyOn(BootReport, 'toBinary').mockImplementation(() => {
      throw new Error('complete protobuf was materialized')
    })
    try {
      expect(validateBootReport(loadBootReportFixture('successful')).pass).toBe(
        true,
      )
      const oversized = cloneBootReportFixture('successful')
      oversized.entrypointId = 'a'.repeat(129)
      expect(
        validateBootReport(oversized).violations?.map((entry) => entry.kind),
      ).toContain(BootValidationViolationKind.REPORT_CONTRACT)
    } finally {
      toBinary.mockRestore()
    }
  })
})

describe('review regressions', () => {
  function expectReportContract(report: BootReport) {
    expect(
      validateBootReport(report).violations?.map((entry) => entry.kind),
    ).toContain(BootValidationViolationKind.REPORT_CONTRACT)
  }

  it('rejects oversized reports before invalid vocabulary returns', () => {
    const report = cloneBootReportFixture('successful')
    report.terminalErrorCode = 'x'.repeat(4 << 20)
    expectReportContract(report)
  })

  it('rejects out-of-range signed and unsigned scalars', () => {
    for (const mutate of [
      (report: BootReport) => {
        report.marks![0].detail![0].value = {
          value: { case: 'unsignedValue', value: -1n },
        }
      },
      (report: BootReport) => {
        report.marks![0].detail![0].value = {
          value: { case: 'signedValue', value: 1n << 63n },
        }
      },
      (report: BootReport) => {
        report.accounting!.samples![0].monotonicMicros = -1n
      },
    ]) {
      const report = cloneBootReportFixture('successful')
      mutate(report)
      expectReportContract(report)
    }
  })

  it('enforces the shared timestamp int64 boundary', () => {
    const oversized = cloneBootReportFixture('successful')
    oversized.privacy!.sharedUnixMicros = 1n << 63n
    oversized.privacy!.shareDestination = 1
    expectReportContract(oversized)

    const maximum = cloneBootReportFixture('successful')
    maximum.privacy!.sharedUnixMicros = (1n << 63n) - 1n
    maximum.privacy!.shareDestination = 1
    expect(validateBootReport(maximum).pass).toBe(true)
  })

  it('accepts numeric identifier suffixes', () => {
    const report = cloneBootReportFixture('successful')
    report.reportId = 'boot-report-1'
    report.spans![0].spanId = 'span-1'
    for (const span of report.spans!.slice(1)) {
      if (span.parentSpanId === 'runtime') span.parentSpanId = 'span-1'
    }
    report.attachments = [
      {
        artifactId: 'boot-artifact-1',
        kind: 1,
        contentHash: 'a'.repeat(64),
        sizeBytes: 1n,
        releaseGeneration: report.build!.releaseGeneration,
      },
    ]
    expect(validateBootReport(report).pass).toBe(true)
  })

  it('ignores stale stored validation enum metadata', () => {
    const report = cloneBootReportFixture('successful')
    report.validation!.violations = [
      { kind: 99 as BootValidationViolationKind },
    ]
    expect(validateBootReport(report).pass).toBe(true)
  })

  it('rejects failed and aborted reports containing the usable mark', () => {
    for (const name of ['failed', 'aborted']) {
      const report = cloneBootReportFixture(name)
      report.marks![0].label = report.usableMark
      expect(
        validateBootReport(report).violations?.map((entry) => entry.kind),
      ).toContain(BootValidationViolationKind.TERMINAL_CONTRACT)
    }
  })

  it('requires accounting samples at exact mark boundaries', () => {
    const report = cloneBootReportFixture('successful')
    report.accounting!.samples![0].monotonicMicros = 1n
    expectReportContract(report)
  })

  it('requires attachment and report release generations to match', () => {
    const report = cloneBootReportFixture('successful')
    report.attachments = [
      {
        artifactId: 'boot-artifact-1',
        kind: 1,
        contentHash: 'a'.repeat(64),
        sizeBytes: 1n,
        releaseGeneration: 'other',
      },
    ]
    expectReportContract(report)
  })
})
