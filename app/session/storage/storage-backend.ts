import type { S3Location } from '@s4wave/core/account/settings/settings.pb.js'
import {
  MoveSpaceStoragePhase,
  type MoveSpaceStorageResponse,
} from '@s4wave/sdk/session/session.pb.js'
import { formatBytes, plural } from '@s4wave/app/system/format.js'
import {
  CheckOutcome,
  type CheckResult,
} from '@go/github.com/s4wave/spacewave/db/block/store/s3/s3.pb.js'

// accountStorageChoice is the new-Space storage choice for the account's own
// storage, in place of a storage backend id.
export const accountStorageChoice = 'account'

// StoragePreset names a provider whose endpoint pattern fills the location.
export type StoragePreset = 'aws' | 'r2' | 'b2' | 'minio' | 'custom'

// StoragePresetInfo describes a preset in the add storage form.
export interface StoragePresetInfo {
  // id is the preset, matching storage add s3 --preset.
  id: StoragePreset
  // label is the provider name.
  label: string
  // regionHint is the region field placeholder, empty when the preset has no region field.
  regionHint: string
  // needsAccountId is true when the endpoint needs the provider account id.
  needsAccountId: boolean
}

// storagePresets lists the presets in the order the form shows them.
export const storagePresets: StoragePresetInfo[] = [
  {
    id: 'aws',
    label: 'Amazon S3',
    regionHint: 'us-east-1',
    needsAccountId: false,
  },
  { id: 'r2', label: 'Cloudflare R2', regionHint: '', needsAccountId: true },
  {
    id: 'b2',
    label: 'Backblaze B2',
    regionHint: 'us-west-004',
    needsAccountId: false,
  },
  {
    id: 'minio',
    label: 'MinIO',
    regionHint: 'us-east-1',
    needsAccountId: false,
  },
  {
    id: 'custom',
    label: 'Other S3-compatible',
    regionHint: 'us-east-1',
    needsAccountId: false,
  },
]

// StorageLocationFields are the add storage form's location inputs.
export interface StorageLocationFields {
  preset: StoragePreset
  bucket: string
  region: string
  endpoint: string
  accountId: string
  prefix: string
}

// StorageLocationResult is a built location, or the field a person still needs to fill.
export type StorageLocationResult =
  | { location: S3Location; missing?: undefined }
  | { location?: undefined; missing: string }

// buildStorageLocation fills the location from the preset the same way
// storage add s3 does. An endpoint entered by hand overrides the preset's.
export function buildStorageLocation(
  fields: StorageLocationFields,
): StorageLocationResult {
  const bucket = fields.bucket.trim()
  if (!bucket) {
    return { missing: 'the bucket name' }
  }

  // Fill the endpoint and region the preset implies.
  let endpoint = fields.endpoint.trim()
  let region = fields.region.trim()
  let disableSsl = false
  switch (fields.preset) {
    case 'aws':
      region ||= 'us-east-1'
      endpoint ||= `s3.${region}.amazonaws.com`
      break
    case 'r2':
      region = 'auto'
      if (!endpoint) {
        const accountId = fields.accountId.trim()
        if (!accountId) {
          return { missing: 'the Cloudflare account ID' }
        }
        endpoint = `${accountId}.r2.cloudflarestorage.com`
      }
      break
    case 'b2':
      if (!endpoint) {
        if (!region) {
          return { missing: 'the region, such as us-west-004' }
        }
        endpoint = `s3.${region}.backblazeb2.com`
      }
      break
    case 'minio':
      endpoint ||= '127.0.0.1:9000'
      disableSsl = true
      break
    case 'custom':
      if (!endpoint) {
        return { missing: 'the endpoint' }
      }
      break
  }

  // Keep the host alone: the scheme comes from disableSsl.
  if (endpoint.startsWith('http://')) {
    disableSsl = true
  }
  endpoint = endpoint.replace(/^https?:\/\//, '').replace(/\/+$/, '')
  return {
    location: {
      endpoint,
      region: region || 'us-east-1',
      bucket,
      objectPrefix: fields.prefix.trim(),
      disableSsl,
    },
  }
}

// StorageCheckView is a check result worded for a person.
export interface StorageCheckView {
  // ok is true when the bucket accepted a write, read, and delete.
  ok: boolean
  // title states the result.
  title: string
  // action tells the person what to change, empty when ok.
  action: string
  // showCors is true when a missing CORS rule may explain the result.
  showCors: boolean
  // detail is the failing step and the server's message.
  detail: string
  // usage is what the bucket holds under the prefix, empty when the check
  // failed or the listing did not complete.
  usage: string
  // usageDetail is why the usage is unknown, empty when it is known.
  usageDetail: string
}

// describeStorageCheck words a check result as an actionable row. inBrowser
// is true when the check ran from a browser, where a bucket without a CORS
// rule for the app's origin looks unreachable.
export function describeStorageCheck(
  result: CheckResult | undefined,
  inBrowser: boolean,
): StorageCheckView {
  const detail = result?.detail ?? ''
  const view = (title: string, action: string, showCors = false) => ({
    ok: false,
    title,
    action,
    showCors,
    detail,
    usage: '',
    usageDetail: '',
  })
  switch (result?.outcome) {
    case CheckOutcome.OK:
      return {
        ok: true,
        title: 'Connected',
        action: '',
        showCors: false,
        detail,
        usage: result.usage
          ? `${formatBytes(result.usage.bytes)} in ${plural(Number(result.usage.objects ?? 0n), 'object')}`
          : '',
        usageDetail: result.usage ? '' : (result.usageError ?? ''),
      }
    case CheckOutcome.UNREACHABLE:
      return inBrowser
        ? view(
            'Cannot reach the bucket',
            'Add the CORS rule below to the bucket, then check again. If it is already there, check the endpoint.',
            true,
          )
        : view(
            'Cannot reach the endpoint',
            'Check the endpoint and your connection.',
          )
    case CheckOutcome.CREDENTIALS_REJECTED:
      return view(
        'The endpoint rejected the access key',
        'Check the access key ID and secret, or create a new key.',
      )
    case CheckOutcome.ACCESS_DENIED:
      return view(
        'The access key cannot write to this bucket',
        'Give the key permission to read, write, and delete objects in the bucket.',
      )
    case CheckOutcome.BUCKET_NOT_FOUND:
      return view(
        'The bucket does not exist',
        'Create the bucket with your provider, or fix its name.',
      )
    case CheckOutcome.WRONG_REGION:
      return view(
        'The bucket is in a different region',
        'Set the region the bucket was created in.',
      )
    default:
      return view('The check failed', 'Read the details, then check again.')
  }
}

// corsMethods are the requests the block store sends to the bucket.
const corsMethods = ['GET', 'PUT', 'HEAD', 'DELETE']

// corsHeaders are the request headers the block store signs and sends.
const corsHeaders = [
  'authorization',
  'content-type',
  'x-amz-content-sha256',
  'x-amz-date',
  'x-amz-security-token',
]

// StorageCorsRule is the CORS configuration a preset's bucket needs.
export interface StorageCorsRule {
  // where says where to paste the rule.
  where: string
  // text is the rule to paste.
  text: string
}

// buildStorageCorsRule returns the rule that lets origin reach the bucket,
// in the format the preset's provider accepts.
export function buildStorageCorsRule(
  preset: StoragePreset,
  origin: string,
): StorageCorsRule {
  switch (preset) {
    case 'b2':
      return {
        where: 'Set it with the B2 CLI: b2 bucket update --cors-rules',
        text: JSON.stringify(
          [
            {
              corsRuleName: 'spacewave',
              allowedOrigins: [origin],
              allowedOperations: ['s3_get', 's3_put', 's3_head', 's3_delete'],
              allowedHeaders: corsHeaders,
              exposeHeaders: ['etag'],
              maxAgeSeconds: 3600,
            },
          ],
          null,
          2,
        ),
      }
    case 'minio':
      return {
        where: 'MinIO sets CORS for the whole server. Start it with:',
        text: `MINIO_API_CORS_ALLOW_ORIGIN=${origin}`,
      }
    default:
      return {
        where:
          preset === 'r2'
            ? 'Paste it in the bucket settings under CORS Policy.'
            : 'Paste it in the bucket permissions under Cross-origin resource sharing.',
        text: JSON.stringify(
          [
            {
              AllowedOrigins: [origin],
              AllowedMethods: corsMethods,
              AllowedHeaders: corsHeaders,
              ExposeHeaders: ['ETag'],
              MaxAgeSeconds: 3600,
            },
          ],
          null,
          2,
        ),
      }
  }
}

// guessStoragePreset infers the preset a saved location was added with.
export function guessStoragePreset(
  location: S3Location | undefined,
): StoragePreset {
  const endpoint = location?.endpoint ?? ''
  if (endpoint.endsWith('.amazonaws.com')) return 'aws'
  if (endpoint.endsWith('.r2.cloudflarestorage.com')) return 'r2'
  if (endpoint.endsWith('.backblazeb2.com')) return 'b2'
  if (location?.disableSsl) return 'minio'
  return 'custom'
}

// formatStorageLocation describes where a backend's bucket is.
export function formatStorageLocation(
  location: S3Location | undefined,
): string {
  const bucket = location?.bucket ?? ''
  const prefix = location?.objectPrefix ? `/${location.objectPrefix}` : ''
  return `${bucket}${prefix} on ${location?.endpoint ?? ''}`
}

// describeMoveProgress words one step of a storage move to destination.
export function describeMoveProgress(
  progress: MoveSpaceStorageResponse,
  destination: string,
): string {
  switch (progress.phase) {
    case MoveSpaceStoragePhase.MoveSpaceStoragePhase_FETCH:
      return `Copying blocks from the old bucket: ${Number(progress.blocksFetched ?? 0n)} of ${Number(progress.blocksTotal ?? 0n)}`
    case MoveSpaceStoragePhase.MoveSpaceStoragePhase_UPLOAD:
      return `Uploading to ${destination}: ${plural(Number(progress.pendingBlocks ?? 0n), 'block')} (${formatBytes(progress.pendingBytes)}) left`
    case MoveSpaceStoragePhase.MoveSpaceStoragePhase_DONE:
      return `Stored in ${destination}`
    default:
      return 'Starting the move'
  }
}
