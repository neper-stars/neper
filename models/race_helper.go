package models

import "encoding/base64"

func (m *Race) RawData() ([]byte, error) {
	dst := make([]byte, base64.StdEncoding.DecodedLen(len(m.Data)))
	n, err := base64.StdEncoding.Decode(dst, []byte(m.Data))
	if err != nil {
		return nil, err
	}
	dst = dst[:n]
	return dst, nil
}

func RaceFromRaw(raw []byte) *Race {
	dst := make([]byte, base64.StdEncoding.EncodedLen(len(raw)))
	base64.StdEncoding.Encode(dst, raw)
	return &Race{
		Data: string(dst),
	}
}
