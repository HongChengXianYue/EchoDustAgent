package approval

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
)

type Category string

const (
	CategoryReadOnly              Category = "read_only"
	CategorySearchInspect         Category = "search_inspect"
	CategoryBuildTest             Category = "build_test"
	CategoryWorkspaceWrite        Category = "workspace_write"
	CategoryVCSLocal              Category = "vcs_local"
	CategoryNetworkDependency     Category = "network_dependency"
	CategoryExternalOrDestructive Category = "external_or_destructive"
	CategorySystemPrivileged      Category = "system_privileged"
)

type Decision string

const (
	DecisionAllow  Decision = "allow"
	DecisionAlways Decision = "always"
	DecisionDeny   Decision = "deny"
)

type Request struct {
	Tool     string
	Category Category
	Args     json.RawMessage
	Reason   string
}

type Approver interface {
	Approve(ctx context.Context, request Request) Decision
}

func RequiresApproval(category Category) bool {
	switch category {
	case CategoryReadOnly, CategorySearchInspect, CategoryBuildTest:
		return false
	default:
		return true
	}
}

func CacheKey(request Request) string {
	args := normalizedArgs(request.Tool, request.Args)
	return request.Tool + "\x00" + string(request.Category) + "\x00" + args
}

func normalizedArgs(tool string, args json.RawMessage) string {
	if tool == "run_command" {
		return strings.TrimSpace(CommandFromArgs(args))
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, args); err == nil {
		return compact.String()
	}
	return strings.TrimSpace(string(args))
}
