package store

type Attribute struct {
	v interface{}
	t AttributeType
}

type Attributes map[string]*Attribute

type AttributeType int8

const (
	NullType AttributeType = iota
	IntegerType
	StringType
	FloatType
	ListType
	AttributesType
)

func NullAttribute() *Attribute {
	return &Attribute{nil, NullType}
}

func IntAttribute(value int) *Attribute {
	return &Attribute{value, IntegerType}
}

func StringAttribute(value string) *Attribute {
	return &Attribute{value, StringType}
}

func FloatAttribute(value float64) *Attribute {
	return &Attribute{value, FloatType}
}

func ListAttribute(value []*Attribute) *Attribute {
	return &Attribute{value, ListType}
}

func AttributesAttribute(value Attributes) *Attribute {
	return &Attribute{value, AttributesType}
}
