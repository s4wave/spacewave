package bldr_project_controller

// runningManifestBuilder contains information about a running manifest builder.
type runningManifestBuilder struct {
	// key is the b58 encoded conf
	key string
	// conf is the builder config
	conf *ManifestBuilderConfig
	// tracker is the manifest builder tracker
	tracker *manifestBuilderTracker
}

// getRunningManifestBuilders returns the list of running manifest builders.
func (c *Controller) getRunningManifestBuilders() []runningManifestBuilder {
	entries := c.manifestBuilders.GetKeysWithData()
	results := make([]runningManifestBuilder, 0, len(entries))
	for _, entry := range entries {
		builderConf, err := UnmarshalManifestBuilderConfigB58(entry.Key)
		if err != nil {
			// not possible, but check anyway.
			continue
		}

		results = append(results, runningManifestBuilder{
			key:     entry.Key,
			tracker: entry.Data,
			conf:    builderConf,
		})
	}

	return results
}
