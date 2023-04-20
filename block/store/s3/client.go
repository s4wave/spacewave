package block_store_s3

import (
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// BuildClient constructs a minio client from the config.
func BuildClient(conf *ClientConfig) (*minio.Client, error) {
	opts := &minio.Options{
		Region: conf.GetRegion(),
	}
	if !conf.GetDisableSsl() {
		opts.Secure = true
	}
	creds := conf.GetCredentials()
	if accessKeyID := creds.GetAccessKeyId(); accessKeyID != "" || creds.GetToken() != "" {
		opts.Creds = credentials.NewStaticV4(accessKeyID, creds.GetSecretAccessKey(), creds.GetToken())
	}
	return minio.New(conf.GetEndpoint(), opts)
}
