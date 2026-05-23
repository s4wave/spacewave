import * as $ from "@goscript/builtin/index.js";

export const InitOrganizationOpID = "org/init-organization";
export const UpdateOrgOpID = "org/update-organization";
export const DeleteOrganizationOpID = "org/delete-organization";
export const OrganizationTypeID = "spacewave/organization";
export const OrgBodyType = "organization";
export const OrgObjectKey = "org/state";
export const OrgRoleOwner = "Owner";
export const OrgRoleMember = "Member";

class UnsupportedOperationError {
  constructor(private readonly operation: string) {}

  Error(): string {
    return `${this.operation}: project override does not implement operation application`;
  }
}

class OrganizationOperation {
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

export function LookupInitOrganizationOp(
  _ctx: unknown,
  opTypeID: string,
): [unknown, $.GoError] {
  if (opTypeID === InitOrganizationOpID) {
    return [new OrganizationOperation(InitOrganizationOpID), null];
  }
  return [null, null];
}

export function LookupUpdateOrgOp(
  _ctx: unknown,
  opTypeID: string,
): [unknown, $.GoError] {
  if (opTypeID === UpdateOrgOpID) {
    return [new OrganizationOperation(UpdateOrgOpID), null];
  }
  return [null, null];
}

export function LookupDeleteOrganizationOp(
  _ctx: unknown,
  opTypeID: string,
): [unknown, $.GoError] {
  if (opTypeID === DeleteOrganizationOpID) {
    return [new OrganizationOperation(DeleteOrganizationOpID), null];
  }
  return [null, null];
}
