package mcp

import (
	"errors"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/chaoscondensate/forecast-ledger/internal/app"
	"github.com/chaoscondensate/forecast-ledger/internal/service"
	"github.com/chaoscondensate/forecast-ledger/internal/storage"
)

// RootSpec uses explicit name=path syntax so tool calls never depend on root
// ordering or an inferred default.
type RootSpec struct {
	Name string
	Path string
}

type RootSet struct {
	byClass map[service.RootClass]map[string]*storage.PathResolver
	roots   service.Roots
}

func NewRootSet(ledgerSpecs, outputSpecs, secretSpecs []string) (*RootSet, error) {
	if len(ledgerSpecs) == 0 {
		return nil, app.WithDetails(app.NewError(app.CodeUsage, "at least one ledger root is required", nil), map[string]any{"class": service.RootLedger, "flag": rootFlag(service.RootLedger)})
	}
	set := &RootSet{byClass: map[service.RootClass]map[string]*storage.PathResolver{}}
	classes := []struct {
		class service.RootClass
		items []string
	}{
		{service.RootLedger, ledgerSpecs}, {service.RootOutput, outputSpecs}, {service.RootSecret, secretSpecs},
	}
	var all []service.Root
	for _, group := range classes {
		set.byClass[group.class] = map[string]*storage.PathResolver{}
		for _, raw := range group.items {
			spec, err := parseRootSpec(raw, group.class)
			if err != nil {
				return nil, err
			}
			if _, duplicate := set.byClass[group.class][spec.Name]; duplicate {
				return nil, app.WithDetails(app.NewError(app.CodeConflict, "root names must be unique within each root class", nil), map[string]any{"class": group.class, "flag": rootFlag(group.class), "route": routeID(group.class, spec.Name)})
			}
			resolver, err := storage.NewPathResolver(spec.Path)
			if err != nil {
				return nil, rootConfigurationError(group.class, spec.Name, err)
			}
			root := service.Root{Name: spec.Name, Class: group.class, Path: resolver.Root()}
			set.byClass[group.class][spec.Name] = resolver
			all = append(all, root)
			switch group.class {
			case service.RootLedger:
				set.roots.Ledger = append(set.roots.Ledger, root)
			case service.RootOutput:
				set.roots.Output = append(set.roots.Output, root)
			case service.RootSecret:
				set.roots.Secret = append(set.roots.Secret, root)
			}
		}
	}
	for left := 0; left < len(all); left++ {
		for right := left + 1; right < len(all); right++ {
			if pathsOverlap(all[left].Path, all[right].Path) {
				return nil, app.WithDetails(app.NewError(app.CodeConflict, "configured roots must not overlap", nil), map[string]any{
					"first_route": routeID(all[left].Class, all[left].Name), "first_class": all[left].Class, "first_flag": rootFlag(all[left].Class),
					"second_route": routeID(all[right].Class, all[right].Name), "second_class": all[right].Class, "second_flag": rootFlag(all[right].Class),
				})
			}
		}
	}
	return set, nil
}

func parseRootSpec(raw string, class service.RootClass) (RootSpec, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return RootSpec{}, app.WithDetails(app.NewError(app.CodeUsage, "root value is empty", nil), map[string]any{"class": class, "flag": rootFlag(class)})
	}
	before, after, found := strings.Cut(raw, "=")
	if !found {
		return RootSpec{}, app.WithDetails(app.NewError(app.CodeUsage, "roots must use explicit name=path syntax", nil), map[string]any{"class": class, "flag": rootFlag(class)})
	}
	name, path := strings.TrimSpace(before), strings.TrimSpace(after)
	if !validRootName(name) || path == "" {
		details := map[string]any{"class": class, "flag": rootFlag(class)}
		if validRootName(name) {
			details["route"] = routeID(class, name)
		}
		return RootSpec{}, app.WithDetails(app.NewError(app.CodeUsage, "roots must use a safe name=path value", nil), details)
	}
	return RootSpec{Name: name, Path: path}, nil
}

func validRootName(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for index, r := range value {
		if unicode.IsLower(r) || unicode.IsDigit(r) || (index > 0 && (r == '-' || r == '_')) {
			continue
		}
		return false
	}
	return true
}

func pathsOverlap(left, right string) bool {
	left, right = filepath.Clean(left), filepath.Clean(right)
	if strings.EqualFold(left, right) {
		return true
	}
	relative, err := filepath.Rel(left, right)
	if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return true
	}
	relative, err = filepath.Rel(right, left)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func (r *RootSet) Resolve(class service.RootClass, reference string, mustExist bool) (string, error) {
	label := "artifact"
	switch class {
	case service.RootLedger:
		label = "ledger file"
	case service.RootOutput:
		label = "output file"
	case service.RootSecret:
		label = "protected file"
	}
	return r.ResolveLabeled(class, reference, mustExist, label)
}

func (r *RootSet) ResolveLabeled(class service.RootClass, reference string, mustExist bool, label string) (string, error) {
	if r == nil {
		return "", app.NewError(app.CodeInternal, "MCP roots are not configured", nil)
	}
	name, relative, found := strings.Cut(reference, ":")
	if !found || name == "" || relative == "" {
		return "", app.WithDetails(app.NewError(app.CodeUsage, "path references must use root-name:relative/path", nil), map[string]any{"class": class, "flag": rootFlag(class)})
	}
	resolver := r.byClass[class][name]
	if resolver == nil {
		return "", app.WithDetails(app.NewError(app.CodeNotFound, "requested root is not configured", nil), map[string]any{"root": name, "class": class, "flag": rootFlag(class), "route": routeID(class, name)})
	}
	resolved, err := resolver.ResolveLabeled(relative, mustExist, label)
	if err != nil {
		message := "path reference is not allowed for the configured root"
		var applicationErr *app.Error
		if mustExist && errors.As(err, &applicationErr) && applicationErr.Code == app.CodeNotFound {
			message = applicationErr.Message
		}
		return "", app.WithDetails(app.NewError(app.ErrorCodeOf(err), message, err), map[string]any{"root": name, "class": class, "flag": rootFlag(class), "route": routeID(class, name)})
	}
	return resolved, nil
}

func rootFlag(class service.RootClass) string {
	switch class {
	case service.RootLedger:
		return "--ledger-root"
	case service.RootOutput:
		return "--output-root"
	case service.RootSecret:
		return "--secret-root"
	default:
		return "--root"
	}
}

func routeID(class service.RootClass, name string) string {
	return string(class) + ":" + name
}

func rootConfigurationError(class service.RootClass, name string, cause error) error {
	code := app.ErrorCodeOf(cause)
	if code == app.CodeInternal {
		code = app.CodeIO
	}
	return app.WithDetails(app.NewError(code, "configured root cannot be opened safely", cause), map[string]any{"class": class, "flag": rootFlag(class), "route": routeID(class, name)})
}

func (r *RootSet) Has(class service.RootClass) bool {
	return r != nil && len(r.byClass[class]) > 0
}

func (r *RootSet) Public() service.Roots {
	if r == nil {
		return service.Roots{}
	}
	return r.roots
}

func (r *RootSet) Names(class service.RootClass) []string {
	var names []string
	for name := range r.byClass[class] {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
