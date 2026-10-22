package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	rpctypes "github.com/gnolang/gno/tm2/pkg/bft/rpc/lib/types"
	"github.com/gnolang/gno/tm2/pkg/sdk/bank"
	"github.com/stretchr/testify/require"
)

// mainnetBlockResults786 is the block_results result served by rpc.gno.land
// for gnoland-1 height 786: the first mainnet block whose transaction emitted
// a bank.TransferEvent, which the indexer must be able to decode.
const mainnetBlockResults786 = `{
  "height": "786",
  "results": {
    "deliver_tx": [
      {
        "ResponseBase": {
          "Error": null,
          "Data": null,
          "Events": [
            {
              "@type": "/bank.TransferEvent",
              "from": "g1qv3dqyw46fut94z9t90jka58saw2e7l99nzqtr",
              "to": "g1y7h659patawdy99mlufj9lp3t9cwpt8fq852zq",
              "coins": "1000000ugnot"
            }
          ],
          "Log": "msg:0,success:true,log:,events:[]",
          "Info": ""
        },
        "GasWanted": "1360839",
        "GasUsed": "1237175"
      }
    ],
    "end_block": {
      "ResponseBase": {
        "Error": null,
        "Data": null,
        "Events": null,
        "Log": "",
        "Info": ""
      },
      "ValidatorUpdates": null,
      "ConsensusParams": null,
      "Events": null
    },
    "begin_block": {
      "ResponseBase": {
        "Error": null,
        "Data": null,
        "Events": null,
        "Log": "",
        "Info": ""
      }
    }
  }
}`

// newRecordedRPCServer serves the given JSON-RPC result for every request and
// hands each request it received to the test through the returned channel.
// The response echoes the request ID, which the RPC client requires to match.
func newRecordedRPCServer(t *testing.T, result string) (*httptest.Server, <-chan rpctypes.RPCRequest) {
	t.Helper()

	requests := make(chan rpctypes.RPCRequest, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request rpctypes.RPCRequest

		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)

			return
		}

		select {
		case requests <- request:
		default:
			t.Errorf("unexpected extra request for method %q", request.Method)
		}

		response, err := json.Marshal(rpctypes.RPCResponse{
			JSONRPC: "2.0",
			ID:      request.ID,
			Result:  json.RawMessage(result),
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)

			return
		}

		w.Header().Set("Content-Type", "application/json")

		if _, err := w.Write(response); err != nil {
			t.Errorf("unable to write the recorded response, %v", err)
		}
	}))

	t.Cleanup(server.Close)

	return server, requests
}

func TestClient_GetBlockResults_DecodesBankTransferEvent(t *testing.T) {
	t.Parallel()

	server, requests := newRecordedRPCServer(t, mainnetBlockResults786)

	client, err := NewClient(server.URL)
	require.NoError(t, err)

	results, err := client.GetBlockResults(context.Background(), 786)
	require.NoError(t, err)

	request := <-requests
	require.Equal(t, "block_results", request.Method)
	require.JSONEq(t, `{"height":"786"}`, string(request.Params))

	require.Equal(t, int64(786), results.Height)
	require.Len(t, results.Results.DeliverTxs, 1)
	require.Len(t, results.Results.DeliverTxs[0].Events, 1)

	event, ok := results.Results.DeliverTxs[0].Events[0].(bank.TransferEvent)
	require.True(t, ok, "event is %T, want bank.TransferEvent", results.Results.DeliverTxs[0].Events[0])

	require.Equal(t, "g1qv3dqyw46fut94z9t90jka58saw2e7l99nzqtr", event.From)
	require.Equal(t, "g1y7h659patawdy99mlufj9lp3t9cwpt8fq852zq", event.To)
	require.Equal(t, "1000000ugnot", event.Coins.String())
}
