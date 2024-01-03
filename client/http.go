package client

import (
	"fmt"

	rpcClient "github.com/gnolang/gno/tm2/pkg/bft/rpc/client"
	core_types "github.com/gnolang/gno/tm2/pkg/bft/rpc/core/types"
	clientTypes "github.com/gnolang/tx-indexer/client/types"
)

// Client is the TM2 HTTP client
type Client struct {
	client *rpcClient.HTTP
}

// NewClient creates a new TM2 HTTP client
func NewClient(remote string) *Client {
	return &Client{
		client: rpcClient.NewHTTP(remote, ""),
	}
}

// CreateBatch creates a new request batch
func (c *Client) CreateBatch() clientTypes.Batch {
	return &Batch{
		batch: c.client.NewBatch(),
	}
}

func (c *Client) GetLatestBlockNumber() (int64, error) {
	status, err := c.client.Status()
	if err != nil {
		return 0, fmt.Errorf("unable to get chain status, %w", err)
	}

	return status.SyncInfo.LatestBlockHeight, nil
}

func (c *Client) GetBlock(blockNum int64) (*core_types.ResultBlock, error) {
	block, err := c.client.Block(&blockNum)
	if err != nil {
		return nil, fmt.Errorf("unable to get block, %w", err)
	}

	return block, nil
}

func (c *Client) GetBlockResults(blockNum int64) (*core_types.ResultBlockResults, error) {
	results, err := c.client.BlockResults(&blockNum)
	if err != nil {
		return nil, fmt.Errorf("unable to get block results, %w", err)
	}

	return results, nil
}
