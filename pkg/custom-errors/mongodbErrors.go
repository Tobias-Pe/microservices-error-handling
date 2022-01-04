package customerrors

import "fmt"

var ErrNoReservation = fmt.Errorf("there is no reservation for this order")

var ErrLowStock = fmt.Errorf("could not reserve an article. there is not enough on stock")

var ErrNoModification = fmt.Errorf("there was no modification")
