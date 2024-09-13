<h2 align="center">⚛️ Tendermint2 Indexer ⚛️</h2>

- [Overview](#overview)
- [Key Features](#key-features)
- [Getting Started](#getting-started)
- [GraphQL Endpoint](#graphql-endpoint)
  - [Examples](#examples)
    - [Get all Transactions with add\_package messages. Show the creator, package name and path.](#get-all-transactions-with-add_package-messages-show-the-creator-package-name-and-path)
    - [Subscribe to get all new blocks in real-time](#subscribe-to-get-all-new-blocks-in-real-time)
- [RPC Endpoints](#rpc-endpoints)
  - [Block Endpoints](#block-endpoints)
    - [`getBlock`](#getblock)
  - [Transaction Endpoints](#transaction-endpoints)
    - [`getTxResult`](#gettxresult)
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
./build/tx-indexer start --remote http://test3.gno.land:36657 --db-path indexer-db
```

or:

```bash
go run cmd/main.go cmd/start.go cmd/waiter.go start --remote http://test3.gno.land:36657 --db-path indexer-db
```

The `--remote` flag specifies the JSON-RPC URL of the chain the indexer should index, and the `--db-path` specifies the
on-disk location for the indexed data.

**Note**: the websocket endpoint exposed is always: `ws://<listen-address>/ws`

For a full list of available features and flags, execute the `--help` command:

```shell
> ./build/tx-indexer start --help

DESCRIPTION
  Starts the indexer service

USAGE
  start [flags]

Starts the indexer service, which includes the fetcher and JSON-RPC server

FLAGS
  -db-path indexer-db             the absolute path for the indexer DB (embedded)
  -http-rate-limit 0              the maximum HTTP requests allowed per minute per IP, unlimited by default
  -listen-address 0.0.0.0:8546    the IP:PORT URL for the indexer JSON-RPC server
  -log-level info                 the log level for the CLI output
  -max-chunk-size 100             the range for fetching blockchain data by a single worker
  -max-slots 100                  the amount of slots (workers) the fetcher employs
  -remote http://127.0.0.1:26657  the JSON-RPC URL of the Gno chain
```

## GraphQL Endpoint

A GraphQL endpoint is available at `/graphql/query`. It supports standard queries for transactions and blocks and subscriptions for real-time events.

A GraphQL playground is available at `/graphql`. There you have all the documentation needed explaining the different fields and available filters.

### Examples

#### Get all Transactions with add_package messages. Show the creator, package name and path.

```graphql
{
  transactions(
    filter: { message: {vm_param: {add_package: {}}}}
  ) {
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
  blocks(filter: {}) {
    height
    version
    chain_id
    time
    proposer_address_raw
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
  "params": [
    "10"
  ]
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
  "params": [
    "AP9YX+QXrIByqonIqStod8G9EI5AMiUZhsXk58wr0ws="
  ]
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
  "params": [
    "c77000bb-700c-41b9-830c-e8b35bdef246"
  ]
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
  "params": [
    "c77000bb-700c-41b9-830c-e8b35bdef246"
  ]
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
  "params": [
    "newHeads"
  ]
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
  "params": [
    "b8934e81-5758-4249-8953-da90aa777ef9"
  ]
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
