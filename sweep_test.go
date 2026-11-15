package aprs_test

import (
	"errors"
	"math"
	"reflect"
	"testing"

	aprs "github.com/packet-net/aprs-go"
	"github.com/packet-net/aprs-go/internal/neutral"
)

// The sweep: a hostile number in EVERY numeric field of every data type, at depth 0, 1 and 2 of
// third-party nesting, and in every numeric argument of every builder. Each call must either
// refuse with an *EncodeError, or write bytes; it never panics; and a NaN or an infinity is
// never written, wherever it is.

var hostileFloats = []float64{
	math.NaN(), math.Inf(1), math.Inf(-1), math.MaxFloat64, -math.MaxFloat64,
	math.SmallestNonzeroFloat64, math.Copysign(0, -1), 0.5, -0.5, 1e-9, 12345678901234567890,
}

// numericLeaves calls visit for every number (float or integer) held in v, never going into a
// third-party packet's inner packet (its own level).
func numericLeaves(v reflect.Value, visit func(reflect.Value)) {
	switch v.Kind() {
	case reflect.Float32, reflect.Float64, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if v.CanSet() {
			visit(v)
		}
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() || v.Type() == reflect.TypeOf((*aprs.Packet)(nil)) {
			return
		}
		numericLeaves(v.Elem(), visit)
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			numericLeaves(v.Field(i), visit)
		}
	case reflect.Slice, reflect.Array:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return
		}
		for i := 0; i < v.Len(); i++ {
			numericLeaves(v.Index(i), visit)
		}
	}
}

// setHostile puts a hostile value in a number: a float as it is, an integer as the nearest it holds.
func setHostile(v reflect.Value, f float64) {
	switch v.Kind() {
	case reflect.Float32, reflect.Float64:
		v.SetFloat(f)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		bits := uint(v.Type().Bits())
		max := float64(uint64(1)<<(bits-1)) - 1
		switch {
		case math.IsNaN(f), f == 0, math.Abs(f) < 1:
			v.SetInt(0)
		case f > max || math.IsInf(f, 1):
			v.SetInt(int64(max))
		case f < -max || math.IsInf(f, -1):
			v.SetInt(-int64(max) - 1)
		default:
			v.SetInt(int64(f))
		}
	default:
		bits := uint(v.Type().Bits())
		max := float64(^uint64(0) >> (64 - bits))
		switch {
		case math.IsNaN(f), f <= 0, math.Abs(f) < 1:
			v.SetUint(0)
		case f > max || math.IsInf(f, 1):
			v.SetUint(uint64(max))
		default:
			v.SetUint(uint64(f))
		}
	}
}

func isNonFinite(f float64) bool { return math.IsNaN(f) || math.IsInf(f, 0) }

// sweepFresh gives data read afresh from a vectors encode case (nil when this package cannot hold it).
func sweepFresh(c vectorCase) aprs.Data {
	in := c["input"].(map[string]any)
	e, ok := in["encode"]
	if !ok {
		return nil
	}
	d, unreadable := neutral.ReadData(e)
	if unreadable != nil {
		return nil
	}
	return d
}

func TestSweepEveryNumberAtEveryDepth(t *testing.T) {
	cases := loadCases(t)
	n0call, apz := aprs.MustAddress("N0CALL"), aprs.MustAddress("APZ001")
	informationOf := func(d aprs.Data) []byte {
		p, err := aprs.NewPacket(n0call, apz, nil, d)
		if err != nil {
			return []byte(">x")
		}
		return p.Information
	}
	wrap := func(depth int, d aprs.Data, info []byte) aprs.Data {
		for ; depth > 0; depth-- {
			d = aprs.ThirdParty{Packet: &aprs.Packet{Source: n0call, Destination: apz, Information: info, Data: d}}
			info = append([]byte("}N0CALL>APZ001:"), info...)
		}
		return d
	}
	writes := 0
	for _, c := range cases {
		base := sweepFresh(c)
		if base == nil {
			continue
		}
		info := informationOf(base)
		count := 0
		root := reflect.New(reflect.TypeOf(base)).Elem()
		root.Set(reflect.ValueOf(base))
		numericLeaves(root, func(reflect.Value) { count++ })
		for leaf := 0; leaf < count; leaf++ {
			for _, f := range hostileFloats {
				for depth := 0; depth <= 2; depth++ {
					fresh := sweepFresh(c)
					cp := reflect.New(reflect.TypeOf(fresh)).Elem()
					cp.Set(reflect.ValueOf(fresh))
					i := 0
					var leafValue reflect.Value
					numericLeaves(cp, func(v reflect.Value) {
						if i == leaf {
							leafValue = v
						}
						i++
					})
					setHostile(leafValue, f)
					nonFinite := leafValue.Kind() == reflect.Float64 && isNonFinite(leafValue.Float())
					data := wrap(depth, cp.Interface().(aprs.Data), info)
					var out []byte
					var err error
					func() {
						defer func() {
							if r := recover(); r != nil {
								t.Fatalf("%s leaf %d value %v depth %d: panic %v", c["id"], leaf, f, depth, r)
							}
						}()
						if _, micE := data.(aprs.MicEReport); micE {
							var p *aprs.Packet
							p, err = aprs.NewPacket(n0call, apz, nil, data)
							if p != nil {
								out = p.Information
							}
						} else {
							out, err = aprs.Encode(data)
						}
					}()
					if err != nil {
						var e *aprs.EncodeError
						if !errors.As(err, &e) {
							t.Fatalf("%s leaf %d value %v depth %d: refused with %T %v", c["id"], leaf, f, depth, err, err)
						}
						continue
					}
					writes++
					if nonFinite {
						t.Fatalf("%s leaf %d value %v depth %d: wrote %q for a number that is not one", c["id"], leaf, f, depth, out)
					}
				}
			}
		}
	}
	t.Logf("%d sweeps wrote bytes", writes)
}

// Every numeric argument of every builder: no panic, a refusal or bytes, and a NaN or an
// infinity in any of them is refused.
func TestSweepEveryBuilderArgument(t *testing.T) {
	st := me(t)
	builders := map[string]func() any{
		"position": func() any { return st.Position(51.5, -0.5) },
		"object":   func() any { return st.Object("OBJ").At(51.5, -0.5) },
		"item":     func() any { return st.Item("ITM").At(51.5, -0.5) },
		"mic-e":    func() any { return st.MicE(51.5, -0.5) },
		"weather":  func() any { return st.Weather() },
	}
	calls := 0
	defer func() {
		if calls < 200 {
			t.Errorf("only %d builder calls were swept", calls)
		}
	}()
	for name, make := range builders {
		typ := reflect.TypeOf(make())
		for m := 0; m < typ.NumMethod(); m++ {
			method := typ.Method(m)
			numeric := method.Type.NumIn() > 1
			for i := 1; i < method.Type.NumIn(); i++ {
				k := method.Type.In(i).Kind()
				numeric = numeric && (k == reflect.Float64 || k == reflect.Uint8 || k == reflect.Uint16 || k == reflect.Int32 || k == reflect.Int)
			}
			if !numeric {
				continue
			}
			for _, f := range hostileFloats {
				args := []reflect.Value{reflect.ValueOf(make())}
				for i := 1; i < method.Type.NumIn(); i++ {
					a := reflect.New(method.Type.In(i)).Elem()
					setHostile(a, f)
					args = append(args, a)
				}
				func() {
					defer func() {
						if r := recover(); r != nil {
							t.Fatalf("%s.%s(%v): panic %v", name, method.Name, f, r)
						}
					}()
					calls++
					out := method.Func.Call(args)
					b := out[0]
					build := b.MethodByName("Build")
					if !build.IsValid() {
						return
					}
					res := build.Call(nil)
					err, _ := res[1].Interface().(error)
					if err != nil {
						var e *aprs.EncodeError
						if !errors.As(err, &e) {
							t.Fatalf("%s.%s(%v): refused with %T %v", name, method.Name, f, err, err)
						}
						return
					}
					if isNonFinite(f) && method.Type.In(1).Kind() == reflect.Float64 {
						p := res[0].Interface().(*aprs.Packet)
						t.Fatalf("%s.%s(%v): wrote %q", name, method.Name, f, p.Information)
					}
				}()
			}
		}
	}
}
