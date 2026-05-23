import * as $ from "@goscript/builtin/index.js";

export const ForgeTaskCreateOpId = "spacewave/forge/task/create";

class UnsupportedOperationError {
  Error(): string {
    return "spacewave/forge/task/create: project override does not implement operation application";
  }
}

class ForgeTaskCreateOperation {
  GetOperationTypeId(): string {
    return ForgeTaskCreateOpId;
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

export function LookupForgeTaskCreateOp(
  _ctx: unknown,
  operationTypeID: string,
): [unknown, $.GoError] {
  if (operationTypeID === ForgeTaskCreateOpId) {
    return [new ForgeTaskCreateOperation(), null];
  }
  return [null, null];
}
