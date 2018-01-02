package eth

import (
	"context"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/golang/glog"
	"github.com/livepeer/go-livepeer/eth/contracts"
)

var (
	ErrMissingContractAddress = fmt.Errorf("missing contract address")
)

type logCallback func(context.Context, types.Log) error
type headerCallback func(context.Context, types.Header) error

type EventMonitor interface {
	SubscribeNewJob(context.Context, chan types.Log, common.Address, logCallback) (ethereum.Subscription, error)
	SubscribeNewRound(context.Context, chan types.Log, logCallback) (ethereum.Subscription, error)
	SubscribeNewBlocks(context.Context, chan *types.Header, headerCallback) (ethereum.Subscription, error)
}

type eventMonitor struct {
	backend         *ethclient.Client
	contractAddrMap map[string]common.Address
}

func NewEventMonitor(backend *ethclient.Client, contractAddrMap map[string]common.Address) EventMonitor {
	return &eventMonitor{
		backend:         backend,
		contractAddrMap: contractAddrMap,
	}
}

func (em *eventMonitor) SubscribeNewJob(ctx context.Context, logsCh chan types.Log, broadcasterAddr common.Address, cb logCallback) (ethereum.Subscription, error) {
	abiJSON, err := abi.JSON(strings.NewReader(contracts.JobsManagerABI))
	if err != nil {
		return nil, err
	}

	var contractAddr common.Address
	var ok bool
	if contractAddr, ok = em.contractAddrMap["JobsManager"]; !ok {
		return nil, ErrMissingContractAddress
	}

	var q ethereum.FilterQuery
	if !IsNullAddress(broadcasterAddr) {
		q = ethereum.FilterQuery{
			Addresses: []common.Address{contractAddr},
			Topics:    [][]common.Hash{[]common.Hash{abiJSON.Events["NewJob"].Id()}, []common.Hash{common.BytesToHash(common.LeftPadBytes(broadcasterAddr[:], 32))}},
		}
	} else {
		q = ethereum.FilterQuery{
			Addresses: []common.Address{contractAddr},
			Topics:    [][]common.Hash{[]common.Hash{abiJSON.Events["NewJob"].Id()}},
		}
	}

	sub, err := em.backend.SubscribeFilterLogs(ctx, q, logsCh)
	if err != nil {
		return nil, err
	}

	go watchLogs(ctx, sub, logsCh, cb)

	return sub, nil
}

func (em *eventMonitor) SubscribeNewRound(ctx context.Context, logsCh chan types.Log, cb logCallback) (ethereum.Subscription, error) {
	abiJSON, err := abi.JSON(strings.NewReader(contracts.RoundsManagerABI))
	if err != nil {
		return nil, err
	}

	var contractAddr common.Address
	var ok bool
	if contractAddr, ok = em.contractAddrMap["RoundsManager"]; !ok {
		return nil, ErrMissingContractAddress
	}

	q := ethereum.FilterQuery{
		Addresses: []common.Address{contractAddr},
		Topics:    [][]common.Hash{[]common.Hash{abiJSON.Events["NewRound"].Id()}},
	}

	sub, err := em.backend.SubscribeFilterLogs(ctx, q, logsCh)
	if err != nil {
		return nil, err
	}

	go watchLogs(ctx, sub, logsCh, cb)

	return sub, nil
}

func (em *eventMonitor) SubscribeNewBlocks(ctx context.Context, headersCh chan *types.Header, cb headerCallback) (ethereum.Subscription, error) {
	sub, err := em.backend.SubscribeNewHead(ctx, headersCh)
	if err != nil {
		return nil, err
	}

	go watchBlocks(ctx, sub, headersCh, cb)

	return sub, nil
}

func watchLogs(ctx context.Context, sub ethereum.Subscription, logsCh chan types.Log, cb logCallback) {
	for {
		select {
		case l, ok := <-logsCh:
			if !ok {
				glog.Infof("Logs channel closed, stop watching logs")
				return
			}

			err := cb(ctx, l)
			if err != nil {
				glog.Errorf("Error with log callback: %v", err)
				return
			}
		case err := <-sub.Err():
			glog.Errorf("Error with log subscription: %v", err)
			return
		}
	}
}

func watchBlocks(ctx context.Context, sub ethereum.Subscription, headersCh chan *types.Header, cb headerCallback) {
	for {
		select {
		case h, ok := <-headersCh:
			if !ok {
				glog.Infof("Headers channel closed, stop watching block headers")
				return
			}

			err := cb(ctx, *h)
			if err != nil {
				glog.Errorf("Error with header callback: %v", err)
				return
			}
		case err := <-sub.Err():
			glog.Errorf("Error with block subscription: %v", err)
			return
		}
	}
}
