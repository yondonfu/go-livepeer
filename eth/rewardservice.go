package eth

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum"
	// "github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/core/types"
	// lpTypes "github.com/livepeer/go-livepeer/eth/types"
)

var (
	ErrRewardServiceStarted = fmt.Errorf("reward service already started")
	ErrRewardServiceStopped = fmt.Errorf("reward service already stopped")
	ErrInactiveTranscoder   = fmt.Errorf("transcoder is not active")
)

type RewardService struct {
	eventMonitor EventMonitor
	client       LivepeerEthClient
	sub          ethereum.Subscription
	logsCh       chan types.Log
}

func (s *RewardService) Start(ctx context.Context) error {
	if s.sub != nil {
		return ErrRewardServiceStarted
	}

	logsCh := make(chan types.Log)
	sub, err := s.eventMonitor.SubscribeNewRound(ctx, logsCh, func(cctx context.Context, l types.Log) error {
		// var newRound lpTypes.NewRound
		// err := abi.Unpack(&newRound, "NewRound", l)
		// if err != nil {
		// 	return err
		// }

		// return s.tryReward(cctx, newRound.Round)
		return nil
	})

	if err != nil {
		return err
	}

	s.logsCh = logsCh
	s.sub = sub

	return nil
}

func (s *RewardService) Stop() error {
	if s.sub == nil {
		return ErrRewardServiceStopped
	}

	close(s.logsCh)
	s.sub.Unsubscribe()

	s.logsCh = nil
	s.sub = nil

	return nil
}

func (s *RewardService) tryReward(ctx context.Context, round *big.Int) error {
	bondingManager := s.client.BondingManager()

	isActive, err := bondingManager.IsActiveTranscoder(s.client.Account().Address, round)
	if err != nil {
		return err
	}

	if !isActive {
		return ErrInactiveTranscoder
	}

	tx, err := bondingManager.Reward()
	if err != nil {
		return err
	}

	return s.client.CheckTx(ctx, tx)
}
