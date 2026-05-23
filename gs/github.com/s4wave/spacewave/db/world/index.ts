import * as $ from "@goscript/builtin/index.js";
import * as context from "@goscript/context/index.js";
import * as directive from "@goscript/github.com/aperturerobotics/controllerbus/directive/index.js";

export const ErrEmptyObjectKey = $.newError("object key cannot be empty");
export const ErrGraphPathDirection = $.newError("graph path step direction is invalid");
export const ErrGraphEdgeBucketDirection = $.newError("graph edge bucket direction is invalid");
export const ErrGraphPathResultLimit = $.newError("graph path result limit must be non-zero");
export const ErrGraphPathStepLimit = $.newError("graph path step limit must be non-zero");
export const ErrGraphPathPredicate = $.newError("graph path step predicate cannot be empty");
export const ErrGraphEdgeBucketLimit = $.newError("graph edge bucket limit must be non-zero");

export interface Operation {
  GetOperationTypeId(): string;
  Validate(): $.GoError;
  MarshalBlock(): [$.Bytes, $.GoError];
  UnmarshalBlock(data: $.Bytes): $.GoError;
  ApplyWorldOp(...args: unknown[]): [boolean, $.GoError];
  ApplyWorldObjectOp(...args: unknown[]): [boolean, $.GoError];
}

export type LookupOp = (
  ctx: context.Context | null,
  operationTypeID: string,
) => [Operation | null, $.GoError];

export type LookupOpSlice = LookupOp[];

export interface LookupWorldOp {
  LookupWorldOpEngineID(): string;
}

export interface GraphQuad {
  GetSubject(): string;
  GetPredicate(): string;
  GetObj(): string;
  GetLabel(): string;
}

class graphQuad implements GraphQuad {
  constructor(
    private subject: string,
    private predicate: string,
    private object: string,
    private label: string,
  ) {}

  GetSubject(): string {
    return this.subject;
  }

  GetPredicate(): string {
    return this.predicate;
  }

  GetObj(): string {
    return this.object;
  }

  GetLabel(): string {
    return this.label;
  }
}

export function KeyToGraphValue(key: string): string {
  return key;
}

export function GraphValueToKey(value: string): [string, $.GoError] {
  return [value, null];
}

export function NewGraphQuad(
  subject: string,
  predicate: string,
  object: string,
  label: string,
): GraphQuad {
  return new graphQuad(subject, predicate, object, label);
}

export function NewGraphQuadWithKeys(
  subjectKey: string,
  predicate: string,
  objectKey: string,
  label: string,
): GraphQuad {
  return NewGraphQuad(
    subjectKey === "" ? "" : KeyToGraphValue(subjectKey),
    predicate,
    objectKey === "" ? "" : KeyToGraphValue(objectKey),
    label,
  );
}

export interface ObjectState {
  GetKey(): string;
}

export interface WorldStateGraph {
  LookupGraphQuads(
    ctx: context.Context | null,
    filter: GraphQuad | null,
    limit: number,
  ): [$.Slice<GraphQuad>, $.GoError] | Promise<[$.Slice<GraphQuad>, $.GoError]>;
  LookupGraphQuadsBatch(
    ctx: context.Context | null,
    filters: $.Slice<GraphQuad>,
    limitPerFilter: number,
  ): [$.Slice<$.Slice<GraphQuad>>, $.GoError] | Promise<[$.Slice<$.Slice<GraphQuad>>, $.GoError]>;
  QueryGraphPath(
    ctx: context.Context | null,
    query: GraphPathQuery | null,
  ): [GraphPathQueryResult | null, $.GoError] | Promise<[GraphPathQueryResult | null, $.GoError]>;
}

export interface WorldState extends WorldStateGraph {
  GetReadOnly(): boolean;
}

export class ObjectRootRef {
  public ObjectKey = "";
  public Exists = false;
  public Rev = 0;
  public RootRef: unknown = null;

  constructor(init?: Partial<ObjectRootRef>) {
    Object.assign(this, init);
  }
}

export enum GraphPathDirection {
  GraphPathDirectionOut = 1,
  GraphPathDirectionIn = 2,
  GraphPathDirectionBoth = 3,
}

export const GraphPathDirectionOut = GraphPathDirection.GraphPathDirectionOut;
export const GraphPathDirectionIn = GraphPathDirection.GraphPathDirectionIn;
export const GraphPathDirectionBoth = GraphPathDirection.GraphPathDirectionBoth;

export class GraphPathStep {
  public Direction: GraphPathDirection = GraphPathDirectionOut;
  public Predicate = "";
  public Limit = 0;

  constructor(init?: Partial<GraphPathStep>) {
    Object.assign(this, init);
  }
}

export class GraphPathQuery {
  public StartObjectKeys: $.Slice<string> = null;
  public ResultLimit = 0;
  public Steps: $.Slice<GraphPathStep | null> = null;
  public IncludeQuads = false;

  constructor(init?: Partial<GraphPathQuery>) {
    Object.assign(this, init);
  }
}

export class GraphPathQueryResult {
  public ObjectKeys: $.Slice<string> = null;
  public Quads: $.Slice<GraphQuad> = null;

  constructor(init?: Partial<GraphPathQueryResult>) {
    Object.assign(this, init);
  }
}

export enum GraphEdgeBucketDirection {
  GraphEdgeBucketDirectionBoth = 0,
  GraphEdgeBucketDirectionOut = 1,
  GraphEdgeBucketDirectionIn = 2,
}

export const GraphEdgeBucketDirectionBoth = GraphEdgeBucketDirection.GraphEdgeBucketDirectionBoth;
export const GraphEdgeBucketDirectionOut = GraphEdgeBucketDirection.GraphEdgeBucketDirectionOut;
export const GraphEdgeBucketDirectionIn = GraphEdgeBucketDirection.GraphEdgeBucketDirectionIn;

export class GraphEdgeBucketQuery {
  public OriginObjectKeys: $.Slice<string> = null;
  public Predicate = "";
  public LimitPerOrigin = 0;
  public Direction: GraphEdgeBucketDirection = GraphEdgeBucketDirectionBoth;

  constructor(init?: Partial<GraphEdgeBucketQuery>) {
    Object.assign(this, init);
  }
}

export class GraphEdgeBucket {
  public OriginObjectKey = "";
  public Outgoing: $.Slice<GraphQuad> = null;
  public Incoming: $.Slice<GraphQuad> = null;
  public OutgoingTruncated = false;
  public IncomingTruncated = false;

  constructor(init?: Partial<GraphEdgeBucket>) {
    Object.assign(this, init);
  }
}

export function CollectGraphPathStepWithKeys(
  _ctx: context.Context | null,
  _ws: WorldStateGraph | null,
  _keys: $.Slice<string>,
  _direction: GraphPathDirection,
  _predicate: string,
  _limit: number,
): [$.Slice<string>, $.GoError] {
  return [[], null];
}

export function CollectObjectBodies(
  _ctx: context.Context | null,
  _ws: WorldState | null,
  _objectKeys: $.Slice<string>,
): [Map<string, $.Bytes>, $.GoError] {
  return [new Map(), null];
}

export class Engine {}

export function LookupOpSlice_LookupOp(
  lookups: LookupOpSlice,
  ctx: context.Context | null,
  operationTypeID: string,
): [Operation | null, $.GoError] {
  return lookupOpSlice(lookups, ctx, operationTypeID);
}

export function NewLookupOpFromSlice(lookups: LookupOpSlice): LookupOp {
  return (ctx: context.Context | null, operationTypeID: string) =>
    lookupOpSlice(lookups, ctx, operationTypeID);
}

export function BuildLookupWorldOpFunc(
  _b: unknown,
  _le: unknown,
  _engineID: string,
): LookupOp {
  return () => [null, null];
}

class LookupWorldOpResolver {
  async Resolve(
    _ctx: context.Context | null,
    _handler: directive.ResolverHandler | null,
  ): Promise<$.GoError> {
    return null;
  }
}

export function NewLookupWorldOpResolver(_lookup: LookupOp): directive.Resolver {
  return new LookupWorldOpResolver();
}

function lookupOpSlice(
  lookups: LookupOpSlice,
  ctx: context.Context | null,
  operationTypeID: string,
): [Operation | null, $.GoError] {
  for (const lookup of lookups ?? []) {
    if (!lookup) {
      continue;
    }
    const [op, err] = lookup(ctx, operationTypeID);
    if (err) {
      return [null, err];
    }
    if (op) {
      return [op, null];
    }
  }
  return [null, null];
}
