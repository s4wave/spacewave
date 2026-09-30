package block_store_s3

import (
	"math"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// gib is the number of bytes in the gigabyte the services bill by.
const gib = 1 << 30

// month is the billing month.
const month = 30 * 24 * time.Hour

// The price lists of known services, published as of 2026-09-30. Deletes are
// free on each.
var (
	// awsPricing is Amazon S3 Standard in us-east-1 with internet egress.
	awsPricing = &Pricing{
		StorageGbMonth:   0.023,
		EgressGb:         0.09,
		ClassAPerMillion: 5,
		ClassBPerMillion: 0.4,
	}
	// r2Pricing is Cloudflare R2 Standard, which has no egress fees.
	r2Pricing = &Pricing{
		StorageGbMonth:   0.015,
		ClassAPerMillion: 4.50,
		ClassBPerMillion: 0.36,
	}
	// b2Pricing is Backblaze B2, whose requests and egress up to three times
	// the stored bytes are free.
	b2Pricing = &Pricing{
		StorageGbMonth: 0.00695,
	}
	// wasabiPricing is Wasabi, which bills an object deleted within 90 days
	// for the rest of them.
	wasabiPricing = &Pricing{
		StorageGbMonth: 0.00799,
		MinStorageDays: 90,
	}
	// selfHostedPricing is a server on a local network, such as MinIO, where
	// requests and transfer are free and disk is cheap.
	selfHostedPricing = &Pricing{
		StorageGbMonth: 0.01,
	}
)

// PricingForEndpoint returns the price list of the service at endpoint. An
// unknown public service is priced as Amazon S3, whose requests and egress
// cost the most, so storage reclaim does not spend more than it frees. The
// result is shared and must not be modified.
func PricingForEndpoint(endpoint string) *Pricing {
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

// Validate checks that every price is a finite number that is not negative.
func (p *Pricing) Validate() error {
	prices := []struct {
		name  string
		value float64
	}{
		{"storage", p.GetStorageGbMonth()},
		{"egress", p.GetEgressGb()},
		{"class A", p.GetClassAPerMillion()},
		{"class B", p.GetClassBPerMillion()},
	}
	for _, price := range prices {
		if math.IsNaN(price.value) || math.IsInf(price.value, 0) || price.value < 0 {
			return errors.Errorf("%s price must be a finite number of at least zero", price.name)
		}
	}
	return nil
}

// minStorage is the storage duration billed for an object deleted sooner.
func (p *Pricing) minStorage() time.Duration {
	return time.Duration(p.GetMinStorageDays()) * 24 * time.Hour
}

// storageCost is the price of storing size bytes for d.
func (p *Pricing) storageCost(size float64, d time.Duration) float64 {
	return size * p.GetStorageGbMonth() / gib * float64(d) / float64(month)
}

// indexEntryBytes estimates the bytes of one entry of a packfile key index.
const indexEntryBytes = 64

// scanCost is the price of judging packs packfiles holding blocks blocks:
// listing their entries and reading each key index with two ranged reads.
func (p *Pricing) scanCost(packs, blocks uint64) float64 {
	lists := (packs + 999) / 1000
	listCost := float64(lists) * p.GetClassAPerMillion() / 1e6
	readCost := float64(2*packs) * p.GetClassBPerMillion() / 1e6
	return listCost + readCost + p.egressCost(blocks*indexEntryBytes)
}

// reclaimCost is the price of reclaiming a packfile of size bytes. Deleting
// the packfile and its entry lists the versions of each. A rewrite also reads
// the packfile whole and writes the new packfile and its entry.
func (p *Pricing) reclaimCost(size uint64, rewrite bool) float64 {
	classA := p.GetClassAPerMillion() / 1e6
	cost := 2 * classA
	if rewrite {
		cost += p.GetClassBPerMillion()/1e6 + p.egressCost(size) + 2*classA
	}
	return cost
}

// egressCost is the price of reading size bytes out of the service.
func (p *Pricing) egressCost(size uint64) float64 {
	return float64(size) * p.GetEgressGb() / gib
}
