# \LimitOrderApi

All URIs are relative to *http://localhost*

Method | HTTP request | Description
------------- | ------------- | -------------
[**Quotelimit**](LimitOrderApi.md#Quotelimit) | **Get** /thorchain/quote/limit | 



## Quotelimit

> QuoteLimitResponse Quotelimit(ctx).Height(height).FromAsset(fromAsset).ToAsset(toAsset).Amount(amount).Destination(destination).RefundAddress(refundAddress).CustomTtl(customTtl).StreamingQuantity(streamingQuantity).AffiliateBps(affiliateBps).Affiliate(affiliate).Execute()





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
    fromAsset := "BTC.BTC" // string | the source asset (optional)
    toAsset := "ETH.ETH" // string | the target asset (optional)
    amount := int64(1000000) // int64 | the source asset amount in 1e8 decimals (optional)
    destination := "0x1c7b17362c84287bd1184447e6dfeaf920c31bbe" // string | the destination address, required to generate memo (optional)
    refundAddress := "0x1c7b17362c84287bd1184447e6dfeaf920c31bbe" // string | the refund address, refunds will be sent here if the swap fails (optional)
    customTtl := int64(10) // int64 | the custom TTL in blocks for limit orders (optional)
    streamingQuantity := int64(10) // int64 | the quantity of swaps within a streaming swap (optional)
    affiliateBps := int64(100) // int64 | the affiliate fee in basis points (optional)
    affiliate := "t" // string | the affiliate (address or thorname) (optional)

    configuration := openapiclient.NewConfiguration()
    apiClient := openapiclient.NewAPIClient(configuration)
    resp, r, err := apiClient.LimitOrderApi.Quotelimit(context.Background()).Height(height).FromAsset(fromAsset).ToAsset(toAsset).Amount(amount).Destination(destination).RefundAddress(refundAddress).CustomTtl(customTtl).StreamingQuantity(streamingQuantity).AffiliateBps(affiliateBps).Affiliate(affiliate).Execute()
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error when calling `LimitOrderApi.Quotelimit``: %v\n", err)
        fmt.Fprintf(os.Stderr, "Full HTTP response: %v\n", r)
    }
    // response from `Quotelimit`: QuoteLimitResponse
    fmt.Fprintf(os.Stdout, "Response from `LimitOrderApi.Quotelimit`: %v\n", resp)
}
```

### Path Parameters



### Other Parameters

Other parameters are passed through a pointer to a apiQuotelimitRequest struct via the builder pattern


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **height** | **int64** | optional block height, defaults to current tip | 
 **fromAsset** | **string** | the source asset | 
 **toAsset** | **string** | the target asset | 
 **amount** | **int64** | the source asset amount in 1e8 decimals | 
 **destination** | **string** | the destination address, required to generate memo | 
 **refundAddress** | **string** | the refund address, refunds will be sent here if the swap fails | 
 **customTtl** | **int64** | the custom TTL in blocks for limit orders | 
 **streamingQuantity** | **int64** | the quantity of swaps within a streaming swap | 
 **affiliateBps** | **int64** | the affiliate fee in basis points | 
 **affiliate** | **string** | the affiliate (address or thorname) | 

### Return type

[**QuoteLimitResponse**](QuoteLimitResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints)
[[Back to Model list]](../README.md#documentation-for-models)
[[Back to README]](../README.md)

