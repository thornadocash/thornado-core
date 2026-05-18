# SwapState

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Interval** | Pointer to **int64** | the interval for streaming swaps | [optional] 
**Quantity** | Pointer to **int64** | the number of swaps to execute | [optional] 
**Ttl** | Pointer to **int64** | time to live | [optional] 
**Count** | Pointer to **int64** | number of swaps executed | [optional] 
**LastHeight** | Pointer to **int64** | last height when a swap was executed | [optional] 
**Deposit** | Pointer to **string** | total deposit amount | [optional] 
**Withdrawn** | Pointer to **string** | amount withdrawn | [optional] 
**In** | Pointer to **string** | total amount swapped in | [optional] 
**Out** | Pointer to **string** | total amount swapped out | [optional] 
**FailedSwaps** | Pointer to **[]int64** | list of failed swap indices | [optional] 
**FailedSwapReasons** | Pointer to **[]string** | reasons for failed swaps | [optional] 

## Methods

### NewSwapState

`func NewSwapState() *SwapState`

NewSwapState instantiates a new SwapState object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewSwapStateWithDefaults

`func NewSwapStateWithDefaults() *SwapState`

NewSwapStateWithDefaults instantiates a new SwapState object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetInterval

`func (o *SwapState) GetInterval() int64`

GetInterval returns the Interval field if non-nil, zero value otherwise.

### GetIntervalOk

`func (o *SwapState) GetIntervalOk() (*int64, bool)`

GetIntervalOk returns a tuple with the Interval field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetInterval

`func (o *SwapState) SetInterval(v int64)`

SetInterval sets Interval field to given value.

### HasInterval

`func (o *SwapState) HasInterval() bool`

HasInterval returns a boolean if a field has been set.

### GetQuantity

`func (o *SwapState) GetQuantity() int64`

GetQuantity returns the Quantity field if non-nil, zero value otherwise.

### GetQuantityOk

`func (o *SwapState) GetQuantityOk() (*int64, bool)`

GetQuantityOk returns a tuple with the Quantity field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetQuantity

`func (o *SwapState) SetQuantity(v int64)`

SetQuantity sets Quantity field to given value.

### HasQuantity

`func (o *SwapState) HasQuantity() bool`

HasQuantity returns a boolean if a field has been set.

### GetTtl

`func (o *SwapState) GetTtl() int64`

GetTtl returns the Ttl field if non-nil, zero value otherwise.

### GetTtlOk

`func (o *SwapState) GetTtlOk() (*int64, bool)`

GetTtlOk returns a tuple with the Ttl field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetTtl

`func (o *SwapState) SetTtl(v int64)`

SetTtl sets Ttl field to given value.

### HasTtl

`func (o *SwapState) HasTtl() bool`

HasTtl returns a boolean if a field has been set.

### GetCount

`func (o *SwapState) GetCount() int64`

GetCount returns the Count field if non-nil, zero value otherwise.

### GetCountOk

`func (o *SwapState) GetCountOk() (*int64, bool)`

GetCountOk returns a tuple with the Count field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCount

`func (o *SwapState) SetCount(v int64)`

SetCount sets Count field to given value.

### HasCount

`func (o *SwapState) HasCount() bool`

HasCount returns a boolean if a field has been set.

### GetLastHeight

`func (o *SwapState) GetLastHeight() int64`

GetLastHeight returns the LastHeight field if non-nil, zero value otherwise.

### GetLastHeightOk

`func (o *SwapState) GetLastHeightOk() (*int64, bool)`

GetLastHeightOk returns a tuple with the LastHeight field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetLastHeight

`func (o *SwapState) SetLastHeight(v int64)`

SetLastHeight sets LastHeight field to given value.

### HasLastHeight

`func (o *SwapState) HasLastHeight() bool`

HasLastHeight returns a boolean if a field has been set.

### GetDeposit

`func (o *SwapState) GetDeposit() string`

GetDeposit returns the Deposit field if non-nil, zero value otherwise.

### GetDepositOk

`func (o *SwapState) GetDepositOk() (*string, bool)`

GetDepositOk returns a tuple with the Deposit field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetDeposit

`func (o *SwapState) SetDeposit(v string)`

SetDeposit sets Deposit field to given value.

### HasDeposit

`func (o *SwapState) HasDeposit() bool`

HasDeposit returns a boolean if a field has been set.

### GetWithdrawn

`func (o *SwapState) GetWithdrawn() string`

GetWithdrawn returns the Withdrawn field if non-nil, zero value otherwise.

### GetWithdrawnOk

`func (o *SwapState) GetWithdrawnOk() (*string, bool)`

GetWithdrawnOk returns a tuple with the Withdrawn field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetWithdrawn

`func (o *SwapState) SetWithdrawn(v string)`

SetWithdrawn sets Withdrawn field to given value.

### HasWithdrawn

`func (o *SwapState) HasWithdrawn() bool`

HasWithdrawn returns a boolean if a field has been set.

### GetIn

`func (o *SwapState) GetIn() string`

GetIn returns the In field if non-nil, zero value otherwise.

### GetInOk

`func (o *SwapState) GetInOk() (*string, bool)`

GetInOk returns a tuple with the In field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetIn

`func (o *SwapState) SetIn(v string)`

SetIn sets In field to given value.

### HasIn

`func (o *SwapState) HasIn() bool`

HasIn returns a boolean if a field has been set.

### GetOut

`func (o *SwapState) GetOut() string`

GetOut returns the Out field if non-nil, zero value otherwise.

### GetOutOk

`func (o *SwapState) GetOutOk() (*string, bool)`

GetOutOk returns a tuple with the Out field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetOut

`func (o *SwapState) SetOut(v string)`

SetOut sets Out field to given value.

### HasOut

`func (o *SwapState) HasOut() bool`

HasOut returns a boolean if a field has been set.

### GetFailedSwaps

`func (o *SwapState) GetFailedSwaps() []int64`

GetFailedSwaps returns the FailedSwaps field if non-nil, zero value otherwise.

### GetFailedSwapsOk

`func (o *SwapState) GetFailedSwapsOk() (*[]int64, bool)`

GetFailedSwapsOk returns a tuple with the FailedSwaps field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetFailedSwaps

`func (o *SwapState) SetFailedSwaps(v []int64)`

SetFailedSwaps sets FailedSwaps field to given value.

### HasFailedSwaps

`func (o *SwapState) HasFailedSwaps() bool`

HasFailedSwaps returns a boolean if a field has been set.

### GetFailedSwapReasons

`func (o *SwapState) GetFailedSwapReasons() []string`

GetFailedSwapReasons returns the FailedSwapReasons field if non-nil, zero value otherwise.

### GetFailedSwapReasonsOk

`func (o *SwapState) GetFailedSwapReasonsOk() (*[]string, bool)`

GetFailedSwapReasonsOk returns a tuple with the FailedSwapReasons field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetFailedSwapReasons

`func (o *SwapState) SetFailedSwapReasons(v []string)`

SetFailedSwapReasons sets FailedSwapReasons field to given value.

### HasFailedSwapReasons

`func (o *SwapState) HasFailedSwapReasons() bool`

HasFailedSwapReasons returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


