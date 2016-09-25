package asset

import (
	"encoding/json"
	"math/big"
)

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
	id2, err := ParseBytesID(data[1 : len(data)-1])
	*id = *id2
	return err
}

// MarshalJSON adds output support for package encoding/json.
func (ids IDSlice) MarshalJSON() ([]byte, error) {
	ids2 := ([]*ID)(ids)
	return json.Marshal(ids2)
}

// MarshalJSON adds parsing support for package encoding/json.
func (ids *IDSlice) UnmarshalJSON(data []byte) error {
	var ids_in []*ID
	err := json.Unmarshal(data, &ids_in)
	if err != nil {
		return err
	}
	copy(*ids, ids_in)
	return nil
}
