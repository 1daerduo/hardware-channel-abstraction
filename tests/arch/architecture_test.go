// Package arch holds the static Architecture Test (Design doc 17 §15): it
// enforces the import-dependency rules of the layered architecture so a
// dependency inversion (e.g. core importing a concrete plugin) fails CI.
package arch

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const module = "example.com/embedded-loop-channel"

type pkg struct {
	ImportPath string
	Imports    []string
}

// TestArchitectureImportRules is the dependency-direction gate.
func TestArchitectureImportRules(t *testing.T) {
	root := moduleRoot(t)
	cmd := exec.Command("go", "list", "-json", "./...")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list failed: %v", err)
	}

	dec := json.NewDecoder(bytes.NewReader(out))
	for {
		var p pkg
		if err := dec.Decode(&p); err == io.EOF {
			break
		} else if err != nil {
			t.Fatalf("decode go list: %v", err)
		}
		check(t, p.ImportPath, p.Imports)
	}
}

func check(t *testing.T, pkgPath string, imports []string) {
	t.Helper()
	for _, imp := range imports {
		if !strings.HasPrefix(imp, module) {
			continue
		}

		// 1. domain is the stable core concept: stdlib only.
		if pkgPath == module+"/domain" {
			t.Errorf("architecture: domain imports %s (must be stdlib-only)", imp)
		}

		// 2. core must not depend on a concrete plugin or the fake simulator.
		if strings.HasPrefix(pkgPath, module+"/core") {
			for _, banned := range []string{"/plugin/adb", "/plugin/uart", "/plugin/mcp", "/fake"} {
				if strings.HasPrefix(imp, module+banned) {
					t.Errorf("architecture: %s imports concrete %s", pkgPath, imp)
				}
			}
		}

		// 3. a concrete plugin must not depend on core internals.
		if isConcretePlugin(pkgPath) && strings.HasPrefix(imp, module+"/core") {
			t.Errorf("architecture: %s imports core %s", pkgPath, imp)
		}

		// 4. fake (reference simulator) depends only on domain.
		if pkgPath == module+"/fake" {
			for _, banned := range []string{"/core", "/plugin", "/runtime", "/sdk", "/transport"} {
				if strings.HasPrefix(imp, module+banned) {
					t.Errorf("architecture: fake imports %s", imp)
				}
			}
		}

		// 5. sdk (the Loop-facing API) must not depend on a concrete plugin.
		if pkgPath == module+"/sdk" {
			for _, banned := range []string{"/plugin/adb", "/plugin/uart", "/plugin/mcp", "/fake"} {
				if strings.HasPrefix(imp, module+banned) {
					t.Errorf("architecture: sdk imports concrete %s", imp)
				}
			}
		}
	}
}

func isConcretePlugin(pkg string) bool {
	return strings.HasPrefix(pkg, module+"/plugin/adb") ||
		strings.HasPrefix(pkg, module+"/plugin/uart") ||
		strings.HasPrefix(pkg, module+"/plugin/mcp")
}

// TestCoreHasNoProtocolSpecials enforces "no protocol special-case in core"
// (Design doc 12 §58): core and sdk must not contain protocol-name string
// literals. A concrete protocol (adb/uart/jtag/mcp/fastboot) may appear only
// inside its own plugin package, never as a branch in the abstraction.
func TestCoreHasNoProtocolSpecials(t *testing.T) {
	root := moduleRoot(t)
	forbidden := []string{`"adb"`, `"uart"`, `"jtag"`, `"mcp"`, `"fastboot"`}
	for _, dir := range []string{"core", "sdk"} {
		base := filepath.Join(root, dir)
		err := filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			if strings.HasSuffix(path, "_test.go") || !strings.HasSuffix(path, ".go") {
				return nil
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, lit := range forbidden {
				if bytes.Contains(b, []byte(lit)) {
					t.Errorf("architecture: %s contains protocol literal %s (core/sdk must be protocol-agnostic)", path, lit)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}
}

// moduleRoot walks up from the current working directory to find go.mod.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("go.mod not found above %s", dir)
	return ""
}
