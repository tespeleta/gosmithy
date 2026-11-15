package golang

import (
	"fmt"
	"math"
	"strconv"

	"github.com/tespeleta/gosmithy/model"
)

// constraints is the effective set of constraints for a member or shape.
// Member constraints override target shape constraints, per Smithy semantics.
type constraints struct {
	lengthMin   *int
	lengthMax   *int
	rangeMin    *float64
	rangeMax    *float64
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

func overrideRange(member, target *model.Shape) (min, max *float64) {
	if member != nil && member.HasTrait(model.TraitRange) {
		min, max, _ = member.Range()
		return min, max
	}
	if target != nil && target.HasTrait(model.TraitRange) {
		min, max, _ = target.Range()
		return min, max
	}
	return nil, nil
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
		model.TypeFloat, model.TypeDouble:
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
func rangeCondition(target *model.Shape, expr string, bound float64, min bool) (string, error) {
	op := "<"
	if min {
		op = "<"
	} else {
		op = ">"
	}
	switch target.Type {
	case model.TypeByte, model.TypeShort, model.TypeInteger, model.TypeLong:
		if isIntegral(bound) && bound >= math.MinInt64 && bound <= math.MaxInt64 {
			// Compare in int64 so a bound outside the Go type's range does not
			// produce an overflow in the generated constant expression.
			return fmt.Sprintf("int64(%s) %s %d", expr, op, int64(bound)), nil
		}
		literal, err := float64Literal(bound)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("float64(%s) %s %s", expr, op, literal), nil
	case model.TypeFloat:
		literal, err := float32Literal(bound)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s %s float32(%s)", expr, op, literal), nil
	case model.TypeDouble:
		literal, err := float64Literal(bound)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s %s %s", expr, op, literal), nil
	default:
		return "", fmt.Errorf("range constraints on %s shapes are not supported", target.Type)
	}
}

func rangeMessage(target *model.Shape, bound float64, min bool) string {
	relation := "less than or equal to"
	if min {
		relation = "greater than or equal to"
	}
	return "must be " + relation + " " + formatRangeFloat(bound)
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
