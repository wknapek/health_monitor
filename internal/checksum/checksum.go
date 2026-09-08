package checksum

type Checksum interface {
	Calculate(data []byte) string
	Validate(checksum string) bool
	String() string
	Configure(config map[string]string) error
}
