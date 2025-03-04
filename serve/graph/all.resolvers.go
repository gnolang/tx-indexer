package graph

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.
// Code generated by github.com/99designs/gqlgen version v0.17.56

import (
	"context"

	"github.com/99designs/gqlgen/graphql"
	bfttypes "github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/gnolang/tx-indexer/serve/graph/model"
	"github.com/gnolang/tx-indexer/storage"
	"github.com/gnolang/tx-indexer/types"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

// Transactions is the resolver for the transactions field.
func (r *queryResolver) Transactions(ctx context.Context, filter model.TransactionFilter) ([]*model.Transaction, error) {
	if filter.Hash != nil {
		tx, err := r.store.GetTxByHash(*filter.Hash)
		if err != nil {
			return nil, gqlerror.Wrap(err)
		}
		return []*model.Transaction{model.NewTransaction(tx)}, nil
	}

	it, err := r.
		store.
		TxIterator(
			uint64(deref(filter.FromBlockHeight)),
			uint64(deref(filter.ToBlockHeight)),
			uint32(deref(filter.FromIndex)),
			uint32(deref(filter.ToIndex)),
		)
	if err != nil {
		return nil, gqlerror.Wrap(err)
	}
	defer it.Close()

	var out []*model.Transaction
	i := 0
	for {
		if i == maxElementsPerQuery {
			graphql.AddErrorf(ctx, "max elements per query reached (%d)", maxElementsPerQuery)
			return out, nil
		}

		if !it.Next() {
			return out, it.Error()
		}

		select {
		case <-ctx.Done():
			graphql.AddError(ctx, ctx.Err())
			return out, nil
		default:
			t, err := it.Value()
			if err != nil {
				graphql.AddError(ctx, err)
				return out, nil
			}

			transaction := model.NewTransaction(t)
			if !FilteredTransactionBy(transaction, filter) {
				continue
			}
			out = append(out, transaction)
			i++
		}
	}
}

// Blocks is the resolver for the blocks field.
func (r *queryResolver) Blocks(ctx context.Context, filter model.BlockFilter) ([]*model.Block, error) {
	it, err := r.
		store.
		BlockIterator(
			uint64(deref(filter.FromHeight)),
			uint64(deref(filter.ToHeight)),
		)
	if err != nil {
		return nil, gqlerror.Wrap(err)
	}
	defer it.Close()

	var out []*model.Block

	i := 0
	for {
		if i == maxElementsPerQuery {
			graphql.AddErrorf(ctx, "max elements per query reached (%d)", maxElementsPerQuery)
			return out, nil
		}

		if !it.Next() {
			return out, it.Error()
		}

		select {
		case <-ctx.Done():
			graphql.AddError(ctx, ctx.Err())
			return out, nil
		default:
			b, err := it.Value()
			if err != nil {
				graphql.AddError(ctx, err)
				return out, nil
			}

			block := model.NewBlock(b)
			if !FilteredBlockBy(block, filter) {
				continue
			}

			out = append(out, block)
			i++
		}
	}
}

// LatestBlockHeight is the resolver for the latestBlockHeight field.
func (r *queryResolver) LatestBlockHeight(ctx context.Context) (int, error) {
	h, err := r.store.GetLatestHeight()
	return int(h), err
}

// GetBlocks is the resolver for the getBlocks field.
func (r *queryResolver) GetBlocks(ctx context.Context, where model.FilterBlock, order *model.BlockOrder) ([]*model.Block, error) {
	fromh, toh := where.MinMaxHeight()
	dfromh := uint64(deref(fromh))
	dtoh := uint64(deref(toh))
	if fromh == toh && toh != nil {
		// min element and max element are the same,
		// so we only need to iterate over one element
		dtoh++
	}

	var err error
	var it storage.Iterator[*bfttypes.Block]
	if order != nil && order.Height == model.OrderDesc {
		it, err = r.
			store.
			BlockReverseIterator(
				dfromh,
				dtoh,
			)
	} else {
		it, err = r.
			store.
			BlockIterator(
				dfromh,
				dtoh,
			)
	}

	if err != nil {
		return nil, gqlerror.Wrap(err)
	}
	defer it.Close()

	var out []*model.Block

	i := 0
	for {
		if i == maxElementsPerQuery {
			graphql.AddErrorf(ctx, "max elements per query reached (%d)", maxElementsPerQuery)
			return out, nil
		}

		if !it.Next() {
			return out, it.Error()
		}

		select {
		case <-ctx.Done():
			graphql.AddError(ctx, ctx.Err())
			return out, nil
		default:
			b, err := it.Value()
			if err != nil {
				graphql.AddError(ctx, err)
				return out, nil
			}

			block := model.NewBlock(b)

			if !where.Eval(block) {
				continue
			}

			out = append(out, block)
			i++
		}
	}
}

// GetTransactions is the resolver for the getTransactions field.
func (r *queryResolver) GetTransactions(ctx context.Context, where model.FilterTransaction, order *model.TransactionOrder) ([]*model.Transaction, error) {
	// corner case
	if where.Hash != nil &&
		where.Hash.Eq != nil &&
		len(where.Or) == 0 {
		tx, err := r.store.GetTxByHash(*where.Hash.Eq)
		if err != nil {
			return nil, gqlerror.Wrap(err)
		}

		otx := model.NewTransaction(tx)

		// evaluate just in case the user is using any other filter than Eq
		if !where.Eval(otx) {
			return nil, nil
		}

		return []*model.Transaction{otx}, nil
	}

	fromh, toh := where.MinMaxBlockHeight()
	dfromh := uint64(deref(fromh))
	dtoh := uint64(deref(toh))
	if fromh == toh && toh != nil {
		// min element and max element are the same,
		// so we only need to iterate over one element
		dtoh++
	}

	fromi, toi := where.MinMaxIndex()
	dfromi := uint32(deref(fromi))
	dtoi := uint32(deref(toi))
	if fromi == toi && toi != nil {
		// min element and max element are the same,
		// so we only need to iterate over one element
		dtoi++
	}

	var err error
	var it storage.Iterator[*bfttypes.TxResult]
	if order != nil && order.HeightAndIndex == model.OrderDesc {
		it, err = r.
			store.
			TxReverseIterator(
				dfromh,
				dtoh,
				dfromi,
				dtoi,
			)
	} else {
		it, err = r.
			store.
			TxIterator(
				dfromh,
				dtoh,
				dfromi,
				dtoi,
			)
	}

	if err != nil {
		return nil, gqlerror.Wrap(err)
	}
	defer it.Close()

	var out []*model.Transaction
	i := 0
	for {
		if i == maxElementsPerQuery {
			graphql.AddErrorf(ctx, "max elements per query reached (%d)", maxElementsPerQuery)
			return out, nil
		}

		if !it.Next() {
			return out, it.Error()
		}

		select {
		case <-ctx.Done():
			graphql.AddError(ctx, ctx.Err())
			return out, nil
		default:
			t, err := it.Value()
			if err != nil {
				graphql.AddError(ctx, err)
				return out, nil
			}

			transaction := model.NewTransaction(t)

			if !where.Eval(transaction) {
				continue
			}
			out = append(out, transaction)
			i++
		}
	}
}

// Transactions is the resolver for the transactions field.
func (r *subscriptionResolver) Transactions(ctx context.Context, filter model.TransactionFilter) (<-chan *model.Transaction, error) {
	return handleChannel(ctx, r.manager, func(nb *types.NewBlock, c chan<- *model.Transaction) {
		for _, tx := range nb.Results {
			transaction := model.NewTransaction(tx)
			if FilteredTransactionBy(transaction, filter) {
				c <- transaction
			}
		}
	}), nil
}

// Blocks is the resolver for the blocks field.
func (r *subscriptionResolver) Blocks(ctx context.Context, filter model.BlockFilter) (<-chan *model.Block, error) {
	return handleChannel(ctx, r.manager, func(nb *types.NewBlock, c chan<- *model.Block) {
		block := model.NewBlock(nb.Block)
		if FilteredBlockBy(block, filter) {
			c <- block
		}
	}), nil
}

// GetTransactions is the resolver for the getTransactions field.
func (r *subscriptionResolver) GetTransactions(ctx context.Context, where model.FilterTransaction) (<-chan *model.Transaction, error) {
	return handleChannel(ctx, r.manager, func(nb *types.NewBlock, c chan<- *model.Transaction) {
		for _, tx := range nb.Results {
			transaction := model.NewTransaction(tx)
			if where.Eval(transaction) {
				c <- transaction
			}
		}
	}), nil
}

// GetBlocks is the resolver for the getBlocks field.
func (r *subscriptionResolver) GetBlocks(ctx context.Context, where model.FilterBlock) (<-chan *model.Block, error) {
	return handleChannel(ctx, r.manager, func(nb *types.NewBlock, c chan<- *model.Block) {
		block := model.NewBlock(nb.Block)
		if where.Eval(block) {
			c <- block
		}
	}), nil
}

// Query returns QueryResolver implementation.
func (r *Resolver) Query() QueryResolver { return &queryResolver{r} }

// Subscription returns SubscriptionResolver implementation.
func (r *Resolver) Subscription() SubscriptionResolver { return &subscriptionResolver{r} }

type queryResolver struct{ *Resolver }
type subscriptionResolver struct{ *Resolver }
