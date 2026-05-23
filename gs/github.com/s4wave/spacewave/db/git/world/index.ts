import * as $ from "@goscript/builtin/index.js";

export const GitInitOpId = "hydra/git/init";
export const GitCloneOpId = "hydra/git/clone";
export const GitCreateWorktreeOpId = "hydra/git/create-worktree";
export const GitFetchOpId = "hydra/git/fetch";
export const GitWorktreeCheckoutOpId = "hydra/git/worktree/checkout";
export const GitStageOpId = "hydra/git/stage";
export const GitUnstageOpId = "hydra/git/unstage";

class UnsupportedOperationError {
  constructor(private readonly operation: string) {}

  Error(): string {
    return `${this.operation}: project override does not implement operation application`;
  }
}

class GitOperation {
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

export function LookupGitOp(
  _ctx: unknown,
  operationTypeID: string,
): [unknown, $.GoError] {
  switch (operationTypeID) {
    case GitInitOpId:
    case GitCloneOpId:
    case GitCreateWorktreeOpId:
    case GitFetchOpId:
    case GitWorktreeCheckoutOpId:
    case GitStageOpId:
    case GitUnstageOpId:
      return [new GitOperation(operationTypeID), null];
    default:
      return [null, null];
  }
}
