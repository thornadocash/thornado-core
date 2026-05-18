# AssetPairSummary

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**SourceAsset** | Pointer to **string** | Source asset identifier | [optional] 
**TargetAsset** | Pointer to **string** | Target asset identifier | [optional] 
**Count** | Pointer to **int64** | Number of limit swaps for this asset pair | [optional] 
**TotalValueUsd** | Pointer to **string** | Total USD value of limit swaps for this asset pair | [optional] 

## Methods

### NewAssetPairSummary

`func NewAssetPairSummary() *AssetPairSummary`

NewAssetPairSummary instantiates a new AssetPairSummary object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewAssetPairSummaryWithDefaults

`func NewAssetPairSummaryWithDefaults() *AssetPairSummary`

NewAssetPairSummaryWithDefaults instantiates a new AssetPairSummary object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetSourceAsset

`func (o *AssetPairSummary) GetSourceAsset() string`

GetSourceAsset returns the SourceAsset field if non-nil, zero value otherwise.

### GetSourceAssetOk

`func (o *AssetPairSummary) GetSourceAssetOk() (*string, bool)`

GetSourceAssetOk returns a tuple with the SourceAsset field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSourceAsset

`func (o *AssetPairSummary) SetSourceAsset(v string)`

SetSourceAsset sets SourceAsset field to given value.

### HasSourceAsset

`func (o *AssetPairSummary) HasSourceAsset() bool`

HasSourceAsset returns a boolean if a field has been set.

### GetTargetAsset

`func (o *AssetPairSummary) GetTargetAsset() string`

GetTargetAsset returns the TargetAsset field if non-nil, zero value otherwise.

### GetTargetAssetOk

`func (o *AssetPairSummary) GetTargetAssetOk() (*string, bool)`

GetTargetAssetOk returns a tuple with the TargetAsset field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetTargetAsset

`func (o *AssetPairSummary) SetTargetAsset(v string)`

SetTargetAsset sets TargetAsset field to given value.

### HasTargetAsset

`func (o *AssetPairSummary) HasTargetAsset() bool`

HasTargetAsset returns a boolean if a field has been set.

### GetCount

`func (o *AssetPairSummary) GetCount() int64`

GetCount returns the Count field if non-nil, zero value otherwise.

### GetCountOk

`func (o *AssetPairSummary) GetCountOk() (*int64, bool)`

GetCountOk returns a tuple with the Count field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCount

`func (o *AssetPairSummary) SetCount(v int64)`

SetCount sets Count field to given value.

### HasCount

`func (o *AssetPairSummary) HasCount() bool`

HasCount returns a boolean if a field has been set.

### GetTotalValueUsd

`func (o *AssetPairSummary) GetTotalValueUsd() string`

GetTotalValueUsd returns the TotalValueUsd field if non-nil, zero value otherwise.

### GetTotalValueUsdOk

`func (o *AssetPairSummary) GetTotalValueUsdOk() (*string, bool)`

GetTotalValueUsdOk returns a tuple with the TotalValueUsd field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetTotalValueUsd

`func (o *AssetPairSummary) SetTotalValueUsd(v string)`

SetTotalValueUsd sets TotalValueUsd field to given value.

### HasTotalValueUsd

`func (o *AssetPairSummary) HasTotalValueUsd() bool`

HasTotalValueUsd returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


