import { EventEmitter } from "events";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  ApplyUpdateRequest,
  ApplyUpdateResponse,
  UpdateTarget,
} from "../../../desktop/update/update.pb.js";

const daemonQuitMocks = vi.hoisted(() => ({
  connect: vi.fn(),
  quit: vi.fn(),
  dispose: vi.fn(),
}));
const appUpdateMocks = vi.hoisted(() => ({
  applyRpc: vi.fn(),
  startHelper: vi.fn(),
}));

Reflect.set(globalThis, "BLDR_DEBUG", false);

const browserWindows: MockBrowserWindow[] = [];
const webRuntimeInstances: MockWebRuntime[] = [];
const mockElectronApp = {
  getAppPath() {
    return "/app";
  },
  getName: vi.fn(() => "Spacewave"),
  getPath: vi.fn(() => "/tmp/electron-state"),
  on: vi.fn(),
  quit: vi.fn(),
  requestSingleInstanceLock: vi.fn(() => true),
  setName: vi.fn(),
};

const mockDefaultSession = {
  on: vi.fn(),
};

class MockWebContents extends EventEmitter {
  private currentUrl = "";

  public readonly openDevTools = vi.fn();
  public readonly setWindowOpenHandler = vi.fn();
  public readonly downloadURL = vi.fn();

  public getURL() {
    return this.currentUrl;
  }

  public setURL(url: string) {
    this.currentUrl = url;
  }
}

class MockBrowserWindow extends EventEmitter {
  public readonly webContents = new MockWebContents();
  public readonly focus = vi.fn();
  public readonly show = vi.fn();
  public readonly isDestroyed = vi.fn(() => false);
  public readonly isMinimized = vi.fn(() => false);
  public readonly restore = vi.fn();
  public readonly opts: object;
  public readonly loadURL = vi.fn((url: string) => {
    this.webContents.setURL(url);
  });

  constructor(opts: object = {}) {
    super();
    this.opts = opts;
  }
}

class MockWebRuntime {
  public readonly removeConnection = vi.fn();
  public readonly invalidateClient = vi.fn((clientId: string) => {
    this.removeConnection(clientId);
  });
  public readonly openServiceWorkerHostStream = vi.fn();
  public readonly openWebDocumentHostStream = vi.fn();
  public readonly getWebRuntimeServer = vi.fn();
  public readonly handleClient = vi.fn();
  public readonly registerServerExtension = vi.fn();

  constructor(public readonly webRuntimeId: string) {
    webRuntimeInstances.push(this);
  }
}

class MockWebRuntimeHostClient {}

vi.mock("electron", () => {
  const Menu = {
    buildFromTemplate: vi.fn(() => ({})),
    setApplicationMenu: vi.fn(),
  };
  const protocol = {
    handle: vi.fn(),
    registerSchemesAsPrivileged: vi.fn(),
  };
  const shell = {
    openExternal: vi.fn(),
  };
  const ipcMain = {
    on: vi.fn(),
    handle: vi.fn(),
  };
  const dialog = {
    showOpenDialog: vi.fn(),
    showSaveDialog: vi.fn(),
    showMessageBox: vi.fn(),
  };
  const session = {
    defaultSession: mockDefaultSession,
  };
  const nativeTheme = {
    themeSource: "system",
  };
  const globalShortcut = {
    isRegistered: vi.fn(() => false),
  };
  class BrowserWindow extends MockBrowserWindow {
    constructor(opts: object = {}) {
      super(opts);
      browserWindows.push(this);
    }
  }
  return {
    default: {
      app: mockElectronApp,
      BrowserWindow,
      Menu,
      protocol,
      shell,
      ipcMain,
      dialog,
      nativeTheme,
      session,
      globalShortcut,
    },
    app: mockElectronApp,
    BrowserWindow,
    Menu,
    protocol,
    shell,
    ipcMain,
    dialog,
    nativeTheme,
    session,
    globalShortcut,
  };
});

vi.mock("starpc", () => ({
  Client: class {
    public readonly request = appUpdateMocks.applyRpc;
  },
  buildRpcStreamOpenStream: vi.fn(() => vi.fn()),
  OpenStreamCtr: class {
    public readonly openStreamFunc = vi.fn();
    public readonly set = vi.fn();
  },
  StreamConn: class {
    constructor() {}
    public readonly buildOpenStreamFunc = vi.fn();
  },
}));

vi.mock("../../document/document_srpc.pb.js", () => ({
  WebDocumentHostClient: class {
    public readonly WebViewRpc = vi.fn();
  },
}));

vi.mock("./app-update.js", () => ({
  startAppBundleUpdate: appUpdateMocks.startHelper,
}));

vi.mock("../../bldr/web-runtime.js", () => ({
  WebRuntime: MockWebRuntime,
}));

vi.mock("../../runtime/sw/sw_srpc.pb.js", () => ({
  ServiceWorkerHostClient: class {},
}));

vi.mock("../../runtime/runtime_srpc.pb.js", () => ({
  WebRuntimeHostClient: MockWebRuntimeHostClient,
}));

vi.mock("../../fetch/fetch.js", () => ({
  proxyFetch: vi.fn(),
}));

vi.mock("./ipc.js", () => ({
  messagePortMainToMessagePort: vi.fn(),
}));

vi.mock("../../../sdk/resource/unix-client.js", () => ({
  connectUnixResourceClient: daemonQuitMocks.connect,
}));

vi.mock("../../../desktop/control/control_srpc.pb.js", () => ({
  DesktopControlServiceClient: class {
    public QuitDesktop = daemonQuitMocks.quit;
  },
}));

vi.mock("./desktop-runtime.js", () => ({
  DesktopRuntimeResource: class {
    public readonly OpenOrFocusMainWindow = vi.fn(async () => ({}));
    public readonly QuitDesktopRuntime = vi.fn(async () => ({}));
    public readonly getState = vi.fn(() => ({
      mainWindowOpen: false,
      quitting: false,
      statusText: "Running",
    }));
    public readonly setMainWindowOpen = vi.fn();
    public readonly setQuitting = vi.fn();
    public readonly resourceServer = {};
    public readonly desktopCLIInstallResource = {
      recheck: vi.fn(async () => ({})),
    };
  },
}));

vi.mock("@go/github.com/aperturerobotics/util/pipesock/pipesock.js", () => ({
  buildPipeName: vi.fn(() => "/tmp/mock-pipe"),
  connectToPipe: vi.fn(() => ({
    on: vi.fn(),
  })),
}));

describe("BldrElectronApp", () => {
  afterEach(() => {
    vi.unstubAllEnvs();
  });

  beforeEach(() => {
    Reflect.set(globalThis, "BLDR_DEBUG", false);
    browserWindows.length = 0;
    webRuntimeInstances.length = 0;
    vi.clearAllMocks();
    mockElectronApp.quit.mockReset();
    mockElectronApp.requestSingleInstanceLock.mockReturnValue(true);
    appUpdateMocks.applyRpc.mockResolvedValue(
      ApplyUpdateResponse.toBinary({
        stagedPath: "/tmp/staged/Spacewave.app",
      }),
    );
    appUpdateMocks.startHelper.mockResolvedValue(undefined);
    vi.resetModules();
  });

  it("invalidates the document runtime client on main-frame reload", async () => {
    const { BldrElectronApp } = await import("./app.js");
    const app = Reflect.construct(BldrElectronApp, [
      mockElectronApp,
      "runtime-1",
      {},
    ]);
    await createWebDocument(app, "electron-init");

    const runtime = webRuntimeInstances[0];
    const win = browserWindows[0];
    expect(runtime).toBeDefined();
    expect(win).toBeDefined();

    win.webContents.emit("did-start-navigation", {
      isMainFrame: true,
      isSameDocument: false,
      url: "app://index.html?webDocumentId=electron-init",
    });
    win.webContents.emit("did-start-navigation", {
      isMainFrame: true,
      isSameDocument: false,
      url: "app://index.html?webDocumentId=electron-init",
    });

    expect(runtime.removeConnection).toHaveBeenCalledTimes(1);
    expect(runtime.removeConnection).toHaveBeenCalledWith("electron-init");
  });

  it("ignores same-document navigation and invalidates on renderer loss", async () => {
    const { BldrElectronApp } = await import("./app.js");
    const app = Reflect.construct(BldrElectronApp, [
      mockElectronApp,
      "runtime-1",
      {},
    ]);
    await createWebDocument(app, "electron-init");

    const runtime = webRuntimeInstances[0];
    const win = browserWindows[0];

    win.webContents.emit("did-start-navigation", {
      isMainFrame: true,
      isSameDocument: true,
      url: "app://index.html?webDocumentId=electron-init#/feed",
    });
    expect(runtime.removeConnection).not.toHaveBeenCalled();

    win.webContents.emit("render-process-gone", {}, { reason: "crashed" });
    expect(runtime.removeConnection).toHaveBeenCalledTimes(1);
    expect(runtime.removeConnection).toHaveBeenCalledWith("electron-init");
  });

  it("drops BrowserWindow ownership when the window closes", async () => {
    const { BldrElectronApp } = await import("./app.js");
    const app = Reflect.construct(BldrElectronApp, [
      mockElectronApp,
      "runtime-1",
      {},
    ]);
    await createWebDocument(app, "electron-init");

    const win = browserWindows[0];
    expect(getBrowserWindow(app, "electron-init")).toBe(win);

    win.emit("closed");

    expect(getBrowserWindow(app, "electron-init")).toBeUndefined();
  });

  it("does not open DevTools from release config alone", async () => {
    const { BldrElectronApp } = await import("./app.js");
    const app = Reflect.construct(BldrElectronApp, [
      mockElectronApp,
      "runtime-1",
      { devTools: true },
    ]);
    await createWebDocument(app, "electron-init");

    expect(browserWindows[0]?.webContents.openDevTools).not.toHaveBeenCalled();
  });

  it("shows windows only after the renderer finishes loading", async () => {
    const { BldrElectronApp } = await import("./app.js");
    const app = Reflect.construct(BldrElectronApp, [
      mockElectronApp,
      "runtime-1",
      {},
    ]);
    await createWebDocument(app, "electron-init");

    const win = browserWindows[0];
    expect(win?.opts).toMatchObject({ show: false });
    expect(win?.show).not.toHaveBeenCalled();

    win?.webContents.emit("did-finish-load");
    expect(win?.show).toHaveBeenCalledTimes(1);
  });

  it("opens DevTools only when debug build enables them", async () => {
    Reflect.set(globalThis, "BLDR_DEBUG", true);
    vi.resetModules();
    const { BldrElectronApp } = await import("./app.js");
    const app = Reflect.construct(BldrElectronApp, [
      mockElectronApp,
      "runtime-1",
      { devTools: true },
    ]);
    await createWebDocument(app, "electron-init");

    expect(browserWindows[0]?.webContents.openDevTools).toHaveBeenCalledTimes(
      1,
    );
  });

  it("quits the Electron main process when all windows are closed", async () => {
    const { BldrElectronApp } = await import("./app.js");
    const app = Reflect.construct(BldrElectronApp, [
      mockElectronApp,
      "runtime-1",
      {},
    ]);
    Reflect.apply(Reflect.get(app, "init"), app, []);

    const handler = getAppHandler("window-all-closed");
    handler();

    expect(mockElectronApp.quit).toHaveBeenCalledTimes(1);
  });

  it("keeps the Electron main process alive when tray background presence is enabled", async () => {
    const [{ DesktopPresencePolicy }, { BldrElectronApp }] = await Promise.all([
      import("../../plugin/electron/electron.pb.js"),
      import("./app.js"),
    ]);
    const app = Reflect.construct(BldrElectronApp, [
      mockElectronApp,
      "runtime-1",
      { desktopPresencePolicy: DesktopPresencePolicy.TRAY_BACKGROUND },
    ]);
    Reflect.apply(Reflect.get(app, "init"), app, []);

    const handler = getAppHandler("window-all-closed");
    handler();

    expect(mockElectronApp.quit).not.toHaveBeenCalled();
  });

  it("routes dock activation and second launch through the desktop runtime resource", async () => {
    const { BldrElectronApp } = await import("./app.js");
    const app = Reflect.construct(BldrElectronApp, [
      mockElectronApp,
      "runtime-1",
      {},
    ]);
    Reflect.apply(Reflect.get(app, "init"), app, []);

    getAppHandler("activate")();
    getAppHandler("second-instance")();

    const resource = Reflect.get(app, "desktopRuntimeResource");
    expect(resource.OpenOrFocusMainWindow).toHaveBeenCalledTimes(2);
  });

  it("registers the desktop runtime resource on the process-lifetime runtime server", async () => {
    const { BldrElectronApp } = await import("./app.js");
    const app = Reflect.construct(BldrElectronApp, [
      mockElectronApp,
      "runtime-1",
      {},
    ]);

    const runtime = webRuntimeInstances[0];
    const resource = Reflect.get(app, "desktopRuntimeResource");
    expect(runtime?.registerServerExtension).toHaveBeenCalledWith(
      resource.resourceServer,
    );
  });

  it("marks the desktop runtime as quitting for native quit paths", async () => {
    const { BldrElectronApp } = await import("./app.js");
    const app = Reflect.construct(BldrElectronApp, [
      mockElectronApp,
      "runtime-1",
      {},
    ]);
    Reflect.apply(Reflect.get(app, "init"), app, []);

    getAppHandler("before-quit")();

    const resource = Reflect.get(app, "desktopRuntimeResource");
    expect(resource.setQuitting).toHaveBeenCalledWith(true);
  });

  it("closes only the desktop shell for explicit desktop quit", async () => {
    const { BldrElectronApp } = await import("./app.js");
    const app = Reflect.construct(BldrElectronApp, [
      mockElectronApp,
      "runtime-1",
      {},
    ]);
    const quitDesktopRuntime = Reflect.get(app, "quitDesktopRuntime");

    await Reflect.apply(quitDesktopRuntime, app, []);

    expect(mockElectronApp.quit).toHaveBeenCalledOnce();
  });

  it("routes the macOS application-menu Quit through the daemon decision before shell exit", async () => {
    vi.stubEnv("SPACEWAVE_DESKTOP_DAEMON_SOCKET_PATH", "/tmp/desktop.sock");
    daemonQuitMocks.connect.mockResolvedValue({
      rpc: {},
      [Symbol.dispose]: daemonQuitMocks.dispose,
    });
    let requestStarted!: () => void;
    const started = new Promise<void>((resolve) => {
      requestStarted = resolve;
    });
    let decide!: (value: { otherWork: string[] }) => void;
    daemonQuitMocks.quit.mockImplementation(
      () =>
        new Promise((resolve) => {
          decide = resolve;
          requestStarted();
        }),
    );
    const [electron, { BldrElectronApp }] = await Promise.all([
      import("electron"),
      import("./app.js"),
    ]);
    const app = Reflect.construct(BldrElectronApp, [
      mockElectronApp,
      "runtime-1",
      {},
    ]);
    const resource = Reflect.get(app, "desktopRuntimeResource");
    let requested!: Promise<void>;
    resource.QuitDesktopRuntime.mockImplementation(() => {
      requested = Reflect.apply(
        Reflect.get(app, "quitDesktopRuntime"),
        app,
        [],
      );
      return requested;
    });
    Reflect.apply(Reflect.get(app, "init"), app, []);
    mockElectronApp.quit.mockImplementation(() => {
      getAppHandler("before-quit")();
    });
    getAppHandler("ready")();

    const template = vi.mocked(electron.Menu.buildFromTemplate).mock
      .calls[0]?.[0];
    const appMenu = template?.[0];
    const quitItem = Array.isArray(appMenu?.submenu)
      ? appMenu.submenu[8]
      : undefined;
    if (!quitItem?.click)
      throw new Error("macOS Quit menu action not registered");
    expect(quitItem.role).toBeUndefined();
    expect(quitItem.accelerator).toBe("Command+Q");

    Reflect.apply(quitItem.click, null, [{}, undefined, {}]);
    await started;
    expect(resource.QuitDesktopRuntime).toHaveBeenCalledOnce();
    expect(daemonQuitMocks.quit).toHaveBeenCalledOnce();
    expect(mockElectronApp.quit).not.toHaveBeenCalled();

    decide({ otherWork: ["glados"] });
    await requested;
    expect(electron.dialog.showMessageBox).toHaveBeenCalledWith(
      expect.objectContaining({
        detail: "Still using it: glados.",
      }),
    );
    expect(mockElectronApp.quit).toHaveBeenCalledOnce();
    expect(resource.QuitDesktopRuntime).toHaveBeenCalledOnce();
  });

  it("closes the macOS shell after the menu Quit claims an unused daemon", async () => {
    vi.stubEnv("SPACEWAVE_DESKTOP_DAEMON_SOCKET_PATH", "/tmp/desktop.sock");
    daemonQuitMocks.connect.mockResolvedValue({
      rpc: {},
      [Symbol.dispose]: daemonQuitMocks.dispose,
    });
    daemonQuitMocks.quit.mockResolvedValue({ otherWork: [] });
    const [electron, { BldrElectronApp }] = await Promise.all([
      import("electron"),
      import("./app.js"),
    ]);
    const app = Reflect.construct(BldrElectronApp, [
      mockElectronApp,
      "runtime-1",
      {},
    ]);
    const resource = Reflect.get(app, "desktopRuntimeResource");
    let requested!: Promise<void>;
    resource.QuitDesktopRuntime.mockImplementation(() => {
      requested = Reflect.apply(
        Reflect.get(app, "quitDesktopRuntime"),
        app,
        [],
      );
      return requested;
    });
    Reflect.apply(Reflect.get(app, "init"), app, []);
    mockElectronApp.quit.mockImplementation(() => {
      getAppHandler("before-quit")();
    });
    getAppHandler("ready")();

    const template = vi.mocked(electron.Menu.buildFromTemplate).mock
      .calls[0]?.[0];
    const appMenu = template?.[0];
    const quitItem = Array.isArray(appMenu?.submenu)
      ? appMenu.submenu[8]
      : undefined;
    if (!quitItem?.click)
      throw new Error("macOS Quit menu action not registered");
    Reflect.apply(quitItem.click, null, [{}, undefined, {}]);
    await requested;

    expect(daemonQuitMocks.quit).toHaveBeenCalledOnce();
    expect(daemonQuitMocks.dispose).toHaveBeenCalledOnce();
    expect(electron.dialog.showMessageBox).not.toHaveBeenCalled();
    expect(mockElectronApp.quit).toHaveBeenCalledOnce();
    expect(resource.QuitDesktopRuntime).toHaveBeenCalledOnce();
    expect(resource.setQuitting).toHaveBeenCalledWith(true);
  });

  it("reports the final busy decision before closing its desktop shell", async () => {
    vi.stubEnv("SPACEWAVE_DESKTOP_DAEMON_SOCKET_PATH", "/tmp/desktop.sock");
    daemonQuitMocks.connect.mockResolvedValue({
      rpc: {},
      [Symbol.dispose]: daemonQuitMocks.dispose,
    });
    let decide!: (value: { otherWork: string[] }) => void;
    daemonQuitMocks.quit.mockReturnValue(
      new Promise((resolve) => {
        decide = resolve;
      }),
    );
    const { BldrElectronApp } = await import("./app.js");
    const app = Reflect.construct(BldrElectronApp, [
      mockElectronApp,
      "runtime-1",
      {},
    ]);

    const quitting = Reflect.apply(
      Reflect.get(app, "quitDesktopRuntime"),
      app,
      [],
    );
    expect(mockElectronApp.quit).not.toHaveBeenCalled();
    decide({
      otherWork: [
        "glados",
        "spacewave (2)",
        "web listener at http://127.0.0.1:8080",
      ],
    });
    await quitting;

    expect(daemonQuitMocks.connect).toHaveBeenCalledWith(
      "unix:///tmp/desktop.sock",
      expect.any(AbortSignal),
    );
    expect(daemonQuitMocks.quit).toHaveBeenCalledOnce();
    expect(daemonQuitMocks.dispose).toHaveBeenCalledOnce();
    const { dialog } = await import("electron");
    expect(dialog.showMessageBox).toHaveBeenCalledWith(
      expect.objectContaining({
        detail:
          "Still using it: glados, spacewave (2), and web listener at http://127.0.0.1:8080.",
      }),
    );
    expect(mockElectronApp.quit).toHaveBeenCalledOnce();
  });

  it("closes the shell when the daemon claims stop but its Quit reply is lost", async () => {
    vi.stubEnv("SPACEWAVE_DESKTOP_DAEMON_SOCKET_PATH", "/tmp/desktop.sock");
    daemonQuitMocks.connect.mockResolvedValue({
      rpc: {},
      [Symbol.dispose]: daemonQuitMocks.dispose,
    });
    let loseReply!: (error: Error) => void;
    daemonQuitMocks.quit.mockReturnValue(
      new Promise((_resolve, reject) => {
        loseReply = reject;
      }),
    );
    const logError = vi.spyOn(console, "error").mockImplementation(() => {});
    try {
      const { BldrElectronApp } = await import("./app.js");
      const app = Reflect.construct(BldrElectronApp, [
        mockElectronApp,
        "runtime-1",
        {},
      ]);

      const quitting = Reflect.apply(
        Reflect.get(app, "quitDesktopRuntime"),
        app,
        [],
      );
      expect(mockElectronApp.quit).not.toHaveBeenCalled();
      loseReply(new Error("Quit reply lost after stop claim"));
      await quitting;

      expect(daemonQuitMocks.dispose).toHaveBeenCalledOnce();
      expect(mockElectronApp.quit).toHaveBeenCalledOnce();
      expect(logError).toHaveBeenCalledWith(
        "desktop Quit could not confirm daemon demand",
        expect.objectContaining({
          message: "Quit reply lost after stop claim",
        }),
      );
    } finally {
      logError.mockRestore();
    }
  });

  it("opens routed requests in new hash-routed windows", async () => {
    const { BldrElectronApp } = await import("./app.js");
    const app = Reflect.construct(BldrElectronApp, [
      mockElectronApp,
      "runtime-1",
      {},
    ]);
    const openOrFocusMainWindow = Reflect.get(app, "openOrFocusMainWindow");

    await Reflect.apply(openOrFocusMainWindow, app, [{ route: "/settings" }]);
    await Reflect.apply(openOrFocusMainWindow, app, [
      { route: "/spaces/space-1" },
    ]);
    await Reflect.apply(openOrFocusMainWindow, app, [
      { route: "#/spaces/space-1" },
    ]);

    expect(browserWindows).toHaveLength(3);
    expect(browserWindows[0]?.loadURL).toHaveBeenNthCalledWith(
      1,
      "app://index.html?webDocumentId=electron-route-1#/settings",
    );
    expect(browserWindows[1]?.loadURL).toHaveBeenNthCalledWith(
      1,
      "app://index.html?webDocumentId=electron-route-2#/spaces/space-1",
    );
    expect(browserWindows[2]?.loadURL).toHaveBeenNthCalledWith(
      1,
      "app://index.html?webDocumentId=electron-route-3#/spaces/space-1",
    );
    expect(browserWindows[0]?.loadURL).toHaveBeenCalledTimes(1);
    expect(browserWindows[1]?.loadURL).toHaveBeenCalledTimes(1);
    expect(browserWindows[2]?.loadURL).toHaveBeenCalledTimes(1);
  });

  it("opens one main window for concurrent requests without a route", async () => {
    const { BldrElectronApp } = await import("./app.js");
    const app = Reflect.construct(BldrElectronApp, [
      mockElectronApp,
      "runtime-1",
      {},
    ]);
    const openOrFocusMainWindow = Reflect.get(app, "openOrFocusMainWindow");

    await Promise.all([
      Reflect.apply(openOrFocusMainWindow, app, [{}]),
      Reflect.apply(openOrFocusMainWindow, app, [{}]),
    ]);

    expect(browserWindows).toHaveLength(1);
    expect(browserWindows[0]?.loadURL).toHaveBeenCalledWith(
      "app://index.html?webDocumentId=electron-init",
    );
    expect(browserWindows[0]?.loadURL).toHaveBeenCalledTimes(1);
    expect(browserWindows[0]?.show).toHaveBeenCalledTimes(1);
    expect(browserWindows[0]?.focus).toHaveBeenCalledTimes(1);
  });

  it("quits duplicate instances after failing the singleton lock", async () => {
    mockElectronApp.requestSingleInstanceLock.mockReturnValue(false);
    const { BldrElectronApp } = await import("./app.js");
    const app = Reflect.construct(BldrElectronApp, [
      mockElectronApp,
      "runtime-1",
      {},
    ]);

    Reflect.apply(Reflect.get(app, "init"), app, []);

    expect(mockElectronApp.quit).toHaveBeenCalledTimes(1);
    expect(mockElectronApp.on).not.toHaveBeenCalledWith(
      "window-all-closed",
      expect.any(Function),
    );
  });

  it("registers a native directory picker ipc handler", async () => {
    const [electron, { BldrElectronApp }] = await Promise.all([
      import("electron"),
      import("./app.js"),
    ]);
    const app = Reflect.construct(BldrElectronApp, [
      mockElectronApp,
      "runtime-1",
      {},
    ]);
    Reflect.apply(Reflect.get(app, "init"), app, []);

    const ready = getAppHandler("ready");
    ready();

    expect(electron.ipcMain.handle).toHaveBeenCalledWith(
      "BLDR_ELECTRON_OPEN_DIRECTORY",
      expect.any(Function),
    );
    const handler = vi
      .mocked(electron.ipcMain.handle)
      .mock.calls.find(
        ([channel]) => channel === "BLDR_ELECTRON_OPEN_DIRECTORY",
      )?.[1];
    if (!handler) throw new Error("directory picker handler not registered");
    vi.mocked(electron.dialog.showOpenDialog).mockResolvedValueOnce({
      canceled: true,
      filePaths: [],
    });

    await Reflect.apply(handler, null, [{}]);

    expect(electron.dialog.showOpenDialog).toHaveBeenCalledWith({
      properties: ["openDirectory", "showHiddenFiles"],
    });
  });

  it("registers native save dialog options for downloads", async () => {
    const { BldrElectronApp } = await import("./app.js");
    const app = Reflect.construct(BldrElectronApp, [
      mockElectronApp,
      "runtime-1",
      {},
    ]);
    Reflect.apply(Reflect.get(app, "init"), app, []);

    const ready = getAppHandler("ready");
    ready();

    const handler = getDownloadHandler();
    const item = {
      getFilename: vi.fn(() => "spacewave-export.zip"),
      setSaveDialogOptions: vi.fn(),
    };

    handler({}, item, {});

    expect(item.setSaveDialogOptions).toHaveBeenCalledWith({
      title: "Save Download",
      defaultPath: "spacewave-export.zip",
      buttonLabel: "Save",
    });
  });

  it("routes export navigations through Electron download handling", async () => {
    const { BldrElectronApp } = await import("./app.js");
    const app = Reflect.construct(BldrElectronApp, [
      mockElectronApp,
      "runtime-1",
      {},
    ]);
    await createWebDocument(app, "electron-init");

    const event = { preventDefault: vi.fn() };
    const win = browserWindows[0];
    win.webContents.emit(
      "will-navigate",
      event,
      "app://index.html/p/spacewave-core/export/u/1/so/space-1",
    );

    expect(event.preventDefault).toHaveBeenCalledTimes(1);
    expect(win.webContents.downloadURL).toHaveBeenCalledWith(
      "app://index.html/p/spacewave-core/export/u/1/so/space-1",
    );
    expect(win.loadURL).toHaveBeenCalledTimes(1);
  });

  it("routes export window opens through Electron download handling", async () => {
    const { BldrElectronApp } = await import("./app.js");
    const app = Reflect.construct(BldrElectronApp, [
      mockElectronApp,
      "runtime-1",
      {},
    ]);
    await createWebDocument(app, "electron-init");

    const win = browserWindows[0];
    const handler = win.webContents.setWindowOpenHandler.mock.calls[0]?.[0];
    if (!handler) throw new Error("window open handler not registered");

    const result = handler({
      url: "app://index.html/p/spacewave-core/export/u/1/so/space-1",
    });

    expect(result).toEqual({ action: "deny" });
    expect(win.webContents.downloadURL).toHaveBeenCalledWith(
      "app://index.html/p/spacewave-core/export/u/1/so/space-1",
    );
    expect(browserWindows).toHaveLength(1);
  });

  it("registers renderer ipc for explicit desktop quit", async () => {
    const [electron, { BldrElectronApp }] = await Promise.all([
      import("electron"),
      import("./app.js"),
    ]);
    const app = Reflect.construct(BldrElectronApp, [
      mockElectronApp,
      "runtime-1",
      {},
    ]);
    Reflect.apply(Reflect.get(app, "init"), app, []);

    const ready = getAppHandler("ready");
    ready();

    const handler = vi
      .mocked(electron.ipcMain.handle)
      .mock.calls.find(
        ([channel]) => channel === "BLDR_ELECTRON_QUIT_DESKTOP_RUNTIME",
      )?.[1];
    if (!handler) throw new Error("desktop quit handler not registered");

    await Reflect.apply(handler, null, [{}]);

    const resource = Reflect.get(app, "desktopRuntimeResource");
    expect(resource.QuitDesktopRuntime).toHaveBeenCalledWith({});
  });

  it("replaces the installed app that opened the desktop and quits Electron after its helper starts", async () => {
    const [electron, { BldrElectronApp }] = await Promise.all([
      import("electron"),
      import("./app.js"),
    ]);
    const app = Reflect.construct(BldrElectronApp, [
      mockElectronApp,
      "runtime-1",
      {},
    ]);
    Reflect.apply(Reflect.get(app, "init"), app, []);
    getAppHandler("ready")();
    await Reflect.apply(Reflect.get(app, "openOrFocusMainWindow"), app, [
      { route: "", installedApp: "/Applications/Spacewave.app" },
    ]);

    const handler = vi
      .mocked(electron.ipcMain.handle)
      .mock.calls.find(
        ([channel]) => channel === "BLDR_ELECTRON_APPLY_APP_UPDATE",
      )?.[1];
    if (!handler) throw new Error("app update handler not registered");
    const win = browserWindows[0];
    await Reflect.apply(handler, null, [
      { sender: win.webContents },
      "update-webview",
    ]);

    expect(appUpdateMocks.applyRpc).toHaveBeenCalledWith(
      "plugin/spacewave-launcher/spacewave.launcher.Launcher",
      "ApplyUpdate",
      ApplyUpdateRequest.toBinary({ target: UpdateTarget.APP }),
    );
    expect(appUpdateMocks.startHelper).toHaveBeenCalledWith(
      "/tmp/staged/Spacewave.app",
      "/Applications/Spacewave.app",
      "/tmp/electron-state",
      process.pid,
    );
    expect(mockElectronApp.quit).toHaveBeenCalledTimes(1);
  });

  it("does not expose exception details from the e2e control server", async () => {
    const { BldrElectronApp } = await import("./app.js");
    const app = Reflect.construct(BldrElectronApp, [
      mockElectronApp,
      "runtime-1",
      {},
    ]);
    const handleE2EControlRequest = Reflect.get(app, "handleE2EControlRequest");
    const res = new MockServerResponse();

    await Reflect.apply(handleE2EControlRequest, app, [
      new MockIncomingMessage("POST", "/desktop-state", ["{bad json"]),
      res,
    ]);

    expect(res.statusCode).toBe(500);
    expect(res.headers.get("content-type")).toBe("application/json");
    expect(JSON.parse(res.body)).toEqual({ error: "internal server error" });
    expect(res.body).not.toContain("JSON");
    expect(res.body).not.toContain("SyntaxError");
  });

  it("reports that no native shortcut owner claims the command keys", async () => {
    const [electron, { BldrElectronApp }] = await Promise.all([
      import("electron"),
      import("./app.js"),
    ]);
    const app = Reflect.construct(BldrElectronApp, [
      mockElectronApp,
      "runtime-1",
      {},
    ]);
    const handleE2EControlRequest = Reflect.get(app, "handleE2EControlRequest");
    const res = new MockServerResponse();

    await Reflect.apply(handleE2EControlRequest, app, [
      new MockIncomingMessage("GET", "/globalshortcut-state"),
      res,
    ]);

    expect(res.statusCode).toBe(200);
    expect(JSON.parse(res.body)).toEqual({
      leaderRegistered: false,
      paletteRegistered: false,
    });
    expect(electron.globalShortcut.isRegistered).toHaveBeenCalledWith(
      "Control+Space",
    );
    expect(electron.globalShortcut.isRegistered).toHaveBeenCalledWith(
      "CommandOrControl+K",
    );
  });
});

async function createWebDocument(app: object, id: string) {
  const create = Reflect.get(app, "createWebDocument");
  if (typeof create !== "function") {
    throw new Error("createWebDocument not found");
  }
  await Reflect.apply(create, app, [{ id }]);
}

function getBrowserWindow(app: object, id: string) {
  const windows = Reflect.get(app, "browserWindows");
  if (!windows || typeof windows !== "object") {
    throw new Error("browserWindows not found");
  }
  return Reflect.get(windows, id);
}

function getAppHandler(event: string) {
  const match = mockElectronApp.on.mock.calls.find(([name]) => name === event);
  const handler = match?.[1];
  if (typeof handler !== "function") {
    throw new Error(`${event} handler not found`);
  }
  return handler;
}

function getDownloadHandler() {
  const match = mockDefaultSession.on.mock.calls.find(
    ([name]) => name === "will-download",
  );
  const handler = match?.[1];
  if (typeof handler !== "function") {
    throw new Error("will-download handler not found");
  }
  return handler;
}

class MockIncomingMessage {
  constructor(
    public readonly method: string,
    public readonly url: string,
    private readonly chunks: string[] = [],
  ) {}

  public async *[Symbol.asyncIterator]() {
    for (const chunk of this.chunks) {
      yield Buffer.from(chunk);
    }
  }
}

class MockServerResponse {
  public statusCode = 0;
  public readonly headers = new Map<string, string>();
  public body = "";

  public setHeader(name: string, value: string) {
    this.headers.set(name.toLowerCase(), value);
  }

  public end(value: string) {
    this.body = value;
  }
}
