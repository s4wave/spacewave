package s4wave_apt

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
)

// AptPackageChecksums returns checksum records for package payload bytes.
func AptPackageChecksums(data []byte) []*AptPackageChecksum {
	checksums := newAptChecksumSet(data)
	return []*AptPackageChecksum{
		{Algorithm: "md5", Hex: checksums.md5},
		{Algorithm: "sha1", Hex: checksums.sha1},
		{Algorithm: "sha256", Hex: checksums.sha256},
	}
}

type aptChecksumSet struct {
	md5    string
	sha1   string
	sha256 string
}

func newAptChecksumSet(data []byte) aptChecksumSet {
	md5Sum := md5.Sum(data)
	sha1Sum := sha1.Sum(data)
	sha256Sum := sha256.Sum256(data)
	return aptChecksumSet{
		md5:    hex.EncodeToString(md5Sum[:]),
		sha1:   hex.EncodeToString(sha1Sum[:]),
		sha256: hex.EncodeToString(sha256Sum[:]),
	}
}
