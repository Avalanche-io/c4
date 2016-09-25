package attributes

import (
	"math/big"

	"github.com/etcenter/c4/asset"
)

type Attributes interface {
	ID() *asset.ID
	Bytes() *big.Int
}
