//go:build !tinygo

package block_store_s3

import (
	"net"
	"net/url"
	"strings"
	"time"
)

// gib is the number of bytes in the gigabyte the services bill by.
const gib = 1 << 30

// month is the billing month.
const month = 30 * 24 * time.Hour

// Pricing is the price list of an S3-compatible service in dollars. Storage
// reclaim weighs the storage a pass frees against the requests and egress it
// costs.
type Pricing struct {
	// StorageByteMonth is the price of storing one byte for one month.
	StorageByteMonth float64
	// EgressByte is the price of reading one byte out of the service.
	EgressByte float64
	// ClassA is the price of one PUT or LIST request.
	ClassA float64
	// ClassB is the price of one GET request.
	ClassB float64
	// MinStorage is the storage duration billed for an object deleted sooner.
	MinStorage time.Duration
}

// The price lists of known services, published as of 2026-09-30. Deletes are
// free on each.
var (
	// awsPricing is Amazon S3 Standard in us-east-1 with internet egress.
	awsPricing = Pricing{
		StorageByteMonth: 0.023 / gib,
		EgressByte:       0.09 / gib,
		ClassA:           0.005 / 1000,
		ClassB:           0.0004 / 1000,
	}
	// r2Pricing is Cloudflare R2 Standard, which has no egress fees.
	r2Pricing = Pricing{
		StorageByteMonth: 0.015 / gib,
		ClassA:           4.50 / 1e6,
		ClassB:           0.36 / 1e6,
	}
	// b2Pricing is Backblaze B2, whose requests and egress up to three times
	// the stored bytes are free.
	b2Pricing = Pricing{
		StorageByteMonth: 6.95 / 1000 / gib,
	}
	// wasabiPricing is Wasabi, which bills an object deleted within 90 days
	// for the rest of them.
	wasabiPricing = Pricing{
		StorageByteMonth: 7.99 / 1000 / gib,
		MinStorage:       90 * 24 * time.Hour,
	}
	// selfHostedPricing is a server on a local network, such as MinIO, where
	// requests and transfer are free and disk is cheap.
	selfHostedPricing = Pricing{
		StorageByteMonth: 0.01 / gib,
	}
)

// PricingForEndpoint returns the price list of the service at endpoint. An
// unknown public service is priced as Amazon S3, whose requests and egress
// cost the most, so storage reclaim does not spend more than it frees.
func PricingForEndpoint(endpoint string) Pricing {
	// Parse the host, with or without a scheme.
	if !strings.Contains(endpoint, "://") {
		endpoint = "//" + endpoint
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return awsPricing
	}
	host := strings.ToLower(u.Hostname())

	// Match the known services by domain.
	switch {
	case strings.HasSuffix(host, ".r2.cloudflarestorage.com"):
		return r2Pricing
	case strings.HasSuffix(host, ".backblazeb2.com"):
		return b2Pricing
	case strings.HasSuffix(host, ".wasabisys.com"):
		return wasabiPricing
	case strings.HasSuffix(host, ".amazonaws.com"):
		return awsPricing
	}

	// Treat a local or private host as self-hosted.
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
			return selfHostedPricing
		}
		return awsPricing
	}
	if host == "localhost" || !strings.Contains(host, ".") {
		return selfHostedPricing
	}
	return awsPricing
}

// storageCost is the price of storing size bytes for d.
func (p *Pricing) storageCost(size float64, d time.Duration) float64 {
	return size * p.StorageByteMonth * float64(d) / float64(month)
}

// indexEntryBytes estimates the bytes of one entry of a packfile key index.
const indexEntryBytes = 64

// scanCost is the price of judging packs packfiles holding blocks blocks:
// listing their entries and reading each key index with two ranged reads.
func (p *Pricing) scanCost(packs, blocks uint64) float64 {
	lists := (packs + 999) / 1000
	return float64(lists)*p.ClassA + float64(2*packs)*p.ClassB + float64(blocks*indexEntryBytes)*p.EgressByte
}

// reclaimCost is the price of reclaiming a packfile of size bytes. Deleting
// the packfile and its entry lists the versions of each. A rewrite also reads
// the packfile whole and writes the new packfile and its entry.
func (p *Pricing) reclaimCost(size uint64, rewrite bool) float64 {
	cost := 2 * p.ClassA
	if rewrite {
		cost += p.ClassB + float64(size)*p.EgressByte + 2*p.ClassA
	}
	return cost
}
