package approval

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"
)

func TestClassifyBuiltInTools(t *testing.T) {
	tests := []struct {
		name string
		tool string
		args json.RawMessage
		want Category
	}{
		{name: "update todos", tool: "update_todos", want: CategoryReadOnly},
		{name: "read file", tool: "read_file", want: CategoryReadOnly},
		{name: "read file range", tool: "read_file_range", want: CategoryReadOnly},
		{name: "find files", tool: "find_files", want: CategoryReadOnly},
		{name: "find symbol", tool: "find_symbol", want: CategoryReadOnly},
		{name: "find references", tool: "find_references", want: CategoryReadOnly},
		{name: "find callers", tool: "find_callers", want: CategoryReadOnly},
		{name: "find callees", tool: "find_callees", want: CategoryReadOnly},
		{name: "search files", tool: "search_files", want: CategorySearchInspect},
		{name: "git status", tool: "git_status", want: CategoryReadOnly},
		{name: "git diff", tool: "git_diff", want: CategorySearchInspect},
		{name: "git log", tool: "git_log", want: CategoryReadOnly},
		{name: "write file", tool: "write_file", want: CategoryWorkspaceWrite},
		{name: "git commit", tool: "run_command", args: commandArgs("git commit -m test"), want: CategoryVCSLocal},
		{name: "go test", tool: "run_command", args: commandArgs("env GOCACHE=/tmp/cache go test ./..."), want: CategoryBuildTest},
		{name: "curl", tool: "run_command", args: commandArgs("curl https://example.com"), want: CategoryNetworkDependency},
		{name: "rm", tool: "run_command", args: commandArgs("rm -rf build"), want: CategoryExternalOrDestructive},
		{name: "sudo", tool: "run_command", args: commandArgs("sudo apt install curl"), want: CategorySystemPrivileged},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Classify(tt.tool, tt.args); got != tt.want {
				t.Fatalf("Classify() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPermanentBlacklistBlocksDangerousCommands(t *testing.T) {
	for _, command := range []string{
		"sudo rm -rf /",
		"rm -fr /*",
		`bash -lc "rm -r -f /"`,
		`rmdir /s /q C:\`,
		`del /f /s /q C:\*`,
		`powershell Remove-Item -Recurse -Force C:\`,
		"format C:",
		"mkfs.ext4 /dev/sda",
		"diskutil eraseDisk APFS Blank /dev/disk2",
	} {
		t.Run(command, func(t *testing.T) {
			if reason, blocked := BlockReason("run_command", commandArgs(command)); !blocked || reason == "" {
				t.Fatalf("BlockReason(%q) = %q, %v; want blocked", command, reason, blocked)
			}
		})
	}
}

func TestPermanentBlacklistDoesNotBlockOrdinaryCommands(t *testing.T) {
	for _, command := range []string{
		"rm -rf build",
		"git status",
		"go test ./...",
	} {
		t.Run(command, func(t *testing.T) {
			if reason, blocked := BlockReason("run_command", commandArgs(command)); blocked {
				t.Fatalf("BlockReason(%q) blocked ordinary command: %s", command, reason)
			}
		})
	}
}

func TestAnalyzeWriteTargets(t *testing.T) {
	workspace := "/tmp/workspace"
	tests := []struct {
		name     string
		tool     string
		args     json.RawMessage
		category Category
		external bool
		targets  []string
	}{
		{
			name:     "write file inside workspace",
			tool:     "write_file",
			args:     json.RawMessage(`{"path":"notes/a.txt"}`),
			category: CategoryWorkspaceWrite,
			targets:  []string{filepath.Join(workspace, "notes/a.txt")},
		},
		{
			name:     "write file outside workspace",
			tool:     "write_file",
			args:     json.RawMessage(`{"path":"../outside.txt"}`),
			category: CategoryWorkspaceWrite,
			external: true,
			targets:  []string{"/tmp/outside.txt"},
		},
		{
			name:     "relative command redirection inside workspace",
			tool:     "run_command",
			args:     commandArgs("echo hello > report.txt"),
			category: CategoryWorkspaceWrite,
			targets:  []string{filepath.Join(workspace, "report.txt")},
		},
		{
			name:     "absolute command redirection outside workspace",
			tool:     "run_command",
			args:     commandArgs("echo hello > /tmp/report.txt"),
			category: CategoryExternalOrDestructive,
			external: true,
			targets:  []string{"/tmp/report.txt"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AnalyzeWrite(tt.tool, tt.args, workspace, tt.category)
			if !got.Writes || got.External != tt.external || strings.Join(got.Targets, "\n") != strings.Join(tt.targets, "\n") {
				t.Fatalf("AnalyzeWrite() = %#v, want external=%v targets=%v", got, tt.external, tt.targets)
			}
		})
	}
}

func TestMemoryApproverAlwaysIsExactToRequest(t *testing.T) {
	next := &sequenceApprover{decisions: []Decision{DecisionAlways, DecisionDeny}}
	approver := NewMemoryApprover(next)
	request := Request{
		Tool:     "run_command",
		Category: CategoryNetworkDependency,
		Args:     commandArgs("curl https://example.com"),
	}

	if got := approver.Approve(context.Background(), request); got != DecisionAlways {
		t.Fatalf("first decision = %q, want always", got)
	}
	if got := approver.Approve(context.Background(), request); got != DecisionAlways {
		t.Fatalf("remembered decision = %q, want always", got)
	}
	changed := request
	changed.Args = commandArgs("curl https://example.org")
	if got := approver.Approve(context.Background(), changed); got != DecisionDeny {
		t.Fatalf("changed command decision = %q, want deny", got)
	}
	if next.calls != 2 {
		t.Fatalf("underlying approver calls = %d, want 2", next.calls)
	}
}

func TestMemoryApproverCachedDecisionOnlyHitsSessionScope(t *testing.T) {
	next := &sequenceApprover{decisions: []Decision{DecisionAlways}}
	approver := NewMemoryApprover(next)
	request := Request{
		Tool:     "write_file",
		Category: CategoryWorkspaceWrite,
		Args:     []byte(`{"path":"notes.txt","content":"hello"}`),
		Scope:    ScopeSession,
		Key:      WorkspaceWriteApprovalKey(),
	}

	if decision, ok := approver.CachedDecision(request); ok || decision != "" {
		t.Fatalf("unexpected cached decision before approval: %q ok=%v", decision, ok)
	}
	if got := approver.Approve(context.Background(), request); got != DecisionAlways {
		t.Fatalf("first decision = %q, want always", got)
	}
	if decision, ok := approver.CachedDecision(request); !ok || decision != DecisionAlways {
		t.Fatalf("cached decision = %q ok=%v, want always true", decision, ok)
	}
	loopRequest := request
	loopRequest.Scope = ScopeLoop
	if decision, ok := approver.CachedDecision(loopRequest); ok || decision != "" {
		t.Fatalf("loop scope should not reuse session cache: %q ok=%v", decision, ok)
	}
}

func TestChooseApprovalSelection(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  Decision
	}{
		{name: "default enter allows", input: "\n", want: DecisionAllow},
		{name: "down arrow selects always", input: "\x1b[B\n", want: DecisionAlways},
		{name: "right arrow selects always", input: "\x1b[C\n", want: DecisionAlways},
		{name: "j selects always", input: "j\n", want: DecisionAlways},
		{name: "two down arrows select deny", input: "\x1b[B\x1b[B\n", want: DecisionDeny},
		{name: "left wraps to deny", input: "\x1b[D\n", want: DecisionDeny},
		{name: "d denies", input: "d", want: DecisionDeny},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := strings.NewReader(tt.input)
			var output bytes.Buffer
			got := chooseApproval(bufio.NewReader(input), input, &output, Request{})
			if got != tt.want {
				t.Fatalf("chooseApproval() = %q, want %q", got, tt.want)
			}
			if !strings.Contains(output.String(), "Allow") ||
				!strings.Contains(output.String(), "Always allow exact call") ||
				!strings.Contains(output.String(), "Deny") {
				t.Fatalf("output missing approval options:\n%s", output.String())
			}
		})
	}
}

func TestChooseApprovalRejectsEOF(t *testing.T) {
	var output bytes.Buffer
	if got := chooseApproval(bufio.NewReader(eofReader{}), eofReader{}, &output, Request{}); got != DecisionDeny {
		t.Fatalf("chooseApproval() = %q, want deny", got)
	}
}

func TestChooseApprovalCanLimitOptions(t *testing.T) {
	request := Request{
		Scope:   ScopeSession,
		Key:     WorkspaceWriteApprovalKey(),
		Options: []Decision{DecisionAlways, DecisionDeny},
	}

	input := strings.NewReader("\n")
	var output bytes.Buffer
	if got := chooseApproval(bufio.NewReader(input), input, &output, request); got != DecisionAlways {
		t.Fatalf("chooseApproval() = %q, want always", got)
	}
	if !strings.Contains(output.String(), "Always allow workspace writes this session") {
		t.Fatalf("output = %q, want session workspace option", output.String())
	}
	if strings.Contains(output.String(), "Allow once") {
		t.Fatalf("output = %q, did not expect allow option", output.String())
	}
}

func TestTerminalApproverRejectsNonInteractiveInput(t *testing.T) {
	approver := NewTerminalApprover(strings.NewReader("\n"), io.Discard)
	request := Request{Tool: "write_file", Category: CategoryWorkspaceWrite, Args: json.RawMessage(`{}`)}
	if got := approver.Approve(context.Background(), request); got != DecisionDeny {
		t.Fatalf("Approve() = %q, want deny", got)
	}
}

type sequenceApprover struct {
	decisions []Decision
	calls     int
}

type eofReader struct{}

func (eofReader) Read(p []byte) (int, error) {
	return 0, io.EOF
}

func (a *sequenceApprover) Approve(ctx context.Context, request Request) Decision {
	if a.calls >= len(a.decisions) {
		return DecisionDeny
	}
	decision := a.decisions[a.calls]
	a.calls++
	return decision
}

func commandArgs(command string) json.RawMessage {
	data, err := json.Marshal(map[string]string{"command": command})
	if err != nil {
		panic(err)
	}
	return data
}
