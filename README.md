![banner](https://skip-protocol.notion.site/image/https%3A%2F%2Fs3-us-west-2.amazonaws.com%2Fsecure.notion-static.com%2F33ea763f-bfa3-4c65-ad35-ad0ee1fd312d%2FGroup_6.png?table=block&id=4e75ce44-3f92-482e-a199-4aa75631706b&spaceId=4ee2f125-c8d3-4d79-9a63-1a260c9b8377&width=2000&userId=&cache=v2)
# mev-tendermint (.8)

***The purpose of mev-tendermint is to create a private mempool (the “sidecar”) containing atomic bundles of txs and gossip bundles of transactions specifically to the proposer of the next block.***

---

# 👨‍💻 How to Integrate [~10min]

**NOTE: if you have any questions/issues with integration, please ask us on our discord:  [https://discord.gg/amAgf9Z39w](https://discord.gg/amAgf9Z39w)**

---

## 1. Get an API Key  🔑

**🚨**  If you don’t already have an API Key, **please get one from the [Skip registration site](https://skip.money/register) 🚨**

💵 **You can also configure your MEV payments on the site.** Remember - Skip takes no fees 🎉

---

## 2. Tendermint replacement ♻️

In the `go.mod` file of the directory you use to compile your chain binary, add a line into `replace` to import the correct `mev-tendermint` version.

🚨  **You can find the correct version of the `replace` you should use here:** [⚙️ Skip Configurations By Chain](https://www.notion.so/Skip-Configurations-By-Chain-a6076cfa743f4ab38194096403e62f3c) 

```tsx
// ---------------------------------
replace (
    // Other stuff...
    github.com/tendermint/tendermint => github.com/skip-mev/mev-tendermint <USE CORRECT TAG FOR YOUR CHAIN>
)
```

🚨 **After modifying the `replace` statement, run `go mod tidy` in your base directory**

---

## 3. Peering Setup 🤝

mev-tendermint introduces a new section of config in `config.toml` called `[sidecar].`

…by the end, the end of your `config.toml` will look something like this (with different string values). **Make sure to include the line `[sidecar]` at the top of this section in `config.toml`.**

🚨 **FIND THE CORRECT VALUES TO USE HERE: [⚙️ Skip Configurations By Chain](https://www.notion.so/Skip-Configurations-By-Chain-a6076cfa743f4ab38194096403e62f3c)** 

```bash
# OTHER CONFIG...

# **EXAMPLE** below (please use the correct values)
[sidecar]
sentinel_peer_string = "d1463b730c6e0dcea59db726836aeaff13a8119f@uni-5-sentinel.skip.money:26656"
sentinel_rpc_string = "uni-5-gateway.skip.money"
api_key = "2314ajinashg2389jfjap"
personal_peer_ids = "557611c7a7307ce023a7d13486b570282521296d,5740acbf39a9ae59953801fe4997421b6736e091"
```

Here’s an explanation of what these are:

### `sentinel_peer_string`

- The `p2p@ip:port` for the Skip Sentinel that is used to establish a secret, authenticated handshake between your node and the Skip sentinel
- For nodes that should not communicate with the sentinel directly (e.g. validator nodes that have sentries), this does not need to be set.
- **⭐ Find the `sentinel_peer_string` here:** [⚙️ Skip Configurations By Chain](https://www.notion.so/Skip-Configurations-By-Chain-a6076cfa743f4ab38194096403e62f3c)

### `sentinel_rpc_string`

- The `api` for the Skip Sentinel that is used to register your node
- For nodes that should not communicate with the sentinel directly (e.g. validator nodes that have sentries), this does not need to be set.
- **⭐ Find the `sentinel_rpc_string` here:** [⚙️ Skip Configurations By Chain](https://www.notion.so/Skip-Configurations-By-Chain-a6076cfa743f4ab38194096403e62f3c)

### `api_key`

- This is the unique string key Skip uses to ensure a node establishing a connection with our Sentinel actually belongs to your validator.
- If you don't have one, please request one from the Skip team on our **[discord](https://discord.gg/amAgf9Z39w)**

### `personal_peer_ids`

- **You only need to set this if you use sentries, otherwise leave empty.**
- This is the list of peer nodes your node will gossip Skip bundles to after receiving them.
    - For your validator, this should be the `p2p ids` of all your **sentry nodes**
    - For your sentry nodes, this should be the `p2p ids` of all your **other** sentry nodes, **and your validator**
- You can find a node’s p2p id using (on the machine for the node):

```json
evmosd tendermint show-node-id --home <HOME_DIR>
```

---

## 4. Recompile your binary, and start! 🎉

That’s it! After making the changes above, you can recompile your binary (e.g. `evmosd`, probably using `make install`),  and restart your node(s)! You will now begin receiving MEV bundles and higher rewards.

---

# ✍️ Monitoring & Troubleshooting

After you have completed the steps above, you can check you connectivity either via:

- Check if you are peered with the sentinel by calling `curl http://localhost:26657/status`
    
    ```jsx
     ”is_peered_with_sentinel”: true
    ```
    
- Check if you are running `mev-tendermint` by running either:
    
    ```bash
    # by running binary
    curl -sL localhost:26657/status | jq .result.mev_info
    
    # or by checking version detail
    evmosd version --long | grep mev
    ```
    
- Via the new prometheus metrics exposed on mev-tendermint, in particular `mev_sentinel_connected`

---

# ⚙️ Handling Chain Upgrades

Handling chain upgrades is simple:

1. Apply the latest patch to your validators & nodes, **without `mev-tendermint`**
    1. If you have local changes to `go.mod` and `go.sum` that prevent you from pulling the new version, you can run:
    
    ```bash
    git stash
    git stash apply
    ```
    
    1. to remove them first, then pull again
2. Recompile your binary with `mev-tendermint` (**same as step 2)**, keeping the same config
3. Restart your nodes and validators, you’re back up! 🎉

---

# 🤿 About `mev-tendermint`

## ✅  Design

The design goals of MEV-Tendermint is to allow & preserve:

1. 🔒  **Privacy** for users submitting bundles
2. 🎁  **Atomicity** for bundles of transactions
3. 🐎  **Priority** guaranteed for highest paying bundles
4. 🏛  **No new security assumptions** for validators and nodes running MEV-Tendermint, including removing the need for ingress or egress for locked-down validators. No new network assumptions are made
5. 🔄  **On-chain transaction submission** via gossip, no need for off-chain submission like HTTP endpoints, endpoint querying, etc
6. 💨  **Impossible to slow down block time**, i.e. no part of mev-tendermint introduces consensus delays

## 🔎  Basic Functionality Overview

🏦  **Auction**

- Prior to the creation of the first proposal for height `n+1` , the Skip Sentinel infrastructure selects an auction-winning bundle (or bundles) to include at the top of block `n+1`
- The auction-winning bundle is defined as the bundle that pays the highest gas price ( sum(txFee)/sum(gasWanted) ) and doesn’t include any reverting transactions
- The sentinel ensures it’s simulations of the bundle are accurate by simulating it against the version of state where it will actually run (by optimistically applying the proposals produced for height `n` )

🗣️  **Gossip**

- Before the first proposal for height `n+1` is created, the Skip sentinel gossips the auction-winning bundle(s) to whichever nodes belonging to that proposer it can access (e.g. sentries if the validator is using a sentry configuration, or validator replicas if it’s using horcrux)
- The nodes that receive the winning bundle(s) gossip it to the other nodes belonging to that proposer to ensure the bundle(s) reach the validator
- This selective gossiping is powered by new config options (`personal_peer_ids`) and takes place over a new channel, but it is secured using the same authentication handshake Tendermint uses to secure all other forms of p2p communication

[reinforce that we have different channels on the same reactor]

🏒  **Handling Transactions**

- Ordinary transactions received over traditional gossip are handled exactly the same way they are today in the mempool
- Transactions received as part of bundles sent from the Skip sentinel are handled and stored in a new data structure called the `sidecar`
- These transactions have additional metadata about the bundle in which they should be included (e.g. bundleOrder, bundleSize). The sidecar uses this data to reconstruct bundles as it receives individual transactions over gossip

[reinforce that we have a new transaction data structure]

🚜  **Reaping** 

- On reap, mev-tendermint first checks whether there are any fully-constructed bundles in the sidecar then reaps these first.
- Next, it reaps from the ordinary mempool, with some additional checks to ensure that transactions reaped from the sidecar don’t get reaped again if they are also present in the standard mempool

[reinforce reaping of bundle goes to top if available]

## 🧱 Components

### **#1 The Sidecar**

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

---

## ䷾ Metrics

`mev-tendermint` exposes new **[prometheus metrics](https://docs.tendermint.com/v0.34/tendermint-core/metrics.html)** that you can use to supplement your dashboards (e.g. with Grafana)

**Metrics exposed:**

| Name | Type | Description |
| --- | --- | --- |
| mev_tx_size_bytes | Histogram | Histogram of mev transaction sizes, in bytes |
| mev_bundle_mempool_size | Gauge | Size of the MEV bundle mempool |
| mev_num_bundles_total | Counter | Number of MEV bundles received in total |
| mev_num_bundles_last_block | Gauge | Number of mev bundles received during the last block |
| mev_num_txs_total | Counter | Number of mev transactions added in total |
| mev_num_txs_last_block | Gauge | Number of mev transactions received in the last block |
| mev_sentinel_connected | Gauge | Whether or not a node is connected to the sentinel, 1 if connected, 0 if not. |