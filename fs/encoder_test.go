package fs_test

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/cheekybits/is"

	"github.com/etcenter/c4/fs"
	"github.com/etcenter/c4/test"
)

func TestEncode(t *testing.T) {
	is := is.New(t)
	tmp := test.TempDir(is)
	defer test.DeleteDir(tmp)
	build_test_fs(is, tmp, 2, 2, 0)

	f := fs.New(tmp)
	// f.IdWorkers(8)
	is.NotNil(f)
	// tmr := time.Now()

	ch := f.Walk()
	is.OK(ch)
	// pipe := fs.JsonEncoder(ch)
	// for d := range pipe {
	// 	fmt.Fprintf(os.Stdout, "%s", string(d))
	// }
	// f.Wait()

	data, err := json.Marshal(f.Nodes)

	if err != nil {
		panic(err)
	}
	_ = data
	// d := time.Now().Sub(start)
	// fmt.Println(string(data))
	m := f.Nodes

	// var deeper func(int, map[string]interface{})
	// deeper = func(depth int, m map[string]interface{}) {
	var deeper func(int, *fs.Item)
	deeper = func(depth int, i *fs.Item) {
		var list []string
		_ = list
		for ele := range i.Iterator(nil) {
			k := ele.Key
			var s string
			switch v := ele.Value.(type) {
			case nil:
				s = fmt.Sprintf("%[1]*s:\n", depth+len(k), k)
			case string:
				s = fmt.Sprintf("%[1]*s:\"%s\"\n", depth+len(k), k, v)
			case bool:
				s = fmt.Sprintf("%[1]*s:%t\n", depth+len(k), k, v)
			case int32, int64:
				s = fmt.Sprintf("%[1]*s:%d\n", depth+len(k), k, v)
			case os.FileMode:
				s = fmt.Sprintf("%[1]*s:\"%s\"\n", depth+len(k), k, v)
			case time.Time:
				s += fmt.Sprintf("%[1]*s:\"%v\"\n", depth+len(k), k, v.Format(time.RFC3339))
			// case map[string]interface{}:
			// 	fmt.Printf("%[1]*s:\n", depth+len(k), k) //depth*2, k)
			// 	deeper(depth+2, v)
			case *fs.Item:
				fmt.Printf("%[1]*s:\n", depth+len(k), k) //depth*2, k)
				deeper(depth+2, v)
			default:
				s = fmt.Sprintf("unknown\n")
			}
			list = append(list, s)
			// fmt.Print(s)
		}
		sort.Strings(list)
		for _, l := range list {
			_ = l
			fmt.Print(l)
		}
		// fmt.Print("\n")
	}
	deeper(5, m)
	data, err = json.Marshal(m)
	is.NoErr(err)
	// fmt.Println(string(data))
	//
	// switch v := anything.(type) {
	// case string:
	// 	fmt.Println(v)
	// case int32, int64:
	// 	fmt.Println(v)
	// case map[string]interface{}:
	// 	fmt.Println(v)

	// default:
	// 	fmt.Println("unknown")
	// }

	// fmt.Printf("Key: %s, Value: %T\n", k, v)
	// }

}
