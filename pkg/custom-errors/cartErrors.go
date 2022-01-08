package customerrors

import "fmt"

var ErrNoCartFound = fmt.Errorf("there is no cart for this id")
