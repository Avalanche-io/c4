package c4_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/Avalanche-io/c4"
	"github.com/xtgo/set"
)

func TestEncoding(t *testing.T) {

	for _, test := range []struct {
		In  io.Reader
		Exp string
	}{
		{
			In:  strings.NewReader(``),
			Exp: "c459dsjfscH38cYeXXYogktxf4Cd9ibshE3BHUo6a58hBXmRQdZrAkZzsWcbWtDg5oQstpDuni4Hirj75GEmTc1sFT",
		},
	} {
		actual := c4.Identify(test.In)
		if actual.String() != test.Exp {
			t.Errorf("IDs don't match, got %q expected %q", actual.String(), test.Exp)
		}
	}
}

func TestAllFFFF(t *testing.T) {
	var b []byte
	for i := 0; i < 64; i++ {
		b = append(b, 0xFF)
	}
	var id c4.ID

	copy(id[:], b)
	if id.String() != `c467rpwLCuS5DGA8KGZXKsVQ7dnPb9goRLoKfgGbLfQg9WoLUgNY77E2jT11fem3coV9nAkguBACzrU1iyZM4B8roQ` {
		t.Errorf("IDs don't match, got %q, expcted %q", id.String(), `c467rpwLCuS5DGA8KGZXKsVQ7dnPb9goRLoKfgGbLfQg9WoLUgNY77E2jT11fem3coV9nAkguBACzrU1iyZM4B8roQ`)
	}

	id2, err := c4.Parse(`c467rpwLCuS5DGA8KGZXKsVQ7dnPb9goRLoKfgGbLfQg9WoLUgNY77E2jT11fem3coV9nAkguBACzrU1iyZM4B8roQ`)
	if err != nil {
		t.Errorf("Unexpected error %q", err)
	}

	for _, bb := range id2[:] {
		if bb != 0xFF {
			t.Errorf("incorrect Parse results")
		}
	}
}

func TestAll0000(t *testing.T) {
	var b []byte
	for i := 0; i < 64; i++ {
		b = append(b, 0x00)
	}
	var id c4.ID
	copy(id[:], b)
	if id.String() != `c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111111` {
		t.Errorf("IDs don't match, got %q, expcted %q", id.String(), `c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111111`)
	}

	id2, err := c4.Parse(`c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111111`)
	if err != nil {
		t.Errorf("Unexpected error %q", err)
	}

	for _, bb := range id2[:] {
		if bb != 0x00 {
			t.Errorf("incorrect Parse results")
		}
	}
}

func TestAppendOrder(t *testing.T) {
	byteData := [4][]byte{
		{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 58},
		{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x0d, 0x24},
		{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 0xfa, 0x28},
		{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xac, 0xad, 0x10},
	}
	expectedIDs := [4]string{
		`c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111121`,
		`c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111211`,
		`c41111111111111111111111111111111111111111111111111111111111111111111111111111111111112111`,
		`c41111111111111111111111111111111111111111111111111111111111111111111111111111111111121111`,
	}
	for k := 0; k < 4; k++ {
		var id c4.ID
		copy(id[:], byteData[k])

		// is.Equal(id.String(), expectedIDs[k])
		if id.String() != expectedIDs[k] {
			t.Errorf("IDs don't match, got %q, expcted %q", id.String(), expectedIDs[k])
		}
		id2, err := c4.Parse(expectedIDs[k])
		if err != nil {
			t.Errorf("Unexpected error %q", err)
		}

		// bignum2 := big.Int(id2)
		// b = (&bignum2).Bytes()
		// size := len(id2[:])
		// for size < 64 {
		// 	b = append([]byte{0}, b...)
		// 	size++
		// }
		for i, bb := range id2[:] {
			if bb != byteData[k][i] {
				t.Errorf("incorrect Parse results")
			}

		}
	}
}

func TestParse(t *testing.T) {
	for _, test := range []struct {
		In  string
		Err string
		Exp string
	}{
		{
			In:  `c43ucjRutKqZSCrW43QGU1uwRZTGoVD7A7kPHKQ1z4X1Ge8mhW4Q1gk48Ld8VFpprQBfUC8JNvHYVgq453hCFrgf9D`,
			Err: ``,
			Exp: "This is a pretend asset file, for testing asset id generation.\n",
		},
		{
			In:  `c430cjRutKqZSCrW43QGU1uwRZTGoVD7A7kPHKQ1z4X1Ge8mhW4Q1gk48Ld8VFpprQBfUC8JNvHYVgq453hCFrgf9D`,
			Err: `non c4 id character at position 3`,
			Exp: "",
		},
		{
			In:  `c430cjRutKqZSCrW43QGU1uwRZTGoVD7A7kPHKQ1z4X1Ge8mhW4Q1gk48Ld8VFpprQBfUC8JNvHYVgq453hCFrgf9`,
			Err: `c4 ids must be 90 characters long, input length 89`,
			Exp: "",
		},
	} {
		id, err := c4.Parse(test.In)
		if len(test.Err) != 0 {
			// is.Err(err)
			if err == nil {
				t.Errorf("Expected error but got none")
			}

			if err.Error() != test.Err {
				t.Errorf("incorrect error got %q, expected %q", err.Error(), test.Err)
			}

		} else {
			expectedID := c4.Identify(strings.NewReader(test.Exp))
			if err != nil {
				t.Errorf("Unexpected error %q", err)
			}

			if expectedID.Cmp(id) != 0 {
				t.Errorf("IDs don't match, got %q, expcted %q", id, expectedID)
			}
		}
	}
}

func TestIDLess(t *testing.T) {
	id1 := c4.Identify(strings.NewReader(`1`)) // c42yrSHMvUcscrQBssLhrRE28YpGUv9Gf95uH8KnwTiBv4odDbVqNnCYFs3xpsLrgVZfHebSaQQsvxgDGmw5CX1fVy
	id2 := c4.Identify(strings.NewReader(`2`)) // c42i2hTBA9Ej4nqEo9iUy3pJRRE53KAH9RwwMSWjmfaQN7LxCymVz1zL9hEjqeFYzxtxXz2wRK7CBtt71AFkRfHodu

	if id1.Less(id2) != false {
		t.Errorf("expected %q to be less than %q", id2, id1)
	}
}

func TestIDCmp(t *testing.T) {
	id1 := c4.Identify(strings.NewReader(`1`)) // c42yrSHMvUcscrQBssLhrRE28YpGUv9Gf95uH8KnwTiBv4odDbVqNnCYFs3xpsLrgVZfHebSaQQsvxgDGmw5CX1fVy
	id2 := c4.Identify(strings.NewReader(`2`)) // c42i2hTBA9Ej4nqEo9iUy3pJRRE53KAH9RwwMSWjmfaQN7LxCymVz1zL9hEjqeFYzxtxXz2wRK7CBtt71AFkRfHodu

	// is.Equal(id1.Cmp(id2), 1)
	if id1.Cmp(id2) != 1 {
		t.Errorf("Incorrect comparison between %q, %q", id1, id2)
	}

	if id2.Cmp(id1) != -1 {
		t.Errorf("Incorrect comparison between %q, %q", id2, id1)
	}

	if id1.Cmp(id1) != 0 {
		t.Errorf("Incorrect comparison between %q, %q", id1, id1)
	}

}

func TestCompareIDs(t *testing.T) {
	var id c4.ID
	for _, test := range []struct {
		Id_A c4.ID
		Id_B c4.ID
		Exp  int
	}{
		{

			Id_A: c4.Identify(strings.NewReader("Test string")),
			Id_B: c4.Identify(strings.NewReader("Test string")),
			Exp:  0,
		},
		{
			Id_A: c4.Identify(strings.NewReader("Test string A")),
			Id_B: c4.Identify(strings.NewReader("Test string B")),
			Exp:  -1,
		},
		{
			Id_A: c4.Identify(strings.NewReader("Test string B")),
			Id_B: c4.Identify(strings.NewReader("Test string A")),
			Exp:  1,
		},
		{
			Id_A: c4.Identify(strings.NewReader("Test string")),
			Id_B: id,
			Exp:  -1,
		},
	} {
		if test.Id_A.Cmp(test.Id_B) != test.Exp {
			t.Errorf("Incorrect comparison between %q, %q", test.Id_A, test.Id_B)
		}
	}

}

func TestBytesToID(t *testing.T) {
	var id c4.ID
	copy(id[:], []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 58})
	if id.String() != "c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111121" {
		t.Errorf("IDs don't match, got %q, expcted %q", id.String(), "c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111121")
	}
}

func TestNILID(t *testing.T) {

	// ID of nothing constant
	nilid := c4.Identify(strings.NewReader(""))

	if nilid.String() != "c459dsjfscH38cYeXXYogktxf4Cd9ibshE3BHUo6a58hBXmRQdZrAkZzsWcbWtDg5oQstpDuni4Hirj75GEmTc1sFT" {
		t.Errorf("IDs don't match, got %q, expcted %q", nilid.String(), "c459dsjfscH38cYeXXYogktxf4Cd9ibshE3BHUo6a58hBXmRQdZrAkZzsWcbWtDg5oQstpDuni4Hirj75GEmTc1sFT")
	}
}

var test_vectors = []string{"alfa", "bravo", "charlie", "delta", "echo", "foxtrot", "golf", "hotel", "india"}
var test_vector_ids = [][]string{
	// Initial list (unsorted).
	[]string{
		"c43zYcLni5LF9rR4Lg4B8h3Jp8SBwjcnyyeh4bc6gTPHndKuKdjUWx1kJPYhZxYt3zV6tQXpDs2shPsPYjgG81wZM1",
		"c42jd8KUQG9DKppN1qt5aWS3PAmdPmNutXyVTb8H123FcuU3shPxpUXsVdcouSALZ4PaDvMYzQSMYCWkb6rop9zhDa",
		"c44erLietE8C1iKmQ3y4ENqA9g82Exdkoxox3KEHops2ux5MTsuMjfbFRvUPsPdi9Pxc3C2MRvLxWT8eFw5XKbRQGw",
		"c42Sv2Wi2Qo8AKbJKnUP6YTSdz8pt9aDaf2Ltx44HF1UDdXANM8Ltk6qEzpncvmVbw6FZxgBumw9Eo2jtGyaQ5gDSC",
		"c41bviGCyTM2stoMYVTVKgBkfC6SitoLRFinp77BcmN9awdaeC9cxPy4zyFQBhmTvRzChawbECK1KBRnw3KnagA5be",
		"c427CsZdfUAHyQBS3hxDFrL9NqgKeRuKkuSkxuYTm26XG7AKAWCjViDuMhHaMmQBkvuHnsxojetbQU1DdxHjzyQw8r",
		"c41yLiwAPdsjiBAAw8AFwQGG3cAWnNbDio21NtHE8yD1Fh5irRE4FsccZvm1WdJ4FNHtR1kt5kev7wERsgYomaQbfs",
		"c44nNyaFuVbt5MCfo2PYWHpwMkBpYTbt14C6TuoLCYH5RLvAFLngER3nqHfXC2GuttcoDxGBi3pY1j3pUF2W3rZD8N",
		"c41nJ6CvPN7m7UkUA3oS2yjXYNSZ7WayxEQXWPae6wFkWwW8WChQWTu61bSeuCERu78BDK1LUEny1qHZnye3oU7DtY",
	},
	// After round 1
	[]string{
		"c42zjM4ARWVNHVkHsaiEWMAxzngUk8op167Dsm1iNpGfxdQBmhwjhWshKRqacPQw3MKwj7kAVxqBwSxADRDKQFAbtu",
		"c45y4hGsfLRcoDpccf7vh8oaEvuFV5UePJmoXWg2W8fr2EqPHLxucBJMmPSXN1wv45okRdjEXkbZn1KzapPwUhYhgz",
		"c41DGFq9sEb7jVmfsvPWnB8R8nENZp1xfoMbS5kK8TkCDpCT28A3wXsAbj8L5ojNLJrENh4UPmrqBCqJvRtG3oeavt",
		"c453g2FnSZnHyUsM95Hs63wVTLmaJLgcB6HULNY7G6xeKggUPsdtN39e9C2qzkoMWKB9gWHVX6aigy1uSzAvyVoS7R",
		"c44nNyaFuVbt5MCfo2PYWHpwMkBpYTbt14C6TuoLCYH5RLvAFLngER3nqHfXC2GuttcoDxGBi3pY1j3pUF2W3rZD8N",
	},
	// After round 2
	[]string{
		"c42WxVx7sogq4LSuxxbzzytXztB3GMwiqfsEPyghJnR5QYVoJ7rVu2yDTpzKTS63eEn2bH4ouhkb1CUTqNfu8RepgB",
		"c45b6ZA4eu1PoCmeYXncTNGAD47sqJPoN1kMgSBsFgXQB9pwRr6u8a6hDWsBbB5x78ZENb5GsnmGejDcCo7aZ4SAsz",
		"c44nNyaFuVbt5MCfo2PYWHpwMkBpYTbt14C6TuoLCYH5RLvAFLngER3nqHfXC2GuttcoDxGBi3pY1j3pUF2W3rZD8N",
	},
	// After round 3
	[]string{
		"c449rzjCF2bwgbWkHLWRRNNQsjxMu36ee6hU3gMr3PxX8zSPpwWZkYp27zgtgFpuBCajMtfYA6PzSmGpRJYLT6pqa5",
		"c44nNyaFuVbt5MCfo2PYWHpwMkBpYTbt14C6TuoLCYH5RLvAFLngER3nqHfXC2GuttcoDxGBi3pY1j3pUF2W3rZD8N",
	},
	// Final ID
	[]string{
		"c435RzTWWsjWD1Fi7dxS3idJ7vFgPVR96oE95RfDDT5ue7hRSPENePDjPDJdnV46g7emDzWK8LzJUjGESMG5qzuXqq",
	},
}

func TestIdentification(t *testing.T) {

	for i, test := range test_vectors {
		c4id := c4.Identify(bytes.NewReader([]byte(test)))
		if c4id.String() != test_vector_ids[0][i] {
			t.Errorf("IDs don't match, got %q expected %q", c4id.String(), test_vector_ids[0][i])
		}

	}
}

func viewBytes(b []byte) string {
	length := 4
	out := fmt.Sprintf("(%d)[", len(b))
	for j := 0; j < length; j++ {
		num := strconv.Itoa(int(b[j]))
		out += fmt.Sprintf(" %s%s", strings.Repeat(" ", 3-len(num)), num)
	}
	out += fmt.Sprintf(" ... ")
	offset := 64 - length
	if len(b) >= 128 {
		for j := 64 - length; j < 64+length; j++ {
			if j == 64 {
				out += " |"
			}
			num := strconv.Itoa(int(b[j]))
			out += fmt.Sprintf(" %s%s", strings.Repeat(" ", 3-len(num)), num)
		}
		offset = 128 - length
		out += fmt.Sprintf(" ... ")
	}
	for j := offset; j < offset+length; j++ {
		num := strconv.Itoa(int(b[j]))
		out += fmt.Sprintf(" %s%s", strings.Repeat(" ", 3-len(num)), num)
	}
	return out + " ]"
}

func TestDigestSum(t *testing.T) {
	type testDataType struct {
		Value string
		Id    c4.ID
		C4ID  string
	}
	test_data := []testDataType{}

	for i, s := range test_vectors {
		dig := c4.Identify(strings.NewReader(s))
		id, err := c4.Parse(test_vector_ids[0][i])

		if err != nil {
			t.Errorf("unexpected error %q", err)
		}

		if id.String() != dig.String() {
			t.Errorf("IDs don't match, got %q expected %q", id, dig)
		}

		if id.String() != test_vector_ids[0][i] {
			t.Errorf("IDs don't match, got %q expected %q", id.String(), test_vector_ids[0][i])
		}

		test_data = append(test_data, testDataType{s, id, test_vector_ids[0][i]})

	}

	pair := make([]byte, 0, 128)
	var l, r c4.ID
	var key string
	var id c4.ID
	lbytes, rbytes := make([]byte, 64), make([]byte, 64)

	for i, dta := range test_data {

		pair = append(pair, dta.Id[:]...)
		key = dta.Value
		id = dta.Id

		if i > 0 && i%2 == 1 {
			// right hand side
			t.Logf("%d: \"%s\"\n %s  %s\n", i, key, id, viewBytes(dta.Id[:]))
			t.Logf("\tpair: %s\n", viewBytes(pair))

			r = dta.Id
			copy(rbytes, r[:])
			data := make([]byte, 64)
			switch r.Cmp(l) {
			case -1:
				copy(data, r[:])
				data = append(data, l[:]...)
			case 0:
				copy(data, l[:])
			case 1:
				copy(data, l[:])
				data = append(data, r[:]...)
			}
			t.Logf("\t   l: %s\n\t   r: %s\n", viewBytes(l[:]), viewBytes(r[:]))
			t.Logf("\tdata: %s\n", viewBytes(data))

			testsum1 := c4.Identify(bytes.NewReader(data))
			sum := l.Sum(r)

			// Check Sum produces the expected ID

			if testsum1.Cmp(sum) != 0 {
				t.Errorf("Digests don't match, got %q expected %q", testsum1, sum)
			}
			// Check that Sum did not alter l, or r
			if bytes.Compare(r[:], rbytes[:]) != 0 {
				t.Error("Sum altered source r")
			}
			if bytes.Compare(l[:], lbytes) != 0 {
				t.Errorf("Sum altered source l")
			}
			t.Logf("\t   testsum1: %s\n\t   sum: %s\n", viewBytes(testsum1[:]), viewBytes(sum[:]))

			var id1, id2 c4.ID
			copy(id1[:], pair[:64])
			copy(id2[:], pair[64:])

			testsum2 := id1.Sum(id2)

			if testsum2.Cmp(sum) != 0 {
				t.Errorf("IDs don't match, got %q expected %q", testsum2, sum)
			}

			pair = pair[:0]
			continue
		}

		// left hand side
		l = dta.Id

		copy(lbytes, l[:])
		t.Logf("%d: \"%s\"\n %s  %s\n", i, key, id, viewBytes(dta.Id[:]))
	}

}

func TestDigestSlice(t *testing.T) {
	ids := make(c4.IDs, len(test_vectors))
	for i, s := range test_vectors {
		ids[i] = c4.Identify(strings.NewReader(s))
	}

	sort.Sort(ids)
	n := set.Uniq(ids)
	ids = ids[:n]

	t.Run("Order", func(t *testing.T) {
		if len(ids) != len(test_vectors) {
			t.Errorf("lengths do not match got %d, expected %d", len(ids), len(test_vectors))
		}
		sorted_test_vector_ids := make([]string, len(test_vector_ids[0]))
		copy(sorted_test_vector_ids, test_vector_ids[0])
		sort.Strings(sorted_test_vector_ids)
		for i, idstring := range sorted_test_vector_ids {
			if idstring != ids[i].String() {
				t.Errorf("IDs don't match, got %q expected %q", idstring, ids[i].String())
			}
		}

		c4id := ids.Tree().ID()
		if c4id.String() != "c435RzTWWsjWD1Fi7dxS3idJ7vFgPVR96oE95RfDDT5ue7hRSPENePDjPDJdnV46g7emDzWK8LzJUjGESMG5qzuXqq" {
			t.Errorf("IDs don't match, got %q expected %q", c4id.String(), "c435RzTWWsjWD1Fi7dxS3idJ7vFgPVR96oE95RfDDT5ue7hRSPENePDjPDJdnV46g7emDzWK8LzJUjGESMG5qzuXqq")
		}
	})

}

func TestDigest(t *testing.T) {
	var b []byte
	for i := 0; i < 64; i++ {
		b = append(b, 0xFF)
	}
	var id c4.ID
	copy(id[:], b)

	if id.String() != `c467rpwLCuS5DGA8KGZXKsVQ7dnPb9goRLoKfgGbLfQg9WoLUgNY77E2jT11fem3coV9nAkguBACzrU1iyZM4B8roQ` {
		t.Errorf("IDs don't match, got %q expected %q", id.String(), `c467rpwLCuS5DGA8KGZXKsVQ7dnPb9goRLoKfgGbLfQg9WoLUgNY77E2jT11fem3coV9nAkguBACzrU1iyZM4B8roQ`)
	}

	id2, err := c4.Parse(`c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111121`)
	tb2 := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 58}
	if err != nil {
		t.Errorf("unexpected error %q", err)
	}
	b2 := id2.Digest()
	for i, bb := range b2 {
		if bb != tb2[i] {
			t.Errorf("error parsing")
		}
	}

	for _, test := range []struct {
		Bytes []byte
		IdStr string
	}{
		{
			Bytes: []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 58},
			IdStr: `c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111121`,
		},
		{
			Bytes: []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x0d, 0x24},
			IdStr: `c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111211`,
		},
		{
			Bytes: []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 0xfa, 0x28},
			IdStr: `c41111111111111111111111111111111111111111111111111111111111111111111111111111111111112111`,
		},
		{
			Bytes: []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xac, 0xad, 0x10},
			IdStr: `c41111111111111111111111111111111111111111111111111111111111111111111111111111111111121111`,
		},
	} {
		id, err := c4.Parse(test.IdStr)
		if err != nil {
			t.Errorf("unexpected error %q", err)
		}
		for i, bb := range id.Digest() {
			if bb != test.Bytes[i] {
				t.Errorf("error parsing")
			}
		}
	}
}

func TestIdentify(t *testing.T) {

	id := c4.Identify(iotest.DataErrReader(strings.NewReader("foo")))
	if id.String() != "c45xZeXwMSpqXjpDumcHMA6mhoAmGHkUo7r9WmN2UgSEQzj9KjgseaQdkEJ11fGb5S1WEENcV3q8RFWwEeVpC7Fjk2" {
		t.Errorf("C4 IDs don't match, got %q, expected %q", id.String(), "c45xZeXwMSpqXjpDumcHMA6mhoAmGHkUo7r9WmN2UgSEQzj9KjgseaQdkEJ11fGb5S1WEENcV3q8RFWwEeVpC7Fjk2")
	}
}

// returns error on read for testing the negative case
type errorReader bool

func (e errorReader) Read(p []byte) (int, error) {
	if e == true {
		return 0, errors.New("errorReader triggered error.")
	}
	return 0, nil
}

func TestIOFailure(t *testing.T) {
	id := c4.Identify(errorReader(true))
	if !id.IsNil() {
		t.Errorf("expected id to be nil but got: %s", id)
	}
}

func TestMarshalJSON(t *testing.T) {
	var empty c4.ID
	type testType struct {
		Name string `json:"name"`
		ID   c4.ID  `json:"id"`
	}

	nilID := c4.Identify(strings.NewReader(""))
	for _, test := range []struct {
		In  testType
		Exp string
	}{
		{
			In:  testType{"Test", nilID},
			Exp: `{"name":"Test","id":"c459dsjfscH38cYeXXYogktxf4Cd9ibshE3BHUo6a58hBXmRQdZrAkZzsWcbWtDg5oQstpDuni4Hirj75GEmTc1sFT"}`,
		},
		{
			In:  testType{"Test", empty},
			Exp: `{"name":"Test","id":""}`,
		},
		{
			In:  testType{"Test", empty},
			Exp: `{"name":"Test","id":""}`,
		},
	} {
		actual, err := json.Marshal(test.In)
		if err != nil {
			t.Errorf("unexpected error %q", err)
		}
		if string(actual) != test.Exp {
			t.Errorf("results do not match got %q, expected %q", string(actual), test.Exp)
		}
	}
}

func TestUnarshalJSON(t *testing.T) {

	type testType struct {
		Name string `json:"name"`
		Id   c4.ID  `json:"id"`
	}
	var unset c4.ID
	nilID := c4.Identify(strings.NewReader(""))

	for i, test := range []struct {
		In  []byte
		Exp testType
	}{
		{
			In:  []byte(`{"name":"Test","id":"c459dsjfscH38cYeXXYogktxf4Cd9ibshE3BHUo6a58hBXmRQdZrAkZzsWcbWtDg5oQstpDuni4Hirj75GEmTc1sFT"}`),
			Exp: testType{"Test", nilID},
		},
		{
			In:  []byte(`{"name":"Test","id":""}`),
			Exp: testType{"Test", unset},
		},
	} {

		var testObject testType
		err := json.Unmarshal([]byte(test.In), &testObject)
		if err != nil {
			t.Errorf("unexpected error %q", err)
		}
		t.Logf("> %d: %v", i, testObject)
		if testObject.Id.IsNil() {
			if !test.Exp.Id.IsNil() {
				t.Errorf("%d results do not match got %v, expected %v", i, testObject, test.Exp)
			}
		} else if testObject.Name != test.Exp.Name || testObject.Id.String() != test.Exp.Id.String() {
			t.Errorf("results do not match got %v, expected %v", testObject, test.Exp)
		}
	}
}
