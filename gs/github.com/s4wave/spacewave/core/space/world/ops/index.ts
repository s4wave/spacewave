import * as $ from "@goscript/builtin/index.js";
import * as world from "@goscript/github.com/s4wave/spacewave/db/world/index.js";

export const SetSpaceSettingsOpId = "space/world/set-settings";
export const InitUnixFSOpId = "space/world/init-unixfs";
export const InitObjectLayoutOpId = "space/world/init-object-layout";
export const InitCanvasDemoOpId = "space/world/init-canvas-demo";
export const CanvasInitOpId = "space/world/init-canvas";
export const CanvasAddNodeOpId = "space/world/canvas-add-node";
export const CanvasRemoveNodeOpId = "space/world/canvas-remove-node";
export const CanvasSetNodeOpId = "space/world/canvas-set-node";
export const CanvasAddEdgeOpId = "space/world/canvas-add-edge";
export const CanvasRemoveEdgeOpId = "space/world/canvas-remove-edge";
export const ConfigID = "space/world/ops/config";
export const ControllerID = "space/world/ops/controller";

export class Config {
  constructor(private readonly engineId = "") {}

  GetEngineId(): string {
    return this.engineId;
  }

  static __typeInfo = $.registerStructType(
    "space_world_ops.Config",
    () => new Config(),
    [{ name: "GetEngineId", args: [], returns: [{ name: "", type: "string" }] }],
    Config,
    {},
  );
}

class UnsupportedOperationError {
  constructor(private readonly operation: string) {}

  Error(): string {
    return `${this.operation}: project override does not implement operation application`;
  }
}

class SpaceWorldOperation {
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

export class SetSpaceSettingsOp extends SpaceWorldOperation {
  constructor() {
    super(SetSpaceSettingsOpId);
  }

  static __typeInfo = registerOperationType("SetSpaceSettingsOp", SetSpaceSettingsOp);
}

export class InitUnixFSOp extends SpaceWorldOperation {
  constructor() {
    super(InitUnixFSOpId);
  }

  static __typeInfo = registerOperationType("InitUnixFSOp", InitUnixFSOp);
}

export class InitObjectLayoutOp extends SpaceWorldOperation {
  constructor() {
    super(InitObjectLayoutOpId);
  }

  static __typeInfo = registerOperationType("InitObjectLayoutOp", InitObjectLayoutOp);
}

export class InitCanvasDemoOp extends SpaceWorldOperation {
  constructor() {
    super(InitCanvasDemoOpId);
  }

  static __typeInfo = registerOperationType("InitCanvasDemoOp", InitCanvasDemoOp);
}

export class CanvasInitOp extends SpaceWorldOperation {
  constructor() {
    super(CanvasInitOpId);
  }

  static __typeInfo = registerOperationType("CanvasInitOp", CanvasInitOp);
}

export class CanvasAddNodeOp extends SpaceWorldOperation {
  constructor() {
    super(CanvasAddNodeOpId);
  }

  static __typeInfo = registerOperationType("CanvasAddNodeOp", CanvasAddNodeOp);
}

export class CanvasRemoveNodeOp extends SpaceWorldOperation {
  constructor() {
    super(CanvasRemoveNodeOpId);
  }

  static __typeInfo = registerOperationType("CanvasRemoveNodeOp", CanvasRemoveNodeOp);
}

export class CanvasSetNodeOp extends SpaceWorldOperation {
  constructor() {
    super(CanvasSetNodeOpId);
  }

  static __typeInfo = registerOperationType("CanvasSetNodeOp", CanvasSetNodeOp);
}

export class CanvasAddEdgeOp extends SpaceWorldOperation {
  constructor() {
    super(CanvasAddEdgeOpId);
  }

  static __typeInfo = registerOperationType("CanvasAddEdgeOp", CanvasAddEdgeOp);
}

export class CanvasRemoveEdgeOp extends SpaceWorldOperation {
  constructor() {
    super(CanvasRemoveEdgeOpId);
  }

  static __typeInfo = registerOperationType("CanvasRemoveEdgeOp", CanvasRemoveEdgeOp);
}

type LookupOperation = () => world.Operation;

const lookup: Record<string, LookupOperation> = {
  [SetSpaceSettingsOpId]: () => new SetSpaceSettingsOp(),
  [InitUnixFSOpId]: () => new InitUnixFSOp(),
  [InitObjectLayoutOpId]: () => new InitObjectLayoutOp(),
  [InitCanvasDemoOpId]: () => new InitCanvasDemoOp(),
  [CanvasInitOpId]: () => new CanvasInitOp(),
  [CanvasAddNodeOpId]: () => new CanvasAddNodeOp(),
  [CanvasRemoveNodeOpId]: () => new CanvasRemoveNodeOp(),
  [CanvasSetNodeOpId]: () => new CanvasSetNodeOp(),
  [CanvasAddEdgeOpId]: () => new CanvasAddEdgeOp(),
  [CanvasRemoveEdgeOpId]: () => new CanvasRemoveEdgeOp(),
};

function registerOperationType(
  name: string,
  ctor: new () => SpaceWorldOperation,
) {
  return $.registerStructType(
    `space_world_ops.${name}`,
    () => new ctor(),
    [
      { name: "GetOperationTypeId", args: [], returns: [{ name: "", type: "string" }] },
      { name: "Validate", args: [], returns: [{ name: "", type: "error" }] },
      {
        name: "MarshalBlock",
        args: [],
        returns: [
          { name: "", type: { kind: $.TypeKind.Slice, elemType: "number" } },
          { name: "", type: "error" },
        ],
      },
      {
        name: "UnmarshalBlock",
        args: [{ name: "data", type: { kind: $.TypeKind.Slice, elemType: "number" } }],
        returns: [{ name: "", type: "error" }],
      },
      {
        name: "ApplyWorldOp",
        args: [],
        returns: [
          { name: "", type: "boolean" },
          { name: "", type: "error" },
        ],
      },
      {
        name: "ApplyWorldObjectOp",
        args: [],
        returns: [
          { name: "", type: "boolean" },
          { name: "", type: "error" },
        ],
      },
    ],
    ctor,
    {},
  );
}

function lookupSpaceWorldOperation(
  operationTypeID: string,
): [world.Operation | null, $.GoError] {
  const construct = lookup[operationTypeID];
  if (!construct) {
    return [null, null];
  }
  return [construct(), null];
}

export function LookupSetSpaceSettingsOp(
  _ctx: unknown,
  operationTypeID: string,
): [world.Operation | null, $.GoError] {
  if (operationTypeID !== SetSpaceSettingsOpId) {
    return [null, null];
  }
  return [new SetSpaceSettingsOp(), null];
}

export function LookupInitUnixFSOp(
  _ctx: unknown,
  operationTypeID: string,
): [world.Operation | null, $.GoError] {
  if (operationTypeID !== InitUnixFSOpId) {
    return [null, null];
  }
  return [new InitUnixFSOp(), null];
}

export function LookupInitObjectLayoutOp(
  _ctx: unknown,
  operationTypeID: string,
): [world.Operation | null, $.GoError] {
  if (operationTypeID !== InitObjectLayoutOpId) {
    return [null, null];
  }
  return [new InitObjectLayoutOp(), null];
}

export function LookupInitCanvasDemoOp(
  _ctx: unknown,
  operationTypeID: string,
): [world.Operation | null, $.GoError] {
  if (operationTypeID !== InitCanvasDemoOpId) {
    return [null, null];
  }
  return [new InitCanvasDemoOp(), null];
}

export function LookupCanvasInitOp(
  _ctx: unknown,
  operationTypeID: string,
): [world.Operation | null, $.GoError] {
  if (operationTypeID !== CanvasInitOpId) {
    return [null, null];
  }
  return [new CanvasInitOp(), null];
}

export function LookupCanvasAddNodeOp(
  _ctx: unknown,
  operationTypeID: string,
): [world.Operation | null, $.GoError] {
  if (operationTypeID !== CanvasAddNodeOpId) {
    return [null, null];
  }
  return [new CanvasAddNodeOp(), null];
}

export function LookupCanvasRemoveNodeOp(
  _ctx: unknown,
  operationTypeID: string,
): [world.Operation | null, $.GoError] {
  if (operationTypeID !== CanvasRemoveNodeOpId) {
    return [null, null];
  }
  return [new CanvasRemoveNodeOp(), null];
}

export function LookupCanvasSetNodeOp(
  _ctx: unknown,
  operationTypeID: string,
): [world.Operation | null, $.GoError] {
  if (operationTypeID !== CanvasSetNodeOpId) {
    return [null, null];
  }
  return [new CanvasSetNodeOp(), null];
}

export function LookupCanvasAddEdgeOp(
  _ctx: unknown,
  operationTypeID: string,
): [world.Operation | null, $.GoError] {
  if (operationTypeID !== CanvasAddEdgeOpId) {
    return [null, null];
  }
  return [new CanvasAddEdgeOp(), null];
}

export function LookupCanvasRemoveEdgeOp(
  _ctx: unknown,
  operationTypeID: string,
): [world.Operation | null, $.GoError] {
  if (operationTypeID !== CanvasRemoveEdgeOpId) {
    return [null, null];
  }
  return [new CanvasRemoveEdgeOp(), null];
}

export function LookupWorldOp(
  _ctx: unknown,
  operationTypeID: string,
): [world.Operation | null, $.GoError] {
  return lookupSpaceWorldOperation(operationTypeID);
}
