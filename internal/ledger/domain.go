package ledger

import (
	"encoding/json"
	"fmt"
)

type OutcomeKind string

const (
	OutcomeBinary      OutcomeKind = "binary"
	OutcomeCategorical OutcomeKind = "categorical"
	OutcomeOrdinal     OutcomeKind = "ordinal"
	OutcomeNumeric     OutcomeKind = "numeric"
	OutcomeDate        OutcomeKind = "date"
	OutcomeDatetime    OutcomeKind = "datetime"
)

type OutcomeSpace struct {
	Kind OutcomeKind `json:"kind" yaml:"kind"`
}

type Option struct {
	ID          Slug    `json:"id" yaml:"id"`
	Label       string  `json:"label" yaml:"label"`
	Description *string `json:"description,omitempty" yaml:"description,omitempty"`
}

type OptionSet struct {
	ID      Slug     `json:"id" yaml:"id"`
	Version int      `json:"version" yaml:"version"`
	Options []Option `json:"options" yaml:"options"`
}

type OptionSetRef struct {
	ID      Slug `json:"id" yaml:"id"`
	Version int  `json:"version" yaml:"version"`
}

type Unit struct {
	Name     string  `json:"name" yaml:"name"`
	Symbol   *string `json:"symbol,omitempty" yaml:"symbol,omitempty"`
	UCUMCode *string `json:"ucum_code,omitempty" yaml:"ucum_code,omitempty"`
}

type Bound[T ~string] struct {
	Value     T    `json:"value" yaml:"value"`
	Inclusive bool `json:"inclusive" yaml:"inclusive"`
}

type Bounds[T ~string] struct {
	Lower *Bound[T] `json:"lower,omitempty" yaml:"lower,omitempty"`
	Upper *Bound[T] `json:"upper,omitempty" yaml:"upper,omitempty"`
}

type Bin[T ~string] struct {
	ID             Slug    `json:"id" yaml:"id"`
	Label          *string `json:"label,omitempty" yaml:"label,omitempty"`
	Lower          T       `json:"lower" yaml:"lower"`
	Upper          T       `json:"upper" yaml:"upper"`
	LowerInclusive bool    `json:"lower_inclusive" yaml:"lower_inclusive"`
	UpperInclusive bool    `json:"upper_inclusive" yaml:"upper_inclusive"`
}

type BinSet[T ~string] struct {
	ID      Slug     `json:"id" yaml:"id"`
	Version int      `json:"version" yaml:"version"`
	Bins    []Bin[T] `json:"bins" yaml:"bins"`
}

type BinSetRef struct {
	ID      Slug `json:"id" yaml:"id"`
	Version int  `json:"version" yaml:"version"`
}

type ValuesKind string

const (
	ValuesContinuous ValuesKind = "continuous"
	ValuesStep       ValuesKind = "step"
	ValuesAllowed    ValuesKind = "allowed_values"
)

type ContinuousValues struct {
	Kind ValuesKind `json:"kind" yaml:"kind"`
}

type StepValues[T ~string, S ~string] struct {
	Kind   ValuesKind `json:"kind" yaml:"kind"`
	Step   S          `json:"step" yaml:"step"`
	Origin *T         `json:"origin,omitempty" yaml:"origin,omitempty"`
}

type AllowedValues[T ~string] struct {
	Kind   ValuesKind `json:"kind" yaml:"kind"`
	Values []T        `json:"values" yaml:"values"`
}

type ValuesPolicy[T ~string, S ~string] struct {
	Continuous *ContinuousValues
	Step       *StepValues[T, S]
	Allowed    *AllowedValues[T]
}

func (v ValuesPolicy[T, S]) MarshalJSON() ([]byte, error) {
	return marshalOne("values policy", v.Continuous, v.Step, v.Allowed)
}

func (v *ValuesPolicy[T, S]) UnmarshalJSON(data []byte) error {
	*v = ValuesPolicy[T, S]{}
	var discriminator struct {
		Kind ValuesKind `json:"kind"`
	}
	if err := json.Unmarshal(data, &discriminator); err != nil {
		return fmt.Errorf("values policy discriminator: %w", err)
	}
	switch discriminator.Kind {
	case ValuesContinuous:
		v.Continuous = new(ContinuousValues)
		return decodeClosed(data, v.Continuous)
	case ValuesStep:
		v.Step = new(StepValues[T, S])
		return decodeClosed(data, v.Step)
	case ValuesAllowed:
		v.Allowed = new(AllowedValues[T])
		return decodeClosed(data, v.Allowed)
	default:
		return fmt.Errorf("unknown values policy kind %q", discriminator.Kind)
	}
}

type Domain struct {
	Binary      *BinaryDomain
	Categorical *CategoricalDomain
	Ordinal     *OrdinalDomain
	Numeric     *NumericDomain
	Date        *DateDomain
	Datetime    *DatetimeDomain
}

type BinaryDomain struct {
	Kind OutcomeKind `json:"kind" yaml:"kind"`
}

type CategoricalDomain struct {
	Kind      OutcomeKind `json:"kind" yaml:"kind"`
	OptionSet OptionSet   `json:"option_set" yaml:"option_set"`
}

type OrdinalDomain struct {
	Kind      OutcomeKind `json:"kind" yaml:"kind"`
	OptionSet OptionSet   `json:"option_set" yaml:"option_set"`
}

type ElicitationScale string

const (
	ScaleLinear ElicitationScale = "linear"
	ScaleLog    ElicitationScale = "log"
)

type NumericDomain struct {
	Kind             OutcomeKind                    `json:"kind" yaml:"kind"`
	Bounds           *Bounds[Decimal]               `json:"bounds,omitempty" yaml:"bounds,omitempty"`
	Values           ValuesPolicy[Decimal, Decimal] `json:"values" yaml:"values"`
	ElicitationScale *ElicitationScale              `json:"elicitation_scale,omitempty" yaml:"elicitation_scale,omitempty"`
	Unit             *Unit                          `json:"unit,omitempty" yaml:"unit,omitempty"`
	BinSets          *[]BinSet[Decimal]             `json:"bin_sets,omitempty" yaml:"bin_sets,omitempty"`
}

type DateDomain struct {
	Kind    OutcomeKind                 `json:"kind" yaml:"kind"`
	Bounds  *Bounds[Date]               `json:"bounds,omitempty" yaml:"bounds,omitempty"`
	Values  ValuesPolicy[Date, DayStep] `json:"values" yaml:"values"`
	BinSets *[]BinSet[Date]             `json:"bin_sets,omitempty" yaml:"bin_sets,omitempty"`
}

type DatetimeDomain struct {
	Kind    OutcomeKind                         `json:"kind" yaml:"kind"`
	Bounds  *Bounds[Timestamp]                  `json:"bounds,omitempty" yaml:"bounds,omitempty"`
	Values  ValuesPolicy[Timestamp, SecondStep] `json:"values" yaml:"values"`
	BinSets *[]BinSet[Timestamp]                `json:"bin_sets,omitempty" yaml:"bin_sets,omitempty"`
}

func (v Domain) MarshalJSON() ([]byte, error) {
	return marshalOne("domain", v.Binary, v.Categorical, v.Ordinal, v.Numeric, v.Date, v.Datetime)
}

func (v *Domain) UnmarshalJSON(data []byte) error {
	*v = Domain{}
	var discriminator struct {
		Kind OutcomeKind `json:"kind"`
	}
	if err := json.Unmarshal(data, &discriminator); err != nil {
		return err
	}
	switch discriminator.Kind {
	case OutcomeBinary:
		v.Binary = new(BinaryDomain)
		return decodeClosed(data, v.Binary)
	case OutcomeCategorical:
		v.Categorical = new(CategoricalDomain)
		return decodeClosed(data, v.Categorical)
	case OutcomeOrdinal:
		v.Ordinal = new(OrdinalDomain)
		return decodeClosed(data, v.Ordinal)
	case OutcomeNumeric:
		v.Numeric = new(NumericDomain)
		return decodeClosed(data, v.Numeric)
	case OutcomeDate:
		v.Date = new(DateDomain)
		return decodeClosed(data, v.Date)
	case OutcomeDatetime:
		v.Datetime = new(DatetimeDomain)
		return decodeClosed(data, v.Datetime)
	default:
		return fmt.Errorf("unknown domain kind %q", discriminator.Kind)
	}
}

type QuestionRevision struct {
	ID                   Slug         `json:"id" yaml:"id"`
	EffectiveAt          Timestamp    `json:"effective_at" yaml:"effective_at"`
	RecordedAt           Timestamp    `json:"recorded_at" yaml:"recorded_at"`
	Title                string       `json:"title" yaml:"title"`
	ResolutionCriteria   string       `json:"resolution_criteria" yaml:"resolution_criteria"`
	ForecastingOpensAt   *Timestamp   `json:"forecasting_opens_at,omitempty" yaml:"forecasting_opens_at,omitempty"`
	ExpectedResolutionAt Timestamp    `json:"expected_resolution_at" yaml:"expected_resolution_at"`
	OutcomeSpace         OutcomeSpace `json:"outcome_space" yaml:"outcome_space"`
	Domain               Domain       `json:"domain" yaml:"domain"`
	Provenance           *Provenance  `json:"provenance,omitempty" yaml:"provenance,omitempty"`
}
