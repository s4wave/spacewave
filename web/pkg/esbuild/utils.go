package web_pkg_esbuild

// mergeMapOverwrite merges two maps together overwriting values in target.
func mergeMapOverwrite[K comparable, T any](target *map[K]T, source map[K]T) {
	if *target == nil {
		*target = make(map[K]T)
	}
	for k, v := range source {
		(*target)[k] = v
	}
}

// mergeValueIfSet overwrites the target value if the source value is not zero.
func mergeValueIfSet[T comparable](target *T, source T) {
	var zero T
	if target != nil && source != zero {
		*target = source
	}
}
