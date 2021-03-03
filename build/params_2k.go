// +build debug 2k

package build

import (
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/actors/policy"
)

const BootstrappersFile = "zcsl.pi"
const GenesisFile = "zcsl.car"
//const BootstrappersFile = ""
//const GenesisFile = ""

const UpgradeBreezeHeight = -1
const BreezeGasTampingDuration = 0

const UpgradeSmokeHeight = -1
const UpgradeIgnitionHeight = -2
const UpgradeRefuelHeight = -3
const UpgradeTapeHeight = -4

const UpgradeActorsV2Height = 10
const UpgradeLiftoffHeight = -5

const UpgradeKumquatHeight = 15
const UpgradeCalicoHeight = 20
const UpgradePersianHeight = 25
const UpgradeOrangeHeight = 27
const UpgradeClausHeight = 30

const UpgradeActorsV3Height = 35

var DrandSchedule = map[abi.ChainEpoch]DrandEnum{
	0: DrandMainnet,
}

func init() {
	policy.SetSupportedProofTypes(abi.RegisteredSealProof_StackedDrg8MiBV1,
	abi.RegisteredSealProof_StackedDrg512MiBV1,
	abi.RegisteredSealProof_StackedDrg32GiBV1,)
	policy.SetConsensusMinerMinPower(abi.NewStoragePower(10<<20))
	policy.SetMinVerifiedDealSize(abi.NewStoragePower(256))
	policy.SetPreCommitChallengeDelay(10)
	BuildType |= Build2k
}

const BlockDelaySecs = uint64(15)

const PropagationDelaySecs = uint64(1)

// SlashablePowerDelay is the number of epochs after ElectionPeriodStart, after
// which the miner is slashed
//
// Epochs
const SlashablePowerDelay = 20

// Epochs
const InteractivePoRepConfidence = 6

const BootstrapPeerThreshold = 1
