#!/bin/bash

blocks=(4630000 4640000 4650000 4660000 4680000 4690000 4700000 4710000 4720000 4740000 4750000 4760000 4770000 4780000 4790000 4810000 4820000 4830000 4840000 4850000 4870000 4880000 4890000 4900000 4910000 4930000 4940000 4950000 4960000 4970000 4990000 5000000 5010000 5020000 5030000 5040000 5060000 5070000 5080000 5090000 5100000 5120000 5130000 5140000 5150000 5160000 5170000 5190000 5200000)

addr=cosmos130mdu9a0etmeuw52qfxk73pn0ga6gawkryh2z6
valaddr=cosmosvaloper130mdu9a0etmeuw52qfxk73pn0ga6gawkxsrlwf

for b in ${blocks[@]}; do
    echo "PULLING BLOCK $b"
    distrRewards=$(gaiacli q distribution rewards $addr --output json --height $b | jq -r '.total[0].amount')
    distrCommission=$(gaiacli q distribution commission $valaddr --output json --height $b | jq -r '.[0].amount')
    stakedAmount=$(gaiacli q staking delegations $addr --output json --height $b | jq -r '.[0].balance')
    accBalance=$(gaiacli q account $addr --output json --height $b | jq -r '.value.BaseVestingAccount.BaseAccount.coins[0].amount')
    echo "$distrRewards,$distrCommission,$stakedAmount,$accBalance"
done
