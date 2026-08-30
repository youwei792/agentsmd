package analyze

import (
	"path/filepath"
	"strings"
)

func analyzeLinters(f *Facts) {
	// Generic configs.
	genericLint := map[string][]string{
		"eslint":        {"eslint.config.js", "eslint.config.mjs", "eslint.config.cjs", "eslint.config.ts", ".eslintrc", ".eslintrc.js", ".eslintrc.json", ".eslintrc.yml", ".eslintrc.yaml", ".eslintrc.cjs"},
		"prettier":      {".prettierrc", ".prettierrc.json", ".prettierrc.yml", ".prettierrc.yaml", ".prettierrc.toml", ".prettierrc.js", "prettier.config.js", "prettier.config.mjs", "prettier.config.cjs"},
		"biome":         {"biome.json", "biome.jsonc"},
		"stylelint":     {".stylelintrc", ".stylelintrc.json", "stylelint.config.js", "stylelint.config.mjs"},
		"golangci-lint": {".golangci.yml", ".golangci.yaml", ".golangci.toml", ".golangci.json"},
		"ruff":          {"ruff.toml", ".ruff.toml"},
		"mypy":          {"mypy.ini", ".mypy.ini"},
		"flake8":        {".flake8"},
		"rubocop":       {".rubocop.yml"},
		"editorconfig":  {".editorconfig"},
	}
	for tool, names := range genericLint {
		if hits := findFiles(f.Root, names...); len(hits) > 0 {
			if tool == "prettier" || tool == "editorconfig" || strings.HasPrefix(tool, "stylelint") {
				f.Formatters = addUnique(f.Formatters, tool)
			} else {
				f.Linters = addUnique(f.Linters, tool)
			}
			f.ConfigFiles = append(f.ConfigFiles, ToolchainFile{Path: hits[0], Detail: tool})
		}
	}

	// Python tool sections inside pyproject.toml.
	if exists(filepath.Join(f.Root, "pyproject.toml")) {
		sec := parseTOMLish(readFile(filepath.Join(f.Root, "pyproject.toml")))
		for _, tool := range []string{"tool.ruff", "tool.black", "tool.isort", "tool.pylint", "tool.flake8"} {
			if len(sec[tool]) > 0 {
				name := strings.TrimPrefix(tool, "tool.")
				if name == "black" || name == "isort" {
					f.Formatters = addUnique(f.Formatters, name)
				} else {
					f.Linters = addUnique(f.Linters, name)
				}
			}
		}
		if _, ok := sec["tool.pytest.ini_options"]; ok {
			f.TestFrameworks = addUnique(f.TestFrameworks, "pytest")
		}
	}

	// Rust formatting.
	if hits := findFiles(f.Root, "rustfmt.toml", ".rustfmt.toml"); len(hits) > 0 {
		f.Formatters = addUnique(f.Formatters, "rustfmt")
	}
	f.Formatters = addUnique(f.Formatters, "") // no-op keeps slice semantics simple
	f.Formatters = removeEmpty(f.Formatters)

	// Clippy is implied by Cargo.toml.
	if f.Has("Rust") {
		f.Linters = addUnique(f.Linters, "clippy")
	}
	if f.Has("Go") {
		f.Linters = addUnique(f.Linters, "go vet")
	}
}

func removeEmpty(s []string) []string {
	out := s[:0]
	for _, v := range s {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}
