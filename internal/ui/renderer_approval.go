package ui

import (
	"encoding/json"
	"fmt"
	"strings"

	"local-agent/internal/runtimeevent"
)

func approvalDetail(event runtimeevent.Event, argsLimit int) string {
	if event.Tool == "" {
		return event.Reason
	}
	detail := event.Tool + " [" + string(event.Category) + "]"
	if event.Reason != "" {
		detail += ": " + event.Reason
	}
	if args := approvalArgsDetail(event, argsLimit); args != "" {
		detail += "\n" + args
	}
	return detail
}

func approvalArgsDetail(event runtimeevent.Event, argsLimit int) string {
	switch event.Tool {
	case "run_command":
		if command := jsonArgString(event.Args, "command"); command != "" {
			return "Command: " + command
		}
	case "write_file":
		var args struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if json.Unmarshal(event.Args, &args) == nil {
			return fmt.Sprintf("Path: %s\nContent: %d lines, %d bytes", args.Path, textLineCount(args.Content), len(args.Content))
		}
	case "replace_in_file":
		var args struct {
			Path    string `json:"path"`
			OldText string `json:"old_text"`
			NewText string `json:"new_text"`
		}
		if json.Unmarshal(event.Args, &args) == nil {
			return fmt.Sprintf(
				"Path: %s\nReplace: %d bytes -> %d bytes",
				args.Path,
				len(args.OldText),
				len(args.NewText),
			)
		}
	case "apply_patch":
		var args struct {
			Patch string `json:"patch"`
		}
		if json.Unmarshal(event.Args, &args) == nil {
			return fmt.Sprintf("Patch: %d lines, %d bytes", textLineCount(args.Patch), len(args.Patch))
		}
	}
	if len(event.Args) > 0 {
		return "Args: " + compactJSON(event.Args, argsLimit)
	}
	return ""
}

func textLineCount(text string) int {
	if text == "" {
		return 0
	}
	return strings.Count(text, "\n") + 1
}
