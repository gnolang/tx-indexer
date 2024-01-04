package encode

import (
	"encoding/base64"
	"fmt"

	"github.com/gnolang/gno/tm2/pkg/amino"
)

// EncodeValue encodes the given value into Amino binary, and then to base64.
//
// The optimal route for all responses served by this indexer implementation would
// be to use plain ol' JSON.
// JSON is nice.
// json.Marshal works.
//
// However, this doesn't work with some (any, really) TM2 types (block, tx...),
// which means a top-level Amino encoder needs to be present when serving the response.
// This opens the pandora's box for another problem: custom type registration.
// In Amino, any custom type needs to be registered using a clunky API, meaning all types need to be
// accounted for beforehand, which is not something that is worth the effort of doing,
// considering there are easier ways to pass around data (remember plain ol' JSON?).
// So, we arrive at this imperfect solution: TM2 types are encoded using Amino binary, and then using base64,
// and as such are passed to the client for processing
func EncodeValue(value any) (string, error) {
	aminoEncoding, err := amino.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("unable to amino encode value")
	}

	return base64.StdEncoding.EncodeToString(aminoEncoding), nil
}
