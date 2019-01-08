// Copyright 2018 Ulf Adams
// Modifications copyright 2019 Caleb Spare
//
// The contents of this file may be used under the terms of the Apache License,
// Version 2.0.
//
//    (See accompanying file LICENSE or copy at
//     http://www.apache.org/licenses/LICENSE-2.0)
//
// Unless required by applicable law or agreed to in writing, this software
// is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.
//
// The code in this file is part of a Go translation of the C code written by
// Ulf Adams which may be found at https://github.com/ulfjack/ryu. That source
// code is licensed under Apache 2.0 and this code is derivative work thereof.

package ryu

import (
	"math/bits"
)

type uint128 struct {
	lo uint64
	hi uint64
}

// dec64 is a floating decimal type representing m * 10^e.
type dec64 struct {
	m uint64
	e int32
}

func (d dec64) append(b []byte, neg bool) []byte {
	// Step 5: Print the decimal representation.
	if neg {
		b = append(b, '-')
	}

	out := d.m
	outLen := decimalLen64(out)
	bufLen := outLen
	if bufLen > 1 {
		bufLen++ // extra space for '.'
	}

	// Print the decimal digits.
	// FIXME: optimize.
	n := len(b)
	if cap(b)-len(b) >= bufLen {
		// Avoid function call in the common case.
		b = b[:len(b)+bufLen]
	} else {
		b = append(b, make([]byte, bufLen)...)
	}
	for i := 0; i < outLen-1; i++ {
		b[n+outLen-i] = '0' + byte(out%10)
		out /= 10
	}
	b[n] = '0' + byte(out%10)

	// Print the '.' if needed.
	if outLen > 1 {
		b[n+1] = '.'
	}

	// Print the exponent.
	b = append(b, 'e')
	exp := d.e + int32(outLen) - 1
	if exp < 0 {
		b = append(b, '-')
		exp = -exp
	} else {
		// Unconditionally print a + here to match strconv's formatting.
		b = append(b, '+')
	}
	// Always print at least two digits to match strconv's formatting.
	d2 := exp % 10
	exp /= 10
	d1 := exp % 10
	d0 := exp / 10
	if d0 > 0 {
		b = append(b, '0'+byte(d0))
	}
	b = append(b, '0'+byte(d1), '0'+byte(d2))

	return b
}

func float64ToDecimalExactInt(mant, exp uint64) (d dec64, ok bool) {
	e := exp - bias64
	if e > mantBits64 {
		return d, false
	}
	shift := mantBits64 - e
	mant |= 1 << mantBits64 // implicit 1
	d.m = mant >> shift
	if d.m<<shift != mant {
		return d, false
	}

	for d.m%10 == 0 {
		d.m /= 10
		d.e++
	}
	return d, true
}

func float64ToDecimal(mant, exp uint64) dec64 {
	var e2 int32
	var m2 uint64
	if exp == 0 {
		// We subtract 2 so that the bounds computation has
		// 2 additional bits.
		e2 = 1 - bias64 - mantBits64 - 2
		m2 = mant
	} else {
		e2 = int32(exp) - bias64 - mantBits64 - 2
		m2 = uint64(1)<<mantBits64 | mant
	}
	even := m2&1 == 0
	acceptBounds := even

	// Step 2: Determine the interval of valid decimal representations.
	mv := 4 * m2
	var mmShift uint64
	if mant != 0 || exp <= 1 {
		mmShift = 1
	}
	// We would compute mp and mm like this:
	// mp := 4 * m2 + 2;
	// mm := mv - 1 - mmShift;

	// Step 3: Convert to a decimal power base uing 128-bit arithmetic.
	var (
		vr, vp, vm        uint64
		e10               int32
		vmIsTrailingZeros bool
		vrIsTrailingZeros bool
	)
	if e2 >= 0 {
		// This expression is slightly faster than max(0, log10Pow2(e2) - 1).
		// FIXME: Confirm.
		q := log10Pow2(e2)
		if e2 > 3 {
			q--
		}
		e10 = int32(q)
		k := pow5InvNumBits64 + pow5Bits(int32(q)) - 1
		i := -e2 + int32(q) + k
		mul := pow5InvSplit64[q]
		vr = mulShift64(4*m2, mul, i)
		vp = mulShift64(4*m2+2, mul, i)
		vm = mulShift64(4*m2-1-mmShift, mul, i)
		if q <= 21 {
			// This should use q <= 22, but I think 21 is also safe.
			// Smaller values may still be safe, but it's more
			// difficult to reason about them. Only one of mp, mv,
			// and mm can be a multiple of 5, if any.
			if mv%5 == 0 {
				vrIsTrailingZeros = multipleOfPowerOfFive64(mv, q)
			} else if acceptBounds {
				// Same as min(e2 + (^mm & 1), pow5Factor64(mm)) >= q
				// <=> e2 + (^mm & 1) >= q && pow5Factor64(mm) >= q
				// <=> true && pow5Factor64(mm) >= q, since e2 >= q.
				vmIsTrailingZeros = multipleOfPowerOfFive64(mv-1-mmShift, q)
			} else {
				if multipleOfPowerOfFive64(mv+2, q) {
					vp--
				}
			}
		}
	} else {
		// This expression is slightly faster than max(0, log10Pow5(-e2) - 1).
		// FIXME: confirm
		q := log10Pow5(-e2)
		if -e2 > 1 {
			q--
		}
		e10 = int32(q) + e2
		i := -e2 - int32(q)
		k := pow5Bits(i) - pow5NumBits64
		j := int32(q) - k
		mul := pow5Split64[i]
		vr = mulShift64(4*m2, mul, j)
		vp = mulShift64(4*m2+2, mul, j)
		vm = mulShift64(4*m2-1-mmShift, mul, j)
		if q <= 1 {
			// {vr,vp,vm} is trailing zeros if {mv,mp,mm} has at least q trailing 0 bits.
			// mv = 4 * m2, so it always has at least two trailing 0 bits.
			vrIsTrailingZeros = true
			if acceptBounds {
				// mm = mv - 1 - mmShift, so it has 1 trailing 0 bit iff mmShift == 1.
				vmIsTrailingZeros = mmShift == 1
			} else {
				// mp = mv + 2, so it always has at least one trailing 0 bit.
				vp--
			}
		} else if q < 63 { // TODO(ulfjack/cespare): Use a tighter bound here.
			// We need to compute min(ntz(mv), pow5Factor64(mv) - e2) >= q - 1
			// <=> ntz(mv) >= q - 1 && pow5Factor64(mv) - e2 >= q - 1
			// <=> ntz(mv) >= q - 1 (e2 is negative and -e2 >= q)
			// <=> (mv & ((1 << (q - 1)) - 1)) == 0
			// We also need to make sure that the left shift does not overflow.
			vrIsTrailingZeros = multipleOfPowerOfTwo64(mv, q-1)
		}
	}

	// Step 4: Find the shortest decimal representation
	// in the interval of valid representations.
	var removed int32
	var lastRemovedDigit uint8
	var out uint64
	// On average, we remove ~2 digits.
	if vmIsTrailingZeros || vrIsTrailingZeros {
		// General case, which happens rarely (~0.7%).
		for {
			vpDiv10 := vp / 10
			vmDiv10 := vm / 10
			if vpDiv10 <= vmDiv10 {
				break
			}
			vmMod10 := vm % 10
			vrDiv10 := vr / 10
			vrMod10 := vr % 10
			vmIsTrailingZeros = vmIsTrailingZeros && vmMod10 == 0
			vrIsTrailingZeros = vrIsTrailingZeros && lastRemovedDigit == 0
			lastRemovedDigit = uint8(vrMod10)
			vr = vrDiv10
			vp = vpDiv10
			vm = vmDiv10
			removed++
		}
		if vmIsTrailingZeros {
			for {
				vmDiv10 := vm / 10
				vmMod10 := vm % 10
				if vmMod10 != 0 {
					break
				}
				vpDiv10 := vp / 10
				vrDiv10 := vr / 10
				vrMod10 := vr % 10
				vrIsTrailingZeros = vrIsTrailingZeros && lastRemovedDigit == 0
				lastRemovedDigit = uint8(vrMod10)
				vr = vrDiv10
				vp = vpDiv10
				vm = vmDiv10
				removed++
			}
		}
		if vrIsTrailingZeros && lastRemovedDigit == 5 && vr%2 == 0 {
			// Round even if the exact number is .....50..0.
			lastRemovedDigit = 4
		}
		out = vr
		// We need to take vr + 1 if vr is outside bounds
		// or we need to round up.
		if (vr == vm && (!acceptBounds || !vmIsTrailingZeros)) || lastRemovedDigit >= 5 {
			out++
		}
	} else {
		// Specialized for the common case (~99.3%).
		// Percentages below are relative to this.
		roundUp := false
		if vp/100 > vm/100 {
			// Optimization: remove two digits at a time (~86.2%).
			roundUp = vr%100 >= 50
			vr /= 100
			vp /= 100
			vm /= 100
			removed += 2
		}
		// Loop iterations below (approximately), without optimization above:
		// 0: 0.03%, 1: 13.8%, 2: 70.6%, 3: 14.0%, 4: 1.40%, 5: 0.14%, 6+: 0.02%
		// Loop iterations below (approximately), with optimization above:
		// 0: 70.6%, 1: 27.8%, 2: 1.40%, 3: 0.14%, 4+: 0.02%
		for vp/10 > vm/10 {
			roundUp = vr%10 >= 5
			vr /= 10
			vp /= 10
			vm /= 10
			removed++
		}
		out = vr
		// We need to take vr + 1 if vr is outside bounds
		// or we need to round up.
		if vr == vm || roundUp {
			out++
		}
	}

	return dec64{m: out, e: e10 + removed}
}

func decimalLen64(u uint64) int {
	// This is slightly faster than a loop. FIXME: Confirm.
	// The average output length is 16.38 digits, so we check high-to-low.
	// Function precondition: v is not an 18, 19, or 20-digit number.
	// (17 digits are sufficient for round-tripping.)
	assert(u < 100000000000000000, "too big")
	switch {
	case u >= 10000000000000000:
		return 17
	case u >= 1000000000000000:
		return 16
	case u >= 100000000000000:
		return 15
	case u >= 10000000000000:
		return 14
	case u >= 1000000000000:
		return 13
	case u >= 100000000000:
		return 12
	case u >= 10000000000:
		return 11
	case u >= 1000000000:
		return 10
	case u >= 100000000:
		return 9
	case u >= 10000000:
		return 8
	case u >= 1000000:
		return 7
	case u >= 100000:
		return 6
	case u >= 10000:
		return 5
	case u >= 1000:
		return 4
	case u >= 100:
		return 3
	case u >= 10:
		return 2
	default:
		return 1
	}
}

func mulShift64(m uint64, mul uint128, shift int32) uint64 {
	hihi, hilo := bits.Mul64(m, mul.hi)
	lohi, _ := bits.Mul64(m, mul.lo)
	sum := uint128{hi: hihi, lo: lohi + hilo}
	if sum.lo < lohi {
		sum.hi++ // overflow
	}
	return shiftRight128(sum, shift-64)
}

func shiftRight128(v uint128, shift int32) uint64 {
	// The shift value is always modulo 64.
	// In the current implementation of the 64-bit version
	// of Ryu, the shift value is always < 64.
	// (It is in the range [2, 59].)
	// Check this here in case a future change requires larger shift
	// values. In this case this function needs to be adjusted.
	assert(shift < 64, "shift < 64")
	// FIXME: optimize
	return (v.hi << uint64(64-shift)) | (v.lo >> uint(shift))
}

func pow5Factor64(v uint64) uint32 {
	for n := uint32(0); ; n++ {
		q, r := v/5, v%5
		if r != 0 {
			return n
		}
		v = q
	}
}

func multipleOfPowerOfFive64(v uint64, p uint32) bool {
	return pow5Factor64(v) >= p
}

func multipleOfPowerOfTwo64(v uint64, p uint32) bool {
	return uint32(bits.TrailingZeros64(v)) >= p
}
