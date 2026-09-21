package ledger

import (
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"time"

	"github.com/chaoscondensate/forecast-ledger/internal/exact"
)

var (
	dateStepPattern     = regexp.MustCompile(`^P([1-9][0-9]*)D$`)
	datetimeStepPattern = regexp.MustCompile(`^PT([1-9][0-9]*)S$`)
	timestampPattern    = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$`)
)

type parsedScalar struct {
	kind    OutcomeKind
	boolean bool
	text    string
	decimal exact.Decimal
	instant time.Time
}

func ParseDate(value Date) (time.Time, error) {
	parsed, err := time.Parse(time.DateOnly, string(value))
	if err != nil || parsed.Format(time.DateOnly) != string(value) {
		return time.Time{}, fmt.Errorf("%q is not a valid YYYY-MM-DD date", value)
	}
	return parsed, nil
}

func ParseTimestamp(value Timestamp) (time.Time, error) {
	if !timestampPattern.MatchString(string(value)) {
		return time.Time{}, fmt.Errorf("%q is not an RFC 3339 timestamp with seconds and an explicit offset", value)
	}
	parsed, err := time.Parse(time.RFC3339Nano, string(value))
	if err != nil {
		return time.Time{}, fmt.Errorf("%q is not a valid RFC 3339 timestamp: %w", value, err)
	}
	return parsed, nil
}

func ParseDayStep(value DayStep) (int64, error) {
	return parsePositiveStep(string(value), dateStepPattern, "P<n>D")
}

func ParseSecondStep(value SecondStep) (int64, error) {
	return parsePositiveStep(string(value), datetimeStepPattern, "PT<n>S")
}

func CompareScalarStrings(kind OutcomeKind, left, right string) (int, error) {
	leftValue, err := parseTextScalar(kind, left)
	if err != nil {
		return 0, err
	}
	rightValue, err := parseTextScalar(kind, right)
	if err != nil {
		return 0, err
	}
	return compareParsed(leftValue, rightValue), nil
}

func CompareScalars(kind OutcomeKind, left, right ScalarValue) (int, error) {
	leftValue, err := parseScalarValue(left, kind)
	if err != nil {
		return 0, err
	}
	rightValue, err := parseScalarValue(right, kind)
	if err != nil {
		return 0, err
	}
	return compareParsed(leftValue, rightValue), nil
}

// ScalarInDomain is the shared path for forecast, relationship, and resolution
// membership checks. It retains the authored ScalarValue and parses only a
// temporary exact comparison value.
func ScalarInDomain(value ScalarValue, domain Domain) (bool, string) {
	kind, ok := domain.Kind()
	if !ok {
		return false, "domain has no variant"
	}
	parsed, err := parseScalarValue(value, kind)
	if err != nil {
		return false, err.Error()
	}
	switch kind {
	case OutcomeBinary:
		return true, ""
	case OutcomeCategorical:
		return optionMember(parsed.text, domain.Categorical.OptionSet), "must reference an option in the bound option set"
	case OutcomeOrdinal:
		return optionMember(parsed.text, domain.Ordinal.OptionSet), "must reference an option in the bound option set"
	case OutcomeNumeric:
		return numericInDomain(parsed, domain.Numeric)
	case OutcomeDate:
		return dateInDomain(parsed, domain.Date)
	case OutcomeDatetime:
		return datetimeInDomain(parsed, domain.Datetime)
	default:
		return false, "unsupported domain kind"
	}
}

// ValidateDomainScalars checks scalar ordering, bounds, and values-policy
// arithmetic. Cross-record identity and bin continuity are checked separately.
func ValidateDomainScalars(domain Domain) error {
	kind, ok := domain.Kind()
	if !ok {
		return errors.New("domain has no variant")
	}
	switch kind {
	case OutcomeBinary, OutcomeCategorical, OutcomeOrdinal:
		return nil
	case OutcomeNumeric:
		if err := validateBounds(kind, domain.Numeric.Bounds); err != nil {
			return err
		}
		return validateNumericPolicy(domain.Numeric.Values, domain.Numeric.Bounds)
	case OutcomeDate:
		if err := validateBounds(kind, domain.Date.Bounds); err != nil {
			return err
		}
		return validateDatePolicy(domain.Date.Values, domain.Date.Bounds)
	case OutcomeDatetime:
		if err := validateBounds(kind, domain.Datetime.Bounds); err != nil {
			return err
		}
		return validateDatetimePolicy(domain.Datetime.Values, domain.Datetime.Bounds)
	default:
		return fmt.Errorf("unsupported domain kind %q", kind)
	}
}

func parseScalarValue(value ScalarValue, kind OutcomeKind) (parsedScalar, error) {
	if kind == OutcomeBinary {
		if value.Boolean == nil || value.String != nil {
			return parsedScalar{}, errors.New("must be a boolean")
		}
		return parsedScalar{kind: kind, boolean: *value.Boolean}, nil
	}
	if value.String == nil || value.Boolean != nil {
		return parsedScalar{}, errors.New("must be a string")
	}
	return parseTextScalar(kind, *value.String)
}

func parseTextScalar(kind OutcomeKind, value string) (parsedScalar, error) {
	result := parsedScalar{kind: kind, text: value}
	switch kind {
	case OutcomeCategorical, OutcomeOrdinal:
		if value == "" {
			return parsedScalar{}, errors.New("must be a non-empty option ID")
		}
	case OutcomeNumeric:
		decimal, err := exact.ParseDecimal(value)
		if err != nil {
			return parsedScalar{}, errors.New("must be a canonical decimal string")
		}
		result.decimal = decimal
	case OutcomeDate:
		date, err := ParseDate(Date(value))
		if err != nil {
			return parsedScalar{}, err
		}
		result.instant = date
	case OutcomeDatetime:
		instant, err := ParseTimestamp(Timestamp(value))
		if err != nil {
			return parsedScalar{}, err
		}
		result.instant = instant
	default:
		return parsedScalar{}, fmt.Errorf("unsupported text scalar kind %q", kind)
	}
	return result, nil
}

func compareParsed(left, right parsedScalar) int {
	switch left.kind {
	case OutcomeNumeric:
		return left.decimal.Cmp(right.decimal)
	case OutcomeDate, OutcomeDatetime:
		if left.instant.Before(right.instant) {
			return -1
		}
		if left.instant.After(right.instant) {
			return 1
		}
		return 0
	default:
		if left.text < right.text {
			return -1
		}
		if left.text > right.text {
			return 1
		}
		return 0
	}
}

func optionMember(value string, options OptionSet) bool {
	for _, option := range options.Options {
		if string(option.ID) == value {
			return true
		}
	}
	return false
}

func numericInDomain(value parsedScalar, domain *NumericDomain) (bool, string) {
	if ok, reason := checkBounds(value, domain.Bounds); !ok {
		return false, reason
	}
	policy := domain.Values
	switch {
	case policy.Continuous != nil:
		return true, ""
	case policy.Allowed != nil:
		for _, allowed := range policy.Allowed.Values {
			if string(allowed) == value.text {
				return true, ""
			}
		}
		return false, "is not in domain.values.allowed_values"
	case policy.Step != nil:
		step, err := exact.ParseDecimal(string(policy.Step.Step))
		if err != nil || step.Sign() <= 0 {
			return false, "domain step must be positive"
		}
		origin := exact.Zero()
		if policy.Step.Origin != nil {
			origin, err = exact.ParseDecimal(string(*policy.Step.Origin))
			if err != nil {
				return false, "domain step origin must be a canonical decimal"
			}
		}
		if !exact.Aligns(value.decimal, origin, step) {
			return false, "does not align with the domain step"
		}
		return true, ""
	default:
		return false, "domain values policy has no variant"
	}
}

func dateInDomain(value parsedScalar, domain *DateDomain) (bool, string) {
	if ok, reason := checkBounds(value, domain.Bounds); !ok {
		return false, reason
	}
	policy := domain.Values
	switch {
	case policy.Continuous != nil:
		return true, ""
	case policy.Allowed != nil:
		for _, allowed := range policy.Allowed.Values {
			if string(allowed) == value.text {
				return true, ""
			}
		}
		return false, "is not in domain.values.allowed_values"
	case policy.Step != nil:
		step, err := ParseDayStep(policy.Step.Step)
		if err != nil {
			return false, err.Error()
		}
		origin := time.Unix(0, 0).UTC()
		if policy.Step.Origin != nil {
			origin, err = ParseDate(*policy.Step.Origin)
			if err != nil {
				return false, err.Error()
			}
		}
		elapsedDays := (value.instant.Unix() - origin.Unix()) / 86400
		if elapsedDays%step != 0 {
			return false, "does not align with the domain step"
		}
		return true, ""
	default:
		return false, "domain values policy has no variant"
	}
}

func datetimeInDomain(value parsedScalar, domain *DatetimeDomain) (bool, string) {
	if ok, reason := checkBounds(value, domain.Bounds); !ok {
		return false, reason
	}
	policy := domain.Values
	switch {
	case policy.Continuous != nil:
		return true, ""
	case policy.Allowed != nil:
		for _, allowed := range policy.Allowed.Values {
			if string(allowed) == value.text {
				return true, ""
			}
		}
		return false, "is not in domain.values.allowed_values"
	case policy.Step != nil:
		step, err := ParseSecondStep(policy.Step.Step)
		if err != nil {
			return false, err.Error()
		}
		origin := time.Unix(0, 0).UTC()
		if policy.Step.Origin != nil {
			origin, err = ParseTimestamp(*policy.Step.Origin)
			if err != nil {
				return false, err.Error()
			}
		}
		if !instantAligns(value.instant, origin, step) {
			return false, "does not align with the domain step"
		}
		return true, ""
	default:
		return false, "domain values policy has no variant"
	}
}

func checkBounds[T ~string](value parsedScalar, bounds *Bounds[T]) (bool, string) {
	if bounds == nil {
		return true, ""
	}
	if bounds.Lower != nil {
		lower, err := parseTextScalar(value.kind, string(bounds.Lower.Value))
		if err != nil {
			return false, "invalid lower bound: " + err.Error()
		}
		comparison := compareParsed(value, lower)
		if comparison < 0 || (comparison == 0 && !bounds.Lower.Inclusive) {
			return false, "is below the domain lower bound"
		}
	}
	if bounds.Upper != nil {
		upper, err := parseTextScalar(value.kind, string(bounds.Upper.Value))
		if err != nil {
			return false, "invalid upper bound: " + err.Error()
		}
		comparison := compareParsed(value, upper)
		if comparison > 0 || (comparison == 0 && !bounds.Upper.Inclusive) {
			return false, "is above the domain upper bound"
		}
	}
	return true, ""
}

func validateBounds[T ~string](kind OutcomeKind, bounds *Bounds[T]) error {
	if bounds == nil || bounds.Lower == nil || bounds.Upper == nil {
		return nil
	}
	lower, err := parseTextScalar(kind, string(bounds.Lower.Value))
	if err != nil {
		return fmt.Errorf("invalid lower bound: %w", err)
	}
	upper, err := parseTextScalar(kind, string(bounds.Upper.Value))
	if err != nil {
		return fmt.Errorf("invalid upper bound: %w", err)
	}
	comparison := compareParsed(lower, upper)
	if comparison > 0 || (comparison == 0 && !(bounds.Lower.Inclusive && bounds.Upper.Inclusive)) {
		return errors.New("bounds describe an empty domain")
	}
	return nil
}

func validateNumericPolicy(policy ValuesPolicy[Decimal, Decimal], bounds *Bounds[Decimal]) error {
	switch {
	case policy.Continuous != nil:
		return nil
	case policy.Step != nil:
		step, err := exact.ParseDecimal(string(policy.Step.Step))
		if err != nil || step.Sign() <= 0 {
			return errors.New("numeric step must be a positive canonical decimal")
		}
		if policy.Step.Origin != nil {
			_, err = exact.ParseDecimal(string(*policy.Step.Origin))
		}
		return err
	case policy.Allowed != nil:
		return validateAllowed(OutcomeNumeric, policy.Allowed.Values, func(value Decimal) (bool, string) {
			parsed, err := parseTextScalar(OutcomeNumeric, string(value))
			if err != nil {
				return false, err.Error()
			}
			return checkBounds(parsed, bounds)
		})
	default:
		return errors.New("numeric values policy has no variant")
	}
}

func validateDatePolicy(policy ValuesPolicy[Date, DayStep], bounds *Bounds[Date]) error {
	switch {
	case policy.Continuous != nil:
		return nil
	case policy.Step != nil:
		if _, err := ParseDayStep(policy.Step.Step); err != nil {
			return err
		}
		if policy.Step.Origin != nil {
			_, err := ParseDate(*policy.Step.Origin)
			return err
		}
		return nil
	case policy.Allowed != nil:
		return validateAllowed(OutcomeDate, policy.Allowed.Values, func(value Date) (bool, string) {
			parsed, err := parseTextScalar(OutcomeDate, string(value))
			if err != nil {
				return false, err.Error()
			}
			return checkBounds(parsed, bounds)
		})
	default:
		return errors.New("date values policy has no variant")
	}
}

func validateDatetimePolicy(policy ValuesPolicy[Timestamp, SecondStep], bounds *Bounds[Timestamp]) error {
	switch {
	case policy.Continuous != nil:
		return nil
	case policy.Step != nil:
		if _, err := ParseSecondStep(policy.Step.Step); err != nil {
			return err
		}
		if policy.Step.Origin != nil {
			_, err := ParseTimestamp(*policy.Step.Origin)
			return err
		}
		return nil
	case policy.Allowed != nil:
		return validateAllowed(OutcomeDatetime, policy.Allowed.Values, func(value Timestamp) (bool, string) {
			parsed, err := parseTextScalar(OutcomeDatetime, string(value))
			if err != nil {
				return false, err.Error()
			}
			return checkBounds(parsed, bounds)
		})
	default:
		return errors.New("datetime values policy has no variant")
	}
}

func validateAllowed[T ~string](kind OutcomeKind, values []T, membership func(T) (bool, string)) error {
	var previous parsedScalar
	for index, value := range values {
		parsed, err := parseTextScalar(kind, string(value))
		if err != nil {
			return err
		}
		if index > 0 && compareParsed(previous, parsed) >= 0 {
			return errors.New("allowed values must be unique and sorted")
		}
		if ok, reason := membership(value); !ok {
			return errors.New(reason)
		}
		previous = parsed
	}
	return nil
}

func parsePositiveStep(value string, pattern *regexp.Regexp, expected string) (int64, error) {
	match := pattern.FindStringSubmatch(value)
	if match == nil {
		return 0, fmt.Errorf("step must use %s with a positive integer", expected)
	}
	amount, err := strconv.ParseInt(match[1], 10, 64)
	if err != nil || amount <= 0 {
		return 0, fmt.Errorf("step must use %s with a bounded positive integer", expected)
	}
	return amount, nil
}

func instantAligns(value, origin time.Time, stepSeconds int64) bool {
	valueNanos := instantNanos(value)
	originNanos := instantNanos(origin)
	difference := new(big.Int).Sub(valueNanos, originNanos)
	step := new(big.Int).Mul(big.NewInt(stepSeconds), big.NewInt(1_000_000_000))
	return new(big.Int).Rem(difference, step).Sign() == 0
}

func instantNanos(value time.Time) *big.Int {
	seconds := new(big.Int).Mul(big.NewInt(value.Unix()), big.NewInt(1_000_000_000))
	return seconds.Add(seconds, big.NewInt(int64(value.Nanosecond())))
}
