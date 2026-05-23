import * as $ from "@goscript/builtin/index.js";

const unsupported = "net/envelope: GoScript override does not implement recovery envelope crypto";
type EnvelopeGrantConfigPtr = EnvelopeGrantConfig | $.VarRef<EnvelopeGrantConfig> | null;
const envelopeProtoMethods: $.MethodSignature[] = [
  { name: "Reset", args: [], returns: [] },
  { name: "ProtoMessage", args: [], returns: [] },
  {
    name: "MarshalVT",
    args: [],
    returns: [
      { name: "", type: { kind: $.TypeKind.Slice, elemType: "number" } },
      { name: "", type: "error" },
    ],
  },
  {
    name: "UnmarshalVT",
    args: [{ name: "data", type: { kind: $.TypeKind.Slice, elemType: "number" } }],
    returns: [{ name: "", type: "error" }],
  },
];

export const ErrNoGrants = $.newError("envelope has no grants");
export const ErrNoKeypairs = $.newError("envelope has no keypairs");
export const ErrInvalidThreshold = $.newError("invalid threshold configuration");
export const ErrContextMismatch = $.newError("envelope context does not match expected context");
export const ErrInsufficientShares = $.newError("insufficient shares to unlock envelope");
export const ErrInvalidShareData = $.newError("invalid share data");
export const ErrInvalidKeypairIndex = $.newError("keypair index out of range");
export const ErrEmptyPayload = $.newError("payload is empty");
export const ErrDecryptionFailed = $.newError("envelope decryption failed");

export class Envelope {
  public _fields: {
    EnvelopeId: $.VarRef<string>;
    ContextHash: $.VarRef<$.Bytes>;
    Threshold: $.VarRef<number>;
    Ciphertext: $.VarRef<$.Bytes>;
    Grants: $.VarRef<$.Slice<EnvelopeGrant | null>>;
    Keypairs: $.VarRef<$.Slice<EnvelopeKeypair | null>>;
    Contents: $.VarRef<$.Bytes>;
  };

  constructor(init?: Partial<{
    EnvelopeId: string;
    ContextHash: $.Bytes;
    Threshold: number;
    Ciphertext: $.Bytes;
    Grants: $.Slice<EnvelopeGrant | null>;
    Keypairs: $.Slice<EnvelopeKeypair | null>;
    Contents: $.Bytes;
  }>) {
    this._fields = {
      EnvelopeId: $.varRef(init?.EnvelopeId ?? ""),
      ContextHash: $.varRef(init?.ContextHash ?? null),
      Threshold: $.varRef(init?.Threshold ?? 0),
      Ciphertext: $.varRef(init?.Ciphertext ?? null),
      Grants: $.varRef(init?.Grants ?? null),
      Keypairs: $.varRef(init?.Keypairs ?? null),
      Contents: $.varRef(init?.Contents ?? null),
    };
  }

  get EnvelopeId(): string { return this._fields.EnvelopeId.value; }
  set EnvelopeId(value: string) { this._fields.EnvelopeId.value = value; }
  get ContextHash(): $.Bytes { return this._fields.ContextHash.value; }
  set ContextHash(value: $.Bytes) { this._fields.ContextHash.value = value; }
  get Threshold(): number { return this._fields.Threshold.value; }
  set Threshold(value: number) { this._fields.Threshold.value = value; }
  get Ciphertext(): $.Bytes { return this._fields.Ciphertext.value; }
  set Ciphertext(value: $.Bytes) { this._fields.Ciphertext.value = value; }
  get Grants(): $.Slice<EnvelopeGrant | null> { return this._fields.Grants.value; }
  set Grants(value: $.Slice<EnvelopeGrant | null>) { this._fields.Grants.value = value; }
  get Keypairs(): $.Slice<EnvelopeKeypair | null> { return this._fields.Keypairs.value; }
  set Keypairs(value: $.Slice<EnvelopeKeypair | null>) { this._fields.Keypairs.value = value; }
  get Contents(): $.Bytes { return this._fields.Contents.value; }
  set Contents(value: $.Bytes) { this._fields.Contents.value = value; }

  Reset(): void { $.assignStruct(this, new Envelope()); }
  ProtoMessage(): void {}
  GetEnvelopeId(): string { return this.EnvelopeId; }
  GetContextHash(): $.Bytes { return this.ContextHash; }
  GetThreshold(): number { return this.Threshold; }
  GetCiphertext(): $.Bytes { return this.Ciphertext; }
  GetGrants(): $.Slice<EnvelopeGrant | null> { return this.Grants; }
  GetKeypairs(): $.Slice<EnvelopeKeypair | null> { return this.Keypairs; }
  GetContents(): $.Bytes { return this.Contents; }
  MarshalVT(): [$.Bytes, $.GoError] { return [new Uint8Array(0), unsupportedError()]; }
  UnmarshalVT(_data: $.Bytes): $.GoError { return unsupportedError(); }
  SizeVT(): number { return 0; }

  static __typeInfo = $.registerStructType(
    "envelope.Envelope",
    () => new Envelope(),
    envelopeProtoMethods,
    Envelope,
    {},
  );
}

export class EnvelopeGrant {
  public _fields: {
    KeypairIndexes: $.VarRef<$.Slice<number>>;
    Ciphertexts: $.VarRef<$.Slice<$.Bytes>>;
  };

  constructor(init?: Partial<{
    KeypairIndexes: $.Slice<number>;
    Ciphertexts: $.Slice<$.Bytes>;
  }>) {
    this._fields = {
      KeypairIndexes: $.varRef(init?.KeypairIndexes ?? null),
      Ciphertexts: $.varRef(init?.Ciphertexts ?? null),
    };
  }

  get KeypairIndexes(): $.Slice<number> { return this._fields.KeypairIndexes.value; }
  set KeypairIndexes(value: $.Slice<number>) { this._fields.KeypairIndexes.value = value; }
  get Ciphertexts(): $.Slice<$.Bytes> { return this._fields.Ciphertexts.value; }
  set Ciphertexts(value: $.Slice<$.Bytes>) { this._fields.Ciphertexts.value = value; }

  Reset(): void { $.assignStruct(this, new EnvelopeGrant()); }
  ProtoMessage(): void {}
  GetKeypairIndexes(): $.Slice<number> { return this.KeypairIndexes; }
  GetCiphertexts(): $.Slice<$.Bytes> { return this.Ciphertexts; }
  MarshalVT(): [$.Bytes, $.GoError] { return [new Uint8Array(0), unsupportedError()]; }
  UnmarshalVT(_data: $.Bytes): $.GoError { return unsupportedError(); }
  SizeVT(): number { return 0; }

  static __typeInfo = $.registerStructType(
    "envelope.EnvelopeGrant",
    () => new EnvelopeGrant(),
    envelopeProtoMethods,
    EnvelopeGrant,
    {},
  );
}

export class EnvelopeGrantInner {
  public _fields: {
    Shares: $.VarRef<$.Slice<EnvelopeShare | null>>;
  };

  constructor(init?: Partial<{ Shares: $.Slice<EnvelopeShare | null> }>) {
    this._fields = {
      Shares: $.varRef(init?.Shares ?? null),
    };
  }

  get Shares(): $.Slice<EnvelopeShare | null> { return this._fields.Shares.value; }
  set Shares(value: $.Slice<EnvelopeShare | null>) { this._fields.Shares.value = value; }

  Reset(): void { $.assignStruct(this, new EnvelopeGrantInner()); }
  ProtoMessage(): void {}
  GetShares(): $.Slice<EnvelopeShare | null> { return this.Shares; }
  MarshalVT(): [$.Bytes, $.GoError] { return [new Uint8Array(0), unsupportedError()]; }
  UnmarshalVT(_data: $.Bytes): $.GoError { return unsupportedError(); }
  SizeVT(): number { return 0; }

  static __typeInfo = $.registerStructType(
    "envelope.EnvelopeGrantInner",
    () => new EnvelopeGrantInner(),
    envelopeProtoMethods,
    EnvelopeGrantInner,
    {},
  );
}

export class EnvelopeShare {
  public _fields: {
    Id: $.VarRef<$.Bytes>;
    Value: $.VarRef<$.Bytes>;
  };

  constructor(init?: Partial<{ Id: $.Bytes; Value: $.Bytes }>) {
    this._fields = {
      Id: $.varRef(init?.Id ?? null),
      Value: $.varRef(init?.Value ?? null),
    };
  }

  get Id(): $.Bytes { return this._fields.Id.value; }
  set Id(value: $.Bytes) { this._fields.Id.value = value; }
  get Value(): $.Bytes { return this._fields.Value.value; }
  set Value(value: $.Bytes) { this._fields.Value.value = value; }

  Reset(): void { $.assignStruct(this, new EnvelopeShare()); }
  ProtoMessage(): void {}
  GetId(): $.Bytes { return this.Id; }
  GetValue(): $.Bytes { return this.Value; }
  MarshalVT(): [$.Bytes, $.GoError] { return [new Uint8Array(0), unsupportedError()]; }
  UnmarshalVT(_data: $.Bytes): $.GoError { return unsupportedError(); }
  SizeVT(): number { return 0; }

  static __typeInfo = $.registerStructType(
    "envelope.EnvelopeShare",
    () => new EnvelopeShare(),
    envelopeProtoMethods,
    EnvelopeShare,
    {},
  );
}

export class EnvelopeKeypair {
  public _fields: {
    PubKey: $.VarRef<$.Bytes>;
    AuthMethodId: $.VarRef<string>;
    AuthMethodParams: $.VarRef<$.Bytes>;
  };

  constructor(init?: Partial<{
    PubKey: $.Bytes;
    AuthMethodId: string;
    AuthMethodParams: $.Bytes;
  }>) {
    this._fields = {
      PubKey: $.varRef(init?.PubKey ?? null),
      AuthMethodId: $.varRef(init?.AuthMethodId ?? ""),
      AuthMethodParams: $.varRef(init?.AuthMethodParams ?? null),
    };
  }

  get PubKey(): $.Bytes { return this._fields.PubKey.value; }
  set PubKey(value: $.Bytes) { this._fields.PubKey.value = value; }
  get AuthMethodId(): string { return this._fields.AuthMethodId.value; }
  set AuthMethodId(value: string) { this._fields.AuthMethodId.value = value; }
  get AuthMethodParams(): $.Bytes { return this._fields.AuthMethodParams.value; }
  set AuthMethodParams(value: $.Bytes) { this._fields.AuthMethodParams.value = value; }

  Reset(): void { $.assignStruct(this, new EnvelopeKeypair()); }
  ProtoMessage(): void {}
  GetPubKey(): $.Bytes { return this.PubKey; }
  GetAuthMethodId(): string { return this.AuthMethodId; }
  GetAuthMethodParams(): $.Bytes { return this.AuthMethodParams; }
  MarshalVT(): [$.Bytes, $.GoError] { return [new Uint8Array(0), unsupportedError()]; }
  UnmarshalVT(_data: $.Bytes): $.GoError { return unsupportedError(); }
  SizeVT(): number { return 0; }

  static __typeInfo = $.registerStructType(
    "envelope.EnvelopeKeypair",
    () => new EnvelopeKeypair(),
    envelopeProtoMethods,
    EnvelopeKeypair,
    {},
  );
}

export class EnvelopeConfig {
  public _fields: {
    EnvelopeId: $.VarRef<string>;
    Threshold: $.VarRef<number>;
    TotalShares: $.VarRef<number>;
    GrantConfigs: $.VarRef<$.Slice<EnvelopeGrantConfigPtr>>;
  };

  constructor(init?: Partial<{
    EnvelopeId: string;
    Threshold: number;
    TotalShares: number;
    GrantConfigs: $.Slice<EnvelopeGrantConfigPtr>;
  }>) {
    this._fields = {
      EnvelopeId: $.varRef(init?.EnvelopeId ?? ""),
      Threshold: $.varRef(init?.Threshold ?? 0),
      TotalShares: $.varRef(init?.TotalShares ?? 0),
      GrantConfigs: $.varRef(init?.GrantConfigs ?? null),
    };
  }

  get EnvelopeId(): string { return this._fields.EnvelopeId.value; }
  set EnvelopeId(value: string) { this._fields.EnvelopeId.value = value; }
  get Threshold(): number { return this._fields.Threshold.value; }
  set Threshold(value: number) { this._fields.Threshold.value = value; }
  get TotalShares(): number { return this._fields.TotalShares.value; }
  set TotalShares(value: number) { this._fields.TotalShares.value = value; }
  get GrantConfigs(): $.Slice<EnvelopeGrantConfigPtr> { return this._fields.GrantConfigs.value; }
  set GrantConfigs(value: $.Slice<EnvelopeGrantConfigPtr>) { this._fields.GrantConfigs.value = value; }

  Reset(): void { $.assignStruct(this, new EnvelopeConfig()); }
  ProtoMessage(): void {}
  GetEnvelopeId(): string { return this.EnvelopeId; }
  GetThreshold(): number { return this.Threshold; }
  GetTotalShares(): number { return this.TotalShares; }
  GetGrantConfigs(): $.Slice<EnvelopeGrantConfigPtr> { return this.GrantConfigs; }
  MarshalVT(): [$.Bytes, $.GoError] { return [new Uint8Array(0), unsupportedError()]; }
  UnmarshalVT(_data: $.Bytes): $.GoError { return unsupportedError(); }
  SizeVT(): number { return 0; }

  static __typeInfo = $.registerStructType(
    "envelope.EnvelopeConfig",
    () => new EnvelopeConfig(),
    envelopeProtoMethods,
    EnvelopeConfig,
    {},
  );
}

export class EnvelopeGrantConfig {
  public _fields: {
    ShareCount: $.VarRef<number>;
    KeypairIndexes: $.VarRef<$.Slice<number>>;
  };

  constructor(init?: Partial<{
    ShareCount: number;
    KeypairIndexes: $.Slice<number>;
  }>) {
    this._fields = {
      ShareCount: $.varRef(init?.ShareCount ?? 0),
      KeypairIndexes: $.varRef(init?.KeypairIndexes ?? null),
    };
  }

  get ShareCount(): number { return this._fields.ShareCount.value; }
  set ShareCount(value: number) { this._fields.ShareCount.value = value; }
  get KeypairIndexes(): $.Slice<number> { return this._fields.KeypairIndexes.value; }
  set KeypairIndexes(value: $.Slice<number>) { this._fields.KeypairIndexes.value = value; }

  Reset(): void { $.assignStruct(this, new EnvelopeGrantConfig()); }
  ProtoMessage(): void {}
  GetShareCount(): number { return this.ShareCount; }
  GetKeypairIndexes(): $.Slice<number> { return this.KeypairIndexes; }
  MarshalVT(): [$.Bytes, $.GoError] { return [new Uint8Array(0), unsupportedError()]; }
  UnmarshalVT(_data: $.Bytes): $.GoError { return unsupportedError(); }
  SizeVT(): number { return 0; }

  static __typeInfo = $.registerStructType(
    "envelope.EnvelopeGrantConfig",
    () => new EnvelopeGrantConfig(),
    envelopeProtoMethods,
    EnvelopeGrantConfig,
    {},
  );
}

export class EnvelopeUnlockResult {
  public _fields: {
    Success: $.VarRef<boolean>;
    SharesAvailable: $.VarRef<number>;
    SharesNeeded: $.VarRef<number>;
    UnlockedGrantIndexes: $.VarRef<$.Slice<number>>;
  };

  constructor(init?: Partial<{
    Success: boolean;
    SharesAvailable: number;
    SharesNeeded: number;
    UnlockedGrantIndexes: $.Slice<number>;
  }>) {
    this._fields = {
      Success: $.varRef(init?.Success ?? false),
      SharesAvailable: $.varRef(init?.SharesAvailable ?? 0),
      SharesNeeded: $.varRef(init?.SharesNeeded ?? 0),
      UnlockedGrantIndexes: $.varRef(init?.UnlockedGrantIndexes ?? null),
    };
  }

  get Success(): boolean { return this._fields.Success.value; }
  set Success(value: boolean) { this._fields.Success.value = value; }
  get SharesAvailable(): number { return this._fields.SharesAvailable.value; }
  set SharesAvailable(value: number) { this._fields.SharesAvailable.value = value; }
  get SharesNeeded(): number { return this._fields.SharesNeeded.value; }
  set SharesNeeded(value: number) { this._fields.SharesNeeded.value = value; }
  get UnlockedGrantIndexes(): $.Slice<number> { return this._fields.UnlockedGrantIndexes.value; }
  set UnlockedGrantIndexes(value: $.Slice<number>) { this._fields.UnlockedGrantIndexes.value = value; }

  Reset(): void { $.assignStruct(this, new EnvelopeUnlockResult()); }
  ProtoMessage(): void {}
  GetSuccess(): boolean { return this.Success; }
  GetSharesAvailable(): number { return this.SharesAvailable; }
  GetSharesNeeded(): number { return this.SharesNeeded; }
  GetUnlockedGrantIndexes(): $.Slice<number> { return this.UnlockedGrantIndexes; }
  MarshalVT(): [$.Bytes, $.GoError] { return [new Uint8Array(0), unsupportedError()]; }
  UnmarshalVT(_data: $.Bytes): $.GoError { return unsupportedError(); }
  SizeVT(): number { return 0; }

  static __typeInfo = $.registerStructType(
    "envelope.EnvelopeUnlockResult",
    () => new EnvelopeUnlockResult(),
    envelopeProtoMethods,
    EnvelopeUnlockResult,
    {},
  );
}

export function BuildEnvelope(
  _rnd: unknown,
  _context: string,
  _payload: $.Bytes,
  _keypairs: $.Slice<unknown>,
  _config: EnvelopeConfig | null,
): [Envelope | null, $.GoError] {
  return [null, unsupportedError()];
}

export function UnlockEnvelope(
  _context: string,
  _env: Envelope | null,
  _privKeys: $.Slice<unknown>,
): [$.Bytes, EnvelopeUnlockResult | null, $.GoError] {
  return [null, null, unsupportedError()];
}

function unsupportedError(): $.GoError {
  return $.newError(unsupported);
}
