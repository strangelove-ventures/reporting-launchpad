package cmd

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"time"

	"github.com/avast/retry-go"
	client "github.com/cosmos/cosmos-sdk/client/context"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/simapp"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	distcommon "github.com/cosmos/cosmos-sdk/x/distribution/client/common"
	disttypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	staketypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
	"golang.org/x/sync/errgroup"
)

var (
	// start = int64(1354580) // midnight 1/1/2021
	// end   = int64(2274386) // midnight 3/8/2021
	// start = int64(1)
	// end   = int64(1536089)
	// val = "kava16lnfpgn6llvn4fstg5nfrljj6aaxyee90zl6c6"
	cdc *codec.Codec
)

func makebm(endpoint string, start, end int64) (map[time.Time]*ctypes.ResultBlock, []time.Time) {
	stat, err := getStatus(endpoint)
	if err != nil {
		log.Fatal(err)
	}
	cli := ctx(start, endpoint, stat.NodeInfo.Network)
	stbl, err := cli.Client.Block(&start)
	if err != nil {
		log.Fatal(err)
	}
	edbl, err := cli.Client.Block(&end)
	if err != nil {
		log.Fatal(err)
	}

	var (
		spb      = stbl.Block.Time.Sub(edbl.Block.Time).Seconds() / float64(start-end)
		blockmap = map[time.Time]*ctypes.ResultBlock{}
		dates    = makedates(stbl.Block.Time, edbl.Block.Time)
	)

	blockmap[stbl.Block.Time] = stbl
	blockmap[edbl.Block.Time] = edbl

	iterdates := dates[1 : len(dates)-1]

	for _, date := range iterdates {
		nh := nbh(stbl, date, spb)
		estbl, err := cli.Client.Block(&nh)
		if err != nil {
			log.Fatal(err)
		}

		spb = secpb(stbl, estbl)

		diff := date.Sub(estbl.Block.Time)
		for math.Abs(diff.Seconds()) > 60 {
			nh := nbh(stbl, date, spb)
			estbl, err = cli.Client.Block(&nh)
			if err != nil {
				log.Fatal(err)
			}
			spb = secpb(stbl, estbl)
			diff = date.Sub(estbl.Block.Time)
		}
		blockmap[date] = estbl
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

func getHeightData(height int64, addr sdk.AccAddress, endpoint, chainid, denom string, t time.Time) (accountBlockData, error) {
	var (
		val                = sdk.ValAddress(addr)
		eg                 = errgroup.Group{}
		com, bal, rew, stk sdk.Coin
		err                error
	)
	eg.Go(func() error {
		return retry.Do(func() error {
			com, err = getCommissionBalance(val, height, endpoint, chainid, denom)
			fmt.Println("getCommissionBalance", err)
			fmt.Println(height)
			return err
		})
	})
	eg.Go(func() error {
		return retry.Do(func() error {
			bal, err = getAccountBalance(addr, height, endpoint, chainid, denom)
			fmt.Println("getAccountBalance", err)
			return err
		})
	})
	eg.Go(func() error {
		return retry.Do(func() error {
			rew, err = getRewardsBalance(addr, height, endpoint, chainid, denom)
			fmt.Println("getRewardsBalance", err)
			return err
		})
	})
	eg.Go(func() error {
		return retry.Do(func() error {
			stk, err = getStakedBalance(addr, height, endpoint, chainid, denom)
			fmt.Println("getStakedBalance", err)
			fmt.Println(height)
			return err
		})
	})
	if err := eg.Wait(); err != nil {
		return accountBlockData{}, err
	}
	return accountBlockData{height, bal, stk, rew, com, t, 0}, nil
}

// account balance
func getAccountBalance(acc sdk.AccAddress, height int64, endpoint, chainid, denom string) (sdk.Coin, error) {
	cli := ctx(height, endpoint, chainid)
	accGetter := authtypes.NewAccountRetriever(cli)
	res, err := accGetter.GetAccount(acc)
	if err != nil {
		return sdk.Coin{}, err
	}

	return sdk.NewCoin(denom, res.GetCoins().AmountOf(denom)), nil
}

// rewards
func getRewardsBalance(acc sdk.AccAddress, height int64, endpoint, chainid, denom string) (sdk.Coin, error) {
	cli := ctx(height, endpoint, chainid)
	params := disttypes.NewQueryDelegatorParams(acc)
	bz, err := json.Marshal(params)
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
	if err = json.Unmarshal(res, &result); err != nil {
		return sdk.Coin{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	out := sdk.NewCoin(denom, sdk.ZeroInt())
	for _, r := range result.Rewards {
		out.Amount = out.Amount.Add(r.Reward.AmountOf(denom).RoundInt())
	}
	return out, nil
}

// commission
func getCommissionBalance(val sdk.ValAddress, height int64, endpoint, chainid, denom string) (sdk.Coin, error) {
	cli := ctx(height, endpoint, chainid)
	res, err := distcommon.QueryValidatorCommission(cli, "distribution", val)
	if err != nil {
		return sdk.Coin{}, err
	}
	var valCom disttypes.ValidatorAccumulatedCommission
	if err := json.Unmarshal(res, &valCom); err != nil {
		return sdk.Coin{}, err
	}
	return sdk.NewCoin(denom, valCom.AmountOf(denom).RoundInt()), nil
}

// staked tokens
func getStakedBalance(acc sdk.AccAddress, height int64, endpoint, chainid, denom string) (sdk.Coin, error) {
	cli := ctx(height, endpoint, chainid)
	params := staketypes.NewQueryDelegatorParams(acc)
	bz, err := json.Marshal(params)
	if err != nil {
		return sdk.Coin{}, err
	}

	route := fmt.Sprintf("custom/staking/%s", staketypes.QueryDelegatorDelegations)
	res, _, err := cli.QueryWithData(route, bz)
	if err != nil {
		return sdk.Coin{}, err
	}

	var resp staketypes.DelegationResponses
	if err := json.Unmarshal(res, &resp); err != nil {
		return sdk.Coin{}, err
	}
	out := sdk.NewCoin(denom, sdk.ZeroInt())
	for _, d := range resp {
		out.Amount = out.Amount.Add(d.Balance.Amount)
	}
	return out, nil
}

func getStatus(endpoint string) (*ctypes.ResultStatus, error) {
	return ctx(0, endpoint, "").Client.Status()
}

func nbh(startBlock *ctypes.ResultBlock, nextDate time.Time, secpb float64) int64 {
	return startBlock.Block.Height + int64(nextDate.Sub(startBlock.Block.Time).Seconds()/secpb)
}

func makedates(startTime, endTime time.Time) []time.Time {
	out := []time.Time{startTime}
	ct := startTime
	for {
		ct = midnight(ct)
		ct = ct.Add(time.Hour * 24)
		if ct.Before(endTime) {
			out = append(out, ct)
		} else if ct.After(endTime) || ct.After(time.Now()) {
			out = append(out, endTime)
			break
		}
	}
	return out
}

func midnight(t0 time.Time) time.Time {
	return time.Date(t0.Year(), t0.Month(), t0.Day(), 0, 0, 0, 0, t0.Location())
}

func ctx(height int64, endpoint, chainid string) client.CLIContext {
	rpc, err := rpchttp.New(endpoint, "/websocket")
	if err != nil {
		log.Fatal(err)
	}
	return client.CLIContext{
		Client:       rpc,
		ChainID:      chainid,
		Input:        os.Stdin,
		Output:       os.Stdout,
		Codec:        simapp.MakeCodec(),
		OutputFormat: "json",
		Height:       height,
		TrustNode:    true,
		Indent:       false,
		SkipConfirm:  true,
		NodeURI:      endpoint,
	}
}

func secpb(b0, b1 *ctypes.ResultBlock) float64 {
	return b0.Block.Time.Sub(b1.Block.Time).Seconds() / float64(b0.Block.Height-b1.Block.Height)
}

func setSDKContext(prefix string) {
	config := sdk.GetConfig()
	config.SetBech32PrefixForAccount(fmt.Sprintf("%s", prefix), fmt.Sprintf("%spub", prefix))
	config.SetBech32PrefixForValidator(fmt.Sprintf("%svaloper", prefix), fmt.Sprintf("%svaloperpub", prefix))
	config.SetBech32PrefixForConsensusNode(fmt.Sprintf("%svalcons", prefix), fmt.Sprintf("%svalconspub", prefix))
}
