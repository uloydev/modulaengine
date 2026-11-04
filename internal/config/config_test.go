package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func cleanEnv(t *testing.T, keys ...string) {
	t.Helper()
	for _, k := range keys {
		old, exists := os.LookupEnv(k)
		_ = os.Unsetenv(k)
		if exists {
			t.Cleanup(func() { _ = os.Setenv(k, old) })
		} else {
			t.Cleanup(func() { _ = os.Unsetenv(k) })
		}
	}
}

func TestParseEnv(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected map[string]string
		wantErr  bool
	}{
		{
			name:     "simple_key_value",
			input:    "PORT=8080\nENV=development",
			expected: map[string]string{"PORT": "8080", "ENV": "development"},
		},
		{
			name:     "whitespace_trimming",
			input:    "   PORT   =   8080   \n  LOG_LEVEL = info  ",
			expected: map[string]string{"PORT": "8080", "LOG_LEVEL": "info"},
		},
		{
			name: "empty_lines_and_comments",
			input: `
# Server configuration
PORT=8080

# Logging
LOG_LEVEL=warn
`,
			expected: map[string]string{"PORT": "8080", "LOG_LEVEL": "warn"},
		},
		{
			name:     "inline_comment_unquoted_with_space",
			input:    "PORT=8080 # default server port\nENV=production # target env",
			expected: map[string]string{"PORT": "8080", "ENV": "production"},
		},
		{
			name:     "inline_comment_unquoted_no_space",
			input:    "PORT=8080#comment\nFLAG=true#inline",
			expected: map[string]string{"PORT": "8080", "FLAG": "true"},
		},
		{
			name:     "double_quoted_with_hash",
			input:    `SECRET="raw#password#with#hashes" # inline comment`,
			expected: map[string]string{"SECRET": "raw#password#with#hashes"},
		},
		{
			name:     "single_quoted_with_hash",
			input:    `SECRET='raw#password#with#hashes' # inline comment`,
			expected: map[string]string{"SECRET": "raw#password#with#hashes"},
		},
		{
			name:     "double_quoted_escape_sequences",
			input:    `MESSAGE="hello\nworld\t\"escaped\"\\done"`,
			expected: map[string]string{"MESSAGE": "hello\nworld\t\"escaped\"\\done"},
		},
		{
			name:     "single_quoted_literal",
			input:    `MESSAGE='literal\nvalue'`,
			expected: map[string]string{"MESSAGE": `literal\nvalue`},
		},
		{
			name:     "empty_values",
			input:    "EMPTY=\nEMPTY_D=\"\"\nEMPTY_S=''",
			expected: map[string]string{"EMPTY": "", "EMPTY_D": "", "EMPTY_S": ""},
		},
		{
			name:     "value_with_multiple_equals",
			input:    "DB_URL=postgres://u:p@localhost:5432/db?sslmode=disable",
			expected: map[string]string{"DB_URL": "postgres://u:p@localhost:5432/db?sslmode=disable"},
		},
		{
			name:     "export_prefix",
			input:    "export PORT=3000\nexport ENV=production",
			expected: map[string]string{"PORT": "3000", "ENV": "production"},
		},
		{
			name:    "invalid_missing_equals",
			input:   "INVALID_LINE_WITHOUT_EQUALS",
			wantErr: true,
		},
		{
			name:    "invalid_empty_key",
			input:   "=VALUE",
			wantErr: true,
		},
		{
			name:    "invalid_unclosed_double_quote",
			input:   `FOO="unclosed`,
			wantErr: true,
		},
		{
			name:    "invalid_unclosed_single_quote",
			input:   `FOO='unclosed`,
			wantErr: true,
		},
		{
			name:    "invalid_trailing_content_after_quote",
			input:   `FOO="valid"extra_garbage`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseEnv([]byte(tt.input))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseEnv() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseEnv() unexpected error: %v", err)
			}
			if len(got) != len(tt.expected) {
				t.Fatalf("ParseEnv() length mismatch: got %d, expected %d", len(got), len(tt.expected))
			}
			for k, wantV := range tt.expected {
				if gotV, ok := got[k]; !ok || gotV != wantV {
					t.Errorf("ParseEnv() key %q = %q; expected %q", k, gotV, wantV)
				}
			}
		})
	}
}

func TestLoadEnvFile(t *testing.T) {
	testKeys := []string{
		"MODULA_TEST_PRESERVE",
		"MODULA_TEST_NEW",
	}

	t.Run("optional_missing_empty_path", func(t *testing.T) {
		cleanEnv(t, testKeys...)
		origDir, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		tempDir := t.TempDir()
		if err := os.Chdir(tempDir); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.Chdir(origDir) }()

		if err := LoadEnvFile(""); err != nil {
			t.Fatalf("expected nil for missing optional .env, got %v", err)
		}
	})

	t.Run("optional_missing_dot_env_path", func(t *testing.T) {
		cleanEnv(t, testKeys...)
		origDir, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		tempDir := t.TempDir()
		if err := os.Chdir(tempDir); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.Chdir(origDir) }()

		if err := LoadEnvFile(".env"); err != nil {
			t.Fatalf("expected nil for missing optional .env, got %v", err)
		}
	})

	t.Run("explicit_missing_file_returns_error", func(t *testing.T) {
		cleanEnv(t, testKeys...)
		err := LoadEnvFile("nonexistent-file.env")
		if err == nil {
			t.Fatal("expected error for explicit nonexistent file, got nil")
		}
	})

	t.Run("os_env_precedence_over_env_file", func(t *testing.T) {
		cleanEnv(t, testKeys...)
		tempDir := t.TempDir()
		envPath := filepath.Join(tempDir, "test.env")
		content := "MODULA_TEST_PRESERVE=from_file\nMODULA_TEST_NEW=from_file\n"
		if err := os.WriteFile(envPath, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}

		t.Setenv("MODULA_TEST_PRESERVE", "from_os")

		if err := LoadEnvFile(envPath); err != nil {
			t.Fatalf("LoadEnvFile() error: %v", err)
		}

		if got := os.Getenv("MODULA_TEST_PRESERVE"); got != "from_os" {
			t.Errorf("expected MODULA_TEST_PRESERVE=from_os, got %q", got)
		}
		if got := os.Getenv("MODULA_TEST_NEW"); got != "from_file" {
			t.Errorf("expected MODULA_TEST_NEW=from_file, got %q", got)
		}
	})
}

func TestLoad_Precedence(t *testing.T) {
	testEnvKeys := []string{
		"PORT",
		"LOG_LEVEL",
		"HEADLESS",
		"ENV",
		"DATABASE_URL",
		"DATABASE_CONTROL_URL",
		"DATABASE_READ_URL",
		"JWT_SECRET",
		"LOG_FILE",
		"LOG_CONSOLE",
	}

	tests := []struct {
		name         string
		envFileBody  string
		osEnvs       map[string]string
		flags        FlagOverrides
		expectedPort int
		expectedLog  string
		expectedHead bool
		expectedEnv  string
	}{
		{
			name:         "defaults_only",
			flags:        FlagOverrides{},
			expectedPort: 8080,
			expectedLog:  "info",
			expectedHead: false,
			expectedEnv:  "development",
		},
		{
			name:        "env_file_overrides_defaults",
			envFileBody: "PORT=9000\nLOG_LEVEL=warn\nHEADLESS=true\nENV=staging\n",
			flags:       FlagOverrides{},
			expectedPort: 9000,
			expectedLog:  "warn",
			expectedHead: true,
			expectedEnv:  "staging",
		},
		{
			name:        "os_env_overrides_env_file",
			envFileBody: "PORT=9000\nLOG_LEVEL=warn\nHEADLESS=true\n",
			osEnvs: map[string]string{
				"PORT":      "9001",
				"LOG_LEVEL": "error",
				"HEADLESS":  "false",
			},
			flags:        FlagOverrides{},
			expectedPort: 9001,
			expectedLog:  "error",
			expectedHead: false,
			expectedEnv:  "development",
		},
		{
			name:        "cli_flags_override_os_env",
			envFileBody: "PORT=9000\nLOG_LEVEL=warn\n",
			osEnvs: map[string]string{
				"PORT":      "9001",
				"LOG_LEVEL": "error",
			},
			flags: func() FlagOverrides {
				p := 9002
				lvl := "debug"
				h := true
				return FlagOverrides{
					Port:     &p,
					LogLevel: &lvl,
					Headless: &h,
				}
			}(),
			expectedPort: 9002,
			expectedLog:  "debug",
			expectedHead: true,
			expectedEnv:  "development",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanEnv(t, testEnvKeys...)

			tempDir := t.TempDir()
			envPath := ""
			if tt.envFileBody != "" {
				envPath = filepath.Join(tempDir, ".env")
				if err := os.WriteFile(envPath, []byte(tt.envFileBody), 0600); err != nil {
					t.Fatal(err)
				}
			}

			for k, v := range tt.osEnvs {
				t.Setenv(k, v)
			}

			cfg, err := Load(envPath, tt.flags)
			if err != nil {
				t.Fatalf("Load() unexpected error: %v", err)
			}

			if cfg.Server.Port != tt.expectedPort {
				t.Errorf("Port = %d, expected %d", cfg.Server.Port, tt.expectedPort)
			}
			if cfg.Log.Level != tt.expectedLog {
				t.Errorf("LogLevel = %s, expected %s", cfg.Log.Level, tt.expectedLog)
			}
			if cfg.Server.Headless != tt.expectedHead {
				t.Errorf("Headless = %v, expected %v", cfg.Server.Headless, tt.expectedHead)
			}
			if cfg.Server.Env != tt.expectedEnv {
				t.Errorf("Env = %s, expected %s", cfg.Server.Env, tt.expectedEnv)
			}
		})
	}
}

func TestLoad_SecurityValidation_Production(t *testing.T) {
	testEnvKeys := []string{
		"PORT",
		"LOG_LEVEL",
		"HEADLESS",
		"ENV",
		"JWT_SECRET",
	}

	tests := []struct {
		name      string
		env       string
		jwtSecret string
		wantErr   error
	}{
		{
			name:      "dev_mode_allows_default_secret",
			env:       "development",
			jwtSecret: DefaultDevJWTSecret,
			wantErr:   nil,
		},
		{
			name:      "dev_mode_allows_short_secret",
			env:       "development",
			jwtSecret: "short",
			wantErr:   nil,
		},
		{
			name:      "prod_mode_rejects_default_dev_secret",
			env:       "production",
			jwtSecret: DefaultDevJWTSecret,
			wantErr:   ErrWeakSecret,
		},
		{
			name:      "prod_mode_rejects_example_dev_secret",
			env:       "production",
			jwtSecret: "super-secret-jwt-signing-key-change-in-production",
			wantErr:   ErrWeakSecret,
		},
		{
			name:      "prod_mode_rejects_secret_under_32_bytes",
			env:       "production",
			jwtSecret: "only-25-bytes-secret-here",
			wantErr:   ErrWeakSecret,
		},
		{
			name:      "prod_mode_accepts_strong_secret",
			env:       "production",
			jwtSecret: "a-very-strong-production-jwt-secret-with-at-least-32-chars",
			wantErr:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanEnv(t, testEnvKeys...)

			t.Setenv("ENV", tt.env)
			t.Setenv("JWT_SECRET", tt.jwtSecret)

			cfg, err := Load("", FlagOverrides{})
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Load() expected error %v, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() unexpected error: %v", err)
			}
			if string(cfg.Security.JWTSecret) != tt.jwtSecret {
				t.Errorf("JWTSecret = %s, expected %s", string(cfg.Security.JWTSecret), tt.jwtSecret)
			}
		})
	}
}
