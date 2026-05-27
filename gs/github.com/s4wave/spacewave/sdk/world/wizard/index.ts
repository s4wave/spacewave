import * as $ from "@goscript/builtin/index.js";
import * as context from "@goscript/context/index.js";
import * as srpc from "@goscript/github.com/aperturerobotics/starpc/srpc/index.js";
import * as resource from "@goscript/github.com/s4wave/spacewave/bldr/resource/index.js";
import * as resource_server from "@goscript/github.com/s4wave/spacewave/bldr/resource/server/index.js";
import * as objecttype from "@goscript/github.com/s4wave/spacewave/sdk/world/objecttype/index.js";

export const CreateWizardObjectOpId = "spacewave/wizard/create";
export const WizardTypePrefix = "wizard/";

export const ErrWizardAlreadyRegistered = $.newError("wizard type id is already registered");
export const ErrWizardRequired = $.newError("wizard is required");
export const ErrWizardTypeIDRequired = $.newError("wizard type id is required");
export const ErrWizardPluginIDRequired = $.newError("wizard plugin id is required");
export const ErrWizardNameRequired = $.newError("wizard display name is required");

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

type MessageInit<T> = Partial<T> | null | undefined;

class MessageBase {
  SizeVT(): number {
    return 0;
  }

  MarshalToSizedBufferVT(_data: $.Slice<number>): [number, $.GoError] {
    return [0, null];
  }

  MarshalVT(): [$.Slice<number>, $.GoError] {
    return [new Uint8Array(0), null];
  }

  UnmarshalVT(_data: $.Slice<number>): $.GoError {
    return null;
  }

  Reset(): void {}
}

export class ObjectWizard extends MessageBase {
  public TypeId = "";
  public DisplayName = "";
  public Category = "";
  public IconName = "";
  public CreateOpId = "";
  public DefaultNamePattern = "";
  public KeyPrefix = "";
  public Persistent = false;
  public WizardTypeId = "";
  public PluginId = "";
  public Experimental = false;
  public RegistrationId = 0;

  constructor(init?: MessageInit<ObjectWizard>) {
    super();
    Object.assign(this, init ?? {});
  }

  GetTypeId(): string {
    return this.TypeId;
  }

  GetDisplayName(): string {
    return this.DisplayName;
  }

  GetPluginId(): string {
    return this.PluginId;
  }

  GetRegistrationId(): number {
    return this.RegistrationId;
  }

  CloneVT(): ObjectWizard {
    return new ObjectWizard({ ...this });
  }
}

export class RegisterWizardRequest extends MessageBase {
  public Wizard: ObjectWizard | null = null;

  constructor(init?: MessageInit<RegisterWizardRequest>) {
    super();
    Object.assign(this, init ?? {});
  }

  GetWizard(): ObjectWizard | null {
    return this.Wizard;
  }
}

export class RegisterWizardResponse extends MessageBase {
  public ResourceId = 0;

  constructor(init?: MessageInit<RegisterWizardResponse>) {
    super();
    Object.assign(this, init ?? {});
  }
}

export class ListWizardsRequest extends MessageBase {}
export class WatchWizardsRequest extends MessageBase {}

export class ListWizardsResponse extends MessageBase {
  public Wizards: $.Slice<ObjectWizard | null> = null;

  constructor(init?: MessageInit<ListWizardsResponse>) {
    super();
    Object.assign(this, init ?? {});
  }
}

export class WatchWizardsResponse extends MessageBase {
  public Wizards: $.Slice<ObjectWizard | null> = null;

  constructor(init?: MessageInit<WatchWizardsResponse>) {
    super();
    Object.assign(this, init ?? {});
  }
}

export type SRPCObjectWizardRegistryResourceServiceServer = {
  RegisterWizard(
    ctx: context.Context | null,
    req: RegisterWizardRequest | null,
  ): [RegisterWizardResponse | null, $.GoError] | Promise<[RegisterWizardResponse | null, $.GoError]>;
  ListWizards(
    ctx: context.Context | null,
    req: ListWizardsRequest | null,
  ): [ListWizardsResponse | null, $.GoError] | Promise<[ListWizardsResponse | null, $.GoError]>;
  WatchWizards(
    req: WatchWizardsRequest | null,
    stream: SRPCObjectWizardRegistryResourceService_WatchWizardsStream | null,
  ): $.GoError | Promise<$.GoError>;
};

export type SRPCObjectWizardRegistryResourceService_WatchWizardsStream = {
  Context(): context.Context | null;
  Send(resp: WatchWizardsResponse | null): $.GoError | Promise<$.GoError>;
};

type wizardRegistryState = {
  nextID: number;
  registrations: Map<number, ObjectWizard>;
};

const defaultWizardRegistryState: wizardRegistryState = {
  nextID: 1,
  registrations: new Map(),
};

export const ObjectWizards: $.Slice<ObjectWizard | null> = $.arrayToSlice([
  new ObjectWizard({
    TypeId: "alpha/object-layout",
    DisplayName: "Object Layout",
    Category: "Layout",
    IconName: "LuLayoutGrid",
    CreateOpId: "space/world/init-object-layout",
    DefaultNamePattern: "Layout",
    KeyPrefix: "object-layout/",
  }),
  new ObjectWizard({
    TypeId: "canvas",
    DisplayName: "Canvas",
    Category: "Layout",
    IconName: "LuLayoutGrid",
    CreateOpId: "space/world/init-canvas",
    DefaultNamePattern: "Canvas",
    KeyPrefix: "canvas/",
  }),
  new ObjectWizard({
    TypeId: "unixfs/fs-node",
    DisplayName: "Filesystem",
    Category: "Files",
    IconName: "LuHardDrive",
    CreateOpId: "space/world/init-unixfs",
    DefaultNamePattern: "Files",
    KeyPrefix: "fs/",
  }),
]);

export class WizardRegistryResource implements SRPCObjectWizardRegistryResourceServiceServer {
  private mux: srpc.Mux;
  private state = defaultWizardRegistryState;

  constructor() {
    this.mux = resource_server.NewResourceMux((mux) =>
      SRPCRegisterObjectWizardRegistryResourceService(mux, this),
    );
  }

  GetMux(): srpc.Mux {
    return this.mux;
  }

  RegisterWizard(
    ctx: context.Context | null,
    req: RegisterWizardRequest | null,
  ): [RegisterWizardResponse | null, $.GoError] {
    const wizard = req?.GetWizard() ?? null;
    if (wizard == null) {
      return [null, ErrWizardRequired];
    }
    if (wizard.GetTypeId() === "") {
      return [null, ErrWizardTypeIDRequired];
    }
    if (wizard.GetPluginId() === "") {
      return [null, ErrWizardPluginIDRequired];
    }
    if (wizard.GetDisplayName() === "") {
      return [null, ErrWizardNameRequired];
    }
    for (const existing of this.state.registrations.values()) {
      if (existing.GetTypeId() === wizard.GetTypeId()) {
        return [null, ErrWizardAlreadyRegistered];
      }
    }

    const regID = this.state.nextID++;
    const stored = wizard.CloneVT();
    stored.RegistrationId = regID;
    this.state.registrations.set(regID, stored);

    const client = resource_server.GetResourceClientContext(ctx);
    if (client == null) {
      this.state.registrations.delete(regID);
      return [null, resource.ErrNoResourceClientContext];
    }
    const [resourceID, err] = client.AddResource(srpc.NewMux(), () => {
      this.state.registrations.delete(regID);
    });
    if (err != null) {
      this.state.registrations.delete(regID);
      return [null, err];
    }
    return [new RegisterWizardResponse({ ResourceId: resourceID }), null];
  }

  ListWizards(
    _ctx: context.Context | null,
    _req: ListWizardsRequest | null,
  ): [ListWizardsResponse | null, $.GoError] {
    return [new ListWizardsResponse({ Wizards: this.getWizards() }), null];
  }

  async WatchWizards(
    _req: WatchWizardsRequest | null,
    stream: SRPCObjectWizardRegistryResourceService_WatchWizardsStream | null,
  ): Promise<$.GoError> {
    if (stream == null) {
      return null;
    }
    return await stream.Send(new WatchWizardsResponse({ Wizards: this.getWizards() }));
  }

  private getWizards(): $.Slice<ObjectWizard | null> {
    const seen = new Set<string>();
    const wizards: (ObjectWizard | null)[] = [];
    for (const wizard of ObjectWizards ?? []) {
      if (wizard == null || wizard.GetTypeId() === "") {
        continue;
      }
      seen.add(wizard.GetTypeId());
      wizards.push(wizard.CloneVT());
    }
    const regs = [...this.state.registrations.values()].sort((a, b) => {
      if (a.GetTypeId() === b.GetTypeId()) {
        return a.GetRegistrationId() - b.GetRegistrationId();
      }
      return a.GetTypeId() < b.GetTypeId() ? -1 : 1;
    });
    for (const wizard of regs) {
      if (seen.has(wizard.GetTypeId())) {
        continue;
      }
      seen.add(wizard.GetTypeId());
      wizards.push(wizard.CloneVT());
    }
    return $.arrayToSlice(wizards);
  }
}

export function NewWizardRegistryResource(): WizardRegistryResource {
  return new WizardRegistryResource();
}

class objectWizardRegistryResourceServiceHandler implements srpc.Handler {
  constructor(
    private impl: SRPCObjectWizardRegistryResourceServiceServer | null,
    private serviceID: string,
  ) {}

  GetServiceID(): string {
    return this.serviceID;
  }

  GetMethodIDs(): $.Slice<string> {
    return $.arrayToSlice(["RegisterWizard", "ListWizards", "WatchWizards"]);
  }

  async InvokeMethod(
    serviceID: string,
    methodID: string,
    stream: srpc.Stream | null,
  ): Promise<[boolean, $.GoError]> {
    if (serviceID !== "" && serviceID !== this.serviceID) {
      return [false, null];
    }
    if (this.impl == null) {
      return [true, resource.ErrResourceNotFound];
    }
    switch (methodID) {
      case "RegisterWizard": {
        const req = new RegisterWizardRequest();
        if (stream != null) {
          const recvErr = await stream.MsgRecv(req);
          if (recvErr != null) {
            return [true, recvErr];
          }
        }
        const [resp, err] = await this.impl.RegisterWizard(stream?.Context() ?? context.Background(), req);
        if (err != null || stream == null) {
          return [true, err];
        }
        return [true, await stream.MsgSend(resp)];
      }
      case "ListWizards": {
        const req = new ListWizardsRequest();
        if (stream != null) {
          const recvErr = await stream.MsgRecv(req);
          if (recvErr != null) {
            return [true, recvErr];
          }
        }
        const [resp, err] = await this.impl.ListWizards(stream?.Context() ?? context.Background(), req);
        if (err != null || stream == null) {
          return [true, err];
        }
        return [true, await stream.MsgSend(resp)];
      }
      case "WatchWizards": {
        const req = new WatchWizardsRequest();
        if (stream != null) {
          const recvErr = await stream.MsgRecv(req);
          if (recvErr != null) {
            return [true, recvErr];
          }
        }
        return [true, await this.impl.WatchWizards(req, {
          Context: () => stream?.Context() ?? context.Background(),
          Send: (resp) => stream == null ? null : stream.MsgSend(resp),
        })];
      }
      default:
        return [false, null];
    }
  }
}

export const SRPCObjectWizardRegistryResourceServiceServiceID =
  "s4wave.wizard.ObjectWizardRegistryResourceService";

export function NewSRPCObjectWizardRegistryResourceServiceHandler(
  impl: SRPCObjectWizardRegistryResourceServiceServer | null,
  serviceID: string,
): srpc.Handler {
  return new objectWizardRegistryResourceServiceHandler(
    impl,
    serviceID === "" ? SRPCObjectWizardRegistryResourceServiceServiceID : serviceID,
  );
}

export function SRPCRegisterObjectWizardRegistryResourceService(
  mux: srpc.Mux | null,
  impl: SRPCObjectWizardRegistryResourceServiceServer | null,
): $.GoError {
  return mux == null
    ? resource.ErrResourceNotFound
    : mux.Register(NewSRPCObjectWizardRegistryResourceServiceHandler(impl, ""));
}

export function LookupObjectWizard(typeID: string): ObjectWizard | null {
  for (const wizard of ObjectWizards ?? []) {
    if (wizard != null && wizard.GetTypeId() === typeID) {
      return wizard.CloneVT();
    }
  }
  return null;
}

function WizardFactory(): [srpc.Invoker | null, (() => void) | null, $.GoError] {
  return [null, null, $.newError("wizard object resources are not supported under goscript")];
}

export async function LookupWizardObjectType(
  _ctx: context.Context | null,
  typeID: string,
): Promise<[objecttype.ObjectType | null, $.GoError]> {
  if (!typeID.startsWith(WizardTypePrefix)) {
    return [null, null];
  }
  return [await objecttype.NewObjectType(typeID, WizardFactory), null];
}
