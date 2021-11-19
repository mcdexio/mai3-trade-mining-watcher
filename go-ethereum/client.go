package go_ethereum

import (
	"context"
	"fmt"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
	"math/big"
	"time"
)

type Block struct {
	Timestamp   int64
	BlockNumber int64
}

type Client struct {
	client *ethclient.Client
	logger logging.Logger
	ctx    context.Context
	speed  int64
	url    string
}

func NewClient(logger logging.Logger, rpcURL string, ctx context.Context) (*Client, error) {
	if rpcURL == "" {
		return nil, fmt.Errorf("rpcURL is empty")
	}
	logger.Info("New client with rpcUrl=%s", rpcURL)
	c, err := ethclient.Dial(rpcURL)
	if err != nil {
		return nil, err
	}
	client := &Client{
		client: c,
		logger: logger,
		ctx:    ctx,
		url:    rpcURL,
	}
	client.calSpeed()
	return client, nil
}

func (c *Client) calSpeed() {
	block, err := c.GetLatestBlock()
	if err != nil {
		return
	}
	bn := block.BlockNumber
	tsSlow, err := c.GetTimestampWithBN(bn - 1000)
	if err != nil {
		return
	}
	speed := (block.Timestamp - tsSlow) / 1000
	c.speed = (c.speed + speed) / 2
}

func (c *Client) GetSigner() (*types.EIP155Signer, error) {
	ctx30, cancel := context.WithTimeout(c.ctx, 30*time.Second)
	defer cancel()
	chainID, err := c.client.ChainID(ctx30)
	if err != nil {
		return nil, fmt.Errorf("fail to get chain id err=%s", err)
	}
	signer := types.NewEIP155Signer(chainID)
	return &signer, nil
}

func (c *Client) GetFilterLogsByQuery(query ethereum.FilterQuery) (logs []types.Log, err error) {
	ctx30, cancel := context.WithTimeout(c.ctx, 30*time.Second)
	defer cancel()
	logs, err = c.client.FilterLogs(ctx30, query)
	if err != nil {
		c.logger.Error("fail to execute a filter query %+v err=%s", query, err)
		return
	}
	return
}

func (c *Client) GetTransactionByHash(txHash common.Hash) (*types.Transaction, error) {
	ctx30, cancel := context.WithTimeout(c.ctx, 30*time.Second)
	defer cancel()
	tx, isPending, err := c.client.TransactionByHash(ctx30, txHash)
	if err != nil {
		return nil, err
	}
	if isPending {
		return nil, fmt.Errorf("tx is pending")
	}
	return tx, nil
}

func (c *Client) GetReceiptFromTransaction(txHash common.Hash) (receipt *types.Receipt, err error) {
	ctx30, cancel := context.WithTimeout(c.ctx, 30*time.Second)
	defer cancel()
	receipt, err = c.client.TransactionReceipt(ctx30, txHash)
	if err != nil {
		return nil, err
	}
	if receipt == nil {
		return nil, fmt.Errorf("receipt is nil")
	}
	return receipt, nil
}

func (c *Client) GetLatestBlock() (*Block, error) {
	ctx30, cancel := context.WithTimeout(c.ctx, 30*time.Second)
	defer cancel()
	header, err := c.client.HeaderByNumber(ctx30, nil)
	if err != nil {
		return nil, fmt.Errorf("fail to get header err=%s", err)
	}
	return &Block{
		BlockNumber: header.Number.Int64(),
		Timestamp:   int64(header.Time),
	}, nil
}

func (c *Client) GetTimestampWithBN(blockNumber int64) (int64, error) {
	ctx30, cancel := context.WithTimeout(c.ctx, 30*time.Second)
	defer cancel()
	header, err := c.client.HeaderByNumber(ctx30, big.NewInt(blockNumber))
	if err != nil {
		return -1, fmt.Errorf("fail to get header err=%s", err)
	}
	return int64(header.Time), nil
}

func (c *Client) GetBlockNumberWithTS(timestamp int64) (int64, error) {
	var guessBN int64
	startTime := time.Now().Unix()
	defer func() {
		endTime := time.Now().Unix()
		c.logger.Info("leave GetBlockNumberWithTS of which @ts:%d, bn:%d, takes %d seconds: url %s", timestamp, guessBN, endTime-startTime, c.url)
	}()

	rightBlock, err := c.GetLatestBlock()
	if err != nil {
		return -1, err
	}
	rightBN := rightBlock.BlockNumber
	rightTS := rightBlock.Timestamp
	if rightTS < timestamp {
		return -1, fmt.Errorf("ts(%+d) is bigger then latest block %+v", timestamp, rightBlock)
	}
	if rightTS == timestamp {
		return rightBN, nil
	}
	tsDiff := rightTS - timestamp

	c.calSpeed()
	bnDiff := tsDiff / c.speed
	leftBN := rightBN - bnDiff
	guessBN = leftBN

	var guessTS int64
	count := 0
	for count < 30 {
		guessTS, err = c.GetTimestampWithBN(guessBN)
		if err != nil {
			return -1, err
		}
		if count%5 == 0 {
			c.logger.Debug("guessTS(%d), guessBN(%d), %+v hours ago", guessTS, guessBN, float64(startTime-guessTS)/60.0/60.0)
		}
		if guessTS == timestamp {
			return guessBN, nil
		} else if guessTS > timestamp {
			bnDiff = rightBN - guessBN
			leftBN = rightBN - 2*bnDiff
			rightBN = guessBN
		} else {
			// guessTS < timestamp
			leftBN = guessBN
		}
		guessBN = leftBN + (rightBN-leftBN)/2
		count++
	}
	c.logger.Info("return guessTS(%d), guessBN(%d), %d hours ago", guessTS, guessBN, (startTime-guessTS)/60.0/60.0)
	return guessBN, nil
}
