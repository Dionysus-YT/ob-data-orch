package exportdomain

import "testing"

func TestValidateControlledStorageURI(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		outputKind string
		uri        string
		wantErr    bool
	}{
		{name: "oss with allowed query", outputKind: "OSS", uri: "oss://bucket/export?endpoint=storage.example.test&region=cn-test-1", wantErr: false},
		{name: "s3 with storage class", outputKind: "S3", uri: "s3://bucket/export?storage-class=standard", wantErr: false},
		{name: "missing path", outputKind: "OSS", uri: "oss://bucket", wantErr: true},
		{name: "scheme mismatch", outputKind: "S3", uri: "oss://bucket/export", wantErr: true},
		{name: "unsupported query", outputKind: "COS", uri: "cos://bucket/export?access-key=synthetic", wantErr: true},
		{name: "unknown scheme", outputKind: "OBS", uri: "file://bucket/export", wantErr: true},
		{name: "line break", outputKind: "OBS", uri: "obs://bucket/export\n", wantErr: true},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := ValidateControlledStorageURI(test.outputKind, test.uri); (err != nil) != test.wantErr {
				t.Fatalf("ValidateControlledStorageURI(%q, %q) error=%v wantErr=%v", test.outputKind, test.uri, err, test.wantErr)
			}
		})
	}
}
