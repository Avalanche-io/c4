package asset_test

import (
	// "bytes"

	"bytes"
	"testing"

	"github.com/cheekybits/is"
	"github.com/etcenter/c4/asset"
)

var test_vectors = []string{"alfa", "bravo", "charlie", "delta", "echo", "foxtrot", "golf", "hotel", "india"}
var test_vector_ids = []string{
	"c43zYcLni5LF9rR4Lg4B8h3Jp8SBwjcnyyeh4bc6gTPHndKuKdjUWx1kJPYhZxYt3zV6tQXpDs2shPsPYjgG81wZM1",
	"c42jd8KUQG9DKppN1qt5aWS3PAmdPmNutXyVTb8H123FcuU3shPxpUXsVdcouSALZ4PaDvMYzQSMYCWkb6rop9zhDa",
	"c44erLietE8C1iKmQ3y4ENqA9g82Exdkoxox3KEHops2ux5MTsuMjfbFRvUPsPdi9Pxc3C2MRvLxWT8eFw5XKbRQGw",
	"c42Sv2Wi2Qo8AKbJKnUP6YTSdz8pt9aDaf2Ltx44HF1UDdXANM8Ltk6qEzpncvmVbw6FZxgBumw9Eo2jtGyaQ5gDSC",
	"c41bviGCyTM2stoMYVTVKgBkfC6SitoLRFinp77BcmN9awdaeC9cxPy4zyFQBhmTvRzChawbECK1KBRnw3KnagA5be",
	"c427CsZdfUAHyQBS3hxDFrL9NqgKeRuKkuSkxuYTm26XG7AKAWCjViDuMhHaMmQBkvuHnsxojetbQU1DdxHjzyQw8r",
	"c41yLiwAPdsjiBAAw8AFwQGG3cAWnNbDio21NtHE8yD1Fh5irRE4FsccZvm1WdJ4FNHtR1kt5kev7wERsgYomaQbfs",
	"c44nNyaFuVbt5MCfo2PYWHpwMkBpYTbt14C6TuoLCYH5RLvAFLngER3nqHfXC2GuttcoDxGBi3pY1j3pUF2W3rZD8N",
	"c41nJ6CvPN7m7UkUA3oS2yjXYNSZ7WayxEQXWPae6wFkWwW8WChQWTu61bSeuCERu78BDK1LUEny1qHZnye3oU7DtY",
}

func TestIdentification(t *testing.T) {
	is := is.New(t)
	for i, t := range test_vectors {
		id, err := asset.Identify(bytes.NewReader([]byte(t)))
		is.NoErr(err)
		is.Equal(id.String(), test_vector_ids[i])
	}
}

func TestSliceID(t *testing.T) {
	is := is.New(t)
	var ids asset.IDSlice
	for _, t := range test_vectors {
		id, err := asset.Identify(bytes.NewReader([]byte(t)))
		is.NoErr(err)
		ids.Push(id)
	}
	id, err := ids.ID()
	is.NoErr(err)
	is.Equal(id.String(), "c435RzTWWsjWD1Fi7dxS3idJ7vFgPVR96oE95RfDDT5ue7hRSPENePDjPDJdnV46g7emDzWK8LzJUjGESMG5qzuXqq")
}

func TestPreSortedSliceID(t *testing.T) {
	is := is.New(t)
	var ids_sorted, ids_unsorted asset.IDSlice
	for i := 0; i < 64; i++ {
		id, err := asset.Identify(bytes.NewReader([]byte{byte(i)}))
		is.NoErr(err)
		ids_sorted.Push(id)
		ids_unsorted.Push(id)
	}
	ids_sorted.Sort()

	// start := time.Now()
	id_unsorted, err := ids_unsorted.ID()
	// fmt.Printf("ids_unsorted speed: %s\n", time.Now().Sub(start))
	is.NoErr(err)
	// start = time.Now()
	id_sorted, err := ids_sorted.PreSortedID()
	// fmt.Printf("ids_PreSorted speed: %s\n", time.Now().Sub(start))
	is.NoErr(err)
	is.Equal(id_unsorted.String(), id_sorted.String())
}

func TestCombinableSliceIDs(t *testing.T) {
	is := is.New(t)
	var all_ids, idsA, idsB asset.IDSlice
	test_count := 64 // must be a power of 2
	for i := 0; i < test_count; i++ {
		id, err := asset.Identify(bytes.NewReader([]byte{byte(i)}))
		is.NoErr(err)
		all_ids.Push(id)
	}
	all_ids.Sort()
	idsA = all_ids[0 : test_count/2]
	idsB = all_ids[test_count/2 : test_count]
	is.Equal(test_count/2, idsA.Len())
	is.Equal(test_count/2, idsB.Len())

	id_all, err := all_ids.PreSortedID()
	is.NoErr(err)
	id_A, err := idsA.PreSortedID()
	is.NoErr(err)
	id_B, err := idsB.PreSortedID()
	is.NoErr(err)
	id_AB, err := id_A.Sum(id_B)
	is.NoErr(err)
	is.Equal(id_AB.String(), id_all.String())
}
