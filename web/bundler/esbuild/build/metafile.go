package bldr_web_bundler_esbuild_build

// EsbuildMetafile contains a JSON object with information about inputs and outputs.
//
// The paths in the outputs are relative to the working directory.
type EsbuildMetafile struct {
	Inputs map[string]struct {
		Bytes   int         `json:"bytes"`
		Imports interface{} `json:"imports"`
	} `json:"inputs"`
	Outputs map[string]EsbuildMetaFileOutput `json:"outputs"`
}

// EsbuildMetaFileOutput is an output in the metafile.
type EsbuildMetaFileOutput struct {
	Bytes      int    `json:"bytes"`
	EntryPoint string `json:"entryPoint"`
	CssBundle  string `json:"cssBundle"`
	// Imports
	// Exports
	// EntryPoint?
	// CssBundle?
}
