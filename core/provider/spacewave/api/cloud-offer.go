package provider_spacewave_api

import (
	_ "embed"
	"fmt"
	"math"
	"slices"

	"github.com/aperturerobotics/fastjson"
)

// cloudOfferJSON is shared with the app and the cloud billing implementation.
//
//go:embed cloud-offer.json
var cloudOfferJSON []byte

var currentCloudOffer = readCloudOffer()

// CloudOffer defines the monthly service and its metered storage price.
//
// The monthly price includes StorageBytes of resident storage. Storage above it
// is metered in GiB-hours and charged at StorageMicrodollarsPerGibMonth, up to
// the spending limit the payer selects from OverageLimitsCents. Reads and
// writes are counted but not billed.
type CloudOffer struct {
	Version                        string   `json:"version"`
	PolicyVersion                  string   `json:"policyVersion"`
	Currency                       string   `json:"currency"`
	MonthlyPriceCents              uint32   `json:"monthlyPriceCents"`
	StorageBytes                   uint64   `json:"storageBytes"`
	StorageMicrodollarsPerGibMonth uint32   `json:"storageMicrodollarsPerGibMonth"`
	OverageLimitsCents             []uint32 `json:"overageLimitsCents"`
	DefaultOverageLimitCents       uint32   `json:"defaultOverageLimitCents"`
}

// CurrentCloudOffer returns the service contract bundled with this revision.
func CurrentCloudOffer() CloudOffer {
	offer := currentCloudOffer
	offer.OverageLimitsCents = slices.Clone(offer.OverageLimitsCents)
	return offer
}

// readCloudOffer parses the immutable build asset without reflection.
func readCloudOffer() CloudOffer {
	// Parse the embedded offer.
	value, err := fastjson.ParseBytes(cloudOfferJSON)
	if err != nil {
		panic(err)
	}

	// Read the scalar terms.
	offer := CloudOffer{
		Version:                        string(value.GetStringBytes("version")),
		PolicyVersion:                  string(value.GetStringBytes("policyVersion")),
		Currency:                       string(value.GetStringBytes("currency")),
		MonthlyPriceCents:              cloudOfferUint32(value.GetUint64("monthlyPriceCents"), "monthlyPriceCents"),
		StorageBytes:                   value.GetUint64("storageBytes"),
		StorageMicrodollarsPerGibMonth: cloudOfferUint32(value.GetUint64("storageMicrodollarsPerGibMonth"), "storageMicrodollarsPerGibMonth"),
		DefaultOverageLimitCents:       cloudOfferUint32(value.GetUint64("defaultOverageLimitCents"), "defaultOverageLimitCents"),
	}

	// Read the spending limits, which must offer the default.
	limits := value.GetArray("overageLimitsCents")
	offer.OverageLimitsCents = make([]uint32, len(limits))
	for idx, limit := range limits {
		offer.OverageLimitsCents[idx] = cloudOfferUint32(limit.GetUint64(), "overageLimitsCents")
	}
	if !slices.Contains(offer.OverageLimitsCents, offer.DefaultOverageLimitCents) {
		panic("cloud offer default spending limit is not one of its limits")
	}
	return offer
}

// cloudOfferUint32 narrows an offer field, panicking on overflow.
func cloudOfferUint32(value uint64, field string) uint32 {
	if value > math.MaxUint32 {
		panic(fmt.Sprintf("cloud offer field %s exceeds uint32 range: %d", field, value))
	}
	return uint32(value) //nolint:gosec // the explicit MaxUint32 check protects the fixed-width offer contract.
}
