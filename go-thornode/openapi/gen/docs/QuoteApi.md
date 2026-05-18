# \QuoteApi

All URIs are relative to *http://localhost*

Method | HTTP request | Description
------------- | ------------- | -------------
[**Quoteswap**](QuoteApi.md#Quoteswap) | **Get** /thorchain/quote/swap | 



## Quoteswap

> QuoteSwapResponse Quoteswap(ctx).Height(height).FromAsset(fromAsset).ToAsset(toAsset).Amount(amount).Destination(destination).RefundAddress(refundAddress).StreamingInterval(streamingInterval).StreamingQuantity(streamingQuantity).ToleranceBps(toleranceBps).LiquidityToleranceBps(liquidityToleranceBps).AffiliateBps(affiliateBps).Affiliate(affiliate).Execute()





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
    streamingInterval := int64(10) // int64 | the interval in which streaming swaps are swapped (optional)
    streamingQuantity := int64(10) // int64 | the quantity of swaps within a streaming swap (optional)
    toleranceBps := int64(100) // int64 | the maximum basis points from the current feeless swap price to set the limit in the generated memo (optional)
    liquidityToleranceBps := int64(100) // int64 | the maximum basis points of tolerance for pool price movements to set the limit in the generated memo (optional)
    affiliateBps := int64(100) // int64 | the affiliate fee in basis points (optional)
    affiliate := "t" // string | the affiliate (address or thorname) (optional)

    configuration := openapiclient.NewConfiguration()
    apiClient := openapiclient.NewAPIClient(configuration)
    resp, r, err := apiClient.QuoteApi.Quoteswap(context.Background()).Height(height).FromAsset(fromAsset).ToAsset(toAsset).Amount(amount).Destination(destination).RefundAddress(refundAddress).StreamingInterval(streamingInterval).StreamingQuantity(streamingQuantity).ToleranceBps(toleranceBps).LiquidityToleranceBps(liquidityToleranceBps).AffiliateBps(affiliateBps).Affiliate(affiliate).Execute()
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error when calling `QuoteApi.Quoteswap``: %v\n", err)
        fmt.Fprintf(os.Stderr, "Full HTTP response: %v\n", r)
    }
    // response from `Quoteswap`: QuoteSwapResponse
    fmt.Fprintf(os.Stdout, "Response from `QuoteApi.Quoteswap`: %v\n", resp)
}
```

### Path Parameters



### Other Parameters

Other parameters are passed through a pointer to a apiQuoteswapRequest struct via the builder pattern


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **height** | **int64** | optional block height, defaults to current tip | 
 **fromAsset** | **string** | the source asset | 
 **toAsset** | **string** | the target asset | 
 **amount** | **int64** | the source asset amount in 1e8 decimals | 
 **destination** | **string** | the destination address, required to generate memo | 
 **refundAddress** | **string** | the refund address, refunds will be sent here if the swap fails | 
 **streamingInterval** | **int64** | the interval in which streaming swaps are swapped | 
 **streamingQuantity** | **int64** | the quantity of swaps within a streaming swap | 
 **toleranceBps** | **int64** | the maximum basis points from the current feeless swap price to set the limit in the generated memo | 
 **liquidityToleranceBps** | **int64** | the maximum basis points of tolerance for pool price movements to set the limit in the generated memo | 
 **affiliateBps** | **int64** | the affiliate fee in basis points | 
 **affiliate** | **string** | the affiliate (address or thorname) | 

### Return type

[**QuoteSwapResponse**](QuoteSwapResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints)
[[Back to Model list]](../README.md#documentation-for-models)
[[Back to README]](../README.md)

