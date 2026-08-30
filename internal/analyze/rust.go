package analyze

import (
	"path/filepath"
	"strings"
)

func analyzeRust(f *Facts) {
	cargo := readFile(filepath.Join(f.Root, "Cargo.toml"))
	if cargo == "" {
		return
	}
	f.Languages = addUnique(f.Languages, "Rust")
	f.Manifests = append(f.Manifests, "Cargo.toml")

	sec := parseTOMLish(cargo)
	if name := sec["package"]["name"]; name != "" {
		f.ConfigFiles = append(f.ConfigFiles, ToolchainFile{Path: "Cargo.toml", Detail: "crate " + name})
	}
	if edition := sec["package"]["edition"]; edition != "" {
		f.ConfigFiles = append(f.ConfigFiles, ToolchainFile{Path: "Cargo.toml", Detail: "edition " + edition})
	}
	if len(sec["workspace"]) > 0 || strings.Contains(cargo, "[workspace]") {
		f.Monorepo = &MonorepoInfo{Kind: "cargo-workspace", Manifest: "Cargo.toml"}
	}

	lower := strings.ToLower(cargo)
	known := map[string]string{
		"axum": "axum", "actix-web": "Actix Web", "rocket": "Rocket",
		"warp": "Warp", "poem": "Poem", "salvo": "Salvo",
		"tokio": "tokio", "clap": "clap (CLI)", "tauri": "Tauri",
		"bevy": "Bevy", "ratatui": "ratatui (TUI)", "crossterm": "crossterm",
	}
	for dep, label := range known {
		if strings.Contains(lower, dep) {
			f.Frameworks = append(f.Frameworks, Framework{Name: label, Source: "Cargo.toml"})
		}
	}
	if strings.Contains(lower, "tempfile") || strings.Contains(lower, "assert_cmd") {
		f.TestFrameworks = addUnique(f.TestFrameworks, "cargo test (helpers)")
	}

	f.Scripts = append(f.Scripts,
		Command{Name: "build", Cmdline: "cargo build", Source: "Cargo.toml", Purpose: "build"},
		Command{Name: "test", Cmdline: "cargo test", Source: "Cargo.toml", Purpose: "test"},
		Command{Name: "check", Cmdline: "cargo check", Source: "Cargo.toml", Purpose: "check"},
	)
}
