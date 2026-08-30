package analyze

import (
	"path/filepath"
	"strings"
)

func analyzePython(f *Facts) {
	pyproject := parseTOMLish(readFile(filepath.Join(f.Root, "pyproject.toml")))

	// Dependency / manifest detection.
	_, hasProject := pyproject["project"]
	_, hasPoetry := pyproject["tool.poetry"]
	if hasProject || hasPoetry {
		f.Manifests = append(f.Manifests, "pyproject.toml")
	}

	var pm string
	switch {
	case exists(filepath.Join(f.Root, "uv.lock")):
		pm = "uv"
	case exists(filepath.Join(f.Root, "pdm.lock")):
		pm = "pdm"
	case exists(filepath.Join(f.Root, "poetry.lock")):
		pm = "poetry"
	case exists(filepath.Join(f.Root, "Pipfile.lock")):
		pm = "pipenv"
	case exists(filepath.Join(f.Root, "requirements.txt")):
		pm = "pip"
	}
	if pm == "" {
		if len(pyproject["project"]) > 0 || exists(filepath.Join(f.Root, "setup.py")) || exists(filepath.Join(f.Root, "setup.cfg")) {
			pm = "pip"
		}
	}
	if pm != "" {
		f.PackageMgrs = addUnique(f.PackageMgrs, pm)
		f.Languages = addUnique(f.Languages, "Python")
	} else {
		// Python source without a modern manifest still counts.
		if len(findFiles(f.Root, "*.py")) > 0 {
			f.Languages = addUnique(f.Languages, "Python")
		}
		return
	}
	f.Manifests = append(f.Manifests, "pyproject.toml")

	if req := pyproject["project"]["requires-python"]; req != "" {
		f.ConfigFiles = append(f.ConfigFiles, ToolchainFile{Path: "pyproject.toml", Detail: "requires python " + req})
	}

	// Frameworks & test frameworks from dependencies.
	var deps []string
	collect := func(s string) {
		for k := range pyproject[s] {
			deps = append(deps, strings.ToLower(k))
		}
	}
	collect("project.dependencies") // rarely populated by this parser; harmless
	if raw := readFile(filepath.Join(f.Root, "pyproject.toml")); raw != "" {
		lower := strings.ToLower(raw)
		for dep, label := range map[string]string{
			"django": "Django", "flask": "Flask", "fastapi": "FastAPI",
			"starlette": "Starlette", "tornado": "Tornado",
			"pytest": "pytest", "hypothesis": "Hypothesis",
			"celery": "Celery", "scrapy": "Scrapy",
			"click": "Click", "typer": "Typer",
		} {
			// Match as a quoted dependency name or a [tool.x] section.
			if strings.Contains(lower, "\""+dep+"\"") || strings.Contains(lower, "'"+dep+"'") {
				if label == "pytest" || label == "Hypothesis" {
					f.TestFrameworks = addUnique(f.TestFrameworks, label)
				} else {
					f.Frameworks = append(f.Frameworks, Framework{Name: label, Source: "pyproject.toml"})
				}
			}
		}
	}

	// Named scripts: [project.scripts] are entry points, not build scripts —
	// but Poetry/pdm test/lint tool configs matter more than scripts here.
	if _, ok := pyproject["tool.pytest.ini_options"]; ok {
		f.TestFrameworks = addUnique(f.TestFrameworks, "pytest")
	}

	// Common python commands as scripts.
	base := pm
	if _, ok := pyproject["tool.poetry"]; ok && pm == "poetry" {
		f.Scripts = append(f.Scripts,
			Command{Name: "install", Cmdline: "poetry install", Source: "pyproject.toml", Purpose: "install"},
			Command{Name: "test", Cmdline: "poetry run pytest", Source: "pyproject.toml", Purpose: "test"},
		)
	} else if pm == "uv" {
		f.Scripts = append(f.Scripts,
			Command{Name: "install", Cmdline: "uv sync", Source: "pyproject.toml", Purpose: "install"},
			Command{Name: "test", Cmdline: "uv run pytest", Source: "pyproject.toml", Purpose: "test"},
		)
	} else {
		_ = base
		f.Scripts = append(f.Scripts,
			Command{Name: "install", Cmdline: "pip install -e .", Source: "pyproject.toml", Purpose: "install"},
			Command{Name: "test", Cmdline: "python -m pytest", Source: "pyproject.toml", Purpose: "test"},
		)
	}
}
