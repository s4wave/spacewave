//go:build !js

package memlab

import "github.com/aperturerobotics/fastjson"

type runtimePairCount struct {
	Service string `json:"service"`
	Method  string `json:"method"`
	Count   int    `json:"count"`
}

type runtimeResourceIDCount struct {
	ResourceID int `json:"resourceId"`
	Count      int `json:"count"`
}

type runtimeRPCSummary struct {
	TotalCount   int                `json:"totalCount"`
	ActiveCount  int                `json:"activeCount"`
	ClosedCount  int                `json:"closedCount"`
	ActiveByPair []runtimePairCount `json:"activeByPair"`
	ClosedByPair []runtimePairCount `json:"closedByPair"`
}

type runtimeResourceSummary struct {
	ClientTotalCount         int                      `json:"clientTotalCount"`
	ClientActiveCount        int                      `json:"clientActiveCount"`
	ClientDisposedCount      int                      `json:"clientDisposedCount"`
	RefTotalCount            int                      `json:"refTotalCount"`
	RefActiveCount           int                      `json:"refActiveCount"`
	RefReleasedCount         int                      `json:"refReleasedCount"`
	ActiveRefsByResourceID   []runtimeResourceIDCount `json:"activeRefsByResourceId"`
	ReleasedRefsByResourceID []runtimeResourceIDCount `json:"releasedRefsByResourceId"`
}

type runtimeDebugSummary struct {
	RPC      runtimeRPCSummary      `json:"rpc"`
	Resource runtimeResourceSummary `json:"resource"`
}

func parseRuntimeDebugSummary(dat string) (runtimeDebugSummary, error) {
	var p fastjson.Parser
	v, err := p.Parse(dat)
	if err != nil {
		return runtimeDebugSummary{}, err
	}
	return runtimeDebugSummary{
		RPC: runtimeRPCSummary{
			TotalCount:   v.GetInt("rpc", "totalCount"),
			ActiveCount:  v.GetInt("rpc", "activeCount"),
			ClosedCount:  v.GetInt("rpc", "closedCount"),
			ActiveByPair: parseRuntimePairCounts(v.GetArray("rpc", "activeByPair")),
			ClosedByPair: parseRuntimePairCounts(v.GetArray("rpc", "closedByPair")),
		},
		Resource: runtimeResourceSummary{
			ClientTotalCount:         v.GetInt("resource", "clientTotalCount"),
			ClientActiveCount:        v.GetInt("resource", "clientActiveCount"),
			ClientDisposedCount:      v.GetInt("resource", "clientDisposedCount"),
			RefTotalCount:            v.GetInt("resource", "refTotalCount"),
			RefActiveCount:           v.GetInt("resource", "refActiveCount"),
			RefReleasedCount:         v.GetInt("resource", "refReleasedCount"),
			ActiveRefsByResourceID:   parseRuntimeResourceIDCounts(v.GetArray("resource", "activeRefsByResourceId")),
			ReleasedRefsByResourceID: parseRuntimeResourceIDCounts(v.GetArray("resource", "releasedRefsByResourceId")),
		},
	}, nil
}

func parseRuntimePairCounts(values []*fastjson.Value) []runtimePairCount {
	counts := make([]runtimePairCount, 0, len(values))
	for _, value := range values {
		if value == nil {
			continue
		}
		counts = append(counts, runtimePairCount{
			Service: string(value.GetStringBytes("service")),
			Method:  string(value.GetStringBytes("method")),
			Count:   value.GetInt("count"),
		})
	}
	return counts
}

func parseRuntimeResourceIDCounts(values []*fastjson.Value) []runtimeResourceIDCount {
	counts := make([]runtimeResourceIDCount, 0, len(values))
	for _, value := range values {
		if value == nil {
			continue
		}
		counts = append(counts, runtimeResourceIDCount{
			ResourceID: value.GetInt("resourceId"),
			Count:      value.GetInt("count"),
		})
	}
	return counts
}
