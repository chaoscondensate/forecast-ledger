package ledger

import (
	"encoding/json"
	"fmt"
)

type ForecastVisibility string

const (
	VisibilityPublic   ForecastVisibility = "public"
	VisibilitySealed   ForecastVisibility = "sealed"
	VisibilityRevealed ForecastVisibility = "revealed"
)

type RepresentationKind string

const (
	RepresentationProbability       RepresentationKind = "probability"
	RepresentationPMF               RepresentationKind = "pmf"
	RepresentationBinnedPMF         RepresentationKind = "binned_pmf"
	RepresentationQuantiles         RepresentationKind = "quantiles"
	RepresentationCDF               RepresentationKind = "cdf"
	RepresentationPoint             RepresentationKind = "point"
	RepresentationCredibleIntervals RepresentationKind = "credible_intervals"
)

type ForecastRepresentation struct {
	Probability       *ProbabilityRepresentation
	PMF               *PMFRepresentation
	BinnedPMF         *BinnedPMFRepresentation
	Quantiles         *QuantilesRepresentation
	CDF               *CDFRepresentation
	Point             *PointRepresentation
	CredibleIntervals *CredibleIntervalsRepresentation
}

type ProbabilityRepresentation struct {
	Kind        RepresentationKind `json:"kind" yaml:"kind"`
	Outcome     bool               `json:"outcome" yaml:"outcome"`
	Probability Probability        `json:"probability" yaml:"probability"`
}

type PMFEntry struct {
	OptionID    Slug        `json:"option_id" yaml:"option_id"`
	Probability Probability `json:"probability" yaml:"probability"`
}

type PMFRepresentation struct {
	Kind         RepresentationKind `json:"kind" yaml:"kind"`
	OptionSetRef OptionSetRef       `json:"option_set_ref" yaml:"option_set_ref"`
	Entries      []PMFEntry         `json:"entries" yaml:"entries"`
}

type BinnedPMFEntry struct {
	BinID       Slug        `json:"bin_id" yaml:"bin_id"`
	Probability Probability `json:"probability" yaml:"probability"`
}

type BinnedPMFRepresentation struct {
	Kind                 RepresentationKind `json:"kind" yaml:"kind"`
	BinSetRef            BinSetRef          `json:"bin_set_ref" yaml:"bin_set_ref"`
	Entries              []BinnedPMFEntry   `json:"entries" yaml:"entries"`
	LeftTailProbability  Probability        `json:"left_tail_probability" yaml:"left_tail_probability"`
	RightTailProbability Probability        `json:"right_tail_probability" yaml:"right_tail_probability"`
}

type QuantileInterpolation string

const (
	QuantileInterpolationNone      QuantileInterpolation = "none"
	QuantileInterpolationLinear    QuantileInterpolation = "linear"
	QuantileInterpolationStepLower QuantileInterpolation = "step_lower"
	QuantileInterpolationStepUpper QuantileInterpolation = "step_upper"
)

type QuantilePoint struct {
	Level OpenProbability `json:"level" yaml:"level"`
	Value ScalarValue     `json:"value" yaml:"value"`
}

type QuantilesRepresentation struct {
	Kind          RepresentationKind    `json:"kind" yaml:"kind"`
	Interpolation QuantileInterpolation `json:"interpolation" yaml:"interpolation"`
	Points        []QuantilePoint       `json:"points" yaml:"points"`
}

type CDFInterpolation string

const (
	CDFInterpolationStepRight CDFInterpolation = "step_right"
	CDFInterpolationLinear    CDFInterpolation = "linear"
)

type CDFPoint struct {
	Value       ScalarValue `json:"value" yaml:"value"`
	Probability Probability `json:"probability" yaml:"probability"`
}

type CDFRepresentation struct {
	Kind                 RepresentationKind `json:"kind" yaml:"kind"`
	Interpolation        CDFInterpolation   `json:"interpolation" yaml:"interpolation"`
	Points               []CDFPoint         `json:"points" yaml:"points"`
	LeftTailProbability  Probability        `json:"left_tail_probability" yaml:"left_tail_probability"`
	RightTailProbability Probability        `json:"right_tail_probability" yaml:"right_tail_probability"`
}

type PointStatistic string

const (
	StatisticMean         PointStatistic = "mean"
	StatisticMedian       PointStatistic = "median"
	StatisticMode         PointStatistic = "mode"
	StatisticBestEstimate PointStatistic = "best_estimate"
)

type PointRepresentation struct {
	Kind      RepresentationKind `json:"kind" yaml:"kind"`
	Statistic PointStatistic     `json:"statistic" yaml:"statistic"`
	Value     ScalarValue        `json:"value" yaml:"value"`
}

type IntervalKind string

const (
	IntervalEqualTailed    IntervalKind = "equal_tailed"
	IntervalCentral        IntervalKind = "central"
	IntervalHighestDensity IntervalKind = "highest_density"
	IntervalAuthorSelected IntervalKind = "author_selected"
)

type CredibleInterval struct {
	Coverage     OpenProbability `json:"coverage" yaml:"coverage"`
	IntervalKind IntervalKind    `json:"interval_kind" yaml:"interval_kind"`
	Lower        ScalarValue     `json:"lower" yaml:"lower"`
	Upper        ScalarValue     `json:"upper" yaml:"upper"`
}

type CredibleIntervalsRepresentation struct {
	Kind      RepresentationKind `json:"kind" yaml:"kind"`
	Intervals []CredibleInterval `json:"intervals" yaml:"intervals"`
}

func (v ForecastRepresentation) MarshalJSON() ([]byte, error) {
	return marshalOne("forecast representation", v.Probability, v.PMF, v.BinnedPMF, v.Quantiles, v.CDF, v.Point, v.CredibleIntervals)
}

func (v *ForecastRepresentation) UnmarshalJSON(data []byte) error {
	*v = ForecastRepresentation{}
	var discriminator struct {
		Kind RepresentationKind `json:"kind"`
	}
	if err := json.Unmarshal(data, &discriminator); err != nil {
		return err
	}
	switch discriminator.Kind {
	case RepresentationProbability:
		v.Probability = new(ProbabilityRepresentation)
		return decodeClosed(data, v.Probability)
	case RepresentationPMF:
		v.PMF = new(PMFRepresentation)
		return decodeClosed(data, v.PMF)
	case RepresentationBinnedPMF:
		v.BinnedPMF = new(BinnedPMFRepresentation)
		return decodeClosed(data, v.BinnedPMF)
	case RepresentationQuantiles:
		v.Quantiles = new(QuantilesRepresentation)
		return decodeClosed(data, v.Quantiles)
	case RepresentationCDF:
		v.CDF = new(CDFRepresentation)
		return decodeClosed(data, v.CDF)
	case RepresentationPoint:
		v.Point = new(PointRepresentation)
		return decodeClosed(data, v.Point)
	case RepresentationCredibleIntervals:
		v.CredibleIntervals = new(CredibleIntervalsRepresentation)
		return decodeClosed(data, v.CredibleIntervals)
	default:
		return fmt.Errorf("unknown forecast representation kind %q", discriminator.Kind)
	}
}

type LifecycleEventType string

const (
	LifecycleWithdrawn  LifecycleEventType = "withdrawn"
	LifecycleExpired    LifecycleEventType = "expired"
	LifecycleReaffirmed LifecycleEventType = "reaffirmed"
)

type LifecycleEvent struct {
	ID          Slug               `json:"id" yaml:"id"`
	Type        LifecycleEventType `json:"type" yaml:"type"`
	EffectiveAt Timestamp          `json:"effective_at" yaml:"effective_at"`
	RecordedAt  Timestamp          `json:"recorded_at" yaml:"recorded_at"`
	Reason      *string            `json:"reason,omitempty" yaml:"reason,omitempty"`
	Provenance  *Provenance        `json:"provenance,omitempty" yaml:"provenance,omitempty"`
}

type Forecast struct {
	ID                   Slug                      `json:"id" yaml:"id"`
	QuestionRevisionID   Slug                      `json:"question_revision_id" yaml:"question_revision_id"`
	ForecastedAt         Timestamp                 `json:"forecasted_at" yaml:"forecasted_at"`
	RecordedAt           Timestamp                 `json:"recorded_at" yaml:"recorded_at"`
	Visibility           ForecastVisibility        `json:"visibility" yaml:"visibility"`
	Representations      *[]ForecastRepresentation `json:"representations,omitempty" yaml:"representations,omitempty"`
	Rationale            *string                   `json:"rationale,omitempty" yaml:"rationale,omitempty"`
	KeyFactors           *[]string                 `json:"key_factors,omitempty" yaml:"key_factors,omitempty"`
	Comment              *string                   `json:"comment,omitempty" yaml:"comment,omitempty"`
	PublicNote           *string                   `json:"public_note,omitempty" yaml:"public_note,omitempty"`
	SupersedesForecastID *Slug                     `json:"supersedes_forecast_id,omitempty" yaml:"supersedes_forecast_id,omitempty"`
	Provenance           *Provenance               `json:"provenance,omitempty" yaml:"provenance,omitempty"`
	LifecycleEvents      *[]LifecycleEvent         `json:"lifecycle_events,omitempty" yaml:"lifecycle_events,omitempty"`
	Commitment           *Commitment               `json:"commitment,omitempty" yaml:"commitment,omitempty"`
	Integrity            Integrity                 `json:"integrity" yaml:"integrity"`
}
