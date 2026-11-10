package main

import "errors"

// packCheckResult is what packcheck found.
type packCheckResult struct {
	dropped, noRow, deadPack, needed, noEntry, missing, mismatch, entriesMissing int64
}

// runPackCheck is not written yet.
func runPackCheck(o opts) (packCheckResult, error) {
	return packCheckResult{}, errors.New("packcheck: not implemented")
}
