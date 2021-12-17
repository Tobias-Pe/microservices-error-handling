package requests

const (
	AddToCartRoutingKey = "api.add.article.cart"
	CartTopic           = "cart"
)

type PutArticleInCartRequest struct {
	ArticleID string
	CartID    string
}
