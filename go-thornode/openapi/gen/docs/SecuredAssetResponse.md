# SecuredAssetResponse

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Asset** | **string** | secured account asset with \&quot;-\&quot; separator | 
**Supply** | **string** | total share tokens issued for the asset | 
**Depth** | **string** | total deposits of the asset | 

## Methods

### NewSecuredAssetResponse

`func NewSecuredAssetResponse(asset string, supply string, depth string, ) *SecuredAssetResponse`

NewSecuredAssetResponse instantiates a new SecuredAssetResponse object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewSecuredAssetResponseWithDefaults

`func NewSecuredAssetResponseWithDefaults() *SecuredAssetResponse`

NewSecuredAssetResponseWithDefaults instantiates a new SecuredAssetResponse object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetAsset

`func (o *SecuredAssetResponse) GetAsset() string`

GetAsset returns the Asset field if non-nil, zero value otherwise.

### GetAssetOk

`func (o *SecuredAssetResponse) GetAssetOk() (*string, bool)`

GetAssetOk returns a tuple with the Asset field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetAsset

`func (o *SecuredAssetResponse) SetAsset(v string)`

SetAsset sets Asset field to given value.


### GetSupply

`func (o *SecuredAssetResponse) GetSupply() string`

GetSupply returns the Supply field if non-nil, zero value otherwise.

### GetSupplyOk

`func (o *SecuredAssetResponse) GetSupplyOk() (*string, bool)`

GetSupplyOk returns a tuple with the Supply field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSupply

`func (o *SecuredAssetResponse) SetSupply(v string)`

SetSupply sets Supply field to given value.


### GetDepth

`func (o *SecuredAssetResponse) GetDepth() string`

GetDepth returns the Depth field if non-nil, zero value otherwise.

### GetDepthOk

`func (o *SecuredAssetResponse) GetDepthOk() (*string, bool)`

GetDepthOk returns a tuple with the Depth field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetDepth

`func (o *SecuredAssetResponse) SetDepth(v string)`

SetDepth sets Depth field to given value.



[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


