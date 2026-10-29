package ai

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestDetectFrameworks_EmptyOrUnreadableDirReturnsNil(t *testing.T) {
	if got := DetectFrameworks(""); got != nil {
		t.Errorf("DetectFrameworks(\"\") = %+v, want nil", got)
	}
	if got := DetectFrameworks("/definitely/does/not/exist"); got != nil {
		t.Errorf("DetectFrameworks(missing dir) = %+v, want nil", got)
	}
}

func TestDetectFrameworks_NoMarkersReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "README.md", "hello")
	if got := DetectFrameworks(dir); len(got) != 0 {
		t.Errorf("expected no hints, got %+v", got)
	}
}

func TestDetectFrameworks_DotnetAspnetcore(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "app.csproj", "<Project></Project>")
	hints := DetectFrameworks(dir)
	if len(hints) != 1 || hints[0].Language != "dotnet" || hints[0].Framework != "aspnetcore" {
		t.Errorf("hints = %+v, want dotnet/aspnetcore", hints)
	}
}

func TestDetectFrameworks_DotnetEFCore(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "app.csproj", `<PackageReference Include="EntityFramework" Version="6.0" />`)
	hints := DetectFrameworks(dir)
	if len(hints) != 1 || hints[0].Framework != "aspnetcore-efcore" {
		t.Errorf("hints = %+v, want aspnetcore-efcore", hints)
	}
}

func TestDetectFrameworks_NodeExpress(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"dependencies":{"express":"^4.0.0"}}`)
	hints := DetectFrameworks(dir)
	if len(hints) != 1 || hints[0].Language != "node" || hints[0].Framework != "express" {
		t.Errorf("hints = %+v, want node/express", hints)
	}
}

func TestDetectFrameworks_NodeFrameworkVariants(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{"fastify", `{"dependencies":{"fastify":"^4.0.0"}}`, "fastify"},
		{"nextjs", `{"dependencies":{"next":"^14.0.0"}}`, "nextjs"},
		{"nestjs", `{"dependencies":{"@nestjs/core":"^10.0.0"}}`, "nestjs"},
		{"plain node", `{"dependencies":{"lodash":"^4.0.0"}}`, "node"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "package.json", tc.content)
			hints := DetectFrameworks(dir)
			if len(hints) != 1 || hints[0].Framework != tc.want {
				t.Errorf("hints = %+v, want framework %q", hints, tc.want)
			}
		})
	}
}

func TestDetectFrameworks_PythonFrameworkVariants(t *testing.T) {
	cases := []struct {
		name    string
		file    string
		content string
		want    string
	}{
		{"django", "requirements.txt", "Django==4.2", "django"},
		{"fastapi", "requirements.txt", "fastapi==0.1", "fastapi"},
		{"flask", "requirements.txt", "Flask==2.0", "flask"},
		{"sqlalchemy", "requirements.txt", "SQLAlchemy==2.0", "sqlalchemy"},
		{"plain python via pyproject", "pyproject.toml", "[tool.poetry]\nname = \"x\"", "python"},
		{"plain python via setup.py", "setup.py", "from setuptools import setup", "python"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, tc.file, tc.content)
			hints := DetectFrameworks(dir)
			if len(hints) != 1 || hints[0].Language != "python" || hints[0].Framework != tc.want {
				t.Errorf("hints = %+v, want python/%q", hints, tc.want)
			}
		})
	}
}

func TestDetectFrameworks_GoFrameworkVariants(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{"gin", "module x\nrequire github.com/gin-gonic/gin v1.9.0", "gin"},
		{"echo", "module x\nrequire github.com/labstack/echo v4.0.0", "echo"},
		{"fiber", "module x\nrequire github.com/gofiber/fiber v2.0.0", "fiber"},
		{"gorm", "module x\nrequire gorm.io/gorm v1.0.0", "gorm"},
		{"plain go", "module x\ngo 1.21", "go"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "go.mod", tc.content)
			hints := DetectFrameworks(dir)
			if len(hints) != 1 || hints[0].Language != "go" || hints[0].Framework != tc.want {
				t.Errorf("hints = %+v, want go/%q", hints, tc.want)
			}
		})
	}
}

func TestDetectFrameworks_JvmFrameworkVariants(t *testing.T) {
	cases := []struct {
		name    string
		file    string
		content string
		want    string
	}{
		{"spring-boot via pom", "pom.xml", "<artifactId>spring-boot-starter</artifactId>", "spring-boot"},
		{"quarkus via gradle", "build.gradle", "implementation 'io.quarkus:quarkus-core'", "quarkus"},
		{"micronaut via gradle.kts", "build.gradle.kts", "implementation(\"io.micronaut:micronaut-core\")", "micronaut"},
		{"plain java", "pom.xml", "<project></project>", "java"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, tc.file, tc.content)
			hints := DetectFrameworks(dir)
			if len(hints) != 1 || hints[0].Language != "java" || hints[0].Framework != tc.want {
				t.Errorf("hints = %+v, want java/%q", hints, tc.want)
			}
		})
	}
}

func TestDetectFrameworks_MultipleMarkersInOneDir(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module x\ngo 1.21")
	writeFile(t, dir, "package.json", "{}")
	hints := DetectFrameworks(dir)
	if len(hints) != 2 {
		t.Fatalf("expected 2 hints (go + node), got %+v", hints)
	}
}

func TestSummariseHints_EmptyReturnsUnknownStack(t *testing.T) {
	if got := SummariseHints(nil); got != "unknown stack" {
		t.Errorf("SummariseHints(nil) = %q, want %q", got, "unknown stack")
	}
}

func TestSummariseHints_JoinsLanguageAndFramework(t *testing.T) {
	hints := []FrameworkHint{
		{Language: "go", Framework: "gin"},
		{Language: "node", Framework: ""},
	}
	got := SummariseHints(hints)
	want := "go/gin, node"
	if got != want {
		t.Errorf("SummariseHints() = %q, want %q", got, want)
	}
}
