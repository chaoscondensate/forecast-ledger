package releasecheck

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemovedLegacyPathsStayAbsent(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	for _, relative := range []string{
		"docs/how-to/migrate-v1.3-to-v2.md",
		"internal/schema/legacy",
		"internal/schema/upstream/forecast-ledger/v2.0.0/docs/compatibility-v1-v2.md",
		"internal/schema/upstream/forecast-ledger/v2.0.0/tests/vectors/forecast-seal-v1.json",
		"internal/schema/upstream/forecast-ledger/v2.0.0/tests/vectors/legacy-v1.3.0-sha256.json",
		"internal/schema/upstream/forecast-ledger/v2.0.0/tools/verify_legacy.py",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(relative))); !os.IsNotExist(err) {
			t.Errorf("removed legacy path exists: %s (err=%v)", relative, err)
		}
	}
}

func TestRemovedLegacyTermsStayOutOfActiveSurfaces(t *testing.T) {
	root := repositoryRoot(t)
	forbidden := []string{
		"v1.3",
		"forecast-envelope/v1",
		"forecast-seal/v1",
		"legacy-v1",
		"migrate-v1.3",
	}
	for _, directory := range []string{"cmd", "internal", "tools", "scripts", "docs"} {
		err := filepath.WalkDir(filepath.Join(root, directory), func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			if strings.HasSuffix(path, "_test.go") {
				return nil
			}
			extension := strings.ToLower(filepath.Ext(path))
			if extension != ".go" && extension != ".md" && extension != ".json" && extension != ".yaml" && extension != ".yml" && extension != ".sh" {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, token := range forbidden {
				if strings.Contains(strings.ToLower(string(data)), token) {
					t.Errorf("active surface %s contains removed legacy term %q", relative, token)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestRemovedAuthoringTermsRemainRejected(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repositoryRoot(t), "internal", "adapters", "cli", "authoring_flags_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"probability_bp", "multiple_choice", "question_annul", "immutable revision"} {
		if !strings.Contains(string(data), required) {
			t.Errorf("removed-surface regression test does not name %q", required)
		}
	}
}

func TestChangelogIsTheOnlyPublicLegacyExplanation(t *testing.T) {
	root := repositoryRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "CHANGELOG.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"Breaking", "No migration or compatibility bundle is provided", "no active users to migrate"} {
		if !strings.Contains(string(data), required) {
			t.Errorf("changelog does not contain %q", required)
		}
	}

	for _, index := range []string{"README.md", "docs/how-to/index.md", "docs/index.md"} {
		content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(index)))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(strings.ToLower(string(content)), "migrate-v1") || strings.Contains(strings.ToLower(string(content)), "v1.3-to-v2") {
			t.Errorf("%s still links legacy migration guidance", index)
		}
	}
}
