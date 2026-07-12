//go:build js

package opfs

import (
	"math"
	"syscall/js"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/opfs/jsutil"
)

// StorageEstimate describes current origin storage usage and quota.
type StorageEstimate struct {
	Usage uint64
	Quota uint64
}

// EstimateStorage reads current origin usage and quota from StorageManager.
func EstimateStorage() (*StorageEstimate, error) {
	storage := js.Global().Get("navigator").Get("storage")
	if storage.IsUndefined() || storage.IsNull() {
		return nil, errors.New("StorageManager is unavailable")
	}
	estimateFn := storage.Get("estimate")
	if estimateFn.IsUndefined() || estimateFn.IsNull() || estimateFn.Type() != js.TypeFunction {
		return nil, errors.New("StorageManager.estimate is unavailable")
	}
	estimate, err := AwaitPromise(jsutil.Call(storage, "estimate"))
	if err != nil {
		return nil, errors.Wrap(err, "estimate browser storage")
	}
	usage := estimate.Get("usage")
	quota := estimate.Get("quota")
	if usage.Type() != js.TypeNumber || quota.Type() != js.TypeNumber {
		return nil, errors.New("StorageManager.estimate returned invalid usage or quota")
	}
	usageValue := usage.Float()
	quotaValue := quota.Float()
	if usageValue < 0 || quotaValue < 0 ||
		math.IsNaN(usageValue) || math.IsNaN(quotaValue) ||
		math.IsInf(usageValue, 0) || math.IsInf(quotaValue, 0) {
		return nil, errors.New("StorageManager.estimate returned non-finite usage or quota")
	}
	return &StorageEstimate{
		Usage: uint64(usageValue),
		Quota: uint64(quotaValue),
	}, nil
}
