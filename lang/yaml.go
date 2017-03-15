package lang

import (
	"fmt"
	"io"

	"gopkg.in/yaml.v2"
)

type Graph interface{}

func YamlLoad(f io.Reader) (Graph, error) {
	data := make([]byte, 0, 1024)
	input := make([]byte, 1024)
	total := 0
	for {
		n, err := f.Read(input)
		total += n
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		data = append(data, input...)
	}
	data = data[:total]
	var t Graph
	err := yaml.Unmarshal([]byte(data), &t)
	fmt.Printf("yaml\n%s\n", string(data))
	fmt.Printf("yaml object\n%v\n", t)
	return t, err
}
