// Package exact implements the bounded exact scalar arithmetic required by the
// Forecast Ledger v2 contract. It never converts authored decimals to float.
package exact

import (
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
)

const MaxDecimalLength = 128

var canonicalDecimalPattern = regexp.MustCompile(`^(?:0|-?[1-9][0-9]*|-?(?:0|[1-9][0-9]*)\.[0-9]*[1-9])$`)

type Decimal struct {
	coefficient big.Int
	scale       int
}

func ParseDecimal(value string) (Decimal, error) {
	if len(value) == 0 || len(value) > MaxDecimalLength || !canonicalDecimalPattern.MatchString(value) {
		return Decimal{}, fmt.Errorf("%q is not a canonical decimal string", value)
	}
	negative := strings.HasPrefix(value, "-")
	unsigned := strings.TrimPrefix(value, "-")
	parts := strings.SplitN(unsigned, ".", 2)
	digits := parts[0]
	scale := 0
	if len(parts) == 2 {
		digits += parts[1]
		scale = len(parts[1])
	}
	coefficient, ok := new(big.Int).SetString(digits, 10)
	if !ok {
		return Decimal{}, fmt.Errorf("parse decimal %q", value)
	}
	if negative {
		coefficient.Neg(coefficient)
	}
	return newDecimal(coefficient, scale), nil
}

func ParseProbability(value string) (Decimal, error) {
	decimal, err := ParseDecimal(value)
	if err != nil {
		return Decimal{}, err
	}
	if decimal.scale > 18 {
		return Decimal{}, errors.New("probability has more than 18 fractional digits")
	}
	zero := Zero()
	one := One()
	if decimal.Cmp(zero) < 0 || decimal.Cmp(one) > 0 {
		return Decimal{}, errors.New("probability must be in [0,1]")
	}
	return decimal, nil
}

func ParseOpenProbability(value string) (Decimal, error) {
	decimal, err := ParseProbability(value)
	if err != nil {
		return Decimal{}, err
	}
	if decimal.Cmp(Zero()) <= 0 || decimal.Cmp(One()) >= 0 {
		return Decimal{}, errors.New("probability must be strictly between 0 and 1")
	}
	return decimal, nil
}

func Zero() Decimal {
	return newDecimal(big.NewInt(0), 0)
}

func One() Decimal {
	return newDecimal(big.NewInt(1), 0)
}

func (d Decimal) String() string {
	coefficient := new(big.Int).Set(&d.coefficient)
	negative := coefficient.Sign() < 0
	coefficient.Abs(coefficient)
	digits := coefficient.String()
	if d.scale > 0 {
		if len(digits) <= d.scale {
			digits = strings.Repeat("0", d.scale-len(digits)+1) + digits
		}
		position := len(digits) - d.scale
		digits = digits[:position] + "." + digits[position:]
	}
	if negative {
		return "-" + digits
	}
	return digits
}

func (d Decimal) FractionalDigits() int {
	return d.scale
}

func (d Decimal) Sign() int {
	return d.coefficient.Sign()
}

func (d Decimal) Cmp(other Decimal) int {
	left, right := aligned(d, other)
	return left.Cmp(right)
}

func (d Decimal) Add(other Decimal) Decimal {
	left, right := aligned(d, other)
	return newDecimal(new(big.Int).Add(left, right), max(d.scale, other.scale))
}

func (d Decimal) Sub(other Decimal) Decimal {
	left, right := aligned(d, other)
	return newDecimal(new(big.Int).Sub(left, right), max(d.scale, other.scale))
}

func Sum(values ...Decimal) Decimal {
	result := Zero()
	for _, value := range values {
		result = result.Add(value)
	}
	return result
}

func InRange(value Decimal, lower *Decimal, lowerInclusive bool, upper *Decimal, upperInclusive bool) bool {
	if lower != nil {
		comparison := value.Cmp(*lower)
		if comparison < 0 || (comparison == 0 && !lowerInclusive) {
			return false
		}
	}
	if upper != nil {
		comparison := value.Cmp(*upper)
		if comparison > 0 || (comparison == 0 && !upperInclusive) {
			return false
		}
	}
	return true
}

func Aligns(value, origin, step Decimal) bool {
	if step.Sign() <= 0 {
		return false
	}
	difference := value.Sub(origin)
	left, right := aligned(difference, step)
	return new(big.Int).Rem(left, right).Sign() == 0
}

func newDecimal(coefficient *big.Int, scale int) Decimal {
	value := new(big.Int).Set(coefficient)
	if value.Sign() == 0 {
		return Decimal{coefficient: *big.NewInt(0)}
	}
	ten := big.NewInt(10)
	remainder := new(big.Int)
	for scale > 0 {
		remainder.Rem(value, ten)
		if remainder.Sign() != 0 {
			break
		}
		value.Quo(value, ten)
		scale--
	}
	return Decimal{coefficient: *value, scale: scale}
}

func aligned(left, right Decimal) (*big.Int, *big.Int) {
	scale := max(left.scale, right.scale)
	return scaleCoefficient(left, scale), scaleCoefficient(right, scale)
}

func scaleCoefficient(value Decimal, scale int) *big.Int {
	result := new(big.Int).Set(&value.coefficient)
	if difference := scale - value.scale; difference > 0 {
		result.Mul(result, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(difference)), nil))
	}
	return result
}
