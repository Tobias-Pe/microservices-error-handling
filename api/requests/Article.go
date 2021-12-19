package requests

const (
	AddToCartRoutingKey    = "api.add.article.cart"
	StockRequestRoutingKey = "stock.article.request"
	SupplyRoutingKey       = "supplier.article.response"
	ArticlesTopic          = "articles"
)

type PutArticleInCartRequest struct {
	ArticleID string
	CartID    string
}

type StockSupplyMessage struct {
	ArticleID string
	Amount    int
}
