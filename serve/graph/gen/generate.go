// go:build ignore

package main

import (
	"fmt"
	"os"

	"github.com/99designs/gqlgen/api"
	"github.com/99designs/gqlgen/codegen/config"
	"github.com/ajnavarro/gqlfiltergen"
)

func main() {
	cfg, err := config.LoadConfigFromDefaultLocations()
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to load config", err.Error())
		os.Exit(2)
	}

	err = api.Generate(cfg,
		api.AddPlugin(gqlfiltergen.NewPlugin(&gqlfiltergen.Options{
			Queries: []string{
				`
   """
   Fetches Blocks matching the specified where criteria. Incomplete results due to errors return both the partial Blocks and the associated errors.
   """
   getBlocks(where: FilterBlock!): [Block!]
`, `
   """
   Retrieves a list of Transactions that match the given where criteria. If the result is incomplete due to errors, both partial results and errors are returned.
   """
   getTransactions(where: FilterTransaction!): [Transaction!]
				`,
			},
		})),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(3)
	}
}
