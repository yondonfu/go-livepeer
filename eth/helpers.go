package eth

import (
	"context"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

func WaitBlocks(ctx context.Context, backend *ethclient.Client, rpcTimeout time.Duration, blocks *big.Int) error {
	block, err := backend.BlockByNumber(ctx, nil)
	if err != nil {
		return err
	}

	targetBlockNum := new(big.Int).Add(block.Number(), blocks)

	for block.Number().Cmp(targetBlockNum) == -1 {
		block, err = backend.BlockByNumber(ctx, nil)
		if err != nil {
			return err
		}
	}

	return nil
}

func IsNullAddress(addr common.Address) bool {
	return addr == common.Address{}
}
