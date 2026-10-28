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
   Fetches Blocks matching the specified where criteria.
   Incomplete results due to errors return both the partial Blocks and
   the associated errors.

   At most 10000 Blocks are returned, whatever limit asks for. A result
   cut short at that hard cap carries an error whose extensions hold "code": "MAX_ELEMENTS_REACHED"
   and "lastCompleteBlockHeight", the height the caller received in full.
   Resume from it with a height greater than that value when ordering
   ascending, or less than it when ordering descending. Read the
   extensions rather than the message text.

   A server started with a query deadline (--query-deadline) answers a
   query that runs past it with the Blocks read before it and an error
   whose extensions hold "code": "QUERY_DEADLINE_EXCEEDED" and
   "queryDeadline", the configured deadline. Every Block returned is
   whole, so resume past the last height returned: greater than it when
   ordering ascending, less than it when ordering descending.
   """
   getBlocks(
      where: FilterBlock!
      order: BlockOrder
      """
      Returns the first limit matching Blocks in the requested order. It
      must be at least 1: a smaller value is an error. A value above the
      10000 cap is clamped to it, and the cap error then applies as without
      limit. Reaching limit is not a truncation, so it raises no error and
      names no lastCompleteBlockHeight. Every Block is whole, so a client
      paging with limit resumes past the last height returned.
      """
      limit: Int
   ): [Block!]

   """
   Retrieves a list of Transactions that match the given
   where criteria. If the result is incomplete due to errors, both partial
   results and errors are returned.

   At most 10000 Transactions are returned, whatever limit asks for. A
   result cut short at that hard cap ends on a block boundary, because a filter cannot express a
   resume point inside a block: its index window applies to every block
   in the range rather than to the boundary block alone. The error's
   extensions hold "code": "MAX_ELEMENTS_REACHED" and
   "lastCompleteBlockHeight", the height the caller received in full;
   resume with block_height greater than it when ordering ascending, or
   less than it when ordering descending. Where one block alone holds
   more Transactions than the cap no boundary exists, so the partial
   block is returned with "code": "BLOCK_EXCEEDS_MAX_ELEMENTS" and
   "blockHeight". Read the extensions rather than the message text.

   A server started with a query deadline (--query-deadline) answers a
   query that runs past it with no Transactions and an error whose
   extensions hold "code": "QUERY_DEADLINE_EXCEEDED" and
   "queryDeadline", the configured deadline. Narrow the block_height
   range and retry.
   """
   getTransactions(
      where: FilterTransaction!
      order: TransactionOrder
      """
      Returns the first limit matching Transactions in the requested order.
      It must be at least 1: a smaller value is an error. A value above the
      10000 cap is clamped to it, and the cap error then applies as without
      limit. Reaching limit is not a truncation, so it raises no error and
      names no lastCompleteBlockHeight. A page cut by limit can end inside a
      block, so it is not a resume point: resuming with block_height past
      its last row can skip the rest of that block.
      """
      limit: Int
   ): [Transaction!]
}

type Subscription {
  """
  Subscribes to real-time updates of Transactions that
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
  Subscribes to real-time updates of Blocks that match the provided
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
