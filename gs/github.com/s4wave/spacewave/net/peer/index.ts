import * as $ from "@goscript/builtin/index.js";
import * as crypto from "@goscript/github.com/s4wave/spacewave/net/crypto/index.js";
import * as hash from "@goscript/github.com/s4wave/spacewave/net/hash/index.js";

type HashType = number;

const mhIdentity = 0;
const signSeparator = $.stringToBytes(" - SIGN - ");
const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz";
const unsupportedEncryption = $.newError("net/peer: GoScript override does not implement peer public-key encryption");

export type ID = Uint8Array | string;
export type IDSlice = $.Slice<ID>;

export const ErrEmptyPeerID = $.newError("peer id cannot be empty");
export const ErrEmptyBody = $.newError("message body cannot be empty");
export const ErrSignatureInvalid = $.newError("message signature invalid");
export const ErrShortMessage = $.newError("message too short");
export const ErrNoPrivKey = $.newError("private key not available for peer");
export const ErrNoPublicKey = $.newError("public key is not embedded in peer ID");
export const ErrInvalidEd25519PubKeyForCurve25519 = $.newError("invalid ed25519 public key for curve25519");

type PeerPrivKey = any;
type PeerPubKey = any;
type SignatureLike = Signature | $.VarRef<Signature> | null;

export interface Peer {
  GetPeerID(): ID;
  GetPubKey(): PeerPubKey;
  GetPrivKey(_ctx: unknown): [PeerPrivKey | null, $.GoError];
}

class memoryPeer implements Peer {
  constructor(
    private readonly privKey: PeerPrivKey | null,
    private readonly pubKey: PeerPubKey,
    private readonly peerID: ID,
  ) {}

  GetPeerID(): ID {
    return this.peerID;
  }

  GetPubKey(): PeerPubKey {
    return this.pubKey;
  }

  GetPrivKey(_ctx: unknown): [PeerPrivKey | null, $.GoError] {
    if (this.privKey == null) {
      return [null, ErrNoPrivKey];
    }
    return [this.privKey, null];
  }
}

export async function NewPeer(privKey: PeerPrivKey | null): Promise<[Peer | null, $.GoError]> {
  if (privKey == null) {
    const [generatedPriv, , err] = await crypto.GenerateKeyPair(crypto.Ed25519, -1);
    if (err != null) {
      return [null, err];
    }
    privKey = generatedPriv;
  }
  const [id, idErr] = IDFromPrivateKey(privKey);
  if (idErr != null) {
    return [null, idErr];
  }
  return [new memoryPeer(privKey, privKey.GetPublic(), id), null];
}

export async function NewPeerWithGenerateED25519(): Promise<[Peer | null, PeerPrivKey | null, PeerPubKey | null, $.GoError]> {
  const [privKey, pubKey, err] = await crypto.GenerateKeyPair(crypto.Ed25519, -1);
  if (err != null) {
    return [null, null, null, err];
  }
  const [peer, peerErr] = await NewPeer(privKey);
  if (peerErr != null) {
    return [null, null, null, peerErr];
  }
  return [peer, privKey, pubKey, null];
}

export function NewPeerWithPubKey(pubKey: PeerPubKey): [Peer | null, $.GoError] {
  const [id, err] = IDFromPublicKey(pubKey);
  if (err != null) {
    return [null, err];
  }
  return [new memoryPeer(null, pubKey, id), null];
}

export async function NewPeerWithID(id: ID): Promise<[Peer | null, $.GoError]> {
  const [pubKey, err] = await ID_ExtractPublicKey(id);
  if (err != null) {
    return [null, err];
  }
  return NewPeerWithPubKey(pubKey);
}

export async function ParsePeerIDWithPubKey(peerIDStr: string): Promise<[ID, PeerPubKey | null, $.GoError]> {
  const [peerID, parseErr] = IDB58Decode(peerIDStr);
  if (parseErr != null) {
    return ["", null, parseErr];
  }
  const [peerPub, pubErr] = await ID_ExtractPublicKey(peerID);
  if (pubErr != null) {
    return [peerID, null, pubErr];
  }
  return [peerID, peerPub, null];
}

export function ID_String(id: ID): string {
  return IDB58Encode(id);
}

export function ID_ShortString(id: ID): string {
  const pid = ID_String(id);
  if (pid.length <= 10) {
    return pid;
  }
  return pid.slice(0, 2) + "*" + pid.slice(pid.length - 6);
}

export function ID_Validate(id: ID): $.GoError {
  if (idBytes(id).length === 0) {
    return ErrEmptyPeerID;
  }
  return null;
}

export function ID_MatchesPublicKey(id: ID, pk: PeerPubKey): boolean {
  const [otherID, err] = IDFromPublicKey(pk);
  return err == null && bytesEqual(idBytes(otherID), idBytes(id));
}

export function ID_MatchesPrivateKey(id: ID, sk: PeerPrivKey): boolean {
  return ID_MatchesPublicKey(id, sk.GetPublic());
}

export async function ID_ExtractPublicKey(id: ID): Promise<[PeerPubKey | null, $.GoError]> {
  const [code, digest, err] = decodeMultihash(idBytes(id));
  if (err != null) {
    return [null, err];
  }
  if (code !== mhIdentity) {
    return [null, ErrNoPublicKey];
  }
  return crypto.UnmarshalPublicKey(digest);
}

export function IDFromBytes(b: $.Bytes): [ID, $.GoError] {
  const bytes = $.bytesToUint8Array(b);
  const [, , err] = decodeMultihash(bytes);
  if (err != null) {
    return ["", err];
  }
  return [copyBytes(bytes), null];
}

export function IDB58Decode(s: string): [ID, $.GoError] {
  const [decoded, err] = base58Decode(s);
  if (err != null) {
    return ["", $.newError("failed to parse peer ID: " + err.Error())];
  }
  return IDFromBytes(decoded);
}

export function IDB58Encode(id: ID): string {
  return base58Encode(idBytes(id));
}

export function IDFromPublicKey(pk: PeerPubKey): [ID, $.GoError] {
  const [data, err] = crypto.MarshalPublicKey(pk);
  if (err != null) {
    return ["", err];
  }
  return [encodeMultihash(mhIdentity, $.bytesToUint8Array(data)), null];
}

export function IDFromPrivateKey(sk: PeerPrivKey): [ID, $.GoError] {
  return IDFromPublicKey(sk.GetPublic());
}

export function EncryptToPubKey(_pubKey: PeerPubKey, _context: string, _msgSrc: $.Bytes): [$.Bytes, $.GoError] {
  return [null, unsupportedEncryption];
}

export function DecryptWithPrivKey(_privKey: PeerPrivKey, _context: string, _ciphertext: $.Bytes): [$.Bytes, $.GoError] {
  return [null, unsupportedEncryption];
}

export function IDsToString(ids: $.Slice<ID>): $.Slice<string> {
  const values = ids == null ? [] : $.asArray(ids).map((id) => ID_String(id));
  return $.arrayToSlice(values);
}

export function IDSlice_Len(es: IDSlice): number {
  return $.len(es);
}

export function IDSlice_Swap(es: IDSlice, i: number, j: number): void {
  const values = $.asArray(es);
  const tmp = values[i];
  values[i] = values[j];
  values[j] = tmp;
}

export function IDSlice_Less(es: IDSlice, i: number, j: number): boolean {
  return compareBytes(idBytes($.asArray(es)[i]), idBytes($.asArray(es)[j])) < 0;
}

export function IDSlice_String(es: IDSlice): string {
  return (es == null ? [] : $.asArray(es).map((id) => ID_String(id))).join(", ");
}

export class Signature {
  public _fields: {
    PubKey: $.VarRef<$.Bytes>;
    HashType: $.VarRef<HashType>;
    SigData: $.VarRef<$.Bytes>;
  };

  constructor(init?: Partial<{ PubKey: $.Bytes; HashType: HashType; SigData: $.Bytes }>) {
    this._fields = {
      PubKey: $.varRef(init?.PubKey ?? null),
      HashType: $.varRef(init?.HashType ?? 0),
      SigData: $.varRef(init?.SigData ?? null),
    };
  }

  get PubKey(): $.Bytes { return this._fields.PubKey.value; }
  set PubKey(value: $.Bytes) { this._fields.PubKey.value = value; }
  get HashType(): HashType { return this._fields.HashType.value; }
  set HashType(value: HashType) { this._fields.HashType.value = value; }
  get SigData(): $.Bytes { return this._fields.SigData.value; }
  set SigData(value: $.Bytes) { this._fields.SigData.value = value; }

  Reset(): void { $.assignStruct(this, new Signature()); }
  ProtoMessage(): void {}
  GetPubKey(this: SignatureLike): $.Bytes { return signatureValue(this)?.PubKey ?? null; }
  GetHashType(this: SignatureLike): HashType { return signatureValue(this)?.HashType ?? 0; }
  GetSigData(this: SignatureLike): $.Bytes { return signatureValue(this)?.SigData ?? null; }

  async Validate(this: SignatureLike): Promise<$.GoError> {
    const hashErr = validateHashType(Signature.prototype.GetHashType.call(this));
    if (hashErr != null) {
      return hashErr;
    }
    if ($.len(Signature.prototype.GetSigData.call(this)) === 0) {
      return ErrSignatureInvalid;
    }
    if ($.len(Signature.prototype.GetPubKey.call(this)) !== 0) {
      const [, err] = await Signature.prototype.ParsePubKey.call(this);
      if (err != null) {
        return $.newError("pub_key: " + err.Error());
      }
    }
    return null;
  }

  async VerifyWithPublic(this: SignatureLike, encContext: string, pubKey: PeerPubKey, data: $.Bytes): Promise<[boolean, $.GoError]> {
    const ht = Signature.prototype.GetHashType.call(this);
    if (ht === hash.HashType_HashType_UNKNOWN) {
      return [false, $.newError("hash type missing")];
    }
    if ($.len(Signature.prototype.GetSigData.call(this)) === 0) {
      return [false, $.newError("signature empty")];
    }
    const hashErr = validateHashType(ht);
    if (hashErr != null) {
      return [false, hashErr];
    }
    const [hashed, hashSumErr] = await hash.Sum(ht, data);
    if (hashSumErr != null) {
      return [false, hashSumErr];
    }
    const signBody = buildSignBody(encContext, ht, $.pointerValue(hashed)!.GetHash());
    return pubKey.Verify(signBody, Signature.prototype.GetSigData.call(this));
  }

  async ParsePubKey(this: SignatureLike): Promise<[PeerPubKey | null, $.GoError]> {
    const pubKey = Signature.prototype.GetPubKey.call(this);
    if ($.len(pubKey) === 0) {
      return [null, null];
    }
    return crypto.UnmarshalPublicKey(pubKey);
  }

  MarshalVT(this: SignatureLike): [$.Bytes, $.GoError] {
    const out: number[] = [];
    appendBytesField(out, 1, Signature.prototype.GetPubKey.call(this));
    if (Signature.prototype.GetHashType.call(this) !== 0) {
      appendVarintField(out, 2, Signature.prototype.GetHashType.call(this));
    }
    appendBytesField(out, 3, Signature.prototype.GetSigData.call(this));
    return [new Uint8Array(out), null];
  }

  UnmarshalVT(data: $.Bytes): $.GoError {
    const decoder = new protoDecoder($.bytesToUint8Array(data));
    this.Reset();
    while (!decoder.done()) {
      const [tag, tagErr] = decoder.uvarint();
      if (tagErr != null) {
        return tagErr;
      }
      const field = tag >>> 3;
      const wire = tag & 7;
      if (field === 1 && wire === 2) {
        const [value, err] = decoder.bytes();
        if (err != null) return err;
        this.PubKey = value;
        continue;
      }
      if (field === 2 && wire === 0) {
        const [value, err] = decoder.uvarint();
        if (err != null) return err;
        this.HashType = value;
        continue;
      }
      if (field === 3 && wire === 2) {
        const [value, err] = decoder.bytes();
        if (err != null) return err;
        this.SigData = value;
        continue;
      }
      const skipErr = decoder.skip(wire);
      if (skipErr != null) {
        return skipErr;
      }
    }
    return null;
  }

  SizeVT(this: SignatureLike): number {
    const [data] = Signature.prototype.MarshalVT.call(this);
    return $.len(data);
  }

  CloneVT(this: SignatureLike): Signature | null {
    const sig = signatureValue(this);
    if (sig == null) {
      return null;
    }
    return new Signature({
      PubKey: copyBytes($.bytesToUint8Array(Signature.prototype.GetPubKey.call(sig))),
      HashType: Signature.prototype.GetHashType.call(sig),
      SigData: copyBytes($.bytesToUint8Array(Signature.prototype.GetSigData.call(sig))),
    });
  }

  CloneMessageVT(this: SignatureLike): Signature | null {
    return Signature.prototype.CloneVT.call(this);
  }

  EqualVT(this: SignatureLike, that: SignatureLike): boolean {
    if (this === that) {
      return true;
    }
    const sig = signatureValue(this);
    const other = signatureValue(that);
    if (sig == null || other == null) {
      return false;
    }
    return bytesEqual($.bytesToUint8Array(Signature.prototype.GetPubKey.call(sig)), $.bytesToUint8Array(Signature.prototype.GetPubKey.call(other))) &&
      Signature.prototype.GetHashType.call(sig) === Signature.prototype.GetHashType.call(other) &&
      bytesEqual($.bytesToUint8Array(Signature.prototype.GetSigData.call(sig)), $.bytesToUint8Array(Signature.prototype.GetSigData.call(other)));
  }

  EqualMessageVT(this: SignatureLike, thatMsg: unknown): boolean {
    return thatMsg instanceof Signature && Signature.prototype.EqualVT.call(this, thatMsg);
  }

  MarshalToSizedBufferVT(this: SignatureLike, data: $.Bytes): [number, $.GoError] {
    const [wire, err] = Signature.prototype.MarshalVT.call(this);
    if (err != null) {
      return [0, err];
    }
    const dst = $.bytesToUint8Array(data);
    dst.set($.bytesToUint8Array(wire), dst.length - $.len(wire));
    return [$.len(wire), null];
  }

  MarshalProtoJSON(_s: unknown): void {}
  UnmarshalProtoJSON(_s: unknown): void {}
  MarshalProtoText(_s?: unknown): string { return ""; }
}

export async function NewSignature(
  encContext: string,
  privKey: PeerPrivKey,
  hashType: HashType,
  data: $.Bytes,
  inclPubKey: boolean,
): Promise<[Signature | null, $.GoError]> {
  const [hashed, hashErr] = await hash.Sum(hashType, data);
  if (hashErr != null) {
    return [null, hashErr];
  }
  return NewSignatureWithHashedData(encContext, privKey, hashType, $.pointerValue(hashed)!.GetHash(), inclPubKey);
}

export async function NewSignatureWithHashedData(
  encContext: string,
  privKey: PeerPrivKey,
  hashType: HashType,
  hashData: $.Bytes,
  inclPubKey: boolean,
): Promise<[Signature | null, $.GoError]> {
  const hashErr = validateHashType(hashType);
  if (hashErr != null) {
    return [null, hashErr];
  }
  const signBody = buildSignBody(encContext, hashType, hashData);
  const [sigData, sigErr] = await privKey.Sign(signBody);
  if (sigErr != null) {
    return [null, sigErr];
  }
  const sig = new Signature({ HashType: hashType, SigData: sigData });
  if (inclPubKey) {
    const [pubKey, pubErr] = crypto.MarshalPublicKey(privKey.GetPublic());
    if (pubErr != null) {
      return [null, pubErr];
    }
    sig.PubKey = pubKey;
  }
  return [sig, null];
}

export class SignedMsg {
  public _fields: {
    FromPeerId: $.VarRef<string>;
    Signature: $.VarRef<Signature | null>;
    Data: $.VarRef<$.Bytes>;
  };

  constructor(init?: Partial<{ FromPeerId: string; Signature: Signature | null; Data: $.Bytes }>) {
    this._fields = {
      FromPeerId: $.varRef(init?.FromPeerId ?? ""),
      Signature: $.varRef(init?.Signature ?? null),
      Data: $.varRef(init?.Data ?? null),
    };
  }

  get FromPeerId(): string { return this._fields.FromPeerId.value; }
  set FromPeerId(value: string) { this._fields.FromPeerId.value = value; }
  get Signature(): Signature | null { return this._fields.Signature.value; }
  set Signature(value: Signature | null) { this._fields.Signature.value = value; }
  get Data(): $.Bytes { return this._fields.Data.value; }
  set Data(value: $.Bytes) { this._fields.Data.value = value; }

  Reset(): void { $.assignStruct(this, new SignedMsg()); }
  ProtoMessage(): void {}
  GetFromPeerId(): string { return this.FromPeerId; }
  GetSignature(): Signature {
    return this.Signature ?? new Signature();
  }
  GetData(): $.Bytes { return this.Data; }

  MarshalVT(): [$.Bytes, $.GoError] {
    const out: number[] = [];
    appendStringField(out, 1, this.GetFromPeerId());
    if (this.Signature != null) {
      const [sigData, sigErr] = this.Signature.MarshalVT();
      if (sigErr != null) {
        return [null, sigErr];
      }
      appendBytesField(out, 2, sigData);
    }
    appendBytesField(out, 3, this.GetData());
    return [new Uint8Array(out), null];
  }

  UnmarshalVT(data: $.Bytes): $.GoError {
    const decoder = new protoDecoder($.bytesToUint8Array(data));
    this.Reset();
    while (!decoder.done()) {
      const [tag, tagErr] = decoder.uvarint();
      if (tagErr != null) {
        return tagErr;
      }
      const field = tag >>> 3;
      const wire = tag & 7;
      if (field === 1 && wire === 2) {
        const [value, err] = decoder.bytes();
        if (err != null) return err;
        this.FromPeerId = $.bytesToString(value);
        continue;
      }
      if (field === 2 && wire === 2) {
        const [value, err] = decoder.bytes();
        if (err != null) return err;
        const sig = new Signature();
        const sigErr = sig.UnmarshalVT(value);
        if (sigErr != null) return sigErr;
        this.Signature = sig;
        continue;
      }
      if (field === 3 && wire === 2) {
        const [value, err] = decoder.bytes();
        if (err != null) return err;
        this.Data = value;
        continue;
      }
      const skipErr = decoder.skip(wire);
      if (skipErr != null) {
        return skipErr;
      }
    }
    return null;
  }

  SizeVT(): number {
    const [data] = this.MarshalVT();
    return $.len(data);
  }

  ComputeMessageID(): string {
    return hexEncode(joinBytes([$.bytesToUint8Array(this.GetSignature().GetSigData()), $.stringToBytes(this.GetFromPeerId())], null));
  }

  ParseFromPeerID(): [ID, $.GoError] {
    return IDB58Decode(this.GetFromPeerId());
  }

  async ExtractPubKey(): Promise<[PeerPubKey | null, ID, $.GoError]> {
    const [fromPeerID, peerErr] = this.ParseFromPeerID();
    if (peerErr != null) {
      return [null, "", peerErr];
    }
    const validateErr = ID_Validate(fromPeerID);
    if (validateErr != null) {
      return [null, "", $.newError("message peer id: " + validateErr.Error())];
    }
    const [pubKey, pubErr] = await ID_ExtractPublicKey(fromPeerID);
    if (pubErr != null) {
      return [null, fromPeerID, pubErr];
    }
    return [pubKey, fromPeerID, null];
  }

  async ExtractAndVerify(encContext: string): Promise<[PeerPubKey | null, ID, $.GoError]> {
    if ($.len(this.GetData()) === 0) {
      return [null, "", ErrEmptyBody];
    }
    if (this.GetFromPeerId().length === 0) {
      return [null, "", ErrEmptyPeerID];
    }
    const sigErr = await this.GetSignature().Validate();
    if (sigErr != null) {
      return [null, "", $.newError("message signature: " + sigErr.Error())];
    }
    const [pubKey, peerID, peerErr] = await this.ExtractPubKey();
    if (peerErr != null) {
      return [null, peerID, peerErr];
    }
    const verifyErr = await this.Verify(encContext, pubKey);
    if (verifyErr != null) {
      return [pubKey, peerID, verifyErr];
    }
    return [pubKey, peerID, null];
  }

  async Sign(encContext: string, privKey: PeerPrivKey, hashType: HashType): Promise<$.GoError> {
    if ($.len(this.GetData()) === 0) {
      return ErrEmptyBody;
    }
    const [sig, err] = await NewSignature(encContext, privKey, hashType, this.GetData(), false);
    if (err != null) {
      return err;
    }
    this.Signature = sig;
    return null;
  }

  async Verify(encContext: string, pubKey: PeerPubKey): Promise<$.GoError> {
    const [ok, sigErr] = await this.GetSignature().VerifyWithPublic(encContext, pubKey, this.GetData());
    if (!ok && sigErr == null) {
      return ErrSignatureInvalid;
    }
    return sigErr;
  }

  CloneVT(): SignedMsg {
    return new SignedMsg({
      FromPeerId: this.GetFromPeerId(),
      Signature: this.Signature?.CloneVT() ?? null,
      Data: copyBytes($.bytesToUint8Array(this.GetData())),
    });
  }

  CloneMessageVT(): SignedMsg {
    return this.CloneVT();
  }

  EqualVT(that: SignedMsg | $.VarRef<SignedMsg> | null): boolean {
    if (that == null) {
      return false;
    }
    const other = $.pointerValue(that);
    const signaturesEqual = this.Signature == null
      ? other.Signature == null
      : this.Signature.EqualVT(other.Signature);
    return this.GetFromPeerId() === other.GetFromPeerId() &&
      signaturesEqual &&
      bytesEqual($.bytesToUint8Array(this.GetData()), $.bytesToUint8Array(other.GetData()));
  }

  EqualMessageVT(thatMsg: unknown): boolean {
    return thatMsg instanceof SignedMsg && this.EqualVT(thatMsg);
  }

  MarshalToSizedBufferVT(data: $.Bytes): [number, $.GoError] {
    const [wire, err] = this.MarshalVT();
    if (err != null) {
      return [0, err];
    }
    const dst = $.bytesToUint8Array(data);
    dst.set($.bytesToUint8Array(wire), dst.length - $.len(wire));
    return [$.len(wire), null];
  }

  MarshalProtoJSON(_s: unknown): void {}
  UnmarshalProtoJSON(_s: unknown): void {}
  MarshalProtoText(_s?: unknown): string { return ""; }
}

export async function NewSignedMsg(
  encContext: string,
  privKey: PeerPrivKey,
  hashType: HashType,
  innerData: $.Bytes,
): Promise<[SignedMsg | null, $.GoError]> {
  const [peerID, peerErr] = IDFromPrivateKey(privKey);
  if (peerErr != null) {
    return [null, peerErr];
  }
  const msg = new SignedMsg({ FromPeerId: IDB58Encode(peerID), Data: innerData });
  const signErr = await msg.Sign(encContext, privKey, hashType);
  if (signErr != null) {
    return [null, signErr];
  }
  return [msg, null];
}

export function UnmarshalSignedMsg(data: $.Bytes): [SignedMsg | null, $.GoError] {
  const msg = new SignedMsg();
  const err = msg.UnmarshalVT(data);
  if (err != null) {
    return [null, err];
  }
  return [msg, null];
}

function validateHashType(hashType: HashType): $.GoError {
  switch (hashType) {
    case hash.HashType_HashType_UNKNOWN:
    case hash.HashType_HashType_SHA256:
    case hash.HashType_HashType_BLAKE3:
      return null;
    default:
      return $.newError("hash type unsupported in goscript: " + hashType);
  }
}

function buildSignBody(encContext: string, hashType: HashType, hashData: $.Bytes): Uint8Array {
  return joinBytes(
    [$.stringToBytes(encContext), $.stringToBytes(String(Math.trunc(hashType))), $.bytesToUint8Array(hashData)],
    signSeparator,
  );
}

function idBytes(id: ID): Uint8Array {
  if (typeof id === "string") {
    if (id.length === 0) {
      return new Uint8Array(0);
    }
    return $.stringToBytes(id);
  }
  return $.bytesToUint8Array(id);
}

function encodeMultihash(code: number, digest: Uint8Array): Uint8Array {
  return joinBytes([encodeUvarint(code), encodeUvarint(digest.length), digest], null);
}

function decodeMultihash(data: Uint8Array): [number, Uint8Array, $.GoError] {
  if (data.length === 0) {
    return [0, new Uint8Array(0), $.newError("multihash too short")];
  }
  const codeResult = readUvarint(data, 0);
  if (codeResult.err != null) {
    return [0, new Uint8Array(0), $.newError("invalid multihash varint")];
  }
  const lenResult = readUvarint(data, codeResult.next);
  if (lenResult.err != null) {
    return [0, new Uint8Array(0), $.newError("invalid multihash digest length varint")];
  }
  const digest = data.subarray(lenResult.next);
  if (digest.length !== lenResult.value) {
    return [
      0,
      new Uint8Array(0),
      $.newError(`multihash digest length mismatch: expected ${lenResult.value}, got ${digest.length}`),
    ];
  }
  return [codeResult.value, digest, null];
}

function encodeUvarint(value: number): Uint8Array {
  const out: number[] = [];
  let n = Math.trunc(value);
  while (n >= 0x80) {
    out.push((n & 0x7f) | 0x80);
    n = Math.floor(n / 0x80);
  }
  out.push(n);
  return new Uint8Array(out);
}

function readUvarint(data: Uint8Array, start: number): { value: number; next: number; err: $.GoError } {
  let value = 0;
  let shift = 1;
  for (let i = start; i < data.length; i++) {
    const b = data[i];
    if (b < 0x80) {
      return { value: value + b * shift, next: i + 1, err: null };
    }
    value += (b & 0x7f) * shift;
    shift *= 0x80;
  }
  return { value: 0, next: start, err: $.newError("invalid varint") };
}

function appendVarintField(out: number[], field: number, value: number): void {
  out.push(...encodeUvarint((field << 3) | 0));
  out.push(...encodeUvarint(value));
}

function appendBytesField(out: number[], field: number, value: $.Bytes): void {
  const bytes = $.bytesToUint8Array(value);
  if (bytes.length === 0) {
    return;
  }
  out.push(...encodeUvarint((field << 3) | 2));
  out.push(...encodeUvarint(bytes.length));
  out.push(...bytes);
}

function appendStringField(out: number[], field: number, value: string): void {
  if (value.length === 0) {
    return;
  }
  appendBytesField(out, field, $.stringToBytes(value));
}

class protoDecoder {
  private offset = 0;

  constructor(private readonly data: Uint8Array) {}

  done(): boolean {
    return this.offset >= this.data.length;
  }

  uvarint(): [number, $.GoError] {
    const result = readUvarint(this.data, this.offset);
    if (result.err != null) {
      return [0, result.err];
    }
    this.offset = result.next;
    return [result.value, null];
  }

  bytes(): [Uint8Array, $.GoError] {
    const [length, lenErr] = this.uvarint();
    if (lenErr != null) {
      return [new Uint8Array(0), lenErr];
    }
    if (this.offset + length > this.data.length) {
      return [new Uint8Array(0), ErrShortMessage];
    }
    const value = this.data.subarray(this.offset, this.offset + length);
    this.offset += length;
    return [value, null];
  }

  skip(wire: number): $.GoError {
    if (wire === 0) {
      const [, err] = this.uvarint();
      return err;
    }
    if (wire === 2) {
      const [, err] = this.bytes();
      return err;
    }
    return $.newError("unsupported protobuf wire type: " + wire);
  }
}

function joinBytes(parts: Uint8Array[], sep: Uint8Array | null): Uint8Array {
  let length = 0;
  for (let i = 0; i < parts.length; i++) {
    length += parts[i].length;
    if (sep != null && i !== 0) {
      length += sep.length;
    }
  }
  const out = new Uint8Array(length);
  let offset = 0;
  for (let i = 0; i < parts.length; i++) {
    if (sep != null && i !== 0) {
      out.set(sep, offset);
      offset += sep.length;
    }
    out.set(parts[i], offset);
    offset += parts[i].length;
  }
  return out;
}

function copyBytes(bytes: Uint8Array): Uint8Array {
  const out = new Uint8Array(bytes.length);
  out.set(bytes);
  return out;
}

function signatureValue(sig: SignatureLike): Signature | null {
  return sig == null ? null : $.pointerValue(sig);
}

function bytesEqual(a: Uint8Array, b: Uint8Array): boolean {
  if (a.length !== b.length) {
    return false;
  }
  for (let i = 0; i < a.length; i++) {
    if (a[i] !== b[i]) {
      return false;
    }
  }
  return true;
}

function compareBytes(a: Uint8Array, b: Uint8Array): number {
  const n = Math.min(a.length, b.length);
  for (let i = 0; i < n; i++) {
    if (a[i] < b[i]) return -1;
    if (a[i] > b[i]) return 1;
  }
  if (a.length < b.length) return -1;
  if (a.length > b.length) return 1;
  return 0;
}

function base58Encode(data: Uint8Array): string {
  if (data.length === 0) {
    return "";
  }
  const digits = [0];
  for (const byte of data) {
    let carry = byte;
    for (let j = 0; j < digits.length; j++) {
      carry += digits[j] << 8;
      digits[j] = carry % 58;
      carry = Math.floor(carry / 58);
    }
    while (carry > 0) {
      digits.push(carry % 58);
      carry = Math.floor(carry / 58);
    }
  }
  let out = "";
  for (const byte of data) {
    if (byte !== 0) break;
    out += "1";
  }
  for (let i = digits.length - 1; i >= 0; i--) {
    out += base58Alphabet[digits[i]];
  }
  return out;
}

function base58Decode(s: string): [Uint8Array, $.GoError] {
  if (s.length === 0) {
    return [new Uint8Array(0), null];
  }
  const bytes = [0];
  for (const ch of s) {
    const value = base58Alphabet.indexOf(ch);
    if (value < 0) {
      return [new Uint8Array(0), $.newError("invalid base58 character: " + ch)];
    }
    let carry = value;
    for (let j = 0; j < bytes.length; j++) {
      carry += bytes[j] * 58;
      bytes[j] = carry & 0xff;
      carry >>= 8;
    }
    while (carry > 0) {
      bytes.push(carry & 0xff);
      carry >>= 8;
    }
  }
  let leadingZeroes = 0;
  for (const ch of s) {
    if (ch !== "1") break;
    leadingZeroes++;
  }
  const out = new Uint8Array(leadingZeroes + bytes.length);
  for (let i = 0; i < bytes.length; i++) {
    out[out.length - 1 - i] = bytes[i];
  }
  return [out, null];
}

function hexEncode(data: Uint8Array): string {
  let out = "";
  for (const byte of data) {
    out += byte.toString(16).padStart(2, "0");
  }
  return out;
}
