package featuregate

import "testing"

func TestRequireRealExecutionDisabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		exists  bool
		wantErr bool
	}{
		{name: "unset", exists: false},
		{name: "empty", exists: true},
		{name: "explicit false", value: "false", exists: true},
		{name: "explicit zero", value: "0", exists: true},
		{name: "enable rejected", value: "true", exists: true, wantErr: true},
		{name: "invalid rejected", value: "sometimes", exists: true, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := RequireRealExecutionDisabled(func(string) (string, bool) {
				return tt.value, tt.exists
			})
			if (err != nil) != tt.wantErr {
				t.Fatalf("RequireRealExecutionDisabled() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
