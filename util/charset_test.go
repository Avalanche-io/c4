package util_test

import (
	"testing"

	c4id "github.com/Avalanche-io/c4/id"
	c4util "github.com/Avalanche-io/c4/util"
)

func TestCheckCharacterSet(t *testing.T) {
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
			if err == nil {
				t.Errorf("failed to receive expected error")
			}

			if id != nil {
				t.Errorf("expected nil, but received value")
			}
			continue
		}
		test_id, _ := c4id.Parse(test.ID)
		if id.String() != test_id.String() {
			t.Errorf("expected ids to be equal got %q, expected %q", test_id, id)
		}
	}
}

func TestOldCharsetIDToNew(t *testing.T) {
	newid, _ := c4id.Parse("c41111VPsgiUnMBCtmjMgyWNKVb8fbnqGiqBf3aXMaVPn2EQhaeEtWKpyEEViWahHEgRb1Y4RE84Y7uvpSrYnmuJrS")
	oldid, _ := c4id.Parse("c41111uoSFHtMmbcTLJmFYvnjuA8EAMQgHQbE3zwmzuoM2epGzDeTvjPYeeuHvzGheFqA1x4qe84x7UVPrRxMLUiRr")

	id := c4util.OldCharsetIDToNew(oldid)
	if id == nil {
		t.Fatalf("expected value, but received nil")
	}
	if newid.String() != id.String() {
		t.Errorf("expected ids to be equal got %q, expected %q", id, newid)
	}

	id = c4util.OldCharsetIDToNew(nil)
	if id != nil {
		t.Errorf("expected nil, but received value")
	}
}

func TestNewCharsetIDToOld(t *testing.T) {
	newid, _ := c4id.Parse("c41111VPsgiUnMBCtmjMgyWNKVb8fbnqGiqBf3aXMaVPn2EQhaeEtWKpyEEViWahHEgRb1Y4RE84Y7uvpSrYnmuJrS")
	oldid, _ := c4id.Parse("c41111uoSFHtMmbcTLJmFYvnjuA8EAMQgHQbE3zwmzuoM2epGzDeTvjPYeeuHvzGheFqA1x4qe84x7UVPrRxMLUiRr")

	id := c4util.NewCharsetIDToOld(newid)
	if id == nil {
		t.Fatalf("expected value, but received nil")
	}
	if oldid.String() != id.String() {
		t.Errorf("expected ids to be equal got %q, expected %q", id, oldid)
	}

	id = c4util.NewCharsetIDToOld(nil)
	if id != nil {
		t.Errorf("expected nil, but received value")
	}
}
