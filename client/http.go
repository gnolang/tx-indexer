package client

import (
	"context"
	"fmt"

	rpcClient "github.com/gnolang/gno/tm2/pkg/bft/rpc/client"
	core_types "github.com/gnolang/gno/tm2/pkg/bft/rpc/core/types"

	clientTypes "github.com/gnolang/tx-indexer/client/types"
)

// Client is the TM2 HTTP client
type Client struct {
	client *rpcClient.RPCClient
}

// NewClient creates a new TM2 HTTP client
func NewClient(remote string) (*Client, error) {
	client, err := rpcClient.NewHTTPClient(remote)
	if err != nil {
		return nil, fmt.Errorf("unable to create HTTP client, %w", err)
	}

	return &Client{
		client: client,
	}, nil
}

// CreateBatch creates a new request batch
func (c *Client) CreateBatch() clientTypes.Batch {
	return &Batch{
		batch: c.client.NewBatch(),
	}
}

func (c *Client) GetLatestBlockNumber(ctx context.Context) (uint64, error) {
	status, err := c.client.Status(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("unable to get chain status, %w", err)
	}

	return uint64(status.SyncInfo.LatestBlockHeight), nil
}

func (c *Client) GetBlock(ctx context.Context, blockNum uint64) (*core_types.ResultBlock, error) {
	bn := int64(blockNum)

	block, err := c.client.Block(ctx, &bn)
	if err != nil {
		return nil, fmt.Errorf("unable to get block, %w", err)
	}

	return block, nil
}

func (c *Client) GetGenesis(ctx context.Context) (*core_types.ResultGenesis, error) {
	genesis, err := c.client.Genesis(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to get genesis block, %w", err)
	}

	return genesis, nil
}

func (c *Client) GetBlockResults(ctx context.Context, blockNum uint64) (*core_types.ResultBlockResults, error) {
	bn := int64(blockNum)

	results, err := c.client.BlockResults(ctx, &bn)
	if err != nil {
		return nil, fmt.Errorf("unable to get block results, %w", err)
	}

	return results, nil
}

// GetStatus returns the current chain status, including the latest
// block height and block time.
func (c *Client) GetStatus(ctx context.Context) (*core_types.ResultStatus, error) {
	status, err := c.client.Status(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("unable to get chain status, %w", err)
	}

	return status, nil
}

// ABCIQuery runs an ABCI query against the chain at the latest height.
func (c *Client) ABCIQuery(ctx context.Context, path string, data []byte) (*core_types.ResultABCIQuery, error) {
	res, err := c.client.ABCIQuery(ctx, path, data)
	if err != nil {
		return nil, fmt.Errorf("unable to run ABCI query %q, %w", path, err)
	}

	return res, nil
}
