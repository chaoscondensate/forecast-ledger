package validation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"reflect"
	"sort"
	"strconv"
	"time"
	_ "time/tzdata"

	"github.com/chaoscondensate/forecast-ledger/internal/document"
	"github.com/chaoscondensate/forecast-ledger/internal/exact"
	"github.com/chaoscondensate/forecast-ledger/internal/forecastcrypto"
	"github.com/chaoscondensate/forecast-ledger/internal/ledger"
	targetbytes "github.com/chaoscondensate/forecast-ledger/internal/target"
)

type SemanticIssue struct {
	Layer   string `json:"layer"`
	Code    string `json:"code"`
	Pointer string `json:"pointer"`
	Message string `json:"message"`
}

func DecodeLedger(source *document.Document) (*ledger.Ledger, error) {
	if source == nil || source.Root == nil {
		return nil, errors.New("document has no root value")
	}
	encoded, err := json.Marshal(source.Root.Any())
	if err != nil {
		return nil, fmt.Errorf("encode parsed ledger: %w", err)
	}
	var result ledger.Ledger
	if err := json.Unmarshal(encoded, &result); err != nil {
		return nil, fmt.Errorf("decode typed ledger: %w", err)
	}
	return &result, nil
}

// ValidateSemantics runs only after structural validation succeeds. artifacts
// is rooted at the ledger directory; nil deliberately skips retained-byte
// checks for prospective in-memory validation.
func ValidateSemantics(model *ledger.Ledger, artifacts fs.FS) ([]SemanticIssue, error) {
	if model == nil {
		return nil, errors.New("ledger is nil")
	}
	validator := semanticValidator{model: model, artifacts: artifacts}
	validator.validate()
	sort.Slice(validator.issues, func(i, j int) bool {
		left, right := validator.issues[i], validator.issues[j]
		if left.Pointer != right.Pointer {
			return left.Pointer < right.Pointer
		}
		if left.Code != right.Code {
			return left.Code < right.Code
		}
		return left.Message < right.Message
	})
	return validator.issues, nil
}

type semanticValidator struct {
	model     *ledger.Ledger
	artifacts fs.FS
	issues    []SemanticIssue
	platforms map[ledger.Slug]struct{}
	questions map[ledger.Slug]*ledger.Question
}

func (v *semanticValidator) validate() {
	v.validateRoot()
	globalForecasts := make([]ledger.Slug, 0)
	for index := range v.model.Questions {
		globalForecasts = append(globalForecasts, v.validateQuestion(index, &v.model.Questions[index])...)
	}
	v.uniqueSlugs(globalForecasts, "/questions/*/forecasts", "semantic.duplicate_forecast_id")
	v.validateRelationships()
	v.validateNotApplicableResolutions()
}

func (v *semanticValidator) validateRoot() {
	if _, err := time.LoadLocation(v.model.DefaultTimezone); err != nil {
		v.add("semantic.timezone", "/default_timezone", "timezone is not a known IANA name")
	}
	if v.model.Forecaster.Kind == ledger.ForecasterTeam && v.model.Forecaster.Members != nil {
		ids := make([]ledger.Slug, len(*v.model.Forecaster.Members))
		for index, member := range *v.model.Forecaster.Members {
			ids[index] = member.ID
		}
		v.uniqueSlugs(ids, "/forecaster/members", "semantic.duplicate_member_id")
	}
	v.platforms = make(map[ledger.Slug]struct{}, len(v.model.Platforms))
	for id := range v.model.Platforms {
		v.platforms[id] = struct{}{}
	}
	v.questions = make(map[ledger.Slug]*ledger.Question, len(v.model.Questions))
	ids := make([]ledger.Slug, len(v.model.Questions))
	for index := range v.model.Questions {
		question := &v.model.Questions[index]
		ids[index] = question.ID
		if _, exists := v.questions[question.ID]; !exists {
			v.questions[question.ID] = question
		}
	}
	v.uniqueSlugs(ids, "/questions", "semantic.duplicate_question_id")
}

func (v *semanticValidator) validateQuestion(index int, question *ledger.Question) []ledger.Slug {
	pointer := "/questions/" + strconv.Itoa(index)
	revisions := make(map[ledger.Slug]*ledger.QuestionRevision, len(question.Revisions))
	revisionIDs := make([]ledger.Slug, len(question.Revisions))
	var previousEffective, previousRecorded time.Time
	for revisionIndex := range question.Revisions {
		revision := &question.Revisions[revisionIndex]
		revisionIDs[revisionIndex] = revision.ID
		if _, exists := revisions[revision.ID]; !exists {
			revisions[revision.ID] = revision
		}
		rp := fmt.Sprintf("%s/revisions/%d", pointer, revisionIndex)
		effective, effectiveErr := ledger.ParseTimestamp(revision.EffectiveAt)
		recorded, recordedErr := ledger.ParseTimestamp(revision.RecordedAt)
		if effectiveErr == nil && recordedErr == nil {
			if recorded.Before(effective) {
				v.add("semantic.revision_chronology", rp+"/recorded_at", "recorded_at must not precede effective_at")
			}
			if !previousEffective.IsZero() && !effective.After(previousEffective) {
				v.add("semantic.revision_effective_order", rp, "revisions must be strictly ordered by effective_at")
			}
			if !previousRecorded.IsZero() && recorded.Before(previousRecorded) {
				v.add("semantic.revision_recorded_order", rp, "revisions must be append-only ordered by recorded_at")
			}
			previousEffective, previousRecorded = effective, recorded
		}
		kind, ok := revision.Domain.Kind()
		if !ok || kind != revision.OutcomeSpace.Kind {
			v.add("semantic.domain_kind", rp+"/domain/kind", "domain kind must match outcome_space.kind")
		}
		v.validateDomain(revision.Domain, rp+"/domain")
		if revision.Provenance != nil {
			v.validateProvenance(revision.Provenance, rp+"/provenance")
		}
	}
	v.uniqueSlugs(revisionIDs, pointer+"/revisions", "semantic.duplicate_revision_id")
	current, exists := revisions[question.CurrentRevisionID]
	if !exists {
		v.add("semantic.current_revision", pointer+"/current_revision_id", "current_revision_id must reference a question revision")
	} else if len(question.Revisions) > 0 && current != &question.Revisions[len(question.Revisions)-1] {
		v.add("semantic.current_revision_last", pointer+"/current_revision_id", "current_revision_id must reference the last revision")
	}

	forecastIDs := make([]ledger.Slug, 0, len(question.Forecasts))
	localForecasts := make(map[ledger.Slug]struct{}, len(question.Forecasts))
	var previousForecastRecorded time.Time
	for forecastIndex := range question.Forecasts {
		forecast := &question.Forecasts[forecastIndex]
		forecastIDs = append(forecastIDs, forecast.ID)
		fp := fmt.Sprintf("%s/forecasts/%d", pointer, forecastIndex)
		revision := revisions[forecast.QuestionRevisionID]
		if revision == nil {
			v.add("semantic.forecast_revision", fp+"/question_revision_id", "must reference a revision of this question")
		} else {
			v.validateForecastAgainstRevision(forecast, revision, fp, &previousForecastRecorded)
		}
		if forecast.SupersedesForecastID != nil {
			if _, exists := localForecasts[*forecast.SupersedesForecastID]; !exists {
				v.add("semantic.supersedes", fp+"/supersedes_forecast_id", "must reference an earlier forecast for this question")
			}
		}
		localForecasts[forecast.ID] = struct{}{}
		if forecast.Provenance != nil {
			v.validateProvenance(forecast.Provenance, fp+"/provenance")
		}
		v.validateLifecycleEvents(forecast, fp+"/lifecycle_events")
		v.validateActivityCheckpoints(question, forecast, fp+"/activity_checkpoints")
		v.validateIntegrity(forecast.Integrity, fp+"/integrity")
		v.validateReveal(question.ID, forecast, fp)
	}
	v.validateResolution(question, revisions, pointer)
	return forecastIDs
}

func (v *semanticValidator) validateForecastAgainstRevision(forecast *ledger.Forecast, revision *ledger.QuestionRevision, pointer string, previousRecorded *time.Time) {
	forecasted, forecastedErr := ledger.ParseTimestamp(forecast.ForecastedAt)
	recorded, recordedErr := ledger.ParseTimestamp(forecast.RecordedAt)
	effective, effectiveErr := ledger.ParseTimestamp(revision.EffectiveAt)
	if forecastedErr == nil && recordedErr == nil {
		if recorded.Before(forecasted) {
			v.add("semantic.forecast_chronology", pointer+"/recorded_at", "recorded_at must not precede forecasted_at")
		}
		if !previousRecorded.IsZero() && recorded.Before(*previousRecorded) {
			v.add("semantic.forecast_order", pointer, "forecasts must be append-only ordered by recorded_at")
		}
		*previousRecorded = recorded
	}
	if forecastedErr == nil && effectiveErr == nil && forecasted.Before(effective) {
		v.add("semantic.forecast_revision_chronology", pointer+"/forecasted_at", "must not precede the bound revision effective_at")
	}
	if forecastedErr == nil && revision.ForecastingOpensAt != nil {
		opens, err := ledger.ParseTimestamp(*revision.ForecastingOpensAt)
		if err == nil && forecasted.Before(opens) {
			v.add("semantic.forecasting_open", pointer+"/forecasted_at", "must not precede forecasting_opens_at")
		}
	}
	if forecast.Representations != nil {
		kinds := make([]ledger.RepresentationKind, 0, len(*forecast.Representations))
		for index, representation := range *forecast.Representations {
			kind, _ := representation.Kind()
			kinds = append(kinds, kind)
			v.validateRepresentation(representation, revision.Domain, fmt.Sprintf("%s/representations/%d", pointer, index))
		}
		v.uniqueRepresentationKinds(kinds, pointer+"/representations")
	}
}

func (v *semanticValidator) validateDomain(domain ledger.Domain, pointer string) {
	if err := ledger.ValidateDomainScalars(domain); err != nil {
		v.add("semantic.domain_scalar", pointer, err.Error())
	}
	switch {
	case domain.Categorical != nil:
		v.validateOptionSet(domain.Categorical.OptionSet, pointer+"/option_set")
	case domain.Ordinal != nil:
		v.validateOptionSet(domain.Ordinal.OptionSet, pointer+"/option_set")
	case domain.Numeric != nil && domain.Numeric.BinSets != nil:
		validateBins(v, ledger.OutcomeNumeric, *domain.Numeric.BinSets, pointer+"/bin_sets")
	case domain.Date != nil && domain.Date.BinSets != nil:
		validateBins(v, ledger.OutcomeDate, *domain.Date.BinSets, pointer+"/bin_sets")
	case domain.Datetime != nil && domain.Datetime.BinSets != nil:
		validateBins(v, ledger.OutcomeDatetime, *domain.Datetime.BinSets, pointer+"/bin_sets")
	}
}

func (v *semanticValidator) validateOptionSet(optionSet ledger.OptionSet, pointer string) {
	ids := make([]ledger.Slug, len(optionSet.Options))
	for index, option := range optionSet.Options {
		ids[index] = option.ID
	}
	v.uniqueSlugs(ids, pointer+"/options", "semantic.duplicate_option_id")
}

func validateBins[T ~string](v *semanticValidator, kind ledger.OutcomeKind, sets []ledger.BinSet[T], pointer string) {
	seenSets := make(map[string]struct{}, len(sets))
	for setIndex, set := range sets {
		sp := fmt.Sprintf("%s/%d", pointer, setIndex)
		key := fmt.Sprintf("%s\x00%d", set.ID, set.Version)
		if _, exists := seenSets[key]; exists {
			v.add("semantic.duplicate_bin_set", sp, "bin-set id and version are duplicated")
		}
		seenSets[key] = struct{}{}
		ids := make([]ledger.Slug, len(set.Bins))
		for binIndex, bin := range set.Bins {
			ids[binIndex] = bin.ID
			bp := fmt.Sprintf("%s/bins/%d", sp, binIndex)
			comparison, err := ledger.CompareScalarStrings(kind, string(bin.Lower), string(bin.Upper))
			if err != nil || comparison >= 0 {
				v.add("semantic.bin_bounds", bp, "bin lower must be strictly less than upper")
			}
			if binIndex > 0 {
				previous := set.Bins[binIndex-1]
				comparison, err = ledger.CompareScalarStrings(kind, string(previous.Upper), string(bin.Lower))
				if err != nil || comparison != 0 {
					v.add("semantic.bin_continuity", bp, "bins must have neither gaps nor overlaps")
				}
				if previous.UpperInclusive == bin.LowerInclusive {
					v.add("semantic.bin_inclusivity", bp, "exactly one adjacent bin must include their shared boundary")
				}
			}
		}
		v.uniqueSlugs(ids, sp+"/bins", "semantic.duplicate_bin_id")
	}
}

func (v *semanticValidator) validateRepresentation(representation ledger.ForecastRepresentation, domain ledger.Domain, pointer string) {
	domainKind, ok := domain.Kind()
	if !ok {
		return
	}
	representationKind, ok := representation.Kind()
	if !ok {
		return
	}
	allowed := map[ledger.OutcomeKind]map[ledger.RepresentationKind]bool{
		ledger.OutcomeBinary:      {ledger.RepresentationProbability: true, ledger.RepresentationPoint: true},
		ledger.OutcomeCategorical: {ledger.RepresentationPMF: true, ledger.RepresentationPoint: true},
		ledger.OutcomeOrdinal:     {ledger.RepresentationPMF: true, ledger.RepresentationPoint: true},
		ledger.OutcomeNumeric:     {ledger.RepresentationBinnedPMF: true, ledger.RepresentationQuantiles: true, ledger.RepresentationCDF: true, ledger.RepresentationPoint: true, ledger.RepresentationCredibleIntervals: true},
		ledger.OutcomeDate:        {ledger.RepresentationBinnedPMF: true, ledger.RepresentationQuantiles: true, ledger.RepresentationCDF: true, ledger.RepresentationPoint: true, ledger.RepresentationCredibleIntervals: true},
		ledger.OutcomeDatetime:    {ledger.RepresentationBinnedPMF: true, ledger.RepresentationQuantiles: true, ledger.RepresentationCDF: true, ledger.RepresentationPoint: true, ledger.RepresentationCredibleIntervals: true},
	}
	if !allowed[domainKind][representationKind] {
		v.add("semantic.representation_domain", pointer, fmt.Sprintf("%q is incompatible with %q outcome space", representationKind, domainKind))
		return
	}
	switch {
	case representation.Probability != nil:
		if representation.Probability.Outcome != true {
			v.add("semantic.probability_outcome", pointer+"/outcome", "probability outcome must be true")
		}
		v.parseProbability(representation.Probability.Probability, pointer+"/probability", false)
	case representation.PMF != nil:
		v.validatePMF(representation.PMF, domain, pointer)
	case representation.BinnedPMF != nil:
		v.validateBinnedPMF(representation.BinnedPMF, domain, pointer)
	case representation.Quantiles != nil:
		v.validateQuantiles(representation.Quantiles, domain, pointer)
	case representation.CDF != nil:
		v.validateCDF(representation.CDF, domain, pointer)
	case representation.Point != nil:
		v.validatePoint(representation.Point, domain, pointer)
	case representation.CredibleIntervals != nil:
		v.validateCredibleIntervals(representation.CredibleIntervals, domain, pointer)
	}
}

func (v *semanticValidator) validatePMF(value *ledger.PMFRepresentation, domain ledger.Domain, pointer string) {
	var optionSet *ledger.OptionSet
	if domain.Categorical != nil {
		optionSet = &domain.Categorical.OptionSet
	} else if domain.Ordinal != nil {
		optionSet = &domain.Ordinal.OptionSet
	}
	if optionSet == nil {
		return
	}
	if value.OptionSetRef.ID != optionSet.ID || value.OptionSetRef.Version != optionSet.Version {
		v.add("semantic.option_set_ref", pointer+"/option_set_ref", "must bind the exact question option-set version")
	}
	expected := make(map[ledger.Slug]struct{}, len(optionSet.Options))
	for _, option := range optionSet.Options {
		expected[option.ID] = struct{}{}
	}
	actual := make(map[ledger.Slug]struct{}, len(value.Entries))
	probabilities := make([]ledger.Probability, 0, len(value.Entries))
	for index, entry := range value.Entries {
		if _, exists := actual[entry.OptionID]; exists {
			v.add("semantic.duplicate_pmf_option", fmt.Sprintf("%s/entries/%d/option_id", pointer, index), "option appears more than once")
		}
		actual[entry.OptionID] = struct{}{}
		probabilities = append(probabilities, entry.Probability)
	}
	if !sameSlugSet(actual, expected) {
		v.add("semantic.pmf_coverage", pointer+"/entries", "must cover every option exactly once")
	}
	if total, ok := v.sumProbabilities(probabilities, pointer+"/entries"); ok && total.Cmp(exact.One()) != 0 {
		v.add("semantic.probability_sum", pointer+"/entries", "probabilities sum to "+total.String()+", not 1")
	}
}

func (v *semanticValidator) validateBinnedPMF(value *ledger.BinnedPMFRepresentation, domain ledger.Domain, pointer string) {
	binIDs, firstLower, lastUpper, lowerBound, upperBound, found := findBinSet(domain, value.BinSetRef)
	if !found {
		v.add("semantic.bin_set_ref", pointer+"/bin_set_ref", "must bind an existing exact bin-set version")
		return
	}
	expected := make(map[ledger.Slug]struct{}, len(binIDs))
	for _, id := range binIDs {
		expected[id] = struct{}{}
	}
	actual := make(map[ledger.Slug]struct{}, len(value.Entries))
	probabilities := make([]ledger.Probability, 0, len(value.Entries)+2)
	for index, entry := range value.Entries {
		if _, exists := actual[entry.BinID]; exists {
			v.add("semantic.duplicate_binned_pmf_bin", fmt.Sprintf("%s/entries/%d/bin_id", pointer, index), "bin appears more than once")
		}
		actual[entry.BinID] = struct{}{}
		probabilities = append(probabilities, entry.Probability)
	}
	if !sameSlugSet(actual, expected) {
		v.add("semantic.binned_pmf_coverage", pointer+"/entries", "must cover every bin exactly once")
	}
	probabilities = append(probabilities, value.LeftTailProbability, value.RightTailProbability)
	if total, ok := v.sumProbabilities(probabilities, pointer); ok && total.Cmp(exact.One()) != 0 {
		v.add("semantic.binned_probability_sum", pointer, "bin and tail probabilities sum to "+total.String()+", not 1")
	}
	if lowerBound != nil && firstLower == *lowerBound {
		if parsed := v.parseProbability(value.LeftTailProbability, pointer+"/left_tail_probability", false); parsed != nil && parsed.Cmp(exact.Zero()) != 0 {
			v.add("semantic.left_tail_at_bound", pointer+"/left_tail_probability", "must be 0 when bins start at the domain bound")
		}
	}
	if upperBound != nil && lastUpper == *upperBound {
		if parsed := v.parseProbability(value.RightTailProbability, pointer+"/right_tail_probability", false); parsed != nil && parsed.Cmp(exact.Zero()) != 0 {
			v.add("semantic.right_tail_at_bound", pointer+"/right_tail_probability", "must be 0 when bins end at the domain bound")
		}
	}
}

func (v *semanticValidator) validateQuantiles(value *ledger.QuantilesRepresentation, domain ledger.Domain, pointer string) {
	domainKind, _ := domain.Kind()
	var previousLevel *exact.Decimal
	var previousValue *ledger.ScalarValue
	for index, point := range value.Points {
		pp := fmt.Sprintf("%s/points/%d", pointer, index)
		level := v.parseOpenProbability(point.Level, pp+"/level")
		if level != nil && previousLevel != nil && level.Cmp(*previousLevel) <= 0 {
			v.add("semantic.quantile_level_order", pointer+"/points", "quantile levels must be strictly increasing")
		}
		if level != nil {
			copy := *level
			previousLevel = &copy
		}
		if ok, reason := ledger.ScalarInDomain(point.Value, domain); !ok {
			v.add("semantic.quantile_domain", pp+"/value", reason)
		} else if previousValue != nil {
			comparison, err := ledger.CompareScalars(domainKind, *previousValue, point.Value)
			if err == nil && comparison > 0 {
				v.add("semantic.quantile_value_order", pointer+"/points", "quantile values must be non-decreasing")
			}
		}
		copy := point.Value
		previousValue = &copy
	}
}

func (v *semanticValidator) validateCDF(value *ledger.CDFRepresentation, domain ledger.Domain, pointer string) {
	domainKind, _ := domain.Kind()
	var previousValue *ledger.ScalarValue
	var previousProbability *exact.Decimal
	probabilities := make([]*exact.Decimal, len(value.Points))
	for index, point := range value.Points {
		pp := fmt.Sprintf("%s/points/%d", pointer, index)
		if ok, reason := ledger.ScalarInDomain(point.Value, domain); !ok {
			v.add("semantic.cdf_domain", pp+"/value", reason)
		} else if previousValue != nil {
			comparison, err := ledger.CompareScalars(domainKind, *previousValue, point.Value)
			if err == nil && comparison >= 0 {
				v.add("semantic.cdf_value_order", pointer+"/points", "CDF values must be strictly increasing")
			}
		}
		copyValue := point.Value
		previousValue = &copyValue
		probability := v.parseProbability(point.Probability, pp+"/probability", false)
		probabilities[index] = probability
		if probability != nil && previousProbability != nil && probability.Cmp(*previousProbability) < 0 {
			v.add("semantic.cdf_probability_order", pointer+"/points", "CDF probabilities must be non-decreasing")
		}
		if probability != nil {
			copyProbability := *probability
			previousProbability = &copyProbability
		}
	}
	left := v.parseProbability(value.LeftTailProbability, pointer+"/left_tail_probability", false)
	right := v.parseProbability(value.RightTailProbability, pointer+"/right_tail_probability", false)
	if len(probabilities) > 0 && left != nil && probabilities[0] != nil && left.Cmp(*probabilities[0]) > 0 {
		v.add("semantic.cdf_left_tail", pointer+"/left_tail_probability", "must not exceed the first CDF probability")
	}
	if len(probabilities) > 0 && right != nil && probabilities[len(probabilities)-1] != nil {
		if probabilities[len(probabilities)-1].Add(*right).Cmp(exact.One()) != 0 {
			v.add("semantic.cdf_right_tail", pointer, "last CDF probability plus right tail probability must equal 1")
		}
	}
}

func (v *semanticValidator) validatePoint(value *ledger.PointRepresentation, domain ledger.Domain, pointer string) {
	domainKind, _ := domain.Kind()
	if value.Statistic == ledger.StatisticMean && domainKind != ledger.OutcomeNumeric && domainKind != ledger.OutcomeDate && domainKind != ledger.OutcomeDatetime {
		v.add("semantic.point_statistic", pointer+"/statistic", "mean is only defined for numeric, date, or datetime outcomes")
	}
	if value.Statistic == ledger.StatisticMode && domainKind == ledger.OutcomeBinary && value.Value.Boolean == nil {
		v.add("semantic.binary_mode", pointer+"/value", "binary mode must be boolean")
	}
	if ok, reason := ledger.ScalarInDomain(value.Value, domain); !ok {
		v.add("semantic.point_domain", pointer+"/value", reason)
	}
}

func (v *semanticValidator) validateCredibleIntervals(value *ledger.CredibleIntervalsRepresentation, domain ledger.Domain, pointer string) {
	domainKind, _ := domain.Kind()
	seen := make(map[ledger.OpenProbability]struct{}, len(value.Intervals))
	for index, interval := range value.Intervals {
		ip := fmt.Sprintf("%s/intervals/%d", pointer, index)
		if _, exists := seen[interval.Coverage]; exists {
			v.add("semantic.duplicate_interval_coverage", ip+"/coverage", "coverage is duplicated")
		}
		seen[interval.Coverage] = struct{}{}
		v.parseOpenProbability(interval.Coverage, ip+"/coverage")
		lowerOK, lowerReason := ledger.ScalarInDomain(interval.Lower, domain)
		upperOK, upperReason := ledger.ScalarInDomain(interval.Upper, domain)
		if !lowerOK {
			v.add("semantic.interval_lower_domain", ip+"/lower", lowerReason)
		}
		if !upperOK {
			v.add("semantic.interval_upper_domain", ip+"/upper", upperReason)
		}
		if lowerOK && upperOK {
			comparison, err := ledger.CompareScalars(domainKind, interval.Lower, interval.Upper)
			if err == nil && comparison > 0 {
				v.add("semantic.interval_order", ip, "lower must not exceed upper")
			}
		}
	}
}

func (v *semanticValidator) validateLifecycleEvents(forecast *ledger.Forecast, pointer string) {
	if forecast.LifecycleEvents == nil {
		return
	}
	ids := make([]ledger.Slug, len(*forecast.LifecycleEvents))
	active := true
	var previousEffective, previousRecorded time.Time
	forecasted, forecastedErr := ledger.ParseTimestamp(forecast.ForecastedAt)
	forecastRecorded, forecastRecordedErr := ledger.ParseTimestamp(forecast.RecordedAt)
	for index := range *forecast.LifecycleEvents {
		event := &(*forecast.LifecycleEvents)[index]
		ids[index] = event.ID
		ep := fmt.Sprintf("%s/%d", pointer, index)
		effective, effectiveErr := ledger.ParseTimestamp(event.EffectiveAt)
		recorded, recordedErr := ledger.ParseTimestamp(event.RecordedAt)
		if effectiveErr == nil && recordedErr == nil {
			if forecastedErr == nil && effective.Before(forecasted) {
				v.add("semantic.lifecycle_forecast_chronology", ep+"/effective_at", "must not precede forecast.forecasted_at")
			}
			if recorded.Before(effective) {
				v.add("semantic.lifecycle_chronology", ep+"/recorded_at", "must not precede effective_at")
			}
			if forecastRecordedErr == nil && recorded.Before(forecastRecorded) {
				v.add("semantic.lifecycle_forecast_chronology", ep+"/recorded_at", "must not precede forecast.recorded_at")
			}
			if !previousEffective.IsZero() && effective.Before(previousEffective) {
				v.add("semantic.lifecycle_effective_order", ep, "events must be ordered by effective_at")
			}
			if !previousRecorded.IsZero() && recorded.Before(previousRecorded) {
				v.add("semantic.lifecycle_recorded_order", ep, "events must be append-only ordered by recorded_at")
			}
			previousEffective, previousRecorded = effective, recorded
		}
		switch event.Type {
		case ledger.LifecycleWithdrawn, ledger.LifecycleExpired:
			if !active {
				v.add("semantic.lifecycle_transition", ep+"/type", "cannot deactivate an inactive forecast")
			}
			active = false
		case ledger.LifecycleReaffirmed:
			if active {
				v.add("semantic.lifecycle_transition", ep+"/type", "cannot reaffirm an active forecast")
			}
			active = true
		}
		if event.Provenance != nil {
			v.validateProvenance(event.Provenance, ep+"/provenance")
		}
	}
	v.uniqueSlugs(ids, pointer, "semantic.duplicate_lifecycle_event_id")
}

func (v *semanticValidator) validateActivityCheckpoints(question *ledger.Question, forecast *ledger.Forecast, pointer string) {
	if forecast.ActivityCheckpoints == nil {
		return
	}
	eventIndexes := make(map[ledger.Slug]int)
	if forecast.LifecycleEvents != nil {
		for index, event := range *forecast.LifecycleEvents {
			eventIndexes[event.ID] = index
		}
	}
	ids := make([]ledger.Slug, len(*forecast.ActivityCheckpoints))
	heads := make([]ledger.Slug, len(*forecast.ActivityCheckpoints))
	previousHead := -1
	var previousRecorded time.Time
	for index := range *forecast.ActivityCheckpoints {
		checkpoint := &(*forecast.ActivityCheckpoints)[index]
		ids[index], heads[index] = checkpoint.ID, checkpoint.HeadEventID
		cp := fmt.Sprintf("%s/%d", pointer, index)
		headIndex, exists := eventIndexes[checkpoint.HeadEventID]
		if !exists {
			v.add("semantic.activity_checkpoint_head", cp+"/head_event_id", "must reference a lifecycle event")
		} else {
			if previousHead >= 0 && headIndex <= previousHead {
				v.add("semantic.activity_checkpoint_order", cp+"/head_event_id", "checkpoints must cover strictly increasing lifecycle prefixes")
			}
			checkpointRecorded, checkpointErr := ledger.ParseTimestamp(checkpoint.RecordedAt)
			headRecorded, headErr := ledger.ParseTimestamp((*forecast.LifecycleEvents)[headIndex].RecordedAt)
			if checkpointErr == nil {
				if headErr == nil && checkpointRecorded.Before(headRecorded) {
					v.add("semantic.activity_checkpoint_chronology", cp+"/recorded_at", "must not precede the covered lifecycle head recorded_at")
				}
				if !previousRecorded.IsZero() && checkpointRecorded.Before(previousRecorded) {
					v.add("semantic.activity_checkpoint_order", cp+"/recorded_at", "checkpoints must be append-only ordered by recorded_at")
				}
				previousRecorded = checkpointRecorded
			}
			previousHead = headIndex
		}
		v.validateLifecycleIntegrity(question, forecast, checkpoint.Integrity, checkpoint.HeadEventID, cp+"/integrity")
	}
	v.uniqueSlugs(ids, pointer, "semantic.duplicate_activity_checkpoint_id")
	v.uniqueSlugs(heads, pointer, "semantic.duplicate_activity_checkpoint_head")
}

func (v *semanticValidator) validateLifecycleIntegrity(question *ledger.Question, forecast *ledger.Forecast, value ledger.LifecycleIntegrity, headEventID ledger.Slug, pointer string) {
	var target *ledger.LifecycleTarget
	switch {
	case value.Pending != nil:
		target = &value.Pending.Target
	case value.Verified != nil:
		target = &value.Verified.Target
	case value.Failed != nil:
		target = value.Failed.Target
	}
	if target == nil {
		return
	}
	if target.Scope != targetbytes.LifecycleSchema {
		v.add("semantic.activity_target_scope", pointer+"/target/scope", "must be "+targetbytes.LifecycleSchema)
	}
	if target.Canonicalization != targetbytes.Canonicalization {
		v.add("semantic.activity_target_canonicalization", pointer+"/target/canonicalization", "must be "+targetbytes.Canonicalization)
	}
	if _, expectedDigest, err := targetbytes.Lifecycle(*question, *forecast, headEventID); err == nil && (target.Digest.Algorithm != "sha-256" || string(target.Digest.Value) != expectedDigest) {
		v.add("semantic.activity_target_digest", pointer+"/target/digest/value", "does not match the canonical forecast-lifecycle/v1 target")
	}
	v.validateArtifact(target.ArtifactPath, target.Digest, pointer+"/target")
}

func (v *semanticValidator) validateProvenance(value *ledger.Provenance, pointer string) {
	if _, exists := v.platforms[value.Platform]; !exists {
		v.add("semantic.unknown_platform", pointer+"/platform", "provenance references an unknown platform")
	}
	if value.Snapshot != nil {
		v.validateArtifact(value.Snapshot.ArtifactPath, value.Snapshot.Digest, pointer+"/snapshot")
	}
}

func (v *semanticValidator) validateIntegrity(value ledger.Integrity, pointer string) {
	switch {
	case value.Pending != nil:
		v.validateArtifact(value.Pending.Target.ArtifactPath, value.Pending.Target.Digest, pointer+"/target")
	case value.Verified != nil:
		v.validateArtifact(value.Verified.Target.ArtifactPath, value.Verified.Target.Digest, pointer+"/target")
	}
}

func (v *semanticValidator) validateReveal(questionID ledger.Slug, forecast *ledger.Forecast, pointer string) {
	if forecast.Visibility != ledger.VisibilityRevealed || forecast.Commitment == nil || forecast.Commitment.Revealed == nil {
		return
	}
	payload, err := forecastcrypto.Reveal(questionID, forecast.QuestionRevisionID, forecast.ID, *forecast.Commitment.Revealed)
	if err != nil {
		v.add("semantic.reveal_verification", pointer+"/commitment", "revealed commitment verification failed")
		return
	}
	checks := []struct {
		name  string
		left  any
		right any
	}{
		{"representations", dereferenceRepresentations(forecast.Representations), payload.Bundle.Representations},
		{"rationale", forecast.Rationale, payload.Bundle.Rationale},
		{"key_factors", forecast.KeyFactors, payload.Bundle.KeyFactors},
		{"comment", forecast.Comment, payload.Bundle.Comment},
	}
	for _, check := range checks {
		if !reflect.DeepEqual(check.left, check.right) {
			v.add("semantic.revealed_mirror", pointer+"/"+check.name, "does not match the decrypted sealed bundle")
		}
	}
}

func (v *semanticValidator) validateArtifact(path ledger.RelativePath, digest ledger.Digest, pointer string) {
	if v.artifacts == nil {
		return
	}
	name := string(path)
	if !fs.ValidPath(name) || name == "." {
		v.add("semantic.artifact_path", pointer+"/artifact_path", "artifact path is not a confined relative path")
		return
	}
	data, err := fs.ReadFile(v.artifacts, name)
	if err != nil {
		v.add("semantic.artifact_missing", pointer+"/artifact_path", "artifact cannot be read")
		return
	}
	actual := sha256.Sum256(data)
	expected, err := hex.DecodeString(string(digest.Value))
	if err != nil || len(expected) != sha256.Size || !equalBytes(actual[:], expected) {
		v.add("semantic.artifact_digest", pointer+"/digest/value", "artifact digest does not match")
	}
}

func (v *semanticValidator) validateResolution(question *ledger.Question, revisions map[ledger.Slug]*ledger.QuestionRevision, pointer string) {
	terminal := question.Status == ledger.QuestionResolved || question.Status == ledger.QuestionAmbiguous || question.Status == ledger.QuestionVoid || question.Status == ledger.QuestionDisputed || question.Status == ledger.QuestionNotApplicable
	if !terminal {
		if question.Resolution != nil {
			v.add("semantic.resolution_state", pointer+"/resolution", "nonterminal question must not have a resolution")
		}
		return
	}
	if question.Resolution == nil {
		v.add("semantic.resolution_state", pointer+"/resolution", "terminal question must have a resolution")
		return
	}
	status := resolutionStatus(*question.Resolution)
	if status != ledger.ResolutionStatus(question.Status) {
		v.add("semantic.resolution_status", pointer+"/resolution/status", "resolution status must match question.status")
	}
	if question.Resolution.Resolved == nil {
		return
	}
	resolution := question.Resolution.Resolved
	revision := revisions[resolution.QuestionRevisionID]
	if revision == nil {
		v.add("semantic.resolution_revision", pointer+"/resolution/question_revision_id", "unknown question revision")
	} else if ok, reason := ledger.ScalarInDomain(resolution.Outcome, revision.Domain); !ok {
		v.add("semantic.resolution_outcome", pointer+"/resolution/outcome", reason)
	}
	known, knownErr := ledger.ParseTimestamp(resolution.OutcomeKnownAt)
	recorded, recordedErr := ledger.ParseTimestamp(resolution.RecordedAt)
	if knownErr == nil && recordedErr == nil && recorded.Before(known) {
		v.add("semantic.resolution_chronology", pointer+"/resolution/recorded_at", "must not precede outcome_known_at")
	}
	if knownErr == nil {
		for index := range question.Forecasts {
			forecast := &question.Forecasts[index]
			if forecast.Integrity.Verified == nil {
				continue
			}
			if !hasVerifiedTimestampBefore(forecast.Integrity.Verified.Timestamps, known) {
				v.add("semantic.timestamp_chronology", fmt.Sprintf("%s/forecasts/%d/integrity/timestamps", pointer, index), "must contain a verified RFC 3161 timestamp predating the known outcome")
			}
		}
	}
}

func (v *semanticValidator) validateRelationships() {
	groups := make(map[ledger.Slug]struct{})
	if v.model.Groups != nil {
		ids := make([]ledger.Slug, len(*v.model.Groups))
		for index, group := range *v.model.Groups {
			ids[index] = group.ID
			groups[group.ID] = struct{}{}
		}
		v.uniqueSlugs(ids, "/groups", "semantic.duplicate_group_id")
	}
	if v.model.Relationships == nil {
		return
	}
	ids := make([]ledger.Slug, len(*v.model.Relationships))
	memberships := make(map[string]int)
	graph := make(map[ledger.Slug][]ledger.Slug, len(v.questions))
	for questionID := range v.questions {
		graph[questionID] = nil
	}
	for index, relationship := range *v.model.Relationships {
		pointer := fmt.Sprintf("/relationships/%d", index)
		switch {
		case relationship.GroupMembership != nil:
			value := relationship.GroupMembership
			ids[index] = value.ID
			if _, exists := groups[value.GroupID]; !exists {
				v.add("semantic.relationship_group", pointer+"/group_id", "unknown group")
			}
			if _, exists := v.questions[value.QuestionID]; !exists {
				v.add("semantic.relationship_question", pointer+"/question_id", "unknown question")
			}
			key := string(value.GroupID) + "\x00" + string(value.QuestionID)
			if _, exists := memberships[key]; exists {
				v.add("semantic.duplicate_membership", pointer, "group membership is duplicated")
			}
			memberships[key] = index
		case relationship.Conditional != nil:
			value := relationship.Conditional
			ids[index] = value.ID
			parent := v.questions[value.ParentQuestionID]
			child := v.questions[value.ChildQuestionID]
			if parent == nil {
				v.add("semantic.relationship_parent", pointer+"/parent_question_id", "unknown question")
			}
			if child == nil {
				v.add("semantic.relationship_child", pointer+"/child_question_id", "unknown question")
			}
			if parent != nil {
				revision, exists := parent.Revision(value.ParentQuestionRevisionID)
				if !exists {
					v.add("semantic.relationship_revision", pointer+"/parent_question_revision_id", "unknown parent revision")
				} else if ok, reason := ledger.ScalarInDomain(value.ParentOutcome, revision.Domain); !ok {
					v.add("semantic.relationship_outcome", pointer+"/parent_outcome", reason)
				}
			}
			if parent != nil && child != nil {
				graph[parent.ID] = append(graph[parent.ID], child.ID)
			}
		}
	}
	v.uniqueSlugs(ids, "/relationships", "semantic.duplicate_relationship_id")
	if conditionalCycle(graph) {
		v.add("semantic.relationship_cycle", "/relationships", "conditional relationships must be acyclic")
	}
}

func (v *semanticValidator) validateNotApplicableResolutions() {
	relationships := make(map[ledger.Slug]*ledger.ConditionalRelationship)
	if v.model.Relationships != nil {
		for index := range *v.model.Relationships {
			if value := (*v.model.Relationships)[index].Conditional; value != nil {
				relationships[value.ID] = value
			}
		}
	}
	for index := range v.model.Questions {
		question := &v.model.Questions[index]
		if question.Resolution == nil || question.Resolution.NotApplicable == nil {
			continue
		}
		pointer := fmt.Sprintf("/questions/%d/resolution/relationship_id", index)
		resolution := question.Resolution.NotApplicable
		relationship := relationships[resolution.RelationshipID]
		if relationship == nil {
			v.add("semantic.not_applicable_relationship", pointer, "must reference a conditional relationship")
			continue
		}
		if relationship.ChildQuestionID != question.ID {
			v.add("semantic.not_applicable_child", pointer, "must reference a conditional relationship for this child question")
			continue
		}
		parent := v.questions[relationship.ParentQuestionID]
		if parent != nil && parent.Resolution != nil && parent.Resolution.Resolved != nil {
			outcome := parent.Resolution.Resolved.Outcome
			if scalarEqual(outcome, relationship.ParentOutcome) {
				v.add("semantic.not_applicable_satisfied", pointer, "cannot be not_applicable when the parent outcome satisfies the condition")
			}
		}
	}
}

func (v *semanticValidator) parseProbability(value ledger.Probability, pointer string, open bool) *exact.Decimal {
	var (
		parsed exact.Decimal
		err    error
	)
	if open {
		parsed, err = exact.ParseOpenProbability(string(value))
	} else {
		parsed, err = exact.ParseProbability(string(value))
	}
	if err != nil {
		v.add("semantic.probability", pointer, err.Error())
		return nil
	}
	return &parsed
}

func (v *semanticValidator) parseOpenProbability(value ledger.OpenProbability, pointer string) *exact.Decimal {
	parsed, err := exact.ParseOpenProbability(string(value))
	if err != nil {
		v.add("semantic.open_probability", pointer, err.Error())
		return nil
	}
	return &parsed
}

func (v *semanticValidator) sumProbabilities(values []ledger.Probability, pointer string) (exact.Decimal, bool) {
	total := exact.Zero()
	valid := true
	for index, value := range values {
		parsed := v.parseProbability(value, fmt.Sprintf("%s/%d", pointer, index), false)
		if parsed == nil {
			valid = false
			continue
		}
		total = total.Add(*parsed)
	}
	return total, valid
}

func (v *semanticValidator) uniqueSlugs(values []ledger.Slug, pointer, code string) {
	seen := make(map[ledger.Slug]struct{}, len(values))
	for index, value := range values {
		if _, exists := seen[value]; exists {
			v.add(code, fmt.Sprintf("%s/%d", pointer, index), "ID is duplicated")
		}
		seen[value] = struct{}{}
	}
}

func (v *semanticValidator) uniqueRepresentationKinds(values []ledger.RepresentationKind, pointer string) {
	seen := make(map[ledger.RepresentationKind]struct{}, len(values))
	for index, value := range values {
		if _, exists := seen[value]; exists {
			v.add("semantic.duplicate_representation_kind", fmt.Sprintf("%s/%d", pointer, index), "representation kind is duplicated")
		}
		seen[value] = struct{}{}
	}
}

func (v *semanticValidator) add(code, pointer, message string) {
	v.issues = append(v.issues, SemanticIssue{Layer: "semantic", Code: code, Pointer: pointer, Message: message})
}

func findBinSet(domain ledger.Domain, reference ledger.BinSetRef) ([]ledger.Slug, string, string, *string, *string, bool) {
	switch {
	case domain.Numeric != nil:
		sets := domain.Numeric.BinSets
		if sets == nil {
			return nil, "", "", nil, nil, false
		}
		for _, set := range *sets {
			if set.ID == reference.ID && set.Version == reference.Version {
				ids, first, last := binSummary(set.Bins)
				lower, upper := numericBoundStrings(domain.Numeric.Bounds)
				return ids, first, last, lower, upper, true
			}
		}
	case domain.Date != nil:
		sets := domain.Date.BinSets
		if sets == nil {
			return nil, "", "", nil, nil, false
		}
		for _, set := range *sets {
			if set.ID == reference.ID && set.Version == reference.Version {
				ids, first, last := binSummary(set.Bins)
				lower, upper := dateBoundStrings(domain.Date.Bounds)
				return ids, first, last, lower, upper, true
			}
		}
	case domain.Datetime != nil:
		sets := domain.Datetime.BinSets
		if sets == nil {
			return nil, "", "", nil, nil, false
		}
		for _, set := range *sets {
			if set.ID == reference.ID && set.Version == reference.Version {
				ids, first, last := binSummary(set.Bins)
				lower, upper := timestampBoundStrings(domain.Datetime.Bounds)
				return ids, first, last, lower, upper, true
			}
		}
	}
	return nil, "", "", nil, nil, false
}

func binSummary[T ~string](bins []ledger.Bin[T]) ([]ledger.Slug, string, string) {
	if len(bins) == 0 {
		return nil, "", ""
	}
	ids := make([]ledger.Slug, len(bins))
	for index, bin := range bins {
		ids[index] = bin.ID
	}
	return ids, string(bins[0].Lower), string(bins[len(bins)-1].Upper)
}

func numericBoundStrings(bounds *ledger.Bounds[ledger.Decimal]) (*string, *string) {
	if bounds == nil {
		return nil, nil
	}
	var lower, upper *string
	if bounds.Lower != nil {
		value := string(bounds.Lower.Value)
		lower = &value
	}
	if bounds.Upper != nil {
		value := string(bounds.Upper.Value)
		upper = &value
	}
	return lower, upper
}

func dateBoundStrings(bounds *ledger.Bounds[ledger.Date]) (*string, *string) {
	if bounds == nil {
		return nil, nil
	}
	var lower, upper *string
	if bounds.Lower != nil {
		value := string(bounds.Lower.Value)
		lower = &value
	}
	if bounds.Upper != nil {
		value := string(bounds.Upper.Value)
		upper = &value
	}
	return lower, upper
}

func timestampBoundStrings(bounds *ledger.Bounds[ledger.Timestamp]) (*string, *string) {
	if bounds == nil {
		return nil, nil
	}
	var lower, upper *string
	if bounds.Lower != nil {
		value := string(bounds.Lower.Value)
		lower = &value
	}
	if bounds.Upper != nil {
		value := string(bounds.Upper.Value)
		upper = &value
	}
	return lower, upper
}

func sameSlugSet(left, right map[ledger.Slug]struct{}) bool {
	if len(left) != len(right) {
		return false
	}
	for value := range left {
		if _, exists := right[value]; !exists {
			return false
		}
	}
	return true
}

func resolutionStatus(value ledger.Resolution) ledger.ResolutionStatus {
	switch {
	case value.Resolved != nil:
		return value.Resolved.Status
	case value.Unresolved != nil:
		return value.Unresolved.Status
	case value.NotApplicable != nil:
		return value.NotApplicable.Status
	default:
		return ""
	}
}

func hasVerifiedTimestampBefore(values []ledger.RFC3161Timestamp, known time.Time) bool {
	for _, value := range values {
		if value.State != ledger.RFC3161Verified || value.GenTime == nil {
			continue
		}
		instant, err := ledger.ParseTimestamp(*value.GenTime)
		if err == nil && instant.Before(known) {
			return true
		}
	}
	return false
}

func conditionalCycle(graph map[ledger.Slug][]ledger.Slug) bool {
	visiting := make(map[ledger.Slug]bool)
	visited := make(map[ledger.Slug]bool)
	var visit func(ledger.Slug) bool
	visit = func(node ledger.Slug) bool {
		if visiting[node] {
			return true
		}
		if visited[node] {
			return false
		}
		visiting[node] = true
		for _, child := range graph[node] {
			if visit(child) {
				return true
			}
		}
		visiting[node] = false
		visited[node] = true
		return false
	}
	for node := range graph {
		if !visited[node] && visit(node) {
			return true
		}
	}
	return false
}

func scalarEqual(left, right ledger.ScalarValue) bool {
	if left.Boolean != nil && right.Boolean != nil {
		return *left.Boolean == *right.Boolean
	}
	if left.String != nil && right.String != nil {
		return *left.String == *right.String
	}
	return false
}

func equalBytes(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	var difference byte
	for index := range left {
		difference |= left[index] ^ right[index]
	}
	return difference == 0
}

func dereferenceRepresentations(value *[]ledger.ForecastRepresentation) []ledger.ForecastRepresentation {
	if value == nil {
		return nil
	}
	return *value
}

func dereferenceString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func dereferenceStrings(value *[]string) []string {
	if value == nil {
		return nil
	}
	return *value
}
