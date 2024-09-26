//go:build ignore

package main

import (
	"fmt"
	"os"

	"github.com/99designs/gqlgen/api"
	"github.com/99designs/gqlgen/codegen/config"
	"github.com/ajnavarro/gqlfiltergen"
)

var queriesInject = `
type Query {
   """
   EXPERIMENTAL: Fetches Blocks matching the specified where criteria. 
   Incomplete results due to errors return both the partial Blocks and 
   the associated errors.
   """
   getBlocks(where: FilterBlock!): [Block!]
   
   """
   EXPERIMENTAL: Retrieves a list of Transactions that match the given 
   where criteria. If the result is incomplete due to errors, both partial
   results and errors are returned.
   """
   getTransactions(where: FilterTransaction!): [Transaction!]
}

type Subscription {
  """
  EXPERIMENTAL: Subscribes to real-time updates of Transactions that 
  match the provided filter criteria. This subscription starts immediately
  and only includes Transactions added to the blockchain after the subscription
  is active.

  This is useful for applications needing to track Transactions in real-time, 
  such as wallets tracking incoming transactions or analytics platforms 
  monitoring blockchain activity.

  Returns:
  - Transaction: Each received update is a Transaction object that matches 
  the where criteria.
  """
  getTransactions(where: FilterTransaction!): Transaction!

  """
  EXPERIMENTAL: Subscribes to real-time updates of Blocks that match the provided
  filter criteria. Similar to the Transactions subscription,
  this subscription is active immediately upon creation and only includes Blocks
  added after the subscription begins.

  This subscription is ideal for services that need to be notified of new Blocks
  for processing or analysis, such as block explorers, data aggregators, or security
  monitoring tools.

  Returns:
  - Block: Each update consists of a Block object that satisfies the filter criteria,
  allowing subscribers to process or analyze new Blocks in real time.
  """
  getBlocks(where: FilterBlock!): Block!
}
`

func main() {
	cfg, err := config.LoadConfigFromDefaultLocations()
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to load config", err.Error())
		os.Exit(2)
	}

	err = api.Generate(cfg,
		api.AddPlugin(gqlfiltergen.NewPlugin(&gqlfiltergen.Options{
			InjectCodeAfter: queriesInject,
		})),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(3)
	}
}
