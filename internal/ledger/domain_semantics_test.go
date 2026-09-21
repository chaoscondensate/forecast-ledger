package ledger

import "testing"

func TestScalarInEveryDomainKind(t *testing.T) {
	t.Parallel()

	yes := true
	no := false
	option := "b"
	number := "1.25"
	date := "2026-01-03"
	datetime := "2026-01-01T01:00:04+01:00"
	tests := []struct {
		name   string
		value  ScalarValue
		domain Domain
		want   bool
	}{
		{"binary", ScalarValue{Boolean: &yes}, Domain{Binary: &BinaryDomain{Kind: OutcomeBinary}}, true},
		{"binary rejects string", ScalarValue{String: &option}, Domain{Binary: &BinaryDomain{Kind: OutcomeBinary}}, false},
		{"categorical", ScalarValue{String: &option}, Domain{Categorical: &CategoricalDomain{Kind: OutcomeCategorical, OptionSet: OptionSet{Options: []Option{{ID: "a"}, {ID: "b"}}}}}, true},
		{"ordinal missing", ScalarValue{Boolean: &no}, Domain{Ordinal: &OrdinalDomain{Kind: OutcomeOrdinal, OptionSet: OptionSet{Options: []Option{{ID: "a"}}}}}, false},
		{"numeric", ScalarValue{String: &number}, numericStepDomain("-2", true, "2", false, "0.05", "0"), true},
		{"date", ScalarValue{String: &date}, dateStepDomain("2026-01-01", true, "2026-01-10", true, "P2D", "2026-01-01"), true},
		{"datetime offset", ScalarValue{String: &datetime}, datetimeStepDomain("2026-01-01T00:00:00Z", true, "2026-01-01T00:00:10Z", true, "PT2S", "2026-01-01T00:00:00Z"), true},
	}
	for _, test := range tests {
		got, _ := ScalarInDomain(test.value, test.domain)
		if got != test.want {
			t.Errorf("%s membership = %v, want %v", test.name, got, test.want)
		}
	}
}

func TestBoundsInclusivityAndExactSteps(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		value string
		want  bool
	}{
		{"-2", true},
		{"1.95", true},
		{"2", false},
		{"2.05", false},
		{"1.951", false},
	} {
		value := test.value
		got, _ := ScalarInDomain(ScalarValue{String: &value}, numericStepDomain("-2", true, "2", false, "0.05", "0"))
		if got != test.want {
			t.Errorf("numeric %s membership = %v, want %v", test.value, got, test.want)
		}
	}
}

func TestDateAndDatetimeDefaultOriginsAndOffsets(t *testing.T) {
	t.Parallel()

	dateDomain := Domain{Date: &DateDomain{
		Kind:   OutcomeDate,
		Values: ValuesPolicy[Date, DayStep]{Step: &StepValues[Date, DayStep]{Kind: ValuesStep, Step: "P2D"}},
	}}
	for value, want := range map[string]bool{"1970-01-01": true, "1970-01-03": true, "1970-01-02": false} {
		value := value
		got, _ := ScalarInDomain(ScalarValue{String: &value}, dateDomain)
		if got != want {
			t.Errorf("date %s membership = %v, want %v", value, got, want)
		}
	}

	datetimeDomain := Domain{Datetime: &DatetimeDomain{
		Kind:   OutcomeDatetime,
		Values: ValuesPolicy[Timestamp, SecondStep]{Step: &StepValues[Timestamp, SecondStep]{Kind: ValuesStep, Step: "PT2S"}},
	}}
	for value, want := range map[string]bool{
		"1970-01-01T01:00:02+01:00": true,
		"1969-12-31T23:59:58Z":      true,
		"1970-01-01T00:00:01Z":      false,
		"1970-01-01T00:00:02.5Z":    false,
	} {
		value := value
		got, _ := ScalarInDomain(ScalarValue{String: &value}, datetimeDomain)
		if got != want {
			t.Errorf("datetime %s membership = %v, want %v", value, got, want)
		}
	}
}

func TestAllowedValuesMustBeSortedUniqueAndInsideBounds(t *testing.T) {
	t.Parallel()

	valid := Domain{Numeric: &NumericDomain{
		Kind:   OutcomeNumeric,
		Bounds: &Bounds[Decimal]{Lower: &Bound[Decimal]{Value: "-1", Inclusive: true}, Upper: &Bound[Decimal]{Value: "2", Inclusive: true}},
		Values: ValuesPolicy[Decimal, Decimal]{Allowed: &AllowedValues[Decimal]{Kind: ValuesAllowed, Values: []Decimal{"-1", "0", "1.5"}}},
	}}
	if err := ValidateDomainScalars(valid); err != nil {
		t.Fatal(err)
	}
	for _, values := range [][]Decimal{{"0", "-1"}, {"0", "0"}, {"0", "3"}, {"0", "1.0"}} {
		invalid := valid
		invalid.Numeric = &NumericDomain{Kind: OutcomeNumeric, Bounds: valid.Numeric.Bounds, Values: ValuesPolicy[Decimal, Decimal]{Allowed: &AllowedValues[Decimal]{Kind: ValuesAllowed, Values: values}}}
		if err := ValidateDomainScalars(invalid); err == nil {
			t.Errorf("accepted invalid allowed values %#v", values)
		}
	}
}

func TestStepAndTemporalParsersRejectMalformedValues(t *testing.T) {
	t.Parallel()

	for _, value := range []DayStep{"P0D", "P-1D", "P1.5D", "PT1S", "P999999999999999999999999D"} {
		if _, err := ParseDayStep(value); err == nil {
			t.Errorf("ParseDayStep accepted %q", value)
		}
	}
	for _, value := range []SecondStep{"PT0S", "PT-1S", "PT1.5S", "P1D", "PT999999999999999999999999S"} {
		if _, err := ParseSecondStep(value); err == nil {
			t.Errorf("ParseSecondStep accepted %q", value)
		}
	}
	for _, value := range []Date{"2026-02-30", "2026-2-01", "not-a-date"} {
		if _, err := ParseDate(value); err == nil {
			t.Errorf("ParseDate accepted %q", value)
		}
	}
	for _, value := range []Timestamp{"2026-01-01", "2026-01-01T00:00Z", "2026-01-01T00:00:00", "2026-01-01T25:00:00Z", "2026-01-01T00:00:00+25:00"} {
		if _, err := ParseTimestamp(value); err == nil {
			t.Errorf("ParseTimestamp accepted %q", value)
		}
	}
}

func TestOffsetAwareScalarOrdering(t *testing.T) {
	t.Parallel()

	comparison, err := CompareScalarStrings(OutcomeDatetime, "2026-01-01T01:00:00+01:00", "2026-01-01T00:00:00Z")
	if err != nil || comparison != 0 {
		t.Fatalf("same instant comparison = %d, %v", comparison, err)
	}
	comparison, err = CompareScalarStrings(OutcomeDatetime, "2025-12-31T23:59:59Z", "2026-01-01T01:00:00+01:00")
	if err != nil || comparison >= 0 {
		t.Fatalf("ordered instant comparison = %d, %v", comparison, err)
	}
}

func FuzzTemporalStepMembership(f *testing.F) {
	f.Add("1970-01-01T00:00:00Z", "PT1S")
	f.Add("2026-01-01T01:00:04+01:00", "PT2S")
	f.Add("bad", "PT0S")
	f.Fuzz(func(t *testing.T, valueText, stepText string) {
		value := valueText
		domain := Domain{Datetime: &DatetimeDomain{
			Kind:   OutcomeDatetime,
			Values: ValuesPolicy[Timestamp, SecondStep]{Step: &StepValues[Timestamp, SecondStep]{Kind: ValuesStep, Step: SecondStep(stepText)}},
		}}
		ScalarInDomain(ScalarValue{String: &value}, domain)
	})
}

func numericStepDomain(lower string, lowerInclusive bool, upper string, upperInclusive bool, step string, origin string) Domain {
	lowerValue, upperValue := Decimal(lower), Decimal(upper)
	originValue := Decimal(origin)
	return Domain{Numeric: &NumericDomain{
		Kind:   OutcomeNumeric,
		Bounds: &Bounds[Decimal]{Lower: &Bound[Decimal]{Value: lowerValue, Inclusive: lowerInclusive}, Upper: &Bound[Decimal]{Value: upperValue, Inclusive: upperInclusive}},
		Values: ValuesPolicy[Decimal, Decimal]{Step: &StepValues[Decimal, Decimal]{Kind: ValuesStep, Step: Decimal(step), Origin: &originValue}},
	}}
}

func dateStepDomain(lower string, lowerInclusive bool, upper string, upperInclusive bool, step string, origin string) Domain {
	lowerValue, upperValue := Date(lower), Date(upper)
	originValue := Date(origin)
	return Domain{Date: &DateDomain{
		Kind:   OutcomeDate,
		Bounds: &Bounds[Date]{Lower: &Bound[Date]{Value: lowerValue, Inclusive: lowerInclusive}, Upper: &Bound[Date]{Value: upperValue, Inclusive: upperInclusive}},
		Values: ValuesPolicy[Date, DayStep]{Step: &StepValues[Date, DayStep]{Kind: ValuesStep, Step: DayStep(step), Origin: &originValue}},
	}}
}

func datetimeStepDomain(lower string, lowerInclusive bool, upper string, upperInclusive bool, step string, origin string) Domain {
	lowerValue, upperValue := Timestamp(lower), Timestamp(upper)
	originValue := Timestamp(origin)
	return Domain{Datetime: &DatetimeDomain{
		Kind:   OutcomeDatetime,
		Bounds: &Bounds[Timestamp]{Lower: &Bound[Timestamp]{Value: lowerValue, Inclusive: lowerInclusive}, Upper: &Bound[Timestamp]{Value: upperValue, Inclusive: upperInclusive}},
		Values: ValuesPolicy[Timestamp, SecondStep]{Step: &StepValues[Timestamp, SecondStep]{Kind: ValuesStep, Step: SecondStep(step), Origin: &originValue}},
	}}
}
