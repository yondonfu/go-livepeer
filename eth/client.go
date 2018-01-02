/*
Package eth client is the go client for the Livepeer Ethereum smart contract.  Contracts here are generated.
*/
package eth

//go:generate abigen --abi protocol/abi/Controller.abi --pkg contracts --type Controller --out contracts/controller.go
//go:generate abigen --abi protocol/abi/LivepeerToken.abi --pkg contracts --type LivepeerToken --out contracts/livepeerToken.go
//go:generate abigen --abi protocol/abi/BondingManager.abi --pkg contracts --type BondingManager --out contracts/bondingManager.go
//go:generate abigen --abi protocol/abi/JobsManager.abi --pkg contracts --type JobsManager --out contracts/jobsManager.go
//go:generate abigen --abi protocol/abi/RoundsManager.abi --pkg contracts --type RoundsManager --out contracts/roundsManager.go
//go:generate abigen --abi protocol/abi/LivepeerTokenFaucet.abi --pkg contracts --type LivepeerTokenFaucet --out contracts/livepeerTokenFaucet.go

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/livepeer/go-livepeer/eth/contracts"
	"github.com/livepeer/go-livepeer/eth/contracts/interfaces"
)

type LivepeerEthClient interface {
	Account() accounts.Account
	Backend() *ethclient.Client
	Addresses() map[string]common.Address
	Setup(gasLimit, gasPrice *big.Int) error
	Controller() interfaces.Controller
	LivepeerToken() interfaces.LivepeerToken
	BondingManager() interfaces.BondingManager
	JobsManager() interfaces.JobsManager
	RoundsManager() interfaces.RoundsManager
	LivepeerTokenFaucet() interfaces.LivepeerTokenFaucet
	Sign([]byte) ([]byte, error)
	CheckTx(context.Context, *types.Transaction) error
}

type client struct {
	backend            *ethclient.Client
	controllerAddr     common.Address
	tokenAddr          common.Address
	bondingManagerAddr common.Address
	jobsManagerAddr    common.Address
	roundsManagerAddr  common.Address
	faucetAddr         common.Address
	controller         *contracts.ControllerSession
	token              *contracts.LivepeerTokenSession
	bondingManager     *contracts.BondingManagerSession
	jobsManager        *contracts.JobsManagerSession
	roundsManager      *contracts.RoundsManagerSession
	faucet             *contracts.LivepeerTokenFaucetSession

	accountManager *AccountManager
	txTimeout      time.Duration
}

func NewClient(accountAddr common.Address, keystoreDir string, backend *ethclient.Client, controllerAddr common.Address, txTimeout time.Duration) (LivepeerEthClient, error) {
	am, err := NewAccountManager(accountAddr, keystoreDir)
	if err != nil {
		return nil, err
	}

	return &client{
		backend:        backend,
		controllerAddr: controllerAddr,
		accountManager: am,
		txTimeout:      txTimeout,
	}, nil
}

func (c *client) Setup(gasLimit, gasPrice *big.Int) error {
	// Unlock account for client
	err := c.accountManager.Unlock()
	if err != nil {
		return err
	}

	// Create transact opts to attach to contract sessions
	opts, err := c.accountManager.CreateTransactOpts(gasLimit, gasPrice)
	if err != nil {
		return err
	}

	return c.setContracts(opts)
}

func (c *client) setContracts(opts *bind.TransactOpts) error {
	controller, err := contracts.NewController(c.controllerAddr, c.backend)
	if err != nil {
		return err
	}

	c.controller = &contracts.ControllerSession{
		Contract:     controller,
		TransactOpts: *opts,
	}

	tokenAddr, err := c.controller.GetContract(crypto.Keccak256Hash([]byte("LivepeerToken")))
	if err != nil {
		return err
	}

	c.tokenAddr = tokenAddr

	token, err := contracts.NewLivepeerToken(tokenAddr, c.backend)
	if err != nil {
		return err
	}

	c.token = &contracts.LivepeerTokenSession{
		Contract:     token,
		TransactOpts: *opts,
	}

	bondingManagerAddr, err := c.controller.GetContract(crypto.Keccak256Hash([]byte("BondingManager")))
	if err != nil {
		return err
	}

	c.bondingManagerAddr = bondingManagerAddr

	bondingManager, err := contracts.NewBondingManager(bondingManagerAddr, c.backend)
	if err != nil {
		return err
	}

	c.bondingManager = &contracts.BondingManagerSession{
		Contract:     bondingManager,
		TransactOpts: *opts,
	}

	jobsManagerAddr, err := c.controller.GetContract(crypto.Keccak256Hash([]byte("JobsManager")))
	if err != nil {
		return err
	}

	c.jobsManagerAddr = jobsManagerAddr

	jobsManager, err := contracts.NewJobsManager(jobsManagerAddr, c.backend)
	if err != nil {
		return err
	}

	c.jobsManager = &contracts.JobsManagerSession{
		Contract:     jobsManager,
		TransactOpts: *opts,
	}

	roundsManagerAddr, err := c.controller.GetContract(crypto.Keccak256Hash([]byte("RoundsManager")))
	if err != nil {
		return err
	}

	c.roundsManagerAddr = roundsManagerAddr

	roundsManager, err := contracts.NewRoundsManager(roundsManagerAddr, c.backend)
	if err != nil {
		return err
	}

	c.roundsManager = &contracts.RoundsManagerSession{
		Contract:     roundsManager,
		TransactOpts: *opts,
	}

	faucetAddr, err := c.controller.GetContract(crypto.Keccak256Hash([]byte("LivepeerTokenFaucet")))
	if err != nil {
		return err
	}

	c.faucetAddr = faucetAddr

	faucet, err := contracts.NewLivepeerTokenFaucet(faucetAddr, c.backend)
	if err != nil {
		return err
	}

	c.faucet = &contracts.LivepeerTokenFaucetSession{
		Contract:     faucet,
		TransactOpts: *opts,
	}

	return nil
}

func (c *client) Controller() interfaces.Controller {
	return c.controller
}

func (c *client) LivepeerToken() interfaces.LivepeerToken {
	return c.token
}

func (c *client) BondingManager() interfaces.BondingManager {
	return c.bondingManager
}

func (c *client) JobsManager() interfaces.JobsManager {
	return c.jobsManager
}

func (c *client) RoundsManager() interfaces.RoundsManager {
	return c.roundsManager
}

func (c *client) LivepeerTokenFaucet() interfaces.LivepeerTokenFaucet {
	return c.faucet
}

func (c *client) CheckTx(ctx context.Context, tx *types.Transaction) error {
	ctx, cancel := context.WithTimeout(ctx, c.txTimeout)
	defer cancel()

	receipt, err := bind.WaitMined(ctx, c.backend, tx)
	if err != nil {
		return err
	}

	if receipt.Status == uint(0) {
		return fmt.Errorf("tx %v failed", tx.Hash())
	} else {
		return nil
	}
}

func (c *client) Sign(msg []byte) ([]byte, error) {
	return c.accountManager.Sign(msg)
}

func (c *client) Account() accounts.Account {
	return c.accountManager.Account
}

func (c *client) Backend() *ethclient.Client {
	return c.backend
}

func (c *client) Addresses() map[string]common.Address {
	m := make(map[string]common.Address)

	m["Controller"] = c.controllerAddr
	m["LivepeerToken"] = c.tokenAddr
	m["BondingManager"] = c.bondingManagerAddr
	m["JobsManager"] = c.jobsManagerAddr
	m["RoundsManager"] = c.roundsManagerAddr
	m["LivepeerTokenFaucet"] = c.faucetAddr

	return m
}
