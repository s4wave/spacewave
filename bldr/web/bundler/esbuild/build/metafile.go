package bldr_web_bundler_esbuild_build

import "github.com/aperturerobotics/fastjson"

// EsbuildMetafile contains a JSON object with information about inputs and outputs.
//
// The paths in the outputs are relative to the working directory.
type EsbuildMetafile struct {
	Inputs  map[string]EsbuildMetafileInput  `json:"inputs"`
	Outputs map[string]EsbuildMetaFileOutput `json:"outputs"`
}

// EsbuildMetafileInput is an input in the metafile.
type EsbuildMetafileInput struct {
	Bytes int `json:"bytes"`
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

// ParseEsbuildMetafile parses an esbuild metafile with the shared fastjson path.
func ParseEsbuildMetafile(dat []byte) (*EsbuildMetafile, error) {
	var p fastjson.Parser
	v, err := p.ParseBytes(dat)
	if err != nil {
		return nil, err
	}

	meta := &EsbuildMetafile{
		Inputs:  make(map[string]EsbuildMetafileInput),
		Outputs: make(map[string]EsbuildMetaFileOutput),
	}
	if inputs := v.GetObject("inputs"); inputs != nil {
		inputs.Visit(func(k []byte, iv *fastjson.Value) {
			meta.Inputs[string(k)] = parseEsbuildMetafileInput(iv)
		})
	}
	if outputs := v.GetObject("outputs"); outputs != nil {
		outputs.Visit(func(k []byte, ov *fastjson.Value) {
			meta.Outputs[string(k)] = parseEsbuildMetaFileOutput(ov)
		})
	}
	return meta, nil
}

func parseEsbuildMetafileInput(v *fastjson.Value) EsbuildMetafileInput {
	if v == nil {
		return EsbuildMetafileInput{}
	}
	return EsbuildMetafileInput{
		Bytes: v.GetInt("bytes"),
	}
}

func parseEsbuildMetaFileOutput(v *fastjson.Value) EsbuildMetaFileOutput {
	if v == nil {
		return EsbuildMetaFileOutput{}
	}
	return EsbuildMetaFileOutput{
		Bytes:      v.GetInt("bytes"),
		EntryPoint: string(v.GetStringBytes("entryPoint")),
		CssBundle:  string(v.GetStringBytes("cssBundle")),
	}
}
