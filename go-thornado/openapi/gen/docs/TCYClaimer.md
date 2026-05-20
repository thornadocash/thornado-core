# TCYClaimer

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**L1Address** | Pointer to **string** |  | [optional] 
**Amount** | **string** |  | 
**Asset** | **string** |  | 

## Methods

### NewTCYClaimer

`func NewTCYClaimer(amount string, asset string, ) *TCYClaimer`

NewTCYClaimer instantiates a new TCYClaimer object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewTCYClaimerWithDefaults

`func NewTCYClaimerWithDefaults() *TCYClaimer`

NewTCYClaimerWithDefaults instantiates a new TCYClaimer object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetL1Address

`func (o *TCYClaimer) GetL1Address() string`

GetL1Address returns the L1Address field if non-nil, zero value otherwise.

### GetL1AddressOk

`func (o *TCYClaimer) GetL1AddressOk() (*string, bool)`

GetL1AddressOk returns a tuple with the L1Address field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetL1Address

`func (o *TCYClaimer) SetL1Address(v string)`

SetL1Address sets L1Address field to given value.

### HasL1Address

`func (o *TCYClaimer) HasL1Address() bool`

HasL1Address returns a boolean if a field has been set.

### GetAmount

`func (o *TCYClaimer) GetAmount() string`

GetAmount returns the Amount field if non-nil, zero value otherwise.

### GetAmountOk

`func (o *TCYClaimer) GetAmountOk() (*string, bool)`

GetAmountOk returns a tuple with the Amount field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetAmount

`func (o *TCYClaimer) SetAmount(v string)`

SetAmount sets Amount field to given value.


### GetAsset

`func (o *TCYClaimer) GetAsset() string`

GetAsset returns the Asset field if non-nil, zero value otherwise.

### GetAssetOk

`func (o *TCYClaimer) GetAssetOk() (*string, bool)`

GetAssetOk returns a tuple with the Asset field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetAsset

`func (o *TCYClaimer) SetAsset(v string)`

SetAsset sets Asset field to given value.



[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


