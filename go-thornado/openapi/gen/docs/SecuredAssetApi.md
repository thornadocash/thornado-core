# \SecuredAssetApi

All URIs are relative to *http://localhost*

Method | HTTP request | Description
------------- | ------------- | -------------
[**SecuredAsset**](SecuredAssetApi.md#SecuredAsset) | **Get** /thorchain/securedasset/{asset} | 



## SecuredAsset

> SecuredAssetResponse SecuredAsset(ctx, asset).Height(height).Execute()





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
    height := int64(789) // int64 | optional block height, defaults to current tip (optional)

    configuration := openapiclient.NewConfiguration()
    apiClient := openapiclient.NewAPIClient(configuration)
    resp, r, err := apiClient.SecuredAssetApi.SecuredAsset(context.Background(), asset).Height(height).Execute()
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error when calling `SecuredAssetApi.SecuredAsset``: %v\n", err)
        fmt.Fprintf(os.Stderr, "Full HTTP response: %v\n", r)
    }
    // response from `SecuredAsset`: SecuredAssetResponse
    fmt.Fprintf(os.Stdout, "Response from `SecuredAssetApi.SecuredAsset`: %v\n", resp)
}
```

### Path Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
**ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
**asset** | **string** |  | 

### Other Parameters

Other parameters are passed through a pointer to a apiSecuredAssetRequest struct via the builder pattern


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------

 **height** | **int64** | optional block height, defaults to current tip | 

### Return type

[**SecuredAssetResponse**](SecuredAssetResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints)
[[Back to Model list]](../README.md#documentation-for-models)
[[Back to README]](../README.md)

