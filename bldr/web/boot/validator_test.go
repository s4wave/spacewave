package boot

import (
	"bytes"
	"encoding/json" //nolint:depguard // json.Compact canonicalizes formatter whitespace in a test fixture.
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func loadBootReportFixture(t *testing.T, name string) *BootReport {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	report := new(BootReport)
	if err := report.UnmarshalJSON(data); err != nil {
		t.Fatal(err)
	}
	return report
}

func bootViolationKinds(validation *BootValidation) []BootValidationViolationKind {
	kinds := make([]BootValidationViolationKind, 0, len(validation.GetViolations()))
	for _, violation := range validation.GetViolations() {
		kinds = append(kinds, violation.GetKind())
	}
	return kinds
}

func TestValidateBootReportGoldenReports(t *testing.T) {
	tests := []struct {
		name  string
		pass  bool
		kinds []BootValidationViolationKind
	}{
		{name: "successful", pass: true},
		{name: "failed", pass: true},
		{name: "aborted", pass: true},
		{name: "concurrent", pass: true},
		{name: "reordered", kinds: []BootValidationViolationKind{
			BootValidationViolationKind_BOOT_VALIDATION_VIOLATION_KIND_MARK_ORDER,
		}},
		{name: "stalled", kinds: []BootValidationViolationKind{
			BootValidationViolationKind_BOOT_VALIDATION_VIOLATION_KIND_GAP_COVERAGE,
			BootValidationViolationKind_BOOT_VALIDATION_VIOLATION_KIND_GENERIC_LEAF,
			BootValidationViolationKind_BOOT_VALIDATION_VIOLATION_KIND_UNKNOWN_SHARE,
		}},
		{name: "actionable-ancestor", kinds: []BootValidationViolationKind{
			BootValidationViolationKind_BOOT_VALIDATION_VIOLATION_KIND_GENERIC_LEAF,
			BootValidationViolationKind_BOOT_VALIDATION_VIOLATION_KIND_GENERIC_LEAF,
			BootValidationViolationKind_BOOT_VALIDATION_VIOLATION_KIND_UNKNOWN_SHARE,
		}},
		{name: "duplicate-span", kinds: []BootValidationViolationKind{
			BootValidationViolationKind_BOOT_VALIDATION_VIOLATION_KIND_SPAN_CONTRACT,
		}},
		{name: "deep-span", kinds: []BootValidationViolationKind{
			BootValidationViolationKind_BOOT_VALIDATION_VIOLATION_KIND_SPAN_CONTRACT,
		}},
		{name: "post-terminal", kinds: []BootValidationViolationKind{
			BootValidationViolationKind_BOOT_VALIDATION_VIOLATION_KIND_TERMINAL_CONTRACT,
		}},
		{name: "invalid-privacy", kinds: []BootValidationViolationKind{
			BootValidationViolationKind_BOOT_VALIDATION_VIOLATION_KIND_TERMINAL_CONTRACT,
		}},
		{name: "invalid-accounting", kinds: []BootValidationViolationKind{
			BootValidationViolationKind_BOOT_VALIDATION_VIOLATION_KIND_REPORT_CONTRACT,
		}},
		{name: "nonfinite-scalar", kinds: []BootValidationViolationKind{
			BootValidationViolationKind_BOOT_VALIDATION_VIOLATION_KIND_REPORT_CONTRACT,
		}},
		{name: "oversized-record", kinds: []BootValidationViolationKind{
			BootValidationViolationKind_BOOT_VALIDATION_VIOLATION_KIND_REPORT_CONTRACT,
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			report := loadBootReportFixture(t, test.name)
			validation := ValidateBootReport(report)
			if !reflect.DeepEqual(validation, report.GetValidation()) {
				t.Fatalf("validation differs from shared golden: got %v, want %v", validation, report.GetValidation())
			}
			if validation.GetPass() != test.pass {
				t.Fatalf("pass = %v, violations = %v", validation.GetPass(), validation.GetViolations())
			}
			if kinds := bootViolationKinds(validation); !slices.Equal(kinds, test.kinds) {
				t.Fatalf("violation kinds = %v, want %v", kinds, test.kinds)
			}
		})
	}
}

func TestBootReportGeneratedCodeRoundTrip(t *testing.T) {
	report := loadBootReportFixture(t, "successful")
	binary, err := report.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	decoded := new(BootReport)
	if err := decoded.UnmarshalVT(binary); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, report) {
		t.Fatal("binary round trip changed report")
	}
	jsonData, err := decoded.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	jsonDecoded := new(BootReport)
	if err := jsonDecoded.UnmarshalJSON(jsonData); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(jsonDecoded, report) {
		t.Fatal("proto JSON round trip changed report")
	}
}

func TestMarshalBootReportJSONMatchesCrossLanguageGolden(t *testing.T) {
	report := loadBootReportFixture(t, "successful")
	golden, err := os.ReadFile(filepath.Join("testdata", "successful.canonical.json"))
	if err != nil {
		t.Fatal(err)
	}
	goldenReport := new(BootReport)
	if err := goldenReport.UnmarshalJSON(golden); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(goldenReport, report) {
		t.Fatal("canonical proto JSON changed report semantics")
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, golden); err != nil {
		t.Fatal(err)
	}
	for range 20 {
		encoded, err := MarshalBootReportJSON(report)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(encoded, compact.Bytes()) {
			t.Fatalf("canonical proto JSON differs:\n got %s\nwant %s", encoded, compact.Bytes())
		}
	}
}

func TestValidateBootReportRejectsMultipleUsableMarks(t *testing.T) {
	report := loadBootReportFixture(t, "successful").CloneVT()
	report.Marks[1].Label = report.GetUsableMark()
	validation := ValidateBootReport(report)
	if !slices.Contains(bootViolationKinds(validation),
		BootValidationViolationKind_BOOT_VALIDATION_VIOLATION_KIND_TERMINAL_CONTRACT) {
		t.Fatalf("violations = %v, want terminal contract", validation.GetViolations())
	}
}

func TestValidateBootReportRejectsPrivacyVocabulary(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "privacy-vectors.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for value := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		t.Run(value, func(t *testing.T) {
			report := loadBootReportFixture(t, "failed").CloneVT()
			report.TerminalErrorCode = value
			validation := ValidateBootReport(report)
			if !slices.Contains(bootViolationKinds(validation),
				BootValidationViolationKind_BOOT_VALIDATION_VIOLATION_KIND_TERMINAL_CONTRACT) {
				t.Fatalf("violations = %v, want terminal contract", validation.GetViolations())
			}
		})
	}
}

func TestValidateBootReportRejectsCollectionAndDetailBounds(t *testing.T) {
	report := loadBootReportFixture(t, "successful").CloneVT()
	report.Marks = make([]*BootMark, maxBootMarks+1)
	if !slices.Contains(bootViolationKinds(ValidateBootReport(report)),
		BootValidationViolationKind_BOOT_VALIDATION_VIOLATION_KIND_REPORT_CONTRACT) {
		t.Fatal("oversized marks did not fail the report contract")
	}

	report = loadBootReportFixture(t, "successful").CloneVT()
	report.Marks[0].Detail[0], report.Marks[0].Detail[1] = report.Marks[0].Detail[1], report.Marks[0].Detail[0]
	if !slices.Contains(bootViolationKinds(ValidateBootReport(report)),
		BootValidationViolationKind_BOOT_VALIDATION_VIOLATION_KIND_REPORT_CONTRACT) {
		t.Fatal("unordered detail did not fail the report contract")
	}
}

func TestValidateBootReportSharedVocabularyVectors(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "vocabulary-vectors.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		parts := strings.Split(line, "|")
		t.Run(parts[0]+"-"+parts[1]+"-"+parts[2], func(t *testing.T) {
			report := loadBootReportFixture(t, "successful").CloneVT()
			switch parts[1] {
			case "report-id":
				report.ReportId = parts[2]
			case "mark-label":
				report.Marks[1].Label = parts[2]
			case "detail-string":
				report.Marks[0].Detail[1].Value.Value = &BootValue_StringValue{StringValue: parts[2]}
			case "operation":
				report.Spans[1].Operation = parts[2]
			case "browser-engine":
				report.Environment.BrowserEngine = parts[2]
			case "os-family":
				report.Environment.OsFamily = parts[2]
			case "worker-mode":
				mode, err := strconv.Atoi(parts[2])
				if err != nil {
					t.Fatal(err)
				}
				report.Build.WorkerMode = BootWorkerMode(mode)
			default:
				t.Fatalf("unknown target %q", parts[1])
			}
			validation := ValidateBootReport(report)
			if parts[0] == "accept" {
				if !validation.GetPass() {
					t.Fatalf("violations = %v, want pass", validation.GetViolations())
				}
				return
			}
			want := map[string]BootValidationViolationKind{
				"REPORT_CONTRACT": BootValidationViolationKind_BOOT_VALIDATION_VIOLATION_KIND_REPORT_CONTRACT,
				"SPAN_CONTRACT":   BootValidationViolationKind_BOOT_VALIDATION_VIOLATION_KIND_SPAN_CONTRACT,
			}[parts[3]]
			if !slices.Contains(bootViolationKinds(validation), want) {
				t.Fatalf("violations = %v, want %v", validation.GetViolations(), want)
			}
		})
	}
}

func TestValidateBootReportRejectsSharedUnsupportedEnumVectorsFromBinary(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "unsupported-enums.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		parts := strings.Split(line, "|")
		t.Run(parts[0], func(t *testing.T) {
			report := loadBootReportFixture(t, "successful").CloneVT()
			switch parts[0] {
			case "report-state":
				report.State = BootReportState(99)
			case "build-type":
				report.Build.BuildType = BootBuildType(99)
			case "runtime-kind":
				report.Build.RuntimeKind = BootRuntimeKind(99)
			case "worker-mode":
				report.Build.WorkerMode = BootWorkerMode(99)
			case "environment-class":
				report.Environment.Class = BootEnvironmentClass(99)
			case "service-worker-state":
				report.Environment.ServiceWorkerState = BootServiceWorkerState(99)
			case "cache-state":
				report.Environment.CacheState = BootCacheState(99)
			case "recovery-decision":
				report.Environment.RecoveryDecision = BootRecoveryDecision(99)
			case "mark-phase":
				report.Marks[0].Phase = BootPhase(99)
			case "counter-unit":
				report.Accounting.Samples[0].Unit = BootCounterUnit(99)
			case "attachment-kind":
				report.Attachments = []*BootAttachment{{
					ArtifactId: "fixture-artifact", Kind: BootAttachmentKind(99),
					ContentHash: strings.Repeat("a", 64), SizeBytes: 1, ReleaseGeneration: "fixture",
				}}
			case "share-destination":
				report.Privacy.SharedUnixMicros = 1
				report.Privacy.ShareDestination = BootShareDestination(99)
			case "span-result":
				report.Spans[0].Result = BootSpanResult(99)
			case "span-work-class":
				report.Spans[0].WorkClass = BootWorkClass(99)
			default:
				t.Fatalf("unknown enum target %q", parts[0])
			}
			binary, marshalErr := report.MarshalVT()
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			decoded := new(BootReport)
			if unmarshalErr := decoded.UnmarshalVT(binary); unmarshalErr != nil {
				t.Fatal(unmarshalErr)
			}
			want := map[string]BootValidationViolationKind{
				"REPORT_CONTRACT":   BootValidationViolationKind_BOOT_VALIDATION_VIOLATION_KIND_REPORT_CONTRACT,
				"TERMINAL_CONTRACT": BootValidationViolationKind_BOOT_VALIDATION_VIOLATION_KIND_TERMINAL_CONTRACT,
				"MARK_ORDER":        BootValidationViolationKind_BOOT_VALIDATION_VIOLATION_KIND_MARK_ORDER,
				"SPAN_CONTRACT":     BootValidationViolationKind_BOOT_VALIDATION_VIOLATION_KIND_SPAN_CONTRACT,
			}[parts[2]]
			if kinds := bootViolationKinds(ValidateBootReport(decoded)); !slices.Contains(kinds, want) {
				t.Fatalf("violations = %v, want %v", kinds, want)
			}
		})
	}
}

func TestValidateBootReportReturnsBeforeOversizedGraphWork(t *testing.T) {
	report := loadBootReportFixture(t, "successful").CloneVT()
	report.Spans = make([]*BootSpan, maxBootSpans+1)
	want := []BootValidationViolationKind{
		BootValidationViolationKind_BOOT_VALIDATION_VIOLATION_KIND_REPORT_CONTRACT,
	}
	if kinds := bootViolationKinds(ValidateBootReport(report)); !slices.Equal(kinds, want) {
		t.Fatalf("violations = %v, want early %v", kinds, want)
	}
}

func TestValidateBootReportReviewRegressions(t *testing.T) {
	hasReportContract := func(report *BootReport) bool {
		return slices.Contains(bootViolationKinds(ValidateBootReport(report)),
			BootValidationViolationKind_BOOT_VALIDATION_VIOLATION_KIND_REPORT_CONTRACT)
	}

	t.Run("oversized invalid vocabulary", func(t *testing.T) {
		report := loadBootReportFixture(t, "successful").CloneVT()
		report.TerminalErrorCode = strings.Repeat("x", maxBootRecordBytes)
		if !hasReportContract(report) {
			t.Fatal("oversized report bypassed the report contract")
		}
	})

	t.Run("numeric identifier suffixes", func(t *testing.T) {
		report := loadBootReportFixture(t, "successful").CloneVT()
		report.ReportId = "boot-report-1"
		report.Spans[0].SpanId = "span-1"
		for _, span := range report.Spans[1:] {
			if span.ParentSpanId == "runtime" {
				span.ParentSpanId = "span-1"
			}
		}
		report.Attachments = []*BootAttachment{{
			ArtifactId: "boot-artifact-1", Kind: BootAttachmentKind_BOOT_ATTACHMENT_KIND_SOURCE_MAP,
			ContentHash: strings.Repeat("a", 64), SizeBytes: 1, ReleaseGeneration: report.Build.ReleaseGeneration,
		}}
		if validation := ValidateBootReport(report); !validation.GetPass() {
			t.Fatalf("numeric suffixes rejected: %v", validation.GetViolations())
		}
	})

	t.Run("stored validation metadata ignored", func(t *testing.T) {
		report := loadBootReportFixture(t, "successful").CloneVT()
		report.Validation.Violations = []*BootValidationViolation{{Kind: BootValidationViolationKind(99)}}
		if validation := ValidateBootReport(report); !validation.GetPass() {
			t.Fatalf("stored validation changed derived result: %v", validation.GetViolations())
		}
	})

	t.Run("failed usable mark", func(t *testing.T) {
		report := loadBootReportFixture(t, "failed").CloneVT()
		report.Marks[0].Label = report.UsableMark
		if !slices.Contains(bootViolationKinds(ValidateBootReport(report)),
			BootValidationViolationKind_BOOT_VALIDATION_VIOLATION_KIND_TERMINAL_CONTRACT) {
			t.Fatal("failed report with usable mark passed terminal contract")
		}
	})

	t.Run("accounting time between marks", func(t *testing.T) {
		report := loadBootReportFixture(t, "successful").CloneVT()
		report.Accounting.Samples[0].MonotonicMicros = 1
		if !hasReportContract(report) {
			t.Fatal("accounting sample between mark boundaries passed")
		}
	})

	t.Run("attachment generation mismatch", func(t *testing.T) {
		report := loadBootReportFixture(t, "successful").CloneVT()
		report.Attachments = []*BootAttachment{{
			ArtifactId: "boot-artifact-1", Kind: BootAttachmentKind_BOOT_ATTACHMENT_KIND_SOURCE_MAP,
			ContentHash: strings.Repeat("a", 64), SizeBytes: 1, ReleaseGeneration: "other",
		}}
		if !hasReportContract(report) {
			t.Fatal("attachment from another release generation passed")
		}
	})
}
