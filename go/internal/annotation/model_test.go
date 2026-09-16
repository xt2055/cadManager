package annotation

import (
	"math"
	"testing"
)

func TestValidateRejectsInvalidGeometry(t *testing.T) {
	good := Content{SchemaVersion: 1, Marks: []Mark{{ID: "mark", Kind: "rect", Layout: "Model", Points: []Point{{0, 0}, {10, 20}}, Color: "#FF6868", Width: 3}}}
	if err := Validate(good); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		change func(*Content)
	}{
		{"future schema", func(c *Content) { c.SchemaVersion = 2 }},
		{"missing layout", func(c *Content) { c.Marks[0].Layout = "" }},
		{"invalid kind", func(c *Content) { c.Marks[0].Kind = "script" }},
		{"NaN", func(c *Content) { c.Marks[0].Points[0].X = math.NaN() }},
		{"infinity", func(c *Content) { c.Marks[0].Width = math.Inf(1) }},
		{"bad bounds", func(c *Content) { c.Marks[0].Points = c.Marks[0].Points[:1] }},
		{"duplicate ID", func(c *Content) { c.Marks = append(c.Marks, c.Marks[0]) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Content{SchemaVersion: 1, Marks: append([]Mark(nil), good.Marks...)}
			c.Marks[0].Points = append([]Point(nil), good.Marks[0].Points...)
			tt.change(&c)
			if Validate(c) == nil {
				t.Fatal("invalid content accepted")
			}
		})
	}
}
