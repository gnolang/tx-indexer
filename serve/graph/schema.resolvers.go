package graph

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.
// Code generated by github.com/99designs/gqlgen version v0.17.43

import (
	"context"

	"github.com/99designs/gqlgen/graphql"
	"github.com/gnolang/tx-indexer/serve/graph/model"
	"github.com/gnolang/tx-indexer/types"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

// Transactions is the resolver for the transactions field.
func (r *queryResolver) Transactions(ctx context.Context, filter model.TransactionFilter) ([]*model.Transaction, error) {
	it, err := r.
		store.
		TxIterator(
			int64(dereferenceInt(filter.FromBlockHeight)),
			int64(dereferenceInt(filter.ToBlockHeight)),
			uint32(dereferenceInt(filter.FromIndex)),
			uint32(dereferenceInt(filter.ToIndex)),
		)
	if err != nil {
		return nil, gqlerror.Wrap(err)
	}
	defer it.Close()

	fgw := dereferenceInt(filter.FromGasUsed)
	tgw := dereferenceInt(filter.ToGasWanted)
	fgu := dereferenceInt(filter.FromGasUsed)
	tgu := dereferenceInt(filter.ToGasUsed)

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

			if !(t.Response.GasUsed >= int64(fgu) && (tgu == 0 || t.Response.GasUsed <= int64(tgu))) {
				continue
			}
			if !(t.Response.GasWanted >= int64(fgw) && (tgw == 0 || t.Response.GasWanted <= int64(tgw))) {
				continue
			}

			out = append(out, model.NewTransaction(t))
			i++
		}
	}
}

// Blocks is the resolver for the blocks field.
func (r *queryResolver) Blocks(ctx context.Context, filter model.BlockFilter) ([]*model.Block, error) {
	it, err := r.
		store.
		BlockIterator(
			int64(dereferenceInt(filter.FromHeight)),
			int64(dereferenceInt(filter.ToHeight)),
		)
	if err != nil {
		return nil, gqlerror.Wrap(err)
	}
	defer it.Close()

	dft := dereferenceTime(filter.FromTime)

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

			if !((b.Time.After(dft) || b.Time.Equal(dft)) && (filter.ToTime == nil || b.Time.Before(*filter.ToTime))) {
				continue
			}

			out = append(out, model.NewBlock(b))
			i++
		}
	}
}

// LatestBlockHeight is the resolver for the latestBlockHeight field.
func (r *queryResolver) LatestBlockHeight(ctx context.Context) (int, error) {
	h, err := r.store.GetLatestHeight()
	return int(h), err
}

// Transactions is the resolver for the transactions field.
func (r *subscriptionResolver) Transactions(ctx context.Context) (<-chan *model.Transaction, error) {
	return handleChannel[*model.Transaction](ctx, r.manager, func(nb *types.NewBlock, c chan *model.Transaction) {
		for _, tx := range nb.Results {
			c <- model.NewTransaction(tx)
		}
	}), nil
}

// Blocks is the resolver for the blocks field.
func (r *subscriptionResolver) Blocks(ctx context.Context) (<-chan *model.Block, error) {
	return handleChannel[*model.Block](ctx, r.manager, func(nb *types.NewBlock, c chan *model.Block) {
		c <- model.NewBlock(nb.Block)
	}), nil
}

// Query returns QueryResolver implementation.
func (r *Resolver) Query() QueryResolver { return &queryResolver{r} }

// Subscription returns SubscriptionResolver implementation.
func (r *Resolver) Subscription() SubscriptionResolver { return &subscriptionResolver{r} }

type queryResolver struct{ *Resolver }
type subscriptionResolver struct{ *Resolver }
