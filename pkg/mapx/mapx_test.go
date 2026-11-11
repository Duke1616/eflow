package mapx

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

type userProfile struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

type testInt int16

func TestMapx_Core(t *testing.T) {
	t.Run("nil_and_defaults", func(t *testing.T) {
		var m Map
		score := NewKey[int]("score", 10)
		role := NewKey[string]("role", "unknown")

		assert.Equal(t, 10, score.Get(m))
		assert.Equal(t, "unknown", role.Get(m))
		assert.Equal(t, 0, NewKey[int]("score").Get(m))

		v, ok := score.GetOK(m)
		assert.False(t, ok)
		assert.Equal(t, 0, v)

		m.Set("days", 7)
		assert.NotNil(t, m)
		assert.Equal(t, 7, m["days"])
		assert.True(t, m.Has("days"))
		assert.False(t, m.Has("missing"))
	})

	t.Run("numeric_and_string_conversions", func(t *testing.T) {
		m := Map{
			"float_num":  float64(42),
			"str_num":    "128",
			"json_num":   json.Number("256"),
			"fraction":   1.5,
			"int8_large": 128,
			"uint64_max": json.Number("18446744073709551615"),
			"flag_true":  "true",
			"flag_1":     1,
			"flag_false": "0",
			"flag_two":   2,
		}

		assert.Equal(t, 42, NewKey[int]("float_num").Get(m))
		assert.Equal(t, int64(42), NewKey[int64]("float_num").Get(m))
		assert.Equal(t, 128, NewKey[int]("str_num").Get(m))
		assert.Equal(t, int64(256), NewKey[int64]("json_num").Get(m))
		assert.Equal(t, int(99), NewKey[int]("fraction", 99).Get(m))
		assert.Equal(t, int8(7), NewKey[int8]("int8_large", 7).Get(m))
		assert.Equal(t, ^uint64(0), NewKey[uint64]("uint64_max").Get(m))
		assert.Equal(t, testInt(12), NewKey[testInt]("named_int").Get(Map{"named_int": int64(12)}))

		_, ok := NewKey[int]("fraction").GetOK(m)
		assert.False(t, ok)
		_, ok = NewKey[int64]("uint64_max").GetOK(m)
		assert.False(t, ok)

		assert.True(t, NewKey[bool]("flag_true").Get(m))
		assert.True(t, NewKey[bool]("flag_1").Get(m))
		assert.True(t, NewKey[bool]("flag_two").Get(m))
		assert.False(t, NewKey[bool]("flag_false").Get(m))
	})

	t.Run("key_struct_and_slice", func(t *testing.T) {
		keyDays := NewKey[int]("days", 3)
		var m Map
		assert.Equal(t, 3, keyDays.Get(m))

		keyDays.Set(&m, 10)
		assert.Equal(t, 10, keyDays.Get(m))

		keyUser := NewKey[userProfile]("user")
		m.Set("user", map[string]any{"name": "duke", "age": 28})
		user := keyUser.Get(m)
		assert.Equal(t, "duke", user.Name)
		assert.Equal(t, 28, user.Age)

		m.Set("arr", []any{"1", 2, "three"})
		assert.Equal(t, []string{"1", "2", "three"}, NewKey[[]string]("arr").Get(m))
	})

	t.Run("merge", func(t *testing.T) {
		var target Map
		target.Merge(Map{"a": 1})
		target.Merge(Map{"b": 2})
		assert.Equal(t, 1, target["a"])
		assert.Equal(t, 2, target["b"])

		clone := target.Clone()
		clone.Set("a", 9)
		assert.Equal(t, 1, target.GetInt("a"))
		assert.Equal(t, 9, clone.GetInt("a"))
	})

	t.Run("bind", func(t *testing.T) {
		var profile userProfile
		m := Map{"user": map[string]any{"name": "duke", "age": 28}}
		err := m.Bind("user", &profile)

		assert.NoError(t, err)
		assert.Equal(t, userProfile{Name: "duke", Age: 28}, profile)
	})
}

func TestNumericAndBooleanConversionBoundaries(t *testing.T) {
	tests := []struct {
		name string
		raw  any
		want any
		ok   bool
	}{
		{name: "signed alias", raw: testInt(-12), want: int64(-12), ok: true},
		{name: "unsigned maximum", raw: ^uint64(0), want: uint64(^uint64(0)), ok: true},
		{name: "unsigned overflow to signed", raw: ^uint64(0), want: int64(0), ok: false},
		{name: "integral float", raw: float64(12), want: int64(12), ok: true},
		{name: "fractional float", raw: float64(12.5), want: int64(0), ok: false},
		{name: "float overflow", raw: math.Exp2(63), want: int64(0), ok: false},
		{name: "NaN", raw: math.NaN(), want: int64(0), ok: false},
		{name: "bool numeric one", raw: json.Number("1.0"), want: true, ok: true},
		{name: "bool numeric zero", raw: float32(0), want: false, ok: true},
		{name: "bool numeric other", raw: int16(2), want: true, ok: true},
		{name: "bool numeric float", raw: float64(0.5), want: true, ok: true},
		{name: "bool non-finite", raw: math.NaN(), want: false, ok: false},
		{name: "bool named string", raw: testString(" YES "), want: true, ok: true},
		{name: "bool invalid string", raw: "on", want: false, ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got any
			var ok bool
			switch tt.want.(type) {
			case int64:
				got, ok = convert[int64](tt.raw)
			case uint64:
				got, ok = convert[uint64](tt.raw)
			case bool:
				got, ok = convert[bool](tt.raw)
			}
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMap_DeepClone_AdvancedTypes(t *testing.T) {
	orig := Map{
		"string_map": map[string]string{"k1": "v1"},
		"float_slice": []float64{1.1, 2.2},
	}

	cloned := orig.DeepClone()

	// 修改深拷贝结构中的元素，验证对原始对象无副作用
	if sm, ok := cloned["string_map"].(map[string]any); ok {
		sm["k1"] = "mutated"
	} else if sm, ok := cloned["string_map"].(map[string]string); ok {
		sm["k1"] = "mutated"
	}

	assert.Equal(t, "v1", orig["string_map"].(map[string]string)["k1"])
}

type testString string
