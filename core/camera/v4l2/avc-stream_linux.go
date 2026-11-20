//go:build linux

package v4l2

import (
	"bytes"
	"encoding/binary"
)

// avcStream converts one captured Annex B access unit per V4L2 buffer to AVC.
type avcStream struct {
	// sequence tracks the next buffer expected while ready.
	sequence uint32
	// ready means a decodable IDR has opened the group.
	ready bool
	// sps retains sequence parameter sets by the IDs used in picture sets.
	sps [32][]byte
	// pps retains picture parameter sets by the IDs used in slices.
	pps [256][]byte
	// activeSPS identifies which SPS the open group was configured to use.
	activeSPS uint32
	// activePPS identifies which PPS the open group was configured to use.
	activePPS uint32
}

// accessUnit copies one complete Annex B picture into length-prefixed form.
// A missing, damaged or unconfigured picture ends the decodable prefix.
func (s *avcStream) accessUnit(data []byte, sequence uint32, damaged bool) (payload, description []byte) {
	// A driver gap breaks the chain of references within a group.
	if damaged || sequence != s.sequence {
		s.ready = false
	}
	s.sequence = sequence + 1
	if damaged {
		return nil, nil
	}

	// Copy the access unit once. Parameter sets belong in avcC and can
	// arrive in a separate buffer or with the IDR.
	startCode := []byte{0, 0, 1}
	_, rest, found := bytes.Cut(data, startCode)
	if !found {
		s.ready = false
		return nil, nil
	}
	var picture bool
	for found {
		// The three-byte boundary also matches four-byte codes. The extra
		// zero and any trailing_zero_8bits do not belong in the AVC sample.
		var nal []byte
		nal, rest, found = bytes.Cut(rest, startCode)
		nal = bytes.TrimRight(nal, "\x00")
		if len(nal) == 0 {
			continue
		}

		// The first VCL NAL selects the parameter sets for this picture.
		// A picture headed by a delta can only extend the active group.
		switch typ := nal[0] & 0x1f; typ {
		case 7, 8:
			s.saveParameterSet(nal, typ)
			continue
		case 5, 1, 2:
			if !picture {
				desc, ok := s.startPicture(nal, typ == 5)
				if !ok {
					s.ready = false
					return nil, nil
				}
				description = desc
				picture = true
			}
		}

		// Include slices and supporting NALs in their original order.
		if payload == nil {
			payload = make([]byte, 0, len(data)+4)
		}
		payload = binary.BigEndian.AppendUint32(payload, uint32(len(nal))) //nolint:gosec // V4L2 bounds the full access unit by a uint32 byte count.
		payload = append(payload, nal...)
	}
	if !picture {
		return nil, nil
	}
	return payload, description
}

// saveParameterSet copies a parameter set before the driver reuses the buffer.
func (s *avcStream) saveParameterSet(nal []byte, typ byte) {
	// A configuration record uses a 16-bit length for each parameter set.
	if len(nal) > 0xffff {
		return
	}

	// Extract the parameter set ID from the beginning of the RBSP.
	offset := 1
	if typ == 7 {
		offset = 4
	}
	if len(nal) <= offset {
		return
	}
	bits := rbsp{data: nal[offset:]}
	id := bits.readUE()
	if bits.invalid {
		return
	}

	// Replacing an active set invalidates deltas until the next IDR.
	switch {
	case typ == 7 && id < uint32(len(s.sps)):
		if !bytes.Equal(s.sps[id], nal) {
			s.sps[id] = bytes.Clone(nal)
			s.ready = s.ready && id != s.activeSPS
		}
	case typ == 8 && id < uint32(len(s.pps)):
		if !bytes.Equal(s.pps[id], nal) {
			s.pps[id] = bytes.Clone(nal)
			s.ready = s.ready && id != s.activePPS
		}
	}
}

// startPicture validates the slice's decoder state and describes each IDR.
func (s *avcStream) startPicture(nal []byte, idr bool) ([]byte, bool) {
	// The slice header starts with first_mb_in_slice, slice_type and PPS ID.
	bits := rbsp{data: nal[1:]}
	bits.readUE()
	bits.readUE()
	ppsID := bits.readUE()
	if bits.invalid || ppsID >= uint32(len(s.pps)) {
		return nil, false
	}
	if !idr {
		return nil, s.ready && s.activePPS == ppsID
	}

	// The referred PPS names the SPS to put in the decoder configuration.
	pps := s.pps[ppsID]
	if len(pps) < 2 {
		return nil, false
	}
	bits = rbsp{data: pps[1:]}
	bits.readUE()
	spsID := bits.readUE()
	if bits.invalid || spsID >= uint32(len(s.sps)) || len(s.sps[spsID]) == 0 {
		return nil, false
	}
	desc := describeAVC(s.sps[spsID], pps)
	if desc == nil {
		return nil, false
	}

	// Start a fresh group using these immutable parameter sets.
	s.ready = true
	s.activeSPS, s.activePPS = spsID, ppsID
	return desc, true
}

// describeAVC builds an AVCDecoderConfigurationRecord with four-byte lengths.
func describeAVC(sps, pps []byte) []byte {
	// The profile, constraint flags and level follow the SPS NAL header.
	desc := make([]byte, 0, 15+len(sps)+len(pps))
	desc = append(desc, 1, sps[1], sps[2], sps[3], 0xff, 0xe1)
	desc = binary.BigEndian.AppendUint16(desc, uint16(len(sps))) //nolint:gosec // The parameter-set cache enforces the avcC length limit.
	desc = append(desc, sps...)
	desc = append(desc, 1)
	desc = binary.BigEndian.AppendUint16(desc, uint16(len(pps))) //nolint:gosec // The parameter-set cache enforces the avcC length limit.
	desc = append(desc, pps...)

	// Profiles with extended SPS syntax also describe pixel layout in avcC.
	switch sps[1] {
	case 44, 83, 86, 100, 110, 118, 122, 128, 134, 135, 138, 139, 144, 244:
		bits := rbsp{data: sps[4:]}
		bits.readUE()
		chroma := bits.readUE()
		if chroma == 3 {
			bits.readBit()
		}
		lumaDepth, chromaDepth := bits.readUE(), bits.readUE()
		if bits.invalid || chroma > 3 || lumaDepth > 6 || chromaDepth > 6 {
			return nil
		}
		desc = append(desc, 0xfc|byte(chroma), 0xf8|byte(lumaDepth), 0xf8|byte(chromaDepth), 0)
	}
	return desc
}
