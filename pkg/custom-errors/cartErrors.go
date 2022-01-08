package customerrors

import "fmt"

var ErrNoCartFound = fmt.Errorf("there is no cart for this id")
var ErrCartIdInvalid = fmt.Errorf("the cart id is not a number")
