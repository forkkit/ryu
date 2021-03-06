// Copyright 2018 Ulf Adams
// Modifications copyright 2019 Caleb Spare
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// The code in this file is a Go translation of the Java code written by
// Ulf Adams which may be found at
//
//     https://github.com/ulfjack/ryu/analysis/PrintFloatLookupTable.java
//     https://github.com/ulfjack/ryu/analysis/PrintDoubleLookupTable.java
//
// That source code is licensed under Apache 2.0 and this code is derivative
// work thereof.

// +build ignore

// This program generates tables.go.

package main

import (
	"bytes"
	"fmt"
	"go/format"
	"io/ioutil"
	"log"
	"math/big"
)

var header = []byte(`// Code generated by running "go generate". DO NOT EDIT.

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

`)

const (
	posTableSize32   = 47
	negTableSize32   = 31
	pow5NumBits32    = 61 // max 63
	pow5InvNumBits32 = 59 // max 63

	posTableSize64   = 326
	negTableSize64   = 291 + 1
	pow5NumBits64    = 121 // max 127
	pow5InvNumBits64 = 122 // max 127
)

func main() {
	b := bytes.NewBuffer(header)

	fmt.Fprintf(b, "const pow5NumBits32 = %d\n", pow5NumBits32)
	fmt.Fprintln(b, "var pow5Split32 = [...]uint64{")
	for i := int64(0); i < posTableSize32; i++ {
		pow5 := big.NewInt(5)
		pow5.Exp(pow5, big.NewInt(i), nil)
		shift := pow5.BitLen() - pow5NumBits32
		rsh(pow5, shift)
		fmt.Fprintf(b, "%d,", pow5.Uint64())
		if i%4 == 3 {
			fmt.Fprintln(b)
		}
	}
	fmt.Fprintln(b, "\n}")

	fmt.Fprintf(b, "const pow5InvNumBits32 = %d\n", pow5InvNumBits32)
	fmt.Fprintln(b, "var pow5InvSplit32 = [...]uint64{")
	for i := int64(0); i < negTableSize32; i++ {
		pow5 := big.NewInt(5)
		pow5.Exp(pow5, big.NewInt(i), nil)
		shift := pow5.BitLen() - 1 + pow5InvNumBits32
		inv := big.NewInt(1)
		rsh(inv, -shift)
		inv.Quo(inv, pow5)
		inv.Add(inv, big.NewInt(1))
		fmt.Fprintf(b, "%d,", inv.Uint64())
		if i%4 == 3 {
			fmt.Fprintln(b)
		}
	}
	fmt.Fprintln(b, "\n}")

	mask64 := big.NewInt(1)
	mask64.Lsh(mask64, 64)
	mask64.Sub(mask64, big.NewInt(1))

	fmt.Fprintf(b, "const pow5NumBits64 = %d\n", pow5NumBits64)
	fmt.Fprintln(b, "var pow5Split64 = [...]uint128{")
	for i := int64(0); i < posTableSize64; i++ {
		pow5 := big.NewInt(5)
		pow5.Exp(pow5, big.NewInt(i), nil)
		shift := pow5.BitLen() - pow5NumBits64
		rsh(pow5, shift)
		lo := new(big.Int).And(pow5, mask64)
		hi := new(big.Int).Rsh(pow5, 64)
		fmt.Fprintf(b, "{%d, %d},\n", lo.Uint64(), hi.Uint64())
	}
	fmt.Fprintln(b, "\n}")

	fmt.Fprintf(b, "const pow5InvNumBits64 = %d\n", pow5InvNumBits64)
	fmt.Fprintln(b, "var pow5InvSplit64 = [...]uint128{")
	for i := int64(0); i < negTableSize64; i++ {
		pow5 := big.NewInt(5)
		pow5.Exp(pow5, big.NewInt(i), nil)
		// We want floor(log_2 5^q) here, which is pow5.BitLen() - 1.
		shift := pow5.BitLen() - 1 + pow5InvNumBits64
		inv := big.NewInt(1)
		rsh(inv, -shift)
		inv.Quo(inv, pow5)
		inv.Add(inv, big.NewInt(1))
		lo := new(big.Int).And(inv, mask64)
		hi := new(big.Int).Rsh(inv, 64)
		fmt.Fprintf(b, "{%d, %d},\n", lo.Uint64(), hi.Uint64())
	}
	fmt.Fprintln(b, "\n}")

	text, err := format.Source(b.Bytes())
	if err != nil {
		log.Fatal(err)
	}
	if err := ioutil.WriteFile("tables.go", text, 0644); err != nil {
		log.Fatal(err)
	}
}

func rsh(x *big.Int, n int) {
	if n < 0 {
		x.Lsh(x, uint(-n))
	} else {
		x.Rsh(x, uint(n))
	}
}
