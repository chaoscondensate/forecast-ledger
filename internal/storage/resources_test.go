package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
)

func TestResourceRecoveryUsesOwnershipAndRollbackClass(t *testing.T) {
	directory := t.TempDir()
	packageRoot := filepath.Join(directory, "package")
	if err := os.Mkdir(packageRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(directory, "target.json")
	response := filepath.Join(directory, "response.tsr")
	key := filepath.Join(directory, "key.json")
	ledger := filepath.Join(directory, "ledger.json")
	packageFile := filepath.Join(packageRoot, "manifest.json")
	unowned := filepath.Join(directory, "existing.txt")
	files := map[string][]byte{
		target: []byte("target"), response: []byte("response"), key: []byte("key"),
		ledger: []byte("ledger"), packageFile: []byte("manifest"), unowned: []byte("existing"),
	}
	for path, data := range files {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	journal := filepath.Join(directory, ".resources.json")
	plan, err := NewResourcePlan(journal, "timestamp.stamp", []ResourceEntry{
		{Kind: ResourceTarget, Type: ResourceFile, Path: target, Owned: true, Rollback: ResourceRollbackRemoveOwned, State: ResourcePlanned},
		{Kind: ResourceTimestampResponse, Type: ResourceFile, Path: response, Owned: true, Rollback: ResourceRollbackRemoveOwned, State: ResourcePlanned},
		{Kind: ResourceKey, Type: ResourceFile, Path: key, Owned: true, Rollback: ResourceRollbackRetainSecret, State: ResourcePlanned},
		{Kind: ResourceLedger, Type: ResourceFile, Path: ledger, Owned: false, Rollback: ResourceRollbackNone, State: ResourcePlanned},
		{Kind: ResourcePackage, Type: ResourceFile, Path: packageFile, Owned: true, Rollback: ResourceRollbackRemoveOwned, State: ResourcePlanned},
		{Kind: ResourcePackage, Type: ResourceDirectory, Path: packageRoot, Owned: true, Rollback: ResourceRollbackRemoveOwned, State: ResourcePlanned},
		{Kind: ResourceTarget, Type: ResourceFile, Path: unowned, Owned: false, Rollback: ResourceRollbackNone, State: ResourcePlanned},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Begin(); err != nil {
		t.Fatal(err)
	}
	for path, data := range map[string][]byte{target: files[target], response: files[response], packageFile: files[packageFile]} {
		if err := plan.MarkCreated(path, ResourceDigest(data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := plan.MarkCreated(packageRoot, ""); err != nil {
		t.Fatal(err)
	}
	if err := plan.MarkCreated(key, ResourceDigest(files[key])); err != nil {
		t.Fatal(err)
	}
	report, err := RecoverResourcePlan(context.Background(), journal)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Removed) != 4 || len(report.Retained) != 1 {
		t.Fatalf("recovery report = %#v", report)
	}
	for _, retained := range []string{key, ledger, unowned} {
		if _, err := os.Stat(retained); err != nil {
			t.Fatalf("retained resource %s: %v", retained, err)
		}
	}
	for _, removed := range []string{target, response, packageFile, packageRoot} {
		if _, err := os.Stat(removed); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("owned rollback resource %s remains: %v", removed, err)
		}
	}
}

func TestResourceRecoveryPreservesPartialReplacementAndJournal(t *testing.T) {
	directory := t.TempDir()
	indexPath := filepath.Join(directory, "evidence-index.json")
	ledgerPath := filepath.Join(directory, "ledger.json")
	artifactPath := filepath.Join(directory, "target.json")
	journalPath := filepath.Join(directory, ".resources.json")
	indexBefore, indexAfter := []byte("index before\n"), []byte("index after\n")
	ledgerBefore := []byte("ledger before\n")
	artifact := []byte("target\n")
	for path, data := range map[string][]byte{indexPath: indexBefore, ledgerPath: ledgerBefore, artifactPath: artifact} {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := NewResourcePlan(journalPath, "target.build", []ResourceEntry{
		{Kind: ResourceTarget, Type: ResourceFile, Path: artifactPath, Owned: true, Rollback: ResourceRollbackRemoveOwned, State: ResourcePlanned},
		{Kind: ResourceEvidenceIndex, Type: ResourceFile, Path: indexPath, Rollback: ResourceRollbackNone, State: ResourcePlanned, BeforeSHA256: ResourceDigest(indexBefore)},
		{Kind: ResourceLedger, Type: ResourceFile, Path: ledgerPath, Rollback: ResourceRollbackNone, State: ResourcePlanned, BeforeSHA256: ResourceDigest(ledgerBefore)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Begin(); err != nil {
		t.Fatal(err)
	}
	if err := plan.MarkCreated(artifactPath, ResourceDigest(artifact)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(indexPath, indexAfter, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := plan.MarkReplaced(indexPath, ResourceDigest(indexAfter)); err != nil {
		t.Fatal(err)
	}
	if _, err := RecoverResourcePlan(context.Background(), journalPath); app.ErrorCodeOf(err) != app.CodeConflict {
		t.Fatalf("partial replacement recovery error = %v", err)
	}
	for _, path := range []string{journalPath, indexPath, ledgerPath, artifactPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("partial replacement did not preserve %s: %v", path, err)
		}
	}
}

func TestResourceRecoveryCompletesFullyCommittedJournal(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "target.json")
	journalPath := filepath.Join(directory, ".resources.json")
	data := []byte("target\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := NewResourcePlan(journalPath, "target.build", []ResourceEntry{{
		Kind: ResourceTarget, Type: ResourceFile, Path: path, Owned: true, Rollback: ResourceRollbackRemoveOwned, State: ResourcePlanned,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Begin(); err != nil {
		t.Fatal(err)
	}
	if err := plan.MarkCreated(path, ResourceDigest(data)); err != nil {
		t.Fatal(err)
	}
	if err := plan.MarkCommitted(path); err != nil {
		t.Fatal(err)
	}
	report, err := RecoverResourcePlan(t.Context(), journalPath)
	if err != nil || len(report.Retained) != 1 || report.Retained[0] != path {
		t.Fatalf("committed recovery = %#v, %v", report, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("committed resource was removed: %v", err)
	}
	if _, err := os.Stat(journalPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("committed journal remains: %v", err)
	}
}

func TestResourceRecoveryRetainsChangedOwnedFileAndJournal(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "target.json")
	if err := os.WriteFile(path, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	journal := filepath.Join(directory, ".resources.json")
	plan, err := NewResourcePlan(journal, "target.build", []ResourceEntry{{
		Kind: ResourceTarget, Type: ResourceFile, Path: path, Owned: true,
		Rollback: ResourceRollbackRemoveOwned, State: ResourcePlanned,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Begin(); err != nil {
		t.Fatal(err)
	}
	if err := plan.MarkCreated(path, ResourceDigest([]byte("first"))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = RecoverResourcePlan(context.Background(), journal)
	if app.ErrorCodeOf(err) != app.CodeConflict {
		t.Fatalf("changed recovery error = %v", err)
	}
	if data, readErr := os.ReadFile(path); readErr != nil || string(data) != "changed" {
		t.Fatalf("changed resource was removed: data=%q err=%v", data, readErr)
	}
	if _, statErr := os.Stat(journal); statErr != nil {
		t.Fatalf("journal was removed after conflict: %v", statErr)
	}
}

func TestResourceCrashRecoveryAndRetryNeverChangesUnownedFiles(t *testing.T) {
	directory := t.TempDir()
	ledgerPath := filepath.Join(directory, "ledger.json")
	targetPath := filepath.Join(directory, "target.json")
	responsePath := filepath.Join(directory, "response.tsr")
	keyPath := filepath.Join(directory, "key.json")
	journalPath := filepath.Join(directory, ".stamp-resources.json")
	ledgerBytes := []byte("original ledger\n")
	targetBytes := []byte("deterministic target")
	responseBytes := []byte("pending response")
	keyBytes := []byte("durable key")
	for path, content := range map[string][]byte{
		ledgerPath: ledgerBytes, targetPath: targetBytes, responsePath: responseBytes, keyPath: keyBytes,
	} {
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := NewResourcePlan(journalPath, "timestamp.stamp", []ResourceEntry{
		{Kind: ResourceLedger, Type: ResourceFile, Path: ledgerPath, Owned: false, Rollback: ResourceRollbackNone, State: ResourcePlanned},
		{Kind: ResourceTarget, Type: ResourceFile, Path: targetPath, Owned: false, Rollback: ResourceRollbackNone, State: ResourcePlanned},
		{Kind: ResourceTimestampResponse, Type: ResourceFile, Path: responsePath, Owned: true, Rollback: ResourceRollbackRemoveOwned, State: ResourcePlanned},
		{Kind: ResourceKey, Type: ResourceFile, Path: keyPath, Owned: true, Rollback: ResourceRollbackRetainSecret, State: ResourcePlanned},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Begin(); err != nil {
		t.Fatal(err)
	}
	if err := plan.MarkCreated(responsePath, ResourceDigest(responseBytes)); err != nil {
		t.Fatal(err)
	}
	if err := plan.MarkCreated(keyPath, ResourceDigest(keyBytes)); err != nil {
		t.Fatal(err)
	}
	if _, err := RecoverResourcePlan(context.Background(), journalPath); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string][]byte{ledgerPath: ledgerBytes, targetPath: targetBytes, keyPath: keyBytes} {
		got, readErr := os.ReadFile(path)
		if readErr != nil || string(got) != string(want) {
			t.Fatalf("retained %s = %q, want %q, err=%v", path, got, want, readErr)
		}
	}
	if _, err := os.Stat(responsePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("created response survived rollback: %v", err)
	}

	targetResult, err := EnsureDeterministicFile(targetPath, targetBytes, 0o600, 1024)
	if err != nil || targetResult.State != DeterministicUnchanged {
		t.Fatalf("retry target result=%#v err=%v", targetResult, err)
	}
	responseResult, err := EnsureDeterministicFile(responsePath, responseBytes, 0o600, 1024)
	if err != nil || responseResult.State != DeterministicCreated {
		t.Fatalf("retry response result=%#v err=%v", responseResult, err)
	}
	if got, err := os.ReadFile(ledgerPath); err != nil || string(got) != string(ledgerBytes) {
		t.Fatalf("retry changed original ledger: data=%q err=%v", got, err)
	}
}

func TestResourcePlanRequiresIndexBeforeLedgerWithBeforeIdentities(t *testing.T) {
	directory := t.TempDir()
	indexPath := filepath.Join(directory, "evidence-index.json")
	ledgerPath := filepath.Join(directory, "ledger.json")
	journalPath := filepath.Join(directory, ".resources.json")
	emptyDigest := ResourceDigest(nil)

	_, err := NewResourcePlan(journalPath, "target.build", []ResourceEntry{
		{Kind: ResourceLedger, Type: ResourceFile, Path: ledgerPath, Rollback: ResourceRollbackNone, State: ResourcePlanned, BeforeSHA256: emptyDigest},
		{Kind: ResourceEvidenceIndex, Type: ResourceFile, Path: indexPath, Rollback: ResourceRollbackNone, State: ResourcePlanned, BeforeSHA256: emptyDigest},
	})
	if app.ErrorCodeOf(err) != app.CodeInvalidData {
		t.Fatalf("reversed replacement order error = %v", err)
	}

	_, err = NewResourcePlan(journalPath, "target.build", []ResourceEntry{
		{Kind: ResourceEvidenceIndex, Type: ResourceFile, Path: indexPath, Rollback: ResourceRollbackNone, State: ResourcePlanned},
		{Kind: ResourceLedger, Type: ResourceFile, Path: ledgerPath, Rollback: ResourceRollbackNone, State: ResourcePlanned},
	})
	if app.ErrorCodeOf(err) != app.CodeInvalidData {
		t.Fatalf("missing before identities error = %v", err)
	}
}

func TestResourcePlanBeginRejectsChangedBeforeIdentity(t *testing.T) {
	directory := t.TempDir()
	indexPath := filepath.Join(directory, "evidence-index.json")
	ledgerPath := filepath.Join(directory, "ledger.json")
	journalPath := filepath.Join(directory, ".resources.json")
	indexBytes := []byte("index before\n")
	ledgerBytes := []byte("ledger before\n")
	if err := os.WriteFile(indexPath, indexBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ledgerPath, ledgerBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := NewResourcePlan(journalPath, "target.build", []ResourceEntry{
		{Kind: ResourceEvidenceIndex, Type: ResourceFile, Path: indexPath, Rollback: ResourceRollbackNone, State: ResourcePlanned, BeforeSHA256: ResourceDigest(indexBytes)},
		{Kind: ResourceLedger, Type: ResourceFile, Path: ledgerPath, Rollback: ResourceRollbackNone, State: ResourcePlanned, BeforeSHA256: ResourceDigest(ledgerBytes)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(indexPath, []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := plan.Begin(); app.ErrorCodeOf(err) != app.CodeConflict {
		t.Fatalf("changed before identity error = %v", err)
	}
	if _, err := os.Stat(journalPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("journal exists after rejected begin: %v", err)
	}
}

func FuzzResourceJournalDecode(f *testing.F) {
	f.Add([]byte(`{"schema":"forecast-resource-journal/v1","operation":"target.build","created_at":"2026-09-23T00:00:00Z","resources":[]}` + "\n"))
	f.Add([]byte("not-json"))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			data = data[:1<<20]
		}
		path := filepath.Join(t.TempDir(), ".resources.json")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		_, _ = readResourceJournal(path)
	})
}
