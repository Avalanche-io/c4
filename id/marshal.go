package id

import "math/big"

// GobEncode implements the gob.GobEncoder interface.
func (id *ID) GobEncode() ([]byte, error) {
	return (*big.Int)(id).GobEncode()
}

// GobEncode implements the gob.GobDecoder interface.
func (id *ID) GobDecode(buf []byte) error {
	return (*big.Int)(id).GobDecode(buf)
}

// MarshalJSON implements the json.MarshalJSON interface.
func (id *ID) MarshalJSON() ([]byte, error) {
	bigID := big.Int(*id)
	if bigID.Cmp(big.NewInt(0)) == 0 {
		return []byte(`""`), nil
	}
	return []byte(`"` + id.String() + `"`), nil
}

// UnmarshalJSON implements the json.UnmarshalJSON interface.
func (id *ID) UnmarshalJSON(data []byte) error {
	// UnmarshalJSON includes quotes in the data so we remove them
	id2, err := Parse(string(data[1 : len(data)-1]))
	*id = *id2
	return err
}
