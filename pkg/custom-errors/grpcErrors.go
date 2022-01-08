package customerrors

import "fmt"

var ErrRequestNil = fmt.Errorf("the request was nil")
