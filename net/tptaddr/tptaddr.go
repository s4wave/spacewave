package tptaddr

import "strings"

// TptAddrDelimiter is the delimiter for tpt addr sections.
const TptAddrDelimiter = '|'

// ParseTptAddr parses a transport address string.
func ParseTptAddr(tptAddr string) (transportID, addr string, err error) {
	var found bool
	transportID, addr, found = strings.Cut(tptAddr, string([]rune{TptAddrDelimiter}))
	if !found || len(transportID) == 0 || len(addr) == 0 {
		return "", "", ErrInvalidTptAddr
	}
	return
}
