import * as $ from '@goscript/builtin/index.js'

const errYAMLUnsupported = $.newError(
  'gopkg.in/yaml.v2: yaml marshal/unmarshal is not supported in goscript',
)

export function Marshal(_value: unknown): [$.Bytes, $.GoError] {
  return [new Uint8Array(0), errYAMLUnsupported]
}

export function Unmarshal(_data: $.Bytes, _out: unknown): $.GoError {
  return errYAMLUnsupported
}

export function UnmarshalStrict(_data: $.Bytes, _out: unknown): $.GoError {
  return errYAMLUnsupported
}
