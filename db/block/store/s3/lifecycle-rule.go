//go:build !tinygo

package block_store_s3

import (
	"strings"

	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
)

// LifecycleRule is one of a bucket's lifecycle rules, as far as a hide cares:
// the keys it reaches and the fields that set it to hide or delete versions.
type LifecycleRule struct {
	// FileNamePrefix is the prefix of the keys the rule reaches; "" reaches
	// every key.
	FileNamePrefix string
	// Expiring names the rule's fields that are set and hide or delete
	// versions: daysFromUploadingToHiding, daysFromHidingToDeleting, and any
	// field this code does not know, which it counts as one.
	Expiring []string
}

// Reaches reports whether the rule hides or deletes versions of some key
// under prefix: it has an expiring field, and its prefix and prefix are one
// within the other.
func (r LifecycleRule) Reaches(prefix string) bool {
	if len(r.Expiring) == 0 {
		return false
	}
	return strings.HasPrefix(prefix, r.FileNamePrefix) || strings.HasPrefix(r.FileNamePrefix, prefix)
}

// harmlessRuleFields are the rule fields that touch no finished version.
var harmlessRuleFields = map[string]bool{
	"fileNamePrefix": true,
	"daysFromStartingToCancelingUnfinishedLargeFiles": true,
}

// parseLifecycleRules parses a bucket's lifecycleRules value, which must be
// a list, perhaps empty, of objects that each name a fileNamePrefix.
func parseLifecycleRules(value *fastjson.Value) ([]LifecycleRule, error) {
	// Refuse a missing value or one that is not a list.
	if value == nil {
		return nil, errors.New("b2_list_buckets: the bucket has no lifecycleRules")
	}
	items, err := value.Array()
	if err != nil {
		return nil, errors.Wrap(err, "b2_list_buckets: lifecycleRules")
	}

	// Name each rule's prefix and its set fields that may touch a version.
	rules := make([]LifecycleRule, 0, len(items))
	for i, item := range items {
		fields, err := item.Object()
		if err != nil {
			return nil, errors.Wrapf(err, "b2_list_buckets: lifecycle rule %d", i)
		}
		prefixValue := fields.Get("fileNamePrefix")
		if prefixValue == nil || prefixValue.Type() != fastjson.TypeString {
			return nil, errors.Errorf("b2_list_buckets: lifecycle rule %d has no fileNamePrefix", i)
		}
		prefix, _ := prefixValue.StringBytes()
		rule := LifecycleRule{FileNamePrefix: string(prefix)}
		fields.Visit(func(key []byte, v *fastjson.Value) {
			if !harmlessRuleFields[string(key)] && v.Type() != fastjson.TypeNull {
				rule.Expiring = append(rule.Expiring, string(key))
			}
		})
		rules = append(rules, rule)
	}
	return rules, nil
}
