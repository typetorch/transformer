// Port of src/util/functions/getPackageJson.ts.

package transformer

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
)

type packageJson struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Main    string `json:"main"`
	Types   any    `json:"types"`
	Typings any    `json:"typings"`
}

type packageJsonResult struct {
	// directory is the directory containing the package.json.
	directory string
	// path is the path to the package.json.
	path   string
	result packageJson
}

// getPackageJson looks at `directory` and its ancestors until a package.json is found.
func getPackageJson(directory string) *packageJsonResult {
	directory = path.Clean(directory)
	if existing, ok := pkgJsonCache.Load(directory); ok {
		return existing.(*packageJsonResult)
	}

	result := getPackageJsonInner(directory)

	pkgJsonCache.Store(directory, result)
	for dir := directory; ; dir = path.Dir(dir) {
		if !isPathDescendantOf(dir, result.directory) {
			break
		}
		pkgJsonCache.Store(dir, result)
		if dir == "/" {
			break
		}
	}

	return result
}

func getPackageJsonInner(directory string) *packageJsonResult {
	for dir := directory; ; dir = path.Dir(dir) {
		packageJsonPath := path.Join(dir, "package.json")
		if text, err := os.ReadFile(packageJsonPath); err == nil {
			result := &packageJsonResult{directory: dir, path: packageJsonPath}
			_ = json.Unmarshal(text, &result.result)
			return result
		}
		if dir == "/" || dir == "." {
			panic(fmt.Sprintf("package.json not found in %s", directory))
		}
	}
}
