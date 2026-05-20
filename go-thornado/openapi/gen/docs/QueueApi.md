# \QueueApi

All URIs are relative to *http://localhost*

Method | HTTP request | Description
------------- | ------------- | -------------
[**LimitSwaps**](QueueApi.md#LimitSwaps) | **Get** /thorchain/queue/limit_swaps | 
[**LimitSwapsSummary**](QueueApi.md#LimitSwapsSummary) | **Get** /thorchain/queue/limit_swaps/summary | 
[**Queue**](QueueApi.md#Queue) | **Get** /thorchain/queue | 
[**QueueOutbound**](QueueApi.md#QueueOutbound) | **Get** /thorchain/queue/outbound | 
[**QueueScheduled**](QueueApi.md#QueueScheduled) | **Get** /thorchain/queue/scheduled | 
[**QueueSwap**](QueueApi.md#QueueSwap) | **Get** /thorchain/queue/swap | 



## LimitSwaps

> LimitSwapsResponse LimitSwaps(ctx).Height(height).Offset(offset).Limit(limit).SourceAsset(sourceAsset).TargetAsset(targetAsset).Sender(sender).SortBy(sortBy).SortOrder(sortOrder).Execute()





### Example

```go
package main

import (
    "context"
    "fmt"
    "os"
    openapiclient "./openapi"
)

func main() {
    height := int64(789) // int64 | optional block height, defaults to current tip (optional)
    offset := int32(56) // int32 | Number of items to skip (optional) (default to 0)
    limit := int32(56) // int32 | Number of items to return (optional) (default to 100)
    sourceAsset := "sourceAsset_example" // string | Filter by source asset (e.g., \"BTC.BTC\") (optional)
    targetAsset := "targetAsset_example" // string | Filter by target asset (e.g., \"ETH.ETH\") (optional)
    sender := "sender_example" // string | Filter by sender address (optional)
    sortBy := "sortBy_example" // string | Sort by field (optional) (default to "ratio")
    sortOrder := "sortOrder_example" // string | Sort order (optional) (default to "asc")

    configuration := openapiclient.NewConfiguration()
    apiClient := openapiclient.NewAPIClient(configuration)
    resp, r, err := apiClient.QueueApi.LimitSwaps(context.Background()).Height(height).Offset(offset).Limit(limit).SourceAsset(sourceAsset).TargetAsset(targetAsset).Sender(sender).SortBy(sortBy).SortOrder(sortOrder).Execute()
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error when calling `QueueApi.LimitSwaps``: %v\n", err)
        fmt.Fprintf(os.Stderr, "Full HTTP response: %v\n", r)
    }
    // response from `LimitSwaps`: LimitSwapsResponse
    fmt.Fprintf(os.Stdout, "Response from `QueueApi.LimitSwaps`: %v\n", resp)
}
```

### Path Parameters



### Other Parameters

Other parameters are passed through a pointer to a apiLimitSwapsRequest struct via the builder pattern


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **height** | **int64** | optional block height, defaults to current tip | 
 **offset** | **int32** | Number of items to skip | [default to 0]
 **limit** | **int32** | Number of items to return | [default to 100]
 **sourceAsset** | **string** | Filter by source asset (e.g., \&quot;BTC.BTC\&quot;) | 
 **targetAsset** | **string** | Filter by target asset (e.g., \&quot;ETH.ETH\&quot;) | 
 **sender** | **string** | Filter by sender address | 
 **sortBy** | **string** | Sort by field | [default to &quot;ratio&quot;]
 **sortOrder** | **string** | Sort order | [default to &quot;asc&quot;]

### Return type

[**LimitSwapsResponse**](LimitSwapsResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints)
[[Back to Model list]](../README.md#documentation-for-models)
[[Back to README]](../README.md)


## LimitSwapsSummary

> LimitSwapsSummaryResponse LimitSwapsSummary(ctx).Height(height).SourceAsset(sourceAsset).TargetAsset(targetAsset).Execute()





### Example

```go
package main

import (
    "context"
    "fmt"
    "os"
    openapiclient "./openapi"
)

func main() {
    height := int64(789) // int64 | optional block height, defaults to current tip (optional)
    sourceAsset := "sourceAsset_example" // string | Filter by source asset (e.g., \"BTC.BTC\") (optional)
    targetAsset := "targetAsset_example" // string | Filter by target asset (e.g., \"ETH.ETH\") (optional)

    configuration := openapiclient.NewConfiguration()
    apiClient := openapiclient.NewAPIClient(configuration)
    resp, r, err := apiClient.QueueApi.LimitSwapsSummary(context.Background()).Height(height).SourceAsset(sourceAsset).TargetAsset(targetAsset).Execute()
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error when calling `QueueApi.LimitSwapsSummary``: %v\n", err)
        fmt.Fprintf(os.Stderr, "Full HTTP response: %v\n", r)
    }
    // response from `LimitSwapsSummary`: LimitSwapsSummaryResponse
    fmt.Fprintf(os.Stdout, "Response from `QueueApi.LimitSwapsSummary`: %v\n", resp)
}
```

### Path Parameters



### Other Parameters

Other parameters are passed through a pointer to a apiLimitSwapsSummaryRequest struct via the builder pattern


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **height** | **int64** | optional block height, defaults to current tip | 
 **sourceAsset** | **string** | Filter by source asset (e.g., \&quot;BTC.BTC\&quot;) | 
 **targetAsset** | **string** | Filter by target asset (e.g., \&quot;ETH.ETH\&quot;) | 

### Return type

[**LimitSwapsSummaryResponse**](LimitSwapsSummaryResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints)
[[Back to Model list]](../README.md#documentation-for-models)
[[Back to README]](../README.md)


## Queue

> QueueResponse Queue(ctx).Height(height).Execute()





### Example

```go
package main

import (
    "context"
    "fmt"
    "os"
    openapiclient "./openapi"
)

func main() {
    height := int64(789) // int64 | optional block height, defaults to current tip (optional)

    configuration := openapiclient.NewConfiguration()
    apiClient := openapiclient.NewAPIClient(configuration)
    resp, r, err := apiClient.QueueApi.Queue(context.Background()).Height(height).Execute()
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error when calling `QueueApi.Queue``: %v\n", err)
        fmt.Fprintf(os.Stderr, "Full HTTP response: %v\n", r)
    }
    // response from `Queue`: QueueResponse
    fmt.Fprintf(os.Stdout, "Response from `QueueApi.Queue`: %v\n", resp)
}
```

### Path Parameters



### Other Parameters

Other parameters are passed through a pointer to a apiQueueRequest struct via the builder pattern


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **height** | **int64** | optional block height, defaults to current tip | 

### Return type

[**QueueResponse**](QueueResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints)
[[Back to Model list]](../README.md#documentation-for-models)
[[Back to README]](../README.md)


## QueueOutbound

> []TxOutItem QueueOutbound(ctx).Height(height).Execute()





### Example

```go
package main

import (
    "context"
    "fmt"
    "os"
    openapiclient "./openapi"
)

func main() {
    height := int64(789) // int64 | optional block height, defaults to current tip (optional)

    configuration := openapiclient.NewConfiguration()
    apiClient := openapiclient.NewAPIClient(configuration)
    resp, r, err := apiClient.QueueApi.QueueOutbound(context.Background()).Height(height).Execute()
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error when calling `QueueApi.QueueOutbound``: %v\n", err)
        fmt.Fprintf(os.Stderr, "Full HTTP response: %v\n", r)
    }
    // response from `QueueOutbound`: []TxOutItem
    fmt.Fprintf(os.Stdout, "Response from `QueueApi.QueueOutbound`: %v\n", resp)
}
```

### Path Parameters



### Other Parameters

Other parameters are passed through a pointer to a apiQueueOutboundRequest struct via the builder pattern


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **height** | **int64** | optional block height, defaults to current tip | 

### Return type

[**[]TxOutItem**](TxOutItem.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints)
[[Back to Model list]](../README.md#documentation-for-models)
[[Back to README]](../README.md)


## QueueScheduled

> []TxOutItem QueueScheduled(ctx).Height(height).Execute()





### Example

```go
package main

import (
    "context"
    "fmt"
    "os"
    openapiclient "./openapi"
)

func main() {
    height := int64(789) // int64 | optional block height, defaults to current tip (optional)

    configuration := openapiclient.NewConfiguration()
    apiClient := openapiclient.NewAPIClient(configuration)
    resp, r, err := apiClient.QueueApi.QueueScheduled(context.Background()).Height(height).Execute()
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error when calling `QueueApi.QueueScheduled``: %v\n", err)
        fmt.Fprintf(os.Stderr, "Full HTTP response: %v\n", r)
    }
    // response from `QueueScheduled`: []TxOutItem
    fmt.Fprintf(os.Stdout, "Response from `QueueApi.QueueScheduled`: %v\n", resp)
}
```

### Path Parameters



### Other Parameters

Other parameters are passed through a pointer to a apiQueueScheduledRequest struct via the builder pattern


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **height** | **int64** | optional block height, defaults to current tip | 

### Return type

[**[]TxOutItem**](TxOutItem.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints)
[[Back to Model list]](../README.md#documentation-for-models)
[[Back to README]](../README.md)


## QueueSwap

> []MsgSwap QueueSwap(ctx).Height(height).Execute()





### Example

```go
package main

import (
    "context"
    "fmt"
    "os"
    openapiclient "./openapi"
)

func main() {
    height := int64(789) // int64 | optional block height, defaults to current tip (optional)

    configuration := openapiclient.NewConfiguration()
    apiClient := openapiclient.NewAPIClient(configuration)
    resp, r, err := apiClient.QueueApi.QueueSwap(context.Background()).Height(height).Execute()
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error when calling `QueueApi.QueueSwap``: %v\n", err)
        fmt.Fprintf(os.Stderr, "Full HTTP response: %v\n", r)
    }
    // response from `QueueSwap`: []MsgSwap
    fmt.Fprintf(os.Stdout, "Response from `QueueApi.QueueSwap`: %v\n", resp)
}
```

### Path Parameters



### Other Parameters

Other parameters are passed through a pointer to a apiQueueSwapRequest struct via the builder pattern


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **height** | **int64** | optional block height, defaults to current tip | 

### Return type

[**[]MsgSwap**](MsgSwap.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints)
[[Back to Model list]](../README.md#documentation-for-models)
[[Back to README]](../README.md)

