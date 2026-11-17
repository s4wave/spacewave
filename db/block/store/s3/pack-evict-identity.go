//go:build !tinygo

package block_store_s3

import (
	"context"
	"strconv"

	"github.com/pkg/errors"
)

// identityAbsent stands in PackIdentity for an object the bucket lacks.
const identityAbsent = "absent"

// PackIdentity names what the bucket holds for pack id: the size and tag of
// the pack object and of its entry object, each "absent" when the bucket has
// none. It returns "" when the backend reports no sizes or no tags, so a
// caller can keep nothing by it. Two equal answers mean the bucket holds the
// same bytes under both keys, so a pack VerifyPack refused for a reason in
// those bytes is refused again; a changed answer means it may verify now.
// Costs two HEAD requests and no body.
func (s *PackStore) PackIdentity(ctx context.Context, id string) (string, error) {
	// Only a backend that reports tags can name the bytes.
	statter, ok := s.client.(objectStatter)
	if !ok {
		return "", nil
	}

	// Name the pack object, then the entry object.
	var parts [2]string
	for i, key := range []string{s.prefix + packDir + id, s.prefix + entryDir + id} {
		stat, err := statter.HeadObject(ctx, s.bucket, key)
		switch {
		case errors.Is(err, ErrNotFound):
			parts[i] = identityAbsent
		case err != nil:
			return "", errors.Wrapf(err, "stat %s", key)
		case stat.ETag == "":
			return "", nil
		default:
			parts[i] = strconv.FormatInt(stat.Size, 10) + ":" + stat.ETag
		}
	}
	return parts[0] + " " + parts[1], nil
}
