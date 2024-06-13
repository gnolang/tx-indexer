package graph

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.
// Code generated by github.com/99designs/gqlgen version v0.17.48

import (
	"context"

	"github.com/99designs/gqlgen/graphql"
	"github.com/gnolang/tx-indexer/serve/graph/model"
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

// Query returns QueryResolver implementation.
func (r *Resolver) Query() QueryResolver { return &queryResolver{r} }

type queryResolver struct{ *Resolver }
