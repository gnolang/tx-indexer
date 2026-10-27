<h2 align="center">⚛️ Tendermint2 Indexer ⚛️</h2>

- [Overview](#overview)
- [Key Features](#key-features)
- [Getting Started](#getting-started)
  - [Genesis bootstrap](#genesis-bootstrap)
- [GraphQL Endpoint](#graphql-endpoint)
  - [Hosted Example](#hosted-example)
  - [Examples](#examples)
    - [Get all Transactions with add\_package messages. Show the creator, package name and path.](#get-all-transactions-with-add_package-messages-show-the-creator-package-name-and-path)
    - [Subscribe to get all new blocks in real-time](#subscribe-to-get-all-new-blocks-in-real-time)
    - [Get the supply of a denomination](#get-the-supply-of-a-denomination)
- [RPC Endpoints](#rpc-endpoints)
  - [Block Endpoints](#block-endpoints)
    - [`getBlock`](#getblock)
  - [Transaction Endpoints](#transaction-endpoints)
    - [`getTxResult`](#gettxresult)
  - [Supply Endpoints](#supply-endpoints)
    - [`getSupply`](#getsupply)
  - [Filter Endpoints](#filter-endpoints)
    - [`newBlockFilter`](#newblockfilter)
    - [`getFilterChanges`](#getfilterchanges)
    - [`uninstallFilter`](#uninstallfilter)
    - [`subscribe`](#subscribe)
    - [`unsubscribe`](#unsubscribe)

## Overview

`tx-indexer` is a tool designed to index TM2 chain data and serve it over RPC, facilitating efficient data retrieval and
management in Tendermint2 networks.

## Key Features

- **JSON-RPC 2.0 Specification Server**: Utilizes the [JSON-RPC 2.0 standard](https://www.jsonrpc.org/specification) for
  request / response handling.
- **HTTP and WS Support**: Handles both HTTP `POST` requests and WebSocket connections.
- **2-Way WS Communication**: Subscribe and receive data updates asynchronously over WebSocket connections.
- **Concurrent Chain Indexing**: Utilizes asynchronous workers for fast and efficient indexing. Data is available for
  serving as soon as it is fetched from the remote chain.
- **Embedded Database**: Features PebbleDB for quick on-disk data access and migration.

## Getting Started

This section guides you through setting up and running the `tx-indexer`.

1. **Clone the Repository**

```shell
git clone https://github.com/gnolang/tx-indexer.git
```

2. **Build the binary**

```bash
cd tx-indexer
make build
```

3. **Run the indexer**

```bash
./build/tx-indexer start --remote https://rpc.gno.land:443 --db-path indexer-db
```

or:

```bash
go run cmd/main.go cmd/start.go cmd/waiter.go start --remote https://rpc.gno.land:443 --db-path indexer-db
```

The `--remote` flag specifies the JSON-RPC URL of the chain the indexer should index, and the `--db-path` specifies the
on-disk location for the indexed data.

Point `--remote` at whichever chain you want to index. `https://rpc.gno.land:443` is
mainnet (`gnoland-1`); testnets are renamed and replaced every few weeks, so look the
current one up in [Gno networks](https://docs.gno.land/testnets)
rather than hardcoding a hostname.

**Note**: the websocket endpoint exposed is always: `ws://<listen-address>/ws`, where `<listen-address>` is set via the `--listen-address` flag when starting the indexer (default: `0.0.0.0:8546`).

For a full list of available features and flags, execute the `--help` command:

```shell
> ./build/tx-indexer start --help

DESCRIPTION
  Starts the indexer service

USAGE
  start [flags]

Starts the indexer service, which includes the fetcher and JSON-RPC server

FLAGS
  -client-ip-header string        the header trusted to carry the client address the request log names, such as CF-Connecting-IP or X-Forwarded-For; unset, the address the request came from
  -client-ip-trusted-hops 1       the number of trusted proxies appending to --client-ip-header: the client address is that many entries from the right of its list
  -cors-allowed-origins *         a comma-separated list of origins allowed to make cross-origin requests to the GraphQL and JSON-RPC endpoints, or "*" to allow any origin
  -db-path indexer-db             the absolute path for the indexer DB (embedded)
  -disable-introspection=false    disable GraphQL introspection queries if needed. This will cause malfunctions when using the GraphQL playground
  -http-rate-limit 0              the maximum HTTP requests allowed per minute per IP, unlimited by default
  -listen-address 0.0.0.0:8546    the IP:PORT URL for the indexer JSON-RPC server
  -log-level info                 the log level for the CLI output
  -max-chunk-size 100             the range for fetching blockchain data by a single worker
  -max-slots 100                  the amount of slots (workers) the fetcher employs
  -metrics-listen-address string  the IP:PORT address to serve the Prometheus metrics on, at /metrics, apart from the public endpoints; off by default
  -query-deadline 0s              the maximum time a GraphQL or JSON-RPC request may run before it is answered with a deadline error, unlimited by default; subscriptions are exempt
  -remote http://127.0.0.1:26657  the JSON-RPC URL of the Gno chain
  -request-log=false              log one line per GraphQL root field or JSON-RPC call served, at info: its operation, status, duration, rows and client address, never its arguments
  -supply-denoms ugnot            comma-separated denominations whose supply (total/spendable/locked) is tracked and served by getSupply
```

### Genesis bootstrap

Before any service starts, the indexer loads the chain genesis into its database: the genesis block, so indexing can
start from height 0, and the genesis balances, where the vesting schedules the supply endpoint needs live. This
happens **once per database** — every later start reads what it needs from the DB and never fetches the genesis
document again.

Two consequences worth knowing when running it:

- **The indexer will not start if it cannot bootstrap the genesis.** It retries while the node is unreachable, and
  fails outright against a node that does not serve a readable gno genesis. Earlier versions logged the failure and
  indexed anyway, at the cost of a missing block 0.
- **A database is bound to its chain.** The chain ID is stored alongside the genesis and checked against the node on
  every start, so pointing `--remote` at a different network while reusing `--db-path` refuses to start rather than
  mixing two chains' data. Use a separate `--db-path` per chain.

### Query deadline

`--query-deadline` bounds how long one GraphQL or JSON-RPC request may run. A list query filters rows as it reads
them, and its 10000 element cap counts the rows it returns rather than the rows it reads, so a filter that matches
little can read the whole chain on every call. The deadline is off (`0s`) by default; set it to a Go duration such as
`10s`.

A transaction list (`getTransactions`, `transactions`) that runs past it answers no rows and an error naming it: the
rows read so far can end inside a block. Narrow the block height range and retry.

```json
{
  "errors": [
    {
      "message": "query deadline exceeded (10s); narrow the block height range",
      "path": ["getTransactions"],
      "extensions": { "code": "QUERY_DEADLINE_EXCEEDED", "queryDeadline": "10s" }
    }
  ],
  "data": { "getTransactions": null }
}
```

A block list (`getBlocks`, `blocks`) answers the same error beside the blocks it read before the deadline. A block row
is a whole block, so resume past the last height returned: above it ascending, below it descending.

Over JSON-RPC, `getGasPrice`, the one method that scans a block range, answers the same message with code `-32000`,
and the same `code` and `queryDeadline` in the error's `data`. The calls of a batch share one deadline: those not yet
run when it passes answer the error without running.

The deadline bounds time, not CPU: the list fields of one GraphQL query run in parallel, so a query naming several can
keep as many cores busy for the length of the deadline.

Subscriptions, over GraphQL or the `/ws` endpoint, last as long as their connection and are not bounded; each JSON-RPC
call sent over `/ws` is. `/health` and `/ready` are not bounded.

### Metrics

`--metrics-listen-address` serves Prometheus metrics at `/metrics` on a listener of its own, apart from the GraphQL and
JSON-RPC endpoints, so it can stay on a private interface. It is off by default, and the indexer then collects none.

A request is one GraphQL root query field or one JSON-RPC call: a batch counts each of its calls. Its `api` label is
`graphql` or `jsonrpc`, and its `operation` label the field or the method. No label carries a client address or a
request's arguments.

| Metric                                | Type      | Labels                       | Value                                                                                                        |
| ------------------------------------- | --------- | ---------------------------- | ------------------------------------------------------------------------------------------------------------ |
| `tx_indexer_requests_total`           | counter   | `api`, `operation`, `status` | requests served; `status` is `ok` (partial answers included), `error`, or `deadline` (`--query-deadline`)    |
| `tx_indexer_request_duration_seconds` | histogram | `api`, `operation`           | the time a request takes, the fields of the rows it answers included                                         |
| `tx_indexer_rows_scanned_total`       | counter   | `api`, `operation`           | the blocks and transactions read from the database, the ones a filter or an index window then drops included |
| `tx_indexer_rows_returned_total`      | counter   | `api`, `operation`           | the blocks and transactions answered                                                                         |
| `tx_indexer_requests_in_flight`       | gauge     | `api`, `operation`           | the requests being served                                                                                    |
| `tx_indexer_indexed_height`           | gauge     |                              | the latest block height written to the database                                                              |
| `tx_indexer_chain_height`             | gauge     |                              | the chain's latest block height, as last read from the node                                                  |
| `tx_indexer_fetch_errors_total`       | counter   | `stage`                      | failed reads from the node: of its latest height (`chain_height`), or of a range of blocks (`range`)         |
| `tx_indexer_fetch_retries_total`      | counter   |                              | the block ranges fetched again after their fetch failed                                                      |

The Go runtime's (`go_*`) and the process's (`process_*`) metrics are served beside them. A subscription's stream is
not counted, as it lasts as long as its connection: a GraphQL subscription is left out, and a JSON-RPC `subscribe` call
counts for its answer alone. Neither is a request that runs no operation: a GraphQL document that does not validate, or
a JSON-RPC call that is not valid JSON-RPC 2.0 or names a method that does not exist.

A GraphQL list answered in part counts as `ok`, with an error beside its rows: one cut at its element cap, or one short
of a row that failed to decode.

The rows scanned for each row returned tell how much reading a filter costs:
`rate(tx_indexer_rows_scanned_total[5m]) / rate(tx_indexer_rows_returned_total[5m])`.

The lag behind the chain, in blocks, is `tx_indexer_chain_height - tx_indexer_indexed_height`. The chain's height is read
once per fetch round, and not while every fetch slot is busy catching up, so it can trail the chain; a node that stops
answering leaves it as it last was, so watch `tx_indexer_fetch_errors_total{stage="chain_height"}` beside the lag. That
counter also counts the genesis bootstrap's failed reads of the node's status, so a node down when the indexer starts
shows there too.

### Request log

`--request-log` logs one line at info per request served, the same requests the metrics count: its `api`, `operation`
and `status`, its `duration`, its `rows-scanned` and `rows-returned`, and the `client` address. It never logs a
request's arguments: a GraphQL query's variables or a JSON-RPC call's params. It is off by default, as it adds a line
per call of every client and their addresses to the log.

The client address is the address the request came from, which behind a proxy is the proxy's. `--client-ip-header`
names a header the proxies in front of the indexer fill with the client's address instead:

- behind Cloudflare, `CF-Connecting-IP`, which Cloudflare sets to the client's address;
- behind load balancers, `X-Forwarded-For`. Each appends the address it was reached from, and the client can write
  anything to the left of those entries, so `--client-ip-trusted-hops` counts the load balancers and the client address
  is that many entries from the right, 1 by default.

A header that is missing, holds fewer entries, or holds no IP address there leaves the address the request came from.
An entry's port, which some proxies append (`203.0.113.7:51234`, `[2001:db8::1]:443`), is dropped.
Trust a header only if every request reaches the indexer through the proxies that set it: one that reaches it directly
writes the header it likes.

## GraphQL Endpoint

The indexer provides a **GraphQL endpoint** for querying blockchain data (transactions, blocks) and subscribing to real-time events:

**Endpoint:**

```
http://<listen-address>/graphql/query
```

**Playground (Interactive UI):**

```
http://<listen-address>/graphql
```

The playground includes built-in documentation for available queries, fields, and filters.

**Note**: Introspection is enabled by default `--disable-introspection=false`; disable it only if security is a priority (Playground won’t work).

#### Hosted Example

- [Mainnet Playground](https://indexer.gno.land/graphql) — `gnoland-1`

Testnets each run their own indexer at `indexer.<network>.testnets.gno.land/graphql`;
see [Gno networks](https://docs.gno.land/testnets) for whichever is
current.

### Examples

#### Get all Transactions with add_package messages. Show the creator, package name and path.

```graphql
{
  getTransactions(where: { messages: { value: { MsgAddPackage: {} } } }) {
    index
    hash
    block_height
    gas_used
    messages {
      route
      typeUrl
      value {
        __typename
        ... on MsgAddPackage {
          creator
          package {
            name
            path
          }
        }
      }
    }
  }
}
```

#### Subscribe to get all new blocks in real-time

```graphql
subscription {
  getBlocks(where: {}) {
    height
    version
    chain_id
    time
    proposer_address_raw
  }
}
```

#### Get the supply of a denomination

Total, spendable (what data aggregators call the circulating supply) and locked supply, served from the indexer's
background snapshot (see [`getSupply`](#getsupply)); only the denoms from `--supply-denoms` are tracked.

```graphql
{
  getSupply(denom: "ugnot") {
    denom
    height
    total
    spendable
    locked
  }
}
```

## RPC Endpoints

Please take note that the indexer JSON-RPC server adheres to the JSON-RPC 2.0 standard for request and response
processing.

### Block Endpoints

#### `getBlock`

Fetches a specific block from the chain.

- **Params**: Decimal block number (`int64`) as a string.
- **Response**: Base64 encoded, Amino encoded binary of the block.

Example request:

```json
{
  "id": 1,
  "jsonrpc": "2.0",
  "method": "getBlock",
  "params": ["10"]
}
```

Example response:

```json
{
  "result": "CsQCCgt2MS4wLjAtcmMuMBIFdGVzdDMYFCILCJm2/aYGELbF420wBEJICiDi4yHZq+xnUtXDDS73aW9PN9yg4r+B0W0PiB2vmaVlSBIkCAISIJG6uIRyUVyoP447ASEpBUwdUaub/eGmJPxSGeA5lbpzSiCOHonO/bm5CFQW8X24a4Qn+SpJyHerFyaAO7TxMPqrgFog92nrytGOQMQp15Sdl3MG3GaeGdyRMAnaA1rGBVil/QhiIPdp68rRjkDEKdeUnZdzBtxmnhnckTAJ2gNaxgVYpf0IaiC4qGdcWYZSTGBBIl/XuiBtgs1cOjdQQ/BC/N12jhWS7HIgDxA96fJ6yy6vIQOGwa29idxYcNSsumetioI2N9SZObyCAShnMXd5cmU0Z3I3bjgyZXpmcGRoZzNueHlwank5Y2FnOXFwa3U1eDZtGpQCCkgKIOLjIdmr7GdS1cMNLvdpb0833KDiv4HRbQ+IHa+ZpWVIEiQIAhIgkbq4hHJRXKg/jjsBISkFTB1Rq5v94aYk/FIZ4DmVunMSxwEIAhASIkgKIOLjIdmr7GdS1cMNLvdpb0833KDiv4HRbQ+IHa+ZpWVIEiQIAhIgkbq4hHJRXKg/jjsBISkFTB1Rq5v94aYk/FIZ4DmVunMqCwiZtv2mBhC2xeNtMihnMXd5cmU0Z3I3bjgyZXpmcGRoZzNueHlwank5Y2FnOXFwa3U1eDZtQkBhkidOtSLpPWcKhDmUwKNHVNAOHzuVntpNwFx0KDPwauTfZRRqoNiwPp7E812FNLNBT3bfS4U3C73SrMuDJZwE",
  "jsonrpc": "2.0",
  "id": 1
}
```

If no block is found (not yet indexed), `null` as the response result is returned:

```json
{
  "result": null,
  "jsonrpc": "2.0",
  "id": 1
}
```

### Transaction Endpoints

#### `getTxResult`

Fetches the specified transaction result from storage.

- **Params**: Base64 hash of the transaction
- **Response**: Base64 encoded, Amino encoded binary of the transaction result

Example request:

```json
{
  "id": 1,
  "jsonrpc": "2.0",
  "method": "getTxResult",
  "params": ["AP9YX+QXrIByqonIqStod8G9EI5AMiUZhsXk58wr0ws="]
}
```

Example response:

```json
{
  "result": "CIjaGRqVEwrfEQoML3ZtLm1fYWRkcGtnEs4RCihnMTludmNrZ2QzY2trZmp6cDUyczc0Z2N4czA5dHduczc1amVucXk1EqERCgdjYXB0YWluEhdnbm8ubGFuZC9yL2RlbW8vY2FwdGFpbhr8EAoJR1JDMjAuZ25vEu4QcGFja2FnZSBjYXB0YWluDQoNCmltcG9ydCAoDQoJInN0ZCINCgkic3RyaW5ncyINCg0KCSJnbm8ubGFuZC9wL2RlbW8vZ3JjL2dyYzIwIg0KCSJnbm8ubGFuZC9wL2RlbW8vdWZtdCINCgkiZ25vLmxhbmQvci9kZW1vL3VzZXJzIg0KKQ0KDQp2YXIgKA0KCWNhcHRhaW4gICpncmMyMC5BZG1pblRva2VuDQoJYWRtaW4gc3RkLkFkZHJlc3MgPSAiZzE5bnZja2dkM2Nra2ZqenA1MnM3NGdjeHMwOXR3bnM3NWplbnF5NSINCikNCg0KZnVuYyBpbml0KCkgew0KCWNhcHRhaW4gPSBncmMyMC5OZXdBZG1pblRva2VuKCJDUE4iLCAiY2FwdGFpbiIsIDYpDQoJY2FwdGFpbi5NaW50KGFkbWluLCA2OTAwMDAwMDAwMCkgLy8gQGFkbWluaXN0cmF0b3INCn0NCg0KLy8gbWV0aG9kIHByb3hpZXMgYXMgcHVibGljIGZ1bmN0aW9ucy4NCi8vDQoNCi8vIGdldHRlcnMuDQoNCmZ1bmMgVG90YWxTdXBwbHkoKSB1aW50NjQgew0KCXJldHVybiBjYXB0YWluLlRvdGFsU3VwcGx5KCkNCn0NCg0KZnVuYyBCYWxhbmNlT2Yob3duZXIgdXNlcnMuQWRkcmVzc09yTmFtZSkgdWludDY0IHsNCgliYWxhbmNlLCBlcnIgOj0gY2FwdGFpbi5CYWxhbmNlT2Yob3duZXIuUmVzb2x2ZSgpKQ0KCWlmIGVyciAhPSBuaWwgew0KCQlwYW5pYyhlcnIpDQoJfQ0KCXJldHVybiBiYWxhbmNlDQp9DQoNCmZ1bmMgQWxsb3dhbmNlKG93bmVyLCBzcGVuZGVyIHVzZXJzLkFkZHJlc3NPck5hbWUpIHVpbnQ2NCB7DQoJYWxsb3dhbmNlLCBlcnIgOj0gY2FwdGFpbi5BbGxvd2FuY2Uob3duZXIuUmVzb2x2ZSgpLCBzcGVuZGVyLlJlc29sdmUoKSkNCglpZiBlcnIgIT0gbmlsIHsNCgkJcGFuaWMoZXJyKQ0KCX0NCglyZXR1cm4gYWxsb3dhbmNlDQp9DQoNCi8vIHNldHRlcnMuDQoNCmZ1bmMgVHJhbnNmZXIodG8gdXNlcnMuQWRkcmVzc09yTmFtZSwgYW1vdW50IHVpbnQ2NCkgew0KCWNhbGxlciA6PSBzdGQuR2V0T3JpZ0NhbGxlcigpDQoJY2FwdGFpbi5UcmFuc2ZlcihjYWxsZXIsIHRvLlJlc29sdmUoKSwgYW1vdW50KQ0KfQ0KDQpmdW5jIEFwcHJvdmUoc3BlbmRlciB1c2Vycy5BZGRyZXNzT3JOYW1lLCBhbW91bnQgdWludDY0KSB7DQoJY2FsbGVyIDo9IHN0ZC5HZXRPcmlnQ2FsbGVyKCkNCgljYXB0YWluLkFwcHJvdmUoY2FsbGVyLCBzcGVuZGVyLlJlc29sdmUoKSwgYW1vdW50KQ0KfQ0KDQpmdW5jIFRyYW5zZmVyRnJvbShmcm9tLCB0byB1c2Vycy5BZGRyZXNzT3JOYW1lLCBhbW91bnQgdWludDY0KSB7DQoJY2FsbGVyIDo9IHN0ZC5HZXRPcmlnQ2FsbGVyKCkNCgljYXB0YWluLlRyYW5zZmVyRnJvbShjYWxsZXIsIGZyb20uUmVzb2x2ZSgpLCB0by5SZXNvbHZlKCksIGFtb3VudCkNCn0NCg0KLy8gYWRtaW5pc3RyYXRpb24uDQoNCmZ1bmMgTWludChhZGRyZXNzIHVzZXJzLkFkZHJlc3NPck5hbWUsIGFtb3VudCB1aW50NjQpIHsNCgljYWxsZXIgOj0gc3RkLkdldE9yaWdDYWxsZXIoKQ0KCWFzc2VydElzQWRtaW4oY2FsbGVyKQ0KCWNhcHRhaW4uTWludChhZGRyZXNzLlJlc29sdmUoKSwgYW1vdW50KQ0KfQ0KDQpmdW5jIEJ1cm4oYWRkcmVzcyB1c2Vycy5BZGRyZXNzT3JOYW1lLCBhbW91bnQgdWludDY0KSB7DQoJY2FsbGVyIDo9IHN0ZC5HZXRPcmlnQ2FsbGVyKCkNCglhc3NlcnRJc0FkbWluKGNhbGxlcikNCgljYXB0YWluLkJ1cm4oYWRkcmVzcy5SZXNvbHZlKCksIGFtb3VudCkNCn0NCg0KLy8gcmVuZGVyLg0KLy8NCg0KZnVuYyBSZW5kZXIocGF0aCBzdHJpbmcpIHN0cmluZyB7DQoJcGFydHMgOj0gc3RyaW5ncy5TcGxpdChwYXRoLCAiLyIpDQoJYyA6PSBsZW4ocGFydHMpDQoNCglzd2l0Y2ggew0KCWNhc2UgcGF0aCA9PSAiIjoNCgkJcmV0dXJuIGNhcHRhaW4uUmVuZGVySG9tZSgpDQoJY2FzZSBjID09IDIgJiYgcGFydHNbMF0gPT0gImJhbGFuY2UiOg0KCQlvd25lciA6PSB1c2Vycy5BZGRyZXNzT3JOYW1lKHBhcnRzWzFdKQ0KCQliYWxhbmNlLCBfIDo9IGNhcHRhaW4uQmFsYW5jZU9mKG93bmVyLlJlc29sdmUoKSkNCgkJcmV0dXJuIHVmbXQuU3ByaW50ZigiJWRcbiIsIGJhbGFuY2UpDQoJZGVmYXVsdDoNCgkJcmV0dXJuICI0MDRcbiINCgl9DQp9DQoNCmZ1bmMgYXNzZXJ0SXNBZG1pbihhZGRyZXNzIHN0ZC5BZGRyZXNzKSB7DQoJaWYgYWRkcmVzcyAhPSBhZG1pbiB7DQoJCXBhbmljKCJyZXN0cmljdGVkIGFjY2VzcyIpDQoJfQ0KfRIRCIDaxAkSCjUwMDAwdWdub3Qafgo6ChMvdG0uUHViS2V5U2VjcDI1NmsxEiMKIQKN1aylL3JWIoeyByEGcAo2LPJv6HVMI+vneCMVg3YUjRJAfVXpl28frJh0hcJQSO6Vrbpx339p+hf9nuaMx44/Sa984DsstdGuIfZbBo9osUnfSocCw/i84rEsIzGnQ9Hb6SIeRGVwbG95ZWQgdGhyb3VnaCBwbGF5Lmduby5sYW5kIp4dCpIdChQKEi9zdGQuSW50ZXJuYWxFcnJvciL5HHJlY292ZXJlZDogcGFja2FnZSBhbHJlYWR5IGV4aXN0czogZ25vLmxhbmQvci9kZW1vL2NhcHRhaW4Kc3RhY2s6Cmdvcm91dGluZSA2MCBbcnVubmluZ106CnJ1bnRpbWUvZGVidWcuU3RhY2soKQoJL3Vzci9sb2NhbC9nby9zcmMvcnVudGltZS9kZWJ1Zy9zdGFjay5nbzoyNCArMHg2NQpnaXRodWIuY29tL2dub2xhbmcvZ25vL3BrZ3Mvc2RrLigqQmFzZUFwcCkucnVuVHguZnVuYzEoKQoJL29wdC9idWlsZC9wa2dzL3Nkay9iYXNlYXBwLmdvOjc0MyArMHgyY2QKcGFuaWMoezB4YmU1NmMwLCAweGMwMGZjMWRkMDB9KQoJL3Vzci9sb2NhbC9nby9zcmMvcnVudGltZS9wYW5pYy5nbzo4ODQgKzB4MjEyCmdpdGh1Yi5jb20vZ25vbGFuZy9nbm8vcGtncy9zZGsvdm0uKCpWTUtlZXBlcikuQWRkUGFja2FnZShfLCB7ezB4ZjYyMTQwLCAweGMwMDk5YmUyNzB9LCAweDIsIHsweGY2MWIwOCwgMHhjMDBmYzFjOWYwfSwgezB4ZjYyMmM4LCAweGMwMDU2YTQ0MjB9LCB7MHhjMDI0YTljOTgwLCAweDV9LCAuLi59LCAuLi4pCgkvb3B0L2J1aWxkL3BrZ3Mvc2RrL3ZtL2tlZXBlci5nbzoxMzggKzB4Njg1CmdpdGh1Yi5jb20vZ25vbGFuZy9nbm8vcGtncy9zZGsvdm0udm1IYW5kbGVyLmhhbmRsZU1zZ0FkZFBhY2thZ2Uoe199LCB7ezB4ZjYyMTQwLCAweGMwMDk5YmUyNzB9LCAweDIsIHsweGY2MWIwOCwgMHhjMDBmYzFjOWYwfSwgezB4ZjYyMmM4LCAweGMwMDU2YTQ0MjB9LCB7MHhjMDI0YTljOTgwLCAweDV9LCAuLi59LCAuLi4pCgkvb3B0L2J1aWxkL3BrZ3Mvc2RrL3ZtL2hhbmRsZXIuZ286NDYgKzB4MmM1CmdpdGh1Yi5jb20vZ25vbGFuZy9nbm8vcGtncy9zZGsvdm0udm1IYW5kbGVyLlByb2Nlc3Moe199LCB7ezB4ZjYyMTQwLCAweGMwMDk5YmUyNzB9LCAweDIsIHsweGY2MWIwOCwgMHhjMDBmYzFjOWYwfSwgezB4ZjYyMmM4LCAweGMwMDU2YTQ0MjB9LCB7MHhjMDI0YTljOTgwLCAweDV9LCAuLi59LCAuLi4pCgkvb3B0L2J1aWxkL3BrZ3Mvc2RrL3ZtL2hhbmRsZXIuZ286MjcgKzB4MjI1CmdpdGh1Yi5jb20vZ25vbGFuZy9nbm8vcGtncy9zZGsuKCpCYXNlQXBwKS5ydW5Nc2dzKF8sIHt7MHhmNjIxNDAsIDB4YzAwOTliZTI3MH0sIDB4MiwgezB4ZjYxYjA4LCAweGMwMGZjMWM5ZjB9LCB7MHhmNjIyYzgsIDB4YzAwNTZhNDQyMH0sIHsweGMwMjRhOWM5ODAsIDB4NX0sIC4uLn0sIC4uLikKCS9vcHQvYnVpbGQvcGtncy9zZGsvYmFzZWFwcC5nbzo2NDQgKzB4NDJmCmdpdGh1Yi5jb20vZ25vbGFuZy9nbm8vcGtncy9zZGsuKCpCYXNlQXBwKS5ydW5UeCgweGMwMDAxYTQyMDAsIDB4MiwgezB4YzAwNDNkYjUwMCwgMHg5OTUsIF99LCB7ezB4YzAxMzBhOWM0MCwgMHgxLCAweDF9LCB7MHg5ODk2ODAsIHt7MHhjMDBlZjJjNjg1LCAuLi59LCAuLi59fSwgLi4ufSkKCS9vcHQvYnVpbGQvcGtncy9zZGsvYmFzZWFwcC5nbzo4MjMgKzB4OTA1CmdpdGh1Yi5jb20vZ25vbGFuZy9nbm8vcGtncy9zZGsuKCpCYXNlQXBwKS5EZWxpdmVyVHgoMHgwPywge3t9LCB7MHhjMDA0M2RiNTAwPywgMHgwPywgMHgwP319KQoJL29wdC9idWlsZC9wa2dzL3Nkay9iYXNlYXBwLmdvOjU4MCArMHgxN2QKZ2l0aHViLmNvbS9nbm9sYW5nL2duby9wa2dzL2JmdC9hYmNpL2NsaWVudC4oKmxvY2FsQ2xpZW50KS5EZWxpdmVyVHhBc3luYygweGMwMTNiNzY1NDAsIHt7fSwgezB4YzAwNDNkYjUwMD8sIDB4MD8sIDB4MD99fSkKCS9vcHQvYnVpbGQvcGtncy9iZnQvYWJjaS9jbGllbnQvbG9jYWxfY2xpZW50LmdvOjgyICsweDEwMwpnaXRodWIuY29tL2dub2xhbmcvZ25vL3BrZ3MvYmZ0L3Byb3h5LigqYXBwQ29ubkNvbnNlbnN1cykuRGVsaXZlclR4QXN5bmMoMHgwPywge3t9LCB7MHhjMDA0M2RiNTAwPywgMHgwPywgMHgwP319KQoJL29wdC9idWlsZC9wa2dzL2JmdC9wcm94eS9hcHBfY29ubi5nbzo3MyArMHgyNgpnaXRodWIuY29tL2dub2xhbmcvZ25vL3BrZ3MvYmZ0L3N0YXRlLmV4ZWNCbG9ja09uUHJveHlBcHAoezB4ZjYzMGEwLCAweGMwMTNiN2MwZjB9LCB7MHhmNjU1NzAsIDB4YzAxM2I2MzZmMH0sIDB4YzAwZjY3NTZjMCwgezB4ZjZhYWYwLCAweGMwMDAxMTliNDB9KQoJL29wdC9idWlsZC9wa2dzL2JmdC9zdGF0ZS9leGVjdXRpb24uZ286MjUzICsweDhkYQpnaXRodWIuY29tL2dub2xhbmcvZ25vL3BrZ3MvYmZ0L3N0YXRlLigqQmxvY2tFeGVjdXRvcikuQXBwbHlCbG9jayhfLCB7ezB4Y2Y0ZGQwLCAweGJ9LCB7MHhjZjRkZDAsIDB4Yn0sIHsweDAsIDB4MH0sIHsweGMwMTNiNGZhZjAsIDB4NX0sIDB4MzM2ODMsIC4uLn0sIC4uLikKCS9vcHQvYnVpbGQvcGtncy9iZnQvc3RhdGUvZXhlY3V0aW9uLmdvOjEwMiArMHgxMTUKZ2l0aHViLmNvbS9nbm9sYW5nL2duby9wa2dzL2JmdC9jb25zZW5zdXMuKCpDb25zZW5zdXNTdGF0ZSkuZmluYWxpemVDb21taXQoMHhjMDA3Y2M0YzAwLCAweDMzNjg0KQoJL29wdC9idWlsZC9wa2dzL2JmdC9jb25zZW5zdXMvc3RhdGUuZ286MTM0OCArMHg5N2UKZ2l0aHViLmNvbS9nbm9sYW5nL2duby9wa2dzL2JmdC9jb25zZW5zdXMuKCpDb25zZW5zdXNTdGF0ZSkudHJ5RmluYWxpemVDb21taXQoMHhjMDA3Y2M0YzAwLCAweDMzNjg0KQoJL29wdC9idWlsZC9wa2dzL2JmdC9jb25zZW5zdXMvc3RhdGUuZ286MTI3NiArMHgzMTMKZ2l0aHViLmNvbS9nbm9sYW5nL2duby9wa2dzL2JmdC9jb25zZW5zdXMuKCpDb25zZW5zdXNTdGF0ZSkuZW50ZXJDb21taXQuZnVuYzEoKQoJL29wdC9idWlsZC9wa2dzL2JmdC9jb25zZW5zdXMvc3RhdGUuZ286MTIyMiArMHg5NwpnaXRodWIuY29tL2dub2xhbmcvZ25vL3BrZ3MvYmZ0L2NvbnNlbnN1cy4oKkNvbnNlbnN1c1N0YXRlKS5lbnRlckNvbW1pdCgweGMwMDdjYzRjMDAsIDB4MzM2ODQsIDB4MCkKCS9vcHQvYnVpbGQvcGtncy9iZnQvY29uc2Vuc3VzL3N0YXRlLmdvOjEyNTMgKzB4YjAzCmdpdGh1Yi5jb20vZ25vbGFuZy9nbm8vcGtncy9iZnQvY29uc2Vuc3VzLigqQ29uc2Vuc3VzU3RhdGUpLmFkZFZvdGUoMHhjMDA3Y2M0YzAwLCAweGMwMDlhNDgyODAsIHsweDAsIDB4MH0pCgkvb3B0L2J1aWxkL3BrZ3MvYmZ0L2NvbnNlbnN1cy9zdGF0ZS5nbzoxNjQxICsweDkxMwpnaXRodWIuY29tL2dub2xhbmcvZ25vL3BrZ3MvYmZ0L2NvbnNlbnN1cy4oKkNvbnNlbnN1c1N0YXRlKS50cnlBZGRWb3RlKDB4YzAwN2NjNGMwMCwgMHg0YjQ0MjY/LCB7MHgwPywgMHhjMDBiODE3YmYwP30pCgkvb3B0L2J1aWxkL3BrZ3MvYmZ0L2NvbnNlbnN1cy9zdGF0ZS5nbzoxNDg0ICsweDI3CmdpdGh1Yi5jb20vZ25vbGFuZy9nbm8vcGtncy9iZnQvY29uc2Vuc3VzLigqQ29uc2Vuc3VzU3RhdGUpLmhhbmRsZU1zZygweGMwMDdjYzRjMDAsIHt7MHhmNWJhYTA/LCAweGMwMWM0OWZiZDg/fSwgezB4MD8sIDB4ZWRkMzI1ZTU2P319KQoJL29wdC9idWlsZC9wa2dzL2JmdC9jb25zZW5zdXMvc3RhdGUuZ286NjkxICsweDNhOApnaXRodWIuY29tL2dub2xhbmcvZ25vL3BrZ3MvYmZ0L2NvbnNlbnN1cy4oKkNvbnNlbnN1c1N0YXRlKS5yZWNlaXZlUm91dGluZSgweGMwMDdjYzRjMDAsIDB4MCkKCS9vcHQvYnVpbGQvcGtncy9iZnQvY29uc2Vuc3VzL3N0YXRlLmdvOjY1MCArMHg0NTcKY3JlYXRlZCBieSBnaXRodWIuY29tL2dub2xhbmcvZ25vL3BrZ3MvYmZ0L2NvbnNlbnN1cy4oKkNvbnNlbnN1c1N0YXRlKS5PblN0YXJ0Cgkvb3B0L2J1aWxkL3BrZ3MvYmZ0L2NvbnNlbnN1cy9zdGF0ZS5nbzozNDQgKzB4NGRjChCA2sQJGIq9Cw==",
  "jsonrpc": "2.0",
  "id": 1
}
```

If no transaction result is found (not yet indexed, or transaction not committed yet), `null` as the response result is
returned:

```json
{
  "result": null,
  "jsonrpc": "2.0",
  "id": 1
}
```

### Supply Endpoints

#### `getSupply`

Returns the supply of a tracked denomination at a recent chain height, split by spendability: how much of it exists
(`total`), how much of that the genesis vesting schedules still lock (`locked`), and the difference (`spendable`),
which is what data aggregators call the circulating supply.

- **Params**: `denom`, a tracked denomination, e.g. `ugnot`
- **Response**: `{ denom, height, total, spendable, locked }`, amounts as strings

The figures come from a background snapshot the indexer refreshes on its own schedule (every 10 seconds), so requests
never reach the chain. The snapshot covers only the denoms listed in `--supply-denoms` (default `ugnot`): the total is
read from the chain's per-denom supply counter, and the locked portion is computed from the vesting schedules in the
chain genesis, evaluated at the snapshot's block time. Locked is not clamped to each account's live balance, so for
accounts that spent below their schedule through fees it can exceed what they still hold; in exchange a refresh reads
one counter per denom instead of one balance per vesting account. Schedules sharing a vesting window are summed and
evaluated as one, so a refresh costs the same whether the genesis holds ten vesting accounts or millions; the summed
figure can be lower than a per-account evaluation by less than one unit of the denom per account. Locked never
exceeds the total, so `total = spendable + locked` always holds. All figures in a snapshot were read at the same
height, in one batched query. If a refresh fails, the last good snapshot keeps serving.

Example request:

```json
{
  "id": 1,
  "jsonrpc": "2.0",
  "method": "getSupply",
  "params": ["ugnot"]
}
```

Example response:

```json
{
  "result": {
    "denom": "ugnot",
    "height": 1337,
    "total": "1000000000000",
    "spendable": "700000000000",
    "locked": "300000000000"
  },
  "jsonrpc": "2.0",
  "id": 1
}
```

### Filter Endpoints

#### `newBlockFilter`

Creates a filter for new block events, to notify when a new block arrives.
To check if the state has changed, call the `getFilterChanges` endpoint.

- **Params**: no params
- **Response**: the filter ID (`string`)

Example request:

```json
{
  "id": 1,
  "jsonrpc": "2.0",
  "method": "newBlockFilter",
  "params": []
}
```

Example response:

```json
{
  "result": "c77000bb-700c-41b9-830c-e8b35bdef246",
  "jsonrpc": "2.0",
  "id": 1
}
```

#### `getFilterChanges`

Polling method for a filter, which returns an array of events that have occurred since the last poll.
Filters that are inactive (not polled) for `5min` are automatically cleaned up.

- **Params**: the filter ID (`string`)
- **Response**: array containing filter data.
  - In case of a block filter, the response is an array of base64 encoded, Amino binary block headers

Example request:

```json
{
  "id": 1,
  "jsonrpc": "2.0",
  "method": "getFilterChanges",
  "params": ["c77000bb-700c-41b9-830c-e8b35bdef246"]
}
```

Example response:

```json
{
  "result": [
    "Cgt2MS4wLjAtcmMuMBIFdGVzdDMYstoZIgsI9teBrQYQ4f+cJzDgH0JICiCOeRC+O7eSKWElYfr3roazF9A23Wvk2AvSWfhU1If41BIkCAISIOmJytoey1P0ZF1/oZUs7Pa7ytV5WJrh41s2PxvWHc4mSiDU0XJ+JFOgK7vm2fVLL29BtwKOGfkv+JxHB1sdgeoSwVog92nrytGOQMQp15Sdl3MG3GaeGdyRMAnaA1rGBVil/QhiIPdp68rRjkDEKdeUnZdzBtxmnhnckTAJ2gNaxgVYpf0IaiC4qGdcWYZSTGBBIl/XuiBtgs1cOjdQQ/BC/N12jhWS7HIggbuwi7zCzwlQP7PCb9EXN2EG7738FHbXIgYPWxBlakiCAShnMXd5cmU0Z3I3bjgyZXpmcGRoZzNueHlwank5Y2FnOXFwa3U1eDZt",
    "Cgt2MS4wLjAtcmMuMBIFdGVzdDMYtNoZIgsIs9iBrQYQ7cjnLDDgH0JICiC9pNnvamowMWsgZczqQ6V5J0YoHNYZWCwFvSFV88+j3xIkCAISIATruI4Rl6mfRoS6GzjaE0aCpWZaYNssV1E+8xRlrDStSiAwCZMWNPsjNM/2fOv37CTjuTPqKW71C5xuBnhEVO5a5Fog92nrytGOQMQp15Sdl3MG3GaeGdyRMAnaA1rGBVil/QhiIPdp68rRjkDEKdeUnZdzBtxmnhnckTAJ2gNaxgVYpf0IaiC4qGdcWYZSTGBBIl/XuiBtgs1cOjdQQ/BC/N12jhWS7HIggbuwi7zCzwlQP7PCb9EXN2EG7738FHbXIgYPWxBlakiCAShnMXd5cmU0Z3I3bjgyZXpmcGRoZzNueHlwank5Y2FnOXFwa3U1eDZt"
  ],
  "jsonrpc": "2.0",
  "id": 1
}
```

#### `uninstallFilter`

Uninstalls a filter with the given filter ID.

- **Params**: the filter ID (`string`)
- **Response**: `true` if the filter was successfully uninstalled, otherwise `false` (`boolean`)

Example request:

```json
{
  "id": 1,
  "jsonrpc": "2.0",
  "method": "uninstallFilter",
  "params": ["c77000bb-700c-41b9-830c-e8b35bdef246"]
}
```

Example response:

```json
{
  "result": true,
  "jsonrpc": "2.0",
  "id": 1
}
```

#### `subscribe`

Starts a subscription to a specific event. **Only available over WS connections**.

Available events:

- `newHeads` - fires a notification each time a new header is appended to the chain

- **Params**: the event type [`newHeads`] (`string`)
- **Response**: the subscription ID (`string`) (initial response), then event data (see example below)
  - For `newHeads` events, the result is a base64 encoded, Amino binary block header

Since this endpoint is only supported over WS connections, it will write data directly to the client.

Example request (over WS):

```json
{
  "id": 1,
  "jsonrpc": "2.0",
  "method": "subscribe",
  "params": ["newHeads"]
}
```

Example initial response (over WS):

```json
{
  "result": "b8934e81-5758-4249-8953-da90aa777ef9",
  "jsonrpc": "2.0",
  "id": 1
}
```

Example response when a `newHeads` event happens (over WS):

```json
{
  "params": {
    "result": "CscCCgt2MS4wLjAtcmMuMBIFdGVzdDMYyNoZIgsIld2BrQYQouHxZjDgH0JICiALMxWpa/sfrgj51bcYbSgeqSOz0taKXLKCYPIMlE/xjBIkCAISIBIVcZZiQcME2mT2GnLIhbmGWrixQpiH+cGaI9Iqo6ggSiDj49aFmYNf78eBsV6deyeeebleL38O8Ov8/XSgbCalxVog92nrytGOQMQp15Sdl3MG3GaeGdyRMAnaA1rGBVil/QhiIPdp68rRjkDEKdeUnZdzBtxmnhnckTAJ2gNaxgVYpf0IaiC4qGdcWYZSTGBBIl/XuiBtgs1cOjdQQ/BC/N12jhWS7HIggbuwi7zCzwlQP7PCb9EXN2EG7738FHbXIgYPWxBlakiCAShnMXd5cmU0Z3I3bjgyZXpmcGRoZzNueHlwank5Y2FnOXFwa3U1eDZtGpYCCkgKIAszFalr+x+uCPnVtxhtKB6pI7PS1opcsoJg8gyUT/GMEiQIAhIgEhVxlmJBwwTaZPYacsiFuYZauLFCmIf5wZoj0iqjqCASyQEIAhDG2hkiSAogCzMVqWv7H64I+dW3GG0oHqkjs9LWilyygmDyDJRP8YwSJAgCEiASFXGWYkHDBNpk9hpyyIW5hlq4sUKYh/nBmiPSKqOoICoLCJXdga0GEKLh8WYyKGcxd3lyZTRncjduODJlemZwZGhnM254eXBqeTljYWc5cXBrdTV4Nm1CQH5b0jjU1gW8mC+zuCyIPy5xjrfRMSNEvFcHoOjXzc4aEgcW5cYXamXv0WLw2g5RFim2qmgjHnuQorU1YXnWDgw=",
    "subscription": "b8934e81-5758-4249-8953-da90aa777ef9"
  },
  "jsonrpc": "2.0",
  "method": "subscription"
}
```

#### `unsubscribe`

Cancels an existing subscription so that no further events are sent. **Only available over WS connections**.

- **Params**: the subscription ID (`string`)
- **Response**: A boolean value indicating if the subscription was canceled successfully (`boolean`)

Example request (over WS):

```json
{
  "id": 1,
  "jsonrpc": "2.0",
  "method": "unsubscribe",
  "params": ["b8934e81-5758-4249-8953-da90aa777ef9"]
}
```

Example response (over WS):

```json
{
  "result": true,
  "jsonrpc": "2.0",
  "id": 1
}
```
