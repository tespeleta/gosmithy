package model_test

import (
	"testing"

	"github.com/tespeleta/gosmithy/model"
)

func TestRangeBoundsPreserveLexemes(t *testing.T) {
	m := assembleSources(t, `$version: "2.0"
namespace example

@range(min: -123456789012345678901234567890, max: 1.230000000000000000000000000001)
bigDecimal Precise
`)
	shape, ok := m.Shape(model.MustShapeID("example#Precise"))
	if !ok {
		t.Fatal("Precise shape not found")
	}
	min, max, ok := shape.RangeBounds()
	if !ok || min == nil || max == nil {
		t.Fatalf("RangeBounds() = %v, %v, %v", min, max, ok)
	}
	if *min != "-123456789012345678901234567890" {
		t.Errorf("min = %q", *min)
	}
	if *max != "1.230000000000000000000000000001" {
		t.Errorf("max = %q", *max)
	}
}
