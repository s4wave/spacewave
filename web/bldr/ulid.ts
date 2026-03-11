import { ulid as generateULID, isValid, decodeTime } from 'ulid'

// ENCODED_SIZE is the encoded length of a ULID.
const ENCODED_SIZE = 26

// MIN_TIMESTAMP is the minimum valid ULID timestamp (Nov 2009).
const MIN_TIMESTAMP = 1257893000000

// ErrInvalidULID is returned if the ULID is in an invalid format.
const ErrInvalidULID = new Error('invalid ulid')

// newULID generates a new randomized ULID in lowercase.
export function newULID(): string {
  return generateULID().toLowerCase()
}

// parseULID parses and validates a lowercase ULID.
// Rejects non-lowercase, invalid format, and timestamps before Nov 2009.
export function parseULID(id: string): string {
  if (id.length !== ENCODED_SIZE) {
    throw ErrInvalidULID
  }
  if (id !== id.toLowerCase()) {
    throw ErrInvalidULID
  }
  if (!isValid(id.toUpperCase())) {
    throw ErrInvalidULID
  }
  const ts = decodeTime(id.toUpperCase())
  if (ts < MIN_TIMESTAMP) {
    throw ErrInvalidULID
  }
  return id
}
