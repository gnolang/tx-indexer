## Using tx-indexing filters

This example shows how you can use the tx-indexing service to perform a simple transaction filter query:
find transactions that cost over 5,000,000 gas within a chain.

*Note: This example indexes the test chain at `http://test3.gno.land:36657`, as used in the 
[tx-indexer Getting Started](https://github.com/gnolang/tx-indexer/blob/ae33bd64265d47f8c3871ac491d2ba60edb44e58/README.md#getting-started).*

The tx-indexer service provides a utility to index a specified chain, as well as an API to manage the indexing and to query the index data. The following diagram depicts this idea:

<div style="width:20%; margin:auto;">

![](tx-inx-ctx.png)

</div>

The tx-indexer service includes a graphql endpoint to query and retrieve the extracted index data.
This example shows you how to do this, by leading you through the following activities:

1. Install and start the tx-indexer service.
2. Query the API to find transactions that use > 5 million gas.
 
These activities are detailed in the following sections.

### Install and start the service

In a shell window, open and start the service as described in the following steps (these are similar to those in the
[Getting Started](https://github.com/gnolang/tx-indexer/blob/ae33bd64265d47f8c3871ac491d2ba60edb44e58/README.md#getting-started)).

1. **Clone the Repository**

```bash
git clone https://github.com/gnolang/tx-indexer.git 
```

2. **Build the binary**

```bash
cd tx-indexer
make build
```

3. **Run the indexer**

```bash
./build/tx-indexer start --remote http://test3.gno.land:36657 --db-path indexer-db
```

This starts up the tx-indexer service, indexing the `test3.gno.land` example chain. Leave this running while it indexes the existing contents of the chain; then leave it running to continue indexing new transactions as they are added.

### Use the service to request filtered transactions

With the tx-indexer running, you can make a request against the service's RPC or graphql endpoints. This example uses the graphql endpoint; it also assumes that the `jq` shell command is available in your environment.

**Step 1: create the query** &mdash; Create a JSON file named `request.json`, with the content shown below.

```
{
  "query": "query {
    transactions(
      filter: { from_gas_used: 5000000}
    ) {
      block_height
      hash
      gas_used
      messages {
        route
        typeUrl
        value {
          __typename
          ... on MsgAddPackage {
            creator
            package {
              name path
            }
          }
        }
      }
    }
  }"
}
```

**Step 2: Post the query** &mdash; Next, execute the following command to post the JSON request to the service's graphql endpoint:

```bash
curl -d @request.json --header "Content-Type: application/json" http://0.0.0.0:8546/graphql/query | jq 
```
*Note: The `jq` command  here is optional; it only formats the JSON response. If it isn't present on your system, you can either install it first or just omit it from the command.*

**Step 3: See the result** &mdash; The service should return output similar to the following:

![tx-indexer graphql filter](tx-i-filter.gif)

<!--
```
{
  "data": {
    "transactions": [
      {
        "block_height": 135249,
        "hash": "YFgFEz6NZJBDaVwLHZXWeDVjUjJQfvNUT+dnqoqDT3A=",
        "gas_used": 7496696,
        "messages": [
          {
            "route": "vm",
            "typeUrl": "add_package",
            "value": {
              "__typename": "MsgAddPackage",
              "creator": "g1juz2yxmdsa6audkp6ep9vfv80c8p5u76e03vvh",
              "package": {
                "name": "boards",
                "path": "gno.land/r/demo/jefft0_test1_boards"
              }
            }
          }
        ]
      },
      {
        "block_height": 136299,
        "hash": "oE/P0WiTrlnm6qVTHi0JF1LZ9JOPjSV6xyIAdtfSYQk=",
        "gas_used": 7496539,
        "messages": [
          {
            "route": "vm",
            "typeUrl": "add_package",
            "value": {
              "__typename": "MsgAddPackage",
              "creator": "g1juz2yxmdsa6audkp6ep9vfv80c8p5u76e03vvh",
              "package": {
                "name": "boards",
                "path": "gno.land/r/demo/jefft0_test2_boards"
              }
            }
          }
        ]
      }
    ]
  }
}
```

-->

This returned data lists the requested fields for all transactions that satisfy the filter; in this case, where gas is greater than 5,000,000.
