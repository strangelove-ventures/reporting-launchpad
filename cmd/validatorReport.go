package cmd

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/avast/retry-go"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

func init() {
	rootCmd.AddCommand(accountSelectCmd)
}

var accountSelectCmd = &cobra.Command{
	Use:   "validator-report [endpoint] [addrprefix] [validator-acc-address] [start-block] [end-block] [coingecko-id] [denom]",
	Args:  cobra.ExactArgs(7),
	Short: "outputs a csv of the data required for validator income reporting",
	RunE: func(cmd *cobra.Command, args []string) error {
		setSDKContext(args[1])
		address, err := sdk.AccAddressFromBech32(args[2])
		if err != nil {
			return err
		}
		start, err := strconv.ParseInt(args[3], 10, 64)
		if err != nil {
			return err
		}
		end, err := strconv.ParseInt(args[4], 10, 64)
		if err != nil {
			return err
		}
		blocks, _ := makebm(args[0], start, end)
		log.Println("called GetDateBlockHeightMapping")
		if err != nil {
			return err
		}

		stat, err := getStatus(args[0])
		if err != nil {
			return err
		}

		log.Printf("starting data pull, note coingecko api only returns 50 days / min...")

		var (
			eg        errgroup.Group
			blockData = map[int64]accountBlockData{}
			sem       = make(chan struct{}, 50)
			blockNums = []int{}
			count     int
		)
		for k, v := range blocks {
			blockNums = append(blockNums, int(v.Block.Height))
			k, v := k, v
			eg.Go(func() error {
				log.Printf("start fetching %d", v.Block.Height)
				bd, err := getHeightData(v.Block.Height, address, args[0], stat.NodeInfo.Network, args[6], k)
				if err != nil {
					return err
				}
				fmt.Println(bd)
				price, err := GetPrice(k, args[5])
				if err != nil {
					return err
				}
				bd.Price = price
				blockData[v.Block.Height] = bd
				<-sem
				count++
				if count%10 == 0 {
					log.Printf("%d of %d complete %f%%", count, len(blocks), (float64(count)/float64(len(blocks)))*100)
				}
				return nil
			})
			sem <- struct{}{}
		}

		// wait for all queries to return
		if err := eg.Wait(); err != nil {
			return err
		}
		log.Println("writing to file")
		// sort block numbers
		sort.Ints(blockNums)

		// create file to save csv
		// TODO: get chain id from status endpoint
		file := fmt.Sprintf("report-%s-%d-%d.csv", "", blockNums[0], blockNums[len(blockNums)-1])
		log.Printf("saving results to file: %s", file)
		out, err := os.Create(file)
		if err != nil {
			return err
		}

		// create csv writer and write data to the csv in order
		csv := csv.NewWriter(out)
		if err := csv.Write(csvHeaders()); err != nil {
			return err
		}
		for _, n := range blockNums {
			if err := csv.Write(blockData[int64(n)].CSVLine()); err != nil {
				return err
			}
		}

		// flush csv to file and return error
		csv.Flush()
		return csv.Error()
	},
}

func csvHeaders() []string {
	return []string{
		"date",
		"height",
		"price usd",
		"account balance native",
		"account balance usd",
		"staked balance native",
		"staked balance usd",
		"rewards balance native",
		"rewards balance usd",
		"commission balance native",
		"commission balance usd",
		"total balance native",
		"total balance usd",
	}
}

func (abd accountBlockData) CSVLine() []string {
	return []string{
		// date
		fmt.Sprintf("%d/%d/%d", abd.Time.Month(), abd.Time.Day(), abd.Time.Year()),
		// height
		fmt.Sprintf("%d", abd.Height),
		// price usd
		fmt.Sprintf("%f", abd.Price),
		// account balance native
		abd.Balance.Amount.Quo(sdk.NewInt(1000000)).String(),
		// account balance usd
		fmt.Sprintf("%f", float64(abd.Balance.Amount.Quo(sdk.NewInt(1000000)).Int64())*abd.Price),
		// staked balance native
		abd.Staked.Amount.Quo(sdk.NewInt(1000000)).String(),
		// staked balance usd
		fmt.Sprintf("%f", float64(abd.Staked.Amount.Quo(sdk.NewInt(1000000)).Int64())*abd.Price),
		// rewards balance native
		abd.Rewards.Amount.Quo(sdk.NewInt(1000000)).String(),
		// rewards balance usd
		fmt.Sprintf("%f", float64(abd.Rewards.Amount.Quo(sdk.NewInt(1000000)).Int64())*abd.Price),
		// commission balance native
		abd.Commission.Amount.Quo(sdk.NewInt(1000000)).String(),
		// commission balance usd
		fmt.Sprintf("%f", float64(abd.Commission.Amount.Quo(sdk.NewInt(1000000)).Int64())*abd.Price),
		// total balance native
		abd.Total().Amount.Quo(sdk.NewInt(1000000)).String(),
		// total balance usd
		fmt.Sprintf("%f", float64(abd.Total().Amount.Quo(sdk.NewInt(1000000)).Int64())*abd.Price),
	}
}

func (bd accountBlockData) Total() sdk.Coin {
	return bd.Balance.Add(bd.Staked).Add(bd.Rewards).Add(bd.Commission)
}

type ErrRateLimitExceeded error

func GetPrice(date time.Time, cgid string) (float64, error) {
	url := fmt.Sprintf("https://api.coingecko.com/api/v3/coins/%s/history?date=%s&localization=false", cgid, fmt.Sprintf("%d-%d-%d", date.Day(), date.Month(), date.Year()))
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/json")

	var resp *http.Response
	retry.Do(func() error {
		resp, err = http.DefaultClient.Do(req)
		switch {
		case resp.StatusCode == 429:
			return ErrRateLimitExceeded(fmt.Errorf("429"))
		case (resp.StatusCode < 200 || resp.StatusCode > 299):
			return fmt.Errorf("non 2xx or 429 status code %d", resp.StatusCode)
		case err != nil:
			return err
		default:
			return nil
		}
	}, retry.RetryIf(func(err error) bool {
		_, ok := err.(ErrRateLimitExceeded)
		return ok
	}), retry.Delay(time.Second*60))
	defer resp.Body.Close()
	bz, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	data := priceHistory{}
	if err := json.Unmarshal(bz, &data); err != nil {
		return 0, err
	}
	return data.MarketData.CurrentPrice["usd"], nil
}

type priceHistory struct {
	ID     string `json:"id"`
	Symbol string `json:"symbol"`
	Name   string `json:"name"`
	Image  struct {
		Thumb string `json:"thumb"`
		Small string `json:"small"`
	} `json:"image"`
	MarketData struct {
		CurrentPrice map[string]float64 `json:"current_price"`
		MarketCap    map[string]float64 `json:"market_cap"`
		TotalVolume  map[string]float64 `json:"total_volume"`
	} `json:"market_data"`
}
