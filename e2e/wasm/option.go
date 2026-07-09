//go:build !js

package wasm

import (
	"os"
	"strings"
	"time"

	"github.com/aperturerobotics/util/enabled"
	"github.com/pkg/errors"
	manifest_build "github.com/s4wave/spacewave/bldr/manifest/build"
	bldr_plugin_compiler_go "github.com/s4wave/spacewave/bldr/plugin/compiler/go"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
	"github.com/s4wave/spacewave/bldr/util/gocompiler"
	"github.com/s4wave/spacewave/e2e/wasm/internal/configjson"
	e2e_wasm_session "github.com/s4wave/spacewave/e2e/wasm/session"
)

const (
	// E2EWasmCompilerEnv selects the browser Go compiler for app tests.
	E2EWasmCompilerEnv = "E2E_WASM_COMPILER"
	// E2EWasmLegacyTinyGoEnv was the old boolean TinyGo selector.
	E2EWasmLegacyTinyGoEnv = "E2E_WASM_TINYGO"
	// E2EWasmWorkerModeEnv selects the browser worker topology for local tests.
	E2EWasmWorkerModeEnv = "E2E_WASM_WORKER_MODE"
	// E2EWasmTinyGoProfileEnv selects the local TinyGo build profile.
	E2EWasmTinyGoProfileEnv = "E2E_WASM_TINYGO_PROFILE"
	// E2EWasmTinyGoOptEnv overrides the local TinyGo optimization level.
	E2EWasmTinyGoOptEnv = "E2E_WASM_TINYGO_OPT"
	// E2EWasmTinyGoPanicEnv overrides the local TinyGo panic strategy.
	E2EWasmTinyGoPanicEnv = "E2E_WASM_TINYGO_PANIC"
	// E2EWasmTinyGoGCEnv overrides the local TinyGo garbage collector.
	E2EWasmTinyGoGCEnv = "E2E_WASM_TINYGO_GC"
	// E2EWasmTinyGoSchedulerEnv overrides the local TinyGo scheduler.
	E2EWasmTinyGoSchedulerEnv = "E2E_WASM_TINYGO_SCHEDULER"
	// E2EWasmTinyGoLLVMFeaturesEnv overrides the local TinyGo LLVM feature set.
	E2EWasmTinyGoLLVMFeaturesEnv = "E2E_WASM_TINYGO_LLVM_FEATURES"
	// E2EWasmTinyGoInterpTimeoutEnv overrides TinyGo's interp timeout locally.
	E2EWasmTinyGoInterpTimeoutEnv = "E2E_WASM_TINYGO_INTERP_TIMEOUT"
	// E2EWasmManifestBuildTimeoutEnv overrides the local startup Manifest build wait.
	E2EWasmManifestBuildTimeoutEnv = "E2E_WASM_MANIFEST_BUILD_TIMEOUT"
	// E2EWasmGoScriptRuntimeTraceEnv opts GoScript browser runs into
	// runtime-trace capture; off by default so routine GoScript e2e stays fast.
	E2EWasmGoScriptRuntimeTraceEnv = "E2E_WASM_GOSCRIPT_RUNTIME_TRACE"
	// E2EWasmDriveBenchEnv opts in the time-to-Drive startup bench; off by
	// default so routine e2e does not pay the bench measurement cost.
	E2EWasmDriveBenchEnv = "E2E_WASM_DRIVE_BENCH"
	// E2EWasmDriveBenchJSProfileEnv opts the Drive bench into same-window
	// Chromium JS CPU profile capture; off by default so routine e2e stays cheap.
	E2EWasmDriveBenchJSProfileEnv = "E2E_WASM_DRIVE_BENCH_JS_PROFILE"
)

var (
	goScriptBrowserGoManifests = []string{
		"spacewave-launcher",
		"spacewave-core",
		"spacewave-sql",
	}
	goScriptBrowserStartPlugins = []string{
		"spacewave-launcher",
		"spacewave-core",
		"spacewave-web",
		"spacewave-app",
		"web",
	}
)

// E2EWasmCompiler selects the browser Go compiler used by e2e/wasm.
type E2EWasmCompiler string

const (
	// E2EWasmCompilerGo keeps the default Bldr browser Go compiler.
	E2EWasmCompilerGo E2EWasmCompiler = "go"
	// E2EWasmCompilerTinyGo selects TinyGo for spacewave-core.
	E2EWasmCompilerTinyGo E2EWasmCompiler = "tinygo"
	// E2EWasmCompilerGoScript selects GoScript for browser launcher/core.
	E2EWasmCompilerGoScript E2EWasmCompiler = "goscript"
)

// WorkerMode selects the browser worker topology used by the harness.
type WorkerMode string

const (
	// WorkerModeDedicated forces dedicated workers for local debugging.
	WorkerModeDedicated WorkerMode = "dedicated"
	// WorkerModeShared keeps SharedWorker topology when the browser supports it.
	WorkerModeShared WorkerMode = "shared"
)

// Option configures the Harness.
type Option func(*options)

type options struct {
	repoRoot                  string
	headless                  *bool
	browserName               string
	workerMode                WorkerMode
	manifestBuildTimeout      time.Duration
	preserveStartupBuildCache *bool
	configMutators            []func(*bldr_project.ProjectConfig) error
}

// WithRepoRoot overrides automatic repo root discovery.
func WithRepoRoot(root string) Option {
	return func(o *options) {
		o.repoRoot = root
	}
}

// WithHeadless controls whether the browser runs headless (default true).
func WithHeadless(headless bool) Option {
	return func(o *options) {
		o.headless = &headless
	}
}

// WithBrowserName chooses the Playwright browser type used by LaunchBrowser.
// Supported values are "chromium", "firefox", and "webkit".
func WithBrowserName(name string) Option {
	return func(o *options) {
		o.browserName = name
	}
}

// WithWorkerMode chooses whether the browser runtime uses dedicated workers or
// SharedWorkers.
func WithWorkerMode(mode WorkerMode) Option {
	return func(o *options) {
		o.workerMode = mode
	}
}

// WithManifestBuildTimeout controls how long Boot waits for startup Manifest
// builds before failing.
func WithManifestBuildTimeout(timeout time.Duration) Option {
	return func(o *options) {
		o.manifestBuildTimeout = timeout
	}
}

// WithStartupBuildCache controls whether Boot preserves the world-backed
// startup Manifest build cache across harness processes.
func WithStartupBuildCache(enabled bool) Option {
	return func(o *options) {
		o.preserveStartupBuildCache = &enabled
	}
}

// WithConfigMutator registers a function that mutates the loaded project
// config before the project controller starts. Use this to inject test-only
// controller wiring such as trace service entries.
func WithConfigMutator(fn func(*bldr_project.ProjectConfig) error) Option {
	return func(o *options) {
		o.configMutators = append(o.configMutators, fn)
	}
}

// WithSessionHarness injects the session harness controller into the
// plugin WASM processes for test orchestration (peer info, signaling
// relay, link establishment).
func WithSessionHarness() Option {
	return WithConfigMutator(e2e_wasm_session.InjectSessionHarnessConfig)
}

// ResolveE2EWasmCompiler resolves the browser Go compiler for local harness
// runs. The unset default matches Bldr web builds: GoScript in browser plugins.
func ResolveE2EWasmCompiler() (E2EWasmCompiler, error) {
	legacyTinyGo := strings.TrimSpace(os.Getenv(E2EWasmLegacyTinyGoEnv))
	if legacyTinyGo != "" {
		return "", errors.Errorf("%s is no longer supported; use %s=tinygo", E2EWasmLegacyTinyGoEnv, E2EWasmCompilerEnv)
	}

	raw := strings.ToLower(strings.TrimSpace(os.Getenv(E2EWasmCompilerEnv)))
	switch raw {
	case "":
		return E2EWasmCompilerGoScript, nil
	case string(E2EWasmCompilerGo):
		return E2EWasmCompilerGo, nil
	case string(E2EWasmCompilerTinyGo):
		return E2EWasmCompilerTinyGo, nil
	case string(E2EWasmCompilerGoScript):
		return E2EWasmCompilerGoScript, nil
	default:
		return "", errors.Errorf("unsupported %s value %q, expected go, tinygo, or goscript", E2EWasmCompilerEnv, raw)
	}
}

// E2EWasmTraceServiceEnabled reports whether the trace service should be
// injected into the app harness config. Native Go always carries it; GoScript
// opts in via E2E_WASM_GOSCRIPT_RUNTIME_TRACE so routine GoScript e2e does not
// pay the trace-service compile cost; TinyGo never carries it.
func E2EWasmTraceServiceEnabled(compiler E2EWasmCompiler) bool {
	switch compiler {
	case E2EWasmCompilerGo:
		return true
	case E2EWasmCompilerGoScript:
		return E2EWasmGoScriptRuntimeTraceEnabled()
	default:
		return false
	}
}

// E2EWasmGoScriptRuntimeTraceEnabled reports whether GoScript browser runs opt
// into runtime-trace capture via E2E_WASM_GOSCRIPT_RUNTIME_TRACE.
func E2EWasmGoScriptRuntimeTraceEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(E2EWasmGoScriptRuntimeTraceEnv))) {
	case "true", "1", "yes", "on":
		return true
	default:
		return false
	}
}

// E2EWasmDriveBenchEnabled reports whether the time-to-Drive startup bench runs,
// gated by E2E_WASM_DRIVE_BENCH so routine e2e skips the measurement.
func E2EWasmDriveBenchEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(E2EWasmDriveBenchEnv))) {
	case "true", "1", "yes", "on":
		return true
	default:
		return false
	}
}

// E2EWasmDriveBenchJSProfileEnabled reports whether the Drive bench should
// capture a same-window Chromium JS CPU profile.
func E2EWasmDriveBenchJSProfileEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(E2EWasmDriveBenchJSProfileEnv))) {
	case "true", "1", "yes", "on":
		return true
	default:
		return false
	}
}

// E2EWasmSlowCompilerEnabled reports whether app readiness should allow the
// longer browser boot budget used by alternate browser compilers.
func E2EWasmSlowCompilerEnabled() bool {
	compiler, err := ResolveE2EWasmCompiler()
	return err == nil && compiler != E2EWasmCompilerGo
}

// ResolveE2EWasmWorkerMode resolves the local worker topology.
func ResolveE2EWasmWorkerMode(explicit WorkerMode) (WorkerMode, error) {
	if explicit != "" {
		return normalizeWorkerMode(string(explicit))
	}
	return normalizeWorkerMode(os.Getenv(E2EWasmWorkerModeEnv))
}

// ResolveE2EWasmManifestBuildTimeout resolves the startup Manifest build wait
// for local harness runs.
func ResolveE2EWasmManifestBuildTimeout(defaultTimeout time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(E2EWasmManifestBuildTimeoutEnv))
	if raw == "" {
		return defaultTimeout, nil
	}
	timeout, err := time.ParseDuration(raw)
	if err != nil {
		return 0, errors.Wrapf(err, "unsupported %s value %q", E2EWasmManifestBuildTimeoutEnv, raw)
	}
	if timeout <= 0 {
		return 0, errors.Errorf("unsupported %s value %q: must be positive", E2EWasmManifestBuildTimeoutEnv, raw)
	}
	return timeout, nil
}

// ApplyE2EWasmTinyGoCompilerEnv applies the local TinyGo profile to Bldr's
// TinyGo compiler env before the startup Manifest builder runs.
func ApplyE2EWasmTinyGoCompilerEnv() error {
	profile := strings.TrimSpace(os.Getenv(E2EWasmTinyGoProfileEnv))
	if profile == "" {
		profile = strings.TrimSpace(os.Getenv(gocompiler.TinyGoProfileEnv))
	}
	if profile == "" {
		profile = gocompiler.TinyGoProfileFast
	}
	if err := os.Setenv(gocompiler.TinyGoProfileEnv, profile); err != nil {
		return errors.Wrap(err, "set TinyGo profile")
	}

	if err := copyOptionalTinyGoEnv(E2EWasmTinyGoOptEnv, gocompiler.TinyGoOptEnv); err != nil {
		return err
	}
	if err := copyOptionalTinyGoEnv(E2EWasmTinyGoPanicEnv, gocompiler.TinyGoPanicStrategyEnv); err != nil {
		return err
	}
	if err := copyOptionalTinyGoEnv(E2EWasmTinyGoGCEnv, gocompiler.TinyGoGCEnv); err != nil {
		return err
	}
	if profile == gocompiler.TinyGoProfileFast && strings.TrimSpace(os.Getenv(gocompiler.TinyGoGCEnv)) == "" {
		if err := os.Setenv(gocompiler.TinyGoGCEnv, "leaking"); err != nil {
			return errors.Wrap(err, "set TinyGo GC")
		}
	}
	if err := copyOptionalTinyGoEnv(E2EWasmTinyGoSchedulerEnv, gocompiler.TinyGoSchedulerEnv); err != nil {
		return err
	}
	if err := copyOptionalTinyGoEnv(E2EWasmTinyGoLLVMFeaturesEnv, gocompiler.TinyGoLLVMFeaturesEnv); err != nil {
		return err
	}
	if err := copyOptionalTinyGoEnv(E2EWasmTinyGoInterpTimeoutEnv, gocompiler.TinyGoInterpTimeoutEnv); err != nil {
		return err
	}

	if _, err := gocompiler.GetDefaultTinygoArgs(); err != nil {
		return err
	}
	return nil
}

// E2EWasmStartupBuildCacheEnabled reports whether the harness should preserve
// world-backed startup Manifest build results between test process boots.
func ResolveE2EWasmStartupBuildCacheEnabled() (bool, error) {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("E2E_WASM_STARTUP_BUILD_CACHE"))) {
	case "true", "1", "yes", "on":
		return true, nil
	case "false", "0", "no", "off":
		return false, nil
	default:
		compiler, err := ResolveE2EWasmCompiler()
		if err != nil {
			return false, err
		}
		return compiler == E2EWasmCompilerTinyGo, nil
	}
}

// E2EWasmStartupBuildCacheEnabled reports whether the harness should preserve
// world-backed startup Manifest build results between test process boots.
func E2EWasmStartupBuildCacheEnabled() bool {
	enabled, err := ResolveE2EWasmStartupBuildCacheEnabled()
	return err == nil && enabled
}

// WithTinyGoCore enables TinyGo for the browser spacewave-core Manifest.
func WithTinyGoCore() Option {
	return WithConfigMutator(ConfigureTinyGoForManifest("spacewave-core"))
}

// WithGoScriptCore enables GoScript for the browser spacewave-core Manifest.
func WithGoScriptCore() Option {
	return WithConfigMutator(ConfigureGoScriptForManifest("spacewave-core"))
}

// WithGoScriptBrowserStartup enables the production-shaped browser GoScript
// startup surface used by staging: launcher plus core, without dev-only debug.
func WithGoScriptBrowserStartup() Option {
	return WithConfigMutator(ConfigureGoScriptBrowserStartup)
}

// EnableTinyGoForManifest enables TinyGo for a Go plugin Manifest's web
// platform override.
func EnableTinyGoForManifest(manifestID string) func(*bldr_project.ProjectConfig) error {
	return EnableGoCompilerForManifest(manifestID, bldr_plugin_compiler_go.GoCompiler_GO_COMPILER_TINYGO)
}

// EnableGoScriptForManifest enables GoScript for a Go plugin Manifest's web
// platform override.
func EnableGoScriptForManifest(manifestID string) func(*bldr_project.ProjectConfig) error {
	return EnableGoCompilerForManifest(manifestID, bldr_plugin_compiler_go.GoCompiler_GO_COMPILER_GOSCRIPT)
}

// EnableGoCompilerForManifest enables a compiler mode for a Go plugin
// Manifest's web platform override.
func EnableGoCompilerForManifest(
	manifestID string,
	mode bldr_plugin_compiler_go.GoCompiler,
) func(*bldr_project.ProjectConfig) error {
	return updateGoPluginManifest(manifestID, func(goConf *bldr_plugin_compiler_go.Config) error {
		setWebGoCompiler(goConf, mode)
		return nil
	})
}

// ConfigureTinyGoForManifest enables TinyGo and removes dev-only config that
// pulls packages TinyGo cannot compile.
func ConfigureTinyGoForManifest(manifestID string) func(*bldr_project.ProjectConfig) error {
	return ConfigureGoCompilerForManifest(manifestID, bldr_plugin_compiler_go.GoCompiler_GO_COMPILER_TINYGO)
}

// ConfigureGoScriptForManifest enables GoScript and removes dev-only config
// that pulls packages the browser lane should not compile.
func ConfigureGoScriptForManifest(manifestID string) func(*bldr_project.ProjectConfig) error {
	return ConfigureGoCompilerForManifest(manifestID, bldr_plugin_compiler_go.GoCompiler_GO_COMPILER_GOSCRIPT)
}

// ConfigureGoScriptBrowserStartup makes the local GoScript e2e lane cover the
// same browser Go plugin startup surface as the staging GoScript release.
func ConfigureGoScriptBrowserStartup(conf *bldr_project.ProjectConfig) error {
	if err := applyBuildManifestOverride(conf, "release-web-e2e", "spacewave-launcher"); err != nil {
		return err
	}
	build := conf.GetBuild()["release-web-e2e"]
	build.BuildPolicy = build.GetBuildPolicy().Merge(manifest_build.NewBuildPolicy(
		enabled.Enabled_ENABLE,
		enabled.Enabled_DISABLE,
		enabled.Enabled_DEFAULT,
	))
	for _, manifestID := range goScriptBrowserGoManifests {
		if err := ConfigureGoScriptForManifest(manifestID)(conf); err != nil {
			return err
		}
	}
	setStartupPlugins(conf, goScriptBrowserStartPlugins)
	return nil
}

// ConfigureGoCompilerForManifest enables an alternate browser Go plugin
// compiler and removes dev-only config outside the seed compiler proof.
func ConfigureGoCompilerForManifest(
	manifestID string,
	mode bldr_plugin_compiler_go.GoCompiler,
) func(*bldr_project.ProjectConfig) error {
	return updateGoPluginManifest(manifestID, func(goConf *bldr_plugin_compiler_go.Config) error {
		setWebGoCompiler(goConf, mode)
		removeDebugTraceConfig(goConf)
		removeSessionHarnessWebRTCConfig(goConf)
		return nil
	})
}

func updateGoPluginManifest(
	manifestID string,
	update func(*bldr_plugin_compiler_go.Config) error,
) func(*bldr_project.ProjectConfig) error {
	return func(conf *bldr_project.ProjectConfig) error {
		manifest := conf.GetManifests()[manifestID]
		if manifest == nil {
			return errors.Errorf("manifest %q not found", manifestID)
		}
		builder := manifest.GetBuilder()
		if builder == nil {
			return errors.Errorf("manifest %q has no builder", manifestID)
		}
		if builder.GetId() != bldr_plugin_compiler_go.ConfigID {
			return errors.Errorf("manifest %q builder is %q, want %q", manifestID, builder.GetId(), bldr_plugin_compiler_go.ConfigID)
		}

		goConf := &bldr_plugin_compiler_go.Config{}
		if data := builder.GetConfig(); len(data) != 0 {
			if err := goConf.UnmarshalJSON(data); err != nil {
				return errors.Wrapf(err, "unmarshal %s builder config", manifestID)
			}
		}
		if err := update(goConf); err != nil {
			return err
		}

		data, err := configjson.MarshalCanonical(goConf)
		if err != nil {
			return errors.Wrapf(err, "marshal %s builder config", manifestID)
		}
		builder.Config = data
		return nil
	}
}

func applyBuildManifestOverride(conf *bldr_project.ProjectConfig, buildID, manifestID string) error {
	build := conf.GetBuild()[buildID]
	if build == nil {
		return errors.Errorf("build %q not found", buildID)
	}
	override := build.GetManifestOverrides()[manifestID]
	if override == nil {
		return errors.Errorf("build %q has no manifest override for %q", buildID, manifestID)
	}
	manifest := conf.GetManifests()[manifestID]
	if manifest == nil {
		return errors.Errorf("manifest %q not found", manifestID)
	}
	builder := manifest.GetBuilder()
	if builder == nil {
		return errors.Errorf("manifest %q has no builder", manifestID)
	}
	builder.Config = append([]byte(nil), override.GetConfig()...)
	return nil
}

func setStartupPlugins(conf *bldr_project.ProjectConfig, plugins []string) {
	if conf.Start == nil {
		conf.Start = &bldr_project.StartConfig{}
	}
	conf.Start.Plugins = append([]string(nil), plugins...)
}

func setWebGoCompiler(goConf *bldr_plugin_compiler_go.Config, mode bldr_plugin_compiler_go.GoCompiler) {
	if goConf.PlatformTypes == nil {
		goConf.PlatformTypes = make(map[string]*bldr_plugin_compiler_go.Config)
	}
	webConf := goConf.PlatformTypes["web"]
	if webConf == nil {
		webConf = &bldr_plugin_compiler_go.Config{}
	}
	webConf.GoCompiler = mode
	goConf.PlatformTypes["web"] = webConf
}

func removeDebugTraceConfig(goConf *bldr_plugin_compiler_go.Config) {
	goConf.GoPkgs = removeGoPkg(goConf.GetGoPkgs(), "./core/debug/trace")
	delete(goConf.ConfigSet, "debug-trace")

	devConf := goConf.GetBuildTypes()["dev"]
	if devConf == nil {
		return
	}
	devConf.GoPkgs = removeGoPkg(devConf.GetGoPkgs(), "./core/debug/trace")
	delete(devConf.ConfigSet, "debug-trace")
}

func removeSessionHarnessWebRTCConfig(goConf *bldr_plugin_compiler_go.Config) {
	goConf.GoPkgs = removeGoPkg(goConf.GetGoPkgs(), "github.com/s4wave/spacewave/net/transport/webrtc")
	delete(goConf.ConfigSet, "e2e-session-harness-webrtc")
}

func removeGoPkg(pkgs []string, remove string) []string {
	out := pkgs[:0]
	for _, pkg := range pkgs {
		if pkg != remove {
			out = append(out, pkg)
		}
	}
	return out
}

func normalizeWorkerMode(raw string) (WorkerMode, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", string(WorkerModeDedicated), "dedicated-worker", "dedicated_workers":
		return WorkerModeDedicated, nil
	case string(WorkerModeShared), "shared-worker", "sharedworker":
		return WorkerModeShared, nil
	default:
		return "", errors.Errorf("unsupported %s value %q, expected dedicated or shared", E2EWasmWorkerModeEnv, raw)
	}
}

func copyOptionalTinyGoEnv(srcKey, dstKey string) error {
	value := strings.TrimSpace(os.Getenv(srcKey))
	if value == "" {
		return nil
	}
	if err := os.Setenv(dstKey, value); err != nil {
		return errors.Wrapf(err, "set %s", dstKey)
	}
	return nil
}
