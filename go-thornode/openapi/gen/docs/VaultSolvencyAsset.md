# VaultSolvencyAsset

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Asset** | **string** | Asset identifier | 
**Amount** | **string** | Solvency amount for the asset. Positive values indicate over-solvency, negative values indicate under-solvency. | 

## Methods

### NewVaultSolvencyAsset

`func NewVaultSolvencyAsset(asset string, amount string, ) *VaultSolvencyAsset`

NewVaultSolvencyAsset instantiates a new VaultSolvencyAsset object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewVaultSolvencyAssetWithDefaults

`func NewVaultSolvencyAssetWithDefaults() *VaultSolvencyAsset`

NewVaultSolvencyAssetWithDefaults instantiates a new VaultSolvencyAsset object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetAsset

`func (o *VaultSolvencyAsset) GetAsset() string`

GetAsset returns the Asset field if non-nil, zero value otherwise.

### GetAssetOk

`func (o *VaultSolvencyAsset) GetAssetOk() (*string, bool)`

GetAssetOk returns a tuple with the Asset field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetAsset

`func (o *VaultSolvencyAsset) SetAsset(v string)`

SetAsset sets Asset field to given value.


### GetAmount

`func (o *VaultSolvencyAsset) GetAmount() string`

GetAmount returns the Amount field if non-nil, zero value otherwise.

### GetAmountOk

`func (o *VaultSolvencyAsset) GetAmountOk() (*string, bool)`

GetAmountOk returns a tuple with the Amount field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetAmount

`func (o *VaultSolvencyAsset) SetAmount(v string)`

SetAmount sets Amount field to given value.



[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


