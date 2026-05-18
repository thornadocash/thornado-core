# SupplyResponse

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Circulating** | **int64** | circulating RUNE supply | 
**Locked** | Pointer to [**LockedSupply**](LockedSupply.md) |  | [optional] 
**Total** | **int64** | total RUNE supply | 

## Methods

### NewSupplyResponse

`func NewSupplyResponse(circulating int64, total int64, ) *SupplyResponse`

NewSupplyResponse instantiates a new SupplyResponse object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewSupplyResponseWithDefaults

`func NewSupplyResponseWithDefaults() *SupplyResponse`

NewSupplyResponseWithDefaults instantiates a new SupplyResponse object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetCirculating

`func (o *SupplyResponse) GetCirculating() int64`

GetCirculating returns the Circulating field if non-nil, zero value otherwise.

### GetCirculatingOk

`func (o *SupplyResponse) GetCirculatingOk() (*int64, bool)`

GetCirculatingOk returns a tuple with the Circulating field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCirculating

`func (o *SupplyResponse) SetCirculating(v int64)`

SetCirculating sets Circulating field to given value.


### GetLocked

`func (o *SupplyResponse) GetLocked() LockedSupply`

GetLocked returns the Locked field if non-nil, zero value otherwise.

### GetLockedOk

`func (o *SupplyResponse) GetLockedOk() (*LockedSupply, bool)`

GetLockedOk returns a tuple with the Locked field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetLocked

`func (o *SupplyResponse) SetLocked(v LockedSupply)`

SetLocked sets Locked field to given value.

### HasLocked

`func (o *SupplyResponse) HasLocked() bool`

HasLocked returns a boolean if a field has been set.

### GetTotal

`func (o *SupplyResponse) GetTotal() int64`

GetTotal returns the Total field if non-nil, zero value otherwise.

### GetTotalOk

`func (o *SupplyResponse) GetTotalOk() (*int64, bool)`

GetTotalOk returns a tuple with the Total field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetTotal

`func (o *SupplyResponse) SetTotal(v int64)`

SetTotal sets Total field to given value.



[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


