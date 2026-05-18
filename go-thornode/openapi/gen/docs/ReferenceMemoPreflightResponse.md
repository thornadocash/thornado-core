# ReferenceMemoPreflightResponse

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Reference** | **string** | the reference ID that would be generated from the amount | 
**Available** | **bool** | whether this reference is currently available (not registered or expired) | 
**CanRegister** | **bool** | whether a new registration can be made with this reference | 
**ExpiresAt** | **string** | block height when current registration expires (0 if available) | 
**Memo** | Pointer to **string** | the currently registered memo (only present if not available) | [optional] 
**UsageCount** | **string** | the number of times this reference has been used | 
**MaxUse** | **string** | the maximum number of times this reference can be used (0 &#x3D; unlimited) | 

## Methods

### NewReferenceMemoPreflightResponse

`func NewReferenceMemoPreflightResponse(reference string, available bool, canRegister bool, expiresAt string, usageCount string, maxUse string, ) *ReferenceMemoPreflightResponse`

NewReferenceMemoPreflightResponse instantiates a new ReferenceMemoPreflightResponse object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewReferenceMemoPreflightResponseWithDefaults

`func NewReferenceMemoPreflightResponseWithDefaults() *ReferenceMemoPreflightResponse`

NewReferenceMemoPreflightResponseWithDefaults instantiates a new ReferenceMemoPreflightResponse object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetReference

`func (o *ReferenceMemoPreflightResponse) GetReference() string`

GetReference returns the Reference field if non-nil, zero value otherwise.

### GetReferenceOk

`func (o *ReferenceMemoPreflightResponse) GetReferenceOk() (*string, bool)`

GetReferenceOk returns a tuple with the Reference field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetReference

`func (o *ReferenceMemoPreflightResponse) SetReference(v string)`

SetReference sets Reference field to given value.


### GetAvailable

`func (o *ReferenceMemoPreflightResponse) GetAvailable() bool`

GetAvailable returns the Available field if non-nil, zero value otherwise.

### GetAvailableOk

`func (o *ReferenceMemoPreflightResponse) GetAvailableOk() (*bool, bool)`

GetAvailableOk returns a tuple with the Available field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetAvailable

`func (o *ReferenceMemoPreflightResponse) SetAvailable(v bool)`

SetAvailable sets Available field to given value.


### GetCanRegister

`func (o *ReferenceMemoPreflightResponse) GetCanRegister() bool`

GetCanRegister returns the CanRegister field if non-nil, zero value otherwise.

### GetCanRegisterOk

`func (o *ReferenceMemoPreflightResponse) GetCanRegisterOk() (*bool, bool)`

GetCanRegisterOk returns a tuple with the CanRegister field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCanRegister

`func (o *ReferenceMemoPreflightResponse) SetCanRegister(v bool)`

SetCanRegister sets CanRegister field to given value.


### GetExpiresAt

`func (o *ReferenceMemoPreflightResponse) GetExpiresAt() string`

GetExpiresAt returns the ExpiresAt field if non-nil, zero value otherwise.

### GetExpiresAtOk

`func (o *ReferenceMemoPreflightResponse) GetExpiresAtOk() (*string, bool)`

GetExpiresAtOk returns a tuple with the ExpiresAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetExpiresAt

`func (o *ReferenceMemoPreflightResponse) SetExpiresAt(v string)`

SetExpiresAt sets ExpiresAt field to given value.


### GetMemo

`func (o *ReferenceMemoPreflightResponse) GetMemo() string`

GetMemo returns the Memo field if non-nil, zero value otherwise.

### GetMemoOk

`func (o *ReferenceMemoPreflightResponse) GetMemoOk() (*string, bool)`

GetMemoOk returns a tuple with the Memo field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetMemo

`func (o *ReferenceMemoPreflightResponse) SetMemo(v string)`

SetMemo sets Memo field to given value.

### HasMemo

`func (o *ReferenceMemoPreflightResponse) HasMemo() bool`

HasMemo returns a boolean if a field has been set.

### GetUsageCount

`func (o *ReferenceMemoPreflightResponse) GetUsageCount() string`

GetUsageCount returns the UsageCount field if non-nil, zero value otherwise.

### GetUsageCountOk

`func (o *ReferenceMemoPreflightResponse) GetUsageCountOk() (*string, bool)`

GetUsageCountOk returns a tuple with the UsageCount field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetUsageCount

`func (o *ReferenceMemoPreflightResponse) SetUsageCount(v string)`

SetUsageCount sets UsageCount field to given value.


### GetMaxUse

`func (o *ReferenceMemoPreflightResponse) GetMaxUse() string`

GetMaxUse returns the MaxUse field if non-nil, zero value otherwise.

### GetMaxUseOk

`func (o *ReferenceMemoPreflightResponse) GetMaxUseOk() (*string, bool)`

GetMaxUseOk returns a tuple with the MaxUse field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetMaxUse

`func (o *ReferenceMemoPreflightResponse) SetMaxUse(v string)`

SetMaxUse sets MaxUse field to given value.



[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


