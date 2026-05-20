# SwapDetailsResponse

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Swap** | Pointer to [**MsgSwap**](MsgSwap.md) |  | [optional] 
**Status** | Pointer to **string** | Current status of the swap | [optional] 
**QueueType** | Pointer to **string** | Type of queue the swap is in | [optional] 

## Methods

### NewSwapDetailsResponse

`func NewSwapDetailsResponse() *SwapDetailsResponse`

NewSwapDetailsResponse instantiates a new SwapDetailsResponse object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewSwapDetailsResponseWithDefaults

`func NewSwapDetailsResponseWithDefaults() *SwapDetailsResponse`

NewSwapDetailsResponseWithDefaults instantiates a new SwapDetailsResponse object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetSwap

`func (o *SwapDetailsResponse) GetSwap() MsgSwap`

GetSwap returns the Swap field if non-nil, zero value otherwise.

### GetSwapOk

`func (o *SwapDetailsResponse) GetSwapOk() (*MsgSwap, bool)`

GetSwapOk returns a tuple with the Swap field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSwap

`func (o *SwapDetailsResponse) SetSwap(v MsgSwap)`

SetSwap sets Swap field to given value.

### HasSwap

`func (o *SwapDetailsResponse) HasSwap() bool`

HasSwap returns a boolean if a field has been set.

### GetStatus

`func (o *SwapDetailsResponse) GetStatus() string`

GetStatus returns the Status field if non-nil, zero value otherwise.

### GetStatusOk

`func (o *SwapDetailsResponse) GetStatusOk() (*string, bool)`

GetStatusOk returns a tuple with the Status field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetStatus

`func (o *SwapDetailsResponse) SetStatus(v string)`

SetStatus sets Status field to given value.

### HasStatus

`func (o *SwapDetailsResponse) HasStatus() bool`

HasStatus returns a boolean if a field has been set.

### GetQueueType

`func (o *SwapDetailsResponse) GetQueueType() string`

GetQueueType returns the QueueType field if non-nil, zero value otherwise.

### GetQueueTypeOk

`func (o *SwapDetailsResponse) GetQueueTypeOk() (*string, bool)`

GetQueueTypeOk returns a tuple with the QueueType field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetQueueType

`func (o *SwapDetailsResponse) SetQueueType(v string)`

SetQueueType sets QueueType field to given value.

### HasQueueType

`func (o *SwapDetailsResponse) HasQueueType() bool`

HasQueueType returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


