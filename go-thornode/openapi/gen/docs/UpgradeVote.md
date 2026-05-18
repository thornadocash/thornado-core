# UpgradeVote

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**NodeAddress** | **string** | the node address of the voter | 
**Vote** | **string** | the vote cast by the node | 

## Methods

### NewUpgradeVote

`func NewUpgradeVote(nodeAddress string, vote string, ) *UpgradeVote`

NewUpgradeVote instantiates a new UpgradeVote object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewUpgradeVoteWithDefaults

`func NewUpgradeVoteWithDefaults() *UpgradeVote`

NewUpgradeVoteWithDefaults instantiates a new UpgradeVote object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetNodeAddress

`func (o *UpgradeVote) GetNodeAddress() string`

GetNodeAddress returns the NodeAddress field if non-nil, zero value otherwise.

### GetNodeAddressOk

`func (o *UpgradeVote) GetNodeAddressOk() (*string, bool)`

GetNodeAddressOk returns a tuple with the NodeAddress field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetNodeAddress

`func (o *UpgradeVote) SetNodeAddress(v string)`

SetNodeAddress sets NodeAddress field to given value.


### GetVote

`func (o *UpgradeVote) GetVote() string`

GetVote returns the Vote field if non-nil, zero value otherwise.

### GetVoteOk

`func (o *UpgradeVote) GetVoteOk() (*string, bool)`

GetVoteOk returns a tuple with the Vote field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetVote

`func (o *UpgradeVote) SetVote(v string)`

SetVote sets Vote field to given value.



[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


