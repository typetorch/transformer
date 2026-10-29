// Port of src/util/functions/parseCommandLine.ts.

package transformer

// commandLine is what the transformer reads from rbxtsc's command line. In goblox it comes from the
// TransformContext rather than argv.
type commandLine struct {
	tsconfigPath string
	project      string
	// projectType is the roblox-ts `--type` option, if it was passed.
	projectType string
}
