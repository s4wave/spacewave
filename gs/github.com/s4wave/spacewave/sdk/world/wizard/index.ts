import * as $ from "@goscript/builtin/index.js";

export const CreateWizardObjectOpId = "spacewave/wizard/create";

class UnsupportedOperationError {
  Error(): string {
    return "spacewave/wizard/create: project override does not implement operation application";
  }
}

class CreateWizardObjectOperation {
  GetOperationTypeId(): string {
    return CreateWizardObjectOpId;
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

export function LookupCreateWizardObjectOp(
  _ctx: unknown,
  operationTypeID: string,
): [unknown, $.GoError] {
  if (operationTypeID === CreateWizardObjectOpId) {
    return [new CreateWizardObjectOperation(), null];
  }
  return [null, null];
}
