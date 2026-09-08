package checksum

import "testing"

// TestControllerImplementsChecksum verifies at compile time that
// *ControllerChecksum satisfies the Checksum interface.
func TestControllerImplementsChecksum(t *testing.T) {
	var _ Checksum = NewChecksumController()
}

func TestNewChecksumController(t *testing.T) {
	c := NewChecksumController()
	if c == nil {
		t.Fatal("expected non-nil controller")
	}
	if p := c.Calculate([]byte("data")); p != "" {
		t.Errorf("Calculate() = %q, want empty string", p)
	}
	if !c.Validate("anything") {
		t.Errorf("Validate() = false, want true")
	}
	if s := c.String(); s != "" {
		t.Errorf("String() = %q, want empty string", s)
	}
	if err := c.Configure(map[string]string{"k": "v"}); err != nil {
		t.Errorf("Configure() = %v, want nil", err)
	}
}
