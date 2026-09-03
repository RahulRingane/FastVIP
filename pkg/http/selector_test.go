package http

import "testing"

func TestNextBackend_RoundRobin(t *testing.T) {
	service := &Service{
		Name: "test",
		Backends: []string{
			"backend-1:8080",
			"backend-2:8080",
			"backend-3:8080",
		},
	}

	expected := []string{
		"backend-1:8080",
		"backend-2:8080",
		"backend-3:8080",
		"backend-1:8080",
		"backend-2:8080",
		"backend-3:8080",
	}

	for _, want := range expected {
		got := service.nextBackend()

		if got != want {
			t.Fatalf("expected %s, got %s", want, got)
		}
	}
}
