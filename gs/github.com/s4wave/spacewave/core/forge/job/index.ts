import * as $ from "@goscript/builtin/index.js";

export const ForgeJobCreateOpId = "spacewave/forge/job/create";

class UnsupportedOperationError {
  Error(): string {
    return "spacewave/forge/job/create: project override does not implement operation application";
  }
}

class ForgeJobCreateOperation {
  GetOperationTypeId(): string {
    return ForgeJobCreateOpId;
  }

  Validate(): $.GoError {
    return null;
  }

  MarshalBlock(): [$.Bytes, $.GoError] {
    return [new Uint8Array(0), new UnsupportedOperationError()];
  }

  UnmarshalBlock(_data: $.Bytes): $.GoError {
    return new UnsupportedOperationError();
  }

  ApplyWorldOp(): [boolean, $.GoError] {
    return [false, new UnsupportedOperationError()];
  }

  ApplyWorldObjectOp(): [boolean, $.GoError] {
    return [false, new UnsupportedOperationError()];
  }
}

export function LookupForgeJobCreateOp(
  _ctx: unknown,
  operationTypeID: string,
): [unknown, $.GoError] {
  if (operationTypeID === ForgeJobCreateOpId) {
    return [new ForgeJobCreateOperation(), null];
  }
  return [null, null];
}
