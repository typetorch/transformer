// Port of src/classes/logger.ts.

package transformer

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

var colors = map[string]int{"red": 31, "yellow": 33, "blue": 34, "gray": 90}

// logger writes `[TypeTorch]: ` prefixed lines (TypeScript's static Logger; verbose comes from `--verbose`).
type logger struct {
	out      io.Writer
	verbose  bool
	useColor bool
}

func newLogger(verbose bool) *logger {
	info, err := os.Stdout.Stat()
	isTTY := err == nil && info.Mode()&os.ModeCharDevice != 0
	_, noColor := os.LookupEnv("NO_COLOR")
	return &logger{out: os.Stdout, verbose: verbose, useColor: isTTY && !noColor}
}

func (l *logger) paint(color string, text string) string {
	if !l.useColor {
		return text
	}
	return fmt.Sprintf("\x1b[%dm%s\x1b[39m", colors[color], text)
}

func (l *logger) write(message string) {
	io.WriteString(l.out, message)
}

func (l *logger) writeLine(messages ...any) {
	for _, message := range messages {
		text, ok := message.(string)
		if !ok {
			encoded, _ := json.MarshalIndent(message, "", "\t")
			text = string(encoded)
		}
		prefix := fmt.Sprintf("[%s]: ", l.paint("gray", "TypeTorch"))
		l.write(prefix + strings.ReplaceAll(text, "\n", "\n"+prefix) + "\n")
	}
}

func (l *logger) painted(color string, messages []any) []any {
	result := make([]any, len(messages))
	for i, message := range messages {
		result[i] = l.paint(color, fmt.Sprint(message))
	}
	return result
}

func (l *logger) info(messages ...any) { l.writeLine(l.painted("blue", messages)...) }

func (l *logger) infoIfVerbose(messages ...any) {
	if l.verbose {
		l.info(messages...)
	}
}

func (l *logger) warn(messages ...any)  { l.writeLine(l.painted("yellow", messages)...) }
func (l *logger) error(messages ...any) { l.writeLine(l.painted("red", messages)...) }
