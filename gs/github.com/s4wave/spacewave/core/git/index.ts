import * as $ from "@goscript/builtin/index.js";

export const CreateGitRepoWizardOpId = "spacewave/git/repo/create";

class UnsupportedOperationError {
  Error(): string {
    return "spacewave/git/repo/create: project override does not implement operation application";
  }
}

class CreateGitRepoWizardOperation {
  GetOperationTypeId(): string {
    return CreateGitRepoWizardOpId;
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

export function LookupCreateGitRepoWizardOp(
  _ctx: unknown,
  operationTypeID: string,
): [unknown, $.GoError] {
  if (operationTypeID === CreateGitRepoWizardOpId) {
    return [new CreateGitRepoWizardOperation(), null];
  }
  return [null, null];
}
