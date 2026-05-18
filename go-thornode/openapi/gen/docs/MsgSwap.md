# MsgSwap

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Tx** | [**Tx**](Tx.md) |  | 
**TargetAsset** | **string** | the asset to be swapped to | 
**Destination** | Pointer to **string** | the destination address to receive the swap output | [optional] 
**TradeTarget** | **string** | the minimum amount of output asset to receive (else cancelling and refunding the swap) | 
**AffiliateAddress** | Pointer to **string** | the affiliate address which will receive any affiliate fee | [optional] 
**AffiliateBasisPoints** | **string** | the affiliate fee in basis points | 
**Signer** | Pointer to **string** | the signer (sender) of the transaction | [optional] 
**Aggregator** | Pointer to **string** | the contract address if an aggregator is specified for a non-THORChain SwapOut | [optional] 
**AggregatorTargetAddress** | Pointer to **string** | the desired output asset of the aggregator SwapOut | [optional] 
**AggregatorTargetLimit** | Pointer to **string** | the minimum amount of SwapOut asset to receive (else cancelling the SwapOut and receiving THORChain&#39;s output) | [optional] 
**SwapType** | Pointer to **string** | market if immediately completed or refunded, limit if held until fulfillable | [optional] 
**StreamQuantity** | Pointer to **int64** | number of swaps to execute in a streaming swap | [optional] 
**StreamInterval** | Pointer to **int64** | the interval (in blocks) to execute the streaming swap | [optional] 
**InitialBlockHeight** | Pointer to **int64** | the initial block height when the streaming swap was first queued | [optional] 
**State** | Pointer to [**SwapState**](SwapState.md) |  | [optional] 
**Version** | Pointer to **string** | the version of the swap (v1 or v2) | [optional] 
**Index** | Pointer to **int32** | the index of the swap in the batch | [optional] 

## Methods

### NewMsgSwap

`func NewMsgSwap(tx Tx, targetAsset string, tradeTarget string, affiliateBasisPoints string, ) *MsgSwap`

NewMsgSwap instantiates a new MsgSwap object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewMsgSwapWithDefaults

`func NewMsgSwapWithDefaults() *MsgSwap`

NewMsgSwapWithDefaults instantiates a new MsgSwap object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetTx

`func (o *MsgSwap) GetTx() Tx`

GetTx returns the Tx field if non-nil, zero value otherwise.

### GetTxOk

`func (o *MsgSwap) GetTxOk() (*Tx, bool)`

GetTxOk returns a tuple with the Tx field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetTx

`func (o *MsgSwap) SetTx(v Tx)`

SetTx sets Tx field to given value.


### GetTargetAsset

`func (o *MsgSwap) GetTargetAsset() string`

GetTargetAsset returns the TargetAsset field if non-nil, zero value otherwise.

### GetTargetAssetOk

`func (o *MsgSwap) GetTargetAssetOk() (*string, bool)`

GetTargetAssetOk returns a tuple with the TargetAsset field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetTargetAsset

`func (o *MsgSwap) SetTargetAsset(v string)`

SetTargetAsset sets TargetAsset field to given value.


### GetDestination

`func (o *MsgSwap) GetDestination() string`

GetDestination returns the Destination field if non-nil, zero value otherwise.

### GetDestinationOk

`func (o *MsgSwap) GetDestinationOk() (*string, bool)`

GetDestinationOk returns a tuple with the Destination field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetDestination

`func (o *MsgSwap) SetDestination(v string)`

SetDestination sets Destination field to given value.

### HasDestination

`func (o *MsgSwap) HasDestination() bool`

HasDestination returns a boolean if a field has been set.

### GetTradeTarget

`func (o *MsgSwap) GetTradeTarget() string`

GetTradeTarget returns the TradeTarget field if non-nil, zero value otherwise.

### GetTradeTargetOk

`func (o *MsgSwap) GetTradeTargetOk() (*string, bool)`

GetTradeTargetOk returns a tuple with the TradeTarget field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetTradeTarget

`func (o *MsgSwap) SetTradeTarget(v string)`

SetTradeTarget sets TradeTarget field to given value.


### GetAffiliateAddress

`func (o *MsgSwap) GetAffiliateAddress() string`

GetAffiliateAddress returns the AffiliateAddress field if non-nil, zero value otherwise.

### GetAffiliateAddressOk

`func (o *MsgSwap) GetAffiliateAddressOk() (*string, bool)`

GetAffiliateAddressOk returns a tuple with the AffiliateAddress field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetAffiliateAddress

`func (o *MsgSwap) SetAffiliateAddress(v string)`

SetAffiliateAddress sets AffiliateAddress field to given value.

### HasAffiliateAddress

`func (o *MsgSwap) HasAffiliateAddress() bool`

HasAffiliateAddress returns a boolean if a field has been set.

### GetAffiliateBasisPoints

`func (o *MsgSwap) GetAffiliateBasisPoints() string`

GetAffiliateBasisPoints returns the AffiliateBasisPoints field if non-nil, zero value otherwise.

### GetAffiliateBasisPointsOk

`func (o *MsgSwap) GetAffiliateBasisPointsOk() (*string, bool)`

GetAffiliateBasisPointsOk returns a tuple with the AffiliateBasisPoints field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetAffiliateBasisPoints

`func (o *MsgSwap) SetAffiliateBasisPoints(v string)`

SetAffiliateBasisPoints sets AffiliateBasisPoints field to given value.


### GetSigner

`func (o *MsgSwap) GetSigner() string`

GetSigner returns the Signer field if non-nil, zero value otherwise.

### GetSignerOk

`func (o *MsgSwap) GetSignerOk() (*string, bool)`

GetSignerOk returns a tuple with the Signer field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSigner

`func (o *MsgSwap) SetSigner(v string)`

SetSigner sets Signer field to given value.

### HasSigner

`func (o *MsgSwap) HasSigner() bool`

HasSigner returns a boolean if a field has been set.

### GetAggregator

`func (o *MsgSwap) GetAggregator() string`

GetAggregator returns the Aggregator field if non-nil, zero value otherwise.

### GetAggregatorOk

`func (o *MsgSwap) GetAggregatorOk() (*string, bool)`

GetAggregatorOk returns a tuple with the Aggregator field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetAggregator

`func (o *MsgSwap) SetAggregator(v string)`

SetAggregator sets Aggregator field to given value.

### HasAggregator

`func (o *MsgSwap) HasAggregator() bool`

HasAggregator returns a boolean if a field has been set.

### GetAggregatorTargetAddress

`func (o *MsgSwap) GetAggregatorTargetAddress() string`

GetAggregatorTargetAddress returns the AggregatorTargetAddress field if non-nil, zero value otherwise.

### GetAggregatorTargetAddressOk

`func (o *MsgSwap) GetAggregatorTargetAddressOk() (*string, bool)`

GetAggregatorTargetAddressOk returns a tuple with the AggregatorTargetAddress field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetAggregatorTargetAddress

`func (o *MsgSwap) SetAggregatorTargetAddress(v string)`

SetAggregatorTargetAddress sets AggregatorTargetAddress field to given value.

### HasAggregatorTargetAddress

`func (o *MsgSwap) HasAggregatorTargetAddress() bool`

HasAggregatorTargetAddress returns a boolean if a field has been set.

### GetAggregatorTargetLimit

`func (o *MsgSwap) GetAggregatorTargetLimit() string`

GetAggregatorTargetLimit returns the AggregatorTargetLimit field if non-nil, zero value otherwise.

### GetAggregatorTargetLimitOk

`func (o *MsgSwap) GetAggregatorTargetLimitOk() (*string, bool)`

GetAggregatorTargetLimitOk returns a tuple with the AggregatorTargetLimit field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetAggregatorTargetLimit

`func (o *MsgSwap) SetAggregatorTargetLimit(v string)`

SetAggregatorTargetLimit sets AggregatorTargetLimit field to given value.

### HasAggregatorTargetLimit

`func (o *MsgSwap) HasAggregatorTargetLimit() bool`

HasAggregatorTargetLimit returns a boolean if a field has been set.

### GetSwapType

`func (o *MsgSwap) GetSwapType() string`

GetSwapType returns the SwapType field if non-nil, zero value otherwise.

### GetSwapTypeOk

`func (o *MsgSwap) GetSwapTypeOk() (*string, bool)`

GetSwapTypeOk returns a tuple with the SwapType field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSwapType

`func (o *MsgSwap) SetSwapType(v string)`

SetSwapType sets SwapType field to given value.

### HasSwapType

`func (o *MsgSwap) HasSwapType() bool`

HasSwapType returns a boolean if a field has been set.

### GetStreamQuantity

`func (o *MsgSwap) GetStreamQuantity() int64`

GetStreamQuantity returns the StreamQuantity field if non-nil, zero value otherwise.

### GetStreamQuantityOk

`func (o *MsgSwap) GetStreamQuantityOk() (*int64, bool)`

GetStreamQuantityOk returns a tuple with the StreamQuantity field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetStreamQuantity

`func (o *MsgSwap) SetStreamQuantity(v int64)`

SetStreamQuantity sets StreamQuantity field to given value.

### HasStreamQuantity

`func (o *MsgSwap) HasStreamQuantity() bool`

HasStreamQuantity returns a boolean if a field has been set.

### GetStreamInterval

`func (o *MsgSwap) GetStreamInterval() int64`

GetStreamInterval returns the StreamInterval field if non-nil, zero value otherwise.

### GetStreamIntervalOk

`func (o *MsgSwap) GetStreamIntervalOk() (*int64, bool)`

GetStreamIntervalOk returns a tuple with the StreamInterval field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetStreamInterval

`func (o *MsgSwap) SetStreamInterval(v int64)`

SetStreamInterval sets StreamInterval field to given value.

### HasStreamInterval

`func (o *MsgSwap) HasStreamInterval() bool`

HasStreamInterval returns a boolean if a field has been set.

### GetInitialBlockHeight

`func (o *MsgSwap) GetInitialBlockHeight() int64`

GetInitialBlockHeight returns the InitialBlockHeight field if non-nil, zero value otherwise.

### GetInitialBlockHeightOk

`func (o *MsgSwap) GetInitialBlockHeightOk() (*int64, bool)`

GetInitialBlockHeightOk returns a tuple with the InitialBlockHeight field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetInitialBlockHeight

`func (o *MsgSwap) SetInitialBlockHeight(v int64)`

SetInitialBlockHeight sets InitialBlockHeight field to given value.

### HasInitialBlockHeight

`func (o *MsgSwap) HasInitialBlockHeight() bool`

HasInitialBlockHeight returns a boolean if a field has been set.

### GetState

`func (o *MsgSwap) GetState() SwapState`

GetState returns the State field if non-nil, zero value otherwise.

### GetStateOk

`func (o *MsgSwap) GetStateOk() (*SwapState, bool)`

GetStateOk returns a tuple with the State field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetState

`func (o *MsgSwap) SetState(v SwapState)`

SetState sets State field to given value.

### HasState

`func (o *MsgSwap) HasState() bool`

HasState returns a boolean if a field has been set.

### GetVersion

`func (o *MsgSwap) GetVersion() string`

GetVersion returns the Version field if non-nil, zero value otherwise.

### GetVersionOk

`func (o *MsgSwap) GetVersionOk() (*string, bool)`

GetVersionOk returns a tuple with the Version field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetVersion

`func (o *MsgSwap) SetVersion(v string)`

SetVersion sets Version field to given value.

### HasVersion

`func (o *MsgSwap) HasVersion() bool`

HasVersion returns a boolean if a field has been set.

### GetIndex

`func (o *MsgSwap) GetIndex() int32`

GetIndex returns the Index field if non-nil, zero value otherwise.

### GetIndexOk

`func (o *MsgSwap) GetIndexOk() (*int32, bool)`

GetIndexOk returns a tuple with the Index field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetIndex

`func (o *MsgSwap) SetIndex(v int32)`

SetIndex sets Index field to given value.

### HasIndex

`func (o *MsgSwap) HasIndex() bool`

HasIndex returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


