package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"time"

	client "github.com/cosmos/cosmos-sdk/client/context"
	"github.com/cosmos/cosmos-sdk/codec"
	simapp "github.com/cosmos/cosmos-sdk/simapp"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	distcommon "github.com/cosmos/cosmos-sdk/x/distribution/client/common"
	disttypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	staktypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
)

var (
	start = int64(1354579) // midnight 1/1/2021
	end   = int64(2274386) // midnight 3/8/2021
	val   = "akash1lhenngdge40r5thghzxqpsryn4x084m9jkpdg2"
	cdc   *codec.Codec
)

func main() {
	setSDKContext()
	addr, err := sdk.AccAddressFromBech32(val)
	if err != nil {
		log.Fatal(err)
	}
	cdc = simapp.MakeCodec()
	// blocks, dates := makebm()
	// for _, d := range dates {
	// 	fmt.Println(d, blocks[d].Block.Height)
	// }
	bal, err := getAccountBalance(addr, start)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(bal)
}

func makebm() (map[time.Time]*ctypes.ResultBlock, []time.Time) {
	cli := ctx(start)
	stbl, err := cli.Client.Block(&start)
	if err != nil {
		log.Fatal(err)
	}
	edbl, err := cli.Client.Block(&end)
	if err != nil {
		log.Fatal(err)
	}

	var (
		spb      = float64(end-start) / edbl.Block.Time.Sub(stbl.Block.Time).Seconds()
		blockmap = map[time.Time]*ctypes.ResultBlock{}
	)

	dates := makedates(stbl.Block.Time, edbl.Block.Time)
	for _, date := range dates {
		if date.After(edbl.Block.Time) {
			break
		}
		nh := nbh(stbl, date, spb)
		estbl, err := cli.Client.Block(&nh)
		if err != nil {
			log.Fatal(err)
		}

		spb = secpb(stbl, estbl)

		diff := date.Sub(estbl.Block.Time)
		for math.Abs(diff.Seconds()) > 30 {
			nh := nbh(stbl, date, spb)
			estbl, err = cli.Client.Block(&nh)
			if err != nil {
				log.Fatal(err)
			}
			spb = secpb(stbl, estbl)
			diff = date.Sub(estbl.Block.Time)
		}
		blockmap[date] = estbl
		stbl = estbl
	}
	return blockmap, dates
}

type accountBlockData struct {
	Height     int64     `json:"height"`
	Balance    sdk.Coin  `json:"balance"`
	Staked     sdk.Coin  `json:"staked"`
	Rewards    sdk.Coin  `json:"rewards"`
	Commission sdk.Coin  `json:"commission"`
	Time       time.Time `json:"time"`
	Price      float64   `json:"price"`
}

func getHeightData(height int64, addr sdk.AccAddress) (accountBlockData, error) {
	var (
		val                = sdk.ValAddress(addr)
		eg                 = errgroup.Group{}
		com, bal, rew, stk sdk.Coin
		err                error
	)
	eg.Go(func() error {
		return retry.Do(func() error {
			com, err = cr.ValidatorCommissionAtHeight(height, val)
			return err
		})
	})
	eg.Go(func() error {
		return retry.Do(func() error {
			bal, err = cr.AccountBalanceAtHeight(height, addr)
			return err
		})
	})
	eg.Go(func() error {
		return retry.Do(func() error {
			rew, err = cr.AccountRewardsAtHeight(height, addr)
			return err
		})
	})
	eg.Go(func() error {
		return retry.Do(func() error {
			stk, err = cr.StakedTokens(height, addr)
			return err
		})
	})
	if err := eg.Wait(); err != nil {
		return AccountBlockData{}, err
	}
	return AccountBlockData{height, bal, stk, rew, com, date, 0}, nil
}

// account balance
func getAccountBalance(acc sdk.AccAddress, height int64) (sdk.Coin, error) {
	cli := ctx(height)
	accGetter := authtypes.NewAccountRetriever(cli)
	res, err := accGetter.GetAccount(acc)
	if err != nil {
		return sdk.Coin{}, err
	}

	return sdk.NewCoin("uakt", res.GetCoins().AmountOf("uakt")), nil
}

// rewards
func getRewardsBalance(acc sdk.AccAddress, height int64) (sdk.Coin, error) {
	cli := ctx(height)
	params := disttypes.NewQueryDelegatorParams(acc)
	bz, err := cdc.MarshalJSON(params)
	if err != nil {
		return sdk.Coin{}, fmt.Errorf("failed to marshal params: %w", err)
	}

	// query for delegator total rewards
	route := fmt.Sprintf("custom/distribution/%s", disttypes.QueryDelegatorTotalRewards)
	res, _, err := cli.QueryWithData(route, bz)
	if err != nil {
		return sdk.Coin{}, err
	}

	var result disttypes.QueryDelegatorTotalRewardsResponse
	if err = cdc.UnmarshalJSON(res, &result); err != nil {
		return sdk.Coin{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	out := sdk.NewCoin("uakt", sdk.ZeroInt())
	for _, r := range result.Rewards {
		out.Amount = out.Amount.Add(r.Reward.AmountOf("uakt").RoundInt())
	}
	return out, nil
}

// commission
func getCommissionBalance(val sdk.ValAddress, height int64) (sdk.Coin, error) {
	cli := ctx(height)
	res, err := distcommon.QueryValidatorCommission(cli, "distribution", val)
	if err != nil {
		return sdk.Coin{}, err
	}
	var valCom disttypes.ValidatorAccumulatedCommission
	cdc.MustUnmarshalJSON(res, &valCom)
	return sdk.NewCoin("uakt", valCom.AmountOf("uakt").RoundInt()), nil
}

// staked tokens
func getStakedBalance(acc sdk.AccAddress, height int64) (sdk.Coin, error) {
	cli := ctx(height)
	bz, err := cdc.MarshalJSON(staktypes.NewQueryDelegatorParams(acc))
	if err != nil {
		return sdk.Coin{}, err
	}

	route := fmt.Sprintf("custom/staking/%s", staktypes.QueryDelegatorDelegations)
	res, _, err := cli.QueryWithData(route, bz)
	if err != nil {
		return sdk.Coin{}, err
	}

	var resp staktypes.DelegationResponses
	if err := cdc.UnmarshalJSON(res, &resp); err != nil {
		return sdk.Coin{}, err
	}
	out := sdk.NewCoin("uakt", sdk.ZeroInt())
	for _, d := range resp {
		out.Amount = out.Amount.Add(d.Balance.Amount)
	}
	return out, nil
}

func nbh(startBlock *ctypes.ResultBlock, nextDate time.Time, secpb float64) int64 {
	return startBlock.Block.Height + int64(nextDate.Sub(startBlock.Block.Time).Seconds()/secpb)
}

func makedates(startTime, endTime time.Time) []time.Time {
	out := []time.Time{}
	ct := startTime
	for ct.Before(endTime) {
		mt := midnight(ct)
		out = append(out, mt)
		ct = ct.Add(time.Hour * 24)
		if ct.After(time.Now()) {
			return out
		}
	}

	return out[:len(out)-1]
}

func midnight(t0 time.Time) time.Time {
	if t0.Hour() < 12 {
		return time.Date(t0.Year(), t0.Month(), t0.Day()+1, 0, 0, 0, 0, t0.Location())
	}
	return time.Date(t0.Year(), t0.Month(), t0.Day(), 0, 0, 0, 0, t0.Location())
}

func ctx(height int64) client.CLIContext {
	rpc, err := rpchttp.New("http://localhost:26657", "/websocket")
	if err != nil {
		log.Fatal(err)
	}
	return client.CLIContext{
		Client:       rpc,
		ChainID:      "akashnet-1",
		Input:        os.Stdin,
		Output:       os.Stdout,
		Codec:        cdc,
		OutputFormat: "json",
		Height:       height,
		TrustNode:    true,
		Indent:       false,
		SkipConfirm:  true,
		NodeURI:      "http://localhost:26657",
	}
}

func secpb(b0, b1 *ctypes.ResultBlock) float64 {
	return b0.Block.Time.Sub(b1.Block.Time).Seconds() / float64(b0.Block.Height-b1.Block.Height)
}

func setSDKContext() {
	config := sdk.GetConfig()
	config.SetBech32PrefixForAccount("akash", "akashpub")
	config.SetBech32PrefixForValidator("akashvaloper", "akashvaloperpub")
	config.SetBech32PrefixForConsensusNode("akashvalcons", "akashvalconspub")
}
