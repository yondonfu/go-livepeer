package interfaces

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

type Controller interface {
	GetContract([32]byte) (common.Address, error)
}

type LivepeerToken interface {
	BalanceOf(common.Address) (*big.Int, error)
	Approve(common.Address, *big.Int) (*types.Transaction, error)
	Transfer(common.Address, *big.Int) (*types.Transaction, error)
}

type BondingManager interface {
	// Bonding
	Bond(*big.Int, common.Address) (*types.Transaction, error)
	Unbond() (*types.Transaction, error)
	Withdraw() (*types.Transaction, error)

	// Transcoder operations
	Transcoder(*big.Int, *big.Int, *big.Int) (*types.Transaction, error)
	ResignAsTranscoder() (*types.Transaction, error)
	Reward() (*types.Transaction, error)

	// Claiming rewards and fees
	ClaimTokenPoolsShares(*big.Int) (*types.Transaction, error)

	// Getters
	GetDelegator(common.Address) (struct {
		BondedAmount                   *big.Int
		UnbondedAmount                 *big.Int
		DelegateAddress                common.Address
		DelegatedAmount                *big.Int
		StartRound                     *big.Int
		WithdrawRound                  *big.Int
		LastClaimTokenPoolsSharesRound *big.Int
	}, error)
	GetTranscoder(common.Address) (struct {
		LastRewardRound        *big.Int
		BlockRewardCut         *big.Int
		FeeShare               *big.Int
		PricePerSegment        *big.Int
		PendingBlockRewardCut  *big.Int
		PendingFeeShare        *big.Int
		PendingPricePerSegment *big.Int
	}, error)
	ElectActiveTranscoder(*big.Int, *big.Int, *big.Int) (common.Address, error)
	GetFirstTranscoderInPool() (common.Address, error)
	GetNextTranscoderInPool(common.Address) (common.Address, error)
	IsActiveTranscoder(common.Address, *big.Int) (bool, error)
	TranscoderTotalStake(common.Address) (*big.Int, error)
}

type JobsManager interface {
	// Job creation
	Deposit(*big.Int) (*types.Transaction, error)
	Job(string, string, *big.Int, *big.Int) (*types.Transaction, error)
	Withdraw() (*types.Transaction, error)

	// Claiming and verifying work
	ClaimWork(*big.Int, [2]*big.Int, [32]byte) (*types.Transaction, error)
	Verify(*big.Int, *big.Int, *big.Int, string, [2][32]byte, []byte, []byte) (*types.Transaction, error)
	DistributeFees(jobID, claimID *big.Int) (*types.Transaction, error)
	BatchDistributeFees(*big.Int, []*big.Int) (*types.Transaction, error)

	// Slashing
	DoubleClaimSegmentSlash(jobId, claimId1, claimId2, segmentNumber *big.Int) (*types.Transaction, error)
	MissedVerificationSlash(*big.Int, *big.Int, *big.Int) (*types.Transaction, error)

	// Getters
	VerificationPeriod() (*big.Int, error)
	VerificationRate() (uint64, error)
	IsClaimSegmentVerified(jobID, claimID, segmentNumber *big.Int) (bool, error)
	GetJob(*big.Int) (struct {
		StreamId           string
		TranscodingOptions string
		MaxPricePerSegment *big.Int
		BroadcasterAddress common.Address
		TranscoderAddress  common.Address
		CreationRound      *big.Int
		CreationBlock      *big.Int
		EndBlock           *big.Int
		Escrow             *big.Int
		TotalClaims        *big.Int
	}, error)
	GetClaim(jobID, claimID *big.Int) (struct {
		SegmentRange         [2]*big.Int
		ClaimRoot            [32]byte
		ClaimBlock           *big.Int
		EndVerificationBlock *big.Int
		EndSlashingBlock     *big.Int
		Status               uint8
	}, error)
}

type RoundsManager interface {
	CurrentRound() (*big.Int, error)
	CurrentRoundInitialized() (bool, error)
	CurrentRoundLocked() (bool, error)
	LastInitializedRound() (*big.Int, error)
	RoundLength() (*big.Int, error)
	InitializeRound() (*types.Transaction, error)
}

type LivepeerTokenFaucet interface {
	Request() (*types.Transaction, error)
	NextValidRequest(common.Address) (*big.Int, error)
}
