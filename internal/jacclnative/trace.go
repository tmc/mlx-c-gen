package jacclnative

import (
	"fmt"
	"os"
)

func tracef(format string, args ...any) {
	if os.Getenv("JACCL_NATIVE_TRACE") == "" {
		return
	}
	fmt.Fprintf(os.Stderr, "jacclnative: "+format+"\n", args...)
}
