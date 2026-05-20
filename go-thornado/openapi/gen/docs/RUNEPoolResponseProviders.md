# RUNEPoolResponseProviders

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Units** | **string** | the units of RUNEPool owned by providers (including pending) | 
**PendingUnits** | **string** | the units of RUNEPool owned by providers that remain pending | 
**PendingRune** | **string** | the amount of RUNE pending | 
**Value** | **string** | the value of the provider share of the RUNEPool (includes pending RUNE) | 
**Pnl** | **string** | the profit and loss of the provider share of the RUNEPool | 
**CurrentDeposit** | **string** | the current RUNE deposited by providers | 

## Methods

### NewRUNEPoolResponseProviders

`func NewRUNEPoolResponseProviders(units string, pendingUnits string, pendingRune string, value string, pnl string, currentDeposit string, ) *RUNEPoolResponseProviders`

NewRUNEPoolResponseProviders instantiates a new RUNEPoolResponseProviders object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewRUNEPoolResponseProvidersWithDefaults

`func NewRUNEPoolResponseProvidersWithDefaults() *RUNEPoolResponseProviders`

NewRUNEPoolResponseProvidersWithDefaults instantiates a new RUNEPoolResponseProviders object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetUnits

`func (o *RUNEPoolResponseProviders) GetUnits() string`

GetUnits returns the Units field if non-nil, zero value otherwise.

### GetUnitsOk

`func (o *RUNEPoolResponseProviders) GetUnitsOk() (*string, bool)`

GetUnitsOk returns a tuple with the Units field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetUnits

`func (o *RUNEPoolResponseProviders) SetUnits(v string)`

SetUnits sets Units field to given value.


### GetPendingUnits

`func (o *RUNEPoolResponseProviders) GetPendingUnits() string`

GetPendingUnits returns the PendingUnits field if non-nil, zero value otherwise.

### GetPendingUnitsOk

`func (o *RUNEPoolResponseProviders) GetPendingUnitsOk() (*string, bool)`

GetPendingUnitsOk returns a tuple with the PendingUnits field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetPendingUnits

`func (o *RUNEPoolResponseProviders) SetPendingUnits(v string)`

SetPendingUnits sets PendingUnits field to given value.


### GetPendingRune

`func (o *RUNEPoolResponseProviders) GetPendingRune() string`

GetPendingRune returns the PendingRune field if non-nil, zero value otherwise.

### GetPendingRuneOk

`func (o *RUNEPoolResponseProviders) GetPendingRuneOk() (*string, bool)`

GetPendingRuneOk returns a tuple with the PendingRune field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetPendingRune

`func (o *RUNEPoolResponseProviders) SetPendingRune(v string)`

SetPendingRune sets PendingRune field to given value.


### GetValue

`func (o *RUNEPoolResponseProviders) GetValue() string`

GetValue returns the Value field if non-nil, zero value otherwise.

### GetValueOk

`func (o *RUNEPoolResponseProviders) GetValueOk() (*string, bool)`

GetValueOk returns a tuple with the Value field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetValue

`func (o *RUNEPoolResponseProviders) SetValue(v string)`

SetValue sets Value field to given value.


### GetPnl

`func (o *RUNEPoolResponseProviders) GetPnl() string`

GetPnl returns the Pnl field if non-nil, zero value otherwise.

### GetPnlOk

`func (o *RUNEPoolResponseProviders) GetPnlOk() (*string, bool)`

GetPnlOk returns a tuple with the Pnl field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetPnl

`func (o *RUNEPoolResponseProviders) SetPnl(v string)`

SetPnl sets Pnl field to given value.


### GetCurrentDeposit

`func (o *RUNEPoolResponseProviders) GetCurrentDeposit() string`

GetCurrentDeposit returns the CurrentDeposit field if non-nil, zero value otherwise.

### GetCurrentDepositOk

`func (o *RUNEPoolResponseProviders) GetCurrentDepositOk() (*string, bool)`

GetCurrentDepositOk returns a tuple with the CurrentDeposit field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCurrentDeposit

`func (o *RUNEPoolResponseProviders) SetCurrentDeposit(v string)`

SetCurrentDeposit sets CurrentDeposit field to given value.



[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


