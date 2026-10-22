package main
import "testing"

func TestValidateInput(t *testing.T) {
	tests := []struct {
		name    string
		input   LinkInput
		wantErr bool
	}{
		{
			name: "valid input",
			input: LinkInput{
				URL:   "https://keploy.io",
				Title: "Keploy",
			},
			wantErr: false,
		},
		{
			name: "missing title",
			input: LinkInput{
				URL: "https://keploy.io",
			},
			wantErr: true,
		},
		{
			name: "unsupported scheme",
			input: LinkInput{
				URL:   "ftp://example.com",
				Title: "Example",
			},
			wantErr: true,
		},
		{
			name: "missing URL",
			input: LinkInput{
				Title: "Keploy",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateInput(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("got error %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
