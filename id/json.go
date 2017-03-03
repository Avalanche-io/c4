package id

import "math/big"

// MarshalJSON adds output support for package encoding/json.
func (id *ID) MarshalJSON() ([]byte, error) {
	bigID := big.Int(*id)
	if bigID.Cmp(big.NewInt(0)) == 0 {
		return []byte(`""`), nil
	}
	return []byte(`"` + id.String() + `"`), nil
}

// MarshalJSON adds parsing support for package encoding/json.
func (id *ID) UnmarshalJSON(data []byte) error {
	// UnmarshalJSON includes quotes in the data so we remove them
	id2, err := Parse(string(data[1 : len(data)-1]))
	*id = *id2
	return err
}
