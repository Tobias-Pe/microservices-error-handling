package requests

const (
	StockRequestRoutingKey = "stock.article.request"
	StockUpdateRoutingKey  = "stock.update"
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
