package ledger

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// Clone returns a structurally independent copy. The JSON round trip is
// deliberate: every public model field participates, union invariants are
// rechecked, collection order is retained, and scalar strings are not parsed.
func Clone(source *Ledger) (*Ledger, error) {
	if source == nil {
		return nil, nil
	}
	data, err := json.Marshal(source)
	if err != nil {
		return nil, fmt.Errorf("marshal ledger clone: %w", err)
	}
	var result Ledger
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("unmarshal ledger clone: %w", err)
	}
	return &result, nil
}

// Equal compares all v2 fields, including absent-versus-empty optional
// collections, collection order, and exact scalar spellings.
func Equal(left, right *Ledger) bool {
	return reflect.DeepEqual(left, right)
}

type Counts struct {
	Groups          int
	Relationships   int
	Questions       int
	Revisions       int
	Forecasts       int
	Representations int
	LifecycleEvents int
}

// SummaryCounts returns record counts without flattening revisions or forecast
// representations into the older v1 concepts.
func SummaryCounts(model *Ledger) Counts {
	var result Counts
	if model == nil {
		return result
	}
	if model.Groups != nil {
		result.Groups = len(*model.Groups)
	}
	if model.Relationships != nil {
		result.Relationships = len(*model.Relationships)
	}
	result.Questions = len(model.Questions)
	for _, question := range model.Questions {
		result.Revisions += len(question.Revisions)
		result.Forecasts += len(question.Forecasts)
		for _, forecast := range question.Forecasts {
			if forecast.Representations != nil {
				result.Representations += len(*forecast.Representations)
			}
			if forecast.LifecycleEvents != nil {
				result.LifecycleEvents += len(*forecast.LifecycleEvents)
			}
		}
	}
	return result
}

func (q *Question) Revision(id Slug) (*QuestionRevision, bool) {
	if q == nil {
		return nil, false
	}
	for index := range q.Revisions {
		if q.Revisions[index].ID == id {
			return &q.Revisions[index], true
		}
	}
	return nil, false
}

func (q *Question) CurrentRevision() (*QuestionRevision, bool) {
	if q == nil {
		return nil, false
	}
	return q.Revision(q.CurrentRevisionID)
}

func (q *Question) Forecast(id Slug) (*Forecast, bool) {
	if q == nil {
		return nil, false
	}
	for index := range q.Forecasts {
		if q.Forecasts[index].ID == id {
			return &q.Forecasts[index], true
		}
	}
	return nil, false
}

func (d Domain) Kind() (OutcomeKind, bool) {
	switch {
	case d.Binary != nil:
		return OutcomeBinary, true
	case d.Categorical != nil:
		return OutcomeCategorical, true
	case d.Ordinal != nil:
		return OutcomeOrdinal, true
	case d.Numeric != nil:
		return OutcomeNumeric, true
	case d.Date != nil:
		return OutcomeDate, true
	case d.Datetime != nil:
		return OutcomeDatetime, true
	default:
		return "", false
	}
}

func (r ForecastRepresentation) Kind() (RepresentationKind, bool) {
	switch {
	case r.Probability != nil:
		return RepresentationProbability, true
	case r.PMF != nil:
		return RepresentationPMF, true
	case r.BinnedPMF != nil:
		return RepresentationBinnedPMF, true
	case r.Quantiles != nil:
		return RepresentationQuantiles, true
	case r.CDF != nil:
		return RepresentationCDF, true
	case r.Point != nil:
		return RepresentationPoint, true
	case r.CredibleIntervals != nil:
		return RepresentationCredibleIntervals, true
	default:
		return "", false
	}
}
