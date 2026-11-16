package golang

import (
	"fmt"
	"math"
	"math/big"
	"strconv"

	"github.com/tespeleta/gosmithy/model"
)

// rangeBound is one @range bound. text keeps the original Smithy number
// lexeme, which is required for bigInteger/bigDecimal; value is the parsed
// float used by byte/short/integer/long/float/double.
type rangeBound struct {
	text     string
	value    float64
	hasValue bool
}

// constraints is the effective set of constraints for a member or shape.
// Member constraints override target shape constraints, per Smithy semantics.
type constraints struct {
	lengthMin   *int
	lengthMax   *int
	rangeMin    *rangeBound
	rangeMax    *rangeBound
	pattern     *string
	uniqueItems bool
}

// constraintsFor resolves effective constraints from a member (which may be
// nil) and its target shape.
func (g *generator) constraintsFor(member, target *model.Shape) constraints {
	var out constraints
	out.lengthMin, out.lengthMax = overrideLength(member, target)
	out.rangeMin, out.rangeMax = overrideRange(member, target)
	out.pattern = overridePattern(member, target)
	if member != nil && member.HasTrait(model.TraitUniqueItems) {
		out.uniqueItems = true
	} else if target != nil && target.HasTrait(model.TraitUniqueItems) {
		out.uniqueItems = true
	}
	return out
}

func overrideLength(member, target *model.Shape) (min, max *int) {
	if member != nil && member.HasTrait(model.TraitLength) {
		min, max, _ = member.Length()
		return min, max
	}
	if target != nil && target.HasTrait(model.TraitLength) {
		min, max, _ = target.Length()
		return min, max
	}
	return nil, nil
}

func overrideRange(member, target *model.Shape) (min, max *rangeBound) {
	var shape *model.Shape
	switch {
	case member != nil && member.HasTrait(model.TraitRange):
		shape = member
	case target != nil && target.HasTrait(model.TraitRange):
		shape = target
	default:
		return nil, nil
	}
	minFloat, maxFloat, _ := shape.Range()
	minText, maxText, _ := shape.RangeBounds()
	resolved := target
	if resolved == nil {
		resolved = member
	}
	return newRangeBound(resolved, minText, minFloat), newRangeBound(resolved, maxText, maxFloat)
}

func newRangeBound(target *model.Shape, text *string, value *float64) *rangeBound {
	if text == nil {
		return nil
	}
	if target != nil && (target.Type == model.TypeBigInteger || target.Type == model.TypeBigDecimal) {
		return &rangeBound{text: *text}
	}
	// Preserve the previous behavior for non-Big types: a bound that cannot be
	// represented as a float is silently skipped by the generator.
	if value == nil {
		return nil
	}
	return &rangeBound{text: *text, value: *value, hasValue: true}
}

func overridePattern(member, target *model.Shape) *string {
	var shape *model.Shape
	switch {
	case member != nil && member.HasTrait(model.TraitPattern):
		shape = member
	case target != nil && target.HasTrait(model.TraitPattern):
		shape = target
	default:
		return nil
	}
	pattern, ok := shape.Pattern()
	if !ok {
		return nil
	}
	return &pattern
}

// hasApplicable reports whether any effective constraint applies to target.
func (c constraints) hasApplicable(target *model.Shape) bool {
	if target == nil {
		return false
	}
	switch target.Type {
	case model.TypeString:
		return c.lengthMin != nil || c.lengthMax != nil || c.pattern != nil
	case model.TypeBlob:
		return c.lengthMin != nil || c.lengthMax != nil
	case model.TypeList, model.TypeSet:
		return c.lengthMin != nil || c.lengthMax != nil || c.uniqueItems
	case model.TypeMap:
		return c.lengthMin != nil || c.lengthMax != nil
	case model.TypeByte, model.TypeShort, model.TypeInteger, model.TypeLong,
		model.TypeFloat, model.TypeDouble, model.TypeBigInteger, model.TypeBigDecimal:
		return c.rangeMin != nil || c.rangeMax != nil
	default:
		return false
	}
}

// memberOverrides reports whether member declares a constraint applicable to
// target. If it does, the target validator's shape-level constraints must not
// be used wholesale; the generator applies effective constraints inline and
// walks the collection contents itself.
func (g *generator) memberOverrides(member, target *model.Shape) bool {
	if member == nil || target == nil {
		return false
	}
	if member.HasTrait(model.TraitLength) {
		switch target.Type {
		case model.TypeString, model.TypeBlob, model.TypeList, model.TypeSet, model.TypeMap:
			return true
		}
	}
	if member.HasTrait(model.TraitRange) {
		switch target.Type {
		case model.TypeByte, model.TypeShort, model.TypeInteger, model.TypeLong,
			model.TypeFloat, model.TypeDouble, model.TypeBigInteger, model.TypeBigDecimal:
			return true
		}
	}
	if member.HasTrait(model.TraitPattern) && target.Type == model.TypeString {
		return true
	}
	if member.HasTrait(model.TraitUniqueItems) {
		switch target.Type {
		case model.TypeList, model.TypeSet:
			return true
		}
	}
	return false
}

func lengthNoun(target *model.Shape) (quantity, unit string) {
	switch target.Type {
	case model.TypeString:
		return "length", "character"
	case model.TypeBlob:
		return "length", "byte"
	default:
		return "size", "item"
	}
}

func lengthExpression(target *model.Shape, expr string) (string, bool) {
	switch target.Type {
	case model.TypeString:
		return "utf8.RuneCountInString(" + expr + ")", true
	case model.TypeBlob, model.TypeList, model.TypeSet, model.TypeMap:
		return "len(" + expr + ")", true
	default:
		return "", false
	}
}

func plural(n int, singular string) string {
	if n == 1 {
		return singular
	}
	return singular + "s"
}

func lengthMessage(min bool, n int, unit string) string {
	if min {
		return fmt.Sprintf("must be at least %d %s", n, plural(n, unit))
	}
	return fmt.Sprintf("must be at most %d %s", n, plural(n, unit))
}

// rangeCondition renders the comparison for one @range bound.
func rangeCondition(target *model.Shape, expr string, bound *rangeBound, min bool) (string, error) {
	op := ">"
	if min {
		op = "<"
	}
	switch target.Type {
	case model.TypeByte, model.TypeShort, model.TypeInteger, model.TypeLong:
		if !bound.hasValue {
			return "", fmt.Errorf("@range bound %q is not representable", bound.text)
		}
		if isIntegral(bound.value) && bound.value >= math.MinInt64 && bound.value <= math.MaxInt64 {
			// Compare in int64 so a bound outside the Go type's range does not
			// produce an overflow in the generated constant expression.
			return fmt.Sprintf("int64(%s) %s %d", expr, op, int64(bound.value)), nil
		}
		literal, err := float64Literal(bound.value)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("float64(%s) %s %s", expr, op, literal), nil
	case model.TypeFloat:
		if !bound.hasValue {
			return "", fmt.Errorf("@range bound %q is not representable", bound.text)
		}
		literal, err := float32Literal(bound.value)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s %s float32(%s)", expr, op, literal), nil
	case model.TypeDouble:
		if !bound.hasValue {
			return "", fmt.Errorf("@range bound %q is not representable", bound.text)
		}
		literal, err := float64Literal(bound.value)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s %s %s", expr, op, literal), nil
	case model.TypeBigInteger:
		literal, err := bigIntegerLiteral(bound.text)
		if err != nil {
			return "", err
		}
		helper := "bigIntGreater"
		if min {
			helper = "bigIntLess"
		}
		return fmt.Sprintf("%s(%s, %s)", helper, expr, quoted(literal)), nil
	case model.TypeBigDecimal:
		helper := "decimalGreater"
		if min {
			helper = "decimalLess"
		}
		return fmt.Sprintf("%s(%s, Decimal(%s))", helper, expr, quoted(bound.text)), nil
	default:
		return "", fmt.Errorf("range constraints on %s shapes are not supported", target.Type)
	}
}

// bigIntegerLiteral normalizes a Smithy number lexeme to an exact base-10
// integer literal. Fractional bounds on bigInteger are rejected by model
// validation; this guards codegen independently.
func bigIntegerLiteral(text string) (string, error) {
	r, ok := new(big.Rat).SetString(text)
	if !ok {
		return "", fmt.Errorf("invalid @range bound %q", text)
	}
	if !r.IsInt() {
		return "", fmt.Errorf("@range bound %q is not an integer", text)
	}
	return r.Num().String(), nil
}

func rangeMessage(target *model.Shape, bound *rangeBound, min bool) string {
	relation := "less than or equal to"
	if min {
		relation = "greater than or equal to"
	}
	switch target.Type {
	case model.TypeBigInteger, model.TypeBigDecimal:
		return "must be " + relation + " " + bound.text
	default:
		return "must be " + relation + " " + formatRangeFloat(bound.value)
	}
}

func isIntegral(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v == math.Trunc(v)
}

func float32Literal(v float64) (string, error) {
	f := float32(v)
	if math.IsInf(float64(f), 0) || math.IsNaN(float64(f)) {
		return "", fmt.Errorf("range bound %v is not a finite float32", v)
	}
	return strconv.FormatFloat(float64(f), 'g', -1, 32), nil
}

func float64Literal(v float64) (string, error) {
	if math.IsInf(v, 0) || math.IsNaN(v) {
		return "", fmt.Errorf("range bound %v is not a finite float64", v)
	}
	return strconv.FormatFloat(v, 'g', -1, 64), nil
}

func formatRangeFloat(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}
