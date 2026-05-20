# \ReferenceMemosApi

All URIs are relative to *http://localhost*

Method | HTTP request | Description
------------- | ------------- | -------------
[**ReferenceMemo**](ReferenceMemosApi.md#ReferenceMemo) | **Get** /thorchain/memo/{asset}/{reference} | 
[**ReferenceMemoByHash**](ReferenceMemosApi.md#ReferenceMemoByHash) | **Get** /thorchain/memo/{hash} | 
[**ReferenceMemoCheck**](ReferenceMemosApi.md#ReferenceMemoCheck) | **Get** /thorchain/memo/check/{asset}/{amount} | 



## ReferenceMemo

> ReferenceMemoResponse ReferenceMemo(ctx, asset, reference).Height(height).Execute()





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
    asset := "BTC.BTC" // string | 
    reference := "20002" // string | the reference number to lookup
    height := int64(789) // int64 | optional block height, defaults to current tip (optional)

    configuration := openapiclient.NewConfiguration()
    apiClient := openapiclient.NewAPIClient(configuration)
    resp, r, err := apiClient.ReferenceMemosApi.ReferenceMemo(context.Background(), asset, reference).Height(height).Execute()
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error when calling `ReferenceMemosApi.ReferenceMemo``: %v\n", err)
        fmt.Fprintf(os.Stderr, "Full HTTP response: %v\n", r)
    }
    // response from `ReferenceMemo`: ReferenceMemoResponse
    fmt.Fprintf(os.Stdout, "Response from `ReferenceMemosApi.ReferenceMemo`: %v\n", resp)
}
```

### Path Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
**ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
**asset** | **string** |  | 
**reference** | **string** | the reference number to lookup | 

### Other Parameters

Other parameters are passed through a pointer to a apiReferenceMemoRequest struct via the builder pattern


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------


 **height** | **int64** | optional block height, defaults to current tip | 

### Return type

[**ReferenceMemoResponse**](ReferenceMemoResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints)
[[Back to Model list]](../README.md#documentation-for-models)
[[Back to README]](../README.md)


## ReferenceMemoByHash

> ReferenceMemoResponse ReferenceMemoByHash(ctx, hash).Height(height).Execute()





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
    hash := "CF524818D42B63D25BBA0CCC4909F127CAA645C0F9CD07324F2824CC151A64C7" // string | 
    height := int64(789) // int64 | optional block height, defaults to current tip (optional)

    configuration := openapiclient.NewConfiguration()
    apiClient := openapiclient.NewAPIClient(configuration)
    resp, r, err := apiClient.ReferenceMemosApi.ReferenceMemoByHash(context.Background(), hash).Height(height).Execute()
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error when calling `ReferenceMemosApi.ReferenceMemoByHash``: %v\n", err)
        fmt.Fprintf(os.Stderr, "Full HTTP response: %v\n", r)
    }
    // response from `ReferenceMemoByHash`: ReferenceMemoResponse
    fmt.Fprintf(os.Stdout, "Response from `ReferenceMemosApi.ReferenceMemoByHash`: %v\n", resp)
}
```

### Path Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
**ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
**hash** | **string** |  | 

### Other Parameters

Other parameters are passed through a pointer to a apiReferenceMemoByHashRequest struct via the builder pattern


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------

 **height** | **int64** | optional block height, defaults to current tip | 

### Return type

[**ReferenceMemoResponse**](ReferenceMemoResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints)
[[Back to Model list]](../README.md#documentation-for-models)
[[Back to README]](../README.md)


## ReferenceMemoCheck

> ReferenceMemoPreflightResponse ReferenceMemoCheck(ctx, asset, amount).Height(height).Execute()





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
    asset := "BTC.BTC" // string | 
    amount := "20002" // string | the transaction amount in base units to check
    height := int64(789) // int64 | optional block height, defaults to current tip (optional)

    configuration := openapiclient.NewConfiguration()
    apiClient := openapiclient.NewAPIClient(configuration)
    resp, r, err := apiClient.ReferenceMemosApi.ReferenceMemoCheck(context.Background(), asset, amount).Height(height).Execute()
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error when calling `ReferenceMemosApi.ReferenceMemoCheck``: %v\n", err)
        fmt.Fprintf(os.Stderr, "Full HTTP response: %v\n", r)
    }
    // response from `ReferenceMemoCheck`: ReferenceMemoPreflightResponse
    fmt.Fprintf(os.Stdout, "Response from `ReferenceMemosApi.ReferenceMemoCheck`: %v\n", resp)
}
```

### Path Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
**ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
**asset** | **string** |  | 
**amount** | **string** | the transaction amount in base units to check | 

### Other Parameters

Other parameters are passed through a pointer to a apiReferenceMemoCheckRequest struct via the builder pattern


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------


 **height** | **int64** | optional block height, defaults to current tip | 

### Return type

[**ReferenceMemoPreflightResponse**](ReferenceMemoPreflightResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints)
[[Back to Model list]](../README.md#documentation-for-models)
[[Back to README]](../README.md)

