package approval

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
)

const (
	workspaceWriteApprovalKey = "workspace_write"
	externalWriteApprovalKey  = "external_write"
)

type WriteImpact struct {
	Writes   bool
	External bool
	Targets  []string
}

func WorkspaceWriteApprovalKey() string {
	return workspaceWriteApprovalKey
}

func ExternalWriteApprovalKey() string {
	return externalWriteApprovalKey
}

func AnalyzeWrite(tool string, args json.RawMessage, workspace string, category Category) WriteImpact {
	switch tool {
	case "write_file", "replace_in_file":
		return analyzePathArg(args, workspace)
	case "apply_patch":
		return analyzePatchTargets(args, workspace)
	case "run_command":
		return analyzeCommandWrites(CommandFromArgs(args), workspace, category)
	case "remember", "forget":
		return externalUnknownWrite()
	default:
		if RequiresApproval(category) {
			return workspaceUnknownWrite(workspace)
		}
		return WriteImpact{}
	}
}

func analyzePathArg(args json.RawMessage, workspace string) WriteImpact {
	var params struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return workspaceUnknownWrite(workspace)
	}
	return impactForPath(workspace, params.Path)
}

func analyzePatchTargets(args json.RawMessage, workspace string) WriteImpact {
	var params struct {
		Patch string `json:"patch"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return workspaceUnknownWrite(workspace)
	}
	impact := WriteImpact{}
	for _, line := range strings.Split(params.Patch, "\n") {
		if !strings.HasPrefix(line, "--- ") && !strings.HasPrefix(line, "+++ ") {
			continue
		}
		fields := strings.Fields(strings.TrimPrefix(strings.TrimPrefix(line, "--- "), "+++ "))
		if len(fields) == 0 || fields[0] == "/dev/null" {
			continue
		}
		path := strings.TrimPrefix(strings.TrimPrefix(fields[0], "a/"), "b/")
		impact = mergeImpact(impact, impactForPath(workspace, path))
	}
	if !impact.Writes {
		return workspaceUnknownWrite(workspace)
	}
	return normalizeImpact(impact)
}

func analyzeCommandWrites(command string, workspace string, category Category) WriteImpact {
	if category == CategoryReadOnly || category == CategorySearchInspect || category == CategoryBuildTest {
		return WriteImpact{}
	}

	impact := WriteImpact{}
	for _, segment := range splitCommandSegments(command) {
		fields := significantFields(segment)
		if len(fields) == 0 {
			impact = mergeImpact(impact, workspaceUnknownWrite(workspace))
			continue
		}
		cmd := executableName(fields[0])
		args := fields[1:]
		impact = mergeImpact(impact, redirectionImpact(args, workspace))
		impact = mergeImpact(impact, commandSpecificWriteImpact(cmd, args, workspace))
	}
	if !impact.Writes && (category == CategoryExternalOrDestructive || category == CategorySystemPrivileged) {
		return externalUnknownWrite()
	}
	return normalizeImpact(impact)
}

func commandSpecificWriteImpact(cmd string, args []string, workspace string) WriteImpact {
	switch cmd {
	case "sudo", "doas", "pkexec", "runas", "su", "systemctl", "launchctl", "service", "sc", "reg", "netsh", "diskpart":
		return externalUnknownWrite()
	case "touch", "mkdir":
		return impactForPaths(workspace, nonOptionArgs(args))
	case "cp", "copy", "mv", "move":
		targets := nonOptionArgs(args)
		if len(targets) == 0 {
			return workspaceUnknownWrite(workspace)
		}
		return impactForPath(workspace, targets[len(targets)-1])
	case "gofmt", "prettier", "ruff", "black":
		targets := nonOptionArgs(args)
		if len(targets) == 0 {
			return workspaceUnknownWrite(workspace)
		}
		return impactForPaths(workspace, targets)
	case "sed":
		if hasArg(args, "-i") || hasArg(args, "-i.bak") {
			return impactForPaths(workspace, nonOptionArgs(args))
		}
	case "rm", "rmdir", "rd", "del", "erase":
		targets := nonOptionArgs(args)
		if len(targets) == 0 {
			return workspaceUnknownWrite(workspace)
		}
		return impactForPaths(workspace, targets)
	case "git":
		action := firstArg(args)
		switch action {
		case "add", "commit", "restore", "checkout", "switch", "merge", "rebase", "stash", "tag", "reset", "clean":
			return workspaceUnknownWrite(workspace)
		}
	case "npm", "yarn", "pnpm", "bun":
		action := firstArg(args)
		if action == "install" || action == "add" || action == "update" || action == "upgrade" {
			return workspaceUnknownWrite(workspace)
		}
	case "go":
		if len(args) >= 2 && args[0] == "mod" && (args[1] == "tidy" || args[1] == "download") {
			return workspaceUnknownWrite(workspace)
		}
	case "pip", "pip3", "cargo", "dotnet", "brew", "apt", "apt-get", "dnf", "yum", "pacman", "choco", "winget":
		return externalUnknownWrite()
	}
	return WriteImpact{}
}

func redirectionImpact(args []string, workspace string) WriteImpact {
	impact := WriteImpact{}
	for i, arg := range args {
		clean := strings.Trim(arg, `"'`)
		switch clean {
		case ">", ">>", "1>", "1>>", "2>", "2>>", "&>", "&>>":
			if i+1 < len(args) {
				impact = mergeImpact(impact, impactForPath(workspace, args[i+1]))
			}
			continue
		}
		if target := inlineRedirectionTarget(clean); target != "" {
			impact = mergeImpact(impact, impactForPath(workspace, target))
		}
	}
	return impact
}

func inlineRedirectionTarget(arg string) string {
	for _, marker := range []string{"&>>", "&>", "2>>", "2>", "1>>", "1>", ">>", ">"} {
		if strings.HasPrefix(arg, marker) && len(arg) > len(marker) {
			return arg[len(marker):]
		}
	}
	return ""
}

func nonOptionArgs(args []string) []string {
	out := make([]string, 0, len(args))
	for _, arg := range args {
		clean := strings.Trim(arg, `"'`)
		if clean == "" || strings.HasPrefix(clean, "-") || strings.HasPrefix(clean, "/") && len(clean) == 2 {
			continue
		}
		if strings.Contains(clean, ">") || clean == "|" {
			continue
		}
		out = append(out, clean)
	}
	return out
}

func impactForPaths(workspace string, paths []string) WriteImpact {
	impact := WriteImpact{}
	for _, path := range paths {
		impact = mergeImpact(impact, impactForPath(workspace, path))
	}
	if !impact.Writes {
		return workspaceUnknownWrite(workspace)
	}
	return normalizeImpact(impact)
}

func impactForPath(workspace string, path string) WriteImpact {
	path = strings.TrimSpace(strings.Trim(path, `"'`))
	if path == "" {
		return workspaceUnknownWrite(workspace)
	}
	base := absWorkspace(workspace)
	target := path
	if !filepath.IsAbs(target) {
		target = filepath.Join(base, target)
	}
	target = filepath.Clean(target)
	if isInsideWorkspace(base, target) {
		return WriteImpact{Writes: true, Targets: []string{target}}
	}
	return WriteImpact{Writes: true, External: true, Targets: []string{target}}
}

func workspaceUnknownWrite(workspace string) WriteImpact {
	return WriteImpact{Writes: true, Targets: []string{filepath.Join(absWorkspace(workspace), ".")}}
}

func externalUnknownWrite() WriteImpact {
	return WriteImpact{Writes: true, External: true, Targets: []string{"__external__"}}
}

func absWorkspace(workspace string) string {
	if strings.TrimSpace(workspace) == "" {
		return "."
	}
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return filepath.Clean(workspace)
	}
	return abs
}

func isInsideWorkspace(workspace string, target string) bool {
	rel, err := filepath.Rel(workspace, target)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func mergeImpact(left, right WriteImpact) WriteImpact {
	if !right.Writes {
		return left
	}
	left.Writes = true
	left.External = left.External || right.External
	left.Targets = append(left.Targets, right.Targets...)
	return normalizeImpact(left)
}

func normalizeImpact(impact WriteImpact) WriteImpact {
	if !impact.Writes {
		return impact
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(impact.Targets))
	for _, target := range impact.Targets {
		if target == "" {
			continue
		}
		if _, ok := seen[target]; ok {
			continue
		}
		seen[target] = struct{}{}
		out = append(out, target)
	}
	sort.Strings(out)
	impact.Targets = out
	return impact
}
