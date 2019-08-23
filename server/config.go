package server

type Config struct {
	Name string
	Endpoint    string
	LogPayloads *bool
}

func (c *Config) withDefaults() {
	if c.LogPayloads == nil {
		t := true
		c.LogPayloads = &t
	}
}
