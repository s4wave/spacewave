package boot

import (
	"cmp"
	"math"
	"math/bits"
	"slices"
	"strings"
)

const (
	noGapDivisor          = uint64(20)
	maxBootDurationMicros = uint64(24 * 60 * 60 * 1_000_000)
	maxBootStringBytes    = 128
	maxBootRecordBytes    = 4 << 20
	maxBootDetailFields   = 32
	maxBootMarks          = 4096
	maxBootSpans          = 4096
	maxBootSamples        = 4096
	maxBootAttachments    = 64
	maxBootPrivacyFields  = 256
	maxBootRecords        = 8192
	maxBootSpanDepth      = 64
)

type bootInterval struct {
	start uint64
	end   uint64
}

type bootSpanFrame struct {
	span  *BootSpan
	start uint64
	end   uint64
}

func bootIntervalUnionDuration(intervals []bootInterval) uint64 {
	if len(intervals) == 0 {
		return 0
	}
	slices.SortFunc(intervals, func(a, b bootInterval) int {
		if c := cmp.Compare(a.start, b.start); c != 0 {
			return c
		}
		return cmp.Compare(a.end, b.end)
	})
	start, end := intervals[0].start, intervals[0].end
	var duration uint64
	for _, interval := range intervals[1:] {
		if interval.start > end {
			duration += end - start
			start, end = interval.start, interval.end
			continue
		}
		if interval.end > end {
			end = interval.end
		}
	}
	return duration + end - start
}

func clippedBootInterval(span *BootSpan, start, end uint64) (bootInterval, bool) {
	if span == nil {
		return bootInterval{}, false
	}
	clipped := bootInterval{start: max(span.GetStartMonotonicMicros(), start), end: min(span.GetEndMonotonicMicros(), end)}
	return clipped, clipped.end > clipped.start
}

var (
	bootMarkLabels = map[string]struct{}{
		"boot.started": {}, "boot.aborted": {}, "boot.sealed": {}, "content-ready": {},
		"runtime.started": {}, "runtime.failed": {}, "runtime.last-observed": {}, "worker.ready": {},
		"boot-status.loading": {}, "boot-status.manifest": {}, "boot-status.manifest-ready": {},
		"boot-status.wasm": {}, "boot-status.entrypoint": {}, "boot-status.entrypoint-error": {},
		"shell.entrypoint-loaded": {}, "shell.container-resolved": {}, "boot-status.runtime": {},
		"runtime.wait-start": {}, "runtime.mode-selected": {}, "service-worker.install-start": {},
		"service-worker.register-start": {}, "runtime.client-open-start": {},
		"service-worker.install-ready": {}, "service-worker.register-ready": {},
		"service-worker.update-ready": {}, "service-worker.activate-ready": {},
		"service-worker.control-ready": {}, "service-worker.port-started": {},
		"service-worker.port-sent": {}, "runtime.worker-create-start": {},
		"runtime.worker-created": {}, "runtime.opfs-bridge-ready": {},
		"runtime.client-open-sent": {}, "runtime.client-channel-opened": {},
		"runtime.client-channel-acked": {}, "runtime.connected": {},
		"runtime.client-connect-ack": {}, "runtime.event-connected": {},
		"runtime.wait-conn-ready": {}, "runtime.wait-ready": {},
		"shell.deferred-boot-ready": {}, "shell.immediate-boot-ready": {},
		"boot-status.ready": {}, "manifest-copy.selected": {},
		"manifest-copy.waiting-for-running": {}, "manifest-copy.copying": {},
		"manifest-copy.done": {}, "manifest-copy.failed": {}, "boot-status.app": {},
		"shell.boot-requested": {}, "quickstart.static-handoff-requested": {},
		"webview.loading-surface-mounted": {}, "webview.registered": {},
		"worker.first-ready": {}, "plugin.frontend-ready": {},
		"plugin.capability-ready": {}, "webview.stylesheet-ready": {},
		"webview.component-ready": {}, "webview.revealed": {},
		"webview.loading-surface-revealed": {},
	}
	bootOwners = map[string]struct{}{
		"bldr": {}, "browser": {}, "cdn": {}, "entrypoint": {}, "fixture": {},
		"manifest-materializer": {}, "network": {}, "opfs": {}, "plugin-host": {},
		"runtime": {}, "scheduler": {}, "service-worker": {}, "shell": {}, "webview": {}, "worker": {},
	}
	bootOperations = map[string]struct{}{
		"access-manifest": {}, "checksum": {}, "copy-manifest": {}, "decode": {},
		"dist-main": {}, "download-entrypoint": {}, "execute-plugin": {},
		"load-dist-main": {}, "open-release-world": {}, "opfs-read": {}, "opfs-write": {},
		"publish-local-ref": {}, "read-release-blocks": {}, "release-block-request": {},
		"release-pack-range": {}, "release-world-provider": {}, "runtime": {},
		"select-startup-manifests": {}, "sync-manifest": {}, "wait-for-provider": {},
		"wait-for-running": {}, "worker-request": {},
	}
	bootDetailKeys = map[string]struct{}{
		"attempt-count": {}, "blocks-copied": {}, "blocks-deduped": {}, "blocks-existing": {},
		"blocks-seen": {}, "blocks-written": {}, "cache-mode": {}, "candidate-count": {},
		"copied-bytes": {}, "demand-read-bytes": {}, "demand-read-count": {},
		"duration-ratio": {}, "logical-source-bytes": {}, "phase": {}, "revision-count": {},
		"skipped-subtrees": {}, "written-bytes": {},
	}
	bootDetailStrings = map[string]struct{}{
		"cold": {}, "warm": {}, "hot": {}, "selected": {}, "waiting-for-running": {},
		"copying": {}, "local-ref-publication": {}, "sync": {}, "done": {}, "failed": {},
	}
	bootCounterNames = map[string]struct{}{
		"blocks-copied": {}, "blocks-deduped": {}, "blocks-existing": {}, "blocks-seen": {},
		"blocks-written": {}, "copied-bytes": {}, "demand-read-bytes": {},
		"demand-read-count": {}, "download-bytes": {}, "logical-source-bytes": {},
		"revision-count": {}, "selected-candidates": {}, "skipped-subtrees": {}, "written-bytes": {},
	}
	bootTerminalErrorCodes = map[string]struct{}{
		"boot-interrupted": {}, "entrypoint-load-failed": {}, "manifest-copy-failed": {},
		"runtime-init-failed": {}, "startup-aborted": {}, "startup-timeout": {},
	}
	bootEntrypoints = map[string]struct{}{
		"canvas": {}, "computers": {}, "drive": {}, "forge": {}, "space": {},
	}
	bootProjects       = map[string]struct{}{"spacewave": {}}
	bootBrowserEngines = map[string]struct{}{"chromium": {}, "firefox": {}, "webkit": {}}
	bootOSFamilies     = map[string]struct{}{"android": {}, "darwin": {}, "ios": {}, "linux": {}, "windows": {}}
	bootArchitectures  = map[string]struct{}{"amd64": {}, "arm64": {}, "wasm": {}}
)

func bootVocabularyContains(values map[string]struct{}, value string) bool {
	_, ok := values[value]
	return ok
}

func validBootIdentifier(value string) bool {
	if value == "" || len(value) > maxBootStringBytes || validBootHex(value, 32) {
		return false
	}
	previousSeparator := false
	for idx := 0; idx < len(value); idx++ {
		char := value[idx]
		letter := char >= 'a' && char <= 'z'
		digit := char >= '0' && char <= '9'
		separator := char == '-'
		if (!letter && !digit && !separator) || (idx == 0 && !letter) ||
			(separator && (previousSeparator || idx == len(value)-1)) {
			return false
		}
		previousSeparator = separator
	}
	return true
}

func validBootReportID(value string) bool {
	if value == "fixture-report" {
		return true
	}
	const prefix = "boot-report-"
	return strings.HasPrefix(value, prefix) && validBootIdentifier("id-"+strings.TrimPrefix(value, prefix))
}

func validOptionalBootReportID(value string) bool {
	return value == "" || validBootReportID(value)
}

func validBootScopeID(value string) bool {
	const prefix = "boot-candidate"
	return value == prefix || (strings.HasPrefix(value, prefix+"-") &&
		validBootIdentifier("id-"+strings.TrimPrefix(value, prefix+"-")))
}

func validBootSpanID(value string) bool {
	switch value {
	case "network", "runtime", "scheduler", "storage", "wait":
		return true
	}
	const prefix = "span-"
	return strings.HasPrefix(value, prefix) && validBootIdentifier("id-"+strings.TrimPrefix(value, prefix))
}

func validOptionalBootSpanID(value string) bool {
	return value == "" || validBootSpanID(value)
}

func validBootArtifactID(value string) bool {
	const prefix = "boot-artifact-"
	return strings.HasPrefix(value, prefix) && validBootIdentifier("id-"+strings.TrimPrefix(value, prefix))
}

func bootSpanIsActionable(span *BootSpan) bool {
	if span == nil || !bootVocabularyContains(bootOwners, span.GetOwner()) ||
		!bootVocabularyContains(bootOperations, span.GetOperation()) {
		return false
	}
	switch span.GetWorkClass() {
	case BootWorkClass_BOOT_WORK_CLASS_COMPUTE:
		return bootVocabularyContains(bootOperations, span.GetSourceTask())
	case BootWorkClass_BOOT_WORK_CLASS_STORAGE_IO,
		BootWorkClass_BOOT_WORK_CLASS_NETWORK_IO,
		BootWorkClass_BOOT_WORK_CLASS_SCHEDULER:
		return true
	case BootWorkClass_BOOT_WORK_CLASS_LOCK_WAIT,
		BootWorkClass_BOOT_WORK_CLASS_DEPENDENCY_WAIT:
		return bootVocabularyContains(bootOperations, span.GetCausalWaitTarget())
	default:
		return false
	}
}

func validBootFieldPath(value string) bool {
	if value == "" || len(value) > maxBootStringBytes {
		return false
	}
	for idx, part := range strings.Split(value, ".") {
		if part == "" {
			return false
		}
		for charIdx := 0; charIdx < len(part); charIdx++ {
			char := part[charIdx]
			letter := char >= 'a' && char <= 'z'
			digit := charIdx > 0 && char >= '0' && char <= '9'
			underscore := charIdx > 0 && char == '_'
			if !letter && !digit && !underscore {
				return false
			}
		}
		if idx >= maxBootPrivacyFields {
			return false
		}
	}
	return true
}

func validBootCommit(value string) bool {
	if value == "" {
		return true
	}
	return validBootHex(value, 7)
}

func validBootHash(value string) bool {
	return validBootHex(value, 32)
}

func validBootHex(value string, minimum int) bool {
	if len(value) < minimum || len(value) > maxBootStringBytes {
		return false
	}
	for idx := range value {
		digit := value[idx] >= '0' && value[idx] <= '9'
		hexLetter := value[idx] >= 'a' && value[idx] <= 'f'
		if !digit && !hexLetter {
			return false
		}
	}
	return true
}

func bootViolation(kind BootValidationViolationKind) *BootValidationViolation {
	return &BootValidationViolation{Kind: kind}
}

func validateBootValue(value *BootValue) bool {
	if value == nil || value.GetValue() == nil {
		return false
	}
	switch field := value.GetValue().(type) {
	case *BootValue_StringValue:
		return bootVocabularyContains(bootDetailStrings, field.StringValue)
	case *BootValue_NumberValue:
		return !math.IsNaN(field.NumberValue) && !math.IsInf(field.NumberValue, 0)
	case *BootValue_SignedValue, *BootValue_UnsignedValue, *BootValue_BoolValue:
		return true
	default:
		return false
	}
}

func validBootBuildType(value BootBuildType) bool {
	switch value {
	case BootBuildType_BOOT_BUILD_TYPE_DEBUG, BootBuildType_BOOT_BUILD_TYPE_RELEASE:
		return true
	default:
		return false
	}
}

func validBootRuntimeKind(value BootRuntimeKind) bool {
	return value >= BootRuntimeKind_BOOT_RUNTIME_KIND_GOSCRIPT && value <= BootRuntimeKind_BOOT_RUNTIME_KIND_NATIVE
}

func validBootWorkerMode(value BootWorkerMode) bool {
	return value >= BootWorkerMode_BOOT_WORKER_MODE_SHARED && value <= BootWorkerMode_BOOT_WORKER_MODE_INLINE
}

func validBootEnvironmentClass(value BootEnvironmentClass) bool {
	return value >= BootEnvironmentClass_BOOT_ENVIRONMENT_CLASS_LOCAL && value <= BootEnvironmentClass_BOOT_ENVIRONMENT_CLASS_PRODUCTION
}

func validBootServiceWorkerState(value BootServiceWorkerState) bool {
	return value >= BootServiceWorkerState_BOOT_SERVICE_WORKER_STATE_UNAVAILABLE && value <= BootServiceWorkerState_BOOT_SERVICE_WORKER_STATE_REDUNDANT
}

func validBootCacheState(value BootCacheState) bool {
	return value >= BootCacheState_BOOT_CACHE_STATE_COLD && value <= BootCacheState_BOOT_CACHE_STATE_HOT
}

func validBootRecoveryDecision(value BootRecoveryDecision) bool {
	return value >= BootRecoveryDecision_BOOT_RECOVERY_DECISION_NONE && value <= BootRecoveryDecision_BOOT_RECOVERY_DECISION_ABORT_STALE
}

func validBootPhase(value BootPhase) bool {
	return value >= BootPhase_BOOT_PHASE_PREPARE && value <= BootPhase_BOOT_PHASE_DONE
}

func validBootCounterUnit(value BootCounterUnit) bool {
	return value >= BootCounterUnit_BOOT_COUNTER_UNIT_COUNT && value <= BootCounterUnit_BOOT_COUNTER_UNIT_MICROSECONDS
}

func validBootAttachmentKind(value BootAttachmentKind) bool {
	return value >= BootAttachmentKind_BOOT_ATTACHMENT_KIND_SOURCE_MAP && value <= BootAttachmentKind_BOOT_ATTACHMENT_KIND_NETWORK_PROFILE
}

func validBootShareDestination(value BootShareDestination) bool {
	return value >= BootShareDestination_BOOT_SHARE_DESTINATION_CLIPBOARD && value <= BootShareDestination_BOOT_SHARE_DESTINATION_SPACE
}

func validBootSpanResult(value BootSpanResult) bool {
	return value >= BootSpanResult_BOOT_SPAN_RESULT_SUCCEEDED && value <= BootSpanResult_BOOT_SPAN_RESULT_CANCELED
}

func validBootWorkClass(value BootWorkClass) bool {
	return value >= BootWorkClass_BOOT_WORK_CLASS_COMPUTE && value <= BootWorkClass_BOOT_WORK_CLASS_SCHEDULER
}

func bootReportExceedsCollectionBounds(report *BootReport) bool {
	if report == nil {
		return false
	}
	marks := len(report.GetMarks())
	spans := len(report.GetSpans())
	samples := len(report.GetAccounting().GetSamples())
	attachments := len(report.GetAttachments())
	privacy := report.GetPrivacy()
	validation := report.GetValidation()
	return marks > maxBootMarks || spans > maxBootSpans || samples > maxBootSamples ||
		attachments > maxBootAttachments || len(privacy.GetRemovedFields()) > maxBootPrivacyFields ||
		len(privacy.GetBucketedFields()) > maxBootPrivacyFields ||
		len(validation.GetPhaseDurations()) > int(BootPhase_BOOT_PHASE_DONE) ||
		len(validation.GetLongestGaps()) > maxBootMarks || len(validation.GetViolations()) > maxBootRecords ||
		marks+spans+samples+attachments > maxBootRecords
}

func validateBootReportContract(report *BootReport, total uint64) bool {
	if report.SizeVT() > maxBootRecordBytes || report.GetSchemaVersion() != 1 || !validBootReportID(report.GetReportId()) ||
		!validOptionalBootReportID(report.GetParentReportId()) || !bootVocabularyContains(bootEntrypoints, report.GetEntrypointId()) ||
		!bootVocabularyContains(bootMarkLabels, report.GetUsableMark()) || report.GetStartedUnixMicros() <= 0 ||
		report.GetMonotonicOriginUnixMicros() <= 0 || total > maxBootDurationMicros ||
		report.GetBuild() == nil || report.GetEnvironment() == nil || report.GetAccounting() == nil ||
		report.GetPrivacy() == nil || report.GetPrivacy().GetExportPolicyVersion() != 1 {
		return false
	}
	build := report.GetBuild()
	if !validBootCommit(build.GetCommit()) || !validBootIdentifier(build.GetReleaseGeneration()) ||
		!bootVocabularyContains(bootProjects, build.GetProjectId()) || !validBootBuildType(build.GetBuildType()) ||
		!validBootRuntimeKind(build.GetRuntimeKind()) ||
		!validBootWorkerMode(build.GetWorkerMode()) {
		return false
	}
	environment := report.GetEnvironment()
	if !validBootEnvironmentClass(environment.GetClass()) ||
		!bootVocabularyContains(bootBrowserEngines, environment.GetBrowserEngine()) || !bootVocabularyContains(bootOSFamilies, environment.GetOsFamily()) ||
		!bootVocabularyContains(bootArchitectures, environment.GetArchitecture()) ||
		!validBootServiceWorkerState(environment.GetServiceWorkerState()) ||
		!validBootCacheState(environment.GetCacheState()) ||
		!validBootRecoveryDecision(environment.GetRecoveryDecision()) {
		return false
	}
	for _, mark := range report.GetMarks() {
		if mark == nil || len(mark.GetDetail()) > maxBootDetailFields || !bootVocabularyContains(bootMarkLabels, mark.GetLabel()) ||
			!bootVocabularyContains(bootOwners, mark.GetSourceOwner()) {
			return false
		}
		previous := ""
		for _, detail := range mark.GetDetail() {
			if detail == nil || !bootVocabularyContains(bootDetailKeys, detail.GetKey()) || detail.GetKey() <= previous || !validateBootValue(detail.GetValue()) {
				return false
			}
			previous = detail.GetKey()
		}
	}
	markTimes := make(map[uint64]struct{}, len(report.GetMarks()))
	for _, mark := range report.GetMarks() {
		markTimes[mark.GetMonotonicMicros()] = struct{}{}
	}
	previousSampleTime := uint64(0)
	for idx, sample := range report.GetAccounting().GetSamples() {
		_, atMark := markTimes[sample.GetMonotonicMicros()]
		if sample == nil || !validBootScopeID(sample.GetScopeId()) || !bootVocabularyContains(bootOwners, sample.GetOwner()) ||
			!bootVocabularyContains(bootCounterNames, sample.GetName()) || !validBootCounterUnit(sample.GetUnit()) ||
			sample.GetMonotonicMicros() > total || !atMark || (!sample.GetKnown() && sample.GetValue() != 0) ||
			(idx != 0 && sample.GetMonotonicMicros() < previousSampleTime) {
			return false
		}
		previousSampleTime = sample.GetMonotonicMicros()
	}
	for _, attachment := range report.GetAttachments() {
		if attachment == nil || !validBootArtifactID(attachment.GetArtifactId()) ||
			!validBootAttachmentKind(attachment.GetKind()) ||
			!validBootHash(attachment.GetContentHash()) || attachment.GetSizeBytes() == 0 ||
			!validBootIdentifier(attachment.GetReleaseGeneration()) ||
			attachment.GetReleaseGeneration() != build.GetReleaseGeneration() {
			return false
		}
	}
	privacy := report.GetPrivacy()
	for _, field := range append(slices.Clone(privacy.GetRemovedFields()), privacy.GetBucketedFields()...) {
		if !validBootFieldPath(field) {
			return false
		}
	}
	shared := privacy.GetSharedUnixMicros()
	destination := privacy.GetShareDestination()
	if (shared == 0) != (destination == BootShareDestination_BOOT_SHARE_DESTINATION_UNKNOWN) || shared < 0 ||
		(destination != BootShareDestination_BOOT_SHARE_DESTINATION_UNKNOWN && !validBootShareDestination(destination)) {
		return false
	}
	return true
}

func validateBootSpanStructure(spans []*BootSpan, total uint64) (map[string][]*BootSpan, bool) {
	spanByID := make(map[string]*BootSpan, len(spans))
	children := make(map[string][]*BootSpan)
	invalid := false
	for _, span := range spans {
		if span == nil {
			invalid = true
			continue
		}
		id := span.GetSpanId()
		_, duplicate := spanByID[id]
		if !validBootSpanID(id) || duplicate || !bootVocabularyContains(bootOwners, span.GetOwner()) || !bootVocabularyContains(bootOperations, span.GetOperation()) ||
			!validBootSpanResult(span.GetResult()) || !validBootWorkClass(span.GetWorkClass()) ||
			span.GetEndMonotonicMicros() <= span.GetStartMonotonicMicros() || span.GetEndMonotonicMicros() > total ||
			!validOptionalBootSpanID(span.GetParentSpanId()) || span.GetCausalWaitTarget() != "" && !bootVocabularyContains(bootOperations, span.GetCausalWaitTarget()) ||
			span.GetSourceTask() != "" && !bootVocabularyContains(bootOperations, span.GetSourceTask()) {
			invalid = true
		}
		if !duplicate && id != "" {
			spanByID[id] = span
		}
		children[span.GetParentSpanId()] = append(children[span.GetParentSpanId()], span)
	}
	for _, span := range spans {
		if span == nil || span.GetParentSpanId() == "" {
			continue
		}
		parent := spanByID[span.GetParentSpanId()]
		if parent == nil || span.GetStartMonotonicMicros() < parent.GetStartMonotonicMicros() ||
			span.GetEndMonotonicMicros() > parent.GetEndMonotonicMicros() {
			invalid = true
		}
	}
	for _, span := range spans {
		seen := make(map[string]struct{}, maxBootSpanDepth)
		current := span
		for depth := 0; current != nil && current.GetParentSpanId() != ""; depth++ {
			if depth >= maxBootSpanDepth {
				invalid = true
				break
			}
			parentID := current.GetParentSpanId()
			if _, visited := seen[parentID]; visited {
				invalid = true
				break
			}
			seen[parentID] = struct{}{}
			current = spanByID[parentID]
		}
	}
	return children, invalid
}

func validateBootSpanTree(start, end, threshold uint64, roots []*BootSpan, children map[string][]*BootSpan, violations *[]*BootValidationViolation) []bootInterval {
	stack := make([]bootSpanFrame, 0, len(roots))
	for _, root := range slices.Backward(roots) {
		stack = append(stack, bootSpanFrame{span: root, start: start, end: end})
	}
	actionable := make([]bootInterval, 0, len(roots))
	visited := make(map[string]struct{}, len(children))
	for len(stack) != 0 {
		frame := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		interval, ok := clippedBootInterval(frame.span, frame.start, frame.end)
		if !ok {
			continue
		}
		id := frame.span.GetSpanId()
		if _, seen := visited[id]; seen {
			continue
		}
		visited[id] = struct{}{}
		nested := children[id]
		if len(nested) == 0 {
			if bootSpanIsActionable(frame.span) {
				actionable = append(actionable, interval)
			} else if interval.end-interval.start > threshold {
				*violations = append(*violations, &BootValidationViolation{
					Kind:                 BootValidationViolationKind_BOOT_VALIDATION_VIOLATION_KIND_GENERIC_LEAF,
					StartMonotonicMicros: interval.start, EndMonotonicMicros: interval.end, SpanId: id,
				})
			}
			continue
		}
		childIntervals := make([]bootInterval, 0, len(nested))
		for _, child := range nested {
			if childInterval, childOK := clippedBootInterval(child, interval.start, interval.end); childOK {
				childIntervals = append(childIntervals, childInterval)
			}
		}
		duration := interval.end - interval.start
		covered := bootIntervalUnionDuration(childIntervals)
		if duration > threshold && (duration-covered > duration/noGapDivisor || duration-covered > threshold) {
			*violations = append(*violations, &BootValidationViolation{
				Kind:                 BootValidationViolationKind_BOOT_VALIDATION_VIOLATION_KIND_GAP_COVERAGE,
				StartMonotonicMicros: interval.start, EndMonotonicMicros: interval.end, SpanId: id,
			})
		}
		for _, n := range slices.Backward(nested) {
			stack = append(stack, bootSpanFrame{span: n, start: interval.start, end: interval.end})
		}
	}
	return actionable
}

// ValidateBootReport derives phase totals and enforces the recursive five-percent
// attribution contract over one terminal BootReport.
func ValidateBootReport(report *BootReport) *BootValidation {
	validation := &BootValidation{}
	if report == nil {
		validation.Violations = append(validation.Violations,
			bootViolation(BootValidationViolationKind_BOOT_VALIDATION_VIOLATION_KIND_REPORT_CONTRACT))
		return validation
	}
	total := report.GetTerminalMonotonicMicros()
	validation.TotalDurationMicros = total
	if bootReportExceedsCollectionBounds(report) {
		validation.Violations = append(validation.Violations,
			bootViolation(BootValidationViolationKind_BOOT_VALIDATION_VIOLATION_KIND_REPORT_CONTRACT))
		return validation
	}
	if !validateBootReportContract(report, total) {
		validation.Violations = append(validation.Violations,
			bootViolation(BootValidationViolationKind_BOOT_VALIDATION_VIOLATION_KIND_REPORT_CONTRACT))
	}

	usableMarkCount := 0
	terminalMarkCount := 0
	terminalMarkTime := uint64(0)
	for _, mark := range report.GetMarks() {
		if mark.GetLabel() == report.GetUsableMark() {
			usableMarkCount++
		}
		if mark.GetLabel() == report.GetTerminalMark() {
			terminalMarkCount++
			terminalMarkTime = mark.GetMonotonicMicros()
		}
	}
	terminalState := report.GetState() == BootReportState_BOOT_REPORT_STATE_READY ||
		report.GetState() == BootReportState_BOOT_REPORT_STATE_FAILED ||
		report.GetState() == BootReportState_BOOT_REPORT_STATE_ABORTED
	terminalInvalid := !terminalState || total == 0 || !bootVocabularyContains(bootMarkLabels, report.GetTerminalMark()) ||
		terminalMarkCount != 1 || terminalMarkTime != total
	switch report.GetState() {
	case BootReportState_BOOT_REPORT_STATE_READY:
		terminalInvalid = terminalInvalid || report.GetTerminalMark() != report.GetUsableMark() || usableMarkCount != 1 || report.GetTerminalErrorCode() != ""
	case BootReportState_BOOT_REPORT_STATE_FAILED:
		terminalInvalid = terminalInvalid || usableMarkCount != 0 || !bootVocabularyContains(bootTerminalErrorCodes, report.GetTerminalErrorCode())
	case BootReportState_BOOT_REPORT_STATE_ABORTED:
		terminalInvalid = terminalInvalid || usableMarkCount != 0 || report.GetTerminalErrorCode() != "" && !bootVocabularyContains(bootTerminalErrorCodes, report.GetTerminalErrorCode())
	}
	if terminalInvalid {
		validation.Violations = append(validation.Violations,
			bootViolation(BootValidationViolationKind_BOOT_VALIDATION_VIOLATION_KIND_TERMINAL_CONTRACT))
	}

	marks := report.GetMarks()
	markOrderInvalid := len(marks) < 2 || marks[0] == nil || marks[0].GetMonotonicMicros() != 0
	for idx, mark := range marks {
		if mark == nil || mark.GetSequence() == 0 || !bootVocabularyContains(bootMarkLabels, mark.GetLabel()) ||
			!bootVocabularyContains(bootOwners, mark.GetSourceOwner()) || !validBootPhase(mark.GetPhase()) ||
			mark.GetMonotonicMicros() > total {
			markOrderInvalid = true
			break
		}
		if idx != 0 && (mark.GetSequence() <= marks[idx-1].GetSequence() ||
			mark.GetMonotonicMicros() < marks[idx-1].GetMonotonicMicros()) {
			markOrderInvalid = true
			break
		}
	}
	if len(marks) != 0 && marks[len(marks)-1].GetMonotonicMicros() != total {
		markOrderInvalid = true
	}
	if markOrderInvalid {
		validation.Violations = append(validation.Violations,
			bootViolation(BootValidationViolationKind_BOOT_VALIDATION_VIOLATION_KIND_MARK_ORDER))
	}

	children, spanContractInvalid := validateBootSpanStructure(report.GetSpans(), total)
	if spanContractInvalid {
		validation.Violations = append(validation.Violations,
			bootViolation(BootValidationViolationKind_BOOT_VALIDATION_VIOLATION_KIND_SPAN_CONTRACT))
	}

	phaseTotals := make(map[BootPhase]uint64)
	threshold := total / noGapDivisor
	if !markOrderInvalid && !spanContractInvalid && total > 0 {
		for idx := 0; idx < len(marks)-1; idx++ {
			before, after := marks[idx], marks[idx+1]
			start, end := before.GetMonotonicMicros(), after.GetMonotonicMicros()
			duration := end - start
			phaseTotals[before.GetPhase()] += duration
			if duration <= threshold {
				continue
			}
			topLevel := children[""]
			topIntervals := make([]bootInterval, 0, len(topLevel))
			for _, span := range topLevel {
				if interval, ok := clippedBootInterval(span, start, end); ok {
					topIntervals = append(topIntervals, interval)
				}
			}
			covered := bootIntervalUnionDuration(topIntervals)
			validation.LongestGaps = append(validation.LongestGaps, &BootGap{
				BeforeSequence: before.GetSequence(), AfterSequence: after.GetSequence(),
				StartMonotonicMicros: start, EndMonotonicMicros: end,
				DurationMicros: duration, CoveredDurationMicros: covered,
			})
			if duration-covered > duration/noGapDivisor || duration-covered > threshold {
				validation.Violations = append(validation.Violations, &BootValidationViolation{
					Kind:                 BootValidationViolationKind_BOOT_VALIDATION_VIOLATION_KIND_GAP_COVERAGE,
					StartMonotonicMicros: start, EndMonotonicMicros: end, MarkSequence: before.GetSequence(),
				})
			}
			actionable := validateBootSpanTree(start, end, threshold, topLevel, children, &validation.Violations)
			validation.UnknownDurationMicros += duration - bootIntervalUnionDuration(actionable)
		}
	}

	if total != 0 {
		hi, lo := bits.Mul64(validation.UnknownDurationMicros, 1_000_000)
		ppm, _ := bits.Div64(hi, lo, total)
		validation.UnknownPartsPerMillion = uint32(ppm)
	}
	if !spanContractInvalid && validation.UnknownDurationMicros > threshold {
		validation.Violations = append(validation.Violations,
			bootViolation(BootValidationViolationKind_BOOT_VALIDATION_VIOLATION_KIND_UNKNOWN_SHARE))
	}
	phases := make([]BootPhase, 0, len(phaseTotals))
	for phase := range phaseTotals {
		if phase != BootPhase_BOOT_PHASE_UNKNOWN {
			phases = append(phases, phase)
		}
	}
	slices.Sort(phases)
	for _, phase := range phases {
		validation.PhaseDurations = append(validation.PhaseDurations, &BootPhaseDuration{Phase: phase, DurationMicros: phaseTotals[phase]})
	}
	slices.SortStableFunc(validation.LongestGaps, func(a, b *BootGap) int {
		if c := cmp.Compare(b.GetDurationMicros(), a.GetDurationMicros()); c != 0 {
			return c
		}
		return cmp.Compare(a.GetBeforeSequence(), b.GetBeforeSequence())
	})
	validation.Pass = len(validation.Violations) == 0
	return validation
}
