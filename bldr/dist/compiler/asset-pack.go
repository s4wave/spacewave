package bldr_dist_compiler

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	bldr_dist_assetpack "github.com/s4wave/spacewave/bldr/dist/assetpack"
)

const webAssetPackPartSize int64 = 256 * 1024 * 1024

func copyWebAssetPack(sourcePath, outputDir, urlPrefix string, partSize int64) ([]bldr_dist_assetpack.Part, error) {
	source, err := os.Open(sourcePath)
	if err != nil {
		return nil, err
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() <= partSize {
		name := "assets.kvfile"
		if err := copyAssetPackPart(source, filepath.Join(outputDir, name), info.Size()); err != nil {
			return nil, err
		}
		return []bldr_dist_assetpack.Part{{URL: urlPrefix + name, Size: info.Size()}}, nil
	}
	parts := make([]bldr_dist_assetpack.Part, 0, (info.Size()+partSize-1)/partSize)
	for remaining, index := info.Size(), 0; remaining > 0; index++ {
		size := min(remaining, partSize)
		name := fmt.Sprintf("assets.kvfile-%03d", index)
		if err := copyAssetPackPart(source, filepath.Join(outputDir, name), size); err != nil {
			return nil, err
		}
		parts = append(parts, bldr_dist_assetpack.Part{URL: urlPrefix + name, Size: size})
		remaining -= size
	}
	return parts, nil
}

func copyAssetPackPart(source io.Reader, path string, size int64) error {
	output, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.CopyN(output, source, size)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
