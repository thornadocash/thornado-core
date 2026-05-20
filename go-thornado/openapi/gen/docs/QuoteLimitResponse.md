# QuoteLimitResponse

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**InboundAddress** | **string** | the inbound address for the transaction on the source chain | 
**InboundConfirmationBlocks** | Pointer to **int64** | the approximate number of source chain blocks required before processing | [optional] 
**InboundConfirmationSeconds** | Pointer to **int64** | the approximate seconds for block confirmations required before processing | [optional] 
**OutboundDelayBlocks** | Pointer to **int64** | the number of thorchain blocks the outbound will be delayed | [optional] 
**OutboundDelaySeconds** | Pointer to **int64** | the approximate seconds for the outbound delay before it will be sent | [optional] 
**Fees** | [**QuoteFees**](QuoteFees.md) |  | 
**Router** | Pointer to **string** | the EVM chain router contract address | [optional] 
**Expiry** | Pointer to **int64** | expiration timestamp in unix seconds | [optional] 
**Warning** | Pointer to **string** | static warning message | [optional] 
**Notes** | Pointer to **string** | notes about the limit order | [optional] 
**DustThreshold** | Pointer to **string** | the dust threshold for the source chain | [optional] 
**RecommendedMinAmountIn** | Pointer to **string** | the recommended minimum amount in for the limit order | [optional] 
**RecommendedGasRate** | Pointer to **string** | the recommended gas rate to use for the inbound to ensure timely confirmation | [optional] 
**GasRateUnits** | Pointer to **string** | the units of the recommended gas rate | [optional] 
**Memo** | Pointer to **string** | generated memo for the limit order | [optional] 
**ExpectedAmountOut** | **string** | the amount of the target asset the user can expect to receive after fees | 
**OrderExpiryBlock** | **int64** | the block height when the limit order will expire | 
**OrderExpiryTimestamp** | **int64** | the timestamp when the limit order will expire | 

## Methods

### NewQuoteLimitResponse

`func NewQuoteLimitResponse(inboundAddress string, fees QuoteFees, expectedAmountOut string, orderExpiryBlock int64, orderExpiryTimestamp int64, ) *QuoteLimitResponse`

NewQuoteLimitResponse instantiates a new QuoteLimitResponse object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewQuoteLimitResponseWithDefaults

`func NewQuoteLimitResponseWithDefaults() *QuoteLimitResponse`

NewQuoteLimitResponseWithDefaults instantiates a new QuoteLimitResponse object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetInboundAddress

`func (o *QuoteLimitResponse) GetInboundAddress() string`

GetInboundAddress returns the InboundAddress field if non-nil, zero value otherwise.

### GetInboundAddressOk

`func (o *QuoteLimitResponse) GetInboundAddressOk() (*string, bool)`

GetInboundAddressOk returns a tuple with the InboundAddress field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetInboundAddress

`func (o *QuoteLimitResponse) SetInboundAddress(v string)`

SetInboundAddress sets InboundAddress field to given value.


### GetInboundConfirmationBlocks

`func (o *QuoteLimitResponse) GetInboundConfirmationBlocks() int64`

GetInboundConfirmationBlocks returns the InboundConfirmationBlocks field if non-nil, zero value otherwise.

### GetInboundConfirmationBlocksOk

`func (o *QuoteLimitResponse) GetInboundConfirmationBlocksOk() (*int64, bool)`

GetInboundConfirmationBlocksOk returns a tuple with the InboundConfirmationBlocks field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetInboundConfirmationBlocks

`func (o *QuoteLimitResponse) SetInboundConfirmationBlocks(v int64)`

SetInboundConfirmationBlocks sets InboundConfirmationBlocks field to given value.

### HasInboundConfirmationBlocks

`func (o *QuoteLimitResponse) HasInboundConfirmationBlocks() bool`

HasInboundConfirmationBlocks returns a boolean if a field has been set.

### GetInboundConfirmationSeconds

`func (o *QuoteLimitResponse) GetInboundConfirmationSeconds() int64`

GetInboundConfirmationSeconds returns the InboundConfirmationSeconds field if non-nil, zero value otherwise.

### GetInboundConfirmationSecondsOk

`func (o *QuoteLimitResponse) GetInboundConfirmationSecondsOk() (*int64, bool)`

GetInboundConfirmationSecondsOk returns a tuple with the InboundConfirmationSeconds field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetInboundConfirmationSeconds

`func (o *QuoteLimitResponse) SetInboundConfirmationSeconds(v int64)`

SetInboundConfirmationSeconds sets InboundConfirmationSeconds field to given value.

### HasInboundConfirmationSeconds

`func (o *QuoteLimitResponse) HasInboundConfirmationSeconds() bool`

HasInboundConfirmationSeconds returns a boolean if a field has been set.

### GetOutboundDelayBlocks

`func (o *QuoteLimitResponse) GetOutboundDelayBlocks() int64`

GetOutboundDelayBlocks returns the OutboundDelayBlocks field if non-nil, zero value otherwise.

### GetOutboundDelayBlocksOk

`func (o *QuoteLimitResponse) GetOutboundDelayBlocksOk() (*int64, bool)`

GetOutboundDelayBlocksOk returns a tuple with the OutboundDelayBlocks field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetOutboundDelayBlocks

`func (o *QuoteLimitResponse) SetOutboundDelayBlocks(v int64)`

SetOutboundDelayBlocks sets OutboundDelayBlocks field to given value.

### HasOutboundDelayBlocks

`func (o *QuoteLimitResponse) HasOutboundDelayBlocks() bool`

HasOutboundDelayBlocks returns a boolean if a field has been set.

### GetOutboundDelaySeconds

`func (o *QuoteLimitResponse) GetOutboundDelaySeconds() int64`

GetOutboundDelaySeconds returns the OutboundDelaySeconds field if non-nil, zero value otherwise.

### GetOutboundDelaySecondsOk

`func (o *QuoteLimitResponse) GetOutboundDelaySecondsOk() (*int64, bool)`

GetOutboundDelaySecondsOk returns a tuple with the OutboundDelaySeconds field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetOutboundDelaySeconds

`func (o *QuoteLimitResponse) SetOutboundDelaySeconds(v int64)`

SetOutboundDelaySeconds sets OutboundDelaySeconds field to given value.

### HasOutboundDelaySeconds

`func (o *QuoteLimitResponse) HasOutboundDelaySeconds() bool`

HasOutboundDelaySeconds returns a boolean if a field has been set.

### GetFees

`func (o *QuoteLimitResponse) GetFees() QuoteFees`

GetFees returns the Fees field if non-nil, zero value otherwise.

### GetFeesOk

`func (o *QuoteLimitResponse) GetFeesOk() (*QuoteFees, bool)`

GetFeesOk returns a tuple with the Fees field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetFees

`func (o *QuoteLimitResponse) SetFees(v QuoteFees)`

SetFees sets Fees field to given value.


### GetRouter

`func (o *QuoteLimitResponse) GetRouter() string`

GetRouter returns the Router field if non-nil, zero value otherwise.

### GetRouterOk

`func (o *QuoteLimitResponse) GetRouterOk() (*string, bool)`

GetRouterOk returns a tuple with the Router field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRouter

`func (o *QuoteLimitResponse) SetRouter(v string)`

SetRouter sets Router field to given value.

### HasRouter

`func (o *QuoteLimitResponse) HasRouter() bool`

HasRouter returns a boolean if a field has been set.

### GetExpiry

`func (o *QuoteLimitResponse) GetExpiry() int64`

GetExpiry returns the Expiry field if non-nil, zero value otherwise.

### GetExpiryOk

`func (o *QuoteLimitResponse) GetExpiryOk() (*int64, bool)`

GetExpiryOk returns a tuple with the Expiry field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetExpiry

`func (o *QuoteLimitResponse) SetExpiry(v int64)`

SetExpiry sets Expiry field to given value.

### HasExpiry

`func (o *QuoteLimitResponse) HasExpiry() bool`

HasExpiry returns a boolean if a field has been set.

### GetWarning

`func (o *QuoteLimitResponse) GetWarning() string`

GetWarning returns the Warning field if non-nil, zero value otherwise.

### GetWarningOk

`func (o *QuoteLimitResponse) GetWarningOk() (*string, bool)`

GetWarningOk returns a tuple with the Warning field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetWarning

`func (o *QuoteLimitResponse) SetWarning(v string)`

SetWarning sets Warning field to given value.

### HasWarning

`func (o *QuoteLimitResponse) HasWarning() bool`

HasWarning returns a boolean if a field has been set.

### GetNotes

`func (o *QuoteLimitResponse) GetNotes() string`

GetNotes returns the Notes field if non-nil, zero value otherwise.

### GetNotesOk

`func (o *QuoteLimitResponse) GetNotesOk() (*string, bool)`

GetNotesOk returns a tuple with the Notes field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetNotes

`func (o *QuoteLimitResponse) SetNotes(v string)`

SetNotes sets Notes field to given value.

### HasNotes

`func (o *QuoteLimitResponse) HasNotes() bool`

HasNotes returns a boolean if a field has been set.

### GetDustThreshold

`func (o *QuoteLimitResponse) GetDustThreshold() string`

GetDustThreshold returns the DustThreshold field if non-nil, zero value otherwise.

### GetDustThresholdOk

`func (o *QuoteLimitResponse) GetDustThresholdOk() (*string, bool)`

GetDustThresholdOk returns a tuple with the DustThreshold field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetDustThreshold

`func (o *QuoteLimitResponse) SetDustThreshold(v string)`

SetDustThreshold sets DustThreshold field to given value.

### HasDustThreshold

`func (o *QuoteLimitResponse) HasDustThreshold() bool`

HasDustThreshold returns a boolean if a field has been set.

### GetRecommendedMinAmountIn

`func (o *QuoteLimitResponse) GetRecommendedMinAmountIn() string`

GetRecommendedMinAmountIn returns the RecommendedMinAmountIn field if non-nil, zero value otherwise.

### GetRecommendedMinAmountInOk

`func (o *QuoteLimitResponse) GetRecommendedMinAmountInOk() (*string, bool)`

GetRecommendedMinAmountInOk returns a tuple with the RecommendedMinAmountIn field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRecommendedMinAmountIn

`func (o *QuoteLimitResponse) SetRecommendedMinAmountIn(v string)`

SetRecommendedMinAmountIn sets RecommendedMinAmountIn field to given value.

### HasRecommendedMinAmountIn

`func (o *QuoteLimitResponse) HasRecommendedMinAmountIn() bool`

HasRecommendedMinAmountIn returns a boolean if a field has been set.

### GetRecommendedGasRate

`func (o *QuoteLimitResponse) GetRecommendedGasRate() string`

GetRecommendedGasRate returns the RecommendedGasRate field if non-nil, zero value otherwise.

### GetRecommendedGasRateOk

`func (o *QuoteLimitResponse) GetRecommendedGasRateOk() (*string, bool)`

GetRecommendedGasRateOk returns a tuple with the RecommendedGasRate field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRecommendedGasRate

`func (o *QuoteLimitResponse) SetRecommendedGasRate(v string)`

SetRecommendedGasRate sets RecommendedGasRate field to given value.

### HasRecommendedGasRate

`func (o *QuoteLimitResponse) HasRecommendedGasRate() bool`

HasRecommendedGasRate returns a boolean if a field has been set.

### GetGasRateUnits

`func (o *QuoteLimitResponse) GetGasRateUnits() string`

GetGasRateUnits returns the GasRateUnits field if non-nil, zero value otherwise.

### GetGasRateUnitsOk

`func (o *QuoteLimitResponse) GetGasRateUnitsOk() (*string, bool)`

GetGasRateUnitsOk returns a tuple with the GasRateUnits field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetGasRateUnits

`func (o *QuoteLimitResponse) SetGasRateUnits(v string)`

SetGasRateUnits sets GasRateUnits field to given value.

### HasGasRateUnits

`func (o *QuoteLimitResponse) HasGasRateUnits() bool`

HasGasRateUnits returns a boolean if a field has been set.

### GetMemo

`func (o *QuoteLimitResponse) GetMemo() string`

GetMemo returns the Memo field if non-nil, zero value otherwise.

### GetMemoOk

`func (o *QuoteLimitResponse) GetMemoOk() (*string, bool)`

GetMemoOk returns a tuple with the Memo field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetMemo

`func (o *QuoteLimitResponse) SetMemo(v string)`

SetMemo sets Memo field to given value.

### HasMemo

`func (o *QuoteLimitResponse) HasMemo() bool`

HasMemo returns a boolean if a field has been set.

### GetExpectedAmountOut

`func (o *QuoteLimitResponse) GetExpectedAmountOut() string`

GetExpectedAmountOut returns the ExpectedAmountOut field if non-nil, zero value otherwise.

### GetExpectedAmountOutOk

`func (o *QuoteLimitResponse) GetExpectedAmountOutOk() (*string, bool)`

GetExpectedAmountOutOk returns a tuple with the ExpectedAmountOut field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetExpectedAmountOut

`func (o *QuoteLimitResponse) SetExpectedAmountOut(v string)`

SetExpectedAmountOut sets ExpectedAmountOut field to given value.


### GetOrderExpiryBlock

`func (o *QuoteLimitResponse) GetOrderExpiryBlock() int64`

GetOrderExpiryBlock returns the OrderExpiryBlock field if non-nil, zero value otherwise.

### GetOrderExpiryBlockOk

`func (o *QuoteLimitResponse) GetOrderExpiryBlockOk() (*int64, bool)`

GetOrderExpiryBlockOk returns a tuple with the OrderExpiryBlock field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetOrderExpiryBlock

`func (o *QuoteLimitResponse) SetOrderExpiryBlock(v int64)`

SetOrderExpiryBlock sets OrderExpiryBlock field to given value.


### GetOrderExpiryTimestamp

`func (o *QuoteLimitResponse) GetOrderExpiryTimestamp() int64`

GetOrderExpiryTimestamp returns the OrderExpiryTimestamp field if non-nil, zero value otherwise.

### GetOrderExpiryTimestampOk

`func (o *QuoteLimitResponse) GetOrderExpiryTimestampOk() (*int64, bool)`

GetOrderExpiryTimestampOk returns a tuple with the OrderExpiryTimestamp field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetOrderExpiryTimestamp

`func (o *QuoteLimitResponse) SetOrderExpiryTimestamp(v int64)`

SetOrderExpiryTimestamp sets OrderExpiryTimestamp field to given value.



[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


