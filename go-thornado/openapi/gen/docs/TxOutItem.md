# TxOutItem

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Height** | Pointer to **int64** |  | [optional] 
**InHash** | Pointer to **string** |  | [optional] 
**OutHash** | Pointer to **string** |  | [optional] 
**Chain** | **string** |  | 
**ToAddress** | **string** |  | 
**VaultPubKey** | Pointer to **string** |  | [optional] 
**VaultPubKeyEddsa** | Pointer to **string** |  | [optional] 
**Coin** | [**Coin**](Coin.md) |  | 
**MaxGas** | [**[]Coin**](Coin.md) |  | 
**GasRate** | Pointer to **int64** |  | [optional] 
**Memo** | Pointer to **string** |  | [optional] 
**OriginalMemo** | Pointer to **string** |  | [optional] 
**Aggregator** | Pointer to **string** | whitelisted DEX Aggregator contract address | [optional] 
**AggregatorTargetAsset** | Pointer to **string** | target asset for the aggregator contract to attempt a swap to | [optional] 
**AggregatorTargetLimit** | Pointer to **string** | the minimum number of tokens the swapper wants to receive of the output asset | [optional] 
**CloutSpent** | Pointer to **string** | clout spent in RUNE for the outbound | [optional] 

## Methods

### NewTxOutItem

`func NewTxOutItem(chain string, toAddress string, coin Coin, maxGas []Coin, ) *TxOutItem`

NewTxOutItem instantiates a new TxOutItem object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewTxOutItemWithDefaults

`func NewTxOutItemWithDefaults() *TxOutItem`

NewTxOutItemWithDefaults instantiates a new TxOutItem object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetHeight

`func (o *TxOutItem) GetHeight() int64`

GetHeight returns the Height field if non-nil, zero value otherwise.

### GetHeightOk

`func (o *TxOutItem) GetHeightOk() (*int64, bool)`

GetHeightOk returns a tuple with the Height field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetHeight

`func (o *TxOutItem) SetHeight(v int64)`

SetHeight sets Height field to given value.

### HasHeight

`func (o *TxOutItem) HasHeight() bool`

HasHeight returns a boolean if a field has been set.

### GetInHash

`func (o *TxOutItem) GetInHash() string`

GetInHash returns the InHash field if non-nil, zero value otherwise.

### GetInHashOk

`func (o *TxOutItem) GetInHashOk() (*string, bool)`

GetInHashOk returns a tuple with the InHash field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetInHash

`func (o *TxOutItem) SetInHash(v string)`

SetInHash sets InHash field to given value.

### HasInHash

`func (o *TxOutItem) HasInHash() bool`

HasInHash returns a boolean if a field has been set.

### GetOutHash

`func (o *TxOutItem) GetOutHash() string`

GetOutHash returns the OutHash field if non-nil, zero value otherwise.

### GetOutHashOk

`func (o *TxOutItem) GetOutHashOk() (*string, bool)`

GetOutHashOk returns a tuple with the OutHash field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetOutHash

`func (o *TxOutItem) SetOutHash(v string)`

SetOutHash sets OutHash field to given value.

### HasOutHash

`func (o *TxOutItem) HasOutHash() bool`

HasOutHash returns a boolean if a field has been set.

### GetChain

`func (o *TxOutItem) GetChain() string`

GetChain returns the Chain field if non-nil, zero value otherwise.

### GetChainOk

`func (o *TxOutItem) GetChainOk() (*string, bool)`

GetChainOk returns a tuple with the Chain field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetChain

`func (o *TxOutItem) SetChain(v string)`

SetChain sets Chain field to given value.


### GetToAddress

`func (o *TxOutItem) GetToAddress() string`

GetToAddress returns the ToAddress field if non-nil, zero value otherwise.

### GetToAddressOk

`func (o *TxOutItem) GetToAddressOk() (*string, bool)`

GetToAddressOk returns a tuple with the ToAddress field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetToAddress

`func (o *TxOutItem) SetToAddress(v string)`

SetToAddress sets ToAddress field to given value.


### GetVaultPubKey

`func (o *TxOutItem) GetVaultPubKey() string`

GetVaultPubKey returns the VaultPubKey field if non-nil, zero value otherwise.

### GetVaultPubKeyOk

`func (o *TxOutItem) GetVaultPubKeyOk() (*string, bool)`

GetVaultPubKeyOk returns a tuple with the VaultPubKey field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetVaultPubKey

`func (o *TxOutItem) SetVaultPubKey(v string)`

SetVaultPubKey sets VaultPubKey field to given value.

### HasVaultPubKey

`func (o *TxOutItem) HasVaultPubKey() bool`

HasVaultPubKey returns a boolean if a field has been set.

### GetVaultPubKeyEddsa

`func (o *TxOutItem) GetVaultPubKeyEddsa() string`

GetVaultPubKeyEddsa returns the VaultPubKeyEddsa field if non-nil, zero value otherwise.

### GetVaultPubKeyEddsaOk

`func (o *TxOutItem) GetVaultPubKeyEddsaOk() (*string, bool)`

GetVaultPubKeyEddsaOk returns a tuple with the VaultPubKeyEddsa field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetVaultPubKeyEddsa

`func (o *TxOutItem) SetVaultPubKeyEddsa(v string)`

SetVaultPubKeyEddsa sets VaultPubKeyEddsa field to given value.

### HasVaultPubKeyEddsa

`func (o *TxOutItem) HasVaultPubKeyEddsa() bool`

HasVaultPubKeyEddsa returns a boolean if a field has been set.

### GetCoin

`func (o *TxOutItem) GetCoin() Coin`

GetCoin returns the Coin field if non-nil, zero value otherwise.

### GetCoinOk

`func (o *TxOutItem) GetCoinOk() (*Coin, bool)`

GetCoinOk returns a tuple with the Coin field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCoin

`func (o *TxOutItem) SetCoin(v Coin)`

SetCoin sets Coin field to given value.


### GetMaxGas

`func (o *TxOutItem) GetMaxGas() []Coin`

GetMaxGas returns the MaxGas field if non-nil, zero value otherwise.

### GetMaxGasOk

`func (o *TxOutItem) GetMaxGasOk() (*[]Coin, bool)`

GetMaxGasOk returns a tuple with the MaxGas field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetMaxGas

`func (o *TxOutItem) SetMaxGas(v []Coin)`

SetMaxGas sets MaxGas field to given value.


### GetGasRate

`func (o *TxOutItem) GetGasRate() int64`

GetGasRate returns the GasRate field if non-nil, zero value otherwise.

### GetGasRateOk

`func (o *TxOutItem) GetGasRateOk() (*int64, bool)`

GetGasRateOk returns a tuple with the GasRate field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetGasRate

`func (o *TxOutItem) SetGasRate(v int64)`

SetGasRate sets GasRate field to given value.

### HasGasRate

`func (o *TxOutItem) HasGasRate() bool`

HasGasRate returns a boolean if a field has been set.

### GetMemo

`func (o *TxOutItem) GetMemo() string`

GetMemo returns the Memo field if non-nil, zero value otherwise.

### GetMemoOk

`func (o *TxOutItem) GetMemoOk() (*string, bool)`

GetMemoOk returns a tuple with the Memo field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetMemo

`func (o *TxOutItem) SetMemo(v string)`

SetMemo sets Memo field to given value.

### HasMemo

`func (o *TxOutItem) HasMemo() bool`

HasMemo returns a boolean if a field has been set.

### GetOriginalMemo

`func (o *TxOutItem) GetOriginalMemo() string`

GetOriginalMemo returns the OriginalMemo field if non-nil, zero value otherwise.

### GetOriginalMemoOk

`func (o *TxOutItem) GetOriginalMemoOk() (*string, bool)`

GetOriginalMemoOk returns a tuple with the OriginalMemo field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetOriginalMemo

`func (o *TxOutItem) SetOriginalMemo(v string)`

SetOriginalMemo sets OriginalMemo field to given value.

### HasOriginalMemo

`func (o *TxOutItem) HasOriginalMemo() bool`

HasOriginalMemo returns a boolean if a field has been set.

### GetAggregator

`func (o *TxOutItem) GetAggregator() string`

GetAggregator returns the Aggregator field if non-nil, zero value otherwise.

### GetAggregatorOk

`func (o *TxOutItem) GetAggregatorOk() (*string, bool)`

GetAggregatorOk returns a tuple with the Aggregator field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetAggregator

`func (o *TxOutItem) SetAggregator(v string)`

SetAggregator sets Aggregator field to given value.

### HasAggregator

`func (o *TxOutItem) HasAggregator() bool`

HasAggregator returns a boolean if a field has been set.

### GetAggregatorTargetAsset

`func (o *TxOutItem) GetAggregatorTargetAsset() string`

GetAggregatorTargetAsset returns the AggregatorTargetAsset field if non-nil, zero value otherwise.

### GetAggregatorTargetAssetOk

`func (o *TxOutItem) GetAggregatorTargetAssetOk() (*string, bool)`

GetAggregatorTargetAssetOk returns a tuple with the AggregatorTargetAsset field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetAggregatorTargetAsset

`func (o *TxOutItem) SetAggregatorTargetAsset(v string)`

SetAggregatorTargetAsset sets AggregatorTargetAsset field to given value.

### HasAggregatorTargetAsset

`func (o *TxOutItem) HasAggregatorTargetAsset() bool`

HasAggregatorTargetAsset returns a boolean if a field has been set.

### GetAggregatorTargetLimit

`func (o *TxOutItem) GetAggregatorTargetLimit() string`

GetAggregatorTargetLimit returns the AggregatorTargetLimit field if non-nil, zero value otherwise.

### GetAggregatorTargetLimitOk

`func (o *TxOutItem) GetAggregatorTargetLimitOk() (*string, bool)`

GetAggregatorTargetLimitOk returns a tuple with the AggregatorTargetLimit field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetAggregatorTargetLimit

`func (o *TxOutItem) SetAggregatorTargetLimit(v string)`

SetAggregatorTargetLimit sets AggregatorTargetLimit field to given value.

### HasAggregatorTargetLimit

`func (o *TxOutItem) HasAggregatorTargetLimit() bool`

HasAggregatorTargetLimit returns a boolean if a field has been set.

### GetCloutSpent

`func (o *TxOutItem) GetCloutSpent() string`

GetCloutSpent returns the CloutSpent field if non-nil, zero value otherwise.

### GetCloutSpentOk

`func (o *TxOutItem) GetCloutSpentOk() (*string, bool)`

GetCloutSpentOk returns a tuple with the CloutSpent field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCloutSpent

`func (o *TxOutItem) SetCloutSpent(v string)`

SetCloutSpent sets CloutSpent field to given value.

### HasCloutSpent

`func (o *TxOutItem) HasCloutSpent() bool`

HasCloutSpent returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


