package id_test

import (
	// "bytes"

	// "strings"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"testing"

	c4 "github.com/avalanche-io/c4/id"
	"github.com/cheekybits/is"

	"bytes"
)

var test_vectors = []string{"alfa", "bravo", "charlie", "delta", "echo", "foxtrot", "golf", "hotel", "india"}
var test_vector_ids = [][]string{
	// Initial list, unsorted.
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
	is := is.New(t)
	for i, t := range test_vectors {
		c4id := c4.Identify(bytes.NewReader([]byte(t)))
		is.Equal(c4id.String(), test_vector_ids[0][i])
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
	is := is.New(t)
	type testDataType struct {
		Value  string
		Id     *c4.ID
		C4ID   string
		Digest c4.Digest
	}
	test_data := []testDataType{}
	e := c4.NewEncoder()
	for i, s := range test_vectors {
		e.Write([]byte(s))
		dig := e.Digest()
		id, err := c4.Parse(test_vector_ids[0][i])
		is.NoErr(err)
		is.Equal(id, dig.ID())
		is.Equal(id.String(), test_vector_ids[0][i])
		test_data = append(test_data, testDataType{s, id, test_vector_ids[0][i], dig})

		e.Reset()
	}

	pair := make([]byte, 0, 128)
	var l, r c4.Digest
	var key string
	var id *c4.ID
	for i, dta := range test_data {

		pair = append(pair, []byte(dta.Digest)...)
		key = dta.Value
		id = dta.Digest.ID()

		if i > 0 && i%2 == 1 {
			// right hand side
			t.Logf("%d: \"%s\"\n %s  %s\n", i, key, id, viewBytes(dta.Digest))
			t.Logf("\tpair: %s\n", viewBytes(pair))

			r = dta.Digest
			// data := make([]byte, 128)
			var data []byte
			switch bytes.Compare(r, l) {
			case -1:
				data = r
				data = append(data, l...)
			case 0:
				data = l
			case 1:
				data = l
				data = append(data, r...)
			}
			t.Logf("\t   l: %s\n\t   r: %s\n", viewBytes(l), viewBytes(r))
			t.Logf("\tdata: %s\n", viewBytes(data))

			e.Write(data)
			testsum1 := e.Digest()
			sum := l.Sum(r)
			e.Reset()
			is.Equal(testsum1, sum)
			t.Logf("\t   testsum1: %s\n\t   sum: %s\n", viewBytes(testsum1), viewBytes(sum))

			// is.Equal(testsum1, sum)
			testsum2 := c4.Digest(pair[:64]).Sum(pair[64:])
			is.Equal(testsum2, sum)
			pair = pair[:0]
			continue
		}
		// left hand side
		l = dta.Digest
		t.Logf("%d: \"%s\"\n %s  %s\n", i, key, id, viewBytes(dta.Digest))
	}

	// is.Equal(expectedSum, testSum.ID())
}

func TestDigestSlice(t *testing.T) {
	is := is.New(t)
	var digests c4.DigestSlice
	e := c4.NewEncoder()
	for _, s := range test_vectors {
		e.Write([]byte(s))
		digests.Insert(e.Digest())
		e.Reset()
	}
	is.Equal(len(digests), len(test_vectors))
	sorted_test_vector_ids := make([]string, len(test_vector_ids[0]))
	copy(sorted_test_vector_ids, test_vector_ids[0])
	sort.Strings(sorted_test_vector_ids)
	for i, id := range digests {
		is.Equal(id.ID().String(), sorted_test_vector_ids[i])
	}
	c4id := digests.Digest().ID()
	is.Equal(c4id.String(), "c435RzTWWsjWD1Fi7dxS3idJ7vFgPVR96oE95RfDDT5ue7hRSPENePDjPDJdnV46g7emDzWK8LzJUjGESMG5qzuXqq")
}

func TestDigest(t *testing.T) {
	is := is.New(t)
	var b []byte
	for i := 0; i < 64; i++ {
		b = append(b, 0xFF)
	}
	id := c4.Digest(b).ID()
	is.Equal(id.String(), `c467rpwLCuS5DGA8KGZXKsVQ7dnPb9goRLoKfgGbLfQg9WoLUgNY77E2jT11fem3coV9nAkguBACzrU1iyZM4B8roQ`)

	id2, err := c4.Parse(`c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111121`)
	tb2 := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 58}
	is.NoErr(err)
	b2 := id2.Digest()
	for i, bb := range b2 {
		is.Equal(bb, tb2[i])
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
		is.NoErr(err)
		for i, bb := range id.Digest() {
			is.Equal(bb, test.Bytes[i])
		}
	}
}
