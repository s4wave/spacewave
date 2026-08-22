package lean

import "github.com/s4wave/spacewave/db/bucket"

func bucketConfigLeanPtr() *bucket.Config {
	return &bucket.Config{Id: "test-bucket", Rev: 1}
}
