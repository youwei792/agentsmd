package analyze

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// packageJSON is the subset of package.json agentsmd cares about.
type packageJSON struct {
	Name           string            `json:"name"`
	Scripts        map[string]string `json:"scripts"`
	Deps           map[string]string `json:"dependencies"`
	DevDeps        map[string]string `json:"devDependencies"`
	Engines        map[string]string `json:"engines"`
	PackageManager string            `json:"packageManager"`
	Workspaces     json.RawMessage   `json:"workspaces"`
}

func readPackageJSON(path string) (*packageJSON, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var pj packageJSON
	if err := json.Unmarshal(b, &pj); err != nil {
		return nil, err
	}
	return &pj, nil
}

// classifyScript guesses the purpose of a named script.
func classifyScript(name string) string {
	n := strings.ToLower(name)
	switch {
	case n == "test" || strings.HasPrefix(n, "test") || strings.HasSuffix(n, ":test") || strings.HasSuffix(n, "-test"):
		return "test"
	case n == "lint" || strings.HasPrefix(n, "lint"):
		return "lint"
	case n == "format" || n == "fmt" || strings.HasPrefix(n, "format") || strings.HasPrefix(n, "fmt"):
		return "format"
	case n == "build" || strings.HasPrefix(n, "build") || n == "compile" || strings.HasPrefix(n, "compile"):
		return "build"
	case n == "dev" || n == "serve" || n == "start" || strings.HasPrefix(n, "dev") || strings.HasPrefix(n, "serve"):
		return "dev"
	case n == "typecheck" || n == "type-check" || n == "check" || n == "tsc" || strings.HasPrefix(n, "typecheck"):
		return "check"
	default:
		return "other"
	}
}

func analyzeNode(f *Facts) {
	pjPath := filepath.Join(f.Root, "package.json")
	pj, err := readPackageJSON(pjPath)
	if err != nil {
		// Sub-packages may still make this a node repo (monorepos).
		nested := findFiles(f.Root, "package.json")
		if len(nested) > 0 {
			f.Languages = addUnique(f.Languages, "TypeScript/JavaScript")
			f.Warnings = append(f.Warnings, "no root package.json, but nested package.json files found (monorepo?)")
		}
		return
	}

	f.Languages = addUnique(f.Languages, "TypeScript/JavaScript")
	f.Manifests = append(f.Manifests, "package.json")

	// Package manager: packageManager field first, then lockfiles.
	pm := ""
	if pj.PackageManager != "" {
		pm = strings.SplitN(pj.PackageManager, "@", 2)[0]
	}
	switch pm {
	case "npm", "pnpm", "yarn", "bun":
		f.PackageMgrs = addUnique(f.PackageMgrs, pm)
	}
	for _, pair := range [][2]string{
		{"pnpm-lock.yaml", "pnpm"},
		{"yarn.lock", "yarn"},
		{"bun.lockb", "bun"},
		{"bun.lock", "bun"},
		{"package-lock.json", "npm"},
	} {
		if exists(filepath.Join(f.Root, pair[0])) {
			f.PackageMgrs = addUnique(f.PackageMgrs, pair[1])
		}
	}

	// Engines / runtime hints.
	if v, ok := pj.Engines["node"]; ok && v != "" {
		f.ConfigFiles = append(f.ConfigFiles, ToolchainFile{Path: "package.json", Detail: "requires node " + v})
	}

	// Workspaces (array form or object form).
	if len(pj.Workspaces) > 0 {
		m := &MonorepoInfo{Kind: "npm-workspaces", Manifest: "package.json"}
		var ws []string
		if json.Unmarshal(pj.Workspaces, &ws) == nil {
			m.Packages = ws
		} else {
			var obj struct {
				Packages []string `json:"packages"`
			}
			if json.Unmarshal(pj.Workspaces, &obj) == nil {
				m.Packages = obj.Packages
			}
		}
		f.Monorepo = m
	}
	if exists(filepath.Join(f.Root, "pnpm-workspace.yaml")) {
		f.Monorepo = &MonorepoInfo{Kind: "pnpm-workspace", Manifest: "pnpm-workspace.yaml"}
	}

	// Frameworks from dependencies.
	deps := mergeMaps(pj.Deps, pj.DevDeps)
	detectNodeFrameworks(f, deps)

	// Tools from dependencies that configure-less detection can miss.
	if _, ok := deps["eslint"]; ok {
		f.Linters = addUnique(f.Linters, "eslint")
	}
	if _, ok := deps["prettier"]; ok {
		f.Formatters = addUnique(f.Formatters, "prettier")
	}
	if _, ok := deps["@biomejs/biome"]; ok {
		f.Linters = addUnique(f.Linters, "biome")
	}

	// Scripts — the cmdline uses the repo's own package manager so that
	// generated instructions never contradict the lockfile (PM-MISMATCH).
	runner := f.PackageManager()
	if runner == "" {
		runner = "npm"
	}
	// Idiomatic invocation per runner; all validate against package.json.
	invocation := map[string]string{
		"npm": "npm run %s", "pnpm": "pnpm %s", "yarn": "yarn %s", "bun": "bun run %s",
	}
	tpl := invocation[runner]
	if tpl == "" {
		tpl = "npm run %s"
	}
	for name := range pj.Scripts {
		f.Scripts = append(f.Scripts, Command{
			Name: name, Cmdline: fmt.Sprintf(tpl, name), Source: "package.json",
			Purpose: classifyScript(name), IsScript: true,
		})
	}
}

func detectNodeFrameworks(f *Facts, deps map[string]string) {
	known := map[string]string{
		"next": "Next.js", "nuxt": "Nuxt", "@nuxt/framework": "Nuxt",
		"astro": "Astro", "gatsby": "Gatsby", "remix": "Remix",
		"@angular/core": "Angular", "vue": "Vue", "react": "React",
		"svelte": "Svelte", "@sveltejs/kit": "SvelteKit",
		"express": "Express", "fastify": "Fastify", "@nestjs/core": "NestJS",
		"hono": "Hono", "koa": "Koa",
		"vite": "Vite", "webpack": "webpack", "esbuild": "esbuild",
		"typescript": "TypeScript", "tailwindcss": "Tailwind CSS",
		"electron": "Electron", "tauri": "Tauri (node side)",
		"jest": "Jest", "vitest": "Vitest", "mocha": "Mocha",
		"@playwright/test": "Playwright", "cypress": "Cypress",
		"eslint": "ESLint", "prettier": "Prettier", "biome": "Biome",
		"prisma": "Prisma", "drizzle-orm": "Drizzle", "typeorm": "TypeORM",
		"zod": "Zod", "electron-builder": "electron-builder",
	}
	for dep, label := range known {
		if v, ok := deps[dep]; ok {
			if label == "Jest" || label == "Vitest" || label == "Mocha" || label == "Playwright" || label == "Cypress" {
				f.TestFrameworks = addUnique(f.TestFrameworks, label)
			}
			f.Frameworks = append(f.Frameworks, Framework{Name: label, Version: v, Source: "package.json"})
		}
	}
}

func mergeMaps(maps ...map[string]string) map[string]string {
	out := map[string]string{}
	for _, m := range maps {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}
