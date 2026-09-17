package product

// Product error codes, part of the API contract (see openapi.yaml).
const (
	CodeProductNotFound     = "PRODUCT_NOT_FOUND"
	CodeInvalidProductPrice = "INVALID_PRODUCT_PRICE"
	CodeProductQueryFailed  = "PRODUCT_QUERY_FAILED"
	CodeProductCreateFailed = "PRODUCT_CREATE_FAILED"
	CodeProductUpdateFailed = "PRODUCT_UPDATE_FAILED"
	CodeProductDeleteFailed = "PRODUCT_DELETE_FAILED"
)
