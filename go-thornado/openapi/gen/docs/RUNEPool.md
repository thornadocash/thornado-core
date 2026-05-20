# RUNEPool

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**ReserveUnits** | **string** | the units of RUNEPool owned by the reserve | 
**PoolUnits** | **string** | the units of RUNEPool owned by providers (including pending) | 
**PendingPoolUnits** | **string** | the units of RUNEPool owned by providers that remain pending | 

## Methods

### NewRUNEPool

`func NewRUNEPool(reserveUnits string, poolUnits string, pendingPoolUnits string, ) *RUNEPool`

NewRUNEPool instantiates a new RUNEPool object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewRUNEPoolWithDefaults

`func NewRUNEPoolWithDefaults() *RUNEPool`

NewRUNEPoolWithDefaults instantiates a new RUNEPool object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetReserveUnits

`func (o *RUNEPool) GetReserveUnits() string`

GetReserveUnits returns the ReserveUnits field if non-nil, zero value otherwise.

### GetReserveUnitsOk

`func (o *RUNEPool) GetReserveUnitsOk() (*string, bool)`

GetReserveUnitsOk returns a tuple with the ReserveUnits field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetReserveUnits

`func (o *RUNEPool) SetReserveUnits(v string)`

SetReserveUnits sets ReserveUnits field to given value.


### GetPoolUnits

`func (o *RUNEPool) GetPoolUnits() string`

GetPoolUnits returns the PoolUnits field if non-nil, zero value otherwise.

### GetPoolUnitsOk

`func (o *RUNEPool) GetPoolUnitsOk() (*string, bool)`

GetPoolUnitsOk returns a tuple with the PoolUnits field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetPoolUnits

`func (o *RUNEPool) SetPoolUnits(v string)`

SetPoolUnits sets PoolUnits field to given value.


### GetPendingPoolUnits

`func (o *RUNEPool) GetPendingPoolUnits() string`

GetPendingPoolUnits returns the PendingPoolUnits field if non-nil, zero value otherwise.

### GetPendingPoolUnitsOk

`func (o *RUNEPool) GetPendingPoolUnitsOk() (*string, bool)`

GetPendingPoolUnitsOk returns a tuple with the PendingPoolUnits field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetPendingPoolUnits

`func (o *RUNEPool) SetPendingPoolUnits(v string)`

SetPendingPoolUnits sets PendingPoolUnits field to given value.



[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


