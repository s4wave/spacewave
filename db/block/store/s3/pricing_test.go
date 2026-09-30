package block_store_s3

import (
	"math"
	"testing"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/packfile"
)

// TestPricingForEndpoint maps endpoints to the price lists of their services.
func TestPricingForEndpoint(t *testing.T) {
	cases := []struct {
		endpoint string
		want     *Pricing
	}{
		{"s3.us-east-1.amazonaws.com", awsPricing},
		{"https://acct.r2.cloudflarestorage.com", r2Pricing},
		{"s3.us-west-004.backblazeb2.com", b2Pricing},
		{"s3.wasabisys.com", wasabiPricing},
		{"127.0.0.1:9000", selfHostedPricing},
		{"http://192.168.1.10:9000", selfHostedPricing},
		{"minio:9000", selfHostedPricing},
		{"localhost:9000", selfHostedPricing},
		{"storage.example.com", awsPricing},
		{"8.8.8.8", awsPricing},
	}
	for _, c := range cases {
		if got := PricingForEndpoint(c.endpoint); got != c.want {
			t.Errorf("PricingForEndpoint(%q) = %+v, want %+v", c.endpoint, got, c.want)
		}
	}
}

// TestWorthReclaim reclaims a packfile only when the storage its dead blocks
// take for reclaimPayback costs more than reclaiming it.
func TestWorthReclaim(t *testing.T) {
	// Describe packfiles by size, dead fraction, and age.
	now := time.Now()
	day := 24 * time.Hour
	pack := func(size uint64, deadPct uint64, age time.Duration) *packJudgment {
		return &packJudgment{
			entry: &packfile.PackfileEntry{SizeBytes: size, CreatedAt: timestamppb.New(now.Add(-age))},
			total: size,
			dead:  size * deadPct / 100,
		}
	}

	// Judge each on each service.
	cases := []struct {
		name    string
		pricing *Pricing
		pack    *packJudgment
		want    bool
	}{
		{"s3 rewrites a 16 MiB pack 60% dead", awsPricing, pack(16<<20, 60, day), true},
		{"s3 keeps a 16 MiB pack 40% dead", awsPricing, pack(16<<20, 40, day), false},
		{"s3 keeps a 1 KiB dead pack", awsPricing, pack(1<<10, 100, day), false},
		{"s3 deletes a 1 MiB dead pack", awsPricing, pack(1<<20, 100, day), true},
		{"r2 rewrites a 192 KiB pack 60% dead", r2Pricing, pack(192<<10, 60, day), true},
		{"s3 keeps a 192 KiB pack 60% dead", awsPricing, pack(192<<10, 60, day), false},
		{"wasabi keeps a young dead pack", wasabiPricing, pack(16<<20, 100, 30*day), false},
		{"wasabi deletes an old dead pack", wasabiPricing, pack(16<<20, 100, 91*day), true},
		{"self-hosted deletes a 1 KiB dead pack", selfHostedPricing, pack(1<<10, 100, 0), true},
		{"self-hosted keeps a live pack", selfHostedPricing, pack(1<<10, 0, day), false},
	}
	for _, c := range cases {
		s := &PackStore{pricing: c.pricing}
		if got := s.worthReclaim(c.pack, now); got != c.want {
			t.Errorf("%s: worthReclaim = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestPricingValidate accepts free and priced lists and rejects negative or
// unbounded prices.
func TestPricingValidate(t *testing.T) {
	cases := []struct {
		name    string
		pricing *Pricing
		valid   bool
	}{
		{"unset", nil, true},
		{"free same-region egress", &Pricing{StorageGbMonth: 0.023, ClassAPerMillion: 5, ClassBPerMillion: 0.4}, true},
		{"negative storage", &Pricing{StorageGbMonth: -1}, false},
		{"infinite egress", &Pricing{EgressGb: math.Inf(1)}, false},
		{"not a number", &Pricing{ClassBPerMillion: math.NaN()}, false},
	}
	for _, c := range cases {
		if err := c.pricing.Validate(); (err == nil) != c.valid {
			t.Errorf("%s: Validate = %v, want valid %v", c.name, err, c.valid)
		}
	}
}

// TestReclaimDueAt schedules the next pass when the storage of the blocks
// expected to die reaches the cost of a pass, no later than
// reclaimMaxInterval.
func TestReclaimDueAt(t *testing.T) {
	// Describe a Space of 128 packfiles holding 4 KiB blocks.
	passedAt := time.Now().Truncate(time.Second)
	state := func(perDay uint64) *ReclaimState {
		return &ReclaimState{
			PassedAt:        timestamppb.New(passedAt),
			DeadBytesPerDay: perDay,
			Packs:           128,
			Blocks:          128 * 4096,
		}
	}

	// Check the due time sits where the gate opens.
	s := &PackStore{pricing: awsPricing}
	prev := state(100 << 20)
	due := s.reclaimDueAt(prev)
	if due.Sub(passedAt) < 8*24*time.Hour || due.Sub(passedAt) > 10*24*time.Hour {
		t.Fatalf("pass due %v after the last, want about 8.8 days", due.Sub(passedAt))
	}
	if s.reclaimDue(prev, due.Add(-time.Minute)) || !s.reclaimDue(prev, due.Add(time.Minute)) {
		t.Fatal("reclaimDue disagrees with reclaimDueAt")
	}

	// Check a slow or zero death rate waits reclaimMaxInterval.
	for _, perDay := range []uint64{0, 1} {
		if due := s.reclaimDueAt(state(perDay)); !due.Equal(passedAt.Add(reclaimMaxInterval)) {
			t.Errorf("%d bytes a day: pass due %v after the last", perDay, due.Sub(passedAt))
		}
	}
}
