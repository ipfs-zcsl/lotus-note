package cli

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/chain/actors"

	miner3 "github.com/filecoin-project/specs-actors/v3/actors/builtin/miner"

	"github.com/filecoin-project/go-state-types/big"
	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	builtin3 "github.com/filecoin-project/specs-actors/v3/actors/builtin"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/store"
	"github.com/urfave/cli/v2"
)

const Confidence = 10

type minerDeadline struct {
	miner address.Address
	index uint64
}

var chainDisputeSetCmd = &cli.Command{
	Name:  "disputer",
	Usage: "interact with the window post disputer",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "max-fee",
			Usage: "Spend up to X attoFIL for DisputeWindowedPoSt message(s)",
		},
		&cli.StringFlag{
			Name:  "from",
			Usage: "optionally specify the account to send funds from",
		},
	},
	Subcommands: []*cli.Command{
		disputerStartCmd,
		disputerMsgCmd,
	},
}

var disputerMsgCmd = &cli.Command{
	Name:      "dispute",
	Usage:     "Send a specific DisputeWindowedPoSt message",
	ArgsUsage: "[minerAddress index postIndex]",
	Flags:     []cli.Flag{},
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 3 {
			fmt.Println("Usage: findpeer [minerAddress index postIndex]")
			return nil
		}

		ctx := ReqContext(cctx)

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		toa, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return fmt.Errorf("given 'to' address %q was invalid: %w", cctx.Args().First(), err)
		}

		deadline, err := strconv.ParseUint(cctx.Args().Get(1), 10, 64)
		if err != nil {
			return err
		}

		postIndex, err := strconv.ParseUint(cctx.Args().Get(2), 10, 64)
		if err != nil {
			return err
		}

		var fromAddr address.Address
		if from := cctx.String("from"); from == "" {
			defaddr, err := api.WalletDefaultAddress(ctx)
			if err != nil {
				return err
			}

			fromAddr = defaddr
		} else {
			addr, err := address.NewFromString(from)
			if err != nil {
				return err
			}

			has, err := api.WalletHas(ctx, addr)
			if err != nil {
				return err
			}

			if !has {
				return xerrors.Errorf("wallet doesn't contain: %s ", addr)
			}

			fromAddr = addr
		}

		dpp, aerr := actors.SerializeParams(&miner3.DisputeWindowedPoStParams{
			Deadline:  deadline,
			PoStIndex: postIndex,
		})

		if aerr != nil {
			return xerrors.Errorf("failed to serailize params: %w", aerr)
		}

		dmsg := &types.Message{
			To:     toa,
			From:   fromAddr,
			Value:  big.Zero(),
			Method: builtin3.MethodsMiner.DisputeWindowedPoSt,
			Params: dpp,
		}

		rslt, err := api.StateCall(ctx, dmsg, types.EmptyTSK)
		if err != nil {
			return xerrors.Errorf("failed to simulate dispute: %w", err)
		}

		var mss *lapi.MessageSendSpec
		if cctx.IsSet("max-fee") {
			maxFee, err := types.BigFromString(cctx.String("max-fee"))
			if err != nil {
				return fmt.Errorf("parsing max-fee: %w", err)
			}
			mss = &lapi.MessageSendSpec{
				MaxFee: maxFee,
			}
		}

		if rslt.MsgRct.ExitCode == 0 {
			sm, err := api.MpoolPushMessage(ctx, dmsg, mss)
			if err != nil {
				return err
			}

			fmt.Println("dispute message ", sm.Cid())
		} else {
			fmt.Println("dispute is unsuccessful")
		}

		return nil
	},
}

var disputerStartCmd = &cli.Command{
	Name:      "start",
	Usage:     "Start the window post disputer",
	ArgsUsage: "[minerAddress]",
	Flags:     []cli.Flag{},
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		var fromAddr address.Address
		if from := cctx.String("from"); from == "" {
			defaddr, err := api.WalletDefaultAddress(ctx)
			if err != nil {
				return err
			}

			fromAddr = defaddr
		} else {
			addr, err := address.NewFromString(from)
			if err != nil {
				return err
			}

			has, err := api.WalletHas(ctx, addr)
			if err != nil {
				return err
			}

			if !has {
				return xerrors.Errorf("wallet doesn't contain: %s ", addr)
			}

			fromAddr = addr
		}

		fmt.Println("checking sync status")

		if err := SyncWait(ctx, api, false); err != nil {
			return xerrors.Errorf("sync wait: %w", err)
		}

		fmt.Println("setting up window post disputer")

		// build initial deadlineMap

		h, err := api.ChainHead(ctx)
		if err != nil {
			return err
		}

		lastEpoch := h.Height()

		minerList, err := api.StateListMiners(ctx, types.EmptyTSK)
		if err != nil {
			return err
		}

		knownMiners := make(map[address.Address]struct{})
		deadlineMap := make(map[abi.ChainEpoch][]minerDeadline)
		for _, miner := range minerList {
			dl, err := api.StateMinerProvingDeadline(ctx, miner, types.EmptyTSK)
			if err != nil {
				return err
			}

			deadlineMap[dl.Close+Confidence] = append(deadlineMap[dl.Close+Confidence], minerDeadline{
				miner: miner,
				index: dl.Index,
			})

			knownMiners[miner] = struct{}{}
		}

		headChanges, err := api.ChainNotify(ctx)
		if err != nil {
			return err
		}

		head, ok := <-headChanges
		if !ok {
			return xerrors.Errorf("Notify stream was invalid")
		}

		if len(head) != 1 {
			return xerrors.Errorf("Notify first entry should have been one item")
		}

		if head[0].Type != store.HCCurrent {
			return xerrors.Errorf("expected current head on Notify stream (got %s)", head[0].Type)
		}

		var mss *lapi.MessageSendSpec
		if cctx.IsSet("max-fee") {
			maxFee, err := types.BigFromString(cctx.String("max-fee"))
			if err != nil {
				return fmt.Errorf("parsing max-fee: %w", err)
			}
			mss = &lapi.MessageSendSpec{
				MaxFee: maxFee,
			}
		}

		healthTicker := time.NewTicker(5 * time.Minute)
		defer healthTicker.Stop()

		statusCheckTicker := time.NewTicker(time.Hour)
		defer statusCheckTicker.Stop()

		lastStatusCheckEpoch := lastEpoch

		fmt.Println("starting up window post disputer")

		disputeLoop := func() (error, bool) {
			select {
			case notif, ok := <-headChanges:
				if !ok {
					return xerrors.Errorf("head change channel errored"), true
				}
				for _, val := range notif {
					switch val.Type {
					case store.HCApply:

						for ; lastEpoch <= val.Val.Height(); lastEpoch++ {

							dls, ok := deadlineMap[lastEpoch]
							if !ok {
								// no deadlines closed at this epoch - Confidence
								continue
							}

							dpmsgs := make([]*types.Message, 0)
							for _, dl := range dls {
								fullDeadlines, err := api.StateMinerDeadlines(ctx, dl.miner, val.Val.Key())
								if err != nil {
									return xerrors.Errorf("failed to load deadlines: %w", err), false
								}

								if int(dl.index) >= len(fullDeadlines) {
									return xerrors.Errorf("deadline index %d not found in deadlines", dl.index), false
								}

								ms, err := makeDisputeWindowedPosts(ctx, api, dl, fullDeadlines[dl.index].OptimisticPoStSubmissionsSnapshotLength, fromAddr)
								if err != nil {
									return xerrors.Errorf("failed to check for disputes: %w", err), false
								}

								dpmsgs = append(dpmsgs, ms...)

								newDl, err := api.StateMinerProvingDeadline(ctx, dl.miner, types.EmptyTSK)
								if err != nil {
									return xerrors.Errorf("failed to update deadlineMap: %w", err), true
								}

								deadlineMap[newDl.Close+Confidence] = append(deadlineMap[newDl.Close+Confidence], minerDeadline{
									miner: dl.miner,
									index: newDl.Index,
								})
							}

							for _, dpmsg := range dpmsgs {
								fmt.Println("disputing a PoSt from miner ", dpmsg.To)
								_, err := api.MpoolPushMessage(ctx, dpmsg, mss)
								if err != nil {
									return xerrors.Errorf("failed to dispute post message: %w", err), false
								}

								// TODO: Track / report on message landing on chain?
							}

							delete(deadlineMap, lastEpoch)
						}

					case store.HCRevert:
						// do nothing
					default:
						return xerrors.Errorf("unexpected head change type %s", val.Type), true
					}
				}
			case <-healthTicker.C:
				fmt.Print("Running health check: ")

				cctx, cancel := context.WithTimeout(ctx, 5*time.Second)

				if _, err := api.ID(cctx); err != nil {
					cancel()
					return xerrors.Errorf("health check failed"), true
				}

				cancel()

				fmt.Println("Node online")
			case <-statusCheckTicker.C:
				fmt.Print("Running status check: ")

				minerList, err = api.StateListMiners(ctx, types.EmptyTSK)
				if err != nil {
					return xerrors.Errorf("getting miner list: %w", err), true
				}

				for _, m := range minerList {
					_, ok := knownMiners[m]
					if !ok {
						dl, err := api.StateMinerProvingDeadline(ctx, m, types.EmptyTSK)
						if err != nil {
							return xerrors.Errorf("getting proving index list: %w", err), true
						}

						deadlineMap[dl.Close+Confidence] = append(deadlineMap[dl.Close+Confidence], minerDeadline{
							miner: m,
							index: dl.Index,
						})

						knownMiners[m] = struct{}{}
					}
				}

				for ; lastStatusCheckEpoch < lastEpoch; lastStatusCheckEpoch++ {
					// if an epoch got "skipped" from the deadlineMap somehow, just fry it now instead of letting it sit around forever
					delete(deadlineMap, lastStatusCheckEpoch)
				}

				fmt.Println("Status check complete")
			case <-ctx.Done():
				return nil, true
			}

			return nil, false
		}

		for {
			err, shutdown := disputeLoop()
			if err != nil && shutdown {
				fmt.Println("disputer errored, shutting down: ", err)
				break
			}

			if err != nil && !shutdown {
				fmt.Println("disputer errored, continuing to run: ", err)
			}

			if shutdown {
				fmt.Println("disputer shutting down")
				break
			}
		}

		return nil
	},
}

// for a given miner, index, and maxPostIndex, tries to dispute posts from 0...postsSnapshotted-1
// returns a list of DisputeWindowedPoSt msgs that are expected to succeed if sent
func makeDisputeWindowedPosts(ctx context.Context, api lapi.FullNode, dl minerDeadline, postsSnapshotted uint64, sender address.Address) ([]*types.Message, error) {
	disputes := make([]*types.Message, 0)
	// TODO: Remove
	fmt.Println("Asked to make up to ", postsSnapshotted)

	for i := uint64(0); i < postsSnapshotted; i++ {

		dpp, aerr := actors.SerializeParams(&miner3.DisputeWindowedPoStParams{
			Deadline:  dl.index,
			PoStIndex: i,
		})

		if aerr != nil {
			return nil, xerrors.Errorf("failed to serailize params: %w", aerr)
		}

		dispute := &types.Message{
			To:     dl.miner,
			From:   sender,
			Value:  big.Zero(),
			Method: builtin3.MethodsMiner.DisputeWindowedPoSt,
			Params: dpp,
		}

		rslt, err := api.StateCall(ctx, dispute, types.EmptyTSK)
		if err == nil && rslt.MsgRct.ExitCode == 0 {
			// TODO: Remove when ready to merge
			fmt.Println("DISPUTE THIS!!")
			disputes = append(disputes, dispute)
		} else {
			// TODO: Remove when ready to merge
			fmt.Println("Sorry, can't dispute this")
			fmt.Println(err)
			fmt.Println(rslt.MsgRct.ExitCode)
		}

	}

	return disputes, nil
}
