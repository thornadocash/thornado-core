# LimitSwapsSummaryResponse

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**TotalLimitSwaps** | Pointer to **int64** | Total number of limit swaps | [optional] 
**TotalValueUsd** | Pointer to **string** | Total USD value of all limit swaps | [optional] 
**AssetPairs** | Pointer to [**[]AssetPairSummary**](AssetPairSummary.md) | Summary statistics by asset pair | [optional] 
**OldestSwapBlocks** | Pointer to **int64** | Age in blocks of the oldest limit swap | [optional] 
**AverageAgeBlocks** | Pointer to **int64** | Average age in blocks of all limit swaps | [optional] 

## Methods

### NewLimitSwapsSummaryResponse

`func NewLimitSwapsSummaryResponse() *LimitSwapsSummaryResponse`

NewLimitSwapsSummaryResponse instantiates a new LimitSwapsSummaryResponse object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewLimitSwapsSummaryResponseWithDefaults

`func NewLimitSwapsSummaryResponseWithDefaults() *LimitSwapsSummaryResponse`

NewLimitSwapsSummaryResponseWithDefaults instantiates a new LimitSwapsSummaryResponse object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetTotalLimitSwaps

`func (o *LimitSwapsSummaryResponse) GetTotalLimitSwaps() int64`

GetTotalLimitSwaps returns the TotalLimitSwaps field if non-nil, zero value otherwise.

### GetTotalLimitSwapsOk

`func (o *LimitSwapsSummaryResponse) GetTotalLimitSwapsOk() (*int64, bool)`

GetTotalLimitSwapsOk returns a tuple with the TotalLimitSwaps field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetTotalLimitSwaps

`func (o *LimitSwapsSummaryResponse) SetTotalLimitSwaps(v int64)`

SetTotalLimitSwaps sets TotalLimitSwaps field to given value.

### HasTotalLimitSwaps

`func (o *LimitSwapsSummaryResponse) HasTotalLimitSwaps() bool`

HasTotalLimitSwaps returns a boolean if a field has been set.

### GetTotalValueUsd

`func (o *LimitSwapsSummaryResponse) GetTotalValueUsd() string`

GetTotalValueUsd returns the TotalValueUsd field if non-nil, zero value otherwise.

### GetTotalValueUsdOk

`func (o *LimitSwapsSummaryResponse) GetTotalValueUsdOk() (*string, bool)`

GetTotalValueUsdOk returns a tuple with the TotalValueUsd field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetTotalValueUsd

`func (o *LimitSwapsSummaryResponse) SetTotalValueUsd(v string)`

SetTotalValueUsd sets TotalValueUsd field to given value.

### HasTotalValueUsd

`func (o *LimitSwapsSummaryResponse) HasTotalValueUsd() bool`

HasTotalValueUsd returns a boolean if a field has been set.

### GetAssetPairs

`func (o *LimitSwapsSummaryResponse) GetAssetPairs() []AssetPairSummary`

GetAssetPairs returns the AssetPairs field if non-nil, zero value otherwise.

### GetAssetPairsOk

`func (o *LimitSwapsSummaryResponse) GetAssetPairsOk() (*[]AssetPairSummary, bool)`

GetAssetPairsOk returns a tuple with the AssetPairs field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetAssetPairs

`func (o *LimitSwapsSummaryResponse) SetAssetPairs(v []AssetPairSummary)`

SetAssetPairs sets AssetPairs field to given value.

### HasAssetPairs

`func (o *LimitSwapsSummaryResponse) HasAssetPairs() bool`

HasAssetPairs returns a boolean if a field has been set.

### GetOldestSwapBlocks

`func (o *LimitSwapsSummaryResponse) GetOldestSwapBlocks() int64`

GetOldestSwapBlocks returns the OldestSwapBlocks field if non-nil, zero value otherwise.

### GetOldestSwapBlocksOk

`func (o *LimitSwapsSummaryResponse) GetOldestSwapBlocksOk() (*int64, bool)`

GetOldestSwapBlocksOk returns a tuple with the OldestSwapBlocks field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetOldestSwapBlocks

`func (o *LimitSwapsSummaryResponse) SetOldestSwapBlocks(v int64)`

SetOldestSwapBlocks sets OldestSwapBlocks field to given value.

### HasOldestSwapBlocks

`func (o *LimitSwapsSummaryResponse) HasOldestSwapBlocks() bool`

HasOldestSwapBlocks returns a boolean if a field has been set.

### GetAverageAgeBlocks

`func (o *LimitSwapsSummaryResponse) GetAverageAgeBlocks() int64`

GetAverageAgeBlocks returns the AverageAgeBlocks field if non-nil, zero value otherwise.

### GetAverageAgeBlocksOk

`func (o *LimitSwapsSummaryResponse) GetAverageAgeBlocksOk() (*int64, bool)`

GetAverageAgeBlocksOk returns a tuple with the AverageAgeBlocks field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetAverageAgeBlocks

`func (o *LimitSwapsSummaryResponse) SetAverageAgeBlocks(v int64)`

SetAverageAgeBlocks sets AverageAgeBlocks field to given value.

### HasAverageAgeBlocks

`func (o *LimitSwapsSummaryResponse) HasAverageAgeBlocks() bool`

HasAverageAgeBlocks returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


