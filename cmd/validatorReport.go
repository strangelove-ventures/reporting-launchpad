package cmd

import (
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

func init() {
	rootCmd.AddCommand(accountSelectCmd)
}

var accountSelectCmd = &cobra.Command{
	Use:   "validator-report [endpoint] [addrprefix] [validator-acc-address] [start-block]",
	Args:  cobra.ExactArgs(3),
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

		stat, err := getStatus(args[0])
		if err != nil {
			return err
		}
		blocks, _ := makebm(args[0], start, stat.SyncInfo.LatestBlockHeight)
		log.Println("called GetDateBlockHeightMapping")
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
		for _, v := range blocks {
			blockNums = append(blockNums, int(v.Block.Height))
			v := v
			eg.Go(func() error {
				log.Printf("start fetching %d", v.Block.Height)
				bd, err := getHeightData(v.Block.Height, address, args[0])
				if err != nil {
					return err
				}
				// price, err :=
				// if err != nil {
				// 	return err
				// }
				// bd.Price = price
				// TODO: implement price
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
		// if err := csv.Write(csvHeaders()); err != nil {
		// 	return err
		// }
		// for _, n := range blockNums {
		// 	if err := csv.Write(blockData[int64(n)].CSVLine()); err != nil {
		// 		return err
		// 	}
		// }

		// flush csv to file and return error
		csv.Flush()
		return csv.Error()
	},
}
