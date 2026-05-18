# ReferenceMemoResponseAsset

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Chain** | **string** | the blockchain identifier | 
**Symbol** | **string** | the asset symbol on the chain | 
**Ticker** | **string** | the asset ticker | 
**Synth** | **bool** | whether this is a synthetic asset | 

## Methods

### NewReferenceMemoResponseAsset

`func NewReferenceMemoResponseAsset(chain string, symbol string, ticker string, synth bool, ) *ReferenceMemoResponseAsset`

NewReferenceMemoResponseAsset instantiates a new ReferenceMemoResponseAsset object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewReferenceMemoResponseAssetWithDefaults

`func NewReferenceMemoResponseAssetWithDefaults() *ReferenceMemoResponseAsset`

NewReferenceMemoResponseAssetWithDefaults instantiates a new ReferenceMemoResponseAsset object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetChain

`func (o *ReferenceMemoResponseAsset) GetChain() string`

GetChain returns the Chain field if non-nil, zero value otherwise.

### GetChainOk

`func (o *ReferenceMemoResponseAsset) GetChainOk() (*string, bool)`

GetChainOk returns a tuple with the Chain field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetChain

`func (o *ReferenceMemoResponseAsset) SetChain(v string)`

SetChain sets Chain field to given value.


### GetSymbol

`func (o *ReferenceMemoResponseAsset) GetSymbol() string`

GetSymbol returns the Symbol field if non-nil, zero value otherwise.

### GetSymbolOk

`func (o *ReferenceMemoResponseAsset) GetSymbolOk() (*string, bool)`

GetSymbolOk returns a tuple with the Symbol field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSymbol

`func (o *ReferenceMemoResponseAsset) SetSymbol(v string)`

SetSymbol sets Symbol field to given value.


### GetTicker

`func (o *ReferenceMemoResponseAsset) GetTicker() string`

GetTicker returns the Ticker field if non-nil, zero value otherwise.

### GetTickerOk

`func (o *ReferenceMemoResponseAsset) GetTickerOk() (*string, bool)`

GetTickerOk returns a tuple with the Ticker field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetTicker

`func (o *ReferenceMemoResponseAsset) SetTicker(v string)`

SetTicker sets Ticker field to given value.


### GetSynth

`func (o *ReferenceMemoResponseAsset) GetSynth() bool`

GetSynth returns the Synth field if non-nil, zero value otherwise.

### GetSynthOk

`func (o *ReferenceMemoResponseAsset) GetSynthOk() (*bool, bool)`

GetSynthOk returns a tuple with the Synth field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSynth

`func (o *ReferenceMemoResponseAsset) SetSynth(v bool)`

SetSynth sets Synth field to given value.



[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


