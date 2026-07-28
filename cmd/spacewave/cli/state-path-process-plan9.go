//go:build !js && !wasip1 && plan9

package spacewave_cli

func statePathLeaseProcessAlive(int) bool {
	return true
}
