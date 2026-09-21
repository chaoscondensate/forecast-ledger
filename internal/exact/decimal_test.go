package exact

import (
	"strings"
	"testing"
)

func TestCanonicalDecimalGrammar(t *testing.T) {
	t.Parallel()

	valid := []string{"0", "1", "-12", "0.62", "-0.25", "123456789012345678901234567890.123456789"}
	for _, input := range valid {
		value, err := ParseDecimal(input)
		if err != nil {
			t.Errorf("ParseDecimal(%q): %v", input, err)
			continue
		}
		if value.String() != input {
			t.Errorf("ParseDecimal(%q).String() = %q", input, value.String())
		}
	}
	invalid := []string{"", "+1", "01", "-01", "1.0", "0.620", "1e3", "-0", ".5", "0.", "--1", strings.Repeat("1", MaxDecimalLength+1)}
	for _, input := range invalid {
		if _, err := ParseDecimal(input); err == nil {
			t.Errorf("ParseDecimal accepted %q", input)
		}
	}
}

func TestProbabilityBoundsAndPrecision(t *testing.T) {
	t.Parallel()

	for _, input := range []string{"0", "1", "0.1", "0.123456789012345678"} {
		if _, err := ParseProbability(input); err != nil {
			t.Errorf("ParseProbability(%q): %v", input, err)
		}
	}
	for _, input := range []string{"-0.1", "1.1", "2", "0.1234567890123456789"} {
		if _, err := ParseProbability(input); err == nil {
			t.Errorf("ParseProbability accepted %q", input)
		}
	}
	for _, input := range []string{"0", "1", "-0.1", "0.1234567890123456789"} {
		if _, err := ParseOpenProbability(input); err == nil {
			t.Errorf("ParseOpenProbability accepted %q", input)
		}
	}
}

func TestExactComparisonAdditionRangeAndAlignment(t *testing.T) {
	t.Parallel()

	oneTenth, _ := ParseDecimal("0.1")
	twoTenths, _ := ParseDecimal("0.2")
	threeTenths, _ := ParseDecimal("0.3")
	if sum := oneTenth.Add(twoTenths); sum.Cmp(threeTenths) != 0 || sum.String() != "0.3" {
		t.Fatalf("0.1 + 0.2 = %s", sum.String())
	}
	negative, _ := ParseDecimal("-12.5")
	zero := Zero()
	if negative.Cmp(zero) >= 0 || !InRange(zero, &negative, true, &threeTenths, false) {
		t.Fatal("comparison or range check failed")
	}
	step, _ := ParseDecimal("0.05")
	origin, _ := ParseDecimal("-0.1")
	aligned, _ := ParseDecimal("0.15")
	misaligned, _ := ParseDecimal("0.151")
	if !Aligns(aligned, origin, step) || Aligns(misaligned, origin, step) {
		t.Fatal("exact step alignment failed")
	}
}

func TestIntegerAdditionPropertyAcrossScales(t *testing.T) {
	t.Parallel()

	for left := -100; left <= 100; left++ {
		for right := -100; right <= 100; right++ {
			leftValue, _ := ParseDecimal(integerString(left))
			rightValue, _ := ParseDecimal(integerString(right))
			want, _ := ParseDecimal(integerString(left + right))
			if got := leftValue.Add(rightValue); got.Cmp(want) != 0 {
				t.Fatalf("%d + %d = %s", left, right, got.String())
			}
		}
	}
}

func FuzzCanonicalDecimalRoundTrip(f *testing.F) {
	for _, seed := range []string{"0", "-12", "0.123456789012345678", "01", "1e3", "-0"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		value, err := ParseDecimal(input)
		if err != nil {
			return
		}
		if value.String() != input {
			t.Fatalf("round trip changed %q to %q", input, value.String())
		}
	})
}

func integerString(value int) string {
	if value == 0 {
		return "0"
	}
	negative := value < 0
	if negative {
		value = -value
	}
	digits := ""
	for value > 0 {
		digits = string(rune('0'+value%10)) + digits
		value /= 10
	}
	if negative {
		return "-" + digits
	}
	return digits
}
