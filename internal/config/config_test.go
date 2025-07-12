package config

import "testing"

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "Valid config",
			config: &Config{
				URL:       "https://example.com",
				OutputDir: "./output",
				Workers:   1,
			},
			wantErr: false,
		},
		{
			name: "Empty URL",
			config: &Config{
				URL:       "",
				OutputDir: "./output",
				Workers:   1,
			},
			wantErr: true,
		},
		{
			name: "Invalid URL",
			config: &Config{
				URL:       "not-a-url",
				OutputDir: "./output",
				Workers:   1,
			},
			wantErr: true,
		},
		{
			name: "Zero workers",
			config: &Config{
				URL:       "https://example.com",
				OutputDir: "./output",
				Workers:   0,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
