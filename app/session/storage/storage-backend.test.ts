import { describe, expect, it } from 'vitest'

import { CheckOutcome } from '@go/github.com/s4wave/spacewave/db/block/store/s3/s3.pb.js'

import {
  buildStorageCorsRule,
  buildStorageLocation,
  describeStorageCheck,
  type StorageLocationFields,
} from './storage-backend.js'

const fields: StorageLocationFields = {
  preset: 'aws',
  bucket: 'my-spaces',
  region: '',
  endpoint: '',
  accountId: '',
  prefix: '',
}

describe('buildStorageLocation', () => {
  it('fills each preset the way storage add s3 does', () => {
    expect(buildStorageLocation(fields).location).toEqual({
      endpoint: 's3.us-east-1.amazonaws.com',
      region: 'us-east-1',
      bucket: 'my-spaces',
      objectPrefix: '',
      disableSsl: false,
    })
    expect(
      buildStorageLocation({ ...fields, preset: 'r2', accountId: 'abc' })
        .location,
    ).toMatchObject({
      endpoint: 'abc.r2.cloudflarestorage.com',
      region: 'auto',
    })
    expect(
      buildStorageLocation({ ...fields, preset: 'b2', region: 'us-west-004' })
        .location,
    ).toMatchObject({ endpoint: 's3.us-west-004.backblazeb2.com' })
    expect(
      buildStorageLocation({ ...fields, preset: 'minio' }).location,
    ).toMatchObject({ endpoint: '127.0.0.1:9000', disableSsl: true })
  })

  it('names the missing field', () => {
    expect(buildStorageLocation({ ...fields, bucket: ' ' }).missing).toBe(
      'the bucket name',
    )
    expect(buildStorageLocation({ ...fields, preset: 'r2' }).missing).toBe(
      'the Cloudflare account ID',
    )
    expect(buildStorageLocation({ ...fields, preset: 'custom' }).missing).toBe(
      'the endpoint',
    )
  })

  it('keeps the host of an endpoint URL and its scheme as disableSsl', () => {
    expect(
      buildStorageLocation({
        ...fields,
        preset: 'custom',
        endpoint: 'http://storage.example.com/',
      }).location,
    ).toMatchObject({ endpoint: 'storage.example.com', disableSsl: true })
  })
})

describe('describeStorageCheck', () => {
  it('suggests CORS for an unreachable bucket only in the browser', () => {
    const result = { outcome: CheckOutcome.UNREACHABLE, detail: 'put: failed' }
    expect(describeStorageCheck(result, true).showCors).toBe(true)
    expect(describeStorageCheck(result, false).showCors).toBe(false)
  })

  it('reports a passing check as connected', () => {
    expect(
      describeStorageCheck({ outcome: CheckOutcome.OK }, true),
    ).toMatchObject({ ok: true, title: 'Connected' })
  })
})

describe('buildStorageCorsRule', () => {
  it('allows the origin and every request the block store sends', () => {
    const rule = JSON.parse(
      buildStorageCorsRule('aws', 'https://app.example').text,
    ) as { AllowedOrigins: string[]; AllowedMethods: string[] }[]
    expect(rule[0].AllowedOrigins).toEqual(['https://app.example'])
    expect(rule[0].AllowedMethods).toEqual(['GET', 'PUT', 'HEAD', 'DELETE'])
  })

  it('uses the B2 operation names and the MinIO server setting', () => {
    expect(buildStorageCorsRule('b2', 'https://a').text).toContain('s3_put')
    expect(buildStorageCorsRule('minio', 'https://a').text).toBe(
      'MINIO_API_CORS_ALLOW_ORIGIN=https://a',
    )
  })
})
