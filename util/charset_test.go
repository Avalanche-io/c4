package util_test

import (
	"testing"

	c4id "github.com/avalanche-io/c4/id"
	c4util "github.com/avalanche-io/c4/util"
	"github.com/cheekybits/is"
)

func TestCheckCharacterSet(t *testing.T) {
	is := is.New(t)
	tests := []struct {
		A       string
		B       string
		IsError bool
		ID      string
	}{
		{
			A:       "c41111uoSFHtMmbcTLJmFYvnjuA8EAMQgHQbE3zwmzuoM2epGzDeTvjPYeeuHvzGheFqA1x4qe84x7UVPrRxMLUiRr",
			B:       "c41111VPsgiUnMBCtmjMgyWNKVb8fbnqGiqBf3aXMaVPn2EQhaeEtWKpyEEViWahHEgRb1Y4RE84Y7uvpSrYnmuJrS",
			IsError: false,
			ID:      "c41111VPsgiUnMBCtmjMgyWNKVb8fbnqGiqBf3aXMaVPn2EQhaeEtWKpyEEViWahHEgRb1Y4RE84Y7uvpSrYnmuJrS",
		},
		{
			A:       "c41111VPsgiUnMBCtmjMgyWNKVb8fbnqGiqBf3aXMaVPn2EQhaeEtWKpyEEViWahHEgRb1Y4RE84Y7uvpSrYnmuJrS",
			B:       "c41111uoSFHtMmbcTLJmFYvnjuA8EAMQgHQbE3zwmzuoM2epGzDeTvjPYeeuHvzGheFqA1x4qe84x7UVPrRxMLUiRr",
			IsError: false,
			ID:      "c41111VPsgiUnMBCtmjMgyWNKVb8fbnqGiqBf3aXMaVPn2EQhaeEtWKpyEEViWahHEgRb1Y4RE84Y7uvpSrYnmuJrS",
		},
		{
			A:       "c41111uoSFHtMmbcTLJmFYvnjuA8EAMQgHQbE3zwmzuoM2epGzDeTvjPYeeuHvzGheFqA1x4qe84x7UVPrRxMLUiRr",
			B:       "c42fcSBQEaPKmsmwJqr2GFGQbDjMfdhtbxq9WWJCe5aZ2XnQwETF5nkjR3zt5KqcWy88ay6de1NeCXGHP5tgxA4W2t",
			IsError: true,
			ID:      "",
		},
		{
			A:       "c41111VPsgiUnMBCtmjMgyWNKVb8fbnqGiqBf3aXMaVPn2EQhaeEtWKpyEEViWahHEgRb1Y4qe84x7UVPrRxMLUiRr",
			B:       "c41111uoSFHtMmbcTLJmFYvnjuA8EAMQgHQbE3zwmzuoM2epGzDeTvjPYeeuHvzGheFqA1x4RE84Y7uvpSrYnmuJrS",
			IsError: true,
			ID:      "",
		},
		{
			A:       "c41111uoSFHtMmbcTLJmFYvnjuA8EAMQgHQbE3zwmzuoM2epGzDeTvjPYeeuHvzGheFqA1x4RE84Y7uvpSrYnmuJrS",
			B:       "c41111VPsgiUnMBCtmjMgyWNKVb8fbnqGiqBf3aXMaVPn2EQhaeEtWKpyEEViWahHEgRb1Y4qe84x7UVPrRxMLUiRr",
			IsError: true,
			ID:      "",
		},
		{
			A:       "",
			B:       "c41111VPsgiUnMBCtmjMgyWNKVb8fbnqGiqBf3aXMaVPn2EQhaeEtWKpyEEViWahHEgRb1Y4qe84x7UVPrRxMLUiRr",
			IsError: true,
			ID:      "",
		},
	}

	for _, test := range tests {
		a, _ := c4id.Parse(test.A)
		b, _ := c4id.Parse(test.B)
		id, err := c4util.CheckCharacterSet(a, b)
		if test.IsError {
			is.Err(err)
			is.Nil(id)
			continue
		}
		test_id, _ := c4id.Parse(test.ID)
		is.Equal(id.String(), test_id.String())
	}
}

func TestOldCharsetIDToNew(t *testing.T) {
	is := is.New(t)
	newid, _ := c4id.Parse("c41111VPsgiUnMBCtmjMgyWNKVb8fbnqGiqBf3aXMaVPn2EQhaeEtWKpyEEViWahHEgRb1Y4RE84Y7uvpSrYnmuJrS")
	oldid, _ := c4id.Parse("c41111uoSFHtMmbcTLJmFYvnjuA8EAMQgHQbE3zwmzuoM2epGzDeTvjPYeeuHvzGheFqA1x4qe84x7UVPrRxMLUiRr")
	id := c4util.OldCharsetIDToNew(oldid)
	is.NotNil(id)
	is.Equal(newid.String(), id.String())
	id = c4util.OldCharsetIDToNew(nil)
	is.Nil(id)
}

func TestNewCharsetIDToOld(t *testing.T) {
	is := is.New(t)
	newid, _ := c4id.Parse("c41111VPsgiUnMBCtmjMgyWNKVb8fbnqGiqBf3aXMaVPn2EQhaeEtWKpyEEViWahHEgRb1Y4RE84Y7uvpSrYnmuJrS")
	oldid, _ := c4id.Parse("c41111uoSFHtMmbcTLJmFYvnjuA8EAMQgHQbE3zwmzuoM2epGzDeTvjPYeeuHvzGheFqA1x4qe84x7UVPrRxMLUiRr")
	id := c4util.NewCharsetIDToOld(newid)
	is.NotNil(id)
	is.Equal(oldid.String(), id.String())
	id = c4util.NewCharsetIDToOld(nil)
	is.Nil(id)
}
