//go:build js && !goscript

package cdn_world_controller

func shouldRetryMissingPublishedHead() bool {
	return true
}
