package requests

const (
	AddToCartRoutingKey = "api.add.article.cart"
	CartTopic           = "articles"
)

type PutArticleInCartRequest struct {
	ArticleID string
	CartID    string
}
