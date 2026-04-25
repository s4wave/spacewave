//go:build !js

package memlab

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
)

// AnalysisResult is the JSON output from analyze.js.
type AnalysisResult struct {
	Snapshots   []SnapshotAnalysis `json:"snapshots"`
	Deltas      ObjectCounts       `json:"deltas"`
	TopRetained []RetainedRpcEntry `json:"topRetained"`
	PairDeltas  []RetainedRpcDelta `json:"pairDeltas"`
}

// SnapshotAnalysis is per-snapshot analysis output.
type SnapshotAnalysis struct {
	Label       string             `json:"label"`
	Path        string             `json:"path"`
	Counts      ObjectCounts       `json:"counts"`
	TopRetained []RetainedRpcEntry `json:"topRetained"`
}

// ObjectCounts holds counts of leak-target objects.
type ObjectCounts struct {
	ClientRpc     int `json:"clientRpc"`
	ChannelStream int `json:"channelStream"`
	Promise       int `json:"promise"`
	Generator     int `json:"generator"`
	OnNext        int `json:"onNext"`
}

// RetainedRpcEntry is a service/method pair with count.
type RetainedRpcEntry struct {
	Service string `json:"service"`
	Method  string `json:"method"`
	Count   int    `json:"count"`
}

// RetainedRpcDelta is a per-pair retained count delta between first and last snapshots.
type RetainedRpcDelta struct {
	Service       string `json:"service"`
	Method        string `json:"method"`
	BaselineCount int    `json:"baselineCount"`
	FinalCount    int    `json:"finalCount"`
	Delta         int    `json:"delta"`
}

// BuildSnapshotArg builds the ordered label=path argument for analyze.js.
func BuildSnapshotArg(snapshots *SnapshotSet) (string, error) {
	var pairs []string
	for _, label := range snapshots.SnapshotLabels() {
		path, ok := snapshots.Snapshots[label]
		if !ok {
			return "", errors.Errorf("missing snapshot path for label %q", label)
		}
		pairs = append(pairs, label+"="+path)
	}
	if len(pairs) == 0 {
		return "", errors.New("no snapshots to analyze")
	}
	return strings.Join(pairs, ","), nil
}

// SortPairDeltas sorts pair deltas by descending delta, then final count, then name.
func SortPairDeltas(pairDeltas []RetainedRpcDelta) {
	if len(pairDeltas) < 2 {
		return
	}
	slices.SortFunc(pairDeltas, func(a, b RetainedRpcDelta) int {
		if a.Delta != b.Delta {
			return b.Delta - a.Delta
		}
		if a.FinalCount != b.FinalCount {
			return b.FinalCount - a.FinalCount
		}
		if a.Service != b.Service {
			return strings.Compare(a.Service, b.Service)
		}
		return strings.Compare(a.Method, b.Method)
	})
}

// scriptDir returns the directory containing this Go source file,
// which is also where analyze.js lives.
func scriptDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Dir(file)
}

// EnsureDeps runs bun install in the memlab dir if node_modules is missing.
// Returns an error if installation fails.
func EnsureDeps() error {
	dir := scriptDir()
	if _, err := os.Stat(filepath.Join(dir, "node_modules")); err == nil {
		return nil
	}
	cmd := exec.Command("bun", "install")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return errors.Wrap(err, "bun install")
	}
	return nil
}

// RunAnalysis invokes analyze.js with the given snapshot set and returns
// the parsed result.
func RunAnalysis(snapshots *SnapshotSet) (*AnalysisResult, error) {
	dir := scriptDir()
	script := filepath.Join(dir, "analyze.js")

	arg, err := BuildSnapshotArg(snapshots)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command("node", script, "--snapshots", arg)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, errors.Errorf("analyze.js failed: %s\n%s", err, string(exitErr.Stderr))
		}
		return nil, errors.Wrap(err, "run analyze.js")
	}

	result, err := parseAnalysisResult(out)
	if err != nil {
		return nil, errors.Wrap(err, "parse analyze.js output")
	}
	SortPairDeltas(result.PairDeltas)
	return result, nil
}

func parseAnalysisResult(dat []byte) (*AnalysisResult, error) {
	var p fastjson.Parser
	v, err := p.ParseBytes(dat)
	if err != nil {
		return nil, err
	}

	result := &AnalysisResult{
		Deltas:      parseObjectCountsValue(v.Get("deltas")),
		TopRetained: parseRetainedRpcEntries(v.GetArray("topRetained")),
		PairDeltas:  parseRetainedRpcDeltas(v.GetArray("pairDeltas")),
	}
	for _, snapshotValue := range v.GetArray("snapshots") {
		result.Snapshots = append(result.Snapshots, parseSnapshotAnalysisValue(snapshotValue))
	}
	return result, nil
}

func parseSnapshotAnalysisValue(v *fastjson.Value) SnapshotAnalysis {
	if v == nil {
		return SnapshotAnalysis{}
	}
	return SnapshotAnalysis{
		Label:       string(v.GetStringBytes("label")),
		Path:        string(v.GetStringBytes("path")),
		Counts:      parseObjectCountsValue(v.Get("counts")),
		TopRetained: parseRetainedRpcEntries(v.GetArray("topRetained")),
	}
}

func parseObjectCountsValue(v *fastjson.Value) ObjectCounts {
	if v == nil {
		return ObjectCounts{}
	}
	return ObjectCounts{
		ClientRpc:     v.GetInt("clientRpc"),
		ChannelStream: v.GetInt("channelStream"),
		Promise:       v.GetInt("promise"),
		Generator:     v.GetInt("generator"),
		OnNext:        v.GetInt("onNext"),
	}
}

func parseRetainedRpcEntries(values []*fastjson.Value) []RetainedRpcEntry {
	entries := make([]RetainedRpcEntry, 0, len(values))
	for _, value := range values {
		if value == nil {
			continue
		}
		entries = append(entries, RetainedRpcEntry{
			Service: string(value.GetStringBytes("service")),
			Method:  string(value.GetStringBytes("method")),
			Count:   value.GetInt("count"),
		})
	}
	return entries
}

func parseRetainedRpcDeltas(values []*fastjson.Value) []RetainedRpcDelta {
	deltas := make([]RetainedRpcDelta, 0, len(values))
	for _, value := range values {
		if value == nil {
			continue
		}
		deltas = append(deltas, RetainedRpcDelta{
			Service:       string(value.GetStringBytes("service")),
			Method:        string(value.GetStringBytes("method")),
			BaselineCount: value.GetInt("baselineCount"),
			FinalCount:    value.GetInt("finalCount"),
			Delta:         value.GetInt("delta"),
		})
	}
	return deltas
}
