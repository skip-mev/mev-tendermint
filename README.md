
![](https://s3.us-west-2.amazonaws.com/secure.notion-static.com/33ea763f-bfa3-4c65-ad35-ad0ee1fd312d/Group_6.png?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Content-Sha256=UNSIGNED-PAYLOAD&X-Amz-Credential=AKIAT73L2G45EIPT3X45%2F20220921%2Fus-west-2%2Fs3%2Faws4_request&X-Amz-Date=20220921T004412Z&X-Amz-Expires=3600&X-Amz-Signature=da5c1352a65fb61d2fc143d8c1293689fe7e1cd21cd834516338f2812d66cf84&X-Amz-SignedHeaders=host&x-id=GetObject)


# mev-tendermint


_**The purpose of mev-tendermint is to expose a private mempool (the “sidecar”) containing atomic bundles of txs and gossips bundles of transactions specifically to the proposer of the next block.**_


---


### Design Goals


The design goals of MEV-Tendermint is to allow & preserve:

1. 🔒  **Privacy** for users submitting bundles
2. 🎁  **Atomicity** for bundles of transactions
3. 🐎  **Priority** guaranteed for highest paying bundles
4. 🏛  **No new security assumptions** for validators and nodes running MEV-Tendermint, including removing the need for ingress or egress for locked-down validators. No new network assumptions are made
5. 🔄  **On-chain transaction submission** via gossip, no need for off-chain submission like HTTP endpoints, endpoint querying, etc
6. 💨  **Impossible to slow down block time**, i.e. no part of mev-tendermint introduces consensus delays

### Components


**#1 The Sidecar**

- A separate, private mempool that respects `bundles` of transactions
	- Relevant files: `mempool/clist_sidecar.go`
- Has **selective gossiping**, meaning it only gossips:
	- Over its own `SidecarChannel`
	- **Only** to peers that are added as its `personal_peers`
		- In practice, `personal_peers` are set to be:
			- Sentry node → **validator** & **Skip sentinel**
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
