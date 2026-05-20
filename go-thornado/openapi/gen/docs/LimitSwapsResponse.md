# LimitSwapsResponse

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**LimitSwaps** | Pointer to [**[]LimitSwapWithDetails**](LimitSwapWithDetails.md) | Array of limit swaps with details | [optional] 
**Pagination** | Pointer to [**PaginationMeta**](PaginationMeta.md) |  | [optional] 

## Methods

### NewLimitSwapsResponse

`func NewLimitSwapsResponse() *LimitSwapsResponse`

NewLimitSwapsResponse instantiates a new LimitSwapsResponse object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewLimitSwapsResponseWithDefaults

`func NewLimitSwapsResponseWithDefaults() *LimitSwapsResponse`

NewLimitSwapsResponseWithDefaults instantiates a new LimitSwapsResponse object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetLimitSwaps

`func (o *LimitSwapsResponse) GetLimitSwaps() []LimitSwapWithDetails`

GetLimitSwaps returns the LimitSwaps field if non-nil, zero value otherwise.

### GetLimitSwapsOk

`func (o *LimitSwapsResponse) GetLimitSwapsOk() (*[]LimitSwapWithDetails, bool)`

GetLimitSwapsOk returns a tuple with the LimitSwaps field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetLimitSwaps

`func (o *LimitSwapsResponse) SetLimitSwaps(v []LimitSwapWithDetails)`

SetLimitSwaps sets LimitSwaps field to given value.

### HasLimitSwaps

`func (o *LimitSwapsResponse) HasLimitSwaps() bool`

HasLimitSwaps returns a boolean if a field has been set.

### GetPagination

`func (o *LimitSwapsResponse) GetPagination() PaginationMeta`

GetPagination returns the Pagination field if non-nil, zero value otherwise.

### GetPaginationOk

`func (o *LimitSwapsResponse) GetPaginationOk() (*PaginationMeta, bool)`

GetPaginationOk returns a tuple with the Pagination field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetPagination

`func (o *LimitSwapsResponse) SetPagination(v PaginationMeta)`

SetPagination sets Pagination field to given value.

### HasPagination

`func (o *LimitSwapsResponse) HasPagination() bool`

HasPagination returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


