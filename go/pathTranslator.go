// Port of src/classes/pathTranslator/index.ts and constants.ts (roblox-ts' PathTranslator), plus
// src/util/functions/createPathTranslator.ts.
//
// Paths are typescript-go's normalized forward-slash paths, so `path` (not `path/filepath`) is used throughout.

package transformer

import (
	"path"
	"strings"

	ts "github.com/microsoft/typescript-go/gobloxshim"
)

const (
	tsExt     = ".ts"
	tsxExt    = ".tsx"
	dExt      = ".d"
	luaExt    = ".lua"
	indexName = "index"
	initName  = "init"
)

type pathInfo struct {
	dirName  string
	fileName string
	exts     []string
}

func pathInfoFrom(filePath string) *pathInfo {
	dirName := path.Dir(filePath)
	base := filePath[len(dirName)+1:]
	if dirName == "/" {
		base = filePath[1:]
	}
	parts := strings.Split(base, ".")
	exts := make([]string, 0, len(parts)-1)
	for _, part := range parts[1:] {
		exts = append(exts, "."+part)
	}
	return &pathInfo{dirName: dirName, fileName: parts[0], exts: exts}
}

func (p *pathInfo) extsPeek(depth int) string {
	index := len(p.exts) - (depth + 1)
	if index < 0 {
		return ""
	}
	return p.exts[index]
}

func (p *pathInfo) popExt() string {
	ext := p.exts[len(p.exts)-1]
	p.exts = p.exts[:len(p.exts)-1]
	return ext
}

func (p *pathInfo) join() string {
	return path.Join(p.dirName, p.fileName+strings.Join(p.exts, ""))
}

type pathTranslator struct {
	rootDir             string
	outDir              string
	buildInfoOutputPath string
	declaration         bool
}

func (t *pathTranslator) makeRelative(from string, to string) func(*pathInfo) string {
	return func(info *pathInfo) string {
		return path.Join(to, relativePath(from, info.join()))
	}
}

// getOutputPath maps an input path to an output path:
//   - `.tsx?` && !`.d.tsx?` -> `.lua`, `index` -> `init`
//   - `src/*` -> `out/*`
func (t *pathTranslator) getOutputPath(filePath string) string {
	makeRelative := t.makeRelative(t.rootDir, t.outDir)
	info := pathInfoFrom(filePath)

	if (info.extsPeek(0) == tsExt || info.extsPeek(0) == tsxExt) && info.extsPeek(1) != dExt {
		info.popExt()
		if info.fileName == indexName {
			info.fileName = initName
		}
		info.exts = append(info.exts, luaExt)
	}

	return makeRelative(info)
}

// getInputPaths maps an output path to possible import paths:
//   - `.lua` -> `.tsx?`, `init` -> `index`
//   - `out/*` -> `src/*`
func (t *pathTranslator) getInputPaths(filePath string) []string {
	makeRelative := t.makeRelative(t.outDir, t.rootDir)
	var possiblePaths []string
	info := pathInfoFrom(filePath)

	// index.*.lua cannot come from a .ts file
	if info.extsPeek(0) == luaExt && info.fileName != indexName {
		info.popExt()
		originalFileName := info.fileName

		if info.fileName == initName {
			info.fileName = indexName
		}

		for _, ext := range []string{tsExt, tsxExt} {
			info.exts = append(info.exts, ext)
			possiblePaths = append(possiblePaths, makeRelative(info))
			info.popExt()
		}

		info.fileName = originalFileName
		info.exts = append(info.exts, luaExt)
	}

	if t.declaration {
		if (info.extsPeek(0) == tsExt || info.extsPeek(0) == tsxExt) && info.extsPeek(1) == dExt {
			tsExtension := info.popExt()
			info.popExt() // .d

			for _, ext := range []string{tsExt, tsxExt} {
				info.exts = append(info.exts, ext)
				possiblePaths = append(possiblePaths, makeRelative(info))
				info.popExt()
			}

			info.exts = append(info.exts, dExt, tsExtension)
		}
	}

	return append(possiblePaths, makeRelative(info))
}

// getImportPath maps a src path to an import path: `.d.tsx?` -> `.tsx?` -> `.lua`, `index` -> `init`.
func (t *pathTranslator) getImportPath(filePath string, isNodeModule bool) string {
	makeRelative := t.makeRelative(t.rootDir, t.outDir)
	info := pathInfoFrom(filePath)

	if info.extsPeek(0) == tsExt || info.extsPeek(0) == tsxExt {
		info.popExt()
		if info.extsPeek(0) == dExt {
			info.popExt()
		}
		if info.fileName == indexName {
			info.fileName = initName
		}
		info.exts = append(info.exts, luaExt)
	}

	if isNodeModule {
		return info.join()
	}
	return makeRelative(info)
}

/// createPathTranslator.ts

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
