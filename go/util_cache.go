// Port of src/util/cache.ts.

package transformer

import (
	"sync"
)

// pkgJsonCache is TypeScript's global `Cache.pkgJsonCache`: only reset when the process restarts.
var pkgJsonCache sync.Map // directory -> *packageJsonResult
