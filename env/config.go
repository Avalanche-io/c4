package env

type Config struct {
	// Root path for c4 configuration and storage
	Root *string
}

// NewConfig returns a new Config pointer that can be chained with builder methods to
// set multiple configuration values inline without using pointers.
//
//     svc := storage.NewAttributeStore(c4.NewConfig().WithRoot("/mnt/c4"))
//
func NewConfig() *Config {
	return &Config{}
}

// WithRoot sets the Root path and returns a Config pointer for chaining.
func (c *Config) WithRoot(path string) *Config {
	c.Root = &path
	return c
}

// Merge merges an array of configs.
func (c *Config) Merge(cfgs ...*Config) {
	for _, src := range cfgs {
		mergeConfigs(c, src)
	}
}

func mergeConfigs(dst *Config, src *Config) {
	if src == nil {
		return
	}

	if src.Root != nil {
		dst.Root = src.Root
	}
}
