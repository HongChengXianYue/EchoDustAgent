package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type ReadFileRangeTool struct {
	Workdir  string
	MaxBytes int
}

func (t *ReadFileRangeTool) Name() string {
	return "read_file_range"
}

func (t *ReadFileRangeTool) Description() string {
	return "Read a line range from a UTF-8 text file using a workdir-relative path."
}

func (t *ReadFileRangeTool) Parameters() json.RawMessage {
	return schemaObject([]string{"path", "start_line", "end_line"}, map[string]any{
		"path": map[string]any{
			"type":        "string",
			"description": "Workdir-relative file path.",
		},
		"start_line": map[string]any{
			"type":        "integer",
			"description": "1-based first line to include.",
		},
		"end_line": map[string]any{
			"type":        "integer",
			"description": "1-based last line to include. Must be >= start_line.",
		},
	})
}

func (t *ReadFileRangeTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var params struct {
		Path      string `json:"path"`
		StartLine int    `json:"start_line"`
		EndLine   int    `json:"end_line"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return Error("invalid arguments: " + err.Error()), nil
	}
	if params.StartLine <= 0 || params.EndLine <= 0 {
		return Error("start_line and end_line must be positive"), nil
	}
	if params.EndLine < params.StartLine {
		return Error("end_line must be greater than or equal to start_line"), nil
	}
	path, err := resolvePath(t.Workdir, params.Path)
	if err != nil {
		return Error(err.Error()), nil
	}
	file, err := os.Open(path)
	if err != nil {
		return Error(err.Error()), nil
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	maxBytes := t.MaxBytes
	if maxBytes <= 0 {
		maxBytes = DefaultOptions().ReadFileMaxBytes
	}
	// Allow longer individual lines than bufio.Scanner's default.
	scanner.Buffer(make([]byte, 0, 64*1024), maxBytes+64*1024)

	var lines []string
	lineNo := 0
	for scanner.Scan() {
		if ctx.Err() != nil {
			return Error(ctx.Err().Error()), nil
		}
		lineNo++
		if lineNo < params.StartLine {
			continue
		}
		if lineNo > params.EndLine {
			break
		}
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return Error(err.Error()), nil
	}
	if len(lines) == 0 {
		return Success(fmt.Sprintf("read %s lines %d-%d (no content in range)", displayPath(t.Workdir, path), params.StartLine, params.EndLine), ""), nil
	}
	output := strings.Join(lines, "\n")
	if lineNo >= params.EndLine {
		output += "\n"
	}
	output = capOutput(output, maxBytes)
	return Success(fmt.Sprintf("read %s lines %d-%d", displayPath(t.Workdir, path), params.StartLine, params.EndLine), output), nil
}
