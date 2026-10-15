package main

import "testing"

func TestEnvOrDefault(t *testing.T) {
	t.Run("environment value", func(t *testing.T) {
		t.Setenv(envSMTPHost, "smtp.example.com")
		if got := envOrDefault(envSMTPHost, "fallback"); got != "smtp.example.com" {
			t.Fatalf("got %q, expected environment value", got)
		}
	})

	t.Run("fallback", func(t *testing.T) {
		t.Setenv(envSMTPHost, "")
		if got := envOrDefault(envSMTPHost, "fallback"); got != "fallback" {
			t.Fatalf("got %q, expected fallback", got)
		}
	})
}

func TestEnvIntOrDefault(t *testing.T) {
	t.Run("environment value", func(t *testing.T) {
		t.Setenv(envSMTPPort, "587")
		if got := envIntOrDefault(envSMTPPort, 1025); got != 587 {
			t.Fatalf("got %d, expected 587", got)
		}
	})

	for _, value := range []string{"", "invalid"} {
		t.Run("fallback for "+value, func(t *testing.T) {
			t.Setenv(envSMTPPort, value)
			if got := envIntOrDefault(envSMTPPort, 1025); got != 1025 {
				t.Fatalf("got %d, expected fallback 1025", got)
			}
		})
	}
}
