package graph

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/99designs/gqlgen/client"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2/gqlerror"

	"github.com/gnolang/tx-indexer/events"
	"github.com/gnolang/tx-indexer/serve/deadline"
	"github.com/gnolang/tx-indexer/serve/graph/model"
	"github.com/gnolang/tx-indexer/storage"
	indexerTypes "github.com/gnolang/tx-indexer/types"
)

const (
	// testDeadline is the query deadline the tests configure
	testDeadline = 50 * time.Millisecond

	// slowRows and slowRowDelay make a scan that outlasts testDeadline
	// twenty times over, while staying under maxElementsPerQuery
	slowRows     = 1_000
	slowRowDelay = time.Millisecond

	// insideRows is a scan that ends 10 ms inside testDeadline
	insideRows = 40
)

// slowStore scans rows rows in every range, each taking slowRowDelay to read.
// Point reads are left to the nil embedded Storage: the deadline tests only
// scan.
type slowStore struct {
	storage.Storage
	rows int64
}

func (s *slowStore) BlockIterator(uint64, uint64) (storage.Iterator[*types.Block], error) {
	return &slowIterator[*types.Block]{value: slowBlock, rows: s.rows}, nil
}

func (s *slowStore) BlockReverseIterator(uint64, uint64) (storage.Iterator[*types.Block], error) {
	return &slowIterator[*types.Block]{value: slowBlock, rows: s.rows}, nil
}

func (s *slowStore) TxIterator(uint64, uint64, uint32, uint32) (storage.Iterator[*types.TxResult], error) {
	return &slowIterator[*types.TxResult]{value: slowTx, rows: s.rows}, nil
}

func (s *slowStore) TxReverseIterator(uint64, uint64, uint32, uint32) (storage.Iterator[*types.TxResult], error) {
	return &slowIterator[*types.TxResult]{value: slowTx, rows: s.rows}, nil
}

func slowBlock(height int64) *types.Block {
	return &types.Block{Header: types.Header{Height: height}}
}

func slowTx(height int64) *types.TxResult {
	return &types.TxResult{Height: height}
}

// slowIterator yields rows rows, sleeping slowRowDelay before each
type slowIterator[T any] struct {
	value func(height int64) T
	rows  int64
	read  int64
}

func (it *slowIterator[T]) Next(context.Context) bool {
	time.Sleep(slowRowDelay)

	it.read++

	return it.read <= it.rows
}

func (it *slowIterator[T]) Value() (T, error) {
	return it.value(it.read), nil
}

func (it *slowIterator[T]) Error() error {
	return nil
}

func (it *slowIterator[T]) Close() error {
	return nil
}

// graphResponse is a GraphQL response as a client reads it
type graphResponse struct {
	Data   map[string]json.RawMessage `json:"data"`
	Errors []struct {
		Extensions map[string]any `json:"extensions"`
		Message    string         `json:"message"`
		Path       []any          `json:"path"`
	} `json:"errors"`
}

// postQuery sends query to the GraphQL endpoint of mux, as a client would
func postQuery(t *testing.T, mux http.Handler, query string) graphResponse {
	t.Helper()

	body, err := json.Marshal(map[string]string{"query": query})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/graphql/query", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp graphResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp), rec.Body.String())

	return resp
}

// listQueries are the queries that scan the storage, keyed by their field
var listQueries = map[string]string{
	"getTransactions": `{ getTransactions(where: {}) { block_height } }`,
	"getBlocks":       `{ getBlocks(where: {}) { height } }`,
	"transactions":    `{ transactions(filter: {}) { block_height } }`,
	"blocks":          `{ blocks(filter: {}) { height } }`,
}

// blockLists are the list queries whose rows are whole blocks
var blockLists = map[string]bool{"blocks": true, "getBlocks": true}

// A scan that runs past the deadline is stopped at the deadline, not before,
// and answers an error naming the deadline. A block list keeps the blocks it
// read before the deadline: a block row is a whole block, so the client
// resumes from the last height. A transaction list answers no rows: they can
// end inside a block and say nothing of how far the scan got.
func TestQueryDeadline_StopsAScanWithANamedError(t *testing.T) {
	t.Parallel()

	for field, query := range listQueries {
		t.Run(field, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				mux := Setup(&slowStore{rows: slowRows}, events.NewManager(), nil, chi.NewMux(), false, testDeadline)

				start := time.Now()
				resp := postQuery(t, mux, query)

				// The deadline lands between two rows, so the scan stops on
				// the first check after it
				elapsed := time.Since(start)
				assert.GreaterOrEqual(t, elapsed, testDeadline, "the scan stopped before the deadline")
				assert.LessOrEqual(t, elapsed, testDeadline+slowRowDelay, "the scan ran past the deadline")

				if blockLists[field] {
					var blocks []struct {
						Height int64 `json:"height"`
					}

					require.NoError(t, json.Unmarshal(resp.Data[field], &blocks))

					// Every block read before the deadline, from the first:
					// the last one lands on either side of it
					read := int(testDeadline / slowRowDelay)
					assert.GreaterOrEqual(t, len(blocks), read-1)
					assert.LessOrEqual(t, len(blocks), read)

					for i, block := range blocks {
						require.Equal(t, int64(i+1), block.Height, "block %d", i)
					}
				} else {
					answered := string(resp.Data[field])
					assert.Truef(t, answered == "null", "answered rows: %.60s", answered)
				}

				require.Len(t, resp.Errors, 1)

				gqlErr := resp.Errors[0]
				assert.Equal(t, "query deadline exceeded (50ms); narrow the block height range", gqlErr.Message)
				assert.Equal(t, []any{field}, gqlErr.Path)
				assert.Equal(t, "QUERY_DEADLINE_EXCEEDED", gqlErr.Extensions["code"])
				assert.Equal(t, "50ms", gqlErr.Extensions["queryDeadline"])
			})
		})
	}
}

// A query sent over the WebSocket transport is bounded like one sent over
// HTTP: the deadline follows the operation, not the transport
func TestQueryDeadline_BoundsAQueryOverWebSocket(t *testing.T) {
	t.Parallel()

	mux := Setup(&slowStore{rows: slowRows}, events.NewManager(), nil, chi.NewMux(), false, testDeadline)

	var resp map[string]any

	err := client.New(mux, client.Path("/graphql/query")).
		WebsocketOnce(listQueries["getTransactions"], &resp)

	var answered client.RawJsonError

	require.ErrorAs(t, err, &answered)
	assert.Contains(t, answered.Error(), "QUERY_DEADLINE_EXCEEDED")
}

// A scan that ends inside the deadline answers in full
func TestQueryDeadline_LeavesAScanInsideItWhole(t *testing.T) {
	t.Parallel()

	for field, query := range listQueries {
		t.Run(field, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				mux := Setup(&slowStore{rows: insideRows}, events.NewManager(), nil, chi.NewMux(), false, testDeadline)

				resp := postQuery(t, mux, query)
				assert.Empty(t, resp.Errors)

				var rows []json.RawMessage
				require.NoError(t, json.Unmarshal(resp.Data[field], &rows))
				assert.Len(t, rows, insideRows)
			})
		})
	}
}

// A deadline of zero is the default, and leaves every scan to run to its end
func TestQueryDeadline_ZeroLeavesScansUnbounded(t *testing.T) {
	t.Parallel()

	for field, query := range listQueries {
		t.Run(field, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				mux := Setup(&slowStore{rows: slowRows}, events.NewManager(), nil, chi.NewMux(), false, 0)

				resp := postQuery(t, mux, query)
				assert.Empty(t, resp.Errors)

				var rows []json.RawMessage
				require.NoError(t, json.Unmarshal(resp.Data[field], &rows))
				assert.Len(t, rows, slowRows)
			})
		})
	}
}

// A subscription lives as long as its connection, so the deadline must not
// close it: one older than the deadline still delivers the next block
func TestQueryDeadline_SubscriptionOutlivesIt(t *testing.T) {
	t.Parallel()

	manager := events.NewManager()
	mux := Setup(&slowStore{rows: slowRows}, manager, nil, chi.NewMux(), false, testDeadline)

	sub := client.New(mux, client.Path("/graphql/query")).
		Websocket(`subscription { getBlocks(where: {}) { height } }`)

	defer func() {
		assert.NoError(t, sub.Close())
	}()

	time.Sleep(3 * testDeadline)

	// The subscription registers with the event manager asynchronously, so
	// keep announcing blocks until one arrives
	stop := make(chan struct{})
	defer close(stop)

	go func() {
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()

		for height := int64(1); ; height++ {
			select {
			case <-stop:
				return
			case <-ticker.C:
				manager.SignalEvent(&indexerTypes.NewBlock{Block: slowBlock(height)})
			}
		}
	}()

	var resp struct {
		GetBlocks struct {
			Height int64 `json:"height"`
		} `json:"getBlocks"`
	}

	require.NoError(t, sub.Next(&resp))
	assert.Positive(t, resp.GetBlocks.Height)
}

// A request whose body does not decode never names an operation, and gqlgen
// still runs it through the deadline extension to answer the error
func TestQueryDeadline_UndecodableRequestIsAnswered(t *testing.T) {
	t.Parallel()

	mux := Setup(&slowStore{rows: slowRows}, events.NewManager(), nil, chi.NewMux(), false, testDeadline)

	req := httptest.NewRequest(http.MethodPost, "/graphql/query", strings.NewReader(`{"query":`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp graphResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp), rec.Body.String())

	require.Len(t, resp.Errors, 1)
	assert.Contains(t, resp.Errors[0].Message, "json request body could not be decoded")
}

// A scan whose client has left answers the cancellation as it is: only the
// deadline names itself
func TestQueryDeadline_ClientLeftIsNotADeadline(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(testContext())
	cancel()

	r := NewResolver(&slowStore{rows: slowRows}, nil, nil)

	out, err := r.Query().GetTransactions(ctx, model.FilterTransaction{}, nil)

	assert.Nil(t, out)
	require.ErrorIs(t, err, context.Canceled)
	assert.NotContains(t, err.Error(), "query deadline")
}

// A narrow index window makes the storage walk past every transaction outside
// it before it yields one, out of the resolver's reach between rows. The
// deadline stops that walk too, and the scan answers it named.
func TestQueryDeadline_StopsAnIndexWindowWalk(t *testing.T) {
	t.Parallel()

	r := testResolver(t)

	// Every transaction at index 0, outside a window from index 5
	seedTxs(t, r, 1_000, 1)

	fromIndex := 5

	testTable := []struct {
		scan func(context.Context) ([]*model.Transaction, error)
		name string
	}{
		{
			func(ctx context.Context) ([]*model.Transaction, error) {
				return r.Query().GetTransactions(
					ctx,
					model.FilterTransaction{Index: &model.FilterInt{Gt: &fromIndex}},
					nil,
				)
			},
			"getTransactions",
		},
		{
			func(ctx context.Context) ([]*model.Transaction, error) {
				return r.Query().GetTransactions(
					ctx,
					model.FilterTransaction{Index: &model.FilterInt{Gt: &fromIndex}},
					&model.TransactionOrder{HeightAndIndex: model.OrderDesc},
				)
			},
			"getTransactions descending",
		},
		{
			func(ctx context.Context) ([]*model.Transaction, error) {
				return r.Query().Transactions(ctx, model.TransactionFilter{FromIndex: &fromIndex})
			},
			"transactions",
		},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := deadline.WithTimeout(testContext(), time.Nanosecond)
			defer cancel()

			<-ctx.Done()

			out, err := testCase.scan(ctx)
			assert.Nil(t, out)

			var gqlErr *gqlerror.Error

			require.ErrorAs(t, err, &gqlErr)
			assert.Equal(t, "QUERY_DEADLINE_EXCEEDED", gqlErr.Extensions["code"])
		})
	}
}
