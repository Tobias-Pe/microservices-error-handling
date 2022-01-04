package customerrors

import (
	"fmt"
)

var ErrStatusProgressionConflict = fmt.Errorf("status is already further progressed")
