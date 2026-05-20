# RUNEPoolResponse

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Pol** | [**POL**](POL.md) |  | 
**Providers** | [**RUNEPoolResponseProviders**](RUNEPoolResponseProviders.md) |  | 
**Reserve** | [**RUNEPoolResponseReserve**](RUNEPoolResponseReserve.md) |  | 

## Methods

### NewRUNEPoolResponse

`func NewRUNEPoolResponse(pol POL, providers RUNEPoolResponseProviders, reserve RUNEPoolResponseReserve, ) *RUNEPoolResponse`

NewRUNEPoolResponse instantiates a new RUNEPoolResponse object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewRUNEPoolResponseWithDefaults

`func NewRUNEPoolResponseWithDefaults() *RUNEPoolResponse`

NewRUNEPoolResponseWithDefaults instantiates a new RUNEPoolResponse object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetPol

`func (o *RUNEPoolResponse) GetPol() POL`

GetPol returns the Pol field if non-nil, zero value otherwise.

### GetPolOk

`func (o *RUNEPoolResponse) GetPolOk() (*POL, bool)`

GetPolOk returns a tuple with the Pol field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetPol

`func (o *RUNEPoolResponse) SetPol(v POL)`

SetPol sets Pol field to given value.


### GetProviders

`func (o *RUNEPoolResponse) GetProviders() RUNEPoolResponseProviders`

GetProviders returns the Providers field if non-nil, zero value otherwise.

### GetProvidersOk

`func (o *RUNEPoolResponse) GetProvidersOk() (*RUNEPoolResponseProviders, bool)`

GetProvidersOk returns a tuple with the Providers field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetProviders

`func (o *RUNEPoolResponse) SetProviders(v RUNEPoolResponseProviders)`

SetProviders sets Providers field to given value.


### GetReserve

`func (o *RUNEPoolResponse) GetReserve() RUNEPoolResponseReserve`

GetReserve returns the Reserve field if non-nil, zero value otherwise.

### GetReserveOk

`func (o *RUNEPoolResponse) GetReserveOk() (*RUNEPoolResponseReserve, bool)`

GetReserveOk returns a tuple with the Reserve field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetReserve

`func (o *RUNEPoolResponse) SetReserve(v RUNEPoolResponseReserve)`

SetReserve sets Reserve field to given value.



[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


