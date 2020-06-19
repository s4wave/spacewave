package auth_triplesec

/*
	rs := rand.New(rand.NewSource(
		int64(crc64.Checksum(
			[]byte(username),
			crc64.MakeTable(crc64.ECMA),
		)),
	))
	salt := make([]byte, triplesec.SaltLen)
	_, err := rs.Read(salt)
	if err != nil {
		return nil, err
	}
*/
