package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type runResult struct {
	code   int
	stdout string
	stderr string
}

func runWithArgsCaptured(t *testing.T, args ...string) runResult {
	t.Helper()

	oldArgs := os.Args
	oldStdin := os.Stdin
	oldStdout := os.Stdout
	oldStderr := os.Stderr
	defer func() {
		os.Args = oldArgs
		os.Stdin = oldStdin
		os.Stdout = oldStdout
		os.Stderr = oldStderr
	}()

	stdinR, stdinW := pipe(t)
	stdoutR, stdoutW := pipe(t)
	stderrR, stderrW := pipe(t)
	defer stdinR.Close()
	defer stdoutR.Close()
	defer stderrR.Close()

	if err := stdinW.Close(); err != nil {
		t.Fatalf("close stdin writer: %v", err)
	}

	os.Args = append([]string{"dothuntcli"}, args...)
	os.Stdin = stdinR
	os.Stdout = stdoutW
	os.Stderr = stderrW

	code := run()

	if err := stdoutW.Close(); err != nil {
		t.Fatalf("close stdout writer: %v", err)
	}
	if err := stderrW.Close(); err != nil {
		t.Fatalf("close stderr writer: %v", err)
	}

	return runResult{
		code:   code,
		stdout: readAll(t, stdoutR),
		stderr: readAll(t, stderrR),
	}
}

// Keep these exit codes stable: they matter in scripts/agents.
func TestRun_NoArgs_Exit2(t *testing.T) {
	isolatePorkbunCredentialSources(t)

	got := runWithArgsCaptured(t)
	if got.code != 2 {
		t.Fatalf("exit=%d, want 2", got.code)
	}
}

func TestRun_UnknownCommand_Exit2(t *testing.T) {
	got := runWithArgsCaptured(t, "nope")
	if got.code != 2 {
		t.Fatalf("exit=%d, want 2", got.code)
	}
}

func TestRun_InvalidFormatFails(t *testing.T) {
	isolatePorkbunCredentialSources(t)

	got := runWithArgsCaptured(t, "--format", "yaml", "check")
	if got.code != 2 {
		t.Fatalf("exit=%d, want 2", got.code)
	}
	if got.stdout != "" {
		t.Fatalf("stdout=%q, want empty", got.stdout)
	}
	want := `invalid --format "yaml" (use auto|table|ndjson|json|plain)`
	if !strings.Contains(got.stderr, want) {
		t.Fatalf("stderr=%q, want %q", got.stderr, want)
	}
	if !strings.Contains(got.stderr, "Usage:") {
		t.Fatalf("stderr=%q, want usage", got.stderr)
	}
}

func TestRun_HelpGoesToStdout(t *testing.T) {
	got := runWithArgsCaptured(t, "--help")
	if got.code != 0 {
		t.Fatalf("exit=%d, want 0", got.code)
	}
	if got.stderr != "" {
		t.Fatalf("stderr=%q, want empty", got.stderr)
	}
	for _, want := range []string{
		"Usage:",
		"Examples:",
		"dothuntcli check openai.com example.com",
		`printf "openai.com\nexample.com\n" | dothuntcli --ndjson check`,
		"dothuntcli --format json --registrar none check example.com",
	} {
		if !strings.Contains(got.stdout, want) {
			t.Fatalf("stdout=%q, want %q", got.stdout, want)
		}
	}
}

func TestRun_CheckHelpIncludesExamples(t *testing.T) {
	got := runWithArgsCaptured(t, "check", "--help")
	if got.code != 0 {
		t.Fatalf("exit=%d, want 0", got.code)
	}
	if got.stderr != "" {
		t.Fatalf("stderr=%q, want empty", got.stderr)
	}
	for _, want := range []string{
		"dothuntcli check openai.com example.com",
		`printf "openai.com\nexample.com\n" | dothuntcli --ndjson check`,
		"dothuntcli --format json --registrar none check example.com",
	} {
		if !strings.Contains(got.stdout, want) {
			t.Fatalf("stdout=%q, want %q", got.stdout, want)
		}
	}
}

func TestRun_UsageErrorsGoToStderr(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "unknown command", args: []string{"nope"}, want: `unknown command "nope"`},
		{name: "unknown flag", args: []string{"check", "--bogus"}, want: "unknown flag: --bogus"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runWithArgsCaptured(t, tt.args...)
			if got.code != 2 {
				t.Fatalf("exit=%d, want 2", got.code)
			}
			if got.stdout != "" {
				t.Fatalf("stdout=%q, want empty", got.stdout)
			}
			if !strings.Contains(got.stderr, tt.want) {
				t.Fatalf("stderr=%q, want %q", got.stderr, tt.want)
			}
			if !strings.Contains(got.stderr, "Usage:") {
				t.Fatalf("stderr=%q, want usage", got.stderr)
			}
		})
	}
}

func TestRun_CheckMissingDomainsShowsError(t *testing.T) {
	got := runWithArgsCaptured(t, "--registrar", "none", "check")
	if got.code != 2 {
		t.Fatalf("exit=%d, want 2", got.code)
	}
	if got.stdout != "" {
		t.Fatalf("stdout=%q, want empty", got.stdout)
	}
	want := "missing domains; pass domains as args or pipe newline-delimited domains on stdin"
	if !strings.Contains(got.stderr, want) {
		t.Fatalf("stderr=%q, want %q", got.stderr, want)
	}
	if !strings.Contains(got.stderr, "Usage:") {
		t.Fatalf("stderr=%q, want usage", got.stderr)
	}
}

func TestRun_RegistrarNoneDoesNotLoadPorkbunCredentials(t *testing.T) {
	orig := readPorkbunCredentialsFromKeychain
	readPorkbunCredentialsFromKeychain = func() (porkbunCredentials, error) {
		return porkbunCredentials{}, errors.New("should not load Porkbun credentials")
	}
	t.Cleanup(func() {
		readPorkbunCredentialsFromKeychain = orig
	})

	got := runWithArgsCaptured(t, "--registrar", "none", "check")
	if got.code != 2 {
		t.Fatalf("exit=%d, want 2", got.code)
	}
	if strings.Contains(got.stderr, "should not load Porkbun credentials") {
		t.Fatalf("stderr=%q, loaded Porkbun credentials unexpectedly", got.stderr)
	}
}

func TestRun_AutoMissingDefaultCredentialsFileIsNotFatal(t *testing.T) {
	isolatePorkbunCredentialSources(t)

	got := runWithArgsCaptured(t, "check")
	if got.code != 2 {
		t.Fatalf("exit=%d, want 2", got.code)
	}
	if !strings.Contains(got.stderr, "missing domains; pass domains as args or pipe newline-delimited domains on stdin") {
		t.Fatalf("stderr=%q, want missing domains error", got.stderr)
	}
	if strings.Contains(got.stderr, "Porkbun credentials file does not exist") {
		t.Fatalf("stderr=%q, default missing credentials file should not be fatal", got.stderr)
	}
}

func TestRun_RegistrarPorkbunMissingCredentialsFailsClearly(t *testing.T) {
	isolatePorkbunCredentialSources(t)

	got := runWithArgsCaptured(t, "--registrar", "porkbun", "check")
	if got.code != 2 {
		t.Fatalf("exit=%d, want 2", got.code)
	}
	if got.stdout != "" {
		t.Fatalf("stdout=%q, want empty", got.stdout)
	}
	if !strings.Contains(got.stderr, "missing Porkbun API keys") {
		t.Fatalf("stderr=%q, want missing Porkbun API keys", got.stderr)
	}
	if !strings.Contains(got.stderr, "Usage:") {
		t.Fatalf("stderr=%q, want usage", got.stderr)
	}
}

func isolatePorkbunCredentialSources(t *testing.T) {
	t.Helper()

	withoutKeychain(t)
	home := t.TempDir()
	t.Setenv(porkbunAPIKeyEnv, "")
	t.Setenv(porkbunSecretAPIKeyEnv, "")
	t.Setenv(porkbunCredentialsFilePathEnv, "")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
}

func pipe(t *testing.T) (*os.File, *os.File) {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	return r, w
}

func readAll(t *testing.T, r *os.File) string {
	t.Helper()

	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	return string(b)
}
