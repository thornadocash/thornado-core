# LockedSupply

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Reserve** | Pointer to **int64** | RUNE locked in the reserve module | [optional] 

## Methods

### NewLockedSupply

`func NewLockedSupply() *LockedSupply`

NewLockedSupply instantiates a new LockedSupply object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewLockedSupplyWithDefaults

`func NewLockedSupplyWithDefaults() *LockedSupply`

NewLockedSupplyWithDefaults instantiates a new LockedSupply object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetReserve

`func (o *LockedSupply) GetReserve() int64`

GetReserve returns the Reserve field if non-nil, zero value otherwise.

### GetReserveOk

`func (o *LockedSupply) GetReserveOk() (*int64, bool)`

GetReserveOk returns a tuple with the Reserve field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetReserve

`func (o *LockedSupply) SetReserve(v int64)`

SetReserve sets Reserve field to given value.

### HasReserve

`func (o *LockedSupply) HasReserve() bool`

HasReserve returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


