package analyze

import (
	"path/filepath"
	"regexp"
	"strings"
)

var goModRequireRe = regexp.MustCompile(`(?m)^\s*(?:require\s+)?([\w./-]+\.[\w./-]+/[\w./-]+)\s+v`)

func analyzeGo(f *Facts) {
	gomod := readFile(filepath.Join(f.Root, "go.mod"))
	if gomod == "" {
		return
	}
	f.Languages = addUnique(f.Languages, "Go")
	f.Manifests = append(f.Manifests, "go.mod")

	for _, line := range strings.Split(gomod, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "go ") {
			f.ConfigFiles = append(f.ConfigFiles, ToolchainFile{Path: "go.mod", Detail: "go " + strings.TrimSpace(strings.TrimPrefix(t, "go"))})
			break
		}
	}

	known := map[string]string{
		"github.com/gin-gonic/gin":            "Gin",
		"github.com/labstack/echo":            "Echo",
		"github.com/gofiber/fiber":            "Fiber",
		"github.com/go-chi/chi":               "Chi",
		"github.com/gorilla/mux":              "Gorilla Mux",
		"github.com/spf13/cobra":              "Cobra (CLI)",
		"google.golang.org/grpc":              "gRPC",
		"github.com/stretchr/testify":         "testify",
		"github.com/golang/mock":              "gomock",
		"go.uber.org/zap":                     "zap",
		"github.com/rs/zerolog":               "zerolog",
		"github.com/charmbracelet/bubbletea":  "Bubble Tea (TUI)",
		"github.com/pressly/goose":            "goose",
		"github.com/golang-migrate/migrate":   "golang-migrate",
		"github.com/jackc/pgx":                "pgx",
		"github.com/mattn/go-sqlite3":         "go-sqlite3",
		"github.com/testcontainers/testcontainers-go": "Testcontainers",
	}
	for _, m := range goModRequireRe.FindAllStringSubmatch(gomod, -1) {
		if label, ok := known[m[1]]; ok {
			if label == "testify" || label == "gomock" || label == "Testcontainers" {
				f.TestFrameworks = addUnique(f.TestFrameworks, label)
			} else {
				f.Frameworks = append(f.Frameworks, Framework{Name: label, Source: "go.mod"})
			}
		}
	}

	if exists(filepath.Join(f.Root, "go.work")) {
		f.Monorepo = &MonorepoInfo{Kind: "go-workspace", Manifest: "go.work"}
	}

	f.Scripts = append(f.Scripts,
		Command{Name: "build", Cmdline: "go build ./...", Source: "go.mod", Purpose: "build"},
		Command{Name: "test", Cmdline: "go test ./...", Source: "go.mod", Purpose: "test"},
		Command{Name: "vet", Cmdline: "go vet ./...", Source: "go.mod", Purpose: "check"},
	)
}
