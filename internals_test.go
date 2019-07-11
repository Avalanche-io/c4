package c4

import (
	"bytes"
	"sort"
	"strings"
	"testing"

	"github.com/xtgo/set"
)

// func TestTreeSizes(t *testing.T) {
// 	for N := 3; N < 1<<22; N += 1 {
// 		total := treeSize(N)
// 		if listSize(total) != N {
// 			t.Fatalf("N: %d, total: %d, NN: %d\n", N, total, listSize(total))
// 		}
// 	}
// }

func testIDs() IDs {
	digests := make(IDs, len(test_vectors))
	for i, s := range test_vectors {
		digests[i] = Identify(strings.NewReader(s))
	}
	sort.Sort(digests)
	n := set.Uniq(digests)
	return digests[:n]
}

func row(rows [][]byte, i int) []ID {
	list := make([]ID, len(rows[i])/64)
	for j := range list {
		copy(list[j][:], rows[i][j*64:])
	}
	return list
}

func TestTree(t *testing.T) {
	digests := testIDs()
	t.Logf("len(digests): %d", digests.Len())
	tree := NewTree(digests)
	if tree == nil {
		t.Fatalf("NewTree failed")
	}
	t.Logf("tree id: %s\n", tree.ID())
	rows := buildRows(tree)
	for i, k := 0, len(test_vector_ids)-1; i < len(rows); i, k = i+1, k-1 {

		ids := row(rows, i)
		expected_digests := make(IDs, len(test_vector_ids[k]))
		for j := range test_vector_ids[k] {
			id, err := Parse(test_vector_ids[k][j])
			if err != nil {
				t.Fatal(err)
			}
			expected_digests[j] = id
		}
		if k == 0 {
			sort.Sort(expected_digests)
		}
		for j, d := range ids {
			if d.String() != expected_digests[j].String() {
				t.Fatalf("tree ids do not match for row: j:%d, k:%d %q != %q\n", j, k, d, expected_digests[j])
			}
		}
	}

	for z := 3; z < 30; z++ {
		count := z
		digests = make(IDs, count)
		for i := range digests {
			id := Identify(bytes.NewReader([]byte{uint8(i)}))
			digests[i] = id
		}
		tree = NewTree(digests)
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
