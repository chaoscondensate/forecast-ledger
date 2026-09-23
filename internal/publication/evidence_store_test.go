package publication

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestInspectManagedStoreInventoriesBoundedRegularFiles(t *testing.T) {
	root := t.TempDir()
	writeManagedTestFile(t, root, "proofs/targets/f-one.json", []byte("target"))
	writeManagedTestFile(t, root, EvidenceIndexPath, []byte("index"))
	writeManagedTestFile(t, root, "trust/tsa.pem", []byte("trust"))
	store, err := InspectManagedStore(root, DefaultManagedStoreLimits())
	if err != nil {
		t.Fatal(err)
	}
	if len(store.Artifacts) != 2 || !bytes.Equal(store.IndexBytes, []byte("index")) || store.TotalBytes != 16 {
		t.Fatalf("store = %#v", store)
	}
	if store.Artifacts["proofs/targets/f-one.json"].SHA256 == "" || store.Artifacts["trust/tsa.pem"].Size != 5 {
		t.Fatalf("artifacts = %#v", store.Artifacts)
	}
}

func TestInspectManagedStoreAllowsNoEvidenceWithoutCreatingAnything(t *testing.T) {
	root := t.TempDir()
	store, err := InspectManagedStore(root, DefaultManagedStoreLimits())
	if err != nil || len(store.Artifacts) != 0 || store.IndexBytes != nil {
		t.Fatalf("empty store = %#v, %v", store, err)
	}
	if _, err := os.Stat(filepath.Join(root, "proofs")); !os.IsNotExist(err) {
		t.Fatalf("inspection created proofs: %v", err)
	}
}

func TestInspectManagedStoreRejectsLinksCollisionsAndBudgets(t *testing.T) {
	t.Run("symlink", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("symlink creation requires platform privileges")
		}
		root := t.TempDir()
		writeManagedTestFile(t, root, "outside", []byte("secret"))
		if err := os.MkdirAll(filepath.Join(root, "proofs"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(root, "outside"), filepath.Join(root, "proofs", "linked")); err != nil {
			t.Fatal(err)
		}
		if _, err := InspectManagedStore(root, DefaultManagedStoreLimits()); err == nil {
			t.Fatal("managed symlink accepted")
		}
	})
	t.Run("case collision", func(t *testing.T) {
		root := t.TempDir()
		writeManagedTestFile(t, root, "proofs/A.json", []byte("one"))
		writeManagedTestFile(t, root, "proofs/a.json", []byte("two"))
		upper, upperErr := os.Lstat(filepath.Join(root, "proofs", "A.json"))
		lower, lowerErr := os.Lstat(filepath.Join(root, "proofs", "a.json"))
		if upperErr == nil && lowerErr == nil && os.SameFile(upper, lower) {
			t.Skip("filesystem is case-insensitive")
		}
		if _, err := InspectManagedStore(root, DefaultManagedStoreLimits()); err == nil {
			t.Fatal("portable collision accepted")
		}
	})
	for name, limits := range map[string]ManagedStoreLimits{
		"entry count": {MaxEntries: 1},
		"file bytes":  {MaxFileBytes: 2},
		"total bytes": {MaxFileBytes: 8, MaxTotalBytes: 3},
		"depth":       {MaxDepth: 1},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeManagedTestFile(t, root, "proofs/deep/one.bin", []byte("one"))
			writeManagedTestFile(t, root, "trust/two.bin", []byte("two"))
			if _, err := InspectManagedStore(root, limits); err == nil {
				t.Fatal("managed evidence budget was not enforced")
			}
		})
	}
}

func writeManagedTestFile(t *testing.T, root, relative string, data []byte) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
