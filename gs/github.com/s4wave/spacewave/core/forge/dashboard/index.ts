import * as $ from "@goscript/builtin/index.js";

export const CreateForgeDashboardOpId = "spacewave/forge/dashboard/create";
export const LinkForgeDashboardOpId = "spacewave/forge/dashboard/link";
export const InitForgeQuickstartOpId = "spacewave/forge/quickstart/init";
export const ForgeDashboardTypeID = "spacewave/forge/dashboard";

class UnsupportedOperationError {
  constructor(private readonly operation: string) {}

  Error(): string {
    return `${this.operation}: project override does not implement operation application`;
  }
}

class ForgeDashboardOperation {
  constructor(private readonly operationTypeID: string) {}

  GetOperationTypeId(): string {
    return this.operationTypeID;
  }

  Validate(): $.GoError {
    return null;
  }

  MarshalBlock(): [$.Bytes, $.GoError] {
    return [
      new Uint8Array(0),
      new UnsupportedOperationError(this.operationTypeID),
    ];
  }

  UnmarshalBlock(_data: $.Bytes): $.GoError {
    return new UnsupportedOperationError(this.operationTypeID);
  }

  ApplyWorldOp(): [boolean, $.GoError] {
    return [false, new UnsupportedOperationError(this.operationTypeID)];
  }

  ApplyWorldObjectOp(): [boolean, $.GoError] {
    return [false, new UnsupportedOperationError(this.operationTypeID)];
  }
}

export function LookupCreateForgeDashboardOp(
  _ctx: unknown,
  operationTypeID: string,
): [unknown, $.GoError] {
  if (operationTypeID === CreateForgeDashboardOpId) {
    return [new ForgeDashboardOperation(CreateForgeDashboardOpId), null];
  }
  return [null, null];
}

export function LookupLinkForgeDashboardOp(
  _ctx: unknown,
  operationTypeID: string,
): [unknown, $.GoError] {
  if (operationTypeID === LinkForgeDashboardOpId) {
    return [new ForgeDashboardOperation(LinkForgeDashboardOpId), null];
  }
  return [null, null];
}

export function LookupInitForgeQuickstartOp(
  _ctx: unknown,
  operationTypeID: string,
): [unknown, $.GoError] {
  if (operationTypeID === InitForgeQuickstartOpId) {
    return [new ForgeDashboardOperation(InitForgeQuickstartOpId), null];
  }
  return [null, null];
}
