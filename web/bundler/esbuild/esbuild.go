package bldr_web_bundler_esbuild

import (
	"errors"
	"slices"
	"strings"

	bldr_esbuild_build "github.com/aperturerobotics/bldr/web/bundler/esbuild/build"
)

// BuildEsbuildOutputMetas builds output metadata from the meta file.
func BuildEsbuildOutputMetas(metaFile *bldr_esbuild_build.EsbuildMetafile, entrypoints []*EsbuildBundleEntrypoint) []*EsbuildOutputMeta {
	metas := make([]*EsbuildOutputMeta, 0, len(metaFile.Outputs))
	files := make([]string, 0, 2)
	for outputPath, outputFile := range metaFile.Outputs {
		// reset files to just [outputPath]
		files = files[:1]
		files[0] = outputPath

		// if there is a css bundle add it to files
		cssBundlePath := outputFile.CssBundle
		if cssBundlePath != "" {
			files = append(files, cssBundlePath)
		}

		// match the outputPath to an entrypoint, if any.
		outputFileEntrypoint := outputFile.EntryPoint
		var matchedEntrypoint *EsbuildBundleEntrypoint
		if outputFileEntrypoint != "" {
			for _, entrypoint := range entrypoints {
				if outputFileEntrypoint != entrypoint.GetInputPath() {
					continue
				}

				matchedEntrypoint = entrypoint
				break
			}
		}

		// add an output entry for each output path
		for _, file := range files {
			var outputCssBundlePath string
			if file != cssBundlePath {
				outputCssBundlePath = cssBundlePath
			}
			metas = append(metas, &EsbuildOutputMeta{
				Path:           file,
				Length:         uint32(outputFile.Bytes), //nolint:gosec
				CssBundlePath:  outputCssBundlePath,
				EntrypointPath: outputFile.EntryPoint,
				EntrypointId:   matchedEntrypoint.GetEntrypointId(),
			})
		}
	}
	return SortEsbuildOutputMetas(metas)
}

// SortEsbuildOutputMetas sorts and compacts a list of esbuild output meta.
func SortEsbuildOutputMetas(metas []*EsbuildOutputMeta) []*EsbuildOutputMeta {
	slices.SortFunc(metas, func(a, b *EsbuildOutputMeta) int {
		return strings.Compare(a.GetPath(), b.GetPath())
	})
	return slices.CompactFunc(metas, func(a, b *EsbuildOutputMeta) bool {
		return a.GetPath() == b.GetPath()
	})
}

// Validate validates the EsbuildBundleEntrypoint configuration.
func (e *EsbuildBundleEntrypoint) Validate() error {
	if e.GetInputPath() == "" {
		return errors.New("input_path is required")
	}
	return nil
}
