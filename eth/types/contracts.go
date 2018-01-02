package types

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

type NewJob struct {
	BroadcasterAddr    common.Address
	JobID              *big.Int
	StreamID           string
	TranscodingOptions string
	MaxPricePerSegment *big.Int
	CreationBlock      *big.Int
}

type NewRound struct {
	Round *big.Int
}
