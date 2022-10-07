# mev-tendermint

***The purpose of mev-tendermint is to create a private mempool (the “sidecar”) containing atomic bundles of txs and gossip bundles of transactions specifically to the proposer of the next block.***

---

### Design Goals

The design goals of MEV-Tendermint is to allow & preserve:

1. 🔒  **Privacy** for users submitting bundles
2. 🎁  **Atomicity** for bundles of transactions
3. 🐎  **Priority** guaranteed for highest paying bundles
4. 🏛  **No new security assumptions** for validators and nodes running MEV-Tendermint, including removing the need for ingress or egress for locked-down validators. No new network assumptions are made
5. 🔄  **On-chain transaction submission** via gossip, no need for off-chain submission like HTTP endpoints, endpoint querying, etc
6. 💨  **Impossible to slow down block time**, i.e. no part of mev-tendermint introduces consensus delays

### Basic Functionality Overview

🏦 **Auction**

- Prior to the creation of the first proposal for height `n+1` , the Skip Sentinel infrastructure selects an auction-winning bundle (or bundles) to include at the top of block `n+1`
- The auction-winning bundle is defined as the bundle that pays the highest gas price ( sum(txFee)/sum(gasWanted) ) and doesn’t include any reverting transactions
- The sentinel ensures it’s simulations of the bundle are accurate by simulating it against the version of state where it will actually run (by optimistically applying the proposals produced for height `n` )

🗣️ **Gossip**

- Before the first proposal for height `n+1` is created, the Skip sentinel gossips the auction-winning bundle(s) to whichever nodes belonging to that proposer it can access (e.g. sentries if the validator is using a sentry configuration, or validator replicas if it’s using horcrux)
- The nodes that receive the winning bundle(s) gossip it to the other nodes belonging to that proposer to ensure the bundle(s) reach the validator
- This selective gossiping is powered by new config options (`personal_peer_ids`) and takes place over a new channel, but it is secured using the same authentication handshake Tendermint uses to secure all other forms of p2p communication

[reinforce that we have different channels on the same reactor]

🏒 **Handling Transactions**

- Ordinary transactions received over traditional gossip are handled exactly the same way they are today in the mempool
- Transactions received as part of bundles sent from the Skip sentinel are handled and stored in a new data structure called the `sidecar`
- These transactions have additional metadata about the bundle in which they should be included (e.g. bundleOrder, bundleSize). The sidecar uses this data to reconstruct bundles as it receives individual transactions over gossip

[reinforce that we have a new transaction data structure]

🚜 **Reaping** 

- On reap, mev-tendermint first checks whether there are any fully-constructed bundles in the sidecar then reaps these first.
- Next, it reaps from the ordinary mempool, with some additional checks to ensure that transactions reaped from the sidecar don’t get reaped again if they are also present in the standard mempool

[reinforce reaping of bundle goes to top if available]

### Components

**#1 The Sidecar**

- A separate, private mempool that respects `bundles` of transactions
    - Relevant files: `mempool/clist_sidecar.go`
- Has **selective gossiping**, meaning it only gossips:
    - Over its own `SidecarChannel`
    - **Only** to peers that are added as its `personal_peers`
        - In practice, `personal_peers` for each node are set to be:
            - Sentry node →  **Skip sentinel** & **the other nodes you’re running that the sentry is aware of (e.g. validator or a layer of sentries closer to the validator)**
            - Validator node → **only its sentries**

**#2 The Mempool Reactor**

- The mempool reactor now supports a `SidecarChannel` over which only gossip for `SidecarTxs` can be handled
    - Relevant files: `mempool/reactor.go`
    - `SidecarTxs` have new metadata that is transmitted over gossip, including
        - `BundleId` - the **global** order of the bundle this `SidecarTx` is in, per height
        - `BundleOrder` - the **local** order of this `SidecarTx` within its bundle
        - `DesiredHeight` - the height of the bundle this `SidecarTx` was submitted for
        - `BundleSize` - the total size of the bundle this `SidecarcarTx` is in
        - `TotalFee` - the total fee of the bundle this `SidecarTx` is in
    - This metadata is submitted at a transaction level as **tendermint currently is not designed to broadcast batches of transactions**

**#3 Selective Reaping**

- The regular mempool now considers `sidecarTxs` (i.e. bundles) in addition to regular txs, and orders the former before the latter
    - Relevant files: `mempool/clist_mempool.go`, `state/execution.go`

# 👨‍💻 How to Configure

### 1. Tendermint replacement ♻️

In the `go.mod` file of the directory you use to compile your chain binary, you need to replace your version of `tendermint` with the correct `mev-tendermint` version, like so:

- For Juno (testnet and mainnet), this is: `v0.34.21-mev`

```tsx
// ---------------------------------
// BEFORE
require (
	// Other stuff...
	github.com/tendermint/tendermint v0.34.21
	// Other stuff...
)

// ---------------------------------
// AFTER
require (
	// Other stuff...
	github.com/skip-mev/mev-tendermint v0.34.21-mev
	// Other stuff...
)
```

### 2. Peering Setup 🤝

mev-tendermint introduces a new section of config in `config.toml` called `sidecar`, which contains 2 settings that you must configure in order to receive bundles from the skip sentinel: 

- `relayer_id` : This is the Tendermint p2p id of the Skip Sentinel that is used to establish a secret, authenticated handshake between your node and the Skip sentinel
    - For nodes that should not communicate with the sentinel directly (e.g. validator nodes that have sentries), this does not need to be set.
- `personal_peer_ids`: These are the Tendermint p2p ids of all the nodes that your node will gossip side car transactions with. To ensure trader privacy, these should only include p2p ids of nodes that you manage.
    - For your validator, this should be the `ids` of all your sentry nodes
    - For your sentry nodes, this should be the `ids` of all your **other** sentry nodes, and your validator

### 3. Information Skip Requires from you  ℹ️

In order to participate in the network, you must share with Skip (feel free to contact us at on our **[website](https://skip.money/)**): 

1. The Tendermint Address of your validator (The `“address”` field of the `priv_validator_key.json` file generated by Tendermint)
2. The Tendermint p2p ids of all nodes you wish the Skip sentinel to gossip with directly (Can be obtained by `tendermint show-node-id`

### 4. Recompile your binary, and start! 🎉

That’s it! After making the changes above, you can recompile your binary like `junod`, and restart your node(s). You will now begin receiving MEV bundles from Skip