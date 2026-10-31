//go:build !js

package spacewave_cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/dustin/go-humanize"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
)

// s3UploadReportInterval is the time between progress lines of s3-upload.
const s3UploadReportInterval = 5 * time.Second

// s3UploadArgs are the arguments of the s3-upload command.
type s3UploadArgs struct {
	// bucket is the bucket that receives the object.
	bucket string
	// endpoint is the S3 endpoint host.
	endpoint string
	// region is the S3 region.
	region string
	// key is the object key.
	key string
	// partMiB is the part size in MiB.
	partMiB int
	// concurrency is the number of parts uploaded at once.
	concurrency int
}

// newDebugS3UploadCommand builds the command that uploads a file or stdin to
// a bucket as a parallel multipart upload.
func newDebugS3UploadCommand() *cli.Command {
	var args s3UploadArgs
	return &cli.Command{
		Name:      "s3-upload",
		Usage:     "upload a file or stdin to an S3 bucket as a parallel multipart upload",
		ArgsUsage: "<file or ->",
		Description: "Streams the file, or stdin for -, to --key in the bucket without knowing its length, sending " +
			"--concurrency parts of --part-mib at once and holding one more part in memory. A failed or interrupted " +
			"upload is aborted, so the bucket keeps no partial object. An upload holds at most 10,000 parts, so the " +
			"default 64 MiB part carries up to 625 GiB. The credentials come from " + packKeyIDEnv + " and " + packKeyEnv +
			" and are never printed.",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "bucket", Usage: "the bucket that receives the object", EnvVars: []string{"HYPERLINE_PROD_BUCKET"}, Destination: &args.bucket},
			&cli.StringFlag{Name: "endpoint", Usage: "the S3 endpoint host", Value: "s3.us-east-005.backblazeb2.com", Destination: &args.endpoint},
			&cli.StringFlag{Name: "region", Usage: "the S3 region", Value: "us-east-005", EnvVars: []string{"HYPERLINE_PROD_BUCKET_REGION"}, Destination: &args.region},
			&cli.StringFlag{Name: "key", Usage: "the object key", Destination: &args.key, Required: true},
			&cli.IntFlag{Name: "part-mib", Usage: "the part size in MiB, from 5 to 5120", Value: block_store_s3.DefaultPartSize >> 20, Destination: &args.partMiB},
			&cli.IntFlag{Name: "concurrency", Usage: "the number of parts uploaded at once", Value: block_store_s3.DefaultUploadConcurrency, Destination: &args.concurrency},
		},
		Action: func(c *cli.Context) error {
			if c.NArg() != 1 {
				return errors.New("expected a file, or - for stdin")
			}
			return uploadToS3(c.Context, c.Args().Get(0), &args, os.Stderr)
		},
	}
}

// uploadToS3 uploads the file at path, or stdin for "-", writing progress
// lines to w.
func uploadToS3(ctx context.Context, path string, args *s3UploadArgs, w io.Writer) error {
	// Check the arguments and the credentials.
	if args.bucket == "" {
		return errors.New("name the bucket with --bucket or HYPERLINE_PROD_BUCKET")
	}
	keyID, key := os.Getenv(packKeyIDEnv), os.Getenv(packKeyEnv)
	if keyID == "" || key == "" {
		return fmt.Errorf("set %s and %s", packKeyIDEnv, packKeyEnv)
	}

	// Open the source.
	src := io.Reader(os.Stdin)
	if path != "-" {
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		src = f
	}

	// Build the client.
	client, err := block_store_s3.BuildClient(&block_store_s3.ClientConfig{
		Endpoint:    args.endpoint,
		Region:      args.region,
		Credentials: &block_store_s3.Credentials{AccessKeyId: keyID, SecretAccessKey: key},
	})
	if err != nil {
		return err
	}

	// Report the stored bytes and the rate until the upload ends.
	var stored atomic.Int64
	start := time.Now()
	done := make(chan struct{})
	reported := make(chan struct{})
	go func() {
		// Print a line each interval until done closes.
		defer close(reported)
		ticker := time.NewTicker(s3UploadReportInterval)
		defer ticker.Stop()
		var last int64
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				n := stored.Load()
				fmt.Fprintln(w, formatS3UploadProgress(time.Since(start), n, n-last, s3UploadReportInterval))
				last = n
			}
		}
	}()

	// Upload the stream and stop the reports.
	opts := &block_store_s3.UploadStreamOptions{
		PartSize:    args.partMiB << 20,
		Concurrency: args.concurrency,
		Progress:    func(n int) { stored.Add(int64(n)) },
	}
	total, err := client.UploadStream(ctx, args.bucket, args.key, src, opts)
	close(done)
	<-reported
	if err != nil {
		return err
	}

	// Print the final line.
	elapsed := time.Since(start)
	fmt.Fprintf(w, "uploaded %s/%s: %s\n", args.bucket, args.key, formatS3UploadProgress(elapsed, total, total, elapsed))
	return nil
}

// formatS3UploadProgress describes n bytes stored after elapsed, with the
// rate of the recent bytes stored over interval.
func formatS3UploadProgress(elapsed time.Duration, n, recent int64, interval time.Duration) string {
	// Compute the average and recent rates in MB/s.
	avg := float64(n) / 1e6 / max(elapsed.Seconds(), 0.001)
	cur := float64(recent) / 1e6 / max(interval.Seconds(), 0.001)

	// Describe them with the stored bytes.
	return fmt.Sprintf("%s: %s stored, %.1f MB/s now, %.1f MB/s average", elapsed.Round(time.Second), humanize.IBytes(uint64(n)), cur, avg) //nolint:gosec // n is a byte count, never negative.
}
