//go:build !js

package memlab

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/testbed/reporoot"
)

// AnalysisResult is the JSON output from analyze.js.
type AnalysisResult struct {
	// Snapshots holds the analysis of each snapshot in argument order.
	Snapshots []SnapshotAnalysis `json:"snapshots"`
	// Deltas is the change in object counts from the first snapshot to the last.
	Deltas ObjectCounts `json:"deltas"`
	// TopRetained lists the most retained service and method pairs.
	TopRetained []RetainedRpcEntry `json:"topRetained"`
	// PairDeltas lists the retained count change of each service and method pair.
	PairDeltas []RetainedRpcDelta `json:"pairDeltas"`
}

// SnapshotAnalysis is per-snapshot analysis output.
type SnapshotAnalysis struct {
	// Label is the snapshot label given on the command line.
	Label string `json:"label"`
	// Path is the heap snapshot file.
	Path string `json:"path"`
	// Counts holds the leak-target object counts in this snapshot.
	Counts ObjectCounts `json:"counts"`
	// TopRetained lists the most retained service and method pairs in this snapshot.
	TopRetained []RetainedRpcEntry `json:"topRetained"`
}

// ObjectCounts holds counts of leak-target objects.
type ObjectCounts struct {
	// ClientRpc counts retained client RPC objects.
	ClientRpc int `json:"clientRpc"`
	// ChannelStream counts retained channel streams.
	ChannelStream int `json:"channelStream"`
	// Promise counts retained promises.
	Promise int `json:"promise"`
	// Generator counts retained generators.
	Generator int `json:"generator"`
	// OnNext counts retained onNext callbacks.
	OnNext int `json:"onNext"`
}

// RetainedRpcEntry is a service/method pair with a count.
type RetainedRpcEntry struct {
	// Service is the RPC service name.
	Service string `json:"service"`
	// Method is the RPC method name.
	Method string `json:"method"`
	// Count is the number of retained calls.
	Count int `json:"count"`
}

// RetainedRpcDelta is a per-pair retained count delta between first and last snapshots.
type RetainedRpcDelta struct {
	// Service is the RPC service name.
	Service string `json:"service"`
	// Method is the RPC method name.
	Method string `json:"method"`
	// BaselineCount is the retained count in the first snapshot.
	BaselineCount int `json:"baselineCount"`
	// FinalCount is the retained count in the last snapshot.
	FinalCount int `json:"finalCount"`
	// Delta is FinalCount minus BaselineCount.
	Delta int `json:"delta"`
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

// scriptDir returns this package's directory, where analyze.js lives:
// e2e/wasm/memlab under the module root above the working directory.
func scriptDir() (string, error) {
	root, err := reporoot.Find()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "e2e", "wasm", "memlab"), nil
}

// EnsureDeps runs bun install in the memlab dir if node_modules is missing.
// Returns an error if installation fails.
func EnsureDeps() error {
	// Skip the install when the memlab dir already has its node_modules.
	dir, err := scriptDir()
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(dir, "node_modules")); err == nil {
		return nil
	}

	// Install the dependencies with bun.
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
	// Locate analyze.js in the memlab dir.
	dir, err := scriptDir()
	if err != nil {
		return nil, err
	}
	script := filepath.Join(dir, "analyze.js")

	// Encode the snapshot set as the script's argument.
	arg, err := BuildSnapshotArg(snapshots)
	if err != nil {
		return nil, err
	}

	// Run analyze.js and keep its stdout, reporting stderr on failure.
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

	// Parse the result and sort its pair deltas.
	result, err := parseAnalysisResult(out)
	if err != nil {
		return nil, errors.Wrap(err, "parse analyze.js output")
	}
	SortPairDeltas(result.PairDeltas)
	return result, nil
}

func parseAnalysisResult(dat []byte) (*AnalysisResult, error) {
	// Parse the whole document once. The fields below read from the document.
	var p fastjson.Parser
	v, err := p.ParseBytes(dat)
	if err != nil {
		return nil, err
	}

	// Decode the totals, then each snapshot in order.
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
