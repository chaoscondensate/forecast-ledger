package storage

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/document"
)

func TestUpdateLedgerValidatesBeforeLockAndTwiceInsideTransaction(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.yaml")
	original := "# keep\ntitle: 'Before'\ncount: 1\n"
	if err := os.WriteFile(path, []byte(original), 0o640); err != nil {
		t.Fatal(err)
	}
	validations := 0
	err := UpdateLedger(context.Background(), path, TransactionOptions{
		Validate: func(parsed *document.Document) error {
			validations++
			if parsed.Root.Kind != document.ValueObject {
				return errors.New("root is not an object")
			}
			return nil
		},
		Mutate: func(parsed *document.Document) ([]byte, error) {
			return document.ReplaceScalars(parsed, []document.ScalarEdit{{Pointer: "/title", Value: "After"}})
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if validations != 3 {
		t.Fatalf("validations = %d, want 3", validations)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "# keep\ntitle: 'After'\ncount: 1\n" {
		t.Fatalf("unexpected ledger bytes:\n%s", data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Windows only represents a file as writable or read-only; it cannot
	// preserve the POSIX group and other permission bits used by this test.
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %o, want 640", info.Mode().Perm())
	}
	if _, err := os.Stat(JournalPath(path)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("journal remains after success: %v", err)
	}
	assertNoTransactionTemps(t, filepath.Dir(path))
}

func TestUpdateLedgerRollsBackOnPreAndPostValidationFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.json")
	original := []byte("{\"value\":1}\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	mutated := false
	err := UpdateLedger(context.Background(), path, TransactionOptions{
		Validate: func(*document.Document) error { return errors.New("invalid before") },
		Mutate: func(*document.Document) ([]byte, error) {
			mutated = true
			return nil, nil
		},
	})
	if app.ErrorCodeOf(err) != app.CodeInvalidData || mutated {
		t.Fatalf("pre-validation err=%v mutated=%v", err, mutated)
	}

	validations := 0
	err = UpdateLedger(context.Background(), path, TransactionOptions{
		Validate: func(*document.Document) error {
			validations++
			if validations == 3 {
				return errors.New("invalid after")
			}
			return nil
		},
		Mutate: func(parsed *document.Document) ([]byte, error) {
			return document.ReplaceScalars(parsed, []document.ScalarEdit{{Pointer: "/value", Value: int64(2)}})
		},
	})
	if app.ErrorCodeOf(err) != app.CodeInvalidData {
		t.Fatalf("post-validation err=%v", err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != string(original) {
		t.Fatalf("ledger changed after failed validation: %s", data)
	}
	assertNoTransactionTemps(t, filepath.Dir(path))
}

func TestYAMLReplacementParseAndPostValidationFailuresLeaveNoTransactionArtifacts(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate MutateDocumentFunc
		check  ValidateDocumentFunc
	}{
		{
			name: "prospective parse",
			mutate: func(*document.Document) ([]byte, error) {
				return []byte("root:\n  nested:\ninvalid\n"), nil
			},
		},
		{
			name: "post-mutation validation",
			mutate: func(parsed *document.Document) ([]byte, error) {
				return document.ApplyPatch(parsed, []document.PatchOperation{{Kind: document.PatchReplace, Pointer: "/root/nested", Value: map[string]any{"replacement": "valid-yaml"}}})
			},
			check: func() ValidateDocumentFunc {
				calls := 0
				return func(*document.Document) error {
					calls++
					if calls == 3 {
						return errors.New("injected post-mutation rejection")
					}
					return nil
				}
			}(),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			path := filepath.Join(directory, "ledger.yaml")
			original := []byte("# keep\r\nroot:\r\n  nested:\r\n    old: value\r\nuntouched: 'quoted' # keep\r\n")
			if err := os.WriteFile(path, original, 0o640); err != nil {
				t.Fatal(err)
			}
			err := UpdateLedger(context.Background(), path, TransactionOptions{Validate: test.check, Mutate: test.mutate})
			if app.ErrorCodeOf(err) != app.CodeInvalidData {
				t.Fatalf("error = %v, want invalid_data", err)
			}
			retained, readErr := os.ReadFile(path)
			if readErr != nil || !bytes.Equal(retained, original) {
				t.Fatalf("original YAML changed: data=%q err=%v", retained, readErr)
			}
			if _, statErr := os.Stat(JournalPath(path)); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("validation failure retained journal: %v", statErr)
			}
			assertNoTransactionTemps(t, directory)
		})
	}
}

func TestRecoveryCompletesCrashAfterJournalSync(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.json")
	if err := os.WriteFile(path, []byte("{\"value\":1}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	crash := errors.New("simulated crash")
	err := UpdateLedger(context.Background(), path, TransactionOptions{
		Mutate: func(parsed *document.Document) ([]byte, error) {
			return document.ReplaceScalars(parsed, []document.ScalarEdit{{Pointer: "/value", Value: int64(2)}})
		},
		Fault: func(stage TransactionStage) error {
			if stage == StageJournalSynced {
				return crash
			}
			return nil
		},
	})
	if app.ErrorCodeOf(err) != app.CodeIO {
		t.Fatalf("injected crash err=%v", err)
	}
	beforeRecovery, _ := os.ReadFile(path)
	if string(beforeRecovery) != "{\"value\":1}\n" {
		t.Fatalf("original changed before recovery: %s", beforeRecovery)
	}
	if _, err := os.Stat(JournalPath(path)); err != nil {
		t.Fatalf("journal missing after crash: %v", err)
	}
	if err := RecoverLedger(context.Background(), path, 0, nil); err != nil {
		t.Fatal(err)
	}
	afterRecovery, _ := os.ReadFile(path)
	if string(afterRecovery) != "{\"value\":2}\n" {
		t.Fatalf("recovery did not install update: %s", afterRecovery)
	}
	if _, err := os.Stat(JournalPath(path)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("journal remains after recovery: %v", err)
	}
	assertNoTransactionTemps(t, filepath.Dir(path))
}

func TestTransactionAutomaticallyRecoversExistingJournalAndRefusesChangedTarget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.json")
	if err := os.WriteFile(path, []byte("{\"value\":1}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := UpdateLedger(context.Background(), path, TransactionOptions{
		Mutate: func(parsed *document.Document) ([]byte, error) {
			return document.ReplaceScalars(parsed, []document.ScalarEdit{{Pointer: "/value", Value: int64(2)}})
		},
		Fault: func(stage TransactionStage) error {
			if stage == StageJournalSynced {
				return errors.New("stop")
			}
			return nil
		},
	})
	if err == nil {
		t.Fatal("fault did not stop transaction")
	}
	err = UpdateLedger(context.Background(), path, TransactionOptions{Mutate: func(parsed *document.Document) ([]byte, error) { return parsed.Raw, nil }})
	if err != nil {
		t.Fatalf("automatic recovery failed: %v", err)
	}
	recovered, readErr := os.ReadFile(path)
	if readErr != nil || string(recovered) != "{\"value\":2}\n" {
		t.Fatalf("automatic recovery data=%q err=%v", recovered, readErr)
	}
	if _, statErr := os.Stat(JournalPath(path)); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("automatic recovery retained journal: %v", statErr)
	}

	err = UpdateLedger(context.Background(), path, TransactionOptions{
		Mutate: func(parsed *document.Document) ([]byte, error) {
			return document.ReplaceScalars(parsed, []document.ScalarEdit{{Pointer: "/value", Value: int64(3)}})
		},
		Fault: func(stage TransactionStage) error {
			if stage == StageJournalSynced {
				return errors.New("stop again")
			}
			return nil
		},
	})
	if err == nil {
		t.Fatal("second fault did not stop transaction")
	}
	if err := os.WriteFile(path, []byte("{\"value\":99}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err = RecoverLedger(context.Background(), path, 0, nil)
	if app.ErrorCodeOf(err) != app.CodeConflict {
		t.Fatalf("changed recovery target accepted: %v", err)
	}
}

func TestAutomaticRecoveryRefusesMissingChangedAndInvalidTemporaryFiles(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(t *testing.T, tempPath string)
	}{
		{name: "missing", change: func(t *testing.T, tempPath string) {
			t.Helper()
			if err := os.Remove(tempPath); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "changed", change: func(t *testing.T, tempPath string) {
			t.Helper()
			if err := os.WriteFile(tempPath, []byte("{\"value\":7}\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			path := filepath.Join(directory, "ledger.json")
			if err := os.WriteFile(path, []byte("{\"value\":1}\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			err := UpdateLedger(context.Background(), path, TransactionOptions{
				Mutate: func(parsed *document.Document) ([]byte, error) {
					return document.ReplaceScalars(parsed, []document.ScalarEdit{{Pointer: "/value", Value: int64(2)}})
				},
				Fault: func(stage TransactionStage) error {
					if stage == StageJournalSynced {
						return errors.New("stop")
					}
					return nil
				},
			})
			if err == nil {
				t.Fatal("fault did not stop transaction")
			}
			journalBytes, err := os.ReadFile(JournalPath(path))
			if err != nil {
				t.Fatal(err)
			}
			journal, err := decodeRecoveryJournal(journalBytes, path)
			if err != nil {
				t.Fatal(err)
			}
			tempPath := filepath.Join(directory, journal.TempBase)
			test.change(t, tempPath)
			err = UpdateLedger(context.Background(), path, TransactionOptions{Mutate: func(parsed *document.Document) ([]byte, error) { return parsed.Raw, nil }})
			if app.ErrorCodeOf(err) != app.CodeConflict {
				t.Fatalf("automatic recovery error=%v, want conflict", err)
			}
			if _, statErr := os.Stat(JournalPath(path)); statErr != nil {
				t.Fatalf("ambiguous journal was not preserved: %v", statErr)
			}
		})
	}
}

func TestAutomaticRecoveryHonorsCancellationBeforeMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.json")
	if err := os.WriteFile(path, []byte("{\"value\":1}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := UpdateLedger(context.Background(), path, TransactionOptions{
		Mutate: func(parsed *document.Document) ([]byte, error) {
			return document.ReplaceScalars(parsed, []document.ScalarEdit{{Pointer: "/value", Value: int64(2)}})
		},
		Fault: func(stage TransactionStage) error {
			if stage == StageJournalSynced {
				return errors.New("stop")
			}
			return nil
		},
	})
	if err == nil {
		t.Fatal("fault did not stop transaction")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	mutated := false
	err = UpdateLedger(ctx, path, TransactionOptions{Mutate: func(parsed *document.Document) ([]byte, error) { mutated = true; return parsed.Raw, nil }})
	if app.ErrorCodeOf(err) != app.CodeInterrupted || mutated {
		t.Fatalf("cancelled retry err=%v mutated=%v", err, mutated)
	}
	if _, statErr := os.Stat(JournalPath(path)); statErr != nil {
		t.Fatalf("cancelled retry removed journal: %v", statErr)
	}
}

func TestRecoveryCanRestoreTemporarilyMissingTarget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.json")
	if err := os.WriteFile(path, []byte("{\"value\":1}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := UpdateLedger(context.Background(), path, TransactionOptions{
		Mutate: func(parsed *document.Document) ([]byte, error) {
			return document.ReplaceScalars(parsed, []document.ScalarEdit{{Pointer: "/value", Value: int64(2)}})
		},
		Fault: func(stage TransactionStage) error {
			if stage == StageJournalSynced {
				return errors.New("stop")
			}
			return nil
		},
	})
	if err == nil {
		t.Fatal("fault did not stop transaction")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := RecoverLedger(context.Background(), path, 0, nil); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "{\"value\":2}\n" {
		t.Fatalf("recovered data=%q err=%v", data, err)
	}
}

func TestConcurrentWriterReturnsImmediateConflict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.json")
	if err := os.WriteFile(path, []byte("{\"value\":1}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	var firstErr error
	var wait sync.WaitGroup
	wait.Add(1)
	go func() {
		defer wait.Done()
		firstErr = UpdateLedger(context.Background(), path, TransactionOptions{
			Mutate: func(parsed *document.Document) ([]byte, error) {
				close(entered)
				<-release
				return document.ReplaceScalars(parsed, []document.ScalarEdit{{Pointer: "/value", Value: int64(2)}})
			},
		})
	}()
	<-entered

	started := time.Now()
	secondErr := UpdateLedger(context.Background(), path, TransactionOptions{
		Mutate: func(parsed *document.Document) ([]byte, error) {
			return document.ReplaceScalars(parsed, []document.ScalarEdit{{Pointer: "/value", Value: int64(3)}})
		},
	})
	if app.ErrorCodeOf(secondErr) != app.CodeConflict {
		t.Fatalf("second writer error = %v", secondErr)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("second writer waited %s instead of returning immediate conflict", elapsed)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "{\"value\":1}\n" {
		t.Fatalf("ledger changed while first writer was paused: data=%q err=%v", data, err)
	}
	close(release)
	wait.Wait()
	if firstErr != nil {
		t.Fatal(firstErr)
	}
	data, err = os.ReadFile(path)
	if err != nil || string(data) != "{\"value\":2}\n" {
		t.Fatalf("first writer did not commit coherently: data=%q err=%v", data, err)
	}
}

func TestFaultInjectionCoversEveryDurabilityAndCleanupBoundary(t *testing.T) {
	preJournal := []TransactionStage{
		StageTempCreated, StageTempPermissions, StageTempWritten, StageTempSynced,
		StageJournalCreated, StageJournalWritten,
	}
	recoverable := []TransactionStage{
		StageJournalSynced, StageBeforeReplace, StageReplaced, StageDirectorySynced, StageBeforeCleanup,
	}
	committed := []TransactionStage{StageJournalRemoved, StageCleanupSynced}
	for _, group := range []struct {
		name   string
		stages []TransactionStage
		state  string
	}{
		{name: "pre-journal", stages: preJournal, state: "original"},
		{name: "recoverable", stages: recoverable, state: "recoverable"},
		{name: "committed", stages: committed, state: "committed"},
	} {
		for _, stage := range group.stages {
			t.Run(group.name+"/"+string(stage), func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "ledger.json")
				if err := os.WriteFile(path, []byte("{\"value\":1}\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				err := UpdateLedger(context.Background(), path, TransactionOptions{
					Mutate: func(parsed *document.Document) ([]byte, error) {
						return document.ReplaceScalars(parsed, []document.ScalarEdit{{Pointer: "/value", Value: int64(2)}})
					},
					Fault: func(current TransactionStage) error {
						if current == stage {
							return errors.New("stop at " + string(stage))
						}
						return nil
					},
				})
				if app.ErrorCodeOf(err) != app.CodeIO {
					t.Fatalf("injected stage error = %v", err)
				}
				data, readErr := os.ReadFile(path)
				if readErr != nil {
					t.Fatal(readErr)
				}
				switch group.state {
				case "original":
					if string(data) != "{\"value\":1}\n" {
						t.Fatalf("pre-journal failure changed ledger: %s", data)
					}
					if _, statErr := os.Stat(JournalPath(path)); !errors.Is(statErr, os.ErrNotExist) {
						t.Fatalf("pre-journal failure retained journal: %v", statErr)
					}
					assertNoTransactionTemps(t, filepath.Dir(path))
				case "recoverable":
					if _, statErr := os.Stat(JournalPath(path)); statErr != nil {
						t.Fatalf("recoverable failure has no journal: %v", statErr)
					}
					if err := UpdateLedger(context.Background(), path, TransactionOptions{Mutate: func(parsed *document.Document) ([]byte, error) { return parsed.Raw, nil }}); err != nil {
						t.Fatal(err)
					}
					recovered, _ := os.ReadFile(path)
					if string(recovered) != "{\"value\":2}\n" {
						t.Fatalf("recovery result = %s", recovered)
					}
				case "committed":
					if string(data) != "{\"value\":2}\n" {
						t.Fatalf("post-cleanup failure lost commit: %s", data)
					}
					if _, statErr := os.Stat(JournalPath(path)); !errors.Is(statErr, os.ErrNotExist) {
						t.Fatalf("post-cleanup failure retained journal: %v", statErr)
					}
				}
			})
		}
	}
}

func assertNoTransactionTemps(t *testing.T, directory string) {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".forecast-ledger-") && strings.HasSuffix(entry.Name(), ".tmp") {
			t.Fatalf("transaction temp remains: %s", entry.Name())
		}
	}
}
