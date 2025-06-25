# RFQ Market Maker (MM) docs

## Overview

The **Request For Quote (RFQ)** system is built on top of the [**Celer Inter-Chain Message Framework (Celer IM)**](https://im-docs.celer.network/developer/celer-im-overview), enabling secure and efficient token swaps both within a single blockchain (intra-chain)** and **across multiple blockchains (inter-chain).

This document outlines the responsibilities and operational flow of the **Market Maker (MM)**, which provides quotes and fulfills orders for RFQ transactions.


## Outline

- [RFQ Basics](#rfq-basics)
  - [Reach an agreement](#reach-an-agreement)
  - [Swap on chain](#swap-on-chain)
    - [SrcDeposit](#srcdeposit)
    - [DstTransfer](#dsttransfer)
    - [SrcRelease](#srcrelease)
  - [Relayer](#relayer)
- [Become an MM](#become-an-mm)
  - [Request for MM qualification](#request-for-mm-qualification)
  - [Run MM application](#run-mm-application)
- [Default MM application](#default-mm-application)
  - [Installation](#installation)
  - [Configuration](#configuration)
  - [Running](#running)
- [Light MM application](#light-mm-application)
  - [Light MM installation](#light-mm-installation)
  - [Light MM Configuration](#light-mm-configuration)
  - [Light MM Running](#light-mm-running)
- [Customize your own MM application](#customize-your-own-mm-application)
  - [Customize subcomponents](#customize-subcomponents)
  - [Customize order processing](#customize-order-processing)
  - [Customize request serving](#customize-request-serving)



## RFQ Basics

A successful RFQ transaction consists of two main steps:

1. User and MM reach a quote agreement through off-chain communications via an RFQ Server.
2. User and MM execute the quote by swapping tokens through the RFQ contracts and Celer IM.

### Reach an agreement

```
 ╔══════╗                            ╔════════════╗                           ╔═════╗
 ║      ║                            ║            ║                           ║     ║
 ║      ║                            ║      R     ║ < = Supported Tokens < =  ║     ║
 ║      ║                            ║      F     ║                           ║     ║
 ║   U  ║ = > Request Quotation = >  ║      Q     ║ = > Price Request = > = > ║     ║
 ║   S  ║                            ║            ║                           ║  M  ║
 ║   E  ║ < = < Quotation = < = < =  ║      S     ║ < = Price Response < = <  ║  M  ║
 ║   R  ║                            ║      E     ║                           ║  s  ║
 ║      ║ = > Confirm quotation = >  ║      V     ║ = > Quote Request = > = > ║     ║
 ║      ║                            ║      E     ║                           ║     ║
 ║      ║                            ║      R     ║ < = Quote Response < = <  ║     ║
 ║      ║                            ║            ║                           ║     ║
 ╚══════╝                            ╚════════════╝                           ╚═════╝
```

>Prerequisite: All MMs should report their supported tokens in a list to RFQ Server via [UpdateConfigs API](./sdk.md#func-client-updateconfigs)
once after MM is ready.

1. User requests quotation from RFQ Server for a possible swap: token X on chain A -> token Y on chain B
2. RFQ Server receives the request and checks all MMs' token configs to determine who can fulfill the swap. It then sends a [PriceRequest](./sdk.md#message-pricerequest) to all available MMs.
3. MM receives the PriceRequest and calculates how much token Y on chain B it is willing to pay for token X on chain A, based on its fee strategy. The MM returns a price response to the RFQ Server, including:
    - the quoted amount,
    - a signature of the response,
    - a validity period for the price.
4. RFQ Server collects responses and selects the MM offering the **highest amount of token Y on chain B**. The best price response is returned to the User as the quotation.
5. If the User accepts the quotation, they confirm it through the RFQ Server.
6. Upon confirmation, RFQ Server sends a [QuoteRequest](./sdk.md#message-quoterequest) to the selected MM. This request includes:
    - the MM’s signed price response,
    - a suggested `SrcDeadline` by which the User should lock token X on chain A,
    - a suggested `DstDeadline` by which the MM should transfer token Y to the User on chain B.
7. MM receives the QuoteRequest and verifies:
    - the signature is valid,
    - the price is still within its validity period,
    - `SrcDeadline` and `DstDeadline` are acceptable,
    - (optional) it has sufficient token Y and freezes it.

    If all checks pass, MM signs the quotation and returns the signature to the RFQ Server for later verification.

Once a valid quote response is signed by the MM and returned, an agreement between the User and MM is established.

### Swap on chain
#### SrcDeposit
```
 ──────────────────────────────────────────────┬───────────────────────────────────────────────────
 CHAIN A                                       │                                            CHAIN B
 ┏━━━━┓                     ┏━━━━━━━━━━━━━┓    │    ┏━━━━━━━━━━━━━┓                    ┏━━━━┓            
 ┃USER┃ > = srcDeposit >  > ┃     RFQ     ┃    │    ┃     RFQ     ┃                    ┃ MM ┃
 ┗━━━━┛                     ┗━━━━━━━━━━━━━┛    │    ┗━━━━━━━━━━━━━┛                    ┗━━━━┛
                                   ∨ send      │                                          ∧
                                   v message1  │                                          ∧
                            ┏━━━━━━━━━━━━━┓    │    ┏━━━━━━━━━━━━━┓                       ∧ inform MM:
                            ┃ Message Bus ┃    │    ┃ Message Bus ┃                       ∧ User has
                            ┗━━━━━━━━━━━━━┛    │    ┗━━━━━━━━━━━━━┛                       ∧ deposited
                                   v           │                                          ∧
 ──────────────────────────────────────────────┴───────────────────────────────────────────────────
                                   v listened by sgn                                      ∧
                             ╔═══════════════════════════════════╗                   ╔════════════╗
                             ║   SGN (State Guardian Network)    ║ > query message > ║ RFQ Server ║
                             ╚═══════════════════════════════════╝                   ╚════════════╝                                              
                                                                                      
```
After the User confirms a quotation, they must deposit token X to the RFQ contract on chain A by calling [`srcDeposit`](https://github.com/celer-network/intent-rfq-contract/blob/4b482dcff6b775002d726a33835a9258378c647a/src/RFQ.sol#L90).

During `srcDeposit`, a message is sent via the **Message Bus**, the core contract of Celer IM. This message is picked up by SGN through an event listener and co-signed by SGN validators. Once the message reaches sufficient voting power, the RFQ Server can fetch it from SGN and mark the corresponding swap as `OrderStatus.STATUS_SRC_DEPOSITED`.

The selected MM is then informed (via polling; see [`PendingOrders`](./sdk.md#func-client-pendingorders)) that the User has deposited on chain A. Along with this, the signature generated by the MM in step 7 is returned to the MM for verification.


#### DstTransfer
```
 ──────────────────────────────────────────────┬───────────────────────────────────────────────────
 CHAIN A                                       │                                            CHAIN B
                                               │                                       ┏━━━━┓
                                               │          > = > transfer token > = > > ┃USER┃
                                               │          ∧                            ┗━━━━┛
 ┏━━━━┓                     ┏━━━━━━━━━━━━━┓    │    ┏━━━━━━━━━━━━━┓                    ┏━━━━┓          
 ┃ MM ┃                     ┃     RFQ     ┃    │    ┃     RFQ     ┃ <  dstTransfer < < ┃ MM ┃
 ┗━━━━┛                     ┗━━━━━━━━━━━━━┛    │    ┗━━━━━━━━━━━━━┛                    ┗━━━━┛
   ∧                                           │          ∨ send                          
   ∧ inform mm                                 │          v message2                       
   ∧ and give him           ┏━━━━━━━━━━━━━┓    │    ┏━━━━━━━━━━━━━┓                       
   ∧ a proof                ┃ Message Bus ┃    │    ┃ Message Bus ┃                        
   ∧                        ┗━━━━━━━━━━━━━┛    │    ┗━━━━━━━━━━━━━┛                       
   ∧                                           │          v                               
 ──────────────────────────────────────────────┴───────────────────────────────────────────────────
   ∧                                    listened by sgn   v                               
 ╔════════════╗              ╔═══════════════════════════════════╗                   
 ║ RFQ Server ║  < message < ║   SGN (State Guardian Network)    ║ 
 ╚════════════╝              ╚═══════════════════════════════════╝                                                                 
                                                                                      
```
When MM is informed that the User has deposited, MM should:
- Verify the signature of the quotation to ensure it matches the one MM signed earlier.
- Double-check the validity of the information from the RFQ Server (e.g., verify on-chain that the User has indeed deposited token X on chain A), in case of a compromised or malicious server.

If everything is valid, MM can call [`dstTransfer`](https://github.com/celer-network/intent-rfq-contract/blob/4b482dcff6b775002d726a33835a9258378c647a/src/RFQ.sol#L145) to transfer token Y to the User on chain B.During `dstTransfer`:
- A message is sent via the MessageBus contract, picked up by SGN, and co-signed by SGN validators.
- A certain amount of token Y is transferred from the MM and sent to the User after the message is successfully sent.

>NOTE:  light MM can optionally delegate the transaction submission on the destination chain to a central relayer service.

Once the message has sufficient voting power, the RFQ Server can fetch it from SGN and mark the corresponding swap as `OrderStatus.STATUS_DST_TRANSFERRED`.

The chosen MM will then be informed (via polling; see [`PendingOrders`](./sdk.md#func-client-pendingorders)) that `dstTransfer` is successful and `srcRelease` is available on chain A to release the token.

Since a proof of order fulfillment generated by SGN is required to release the token, the RFQ Server will also deliver this proof to the MM.

 
#### SrcRelease
```
 ──────────────────────────────────────────────┬───────────────────────────────────────────────────
 CHAIN A                                       │                                            CHAIN B
    < < < < < transfer token < < < <           │
    v                              ∧           │
 ┏━━━━┓                     ┏━━━━━━━━━━━━━┓    │    ┏━━━━━━━━━━━━━┓                                
 ┃ MM ┃ > = srcRelease >  > ┃     RFQ     ┃    │    ┃     RFQ     ┃                    
 ┗━━━━┛                     ┗━━━━━━━━━━━━━┛    │    ┗━━━━━━━━━━━━━┛                    
                                   ∨ verify    │                                          
                                   v proof     │                                          
                            ┏━━━━━━━━━━━━━┓    │    ┏━━━━━━━━━━━━━┓                        
                            ┃ Message Bus ┃    │    ┃ Message Bus ┃                     
                            ┗━━━━━━━━━━━━━┛    │    ┗━━━━━━━━━━━━━┛                       
                                               │                                          
 ──────────────────────────────────────────────┴───────────────────────────────────────────────────
                                                                        
```
When MM receives the proof of order fulfillment, it can call [`srcRelease`](https://github.com/celer-network/intent-rfq-contract/blob/4b482dcff6b775002d726a33835a9258378c647a/src/RFQ.sol#L222) to release token X on chain A.

During `srcRelease`, the proof is verified via the MessageBus contract. If all checks pass, the locked token X deposited by the User is transferred to the MM, after deducting the RFQ protocol fee.

>NOTE:  light MM can optionally delegate the transaction submission on the source chain to a central relayer service.

Then the on-chain swap between the User and MM is completed.

### Relayer
The **relayer** is a central service hosted by the Celer Intent protocol to assist MMs who:
- Do not want to request the RFQ Server for token reporting or to fetch pending orders.
- Do not want to send any transactions on either the destination or source chain.

As a consequence of having the relayer submit transactions on behalf of MMs:
- The MM must expose additional APIs to complete the swap flow, including an API for signing specific data.
- Base fees (i.e., transaction gas costs and message fees) are charged by the Celer Intent protocol instead of the MM, and are accumulated in the RFQ contract.

The relayer's working mechanism is simple and based on MM's signature verification.

Instead of directly sending tokens to the User, the MM signs the quote's hash to authorize a third party to transfer tokens from its address to the User. The relayer can then use this signature to call the RFQ contract via `dstTransferWithSig`, where the MM's signature is verified and the tokens are transferred from MM to the User.

The relayer also assists in releasing tokens on the source chain for the MM. This requires no additional permissions, since `srcRelease` is designed to be callable by anyone. Regardless of who calls it, the tokens are always released to the MM's address.


## Become an MM

### Request for MM qualification
An API key is needed for an MM to use RFQ Server's services. Contact us for requesting an API key.

### Run MM application
For default MM application, see the guide at [Default MM Application](#default-mm-application).

For light MM application, see the guide at [Light MM Application](#light-mm-application).

For customized MM application, run it as you preferred.

## Default MM application

### Installation

1. Download `intent-rfq-mm`
```
git clone https://github.com/celer-network/intent-rfq-mm.git
cd intent-rfq-mm
```
2. Build `intent-rfq-mm`
```
make install
```

### Configuration
Make a new folder to store your configuration file and ETH keystore file.
```
mkdir .intent-rfq-mm
cd .intent-rfq-mm
mkdir config eth-ks
touch config/chain.toml config/lp.toml config/fee.toml config/mm.toml
// move all used address's keystore file into .intent-rfq-mm/eth-ks/
mv <path-to-your-eth-keystore-file> eth-ks/<give-a-name>.json
```
The `.intent-rfq-mm` folder's structure will look like:
```
.intent-rfq-mm/
  - config/
      - chain.toml
      - lp.toml
      - fee.toml
      - mm.toml
  - eth-ks/
      - <give-a-name>.json
      - <give-b-name>.json
```

1. Chain configuration

Each chain is configured by a `multichain`.
Take Arbitrum Sepolia as an example. Before using, don't forget to update `chainId`, `name`, and fill up `gateway` and `rfq`.
RFQ contract address could be found at [Information](#information).

```
[[multichain]]
chainID = 5
name = "Arbitrum Sepolia"
gateway = "<your-rpc>" # fill in your Arbitrum Sepolia rpc provider url
rfq = "<copy-addr-from-'Support->Contract address'>"
blkdelay = 5 # how many blocks confirmations are required
blkinterval = 15 # polling interval for querying tx's status
# belows are optional transaction options
# maxfeepergasgwei = 10 # acceptable max fee price
# maxPriorityFeePerGasGwei = 2 # acceptable max priority fee price
# gaslimit = 200000 # fix gas limit and skip gas estimation, often used for debuging
# addgasestimateratio = 0.3 # adjust result from gas estimation, actual gasLimit = (1+addgasestimateratio)*estimation 
[multichain.native]
symbol = "ETH"
# if any liquidity of native token or wrapped native token on this chain is configured in lp.toml, this address should 
# be set, and set to wrapped native token address.
address = "<wrapped-native-token-address>"
decimals = 18
```

Transaction options are used in condition. Normally, if you got some error about "out of gas", we recommend using
`addgasestimateratio` with value `0.3` at first, and gradually increase its value if the error still occurs.

A dubug tip: if you got any error not about gas during pre-running, and you don't figure out the reason, try give a `gaslimit`
that is big enough. After the transaction is sent, debug it in [Tenderly](https://dashboard.tenderly.co/).

2. Liquidity configuration

Liquidity is configured per chain and per token. An example full configuration of liquidity on Arbitrum Sepolia is:
```
[[lp]]
chainid = 5
address = "<lp address>"
# if this lp is a contract, should keep keystore unset or empty string
keystore = "./eth-ks/<give-a-name>.json"
passphrase = "<password-of-your-keystore>"
# release native token or wrapped native token on this chain, used when the token deposited by User is native token or wrapped native token
releasenative = false
[[lp.liqs]]
address = ""
symbol = "USDT"
# token's available amount. if not set, would query the current token balance during initialization
amount = "5000000000"
# the amount of token to be approved to RFQ contract during initialization
approve = "1000000000000"
decimals = 6
# how long you prefer to freeze this token. The unit is second.
freezetime = 300
[[lp.liqs]]
# address of full `f` represents native token
address = "0xffffffffffffffffffffffffffffffffffffffff"
symbol = "ETH"
#amount = "0"
decimals = 18
freezetime = 200
```
You can use different account for each chain or just use one account for all chains. For a local account, fill `keystore` and `passphrase`. `keystore` should be set to path of your
keystore file relative to `.intent-rfq-mm` floder. For an external account, read the following guide.
>**EXTERNAL ACCOUNT GUIDE.** First of all, this is a feature of light-MM. Since default MM will send tx for itself, a `lp` with hot key is required. (*External accounts as `lp`s with one specific local account for sending txs is possible. Customize your own MM and welcome any PRs*.) To use an external account(e.g. a contract that holds tokens), let `keystore` and `passphrase` be empty and set `address` to the address of the external account. **Most importantly, somehow let this external account call [registerAllowedSigner](https://github.com/celer-network/intent-rfq-contract/blob/4b482dcff6b775002d726a33835a9258378c647a/src/RFQ.sol#L319) of RFQ contract on specific chain with the address of `requestsigner`.**

For each token, `address`, `symbol`, `decimals` and `freezetime` are required, while `amount` and `approve` are optional.

- `freezetime`: How long the MM prefer to freeze a token. Take 300 as example. It means, counting from the User confirm a 
quotation, he should finish depositing token within 300 second.
- `amount`: How much token the MM could supply. MM can set it to any value regardless of current token balance. If it is
not set, current token balance would be used instead.
- `approve`: How much token will be approved to RFQ contract. If it is set, transaction would be sent during initialization
for approving. *Once MM has approved sufficient amount, remove this field to prevent re-approve.*
- `address`: Token address. In particular, `0xffffffffffffffffffffffffffffffffffffffff` is used to represent native token.
*If native token is configured for one chain, relatively `multichain.native.address` must be set and set to wrapped native token address*.

3. Fee configuration

Fee strategy is configured globally with overrides per chain pair and per token pair. An example full configuration of 
fee is:
```
[fee]
# how much gas of dst chain you wanna charge, should be higher than actual consumption
dstgascost = 100000
# how much gas of src chain you wanna charge, should be higher than actual consumption
srcgascost = 150000
# global percentage fee, 100% = 1000000
percglobal = 1000

[[fee.gasprices]]
chainid = 5
# how much wei you wanna charge for each gas consumed when failed to call eth_gasPrice 
price = 5000000000

[[fee.gasprices]]
chainid = 97
# how much wei you wanna charge for each gas consumed when failed to call eth_gasPrice
price = 7000000000

[[fee.chainoverrides]]
# override percentage fee from srcchainid to dstchainid
srcchainid = 5
dstchainid = 97
perc = 2000

[[fee.tokenoverrides]]
# override percentage fee from srctoken on srcchainid to dsttoken on dstchainid
srcchainid = 5
srctoken = "0xf4B2cbc3bA04c478F0dC824f4806aC39982Dce73"
dstchainid = 97
dsttoken = "0x7d43AABC515C356145049227CeE54B608342c0ad"
perc = 3000
```

Except of `fee.chainoverrides` and `fee.tokenoverrides`, all the other fields are required for fee configuration. Generally,
MM needs to separately send one tx on dst and src chain, in order to complete a swap order. That's  why we need
to configure `fee.dstgascost`, `fee.srcgascost` and `fee.gasprice`. The actual charged fee value to cover gas consumption
on two chains will be
`fee.dstgascost * <current-gasprice-on-dst> * <current-native-token-price-in-wei> + fee.srcgascost * <current-gasprice-on-src> * <current-native-token-price-in-wei>`. 
At last, the fee value will be converted to token amount which is deducted from the amount of token transferred to User.

4. MM configuration

This configuration contains several important parameters related to MM application's operation.
```
[priceprovider]
# url required by default price provider implementation
url = "https://cbridge-stat.s3.us-west-2.amazonaws.com/prod2/cbridge-price.json"

[rfqserver]
url = "<url-of-rfq-server>"
apikey = "<your-api-key>"

[requestsigner]
# indicates which chain's signer will be used as request signer
chainid = 5
# Optional. if keystore(file path) is not empty, then the address denoted by the keystore will be used as request signer.
keystore = ""
passphrase = ""

[mm]
# token pair policy list indicates from which token to which token the mm is interested in
tpPolicyList = ["All"]
# grpc port that mm listens on
grpcPort = 5555
# restful api port that mm listens on
grpcGatewayPort=6666
# all periods' unit is second
# indicates the period during which a price response from this mm is valid
priceValidPeriod = 300
# indicates the minimum period for this mm to complete transferring on dst chain, counting from the user confirms the quotation
dstTransferPeriod = 600
# if faled to report token configs to rfq server, mm will be stucked and retry every <reportperiod> seconds until success.
reportRetryPeriod = 5
# time interval for getting and processing pending orders from rfq server
processPeriod = 5
# indicates whether this mm is light versioned
lightMM = false
# change host to "0.0.0.0" in need
host="localhost"
```
Token pair policy format can be found in [SDK doc](./sdk.md#func-defaultliquidityprovider-setuptokenpairs).

Do not modify `priceprovider.url`. A large json format data of token prices stored under `priceprovider.url`, and is updated
periodically by other external process. At present, it's the only implementation of price service within default MM application.
If you're not comfortable with this implementation, you can either try to customize your own MM application or waiting for
later updates of default MM application.

Get `rfqserver.url` at [Information](#information) and fill up your API key.

As mentioned before, MM should be able to sign any data and verify own signatures. Beside of that, MM should also be able to sign quote hash if it's light versioned. `requestsigner` is just the signer who is responsible for those tasks. To configure this signer, you'll have the following choices:
- only set `requestsigner.chainid`. Then MM will use the specific `lp` in `lp.toml` as signer. Since it's a signer, this `lp` shouldn't be an external account(`lp.keystore` and `lp.passphrase` are unset).
- only set `requestsigner.keystore` and `requestsigner.passphrase`. This is needed often when the MM is light versioned and `lp` is external account. 
>**IMPORTANT.** If the MM is light versioned, this configured signer will be used to sign quote hash for transferring tokens on behalf of `lp`. So make sure that **!! all !!** `lp` whose address is different with address of the configured signer have allowed the signer to do so. To give allowance, let `lp` call [registerAllowedSigner](https://github.com/celer-network/intent-rfq-contract/blob/4b482dcff6b775002d726a33835a9258378c647a/src/RFQ.sol#L319) of RFQ contract on specific chain.

### Running

Create a intent-rfq-mm system service
```
touch /etc/systemd/system/intent-rfq-mm.service
# create the log directory for executor
mkdir -p /var/log/intent-rfq-mm
```

>IMPORTANT: check if the user, user group and paths defined in your systemd file are correct.

```
# intent-rfq-mm.service

[Unit]
Description=Default MM application
After=network-online.target

[Service]
Environment=HOME=/home/ubuntu
ExecStart=/home/ubuntu/go/bin/intent-rfq-mm start \
  --home /home/ubuntu/.intent-rfq-mm/ \
  --logdir /var/log/intent-rfq-mm/app --loglevel debug
StandardError=append:/var/log/intent-rfq-mm/error.log
Restart=always
RestartSec=10
User=ubuntu
Group=ubuntu
LimitNOFILE=4096

[Install]
WantedBy=multi-user.target
```
Enable and start executor service
```
sudo systemctl enable intent-rfq-mm
sudo systemctl start intent-rfq-mm
// Check if logs look ok
tail -f -n30 /var/log/intent-rfq-mm/app/<log-file-with-start-time>.log
```

## Light MM application

The difference between Light MM and Default MM:
* Light MM will not actively send any request to RFQ server.
* Light MM will serve more api request. One for signing quote hash, and one for providing supported tokens' info.
* Light MM will not send any tx on chain by himself. Tx for `dstTransfer` and `srcRelease` will be sent by Relayer.
* Light MM will not charge tx gas cost and message fee. It will charged by Celer Intent protocol.

### Light MM Installation

See [installation](#installation). 

Before building, switch to `light-mm` branch. If this branch has already been merged to `main`, then the switch operation
is not needed.

### Light MM Configuration

Follow the [configuration](#configuration) of Default MM, and make following changes:
* Set `lightMM` in `mm.toml` to `true`
* `rfqserver` in `mm.tonl` is no longer needed
* `fee.dstgascost`, `fee.srcgascost` and `fee.gasprices` in `fee.toml` are no longer needed
* (In need) If request signer is different with liquidity provider, then liquidity provider is required to call 
`RegisterAllowedSigner` at RFQ contract to make request signer's signature valid.

### Light MM Running

See [running](#running).

## Customize your own MM application

With the [SDK](./sdk.md#sdk), the way to customize your own MM application is totally up to yourself, as long as it meets
minimum requirements:
* Implement [ApiServer](./sdk.md#interface-apiserver)
* Utilize [RFQ Client](./sdk.md#type-client) to report supported tokens to RFQ Server
* Utilize [RFQ Client](./sdk.md#type-client) to get pending orders, process orders, and update orders

Besides, MM is suggested to have the ability of customizing fee and managing his liquidity on different chain, which includes but not limited
to:

- flexible fee configuration
- freeze and unfreeze requested token at appropriate time
- reuse the just released token for next swap orders.
- withdraw and add liquidity from/to remote liquidity pool if needed (It's not supported now by default MM application)

At last, do not forget to start serving requests, for example:
```go
yourMMApp := NewYourMMApp(...)
listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d",host, port))
if err != nil {
	panic(err)
}
grpcServer := grpc.NewServer(ops...)
rfqmmproto.RegisterApiServer(grpcServer, yourMMApp)
grpcServer.Serve(listener)
```

But if you think the structure of [Server](./sdk.md#type-server) is ok, then you can only customize its subcomponents that
you want to change. With this Server, you can still customize how does it serve price&quote requests and process orders.

### Customize subcomponents
There are four subcomponents, which are:
* [Chain Querier](./sdk.md#interface-chainquerier)
* [Liquidity Provider](./sdk.md#interface-liquidityprovider)
* [Amount Calculator](./sdk.md#interface-amountcalculator)
* [Request Signer](./sdk.md#interface-requestsigner)

Click on each to see the interface detail.

### Customize order processing
Requirements:
* [Validate Quote](./sdk.md#func-server-validatequote) for each order before processing it 
* double check any information comes from RFQ Server before transferring out token, especially for User has deposited
* stop processing order if there is any unhandled error
* as a consequence of RFQ Server help MMs maintain orders, an MM should timely update the status of an order through
  [UpdateOrders API](./sdk.md#func-client-updateorders). The most important times to call this api are:
  1. update to `OrderStatus_STATUS_MM_REJECTED` at any appropriate time when the MM thinks he should reject this order before any token transfer on dst chain.
  2. update to `OrderStatus_STATUS_MM_DST_EXECUTED` when the MM has sent a tx on dst chain for transferring token to the User, regardless of whether it's mined or not and its execution status.
  3. update to `OrderStatus_STATUS_MM_SRC_EXECUTED` when the MM has sent a tx on src chain for releasing token to himself, regardless of whether it's mined or not and its execution status.
  4. specially, update order from `OrderStatus_STATUS_SRC_DEPOSITED` to `OrderStatus_STATUS_REFUND_INITIATED` when it's a 
same chain swap `quote.GetSrcChainId() == quote.GetDstChainId()` and `quote.DstDeadline` has passed.


Example:
```go
// server := NewServer(...)
if server.Ctl == nil {
    log.Panicln("nil control channel")
}
ticker := time.NewTicker(time.Duration(server.Config.ProcessPeriod) * time.Second)
for {
    select {
    case <-ticker.C:
    // check component's functionality
    if server.LiquidityProvider.IsPaused() {
        server.StopProcessing("liquidity provider is paused in some reason")
        continue
    }
    resp, err := server.RfqClient.PendingOrders(context.Background(), &rfqproto.PendingOrdersRequest{})
    if err != nil {
        // handler err
        continue
    }
	// your customized processOrders
    // processOrders(server, resp.Orders)
    case <-server.Ctl:
        return
    }
}
```

### Customize request serving
If you want to customize request serving, you'd better package Server into a new structure. So that you can implement new
Price and Quote, and share the subcomponents of Server at the same time.

Example
```go
type YourMMApp struct {
    Server *rfqmm.Server
}
func (mm *YourMMApp) Price(ctx context.Context, request *proto.PriceRequest) (response *proto.PriceResponse, err error) {
    // todo, remove panic() and write your own implementation
    panic()
}
func (mm *YourMMApp) Quote(ctx context.Context, request *proto.QuoteRequest) (response *proto.QuoteResponse, err error) {
    // todo, remove panic() and write your own implementation
    panic()
}
func (mm *YourMMApp) Serve(ops ...grpc.ServerOption) {
    port := mm.Server.Config.GrpcPort
    host := mm.Server.Config.Host
    listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, port))
    if err != nil {
        panic(err)
    }
    grpcServer := grpc.NewServer(ops...)
    proto.RegisterApiServer(grpcServer, mm)
    grpcServer.Serve(listener)
}
```
