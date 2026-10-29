package ai

import (
	"os"
	"path/filepath"
	"strings"
)

// FrameworkHint describes a detected technology in a service's source tree.
type FrameworkHint struct {
	Language  string   // e.g. "dotnet", "node", "python", "go", "java"
	Framework string   // e.g. "aspnetcore", "express", "django", "gin", "spring"
	Files     []string // the files that triggered detection
}

// DetectFrameworks inspects the directory at serviceDir for well-known
// marker files and returns a list of detected technology hints. Returns an
// empty slice if the directory cannot be read or no markers are found.
func DetectFrameworks(serviceDir string) []FrameworkHint {
	if serviceDir == "" {
		return nil
	}

	entries, err := os.ReadDir(serviceDir)
	if err != nil {
		return nil
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() {
			files = append(files, e.Name())
		}
	}

	var hints []FrameworkHint

	// .NET
	csprojFiles := matchedFiles(files, func(f string) bool { return strings.HasSuffix(f, ".csproj") || strings.HasSuffix(f, ".fsproj") })
	if len(csprojFiles) > 0 {
		lang := "dotnet"
		framework := "aspnetcore"
		// Check for EF Core in any matched *.csproj/*.fsproj file's content.
		for _, f := range csprojFiles {
			if hasContentMatch(serviceDir, files, f, "EntityFramework") {
				framework = "aspnetcore-efcore"
				break
			}
		}
		hints = append(hints, FrameworkHint{Language: lang, Framework: framework, Files: csprojFiles})
	}

	// Node.js
	if contains(files, "package.json") {
		framework := detectNodeFramework(serviceDir)
		hints = append(hints, FrameworkHint{Language: "node", Framework: framework, Files: []string{"package.json"}})
	}

	// Python
	if contains(files, "requirements.txt") || contains(files, "pyproject.toml") || contains(files, "setup.py") {
		framework := detectPythonFramework(serviceDir, files)
		matched := matchedFiles(files, func(f string) bool {
			return f == "requirements.txt" || f == "pyproject.toml" || f == "setup.py"
		})
		hints = append(hints, FrameworkHint{Language: "python", Framework: framework, Files: matched})
	}

	// Go
	if contains(files, "go.mod") {
		framework := detectGoFramework(serviceDir)
		hints = append(hints, FrameworkHint{Language: "go", Framework: framework, Files: []string{"go.mod"}})
	}

	// Java / Kotlin
	if contains(files, "pom.xml") || contains(files, "build.gradle") || contains(files, "build.gradle.kts") {
		framework := detectJvmFramework(serviceDir, files)
		matched := matchedFiles(files, func(f string) bool {
			return f == "pom.xml" || f == "build.gradle" || f == "build.gradle.kts"
		})
		hints = append(hints, FrameworkHint{Language: "java", Framework: framework, Files: matched})
	}

	return hints
}

// SummariseHints returns a short human-readable string for use in LLM prompts.
func SummariseHints(hints []FrameworkHint) string {
	if len(hints) == 0 {
		return "unknown stack"
	}
	var parts []string
	for _, h := range hints {
		if h.Framework != "" {
			parts = append(parts, h.Language+"/"+h.Framework)
		} else {
			parts = append(parts, h.Language)
		}
	}
	return strings.Join(parts, ", ")
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func detectNodeFramework(dir string) string {
	if hasContentMatch(dir, []string{"package.json"}, "package.json", "\"express\"") {
		return "express"
	}
	if hasContentMatch(dir, []string{"package.json"}, "package.json", "\"fastify\"") {
		return "fastify"
	}
	if hasContentMatch(dir, []string{"package.json"}, "package.json", "\"next\"") {
		return "nextjs"
	}
	if hasContentMatch(dir, []string{"package.json"}, "package.json", "\"@nestjs/core\"") {
		return "nestjs"
	}
	return "node"
}

func detectPythonFramework(dir string, files []string) string {
	for _, f := range []string{"requirements.txt", "pyproject.toml"} {
		if !contains(files, f) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			continue
		}
		content := strings.ToLower(string(data))
		switch {
		case strings.Contains(content, "django"):
			return "django"
		case strings.Contains(content, "fastapi"):
			return "fastapi"
		case strings.Contains(content, "flask"):
			return "flask"
		case strings.Contains(content, "sqlalchemy"):
			return "sqlalchemy"
		}
	}
	return "python"
}

func detectGoFramework(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return "go"
	}
	content := string(data)
	switch {
	case strings.Contains(content, "github.com/gin-gonic/gin"):
		return "gin"
	case strings.Contains(content, "github.com/labstack/echo"):
		return "echo"
	case strings.Contains(content, "github.com/gofiber/fiber"):
		return "fiber"
	case strings.Contains(content, "gorm.io/gorm"):
		return "gorm"
	}
	return "go"
}

func detectJvmFramework(dir string, files []string) string {
	for _, f := range files {
		if f != "pom.xml" && f != "build.gradle" && f != "build.gradle.kts" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			continue
		}
		content := strings.ToLower(string(data))
		switch {
		case strings.Contains(content, "spring-boot"):
			return "spring-boot"
		case strings.Contains(content, "quarkus"):
			return "quarkus"
		case strings.Contains(content, "micronaut"):
			return "micronaut"
		}
	}
	return "java"
}

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

func matchedFiles(slice []string, fn func(string) bool) []string {
	var out []string
	for _, v := range slice {
		if fn(v) {
			out = append(out, v)
		}
	}
	return out
}

func hasContentMatch(dir string, files []string, name, needle string) bool {
	if !contains(files, name) {
		return false
	}
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return false
	}
	return strings.Contains(string(data), needle)
}
