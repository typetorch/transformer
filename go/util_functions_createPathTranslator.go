// Port of src/util/functions/createPathTranslator.ts.

package transformer

import (
	"path"
	"strings"

	ts "github.com/microsoft/typescript-go/pkg/goblox"
)

func findAncestorDir(dirs []string) string {
	for i, dir := range dirs {
		dir = path.Clean(dir)
		if !strings.HasSuffix(dir, "/") {
			dir += "/"
		}
		dirs[i] = dir
	}
	currentDir := dirs[0]
	for {
		all := true
		for _, dir := range dirs {
			if !strings.HasPrefix(dir, currentDir) {
				all = false
				break
			}
		}
		if all {
			return currentDir
		}
		// Node's path.join(currentDir, ".."): no trailing separator from here on.
		currentDir = path.Join(currentDir, "..")
	}
}

func getRootDirs(options *ts.CompilerOptions) []string {
	if options.RootDir != "" {
		return []string{options.RootDir}
	}
	if len(options.RootDirs) == 0 {
		assert(false, "rootDir or rootDirs must be specified")
	}
	return options.RootDirs
}

func createPathTranslator(program *ts.Program) *pathTranslator {
	options := program.Options()
	rootDir := findAncestorDir(append([]string{program.CommonSourceDirectory()}, getRootDirs(options)...))
	return &pathTranslator{rootDir: rootDir, outDir: options.OutDir, declaration: options.Declaration.IsTrue()}
}
