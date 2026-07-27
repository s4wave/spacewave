package slicecoverage

import (
	"go/ast"
	goparser "go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/ghodss/yaml"
)

type workflow struct {
	Jobs map[string]struct {
		Strategy struct {
			Matrix struct {
				Slice []wasmSlice `json:"slice"`
			} `json:"matrix"`
		} `json:"strategy"`
	} `json:"jobs"`
}

type wasmSlice struct {
	Name    string  `json:"name"`
	Package *string `json:"package"`
	Run     string  `json:"run"`
}

// No slice regex selects these tests, so CI never runs them. This list exists
// so a new unenrolled test fails this check instead of passing unnoticed.
// Entries leave the list as tests are enrolled, and an addition is a deliberate
// decision a reviewer sees in the diff.
var notEnrolledInAnySlice = []string{
	"TestBrowserHelpersAndRawAccess",
	"TestBrowserLaunchFromGo",
	"TestBrowserRouteNavigation",
	"TestBrowserWorkerQuicRwcFixture",
	"TestBuildHarnessStateRootKeepsCachedRunsStable",
	"TestChromiumLaunchOptions",
	"TestConfigureGoScriptBrowserStartupUsesLauncherCoreAndBuildsSQL",
	"TestConfigureGoScriptForManifestPreservesTraceService",
	"TestConfigureGoScriptForManifestRetainsSessionHarnessWebRTC",
	"TestCrashReportIgnoresBenignNormalCloseTeardownAbort",
	"TestCrashReportStillCatchesRealAbortPageError",
	"TestDedicatedWorkerHostFallback",
	"TestE2EWasmBrowserWebRTCEnabled",
	"TestE2EWasmDriveBenchJSProfileEnabled",
	"TestE2EWasmTraceServiceEnabled",
	"TestEndToEndLinkEstablishment",
	"TestGoScriptBlogDynamicQuickstartRetainedStateParity",
	"TestGoScriptCanvasQuickstartCreateMutate",
	"TestGoScriptCanvasQuickstartRouteResourceProbe",
	"TestGoScriptChatQuickstartMessagingParity",
	"TestGoScriptCliTerminalCommands",
	"TestGoScriptDeviceQuickstartOpensComputersDashboard",
	"TestGoScriptDriveAccountDeleteRemovesOpfsSubtree",
	"TestGoScriptDriveBrowserLayoutDropReloadUIParity",
	"TestGoScriptDriveBrowserMoveDragDeleteUIParity",
	"TestGoScriptDriveStartupBench",
	"TestGoScriptFSHandleBrowserResourceOperations",
	"TestGoScriptGitQuickstartLocalCreateReloadParity",
	"TestGoScriptNotesDynamicQuickstartsParity",
	"TestGoScriptObjectLayoutRuntimeParity",
	"TestGoScriptObjectLayoutSeedModelParity",
	"TestGoScriptProjectedExportDownloadBrowserParity",
	"TestGoScriptQuickstartDriveDirectRouteMountGate",
	"TestGoScriptQuickstartDrivePerformanceProof",
	"TestGoScriptQuickstartSpaceDirectRouteMountGate",
	"TestGoScriptSharedObjectDirectRouteBodyMountGate",
	"TestGoScriptSharedWorkerOPFSProbe",
	"TestGoScriptUnixFSLargeMultiFileDropCompletes",
	"TestHarnessStateRootConcurrentOwnersUseDistinctRoots",
	"TestHarnessStateRootOwnerMarkerRoundTrip",
	"TestHarnessStateRootReleaseStopsConsumersBeforeUnlock",
	"TestHarnessStateRootSerialOwnersReuseStableRoot",
	"TestQuickstartDriveLargeUploadBudgetReport",
	"TestQuickstartDriveNavigateTrace",
	"TestQuickstartDriveSplitTabNavigation",
	"TestQuickstartDriveTrace",
	"TestQuickstartDriveUploadBudgetProfiles",
	"TestQuickstartDriveUploadOverwriteBudgetProfile",
	"TestQuickstartDriveUploadTrace",
	"TestQuickstartKvEditPersistsAfterReload",
	"TestQuickstartSqlRunCreatesLinkedQueryResult",
	"TestQuickstartSqlWorkbenchPinsPersistAfterReload",
	"TestQuickstartV86BootSmoke",
	"TestReapHarnessCacheOffStateRootsDeletesDeadPIDMarker",
	"TestReapHarnessCacheOffStateRootsDeletesOldMarkerlessStateRoot",
	"TestReapHarnessCacheOffStateRootsPreservesLivePIDMarker",
	"TestReapHarnessCacheOffStateRootsPreservesStableStateRoot",
	"TestReapHarnessCacheOffStateRootsPreservesYoungMarkerlessStateRoot",
	"TestResolveBldrDependencyUsesExplicitSpacewaveRepoRoot",
	"TestResolveE2EWasmManifestBuildTimeout",
	"TestResolveE2EWasmManifestBuildTimeoutRejectsInvalid",
	"TestResourceSetupHelpers",
	"TestRetainedStatePageSessionReusesWarmContext",
	"TestRetainedStateResourceSessionSupportsSequentialReuse",
	"TestRetainedStateSessionReleaseCleansPerSessionState",
	"TestSessionHarnessPeerInfo",
	"TestSharedWorkerOpfsReturnGate",
	"TestSignalRelayCrossConnect",
	"TestStartupBuildCacheDefaultsOnWithExplicitFreshBuildEscape",
	"TestSummarizeBrowserCPUProfileBucketsSamples",
	"TestSummarizeTraceBuildsOperationShapeFromTasksAndLogs",
	"TestTransientAppWaitErrorIncludesStartupNavigation",
	"TestWaitForCountConsumesOwnerUpdates",
	"TestWasmHarnessTraceConfig",
}

func TestAllWasmTestsAreEnrolled(t *testing.T) {
	repoRoot := testRepoRoot(t)

	workflowData, err := os.ReadFile(filepath.Join(repoRoot, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatalf("read CI workflow: %v", err)
	}
	var config workflow
	if err := yaml.Unmarshal(workflowData, &config); err != nil {
		t.Fatalf("parse CI workflow: %v", err)
	}

	e2eWasm, ok := config.Jobs["e2e-wasm"]
	if !ok {
		t.Fatal("CI workflow has no e2e-wasm job")
	}
	var sliceRegexps []*regexp.Regexp
	for _, slice := range e2eWasm.Strategy.Matrix.Slice {
		// A slice naming its own package runs TestScenarios under build tags, so
		// what it covers is a tag question rather than a name question. This
		// check answers the name question only.
		if slice.Package != nil {
			continue
		}
		sliceRegexps = append(sliceRegexps, compileSliceRegexp(t, slice))
	}

	wasmTests := findWasmTests(t, filepath.Join(repoRoot, "e2e", "wasm"))
	knownTests := make(map[string]bool, len(wasmTests))
	for _, testName := range wasmTests {
		knownTests[testName] = true
	}
	for _, testName := range notEnrolledInAnySlice {
		if !knownTests[testName] {
			t.Errorf("not-enrolled test name no longer exists: %s", testName)
		}
	}

	var unenrolled []string
	for _, testName := range wasmTests {
		covered := false
		for _, sliceRegexp := range sliceRegexps {
			if sliceRegexp.MatchString(testName) {
				covered = true
				break
			}
		}
		if !covered && !slices.Contains(notEnrolledInAnySlice, testName) {
			unenrolled = append(unenrolled, testName)
		}
	}
	if len(unenrolled) != 0 {
		t.Errorf("these e2e/wasm tests are not selected by any slice; enroll each test in a slice, or deliberately add its name to notEnrolledInAnySlice after review:\n%s", strings.Join(unenrolled, "\n"))
	}
}

func testRepoRoot(t *testing.T) string {
	t.Helper()
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("find coverage test file")
	}
	return filepath.Join(filepath.Dir(testFile), "..", "..", "..")
}

func compileSliceRegexp(t *testing.T, slice wasmSlice) (compiled *regexp.Regexp) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("slice %q has invalid run regex: %v", slice.Name, recovered)
		}
	}()
	return regexp.MustCompile(slice.Run)
}

func findWasmTests(t *testing.T, wasmDir string) []string {
	t.Helper()
	entries, err := os.ReadDir(wasmDir)
	if err != nil {
		t.Fatalf("read wasm test directory: %v", err)
	}

	fileSet := token.NewFileSet()
	var testNames []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}

		// parser.ParseFile ignores build constraints, keeping tagged tests in this inventory.
		file, err := goparser.ParseFile(fileSet, filepath.Join(wasmDir, entry.Name()), nil, goparser.AllErrors)
		if err != nil {
			t.Fatalf("parse %s: %v", entry.Name(), err)
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv != nil || !strings.HasPrefix(function.Name.Name, "Test") {
				continue
			}
			if function.Type.Params == nil || len(function.Type.Params.List) != 1 {
				continue
			}
			parameter := function.Type.Params.List[0]
			if len(parameter.Names) != 1 || !isTestingT(parameter.Type) {
				continue
			}
			testNames = append(testNames, function.Name.Name)
		}
	}
	sort.Strings(testNames)
	return testNames
}

func isTestingT(expression ast.Expr) bool {
	pointer, ok := expression.(*ast.StarExpr)
	if !ok {
		return false
	}
	selector, ok := pointer.X.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	packageName, ok := selector.X.(*ast.Ident)
	return ok && packageName.Name == "testing" && selector.Sel.Name == "T"
}
