package consul

type Config struct {
	Endpoint string
	Name     string `key:"-"`
}
