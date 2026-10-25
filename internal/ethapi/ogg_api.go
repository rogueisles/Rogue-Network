package ethapi

import (
	"github.com/TeamEGEM/go-egem/common"
	"github.com/TeamEGEM/go-egem/common/hexutil"
)

// OGGBlockReward reports subsidy allocations, recipients and separate special
// issuance. Reward-suffixed fields preserve the explorer response contract.
type OGGBlockReward struct {
	TargetBlockTimeMillis  hexutil.Uint64  `json:"targetBlockTimeMillis"`
	BlockNumber            hexutil.Uint64  `json:"blockNumber"`
	Total                  *hexutil.Big    `json:"total"`
	Miner                  *hexutil.Big    `json:"miner"`
	Staking                *hexutil.Big    `json:"staking"`
	Tribe                  *hexutil.Big    `json:"tribe"`
	Maintenance            *hexutil.Big    `json:"maintenance"`
	P2E                    *hexutil.Big    `json:"p2e"`
	Treasury               *hexutil.Big    `json:"treasury"`
	SpecialIssuance        *hexutil.Big    `json:"specialIssuance"`
	CurrencySymbol         string          `json:"currencySymbol"`
	BlockHash              *common.Hash    `json:"blockHash"`
	Pending                bool            `json:"pending"`
	TotalReward            *hexutil.Big    `json:"totalReward"`
	MinerReward            *hexutil.Big    `json:"minerReward"`
	MinerAddress           common.Address  `json:"minerAddress"`
	StakingReward          *hexutil.Big    `json:"stakingReward"`
	StakingAddress         common.Address  `json:"stakingAddress"`
	TribeReward            *hexutil.Big    `json:"tribeReward"`
	TribeAddress           common.Address  `json:"tribeAddress"`
	MaintenanceReward      *hexutil.Big    `json:"maintenanceReward"`
	MaintenanceAddress     common.Address  `json:"maintenanceAddress"`
	P2EReward              *hexutil.Big    `json:"p2eReward"`
	P2EAddress             common.Address  `json:"p2eAddress"`
	TreasuryReward         *hexutil.Big    `json:"treasuryReward"`
	TreasuryAddress        common.Address  `json:"treasuryAddress"`
	SpecialIssuanceAddress *common.Address `json:"specialIssuanceAddress,omitempty"`
}
