/* eslint-disable */
import Long from 'long'
import * as _m0 from 'protobufjs/minimal'
import { Observable } from 'rxjs'
import { map } from 'rxjs/operators'

export const protobufPackage = 'web.demo'

export interface DemoEchoMsg {
  msg: string
}

function createBaseDemoEchoMsg(): DemoEchoMsg {
  return { msg: '' }
}

export const DemoEchoMsg = {
  encode(
    message: DemoEchoMsg,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.msg !== '') {
      writer.uint32(10).string(message.msg)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): DemoEchoMsg {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseDemoEchoMsg()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.msg = reader.string()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  fromJSON(object: any): DemoEchoMsg {
    return {
      msg: isSet(object.msg) ? String(object.msg) : '',
    }
  },

  toJSON(message: DemoEchoMsg): unknown {
    const obj: any = {}
    message.msg !== undefined && (obj.msg = message.msg)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<DemoEchoMsg>, I>>(
    object: I
  ): DemoEchoMsg {
    const message = createBaseDemoEchoMsg()
    message.msg = object.msg ?? ''
    return message
  },
}

export interface DemoService {
  DemoEcho(request: DemoEchoMsg): Observable<DemoEchoMsg>
}

export class DemoServiceClientImpl implements DemoService {
  private readonly rpc: Rpc
  constructor(rpc: Rpc) {
    this.rpc = rpc
    this.DemoEcho = this.DemoEcho.bind(this)
  }
  DemoEcho(request: DemoEchoMsg): Observable<DemoEchoMsg> {
    const data = DemoEchoMsg.encode(request).finish()
    const result = this.rpc.serverStreamingRequest(
      'web.demo.DemoService',
      'DemoEcho',
      data
    )
    return result.pipe(map((data) => DemoEchoMsg.decode(new _m0.Reader(data))))
  }
}

interface Rpc {
  request(
    service: string,
    method: string,
    data: Uint8Array
  ): Promise<Uint8Array>
  clientStreamingRequest(
    service: string,
    method: string,
    data: Observable<Uint8Array>
  ): Promise<Uint8Array>
  serverStreamingRequest(
    service: string,
    method: string,
    data: Uint8Array
  ): Observable<Uint8Array>
  bidirectionalStreamingRequest(
    service: string,
    method: string,
    data: Observable<Uint8Array>
  ): Observable<Uint8Array>
}

type Builtin =
  | Date
  | Function
  | Uint8Array
  | string
  | number
  | boolean
  | undefined

export type DeepPartial<T> = T extends Builtin
  ? T
  : T extends Long
  ? string | number | Long
  : T extends Array<infer U>
  ? Array<DeepPartial<U>>
  : T extends ReadonlyArray<infer U>
  ? ReadonlyArray<DeepPartial<U>>
  : T extends { $case: string }
  ? { [K in keyof Omit<T, '$case'>]?: DeepPartial<T[K]> } & {
      $case: T['$case']
    }
  : T extends {}
  ? { [K in keyof T]?: DeepPartial<T[K]> }
  : Partial<T>

type KeysOfUnion<T> = T extends T ? keyof T : never
export type Exact<P, I extends P> = P extends Builtin
  ? P
  : P & { [K in keyof P]: Exact<P[K], I[K]> } & Record<
        Exclude<keyof I, KeysOfUnion<P>>,
        never
      >

if (_m0.util.Long !== Long) {
  _m0.util.Long = Long as any
  _m0.configure()
}

function isSet(value: any): boolean {
  return value !== null && value !== undefined
}
