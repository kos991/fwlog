package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestSystemdServiceAllowsRuntimeDataDirectory(t *testing.T) {
	content, err := os.ReadFile("nat-query-service.service")
	if err != nil {
		t.Fatalf("read service file: %v", err)
	}

	match := regexp.MustCompile(`(?m)^ReadWritePaths=(.+)$`).FindStringSubmatch(string(content))
	if match == nil {
		t.Fatal("ReadWritePaths is required when ProtectSystem is enabled")
	}

	paths := strings.Fields(match[1])
	if !contains(paths, "/data") {
		t.Fatalf("ReadWritePaths must include /data so the service can create /data/index and /data/export; got %q", match[1])
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestCIWorkflowMatchesClickHouseBuildFlow(t *testing.T) {
	assertWorkflowMatchesBuildFlow(
		t,
		".github/workflows/ci.yml",
		[]string{
			"node-version: 20",
			"working-directory: web",
			"npm ci",
			"npm run build",
			"go test ./...",
			"go build -trimpath -ldflags \"-s -w\" -o dist/nat-query-service-linux-amd64 .",
			"name: linux-amd64",
		},
	)
}

func TestReleaseWorkflowMatchesClickHouseBuildFlow(t *testing.T) {
	assertWorkflowMatchesBuildFlow(
		t,
		".github/workflows/release-build.yml",
		[]string{
			"node-version: 20",
			"working-directory: web",
			"npm ci",
			"npm run build",
			"go build -trimpath -ldflags \"-s -w\" -o \"release/$asset\" .",
			"GOOS: linux",
			"GOARCH: amd64",
			"cp nat-query-service.service release/",
			"cp scripts/deploy-142-from-release.sh release/",
		},
	)
}

func assertWorkflowMatchesBuildFlow(t *testing.T, path string, required []string) {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read workflow %s: %v", path, err)
	}

	text := string(content)
	lowerText := strings.ToLower(text)
	for _, forbidden := range forbiddenWorkflowTerms() {
		if strings.Contains(lowerText, strings.ToLower(forbidden)) {
			t.Fatalf("%s must not contain %q after ClickHouse replacement", path, forbidden)
		}
	}

	for _, want := range required {
		if !strings.Contains(text, want) {
			t.Fatalf("%s missing %q", path, want)
		}
	}

	assertComesBefore(t, path, text, "npm run build", "go test ./...")
	assertComesBefore(t, path, text, "npm run build", "go build")

	explicitFileBuild := strings.Join([]string{"main.go", "ip_engine.go"}, " ")
	if strings.Contains(text, explicitFileBuild) {
		t.Fatalf("%s must use package-level go build instead of explicit files", path)
	}
}

func assertComesBefore(t *testing.T, path, text, first, second string) {
	t.Helper()

	firstIndex := strings.Index(text, first)
	if firstIndex == -1 {
		t.Fatalf("%s missing %q", path, first)
	}

	secondIndex := strings.Index(text, second)
	if secondIndex == -1 {
		t.Fatalf("%s missing %q", path, second)
	}

	if firstIndex > secondIndex {
		t.Fatalf("%s requires %q before %q", path, first, second)
	}
}

func forbiddenWorkflowTerms() []string {
	return []string{
		strings.Join([]string{"duck", "db"}, ""),
		strings.Join([]string{"lib", "duck", "db"}, ""),
		strings.Join([]string{"duck", "db_use_", "lib"}, ""),
		strings.Join([]string{"go-", "duck", "db"}, ""),
	}
}
