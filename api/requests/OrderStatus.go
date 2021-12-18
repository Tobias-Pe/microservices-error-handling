package requests

const (
	OrderStatusFetch    = "order.status.fetching"
	OrderStatusPay      = "order.status.paying"
	OrderStatusReserve  = "order.status.reserving"
	OrderStatusShip     = "order.status.shipping"
	OrderStatusComplete = "order.status.completed"
	OrderStatusAbort    = "order.status.aborted"
	OrderTopic          = "orders"
)
