#!/bin/bash

# env
export CI=false
export LOG_COLOR=true
export SERVER_LOG_TO_STACKDRIVER=false
export SERVER_LOG_TO_STDOUT=true
export SERVER_PROJECT_ID=mai3-trade-mining-watcher
export SERVER_LOGLEVEL=6
export HOSTNAME=localhost

# DB parameters
export DB_MAX_IDLE_CONNS=-1
export DB_MAX_OPEN_CONNS=16
export DB_ARGS=postgres://mcdex@localhost:5432/mcdex?sslmode=disable # Disable ssl for now

# setting
export DEFAULT_EPOCH_0_START_TIME=1634058000
export RESET_DATABASE=false
export SYNC_DELAY=0
export SNAPSHOT_INTERVAL=60
export MULTI_CHAIN_EPOCH_START=0

# Arb-rinkeby graph url & inverse white list
export ARB_RINKEBY_CHAIN=false
export ARB_RINKEBY_MAI3_GRAPH_URL=https://api.thegraph.com/subgraphs/name/champfu-mcdex/mai3-trading-mining2
export ARB_RINKEBY_BLOCK_GRAPH_URL=https://api.thegraph.com/subgraphs/name/renpu-mcarlo/arbitrum-rinkeby-blocks
export ARB_RINKEBY_BTC_INVERSE_CONTRACT_WHITELIST0='0x3d3744dc7a17d757a2568ddb171d162a7e12f80a-0' # arb-rinkeby USD-BTC
export ARB_RINKEBY_ETH_INVERSE_CONTRACT_WHITELIST0='0x727e5a9a04080741cbc8a2dc891e28ca8af6537e-0' # USD-ETHB
export ARB_RINKEBY_BTC_USD_PERP_ID="0xc32a2dfee97e2babc90a2b5e6aef41e789ef2e13-1"
export ARB_RINKEBY_ETH_USD_PERP_ID="0xc32a2dfee97e2babc90a2b5e6aef41e789ef2e13-0"
export ARB_RINKEBY_PRC_SERVER="https://rinkeby.arbitrum.io/rpc"

# BSC graph url & inverse white list
export BSC_CHAIN=false
export BSC_MAI3_GRAPH_URL=https://api.thegraph.com/subgraphs/name/mcdexio/mcdex3-bsc-trade-mining
# export BSC_MAI3_GRAPH_URL=https://api.thegraph.com/subgraphs/name/champfu-mcdex/bsc-mining-fee
# export BSC_MAI3_GRAPH_URL=https://graph-bsc.mcdex.io/subgraphs/name/mcdexio/mcdex3-bsc-trade-mining
export BSC_BLOCK_GRAPH_URL=https://api.thegraph.com/subgraphs/name/venomprotocol/bsc-blocks
# export BSC_BLOCK_GRAPH_URL=https://graph-bsc.mcdex.io/subgraphs/name/generatefinance/bsc-blocks
export BSC_BTC_INVERSE_CONTRACT_WHITELIST0='0x2ea001032b0eb424120b4dec51bf02db0df46c78-0' # bsc USD-BTC
export BSC_ETH_INVERSE_CONTRACT_WHITELIST0='0xf6b2d76c248af20009188139660a516e5c4e0532-0' # bsc USD-ETH
export BSC_ETH_INVERSE_CONTRACT_WHITELIST1='0xf6b2d76c248af20009188139660a516e5c4e0532-1' # bsc BTC-ETH
export BSC_SATS_INVERSE_CONTRACT_WHITELIST0='0xfdd10c021b43c4be1b9f0473bad686e546d98b00-0' # bsc USD-SATS
export BSC_BTC_USD_PERP_ID="0xdb282bbace4e375ff2901b84aceb33016d0d663d-0"
export BSC_ETH_USD_PERP_ID="0xdb282bbace4e375ff2901b84aceb33016d0d663d-1"
export BSC_BNB_USD_PERP_ID="0xdb282bbace4e375ff2901b84aceb33016d0d663d-2"
export BSC_PRC_SERVER="https://bsc-dataseed.binance.org/"

# Arb1 graph url & inverse white list
export ARB_ONE_CHAIN=true
# export ARB_ONE_MAI3_GRAPH_URL=https://api.thegraph.com/subgraphs/name/mcdexio/mcdex3-arb-trade-mining2
export ARB_ONE_MAI3_GRAPH_URL=https://api.thegraph.com/subgraphs/name/champfu-mcdex/arb-mining-fee
# export ARB_ONE_BLOCK_GRAPH_URL=https://api.thegraph.com/subgraphs/name/ianlapham/arbitrum-one-blocks
export ARB_ONE_BLOCK_GRAPH_URL=https://graph-arb1.mcdex.io/subgraphs/name/ianlapham/arbitrum-one-blocks
export ARB_ONE_ETH_INVERSE_CONTRACT_WHITELIST0='0xc7b2ad78fded2bbc74b50dc1881ce0f81a7a0cca-0' # USD-ETH
export ARB_ONE_BTC_USD_PERP_ID="0xab324146c49b23658e5b3930e641bdbdf089cbac-1"
export ARB_ONE_ETH_USD_PERP_ID="0xab324146c49b23658e5b3930e641bdbdf089cbac-0"
export ARB_ONE_PRC_SERVER="https://arb1.arbitrum.io/rpc"
