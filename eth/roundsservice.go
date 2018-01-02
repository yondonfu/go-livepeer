package eth

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
)

var (
	ErrRoundsServiceStarted = fmt.Errorf("rounds service already started")
	ErrRoundsServiceStopped = fmt.Errorf("rounds service already stopped")
)

type RoundsService struct {
	eventMonitor EventMonitor
	client       LivepeerEthClient
	sub          ethereum.Subscription
	headersCh    chan *types.Header
}

func NewRoundsService(eventMonitor EventMonitor, client LivepeerEthClient) *RoundsService {
	return &RoundsService{
		eventMonitor: eventMonitor,
		client:       client,
	}
}

func (s *RoundsService) Start(ctx context.Context) error {
	if s.sub != nil {
		return ErrRoundsServiceStarted
	}

	headersCh := make(chan *types.Header)
	sub, err := s.eventMonitor.SubscribeNewBlocks(ctx, headersCh, func(cctx context.Context, h types.Header) error {
		return s.tryInitializeRound(cctx)
	})

	if err != nil {
		return err
	}

	s.headersCh = headersCh
	s.sub = sub

	return nil
}

func (s *RoundsService) Stop() error {
	if s.sub == nil {
		return ErrRoundsServiceStopped
	}

	close(s.headersCh)
	s.sub.Unsubscribe()

	s.headersCh = nil
	s.sub = nil

	return nil
}

func (s *RoundsService) tryInitializeRound(ctx context.Context) error {
	roundsManager := s.client.RoundsManager()

	currentRound, err := roundsManager.CurrentRound()
	if err != nil {
		return err
	}

	lastInitializedRound, err := roundsManager.LastInitializedRound()
	if err != nil {
		return err
	}

	if lastInitializedRound.Cmp(currentRound) == -1 {
		tx, err := roundsManager.InitializeRound()
		if err != nil {
			return err
		}

		return s.client.CheckTx(ctx, tx)
	} else {
		return nil
	}
}
