package approval

import (
	"encoding/json"
	"regexp"
	"strings"
)

var blockedCommandPatterns = []struct {
	reason string
	regex  *regexp.Regexp
}{
	{
		reason: "root filesystem deletion is permanently blocked",
		regex:  regexp.MustCompile(`(?i)\brm\s+(?:-[^\s]*[rf][^\s]*\s+){1,4}(?:--no-preserve-root\s+)?['"]?/\*?['"]?(?:\s|$|[;&|])`),
	},
	{
		reason: "Windows drive deletion is permanently blocked",
		regex:  regexp.MustCompile(`(?i)\b(?:rd|rmdir)\s+(?:/[sq]\s+){1,4}['"]?[a-z]:[\\/]?\*?['"]?(?:\s|$|[;&|])`),
	},
	{
		reason: "Windows drive deletion is permanently blocked",
		regex:  regexp.MustCompile(`(?i)\bdel\s+(?:/[fsq]\s+){1,4}['"]?[a-z]:[\\/]\*['"]?(?:\s|$|[;&|])`),
	},
	{
		reason: "PowerShell drive deletion is permanently blocked",
		regex:  regexp.MustCompile(`(?i)\b(?:remove-item|rm|del)\s+.*(?:-recurse|-r)\s+.*(?:-force|-f)\s+['"]?[a-z]:[\\/]?\*?['"]?(?:\s|$|[;&|])`),
	},
	{
		reason: "formatting a drive is permanently blocked",
		regex:  regexp.MustCompile(`(?i)\bformat\s+[a-z]:`),
	},
	{
		reason: "formatting a block device is permanently blocked",
		regex:  regexp.MustCompile(`(?i)\bmkfs(?:\.[a-z0-9_+-]+)?\s+/dev/[^\s]+`),
	},
	{
		reason: "disk erase is permanently blocked",
		regex:  regexp.MustCompile(`(?i)\bdiskutil\s+erase(?:disk|volume)\b`),
	},
	{
		reason: "raw block-device overwrite is permanently blocked",
		regex:  regexp.MustCompile(`(?i)\bdd\s+.*\bof=/dev/[^\s]+`),
	},
}

func BlockReason(tool string, args json.RawMessage) (string, bool) {
	if tool != "run_command" {
		return "", false
	}
	command := CommandFromArgs(args)
	if command == "" {
		return "", false
	}
	for _, pattern := range blockedCommandPatterns {
		if pattern.regex.MatchString(command) {
			return pattern.reason, true
		}
	}
	for _, segment := range splitCommandSegments(command) {
		if isUnixRootDelete(segment) {
			return "root filesystem deletion is permanently blocked", true
		}
		if isWindowsDriveDelete(segment) {
			return "Windows drive deletion is permanently blocked", true
		}
	}
	return "", false
}

func isUnixRootDelete(segment string) bool {
	fields := significantFields(segment)
	if len(fields) == 0 {
		return false
	}
	if executableName(fields[0]) == "sudo" || executableName(fields[0]) == "doas" {
		fields = fields[1:]
	}
	if len(fields) < 2 || executableName(fields[0]) != "rm" {
		return false
	}
	recursive := false
	force := false
	for _, field := range fields[1:] {
		arg := strings.Trim(strings.ToLower(field), `"'`)
		switch {
		case arg == "--recursive":
			recursive = true
		case arg == "--force":
			force = true
		case strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--"):
			recursive = recursive || strings.Contains(arg, "r") || strings.Contains(arg, "R")
			force = force || strings.Contains(arg, "f")
		}
	}
	if !recursive || !force {
		return false
	}
	for _, field := range fields[1:] {
		if isRootTarget(field) {
			return true
		}
	}
	return false
}

func isRootTarget(field string) bool {
	target := strings.Trim(strings.ToLower(field), `"'`)
	target = strings.TrimSuffix(target, "/")
	return target == "" || target == "/" || target == "/*"
}

func isWindowsDriveDelete(segment string) bool {
	fields := significantFields(segment)
	if len(fields) == 0 {
		return false
	}
	cmd := executableName(fields[0])
	if cmd != "rd" && cmd != "rmdir" && cmd != "del" {
		return false
	}
	recursive := false
	quietOrForce := false
	for _, field := range fields[1:] {
		arg := strings.ToLower(strings.Trim(field, `"'`))
		recursive = recursive || arg == "/s"
		quietOrForce = quietOrForce || arg == "/q" || arg == "/f"
	}
	if !recursive || !quietOrForce {
		return false
	}
	for _, field := range fields[1:] {
		if isWindowsDriveRoot(field) {
			return true
		}
	}
	return false
}

func isWindowsDriveRoot(field string) bool {
	target := strings.ToLower(strings.Trim(field, `"'`))
	target = strings.ReplaceAll(target, "/", `\`)
	target = strings.TrimSuffix(target, `\*`)
	target = strings.TrimSuffix(target, `\`)
	return regexp.MustCompile(`^[a-z]:$`).MatchString(target)
}
