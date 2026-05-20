# VaultInfo

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**PubKey** | **string** |  | 
**PubKeyEddsa** | Pointer to **string** |  | [optional] 
**Routers** | [**[]VaultRouter**](VaultRouter.md) |  | 
**Membership** | Pointer to **[]string** | the list of node public keys which are members of the vault | [optional] 

## Methods

### NewVaultInfo

`func NewVaultInfo(pubKey string, routers []VaultRouter, ) *VaultInfo`

NewVaultInfo instantiates a new VaultInfo object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewVaultInfoWithDefaults

`func NewVaultInfoWithDefaults() *VaultInfo`

NewVaultInfoWithDefaults instantiates a new VaultInfo object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetPubKey

`func (o *VaultInfo) GetPubKey() string`

GetPubKey returns the PubKey field if non-nil, zero value otherwise.

### GetPubKeyOk

`func (o *VaultInfo) GetPubKeyOk() (*string, bool)`

GetPubKeyOk returns a tuple with the PubKey field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetPubKey

`func (o *VaultInfo) SetPubKey(v string)`

SetPubKey sets PubKey field to given value.


### GetPubKeyEddsa

`func (o *VaultInfo) GetPubKeyEddsa() string`

GetPubKeyEddsa returns the PubKeyEddsa field if non-nil, zero value otherwise.

### GetPubKeyEddsaOk

`func (o *VaultInfo) GetPubKeyEddsaOk() (*string, bool)`

GetPubKeyEddsaOk returns a tuple with the PubKeyEddsa field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetPubKeyEddsa

`func (o *VaultInfo) SetPubKeyEddsa(v string)`

SetPubKeyEddsa sets PubKeyEddsa field to given value.

### HasPubKeyEddsa

`func (o *VaultInfo) HasPubKeyEddsa() bool`

HasPubKeyEddsa returns a boolean if a field has been set.

### GetRouters

`func (o *VaultInfo) GetRouters() []VaultRouter`

GetRouters returns the Routers field if non-nil, zero value otherwise.

### GetRoutersOk

`func (o *VaultInfo) GetRoutersOk() (*[]VaultRouter, bool)`

GetRoutersOk returns a tuple with the Routers field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRouters

`func (o *VaultInfo) SetRouters(v []VaultRouter)`

SetRouters sets Routers field to given value.


### GetMembership

`func (o *VaultInfo) GetMembership() []string`

GetMembership returns the Membership field if non-nil, zero value otherwise.

### GetMembershipOk

`func (o *VaultInfo) GetMembershipOk() (*[]string, bool)`

GetMembershipOk returns a tuple with the Membership field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetMembership

`func (o *VaultInfo) SetMembership(v []string)`

SetMembership sets Membership field to given value.

### HasMembership

`func (o *VaultInfo) HasMembership() bool`

HasMembership returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


