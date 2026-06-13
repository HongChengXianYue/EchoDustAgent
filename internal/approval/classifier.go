package approval

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

func Classify(tool string, args json.RawMessage) Category {
	switch tool {
	case "list_files", "read_file":
		return CategoryReadOnly
	case "search_files":
		return CategorySearchInspect
	case "write_file", "replace_in_file", "apply_patch":
		return CategoryWorkspaceWrite
	case "run_command":
		return ClassifyCommand(CommandFromArgs(args))
	default:
		return CategoryExternalOrDestructive
	}
}

func CommandFromArgs(args json.RawMessage) string {
	var params struct {
		Command string `json:"command"`
	}
	_ = json.Unmarshal(args, &params)
	return strings.TrimSpace(params.Command)
}

func ClassifyCommand(command string) Category {
	segments := splitCommandSegments(command)
	if len(segments) == 0 {
		return CategoryExternalOrDestructive
	}

	category := CategoryReadOnly
	for _, segment := range segments {
		category = higherRisk(category, classifyCommandSegment(segment))
	}
	return category
}

func classifyCommandSegment(segment string) Category {
	fields := significantFields(segment)
	if len(fields) == 0 {
		return CategoryExternalOrDestructive
	}
	cmd := executableName(fields[0])
	args := fields[1:]

	if isSystemCommand(cmd) {
		return CategorySystemPrivileged
	}
	if isDestructiveCommand(cmd, args) {
		return CategoryExternalOrDestructive
	}
	if cmd == "git" {
		return classifyGit(args)
	}
	if isNetworkCommand(cmd, args) {
		return CategoryNetworkDependency
	}
	if isWorkspaceWriteCommand(cmd, args) {
		return CategoryWorkspaceWrite
	}
	if isBuildTestCommand(cmd, args) {
		return CategoryBuildTest
	}
	if isSearchInspectCommand(cmd, args) {
		return CategorySearchInspect
	}
	if isReadOnlyCommand(cmd, args) {
		return CategoryReadOnly
	}
	return CategoryExternalOrDestructive
}

func splitCommandSegments(command string) []string {
	parts := strings.FieldsFunc(command, func(r rune) bool {
		return r == ';' || r == '|' || r == '&' || r == '\n'
	})
	segments := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			segments = append(segments, trimmed)
		}
	}
	return segments
}

func significantFields(segment string) []string {
	fields := strings.Fields(segment)
	for len(fields) > 0 {
		cmd := executableName(fields[0])
		switch {
		case cmd == "env":
			fields = fields[1:]
		case strings.Contains(fields[0], "=") && !strings.HasPrefix(fields[0], "-"):
			fields = fields[1:]
		default:
			return fields
		}
	}
	return nil
}

func executableName(value string) string {
	value = strings.Trim(value, `"'`)
	value = strings.ReplaceAll(value, "\\", "/")
	value = filepath.Base(value)
	value = strings.ToLower(value)
	for _, suffix := range []string{".exe", ".cmd", ".bat", ".ps1"} {
		value = strings.TrimSuffix(value, suffix)
	}
	return value
}

func firstArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return strings.ToLower(strings.Trim(args[0], `"'`))
}

func hasArg(args []string, want string) bool {
	for _, arg := range args {
		if strings.EqualFold(strings.Trim(arg, `"'`), want) {
			return true
		}
	}
	return false
}

func isSystemCommand(cmd string) bool {
	switch cmd {
	case "sudo", "doas", "pkexec", "runas", "su", "systemctl", "launchctl", "service", "sc", "reg", "netsh", "diskpart":
		return true
	default:
		return false
	}
}

func isDestructiveCommand(cmd string, args []string) bool {
	switch cmd {
	case "rm", "rmdir", "rd", "del", "erase", "format", "mkfs", "mkfs.ext4", "mkfs.ntfs", "diskutil", "dd":
		return true
	case "git":
		action := firstArg(args)
		return action == "clean" || (action == "reset" && hasArg(args, "--hard"))
	default:
		return false
	}
}

func classifyGit(args []string) Category {
	switch firstArg(args) {
	case "status", "diff", "log", "show", "branch":
		return CategoryReadOnly
	case "grep":
		return CategorySearchInspect
	case "fetch", "pull", "push", "clone", "submodule":
		return CategoryNetworkDependency
	case "clean":
		return CategoryExternalOrDestructive
	case "reset":
		if hasArg(args, "--hard") {
			return CategoryExternalOrDestructive
		}
		return CategoryVCSLocal
	case "add", "commit", "restore", "checkout", "switch", "merge", "rebase", "stash", "tag":
		return CategoryVCSLocal
	default:
		return CategoryExternalOrDestructive
	}
}

func isNetworkCommand(cmd string, args []string) bool {
	switch cmd {
	case "curl", "wget", "ssh", "scp", "sftp", "ftp", "rsync", "gh":
		return true
	case "npm", "yarn", "pnpm", "bun":
		action := firstArg(args)
		return action == "install" || action == "add" || action == "update" || action == "upgrade"
	case "pip", "pip3":
		return firstArg(args) == "install"
	case "python", "python3":
		return len(args) >= 3 && args[0] == "-m" && strings.HasPrefix(args[1], "pip") && args[2] == "install"
	case "go":
		if len(args) >= 2 && args[0] == "mod" {
			return args[1] == "tidy" || args[1] == "download"
		}
		return firstArg(args) == "get" || firstArg(args) == "install"
	case "cargo":
		return firstArg(args) == "install"
	case "dotnet":
		return firstArg(args) == "restore" || (len(args) >= 2 && args[0] == "add" && args[1] == "package")
	case "brew", "apt", "apt-get", "dnf", "yum", "pacman", "choco", "winget":
		return true
	default:
		return false
	}
}

func isWorkspaceWriteCommand(cmd string, args []string) bool {
	if containsRedirection(args) {
		return true
	}
	switch cmd {
	case "touch", "mkdir", "cp", "copy", "mv", "move", "gofmt", "prettier", "ruff", "black":
		return true
	case "sed":
		return hasArg(args, "-i") || hasArg(args, "-i.bak")
	default:
		return false
	}
}

func containsRedirection(args []string) bool {
	for _, arg := range args {
		if strings.Contains(arg, ">") {
			return true
		}
	}
	return false
}

func isBuildTestCommand(cmd string, args []string) bool {
	switch cmd {
	case "make", "cmake", "pytest", "cargo", "dotnet", "mvn", "gradle":
		return true
	case "go":
		switch firstArg(args) {
		case "test", "vet", "build", "run", "list":
			return true
		}
	case "npm", "yarn", "pnpm", "bun":
		action := firstArg(args)
		return action == "test" || action == "run" || action == "build"
	case "python", "python3":
		return len(args) >= 2 && args[0] == "-m" && args[1] == "pytest"
	}
	return false
}

func isSearchInspectCommand(cmd string, args []string) bool {
	switch cmd {
	case "rg", "grep", "find", "fd", "sed", "awk", "head", "tail", "wc", "which", "where", "whereis":
		return true
	case "ls":
		return hasArg(args, "-la") || len(args) == 0
	default:
		return false
	}
}

func isReadOnlyCommand(cmd string, args []string) bool {
	switch cmd {
	case "ls", "dir", "pwd", "cd", "cat", "type", "less", "more", "date", "uname", "whoami", "id", "ps", "lsof":
		return true
	default:
		return false
	}
}

func higherRisk(left, right Category) Category {
	if riskRank(right) > riskRank(left) {
		return right
	}
	return left
}

func riskRank(category Category) int {
	switch category {
	case CategoryReadOnly:
		return 1
	case CategorySearchInspect:
		return 2
	case CategoryBuildTest:
		return 3
	case CategoryWorkspaceWrite:
		return 4
	case CategoryVCSLocal:
		return 5
	case CategoryNetworkDependency:
		return 6
	case CategoryExternalOrDestructive:
		return 7
	case CategorySystemPrivileged:
		return 8
	default:
		return 7
	}
}
