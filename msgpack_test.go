
/*
go-msgpack - Msgpack library for Go. Provides pack/unpack and net/rpc support.
https://github.com/ugorji/go-msgpack

Copyright (c) 2012, Ugorji Nwoke.
All rights reserved.

Redistribution and use in source and binary forms, with or without modification,
are permitted provided that the following conditions are met:

* Redistributions of source code must retain the above copyright notice,
  this list of conditions and the following disclaimer.
* Redistributions in binary form must reproduce the above copyright notice,
  this list of conditions and the following disclaimer in the documentation
  and/or other materials provided with the distribution.
* Neither the name of the author nor the names of its contributors may be used
  to endorse or promote products derived from this software
  without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE FOR
ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES
(INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES;
LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON
ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS
SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
*/

package msgpack

// Test works by using a slice of interfaces.
// It can test for encoding/decoding into/from a nil interface{}
// or passing the object to encode/decode into.
//
// If nil interface{}, then we have a different set of values to 
// verify against, understanding that decoding into a nil interface
// for example only uses 64-bit primitives (int64, uint64, float64),
// and []byte for primitive values, etc.
// 
// There are basically 2 tests here.
// First test internally encodes and decodes things and verifies that
// the artifact was as expected.
// Second test will use python msgpack to create a bunch of golden files,
// read those files, and compare them to what it should be. It then 
// writes those files back out and compares the byte streams.
//
// Altogether, the tests are pretty extensive.



import (
	"reflect"
	"testing"
	"fmt"
	"net/rpc"
	"bytes"
	"time"
	"os"
	"os/exec"
	"io/ioutil"
	"path/filepath"
	"strconv"
)

var (
	testLogToT = true
	failNowOnFail = false
	testShowLog = true
	skipVerifyVal interface{} = &(struct{}{})
	timeToCompare = time.Date(2012, 2, 2, 2, 2, 2, 2000, time.UTC) //time.Time{} //
	//"2012-02-02T02:02:02.000002000Z" //1328148122000002
	timeToCompareAs interface{} = timeToCompare.UnixNano() 
	table []interface{}               // main items we encode
	tableVerify []interface{}         // we verify encoded things against this after decode
	tableTestNilVerify []interface{}  // for nil interface, use this to verify (rules are different)
	tablePythonVerify []interface{}   // for verifying for python, since Python sometimes
                                          // will encode a float32 as float64, or large int as uint
)

type TestStruc struct {
	S string
	I64 int64
	I16 int16
	Ui64 uint64
	Ui8 uint8
	B bool
	By byte
	
	Sslice []string
	I64slice []int64
	I16slice []int16
	Ui64slice []uint64
	Ui8slice []uint8
	Bslice []bool
	Byslice []byte
	
	Islice []interface{}
	
	//M map[interface{}]interface{}  `json:"-",bson:"-"`
	Ms map[string]interface{}
	Msi64 map[string]int64
	
	Nintf interface{}    //don't set this, so we can test for nil
	T time.Time          
	Nmap map[string]bool //don't set this, so we can test for nil
	Nslice []byte        //don't set this, so we can test for nil
	Nint64 *int64
	Nteststruc *TestStruc
}

func init() {
	_, _ = fmt.Printf, os.Remove
	primitives := []interface{}{
		int8(-8),
		int16(-1616),
		int32(-32323232),
		int64(-6464646464646464),
		uint8(8),
		uint16(1616),
		uint32(32323232),
		uint64(6464646464646464),
		byte(8),
		float32(-3232.0),
		float64(-6464646464.0),
		float32(3232.0),
		float64(6464646464.0),
		false,
		true,
		nil,
		timeToCompare,
		"someday",
		"",
		"bytestring",
	}
	mapsAndStrucs := []interface{}{
		map[string]bool{
			"true":true,
			"false":false,
		},
		map[string]interface{}{
			"true": "True",
			"false": false,
			"int64(0)": int8(0),
		},
		//add a complex combo map in here. (map has list which has map)
		//note that after the first thing, everything else should be generic.
		map[string]interface{}{
			"list": []interface{}{
				int16(1616),
				int32(32323232),
				true,
				float32(-3232.0),
				map[string]interface{} {
					"TRUE":true,
					"FALSE":false,
				},
				[]interface{}{true, false},
			},
			"int32": int32(32323232),
			"bool": true,
			"LONG STRING": "123456789012345678901234567890123456789012345678901234567890",
			"SHORT STRING": "1234567890",
		},
		map[interface{}]interface{}{
			true: "true",
			int8(8): false,
			"false": int8(0),
		},
		newTestStruc(0),
	}
	
	table = []interface{}{}
	table = append(table, primitives...)    //0-19 are primitives
	table = append(table, primitives)       //20 is a list of primitives
	table = append(table, mapsAndStrucs...) //21-24 are maps. 25 is a *struct

	// we verify against the same table, but skip 23 
	// because interface{} equality is not defined exact for exact objects or nil.
	var a, b []interface{}
	var c map[string]interface{}
	a = make([]interface{}, len(table))
	copy(a, table)
	b = make([]interface{}, len(a[20].([]interface{})))
	copy(b, a[20].([]interface{}))
	a[20] = b
	b[0], b[4], b[8], b[16], b[19] = int8(-8), int8(8), int8(8), int64(1328148122000002), "bytestring"
	a[23] = skipVerifyVal 
	//a[25] = skipVerifyVal
	tableVerify = a
	
	//when decoding into nil, for testing, 
	//we treat each []byte as string, and uint < 127 are decoded as int8.
	a = make([]interface{}, len(tableVerify))
	copy(a, tableVerify)
	a[0], a[4], a[8], a[16], a[19] = int8(-8), int8(8), int8(8), int64(1328148122000002), "bytestring"
	a[21] = map[string]interface{}{"true":true, "false":false}
	a[23] = table[23]
	a[25] = skipVerifyVal
	tableTestNilVerify = a
	
	//python msgpack encodes large positive numbers as unsigned, and all floats as float64
	a = make([]interface{}, len(tableTestNilVerify)-2)
	copy(a, tableTestNilVerify)
	a[23] = table[23]
	a[9], a[11], a[16] = float64(-3232.0), float64(3232.0), uint64(1328148122000002)
	b = make([]interface{}, len(a[20].([]interface{})))
	copy(b, a[20].([]interface{}))
	a[20] = b
	b[9], b[11], b[16] = float64(-3232.0), float64(3232.0), uint64(1328148122000002)
	c = make(map[string]interface{})
	for k, v := range a[23].(map[string]interface{}) { 
		c[k] = v
	}
	a[23] = c
	c["int32"] = uint32(32323232)
	b = c["list"].([]interface{})
	b[0], b[1], b[3] = uint16(1616), uint32(32323232), float64(-3232.0)
	tablePythonVerify = a
}

func lf(x interface{}, format string, args ...interface{}) {
	if t, ok := x.(*testing.T); ok && t != nil && testLogToT {
		t.Logf(format, args...)	
	} else if b, ok := x.(*testing.B); ok && b != nil && testLogToT {
		b.Logf(format, args...)
	} else if testShowLog {
		if len(format) == 0 || format[len(format)-1] != '\n' {
			format = format + "\n"
		}
		fmt.Printf(format, args...)
	}
}

func failT(t *testing.T) {
	if failNowOnFail {
		t.FailNow()
	} else {
		t.Fail()
	}
}

func newTestStruc(depth int) (ts TestStruc) {
	ts = TestStruc {
		S: "some string",
		I64: 64,
		I16: 16,
		Ui64: 64,
		Ui8: 160,
		B: true,
		By: 5,
		
		Sslice: []string{"one", "two", "three"},
		I64slice: []int64{1, 2, 3},
		I16slice: []int16{4, 5, 6},
		Ui64slice: []uint64{7, 8, 9},
		Ui8slice: []uint8{10, 11, 12},
		Bslice: []bool{true, false, true, false},
		Byslice: []byte{13, 14, 15},
		
		Islice: []interface{}{"true", true, "no", false, int8(88), float64(0.4)},
		
		//remove this one, as json and bson require string keys in maps.
		//M: map[interface{}]interface{}{
		//	true: "true",
		//	int64(9): false,
		//},
		Ms: map[string]interface{}{
			"true": "true",
			"int64(9)": false,
		},
		Msi64: map[string]int64{
			"one": 1,
			"two": 2,
		},
		T: timeToCompare,
	}
	if depth > 0 {
		depth--
		ts.Ms["TestStruc." + strconv.Itoa(depth)] = newTestStruc(depth)
		ts.Islice = append(ts.Islice, newTestStruc(depth))
	}
	return
}

// doTestMsgpacks allows us test for different variations based on arguments passed.
func doTestMsgpacks(t *testing.T, testNil bool, opts *DecoderOptions,	
	vs []interface{}, vsVerify []interface{}) {
	//if testNil, then just test for when a pointer to a nil interface{} is passed. It should work.
	//Current setup allows us test (at least manually) the nil interface or typed interface.
	lf(t, "$*$*$*$*$*$*$*$*$*$*$*$*$*$*$*$*$*$*$*$*$*$*$*$*$*$*$\n")
	lf(t, "================ TestNil: %v ================\n", testNil)
	for i, v0 := range vs {
		lf(t, "..............................................")
		lf(t, "         Testing: #%d: %T, %#v\n", i, v0, v0)
		b0, err := Marshal(v0, nil)
		if err != nil {
			lf(t, err.Error())
			failT(t)
			continue
		}
		lf(t, "         Encoded bytes: len: %v, %v\n", len(b0), b0)
		
		var v1 interface{}
		
		dec := NewDecoder(bytes.NewBuffer(b0), opts)
		if testNil {
			err = dec.Decode(&v1)
		} else {
			v0rt := intfTyp
			if v0 != nil { 
				v0rt = reflect.TypeOf(v0) 
			}
			v1 = reflect.New(v0rt).Interface()
			err = dec.Decode(v1)
		}
		
		if v1 != nil {
			lf(t, "         v1 returned: %T, %#v", v1, v1)
			//we always indirect, because ptr to typed value may be passed (if not testNil)
			v1 = reflect.Indirect(reflect.ValueOf(v1)).Interface()
		}
		//v1 = indirIntf(v1, nil, -1)
		if err != nil {
			lf(t, "-------- Error: %v. Partial return: %v", err, v1)
			failT(t)
			continue
		}
		v0check := vsVerify[i]
		if v0check == skipVerifyVal { 
			lf(t, "        Nil Check skipped: Decoded: %T, %#v\n", v1, v1)
			continue 
		}
		
		if reflect.DeepEqual(v0check, v1) { 
			lf(t, "++++++++ Before and After marshal matched\n")
		} else {
			lf(t, "-------- Before and After marshal do not match: " + 
				"(%T). ====> AGAINST: %T, %#v, DECODED: %T, %#v\n", v0, v0check, v0check, v1, v1)
			failT(t)
		}
	}
}

func TestMsgpacks(t *testing.T) {	
	doTestMsgpacks(t, false, &DecoderOptions{nil, nil, false, true, true, USEC}, table, tableVerify) 
	doTestMsgpacks(t, true,  &DecoderOptions{mapStringIntfTyp, nil, true, true, true, USEC}, 
		table[:24], tableTestNilVerify[:24]) 
	doTestMsgpacks(t, true, &DecoderOptions{nil, nil, false, true, true, USEC}, 
		table[24:], tableTestNilVerify[24:]) 
}

func TestDecodeToTypedNil(t *testing.T) {
	b, err := Marshal(32, nil)
	var i *int32
	if err = Unmarshal(b, i, nil); err == nil {
		lf(t, "------- Expecting error because we cannot unmarshal to int32 nil ptr")
		t.FailNow()
	}
	var i2 int32 = 0
	if err = Unmarshal(b, &i2, nil); err != nil {
		lf(t, "------- Cannot unmarshal to int32 ptr. Error: %v", err)
		t.FailNow()
	}
	if i2 != int32(32) {
		lf(t, "------- didn't unmarshal to 32: Received: %d", *i)
		t.FailNow()
	}
}

func TestDecodePtr(t *testing.T) {
	ts := newTestStruc(0)
	b, err := Marshal(&ts, nil)
	if err != nil {
		lf(t, "------- Cannot Marshal pointer to struct. Error: %v", err)
		t.FailNow()
	} else if len(b) < 40 {
		lf(t, "------- Size must be > 40. Size: %d", len(b))
		t.FailNow()
	}
	ts2 := new(TestStruc)
	err = Unmarshal(b, &ts2, nil)
	if err != nil {
		lf(t, "------- Cannot Unmarshal pointer to struct. Error: %v", err)
		t.FailNow()
	} else if ts2.I64 != 64 {
		lf(t, "------- Unmarshal wrong. Expect I64 = 64. Got: %v", ts2.I64)
		t.FailNow()
	}
}

// Test that we honor the rpc.ClientCodec and rpc.ServerCodec
func TestRpcInterface(t *testing.T) {
	c := new(rpcCodec)
	_ = func() { c.Close() }
	//f, _ := os.Open("some-random-file-jkjkjfkldlsfalkdljflsjljad")
	//c = NewRPCCodec(f)
	//c.Close()
	var _ rpc.ClientCodec = c
	var _ rpc.ServerCodec = c
}

// Comprehensive testing that generates data encoded from python msgpack, 
// and validates that our code can read and write it out accordingly.
func TestPythonGenStreams(t *testing.T) {
	lf(t, "TestPythonGenStreams")
	tmpdir, err := ioutil.TempDir("", "golang-msgpack-test") 
	if err != nil {
		lf(t, "-------- Unable to create temp directory\n")
		t.FailNow()
	}
	defer os.RemoveAll(tmpdir)
	lf(t, "tmpdir: %v", tmpdir)
	cmd := exec.Command("python", "helper.py", "testdata", tmpdir)
	//cmd.Stdin = strings.NewReader("some input")
	//cmd.Stdout = &out
	var cmdout []byte
	if cmdout, err = cmd.CombinedOutput(); err != nil {
		lf(t, "-------- Error running python build.py. Err: %v", err)
		lf(t, "         %v", string(cmdout))
		t.FailNow()
	}
	
	for i, v := range tablePythonVerify {
		//load up the golden file based on number
		//decode it
		//compare to in-mem object
		//encode it again
		//compare to output stream
		lf(t, "..............................................")
		lf(t, "         Testing: #%d: %T, %#v\n", i, v, v) 
		var bss []byte
		bss, err = ioutil.ReadFile(filepath.Join(tmpdir, strconv.Itoa(i) + ".golden"))
		if err != nil {
			lf(t, "-------- Error reading golden file: %d. Err: %v", i, err)
			failT(t)
			continue
		}
		dec := NewDecoder(bytes.NewBuffer(bss),
			&DecoderOptions{mapStringIntfTyp, nil, true, true, true, USEC})
		var v1 interface{}
		if err = dec.Decode(&v1); err != nil {
			lf(t, "-------- Error decoding stream: %d: Err: %v", i, err)
			failT(t)
			continue
		}
		if v == skipVerifyVal {
			continue
		}
		//no need to indirect, because we pass a nil ptr, so we already have the value 
		//if v1 != nil { v1 = reflect.Indirect(reflect.ValueOf(v1)).Interface() }
		if reflect.DeepEqual(v, v1) { 
			lf(t, "++++++++ Objects match")
		} else {
			lf(t, "-------- Objects do not match: Source: %T. Decoded: %T", v, v1)
			lf(t, "--------   AGAINST: %#v", v)
			lf(t, "--------   DECODED: %#v", v1)
			failT(t)
		}
		bsb := new(bytes.Buffer)
		if err = NewEncoder(bsb, nil).Encode(v1); err != nil {
			lf(t, "Error encoding to stream: %d: Err: %v", i, err)
			failT(t)
			continue
		}
		if reflect.DeepEqual(bsb.Bytes(), bss) { 
			lf(t, "++++++++ Bytes match")
		} else {
			lf(t, "???????? Bytes do not match")
			xs := "--------"
			if reflect.ValueOf(v).Kind() == reflect.Map {
				xs = "        "
				lf(t, "%s It's a map. Ok that they don't match (dependent on ordering).", xs)
			} else {
				lf(t, "%s It's not a map. They should match.", xs)
				failT(t)
			}
			lf(t, "%s   FROM_FILE: %4d] %v", xs, len(bss), bss)
			lf(t, "%s     ENCODED: %4d] %v", xs, len(bsb.Bytes()), bsb.Bytes())
		}
	}
	
}
