# LimitSwapWithDetails

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Swap** | Pointer to [**MsgSwap**](MsgSwap.md) |  | [optional] 
**Ratio** | Pointer to **string** | The ratio threshold for this limit swap | [optional] 
**BlocksSinceCreated** | Pointer to **int64** | Number of blocks since the swap was created | [optional] 
**TimeToExpiryBlocks** | Pointer to **int64** | Number of blocks until the swap expires | [optional] 
**CreatedTimestamp** | Pointer to **int64** | Unix timestamp when the swap was created | [optional] 

## Methods

### NewLimitSwapWithDetails

`func NewLimitSwapWithDetails() *LimitSwapWithDetails`

NewLimitSwapWithDetails instantiates a new LimitSwapWithDetails object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewLimitSwapWithDetailsWithDefaults

`func NewLimitSwapWithDetailsWithDefaults() *LimitSwapWithDetails`

NewLimitSwapWithDetailsWithDefaults instantiates a new LimitSwapWithDetails object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetSwap

`func (o *LimitSwapWithDetails) GetSwap() MsgSwap`

GetSwap returns the Swap field if non-nil, zero value otherwise.

### GetSwapOk

`func (o *LimitSwapWithDetails) GetSwapOk() (*MsgSwap, bool)`

GetSwapOk returns a tuple with the Swap field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSwap

`func (o *LimitSwapWithDetails) SetSwap(v MsgSwap)`

SetSwap sets Swap field to given value.

### HasSwap

`func (o *LimitSwapWithDetails) HasSwap() bool`

HasSwap returns a boolean if a field has been set.

### GetRatio

`func (o *LimitSwapWithDetails) GetRatio() string`

GetRatio returns the Ratio field if non-nil, zero value otherwise.

### GetRatioOk

`func (o *LimitSwapWithDetails) GetRatioOk() (*string, bool)`

GetRatioOk returns a tuple with the Ratio field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRatio

`func (o *LimitSwapWithDetails) SetRatio(v string)`

SetRatio sets Ratio field to given value.

### HasRatio

`func (o *LimitSwapWithDetails) HasRatio() bool`

HasRatio returns a boolean if a field has been set.

### GetBlocksSinceCreated

`func (o *LimitSwapWithDetails) GetBlocksSinceCreated() int64`

GetBlocksSinceCreated returns the BlocksSinceCreated field if non-nil, zero value otherwise.

### GetBlocksSinceCreatedOk

`func (o *LimitSwapWithDetails) GetBlocksSinceCreatedOk() (*int64, bool)`

GetBlocksSinceCreatedOk returns a tuple with the BlocksSinceCreated field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetBlocksSinceCreated

`func (o *LimitSwapWithDetails) SetBlocksSinceCreated(v int64)`

SetBlocksSinceCreated sets BlocksSinceCreated field to given value.

### HasBlocksSinceCreated

`func (o *LimitSwapWithDetails) HasBlocksSinceCreated() bool`

HasBlocksSinceCreated returns a boolean if a field has been set.

### GetTimeToExpiryBlocks

`func (o *LimitSwapWithDetails) GetTimeToExpiryBlocks() int64`

GetTimeToExpiryBlocks returns the TimeToExpiryBlocks field if non-nil, zero value otherwise.

### GetTimeToExpiryBlocksOk

`func (o *LimitSwapWithDetails) GetTimeToExpiryBlocksOk() (*int64, bool)`

GetTimeToExpiryBlocksOk returns a tuple with the TimeToExpiryBlocks field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetTimeToExpiryBlocks

`func (o *LimitSwapWithDetails) SetTimeToExpiryBlocks(v int64)`

SetTimeToExpiryBlocks sets TimeToExpiryBlocks field to given value.

### HasTimeToExpiryBlocks

`func (o *LimitSwapWithDetails) HasTimeToExpiryBlocks() bool`

HasTimeToExpiryBlocks returns a boolean if a field has been set.

### GetCreatedTimestamp

`func (o *LimitSwapWithDetails) GetCreatedTimestamp() int64`

GetCreatedTimestamp returns the CreatedTimestamp field if non-nil, zero value otherwise.

### GetCreatedTimestampOk

`func (o *LimitSwapWithDetails) GetCreatedTimestampOk() (*int64, bool)`

GetCreatedTimestampOk returns a tuple with the CreatedTimestamp field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCreatedTimestamp

`func (o *LimitSwapWithDetails) SetCreatedTimestamp(v int64)`

SetCreatedTimestamp sets CreatedTimestamp field to given value.

### HasCreatedTimestamp

`func (o *LimitSwapWithDetails) HasCreatedTimestamp() bool`

HasCreatedTimestamp returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


