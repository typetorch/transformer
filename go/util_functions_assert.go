// Port of src/util/functions/assert.ts.

package transformer

import (
	"fmt"
	"strings"
)

// assert panics (a transformer bug, reported like any other crash) when value is false.
func assert(value bool, message ...string) {
	if !value {
		panic(fmt.Sprintf("Assertion Failed! %s\nPlease submit a bug report here:\nhttps://github.com/typetorch/transformer/issues",
			strings.Join(message, "")))
	}
}
