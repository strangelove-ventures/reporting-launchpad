/*
Copyright © 2021 NAME HERE <EMAIL ADDRESS>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cmd

import (
	"fmt"
	"strconv"

	simapp "github.com/cosmos/cosmos-sdk/simapp"
	"github.com/spf13/cobra"
)

// dayBlocksCmd represents the dayBlocks command
var dayBlocksCmd = &cobra.Command{
	Use:   "dayBlocks [endpoint] [start] [end]",
	Short: "A brief description of your command",
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: validation on the endpoint
		start, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil {
			return err
		}
		end, err := strconv.ParseInt(args[2], 10, 64)
		if err != nil {
			return err
		}
		cdc = simapp.MakeCodec()
		blocks, dates := makebm(args[0], start, end)
		for _, d := range dates {
			fmt.Println(d, blocks[d].Block.Height)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(dayBlocksCmd)
}
