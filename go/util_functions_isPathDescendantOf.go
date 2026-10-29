// Port of src/util/functions/isPathDescendantOf.ts.

package transformer

import (
	"path"
	"strings"
)

// isPathDescendantOf checks if the `filePath` path is a descendant of the `dirPath` path.
func isPathDescendantOf(filePath string, dirPath string) bool {
	return dirPath == filePath || !strings.HasPrefix(relativePath(dirPath, filePath), "..")
}

// isPathDescendantOfAny checks if `filePath` is a descendant of any of `dirPaths`.
func isPathDescendantOfAny(filePath string, dirPaths []string) bool {
	for _, dirPath := range dirPaths {
		if isPathDescendantOf(filePath, dirPath) {
			return true
		}
	}
	return false
}

// relativePath is Node's path.relative for absolute forward-slash paths.
func relativePath(from string, to string) string {
	from, to = path.Clean(from), path.Clean(to)
	if from == to {
		return ""
	}
	fromParts := splitPath(from)
	toParts := splitPath(to)
	common := 0
	for common < len(fromParts) && common < len(toParts) && fromParts[common] == toParts[common] {
		common++
	}
	parts := make([]string, 0, len(fromParts)-common+len(toParts)-common)
	for range fromParts[common:] {
		parts = append(parts, "..")
	}
	parts = append(parts, toParts[common:]...)
	return strings.Join(parts, "/")
}

func splitPath(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}
