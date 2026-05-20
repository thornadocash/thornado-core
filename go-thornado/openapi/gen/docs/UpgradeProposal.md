# UpgradeProposal

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Name** | **string** | the name of the upgrade | 
**Height** | **int64** | the block height at which the upgrade will occur | 
**Info** | **string** | the description of the upgrade, typically json with URLs to binaries for use with automation tools | 
**Approved** | Pointer to **bool** | whether the upgrade has been approved by the active validators | [optional] 
**ApprovedPercent** | Pointer to **string** | the percentage of active validators that have approved the upgrade | [optional] 
**ValidatorsToQuorum** | Pointer to **int64** | the amount of additional active validators required to reach quorum for the upgrade | [optional] 
**Approvers** | Pointer to **[]string** | the list of node addresses that have approved the upgrade | [optional] 
**Rejecters** | Pointer to **[]string** | the list of node addresses that have rejected the upgrade | [optional] 

## Methods

### NewUpgradeProposal

`func NewUpgradeProposal(name string, height int64, info string, ) *UpgradeProposal`

NewUpgradeProposal instantiates a new UpgradeProposal object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewUpgradeProposalWithDefaults

`func NewUpgradeProposalWithDefaults() *UpgradeProposal`

NewUpgradeProposalWithDefaults instantiates a new UpgradeProposal object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetName

`func (o *UpgradeProposal) GetName() string`

GetName returns the Name field if non-nil, zero value otherwise.

### GetNameOk

`func (o *UpgradeProposal) GetNameOk() (*string, bool)`

GetNameOk returns a tuple with the Name field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetName

`func (o *UpgradeProposal) SetName(v string)`

SetName sets Name field to given value.


### GetHeight

`func (o *UpgradeProposal) GetHeight() int64`

GetHeight returns the Height field if non-nil, zero value otherwise.

### GetHeightOk

`func (o *UpgradeProposal) GetHeightOk() (*int64, bool)`

GetHeightOk returns a tuple with the Height field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetHeight

`func (o *UpgradeProposal) SetHeight(v int64)`

SetHeight sets Height field to given value.


### GetInfo

`func (o *UpgradeProposal) GetInfo() string`

GetInfo returns the Info field if non-nil, zero value otherwise.

### GetInfoOk

`func (o *UpgradeProposal) GetInfoOk() (*string, bool)`

GetInfoOk returns a tuple with the Info field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetInfo

`func (o *UpgradeProposal) SetInfo(v string)`

SetInfo sets Info field to given value.


### GetApproved

`func (o *UpgradeProposal) GetApproved() bool`

GetApproved returns the Approved field if non-nil, zero value otherwise.

### GetApprovedOk

`func (o *UpgradeProposal) GetApprovedOk() (*bool, bool)`

GetApprovedOk returns a tuple with the Approved field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetApproved

`func (o *UpgradeProposal) SetApproved(v bool)`

SetApproved sets Approved field to given value.

### HasApproved

`func (o *UpgradeProposal) HasApproved() bool`

HasApproved returns a boolean if a field has been set.

### GetApprovedPercent

`func (o *UpgradeProposal) GetApprovedPercent() string`

GetApprovedPercent returns the ApprovedPercent field if non-nil, zero value otherwise.

### GetApprovedPercentOk

`func (o *UpgradeProposal) GetApprovedPercentOk() (*string, bool)`

GetApprovedPercentOk returns a tuple with the ApprovedPercent field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetApprovedPercent

`func (o *UpgradeProposal) SetApprovedPercent(v string)`

SetApprovedPercent sets ApprovedPercent field to given value.

### HasApprovedPercent

`func (o *UpgradeProposal) HasApprovedPercent() bool`

HasApprovedPercent returns a boolean if a field has been set.

### GetValidatorsToQuorum

`func (o *UpgradeProposal) GetValidatorsToQuorum() int64`

GetValidatorsToQuorum returns the ValidatorsToQuorum field if non-nil, zero value otherwise.

### GetValidatorsToQuorumOk

`func (o *UpgradeProposal) GetValidatorsToQuorumOk() (*int64, bool)`

GetValidatorsToQuorumOk returns a tuple with the ValidatorsToQuorum field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetValidatorsToQuorum

`func (o *UpgradeProposal) SetValidatorsToQuorum(v int64)`

SetValidatorsToQuorum sets ValidatorsToQuorum field to given value.

### HasValidatorsToQuorum

`func (o *UpgradeProposal) HasValidatorsToQuorum() bool`

HasValidatorsToQuorum returns a boolean if a field has been set.

### GetApprovers

`func (o *UpgradeProposal) GetApprovers() []string`

GetApprovers returns the Approvers field if non-nil, zero value otherwise.

### GetApproversOk

`func (o *UpgradeProposal) GetApproversOk() (*[]string, bool)`

GetApproversOk returns a tuple with the Approvers field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetApprovers

`func (o *UpgradeProposal) SetApprovers(v []string)`

SetApprovers sets Approvers field to given value.

### HasApprovers

`func (o *UpgradeProposal) HasApprovers() bool`

HasApprovers returns a boolean if a field has been set.

### GetRejecters

`func (o *UpgradeProposal) GetRejecters() []string`

GetRejecters returns the Rejecters field if non-nil, zero value otherwise.

### GetRejectersOk

`func (o *UpgradeProposal) GetRejectersOk() (*[]string, bool)`

GetRejectersOk returns a tuple with the Rejecters field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRejecters

`func (o *UpgradeProposal) SetRejecters(v []string)`

SetRejecters sets Rejecters field to given value.

### HasRejecters

`func (o *UpgradeProposal) HasRejecters() bool`

HasRejecters returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


