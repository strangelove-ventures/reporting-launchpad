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

	simapp "github.com/cosmos/cosmos-sdk/simapp"
	"github.com/spf13/cobra"
)

// dayBlocksCmd represents the dayBlocks command
var dayBlocksCmd = &cobra.Command{
	Use:   "dayBlocks",
	Short: "A brief description of your command",
	Run: func(cmd *cobra.Command, args []string) {
		setSDKContext()
		// addr, err := sdk.AccAddressFromBech32(val)
		// if err != nil {
		// 	log.Fatal(err)
		//}
		cdc = simapp.MakeCodec()
		blocks, dates := makebm()
		for _, d := range dates {
			fmt.Println(d, blocks[d].Block.Height)
		}

		//fmt.Println(bal)
	},
}

func init() {
	rootCmd.AddCommand(dayBlocksCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// dayBlocksCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// dayBlocksCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
