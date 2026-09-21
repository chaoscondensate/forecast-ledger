package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/document"
	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
	"github.com/chaoscondensate/forecast-ledger/internal/storage"
	"github.com/chaoscondensate/forecast-ledger/internal/validation"
)

type CollectionFileResult struct {
	LedgerID        ledger.Slug `json:"ledger_id"`
	ID              ledger.Slug `json:"id"`
	Changed         bool        `json:"changed"`
	ChangedPointers []string    `json:"changed_pointers"`
	BeforeSHA256    string      `json:"before_sha256"`
	AfterSHA256     string      `json:"after_sha256"`
}
type collectionBuilder func(*ledger.Ledger) (CollectionMutation, error)

func planCollectionMutation(ctx context.Context, path string, id ledger.Slug, builder collectionBuilder) (CollectionFileResult, error) {
	loaded, err := LoadAndValidateLedger(ctx, path, nil)
	if err != nil {
		return CollectionFileResult{}, err
	}
	mutation, err := builder(loaded.Model)
	if err != nil {
		return CollectionFileResult{}, err
	}
	if err := validateProspectiveFileMutation(loaded, mutation.Patches); err != nil {
		return CollectionFileResult{}, err
	}
	return collectionFileResult(loaded.Document, loaded.Model, id, mutation)
}
func commitCollectionMutation(ctx context.Context, path string, id ledger.Slug, builder collectionBuilder) (CollectionFileResult, error) {
	resolved, err := storage.ResolveLedgerPath(path, true)
	if err != nil {
		return CollectionFileResult{}, err
	}
	artifacts := os.DirFS(filepath.Dir(resolved))
	var result CollectionFileResult
	err = storage.UpdateLedger(ctx, resolved, storage.TransactionOptions{Validate: func(parsed *document.Document) error { return ValidateLedgerDocument(parsed, artifacts) }, Mutate: func(parsed *document.Document) ([]byte, error) {
		model, decodeErr := validation.DecodeLedger(parsed)
		if decodeErr != nil {
			return nil, app.NewError(app.CodeInternal, "validated ledger cannot be decoded", decodeErr)
		}
		mutation, buildErr := builder(model)
		if buildErr != nil {
			return nil, buildErr
		}
		result, buildErr = collectionFileResult(parsed, model, id, mutation)
		if buildErr != nil {
			return nil, buildErr
		}
		return document.ApplyPatch(parsed, mutation.Patches)
	}})
	return result, err
}
func collectionFileResult(parsed *document.Document, model *ledger.Ledger, id ledger.Slug, mutation CollectionMutation) (CollectionFileResult, error) {
	patched, err := document.ApplyPatch(parsed, mutation.Patches)
	if err != nil {
		return CollectionFileResult{}, app.NewError(app.CodeInternal, "collection mutation cannot be applied", err)
	}
	before, after := sha256.Sum256(parsed.Raw), sha256.Sum256(patched)
	return CollectionFileResult{LedgerID: model.LedgerID, ID: id, Changed: len(mutation.Patches) > 0, ChangedPointers: ChangedPointers(mutation.Patches), BeforeSHA256: hex.EncodeToString(before[:]), AfterSHA256: hex.EncodeToString(after[:])}, nil
}

func PlanGroupAddFile(ctx context.Context, path string, id ledger.Slug, input GroupCreateInput) (CollectionFileResult, error) {
	return planCollectionMutation(ctx, path, id, func(model *ledger.Ledger) (CollectionMutation, error) { return BuildGroupAdd(model, id, input) })
}
func CommitGroupAddFile(ctx context.Context, path string, id ledger.Slug, input GroupCreateInput) (CollectionFileResult, error) {
	return commitCollectionMutation(ctx, path, id, func(model *ledger.Ledger) (CollectionMutation, error) { return BuildGroupAdd(model, id, input) })
}
func PlanGroupUpdateFile(ctx context.Context, path string, id ledger.Slug, input GroupPatchInput) (CollectionFileResult, error) {
	return planCollectionMutation(ctx, path, id, func(model *ledger.Ledger) (CollectionMutation, error) { return BuildGroupUpdate(model, id, input) })
}
func CommitGroupUpdateFile(ctx context.Context, path string, id ledger.Slug, input GroupPatchInput) (CollectionFileResult, error) {
	return commitCollectionMutation(ctx, path, id, func(model *ledger.Ledger) (CollectionMutation, error) { return BuildGroupUpdate(model, id, input) })
}
func PlanGroupRemoveFile(ctx context.Context, path string, id ledger.Slug) (CollectionFileResult, error) {
	return planCollectionMutation(ctx, path, id, func(model *ledger.Ledger) (CollectionMutation, error) { return BuildGroupRemove(model, id) })
}
func CommitGroupRemoveFile(ctx context.Context, path string, id ledger.Slug) (CollectionFileResult, error) {
	return commitCollectionMutation(ctx, path, id, func(model *ledger.Ledger) (CollectionMutation, error) { return BuildGroupRemove(model, id) })
}
func LoadGroupList(ctx context.Context, path string, stdin io.Reader) (ledger.Slug, []ledger.Group, error) {
	loaded, err := LoadAndValidateLedger(ctx, path, stdin)
	if err != nil {
		return "", nil, err
	}
	values, err := ListGroups(loaded.Model)
	return loaded.Model.LedgerID, values, err
}
func LoadGroupShow(ctx context.Context, path string, stdin io.Reader, id ledger.Slug) (ledger.Slug, ledger.Group, error) {
	loaded, err := LoadAndValidateLedger(ctx, path, stdin)
	if err != nil {
		return "", ledger.Group{}, err
	}
	value, err := ShowGroup(loaded.Model, id)
	return loaded.Model.LedgerID, value, err
}

func PlanRelationshipAddFile(ctx context.Context, path string, input RelationshipInput) (CollectionFileResult, error) {
	return planCollectionMutation(ctx, path, relationshipID(input.Relationship), func(model *ledger.Ledger) (CollectionMutation, error) { return BuildRelationshipAdd(model, input) })
}
func CommitRelationshipAddFile(ctx context.Context, path string, input RelationshipInput) (CollectionFileResult, error) {
	return commitCollectionMutation(ctx, path, relationshipID(input.Relationship), func(model *ledger.Ledger) (CollectionMutation, error) { return BuildRelationshipAdd(model, input) })
}
func PlanRelationshipRemoveFile(ctx context.Context, path string, id ledger.Slug) (CollectionFileResult, error) {
	return planCollectionMutation(ctx, path, id, func(model *ledger.Ledger) (CollectionMutation, error) { return BuildRelationshipRemove(model, id) })
}
func CommitRelationshipRemoveFile(ctx context.Context, path string, id ledger.Slug) (CollectionFileResult, error) {
	return commitCollectionMutation(ctx, path, id, func(model *ledger.Ledger) (CollectionMutation, error) { return BuildRelationshipRemove(model, id) })
}
func LoadRelationshipList(ctx context.Context, path string, stdin io.Reader) (ledger.Slug, []ledger.Relationship, error) {
	loaded, err := LoadAndValidateLedger(ctx, path, stdin)
	if err != nil {
		return "", nil, err
	}
	values, err := ListRelationships(loaded.Model)
	return loaded.Model.LedgerID, values, err
}
func LoadRelationshipShow(ctx context.Context, path string, stdin io.Reader, id ledger.Slug) (ledger.Slug, ledger.Relationship, error) {
	loaded, err := LoadAndValidateLedger(ctx, path, stdin)
	if err != nil {
		return "", ledger.Relationship{}, err
	}
	value, err := ShowRelationship(loaded.Model, id)
	return loaded.Model.LedgerID, value, err
}
