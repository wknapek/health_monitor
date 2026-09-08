package checksum

type ControllerChecksum struct {
}

func NewChecksumController() *ControllerChecksum {
	return &ControllerChecksum{}
}

func (e ControllerChecksum) Calculate(data []byte) string {
	//TODO implement me
	return ""
}

func (e ControllerChecksum) Validate(checksum string) bool {
	//TODO implement me
	return true
}

func (e ControllerChecksum) String() string {
	//TODO implement me
	return ""
}

func (e ControllerChecksum) Configure(config map[string]string) error {
	//TODO implement me
	return nil
}
