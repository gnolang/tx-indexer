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

// ABCIQueryBatchAtHeight runs every path as an ABCI query at exactly the
// given height, in one batched round trip. Results are returned in path
// order, so a set of reads can be taken from one consistent chain state.
func (c *Client) ABCIQueryBatchAtHeight(
	ctx context.Context,
	height int64,
	paths []string,
) ([]*core_types.ResultABCIQuery, error) {
	batch := c.client.NewBatch()

	for _, path := range paths {
		if err := batch.ABCIQueryWithOptions(path, nil, rpcClient.ABCIQueryOptions{Height: height}); err != nil {
			return nil, fmt.Errorf("unable to queue ABCI query %q, %w", path, err)
		}
	}

	results, err := batch.Send(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to send ABCI query batch, %w", err)
	}

	queries := make([]*core_types.ResultABCIQuery, len(results))

	for i, result := range results {
		query, ok := result.(*core_types.ResultABCIQuery)
		if !ok {
			return nil, fmt.Errorf("unexpected batch result type %T at index %d", result, i)
		}

		queries[i] = query
	}

	return queries, nil
}
