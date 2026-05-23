import * as $ from '@goscript/builtin/index.js'

export function Marshal(_value: unknown): [$.Bytes, $.GoError] {
  return [new Uint8Array(0), null]
}

export function Unmarshal(_data: $.Bytes, _out: unknown): $.GoError {
  return null
}

export function UnmarshalStrict(_data: $.Bytes, _out: unknown): $.GoError {
  return null
}
